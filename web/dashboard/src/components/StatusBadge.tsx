import {
  Activity,
  AlertCircle,
  Building2,
  CheckCircle2,
  Home,
  Laptop,
  Monitor,
  Server,
  Terminal,
  XCircle,
} from "lucide-react";
import type { ProxyNode } from "@/lib/api";
import { cn } from "@/lib/utils";

type OnlineStatus = "online" | "offline";
type HealthStatus = "healthy" | "unhealthy";

const onlineStyles: Record<OnlineStatus, string> = {
  online: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  offline: "bg-muted text-muted-foreground",
};

const healthStyles: Record<HealthStatus, string> = {
  healthy: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  unhealthy: "bg-destructive/15 text-destructive",
};

export function OnlineBadge({ online }: { online: boolean }) {
  const status: OnlineStatus = online ? "online" : "offline";
  const Icon = online ? CheckCircle2 : XCircle;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        onlineStyles[status],
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {online ? "Online" : "Offline"}
    </span>
  );
}

export function HealthBadge({ healthy }: { healthy: boolean }) {
  const status: HealthStatus = healthy ? "healthy" : "unhealthy";
  const Icon = healthy ? Activity : AlertCircle;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        healthStyles[status],
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {healthy ? "Healthy" : "Unhealthy"}
    </span>
  );
}

const platformStyles = {
  linux: "bg-slate-500/15 text-slate-700 dark:text-slate-300",
  windows: "bg-sky-500/15 text-sky-700 dark:text-sky-400",
  darwin: "bg-zinc-500/15 text-zinc-700 dark:text-zinc-300",
} as const;

const platformLabels = {
  linux: "Linux",
  windows: "Windows",
  darwin: "Mac",
} as const;

const platformIcons = {
  linux: Terminal,
  windows: Monitor,
  darwin: Laptop,
} as const;

export type SupportedPlatform = keyof typeof platformLabels;

export function normalizePlatform(platform?: string): SupportedPlatform | null {
  const value = platform?.toLowerCase();
  if (value === "linux" || value === "windows" || value === "darwin") {
    return value;
  }
  return null;
}

export function PlatformBadge({ platform }: { platform?: string }) {
  const normalized = normalizePlatform(platform);
  if (!normalized) {
    return <span className="text-sm text-muted-foreground">—</span>;
  }

  const Icon = platformIcons[normalized];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        platformStyles[normalized],
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {platformLabels[normalized]}
    </span>
  );
}

export type AgentTypeKind = "vps" | "desktop" | "residential" | "dc";

const typeStyles: Record<AgentTypeKind, string> = {
  vps: "bg-indigo-500/15 text-indigo-700 dark:text-indigo-400",
  desktop: "bg-violet-500/15 text-violet-700 dark:text-violet-400",
  residential: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  dc: "bg-cyan-500/15 text-cyan-700 dark:text-cyan-400",
};

const typeLabels: Record<AgentTypeKind, string> = {
  vps: "VPS",
  desktop: "Desktop",
  residential: "Residential",
  dc: "DC",
};

const typeIcons: Record<AgentTypeKind, typeof Server> = {
  vps: Server,
  desktop: Monitor,
  residential: Home,
  dc: Building2,
};

export function resolveAgentType(node: Pick<ProxyNode, "device_class" | "network_type">): AgentTypeKind | null {
  const networkType = node.network_type?.toLowerCase();
  if (networkType === "residential") return "residential";
  if (networkType === "datacenter") return "dc";

  const deviceClass = node.device_class?.toLowerCase();
  if (deviceClass === "vps") return "vps";
  if (deviceClass === "desktop") return "desktop";

  return null;
}

export function TypeBadge({ node }: { node: Pick<ProxyNode, "device_class" | "network_type"> }) {
  const kind = resolveAgentType(node);
  if (!kind) {
    return <span className="text-sm text-muted-foreground">—</span>;
  }

  const Icon = typeIcons[kind];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        typeStyles[kind],
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {typeLabels[kind]}
    </span>
  );
}
