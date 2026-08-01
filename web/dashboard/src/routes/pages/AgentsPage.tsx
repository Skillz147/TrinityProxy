import { useCallback, useEffect, useMemo, useState, type MouseEvent } from "react";
import { Eye, Link2, Server, Trash2 } from "lucide-react";
import { EmptyState } from "@/components/EmptyState";
import { ErrorState } from "@/components/ErrorState";
import { SocksCredentialsDialog } from "@/components/SocksCredentialsDialog";
import {
  HealthBadge,
  normalizePlatform,
  OnlineBadge,
  PlatformBadge,
  TypeBadge,
} from "@/components/StatusBadge";
import { TableSkeleton } from "@/components/ui/Skeleton";
import { useAuth } from "@/context/AuthContext";
import { useHeaderSlot } from "@/context/HeaderSlotContext";
import {
  ApiError,
  fetchDashboardNodes,
  fetchNodeCredentials,
  deleteDashboardNode,
  type ProxyNode,
} from "@/lib/api";
import { toast } from "@/lib/toast";
import { getPageHeader } from "@/components/layout/Sidebar";
import {
  AgentsToolbar,
  type AgentFilter,
  type StatusFilter,
} from "@/routes/pages/AgentsToolbar";

interface ContextMenuState {
  node: ProxyNode;
  x: number;
  y: number;
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

function matchesPlatformFilter(node: ProxyNode, filter: AgentFilter): boolean {
  if (filter === "all") return true;

  const platform = normalizePlatform(node.platform);
  const deviceClass = node.device_class?.toLowerCase();

  switch (filter) {
    case "linux":
      return platform === "linux";
    case "windows":
      return platform === "windows";
    case "mac":
      return platform === "darwin";
    case "vps":
      return deviceClass === "vps";
    case "desktop":
      return deviceClass === "desktop";
    default:
      return true;
  }
}

function matchesSearch(node: ProxyNode, query: string): boolean {
  if (!query.trim()) return true;
  const haystack = [node.ip, node.city, node.username, node.country, node.region]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(query.trim().toLowerCase());
}

function matchesStatus(node: ProxyNode, status: StatusFilter): boolean {
  if (status === "all") return true;
  if (status === "online") return node.is_online;
  return !node.is_online;
}

async function copyText(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
  } catch {
    toast.error("Unable to copy — check browser permissions");
  }
}

