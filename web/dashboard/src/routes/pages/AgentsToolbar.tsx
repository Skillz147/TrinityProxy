import { RefreshCw, Search } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { cn } from "@/lib/utils";

export type AgentFilter = "all" | "linux" | "windows" | "mac" | "vps" | "desktop";
export type StatusFilter = "all" | "online" | "offline";

const AGENT_FILTERS: { value: AgentFilter; label: string }[] = [
  { value: "all", label: "All types" },
  { value: "linux", label: "Linux" },
  { value: "windows", label: "Windows" },
  { value: "mac", label: "Mac" },
  { value: "vps", label: "VPS" },
  { value: "desktop", label: "Desktop" },
];

const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All status" },
  { value: "online", label: "Online" },
  { value: "offline", label: "Offline" },
];

const filterSelectClass =
  "h-9 rounded-md border border-input bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export interface AgentsToolbarProps {
  title?: string;
  subtitle?: string;
  searchQuery: string;
  onSearchChange: (value: string) => void;
  countryFilter: string;
  onCountryChange: (value: string) => void;
  countries: string[];
  platformFilter: AgentFilter;
  onPlatformChange: (value: AgentFilter) => void;
  statusFilter: StatusFilter;
  onStatusChange: (value: StatusFilter) => void;
  onRefresh: () => void;
  isLoading: boolean;
  isRefreshing: boolean;
  filteredCount: number;
  totalCount: number;
  showCount: boolean;
}

export function AgentsToolbar({
  title = "Agents",
  subtitle,
  searchQuery,
  onSearchChange,
  countryFilter,
  onCountryChange,
  countries,
  platformFilter,
  onPlatformChange,
  statusFilter,
  onStatusChange,
  onRefresh,
  isLoading,
  isRefreshing,
  filteredCount,
  totalCount,
  showCount,
}: AgentsToolbarProps) {
  return (
    <div className="flex w-full min-w-0 flex-1 flex-wrap items-center gap-3 sm:flex-nowrap">
      <div className="w-[180px] shrink-0 sm:w-[220px]">
        <h1 className="text-sm font-semibold leading-tight">{title}</h1>
        {subtitle && (
          <p className="text-xs leading-tight text-muted-foreground">{subtitle}</p>
        )}
      </div>

      <div className="flex min-w-0 flex-1 basis-full items-center gap-2 sm:basis-auto">
        <div className="relative min-w-0 flex-1 max-w-md">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            placeholder="Search IP, city, user, country…"
            value={searchQuery}
            onChange={(event) => onSearchChange(event.target.value)}
            className="h-9 pl-8"
            aria-label="Search agents"
          />
        </div>

        <div className="flex flex-1 items-center justify-end gap-2">
          <select
            aria-label="Filter by country"
            className={cn(filterSelectClass, "min-w-[120px]")}
            value={countryFilter}
            onChange={(event) => onCountryChange(event.target.value)}
          >
            <option value="all">All countries</option>
            {countries.map((country) => (
              <option key={country} value={country}>
                {country}
              </option>
            ))}
          </select>

          <select
            aria-label="Filter by platform or type"
            className={cn(filterSelectClass, "min-w-[120px]")}
            value={platformFilter}
            onChange={(event) => onPlatformChange(event.target.value as AgentFilter)}
          >
            {AGENT_FILTERS.map(({ value, label }) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>

          <select
            aria-label="Filter by status"
            className={cn(filterSelectClass, "min-w-[110px]")}
            value={statusFilter}
            onChange={(event) => onStatusChange(event.target.value as StatusFilter)}
          >
            {STATUS_FILTERS.map(({ value, label }) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="flex shrink-0 items-center gap-2 max-sm:ml-auto">
        {showCount && (
          <span className="text-xs text-muted-foreground tabular-nums whitespace-nowrap">
            {filteredCount} of {totalCount}
          </span>
        )}
        <Button
          variant="outline"
          size="sm"
          className="h-9 shrink-0"
          onClick={onRefresh}
          disabled={isLoading || isRefreshing}
        >
          <RefreshCw className={cn("h-4 w-4", isRefreshing && "animate-spin")} />
          Refresh
        </Button>
      </div>
    </div>
  );
}
