import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Globe,
  LayoutGrid,
  Monitor,
  Radio,
  RefreshCw,
  Rocket,
  Server,
  Settings,
  ShieldAlert,
  WifiOff,
} from "lucide-react";
import {
  DashboardPieChart,
  DashboardPieChartSkeleton,
  type PieChartDatum,
} from "@/components/DashboardPieChart";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import {
  HealthBadge,
  OnlineBadge,
  PlatformBadge,
  TypeBadge,
} from "@/components/StatusBadge";
import { Button } from "@/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/Card";
import { Skeleton, TableSkeleton } from "@/components/ui/Skeleton";
import { useAuth } from "@/context/AuthContext";
import { ApiError, fetchDashboardStats, type DashboardStats } from "@/lib/api";
import { cn } from "@/lib/utils";

type SystemHealth = DashboardStats["system_health"];

const healthBannerStyles: Record<
  SystemHealth,
  { bg: string; border: string; icon: typeof CheckCircle2; label: string; description: string }
> = {
  healthy: {
    bg: "bg-emerald-500/10",
    border: "border-emerald-500/30",
    icon: CheckCircle2,
    label: "System Healthy",
    description: "All agents are online and passing health checks.",
  },
  degraded: {
    bg: "bg-orange-500/10",
    border: "border-orange-500/30",
    icon: AlertTriangle,
    label: "System Degraded",
    description: "Some agents are offline or reporting health issues.",
  },
  critical: {
    bg: "bg-destructive/10",
    border: "border-destructive/30",
    icon: ShieldAlert,
    label: "System Critical",
    description: "Majority of agents are offline or unhealthy. Immediate attention required.",
  },
};

const CHART_COLORS = {
  online: "#10b981",
  offline: "#f97316",
  unhealthy: "#ef4444",
  linux: "#64748b",
  windows: "#0ea5e9",
  darwin: "#a1a1aa",
  unknown: "#6b7280",
  vps: "#6366f1",
  desktop: "#8b5cf6",
  country: ["#22c55e", "#3b82f6", "#eab308", "#ec4899", "#14b8a6", "#f59e0b", "#8b5cf6"],
} as const;

interface StatCardProps {
  title: string;
  value: number | string;
  description: string;
  icon: React.ComponentType<{ className?: string }>;
  accent?: "default" | "success" | "warning" | "danger";
}

const accentStyles = {
  default: "text-primary",
  success: "text-emerald-600 dark:text-emerald-400",
  warning: "text-orange-600 dark:text-orange-400",
  danger: "text-destructive",
};

function StatCard({ title, value, description, icon: Icon, accent = "default" }: StatCardProps) {
  return (
    <Card className="w-full">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className={cn("h-5 w-5", accentStyles[accent])} aria-hidden="true" />
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold tabular-nums">{value}</div>
        <CardDescription className="mt-1">{description}</CardDescription>
      </CardContent>
    </Card>
  );
}

function breakdownToPieData(
  breakdown: Record<string, number>,
  colorMap?: Record<string, string>,
  palette?: readonly string[],
): PieChartDatum[] {
  const entries = Object.entries(breakdown).filter(([, v]) => v > 0);
  return entries.map(([name, value], index) => ({
    name: formatBreakdownLabel(name),
    value,
    color:
      colorMap?.[name.toLowerCase()] ??
      palette?.[index % (palette?.length ?? 1)] ??
      CHART_COLORS.unknown,
  }));
}

function formatBreakdownLabel(key: string): string {
  const labels: Record<string, string> = {
    online: "Online",
    offline: "Offline",
    unhealthy: "Unhealthy",
    linux: "Linux",
    windows: "Windows",
    darwin: "macOS",
    vps: "VPS",
    desktop: "Desktop",
    unknown: "Unknown",
  };
  return labels[key.toLowerCase()] ?? key;
}

function formatLastSeen(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function HealthBanner({ health, totalAgents }: { health: SystemHealth; totalAgents: number }) {
  const config = healthBannerStyles[health];
  const Icon = config.icon;

  return (
    <div
      className={cn(
        "w-full rounded-lg border p-4 md:p-5",
        config.bg,
        config.border,
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <Icon className={cn("h-6 w-6 shrink-0 mt-0.5", accentStyles[health === "healthy" ? "success" : health === "degraded" ? "warning" : "danger"])} />
          <div>
            <h2 className="text-lg font-semibold tracking-tight">{config.label}</h2>
            <p className="text-sm text-muted-foreground mt-0.5">
              {totalAgents === 0
                ? "No agents registered yet. Deploy an agent to begin monitoring."
                : config.description}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground sm:shrink-0">
          <Activity className="h-4 w-4" aria-hidden="true" />
          <span className="tabular-nums">{totalAgents} registered agents</span>
        </div>
      </div>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="w-full space-y-6" aria-busy="true" aria-label="Loading dashboard">
      <Skeleton className="h-24 w-full rounded-lg" />
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <Card key={i} className="w-full">
            <CardHeader className="pb-2">
              <Skeleton className="h-4 w-24" />
            </CardHeader>
            <CardContent className="space-y-2">
              <Skeleton className="h-9 w-16" />
              <Skeleton className="h-3 w-32" />
            </CardContent>
          </Card>
        ))}
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <DashboardPieChartSkeleton title="status" />
        <DashboardPieChartSkeleton title="platform" />
        <DashboardPieChartSkeleton title="device type" />
        <DashboardPieChartSkeleton title="country" />
      </div>
      <Card className="w-full">
        <CardHeader>
          <Skeleton className="h-5 w-40" />
        </CardHeader>
        <CardContent>
          <TableSkeleton rows={5} columns={5} />
        </CardContent>
      </Card>
    </div>
  );
}

