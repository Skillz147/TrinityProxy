import { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Skeleton } from "@/components/ui/Skeleton";
import { normalizePlatform } from "@/components/StatusBadge";
import {
  ApiError,
  deleteDashboardNode,
  fetchDeployCommands,
  type DeployCommandsResponse,
  type ProxyNode,
} from "@/lib/api";
import { toast } from "@/lib/toast";

interface RemoveAgentDialogProps {
  node: ProxyNode | null;
  token: string | null;
  onClose: () => void;
  onRemoved: (nodeId: string) => void;
}

function resolveDeployPlatformId(node: ProxyNode): string {
  const platform = normalizePlatform(node.platform);
  switch (platform) {
    case "windows":
      return "windows";
    case "darwin":
      return "macos";
    case "linux":
    default:
      return "linux-vps";
  }
}

function findRemoveCommand(
  data: DeployCommandsResponse | null,
  node: ProxyNode,
): { command: string; description: string } | null {
  if (!data) return null;

  const platformId = resolveDeployPlatformId(node);
  const platform = data.platforms.find((item) => item.id === platformId);
  const removeOp = platform?.operations?.find((op) => op.id === "remove");
  if (!removeOp) return null;

  return {
    command: removeOp.command,
    description: removeOp.description,
  };
}

async function copyText(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
  } catch {
    toast.error("Unable to copy — check browser permissions");
  }
}

export function RemoveAgentDialog({
  node,
  token,
  onClose,
  onRemoved,
}: RemoveAgentDialogProps) {
  const [deployCommands, setDeployCommands] = useState<DeployCommandsResponse | null>(
    null,
  );
  const [isLoadingCommands, setIsLoadingCommands] = useState(false);
  const [isRemoving, setIsRemoving] = useState(false);

  const loadDeployCommands = useCallback(async () => {
    setIsLoadingCommands(true);
    try {
      const response = await fetchDeployCommands(token);
      setDeployCommands(response);
    } catch {
      setDeployCommands(null);
    } finally {
      setIsLoadingCommands(false);
    }
  }, [token]);

  useEffect(() => {
    if (!node) {
      setDeployCommands(null);
      setIsLoadingCommands(false);
      setIsRemoving(false);
      return;
    }

    void loadDeployCommands();
  }, [node, loadDeployCommands]);

  const removeCommand = useMemo(
    () => (node ? findRemoveCommand(deployCommands, node) : null),
    [deployCommands, node],
  );

  const handleRemove = async () => {
    if (!node) return;

    setIsRemoving(true);
    const label = `${node.ip}:${node.port}`;

    try {
      await deleteDashboardNode(token, node.id);
      onRemoved(node.id);
      onClose();
      toast.success(`Removed ${label} from dashboard`);
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Unable to remove agent.";
      toast.error(message);
    } finally {
      setIsRemoving(false);
    }
  };

  const agentLabel = node ? `${node.ip}:${node.port}` : "";

  return (
    <Dialog
      open={node !== null}
      onClose={onClose}
      title="Remove from dashboard"
      description={node ? `${agentLabel} · ${node.username}` : undefined}
      footer={
        <div className="flex justify-end gap-3">
          <Button variant="outline" onClick={onClose} disabled={isRemoving}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            leftIcon={<Trash2 className="h-4 w-4" />}
            onClick={() => void handleRemove()}
            disabled={isRemoving}
          >
            {isRemoving ? "Removing…" : "Remove from dashboard"}
          </Button>
        </div>
      }
    >
      <div className="space-y-4 text-sm">
        <p className="text-muted-foreground">
          This removes the agent registration from the TrinityProxy dashboard database
          only. It does <span className="font-medium text-foreground">not</span> stop
          or uninstall the agent on the remote machine.
        </p>
        <p className="text-muted-foreground">
          The agent will disappear from this list immediately. If it is still running
          on a VPS or desktop, it may register again on its next heartbeat unless you
          uninstall it there too.
        </p>

        {isLoadingCommands && (
          <div className="space-y-2" aria-busy="true" aria-label="Loading uninstall command">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-24 w-full" />
          </div>
        )}

        {!isLoadingCommands && removeCommand && (
          <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
            <p className="font-medium text-foreground">Uninstall on the agent host</p>
            <p className="text-muted-foreground">{removeCommand.description}</p>
            <pre className="max-h-48 overflow-auto rounded-md border border-border bg-background p-3 font-mono text-xs leading-relaxed text-foreground">
              {removeCommand.command}
            </pre>
            <Button
              variant="outline"
              size="sm"
              leftIcon={<Copy className="h-4 w-4" />}
              onClick={() =>
                void copyText(removeCommand.command, "Uninstall command copied")
              }
            >
              Copy uninstall command
            </Button>
          </div>
        )}

        {!isLoadingCommands && !removeCommand && node && (
          <p className="rounded-lg border border-border bg-muted/30 p-4 text-muted-foreground">
            To uninstall the agent on the host, use the Remove command on the{" "}
            <span className="font-medium text-foreground">Deploy Agent</span> page for
            this platform.
          </p>
        )}
      </div>
    </Dialog>
  );
}
