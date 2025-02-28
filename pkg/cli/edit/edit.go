package edit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/kubectl/pkg/cmd/util/editor"
)

var (
	envs = []string{
		"OTTO_EDITOR",
		"EDITOR",
	}
)

func stripComments(buf []byte) []byte {
	var (
		result = bytes.Buffer{}
		start  = true
	)
	for _, line := range strings.Split(string(buf), "\n") {
		if start {
			if strings.HasPrefix(line, "# ") {
				continue
			}
			start = false
		}
		result.WriteString(line)
		result.WriteString("\n")
	}
	return result.Bytes()
}

func commentError(err error, buf []byte) []byte {
	var header bytes.Buffer
	for _, line := range strings.Split(strings.ReplaceAll(err.Error(), "\r", ""), "\n") {
		header.WriteString("# ")
		header.WriteString(line)
		header.WriteString("\n")
	}
	return append(header.Bytes(), buf...)
}

func Edit(ctx context.Context, content []byte, suffix string, save func([]byte) error) error {
	editor := editor.NewDefaultEditor(envs)
	for {
		buf, file, err := editor.LaunchTempFile("otto", suffix, bytes.NewReader(content))
		if file != "" {
			_ = os.Remove(file)
		}
		if err != nil {
			return err
		}

		if bytes.Equal(buf, content) {
			return fmt.Errorf("aborted")
		}

		buf = stripComments(buf)

		if err := save(buf); err != nil {
			content = commentError(err, buf)
			continue
		}

		return nil
	}
}