function AgentContextMenu({
  menu,
  token,
  onClose,
  onViewCredentials,
  onRemove,
}: {
  menu: ContextMenuState;
  token: string | null;
  onClose: () => void;
  onViewCredentials: (node: ProxyNode) => void;
  onRemove: (node: ProxyNode) => void;
}) {
  const [isLoading, setIsLoading] = useState(false);

  const handleViewCredentials = () => {
    onViewCredentials(menu.node);
    onClose();
  };

  const handleCopyConnectionString = async () => {
    setIsLoading(true);
    try {
      const creds = await fetchNodeCredentials(token, menu.node.id);
      await copyText(creds.connection_string, "Connection string copied");
      onClose();
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Unable to load credentials.";
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div
      className="fixed z-50 min-w-[220px] overflow-hidden rounded-md border border-border bg-card p-1 text-card-foreground shadow-md"
      style={{ left: menu.x, top: menu.y }}
      role="menu"
      onMouseDown={(event) => event.stopPropagation()}
    >
      <button
        type="button"
        role="menuitem"
        className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground"
        onClick={handleViewCredentials}
      >
        <Eye className="h-4 w-4 shrink-0" />
        View SOCKS credentials
      </button>
      <button
        type="button"
        role="menuitem"
        disabled={isLoading}
        className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:opacity-50"
        onClick={() => void handleCopyConnectionString()}
      >
        <Link2 className="h-4 w-4 shrink-0" />
        Copy connection string
      </button>
      <button
        type="button"
        role="menuitem"
        className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive hover:bg-destructive/10"
        onClick={() => {
          onRemove(menu.node);
          onClose();
        }}
      >
        <Trash2 className="h-4 w-4 shrink-0" />
        Remove agent
      </button>
    </div>
  );
}

function AgentTableRow({
  node,
  onContextMenu,
}: {
  node: ProxyNode;
  onContextMenu: (event: MouseEvent, node: ProxyNode) => void;
}) {
  return (
    <tr
      className="border-b border-border last:border-0 hover:bg-muted/30"
      onContextMenu={(event) => onContextMenu(event, node)}
    >
      <td className="whitespace-nowrap px-4 py-3 font-mono text-sm">{node.ip}</td>
      <td className="whitespace-nowrap px-4 py-3 text-sm tabular-nums">{node.port}</td>
      <td className="whitespace-nowrap px-4 py-3 text-sm">{node.country || "—"}</td>
      <td className="whitespace-nowrap px-4 py-3 text-sm">{node.city || "—"}</td>
      <td className="whitespace-nowrap px-4 py-3 text-sm">{node.zip || "—"}</td>
      <td className="whitespace-nowrap px-4 py-3 font-mono text-sm">{node.username}</td>
      <td className="whitespace-nowrap px-4 py-3">
        <PlatformBadge platform={node.platform} />
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <TypeBadge node={node} />
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <OnlineBadge online={node.is_online} />
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <HealthBadge healthy={node.is_healthy} />
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-muted-foreground">
        {formatLastSeen(node.last_seen)}
      </td>
    </tr>
  );
}

function AgentCard({
  node,
  onContextMenu,
}: {
  node: ProxyNode;
  onContextMenu: (event: MouseEvent, node: ProxyNode) => void;
}) {
  return (
    <article
      className="rounded-lg border border-border bg-card p-4 shadow-sm"
      onContextMenu={(event) => onContextMenu(event, node)}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="font-mono text-sm font-medium">
            {node.ip}:{node.port}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {[node.city, node.country].filter(Boolean).join(", ") || "Unknown location"}
            {node.zip ? ` · ${node.zip}` : ""}
          </p>
        </div>
        <div className="flex flex-col items-end gap-1">
          <OnlineBadge online={node.is_online} />
          <HealthBadge healthy={node.is_healthy} />
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <PlatformBadge platform={node.platform} />
        <TypeBadge node={node} />
      </div>
      <dl className="mt-4 grid grid-cols-2 gap-2 text-xs">
        <div>
          <dt className="text-muted-foreground">Username</dt>
          <dd className="font-mono">{node.username}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Last seen</dt>
          <dd>{formatLastSeen(node.last_seen)}</dd>
        </div>
      </dl>
    </article>
  );
}

export function AgentsPage() {
  const { token } = useAuth();
  const { setToolbar } = useHeaderSlot();
  const [nodes, setNodes] = useState<ProxyNode[]>([]);
  const [searchQuery, setSearchQuery] = useState("");
  const [countryFilter, setCountryFilter] = useState("all");
  const [platformFilter, setPlatformFilter] = useState<AgentFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [credentialsNode, setCredentialsNode] = useState<ProxyNode | null>(null);

  const countries = useMemo(() => {
    const values = new Set<string>();
    for (const node of nodes) {
      if (node.country?.trim()) {
        values.add(node.country.trim());
      }
    }
    return Array.from(values).sort((a, b) => a.localeCompare(b));
  }, [nodes]);

  const filteredNodes = useMemo(
    () =>
      nodes.filter(
        (node) =>
          matchesSearch(node, searchQuery) &&
          (countryFilter === "all" || node.country === countryFilter) &&
          matchesPlatformFilter(node, platformFilter) &&
          matchesStatus(node, statusFilter),
      ),
    [nodes, searchQuery, countryFilter, platformFilter, statusFilter],
  );

  const loadNodes = useCallback(
    async (options?: { refresh?: boolean }) => {
      const refreshing = options?.refresh ?? false;
      if (refreshing) {
        setIsRefreshing(true);
      } else {
        setIsLoading(true);
      }
      setError(null);

      try {
        const data = await fetchDashboardNodes(token);
        setNodes(data.nodes);
        if (refreshing) {
          toast.success(`Refreshed — ${data.count} agent${data.count === 1 ? "" : "s"}`);
        }
      } catch (err) {
        const message =
          err instanceof ApiError ? err.message : "Unable to load agents.";
        setError(message);
        if (refreshing) {
          toast.error(message);
        }
      } finally {
        setIsLoading(false);
        setIsRefreshing(false);
      }
    },
    [token],
  );

  useEffect(() => {
    void loadNodes();
  }, [loadNodes]);

  const { title: pageTitle, subtitle: pageSubtitle } = getPageHeader("/agents");

  const headerToolbar = useMemo(
    () => (
      <AgentsToolbar
        title={pageTitle}
        subtitle={pageSubtitle}
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        countryFilter={countryFilter}
        onCountryChange={setCountryFilter}
        countries={countries}
        platformFilter={platformFilter}
        onPlatformChange={setPlatformFilter}
        statusFilter={statusFilter}
        onStatusChange={setStatusFilter}
        onRefresh={() => void loadNodes({ refresh: true })}
        isLoading={isLoading}
        isRefreshing={isRefreshing}
        filteredCount={filteredNodes.length}
        totalCount={nodes.length}
        showCount={!isLoading && !error && nodes.length > 0}
      />
    ),
    [
      pageTitle,
      pageSubtitle,
      searchQuery,
      countryFilter,
      countries,
      platformFilter,
      statusFilter,
      isLoading,
      isRefreshing,
      filteredNodes.length,
      nodes.length,
      error,
      loadNodes,
    ],
  );

  useEffect(() => {
    setToolbar(headerToolbar);
    return () => setToolbar(null);
  }, [headerToolbar, setToolbar]);

  useEffect(() => {
    if (!contextMenu) return;

    const closeMenu = () => setContextMenu(null);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeMenu();
    };

    window.addEventListener("click", closeMenu);
    window.addEventListener("scroll", closeMenu, true);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("click", closeMenu);
      window.removeEventListener("scroll", closeMenu, true);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [contextMenu]);

  const handleRowContextMenu = (event: MouseEvent, node: ProxyNode) => {
    event.preventDefault();
    setContextMenu({ node, x: event.clientX, y: event.clientY });
  };

  const handleRemoveAgent = async (node: ProxyNode) => {
    const label = `${node.ip}:${node.port}`;
    if (!window.confirm(`Remove agent ${label} from the dashboard?`)) {
      return;
    }

    try {
      await deleteDashboardNode(token, node.id);
      setNodes((current) => current.filter((item) => item.id !== node.id));
      toast.success(`Removed agent ${label}`);
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Unable to remove agent.";
      toast.error(message);
    }
  };

  return (
    <div className="space-y-6">
        {isLoading && <TableSkeleton rows={6} columns={7} />}

        {!isLoading && error && (
          <ErrorState message={error} onRetry={() => void loadNodes()} />
        )}

        {!isLoading && !error && nodes.length === 0 && (
          <EmptyState
            icon={Server}
            title="No agents yet"
            description="Deploy an agent on a new VPS using the bootstrap script on the Deploy Agent page."
          />
        )}

        {!isLoading && !error && nodes.length > 0 && filteredNodes.length === 0 && (
          <EmptyState
            icon={Server}
            title="No matching agents"
            description="Try different filters or refresh the list."
          />
        )}

        {!isLoading && !error && filteredNodes.length > 0 && (
          <>
            <div className="hidden overflow-x-auto rounded-lg border border-border md:block">
              <table className="w-full min-w-[1120px] text-left text-sm">
                <thead className="border-b border-border">
                  <tr>
                    {[
                      "IP",
                      "Port",
                      "Country",
                      "City",
                      "Zip",
                      "Username",
                      "Platform",
                      "Type",
                      "Status",
                      "Health",
                      "Last seen",
                    ].map((heading) => (
                      <th
                        key={heading}
                        scope="col"
                        className="sticky top-0 z-10 whitespace-nowrap bg-background px-4 py-3 text-xs font-medium uppercase tracking-wide text-muted-foreground"
                      >
                        {heading}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {filteredNodes.map((node) => (
                    <AgentTableRow
                      key={node.id}
                      node={node}
                      onContextMenu={handleRowContextMenu}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            <div className="grid gap-4 md:hidden">
              {filteredNodes.map((node) => (
                <AgentCard
                  key={node.id}
                  node={node}
                  onContextMenu={handleRowContextMenu}
                />
              ))}
            </div>
          </>
        )}

      {contextMenu && (
        <AgentContextMenu
          menu={contextMenu}
          token={token}
          onClose={() => setContextMenu(null)}
          onViewCredentials={setCredentialsNode}
          onRemove={(node) => void handleRemoveAgent(node)}
        />
      )}

      <SocksCredentialsDialog
        node={credentialsNode}
        token={token}
        onClose={() => setCredentialsNode(null)}
      />
    </div>
  );
}
