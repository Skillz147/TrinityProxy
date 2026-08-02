import { cn } from "@/lib/utils";
import type { AgentRemoteCommand } from "@/lib/api";

const STATUS_LABELS: Record<string, string> = {
  pending: "Pending",
  running: "Running",
  success: "Success",
  failure: "Failed",
};

const ACTION_LABELS: Record<string, string> = {
  uninstall: "Uninstall",
  restart: "Restart",
  status: "Status",
  repair: "Repair",
};

export function RemoteCommandBadge({
  command,
  compact = false,
}: {
  command?: AgentRemoteCommand | null;
  compact?: boolean;
}) {
  if (!command) {
    return <span className="text-xs text-muted-foreground">—</span>;
  }

  const statusClass =
    command.status === "success"
      ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"
      : command.status === "failure"
        ? "bg-destructive/10 text-destructive"
        : command.status === "running"
          ? "bg-primary/10 text-primary"
          : "bg-muted text-muted-foreground";

  return (
    <div className={cn("space-y-0.5", compact && "space-y-0")}>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-xs text-muted-foreground">
          {ACTION_LABELS[command.action] ?? command.action}
        </span>
        <span
          className={cn(
            "inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide",
            statusClass,
          )}
        >
          {STATUS_LABELS[command.status] ?? command.status}
        </span>
      </div>
      {!compact && command.result && command.status !== "pending" && (
        <p
          className="max-w-xs truncate font-mono text-[10px] text-muted-foreground"
          title={command.result}
        >
          {command.result}
        </p>
      )}
    </div>
  );
}
