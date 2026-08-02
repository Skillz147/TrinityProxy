import { useEffect, useState } from "react";
import { AlertTriangle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import {
  ApiError,
  enqueueNodeCommand,
  type DeployLogLevel,
  type ProxyNode,
  type RemoteCommandAction,
} from "@/lib/api";
import { toast } from "@/lib/toast";

const ACTION_LABELS: Record<
  RemoteCommandAction,
  { title: string; confirm: string; destructive?: boolean }
> = {
  uninstall: {
    title: "Uninstall agent remotely",
    confirm: "Uninstall remotely",
    destructive: true,
  },
  restart: {
    title: "Restart agent",
    confirm: "Restart agent",
  },
  status: {
    title: "Check agent status",
    confirm: "Run status check",
  },
  repair: {
    title: "Repair / reinstall agent",
    confirm: "Run repair",
  },
};

const LOG_LEVEL_OPTIONS: { id: DeployLogLevel; label: string }[] = [
  { id: "quiet", label: "Quiet" },
  { id: "silent", label: "Silent" },
  { id: "info", label: "Info" },
  { id: "debug", label: "Debug" },
];

interface AgentRemoteActionDialogProps {
  node: ProxyNode | null;
  action: RemoteCommandAction | null;
  token: string | null;
  onClose: () => void;
  onQueued: () => void;
}

export function AgentRemoteActionDialog({
  node,
  action,
  token,
  onClose,
  onQueued,
}: AgentRemoteActionDialogProps) {
  const [logLevel, setLogLevel] = useState<DeployLogLevel>("info");
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!node || !action) {
      setLogLevel("info");
      setIsSubmitting(false);
    }
  }, [node, action]);

  const meta = action ? ACTION_LABELS[action] : null;
  const agentLabel = node ? `${node.ip}:${node.port}` : "";
  const showLogLevel = action === "repair";

  const handleSubmit = async () => {
    if (!node || !action) return;

    setIsSubmitting(true);
    try {
      await enqueueNodeCommand(token, node.id, {
        action,
        log_level: showLogLevel ? logLevel : undefined,
      });
      toast.success(
        `${meta?.title ?? "Command"} queued — agent will run it on next heartbeat`,
      );
      onQueued();
      onClose();
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Unable to queue command.";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog
      open={node !== null && action !== null}
      onClose={onClose}
      title={meta?.title ?? "Remote action"}
      description={node ? `${agentLabel} · ${node.username}` : undefined}
      footer={
        <div className="flex justify-end gap-3">
          <Button variant="outline" onClick={onClose} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button
            variant={meta?.destructive ? "destructive" : "primary"}
            onClick={() => void handleSubmit()}
            disabled={isSubmitting}
          >
            {isSubmitting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Queuing…
              </>
            ) : (
              meta?.confirm ?? "Confirm"
            )}
          </Button>
        </div>
      }
    >
      <div className="space-y-4 text-sm">
        {action === "uninstall" && (
          <div className="flex items-start gap-2.5 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-muted-foreground">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
            <p>
              This queues an uninstall on the agent host. The service will stop,
              files will be removed, and the agent will deregister from the
              dashboard when the command completes successfully.
            </p>
          </div>
        )}

        {action === "restart" && (
          <p className="text-muted-foreground">
            Restarts the TrinityProxy agent service on the remote host. The SOCKS
            proxy will briefly go offline during the restart.
          </p>
        )}

        {action === "status" && (
          <p className="text-muted-foreground">
            Runs a status check on the agent (service state and SOCKS port). Results
            appear in the Command column after the agent heartbeats.
          </p>
        )}

        {action === "repair" && (
          <>
            <p className="text-muted-foreground">
              Re-runs the platform install/repair script on the agent. Useful when
              the service is broken or the binary is outdated.
            </p>
            <div className="space-y-2">
              <p className="font-medium text-foreground">Install log level</p>
              <div className="flex flex-wrap gap-2">
                {LOG_LEVEL_OPTIONS.map((option) => (
                  <Button
                    key={option.id}
                    variant={logLevel === option.id ? "primary" : "outline"}
                    size="sm"
                    onClick={() => setLogLevel(option.id)}
                  >
                    {option.label}
                  </Button>
                ))}
              </div>
            </div>
          </>
        )}

        {!node?.is_online && (
          <p className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-muted-foreground">
            This agent appears offline. The command stays queued until it
            heartbeats again.
          </p>
        )}
      </div>
    </Dialog>
  );
}
