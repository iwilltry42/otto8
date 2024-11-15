package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/api/server"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflow"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkflowHandler struct {
	gptscript *gptscript.GPTScript
	serverURL string
	invoker   *invoke.Invoker
}

func NewWorkflowHandler(gClient *gptscript.GPTScript, serverURL string, invoker *invoke.Invoker) *WorkflowHandler {
	return &WorkflowHandler{
		gptscript: gClient,
		serverURL: serverURL,
		invoker:   invoker,
	}
}

func (a *WorkflowHandler) Authenticate(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	agent, err := render.Workflow(req.Context(), req.Storage, &workflow, render.WorkflowOptions{})
	if err != nil {
		return err
	}

	agent.Spec.Manifest.Prompt = "#!sys.echo\nDONE"
	if len(agent.Spec.Credentials) == 0 {
		return nil
	}

	resp, err := a.invoker.Agent(req.Context(), req.Storage, agent, "", invoke.Options{
		Synchronous:           true,
		ThreadCredentialScope: new(bool),
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Otto-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (a *WorkflowHandler) Update(req api.Context) error {
	var (
		id       = req.PathValue("id")
		wf       v1.Workflow
		manifest types.WorkflowManifest
	)

	if err := req.Read(&manifest); err != nil {
		return err
	}

	manifest = workflow.PopulateIDs(manifest)

	if err := req.Get(&wf, id); err != nil {
		return err
	}

	wf.Spec.Manifest = manifest
	if err := req.Update(&wf); err != nil {
		return err
	}

	return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
}

func (a *WorkflowHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *WorkflowHandler) Create(req api.Context) error {
	var manifest types.WorkflowManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}
	manifest = workflow.PopulateIDs(manifest)
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&workflow); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return req.Write(convertWorkflow(workflow, server.GetURLPrefix(req)))
}

func convertWorkflow(workflow v1.Workflow, prefix string) *types.Workflow {
	var links []string
	if prefix != "" {
		refName := workflow.Name
		if workflow.Status.External.RefNameAssigned && workflow.Spec.Manifest.RefName != "" {
			refName = workflow.Spec.Manifest.RefName
		}
		links = []string{"invoke", prefix + "/invoke/" + refName}
	}
	return &types.Workflow{
		Metadata:               MetadataFrom(&workflow, links...),
		WorkflowManifest:       workflow.Spec.Manifest,
		WorkflowExternalStatus: workflow.Status.External,
	}
}

func (a *WorkflowHandler) ByID(req api.Context) error {
	var (
		workflow v1.Workflow
		id       = req.PathValue("id")
	)
	if system.IsWorkflowID(id) {
		if err := req.Get(&workflow, id); err != nil {
			return err
		}
	} else {
		var ref v1.Reference
		if err := req.Get(&ref, id); err != nil {
			return err
		}
		if ref.Spec.WorkflowName == "" {
			return types.NewErrNotFound("reference %q is not an agent reference", ref.Name)
		}
		if err := req.Get(&workflow, ref.Spec.WorkflowName); err != nil {
			return err
		}
	}

	return req.Write(convertWorkflow(workflow, server.GetURLPrefix(req)))
}

func (a *WorkflowHandler) List(req api.Context) error {
	var workflowList v1.WorkflowList
	if err := req.List(&workflowList); err != nil {
		return err
	}

	var resp types.WorkflowList
	for _, workflow := range workflowList.Items {
		resp.Items = append(resp.Items, *convertWorkflow(workflow, server.GetURLPrefix(req)))
	}

	return req.Write(resp)
}

func (a *WorkflowHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}

	ref := req.PathValue("ref")
	authStatus := wf.Status.External.AuthStatus[ref]

	// If auth is not required, then don't continue.
	if authStatus.Required != nil && !*authStatus.Required {
		return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
	}

	// if auth is already authenticated, then don't continue.
	if authStatus.Authenticated {
		return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
	}

	credentialTool, err := v1.CredentialTool(req.Context(), req.Storage, req.Namespace(), ref)
	if err != nil {
		return err
	}

	if credentialTool == "" {
		// The only way to get here is if the controller hasn't set the field yet.
		if wf.Status.External.AuthStatus == nil {
			wf.Status.External.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}

		authStatus.Required = &[]bool{false}[0]
		wf.Status.External.AuthStatus[ref] = authStatus
		return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
	}

	oauthLogin := &v1.OAuthAppLogin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthAppLoginPrefix + wf.Name + ref,
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppLoginSpec{
			CredentialContext: wf.Name,
			ToolReference:     ref,
			OAuthApps:         wf.Spec.Manifest.OAuthApps,
		},
	}

	if err = req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err = wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) bool {
		return obj.Status.External.Authenticated || obj.Status.External.Error != "" || obj.Status.External.URL != ""
	}, wait.Option{
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure credential for workflow %q: %w", wf.Name, err)
	}

	// Don't need to actually update the knowledge ref, there is a controller that will do that.
	if wf.Status.External.AuthStatus == nil {
		wf.Status.External.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
	}
	wf.Status.External.AuthStatus[ref] = oauthLogin.Status.External

	return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
}

func (a *WorkflowHandler) Script(req api.Context) error {
	var (
		id     = req.Request.PathValue("id")
		stepID = req.Request.URL.Query().Get("step")
		wf     v1.Workflow
	)
	if err := req.Get(&wf, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	step, _ := types.FindStep(&wf.Spec.Manifest, stepID)
	agent, err := render.Workflow(req.Context(), req.Storage, &wf, render.WorkflowOptions{
		Step: step,
	})
	if err != nil {
		return err
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, agent, a.serverURL, render.AgentOptions{})
	if err != nil {
		return err
	}

	nodes := gptscript.ToolDefsToNodes(tools)
	nodes = append(nodes, gptscript.Node{
		TextNode: &gptscript.TextNode{
			Text: "!otto-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}
