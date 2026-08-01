import { NavLink, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  Server,
  Rocket,
  Settings,
  PanelLeftClose,
  PanelLeftOpen,
  X,
  LogOut,
  BookOpen,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { useAuth } from "@/context/AuthContext";

export const SIDEBAR_STORAGE_KEY = "trinity-sidebar-collapsed";

export const navItems = [
  {
    to: "/",
    label: "Dashboard",
    icon: LayoutDashboard,
    end: true,
    title: "Dashboard",
    subtitle: "Fleet overview and health at a glance.",
  },
  {
    to: "/agents",
    label: "Agents",
    icon: Server,
    title: "Agents",
    subtitle: "Registered proxy nodes and live status.",
  },
  {
    to: "/deploy",
    label: "Deploy Agent",
    icon: Rocket,
    title: "Deploy Agent",
    subtitle: "Bootstrap a new proxy node on a fresh VPS.",
  },
  {
    to: "/settings",
    label: "Settings",
    icon: Settings,
    title: "Settings",
    subtitle: "Account and dashboard preferences.",
  },
] as const;

export function getPageHeader(pathname: string) {
  const item =
    navItems.find(({ to, ...rest }) =>
      "end" in rest && rest.end ? pathname === to : pathname === to || pathname.startsWith(`${to}/`),
    ) ?? navItems[0];

  return { title: item.title, subtitle: item.subtitle };
}

export interface SidebarProps {
  collapsed: boolean;
  onToggleCollapse: () => void;
  mobileOpen: boolean;
  onMobileClose: () => void;
}

export function Sidebar({
  collapsed,
  onToggleCollapse,
  mobileOpen,
  onMobileClose,
}: SidebarProps) {
  const navigate = useNavigate();
  const { logout } = useAuth();

  const handleSignOut = async () => {
    await logout();
    navigate("/login", { replace: true });
  };

  return (
    <>
      {mobileOpen && (
        <button
          type="button"
          aria-label="Close navigation menu"
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={onMobileClose}
        />
      )}

      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 flex h-full shrink-0 flex-col overflow-y-auto border-r border-border bg-card transition-all duration-200 md:static md:z-auto md:overflow-hidden",
          collapsed ? "w-16" : "w-64",
          mobileOpen ? "translate-x-0" : "-translate-x-full md:translate-x-0",
        )}
      >
        <div
          className={cn(
            "flex h-14 shrink-0 items-center border-b border-border px-3",
            collapsed ? "justify-center" : "justify-between",
          )}
        >
          {!collapsed && (
            <span className="truncate text-sm font-semibold tracking-tight">
              TrinityProxy
            </span>
          )}

          <button
            type="button"
            aria-label={mobileOpen ? "Close sidebar" : "Toggle sidebar"}
            className="inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground md:hidden"
            onClick={onMobileClose}
          >
            <X className="h-5 w-5" />
          </button>

          <button
            type="button"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="hidden h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground md:inline-flex"
            onClick={onToggleCollapse}
          >
            {collapsed ? (
              <PanelLeftOpen className="h-5 w-5" />
            ) : (
              <PanelLeftClose className="h-5 w-5" />
            )}
          </button>
        </div>

        <nav className="min-h-0 flex-1 space-y-1 overflow-hidden p-2">
          {navItems.map(({ to, label, icon: Icon, ...rest }) => (
            <NavLink
              key={to}
              to={to}
              end={"end" in rest ? rest.end : false}
              onClick={onMobileClose}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  collapsed && "justify-center px-2",
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                )
              }
              title={collapsed ? label : undefined}
            >
              <Icon className="h-5 w-5 shrink-0" />
              {!collapsed && <span className="truncate">{label}</span>}
            </NavLink>
          ))}
        </nav>

        <div className="shrink-0 border-t border-border p-2">
          {!collapsed && (
            <p className="px-3 pb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Quick start
            </p>
          )}
          <NavLink
            to="/deploy"
            onClick={onMobileClose}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                collapsed && "justify-center px-2",
                isActive
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
              )
            }
            title={collapsed ? "Deploy agent" : undefined}
          >
            <BookOpen className="h-5 w-5 shrink-0" />
            {!collapsed && <span className="truncate">Deploy agent</span>}
          </NavLink>
          <Button
            variant="ghost"
            className={cn(
              "mt-1 w-full justify-start gap-3 text-muted-foreground",
              collapsed && "justify-center px-2",
            )}
            onClick={() => void handleSignOut()}
            title={collapsed ? "Sign out" : undefined}
          >
            <LogOut className="h-5 w-5 shrink-0" />
            {!collapsed && <span>Sign out</span>}
          </Button>
        </div>
      </aside>
    </>
  );
}
