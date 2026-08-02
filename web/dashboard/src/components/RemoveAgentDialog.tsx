import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { ApiError, deleteDashboardNode, type ProxyNode } from "@/lib/api";
import { toast } from "@/lib/toast";

interface RemoveAgentDialogProps {
  node: ProxyNode | null;
  token: string | null;
  onClose: () => void;
  onRemoved: (nodeId: string) => void;
}

export function RemoveAgentDialog({
  node,
  token,
  onClose,
  onRemoved,
}: RemoveAgentDialogProps) {
  const [isRemoving, setIsRemoving] = useState(false);

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
          To uninstall the agent remotely first, use{" "}
          <span className="font-medium text-foreground">Uninstall agent remotely</span>{" "}
          from the agent row context menu on the Agents page.
        </p>
      </div>
    </Dialog>
  );
}
