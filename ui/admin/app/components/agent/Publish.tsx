import { Eye } from "lucide-react";
import { useState } from "react";

import { Agent } from "~/lib/model/agents";

import {
    TypographyMuted,
    TypographyMutedAccent,
} from "~/components/Typography";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogFooter,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

type PublishProps = {
    className?: string;
    agent: Agent;
    onChange: (agent: Agent) => void;
};

export function Publish({ className, agent, onChange }: PublishProps) {
    const [refName, setRefName] = useState(agent.refName);

    const handlePublish = () => {
        onChange({
            ...agent,
            refName,
        });
    };

    return (
        <Dialog>
            <DialogTrigger asChild>
                <Button className={className} variant="secondary" size="sm">
                    <Eye className="w-4 h-4" />
                    Publish
                </Button>
            </DialogTrigger>
            <DialogContent className="max-w-3xl p-10">
                <div className="flex justify-between items-center w-full gap-2 pt-6">
                    <DialogTitle className="font-normal text-md">
                        Enter a handle for this agent:
                    </DialogTitle>
                    <Input
                        className="w-1/2"
                        value={refName}
                        onChange={(e) => setRefName(e.target.value)}
                    />
                </div>
                <div className="space-y-4 py-4">
                    <TypographyMuted>
                        This agent will be available at:
                    </TypographyMuted>
                    <TypographyMutedAccent>
                        {`${window.location.protocol}//${window.location.host}/${refName}`}
                    </TypographyMutedAccent>
                    <TypographyMuted>
                        If you have another agent with this handle, you will
                        need to unpublish it before this agent can be accessed
                        at the above URL.
                    </TypographyMuted>
                </div>
                <DialogFooter>
                    <div className="w-full flex justify-center items-center gap-10 pt-4">
                        <DialogClose asChild>
                            <Button className="w-1/2" variant="outline">
                                Cancel
                            </Button>
                        </DialogClose>
                        <DialogClose asChild>
                            <Button className="w-1/2" onClick={handlePublish}>
                                <Eye className="w-4 h-4" />
                                Publish
                            </Button>
                        </DialogClose>
                    </div>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