export function DashboardPage() {
  const { token } = useAuth();
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadStats = useCallback(async (refresh = false) => {
    if (refresh) {
      setIsRefreshing(true);
    } else {
      setIsLoading(true);
    }
    setError(null);
    try {
      const data = await fetchDashboardStats(token);
      setStats(data);
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : "Unable to load dashboard stats.";
      setError(message);
      setStats(null);
    } finally {
      setIsLoading(false);
      setIsRefreshing(false);
    }
  }, [token]);

  useEffect(() => {
    void loadStats();
  }, [loadStats]);

  const statusChartData = useMemo(
    () =>
      stats
        ? breakdownToPieData(stats.status_breakdown, {
            online: CHART_COLORS.online,
            offline: CHART_COLORS.offline,
            unhealthy: CHART_COLORS.unhealthy,
          })
        : [],
    [stats],
  );

  const platformChartData = useMemo(
    () =>
      stats
        ? breakdownToPieData(stats.platform_breakdown, {
            linux: CHART_COLORS.linux,
            windows: CHART_COLORS.windows,
            darwin: CHART_COLORS.darwin,
            unknown: CHART_COLORS.unknown,
          })
        : [],
    [stats],
  );

  const deviceChartData = useMemo(
    () =>
      stats
        ? breakdownToPieData(stats.device_class_breakdown, {
            vps: CHART_COLORS.vps,
            desktop: CHART_COLORS.desktop,
            unknown: CHART_COLORS.unknown,
          })
        : [],
    [stats],
  );

  const countryChartData = useMemo(
    () => (stats ? breakdownToPieData(stats.country_breakdown, undefined, CHART_COLORS.country) : []),
    [stats],
  );

  if (isLoading) {
    return <DashboardSkeleton />;
  }

  if (error) {
    return (
      <div className="w-full">
        <ErrorState message={error} onRetry={() => void loadStats()} />
      </div>
    );
  }

  if (!stats) {
    return null;
  }

  const health = stats.system_health;

  return (
    <div className="w-full space-y-6">
      <HealthBanner health={health} totalAgents={stats.total_agents} />

      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap">
        <Link to="/agents">
          <Button leftIcon={<LayoutGrid className="h-4 w-4" />}>View Agents</Button>
        </Link>
        <Link to="/deploy">
          <Button variant="secondary" leftIcon={<Rocket className="h-4 w-4" />}>
            Deploy Agent
          </Button>
        </Link>
        <Link to="/settings">
          <Button variant="outline" leftIcon={<Settings className="h-4 w-4" />}>
            Settings
          </Button>
        </Link>
        <Button
          variant="outline"
          leftIcon={<RefreshCw className={cn("h-4 w-4", isRefreshing && "animate-spin")} />}
          loading={isRefreshing}
          onClick={() => void loadStats(true)}
        >
          Refresh
        </Button>
      </div>

      {stats.total_agents === 0 ? (
        <EmptyState
          icon={Server}
          title="No agents registered"
          description="Deploy your first agent from the Deploy Agent page. Once it heartbeats in, monitoring data will appear here."
        />
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
            <StatCard
              title="Total agents"
              value={stats.total_agents}
              description="All registered proxy nodes"
              icon={Server}
            />
            <StatCard
              title="Online"
              value={stats.online}
              description="Recently heartbeating"
              icon={Radio}
              accent="success"
            />
            <StatCard
              title="Offline"
              value={stats.offline}
              description="No recent heartbeat"
              icon={WifiOff}
              accent="warning"
            />
            <StatCard
              title="Unhealthy"
              value={stats.unhealthy}
              description="Failed health probes"
              icon={AlertTriangle}
              accent={stats.unhealthy > 0 ? "danger" : "default"}
            />
            <StatCard
              title="Probe failures"
              value={stats.probe_failures}
              description="Cumulative SOCKS probe failures"
              icon={stats.probe_failures > 0 ? AlertTriangle : Activity}
              accent={stats.probe_failures > 0 ? "danger" : "default"}
            />
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <DashboardPieChart
              title="Agent status"
              data={statusChartData}
              emptyLabel="No agent status data"
            />
            <DashboardPieChart
              title="Platform"
              data={platformChartData}
              emptyLabel="No platform data"
            />
            <DashboardPieChart
              title="Device type"
              data={deviceChartData}
              emptyLabel="No device type data"
            />
            <DashboardPieChart
              title="Country distribution"
              data={countryChartData}
              emptyLabel="No country data"
            />
          </div>

          {countryChartData.length > 0 && (
            <Card className="w-full">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Globe className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                  Fleet geography
                </CardTitle>
                <CardDescription>
                  Agents spread across {countryChartData.length} countries
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ul className="space-y-2">
                  {countryChartData
                    .sort((a, b) => b.value - a.value)
                    .slice(0, 6)
                    .map((item) => (
                      <li
                        key={item.name}
                        className="flex items-center justify-between text-sm"
                      >
                        <span className="flex items-center gap-2">
                          <span
                            className="h-2.5 w-2.5 rounded-full shrink-0"
                            style={{ backgroundColor: item.color }}
                            aria-hidden="true"
                          />
                          {item.name}
                        </span>
                        <span className="tabular-nums text-muted-foreground">
                          {item.value}
                        </span>
                      </li>
                    ))}
                </ul>
              </CardContent>
            </Card>
          )}

          <Card className="w-full">
            <CardHeader className="flex flex-row items-center justify-between space-y-0">
              <div>
                <CardTitle className="text-base">Recent agents</CardTitle>
                <CardDescription>Last seen agents — click a row to view all agents</CardDescription>
              </div>
              <Link to="/agents">
                <Button variant="ghost" size="sm">View all</Button>
              </Link>
            </CardHeader>
            <CardContent className="p-0">
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-left text-muted-foreground">
                      <th className="px-6 py-3 font-medium">Agent</th>
                      <th className="px-4 py-3 font-medium">Location</th>
                      <th className="px-4 py-3 font-medium">Platform</th>
                      <th className="px-4 py-3 font-medium">Type</th>
                      <th className="px-4 py-3 font-medium">Status</th>
                      <th className="px-6 py-3 font-medium">Last seen</th>
                    </tr>
                  </thead>
                  <tbody>
                    {stats.recent_nodes.map((node) => (
                      <tr
                        key={node.id}
                        className="border-b border-border last:border-0 hover:bg-muted/50 transition-colors"
                      >
                        <td className="px-6 py-3">
                          <Link
                            to="/agents"
                            className="font-medium text-foreground hover:text-primary transition-colors"
                          >
                            {node.ip}
                          </Link>
                          <div className="text-xs text-muted-foreground">{node.username}</div>
                        </td>
                        <td className="px-4 py-3 text-muted-foreground">
                          {node.city ? `${node.city}, ` : ""}
                          {node.country || "—"}
                        </td>
                        <td className="px-4 py-3">
                          <PlatformBadge platform={node.platform} />
                        </td>
                        <td className="px-4 py-3">
                          <TypeBadge node={node} />
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1.5">
                            <OnlineBadge online={node.is_online} />
                            <HealthBadge healthy={node.is_healthy} />
                          </div>
                        </td>
                        <td className="px-6 py-3 text-muted-foreground tabular-nums">
                          {formatLastSeen(node.last_seen)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>

          <div className="grid gap-4 sm:grid-cols-2">
            <Card className="w-full">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Monitor className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                  Metrics snapshot
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Cached nodes online</span>
                  <span className="tabular-nums font-medium">{stats.nodes_online}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Online rate</span>
                  <span className="tabular-nums font-medium">
                    {stats.total_agents > 0
                      ? `${Math.round((stats.online / stats.total_agents) * 100)}%`
                      : "—"}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Healthy rate</span>
                  <span className="tabular-nums font-medium">
                    {stats.total_agents > 0
                      ? `${Math.round(((stats.total_agents - stats.unhealthy) / stats.total_agents) * 100)}%`
                      : "—"}
                  </span>
                </div>
              </CardContent>
            </Card>
            <Card className="w-full">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Activity className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                  System notes
                </CardTitle>
              </CardHeader>
              <CardContent className="text-sm text-muted-foreground space-y-2">
                {stats.probe_failures > 0 && (
                  <p>
                    {stats.probe_failures} cumulative SOCKS probe failure
                    {stats.probe_failures === 1 ? "" : "s"} recorded.
                  </p>
                )}
                {stats.offline > 0 && (
                  <p>
                    {stats.offline} agent{stats.offline === 1 ? "" : "s"} missed recent heartbeats.
                  </p>
                )}
                {stats.unhealthy > 0 && (
                  <p>
                    {stats.unhealthy} agent{stats.unhealthy === 1 ? "" : "s"} reported unhealthy status.
                  </p>
                )}
                {stats.probe_failures === 0 && stats.offline === 0 && stats.unhealthy === 0 && (
                  <p>All agents are online and healthy. No issues detected.</p>
                )}
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
