import { Menu, Moon, Sun } from "lucide-react";
import { useLocation } from "react-router-dom";
import { getPageHeader } from "@/components/layout/Sidebar";
import { Button } from "@/components/ui/Button";
import { useHeaderSlot } from "@/context/HeaderSlotContext";
import { useTheme } from "@/context/ThemeProvider";

export interface HeaderProps {
  onOpenMobileSidebar: () => void;
}

function ThemeToggleButton() {
  const { resolvedTheme, setTheme } = useTheme();

  const toggleTheme = () => {
    setTheme(resolvedTheme === "dark" ? "light" : "dark");
  };

  return (
    <Button variant="outline" size="icon" aria-label="Toggle theme" onClick={toggleTheme}>
      {resolvedTheme === "dark" ? (
        <Sun className="h-4 w-4" />
      ) : (
        <Moon className="h-4 w-4" />
      )}
    </Button>
  );
}

function MobileMenuButton({ onOpenMobileSidebar }: { onOpenMobileSidebar: () => void }) {
  return (
    <button
      type="button"
      aria-label="Open navigation menu"
      className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground md:hidden"
      onClick={onOpenMobileSidebar}
    >
      <Menu className="h-5 w-5" />
    </button>
  );
}

function isAgentsRoute(pathname: string) {
  return pathname === "/agents" || pathname.startsWith("/agents/");
}

export function Header({ onOpenMobileSidebar }: HeaderProps) {
  const { pathname } = useLocation();
  const { toolbar } = useHeaderSlot();
  const { title, subtitle } = getPageHeader(pathname);
  const agentsInlineHeader = Boolean(toolbar) && isAgentsRoute(pathname);

  if (toolbar) {
    return (
      <header className="shrink-0 border-b border-border bg-background">
        <div
          className={
            agentsInlineHeader
              ? "flex min-h-14 w-full items-center gap-3 px-4 py-1.5"
              : "flex min-h-14 items-center gap-2 px-4 py-2"
          }
        >
          <MobileMenuButton onOpenMobileSidebar={onOpenMobileSidebar} />
          <div className="min-w-0 flex-1">{toolbar}</div>
          <div className="ml-auto flex shrink-0 items-center">
            <ThemeToggleButton />
          </div>
        </div>

        {!agentsInlineHeader && (
          <div className="px-4 pb-3 pt-0">
            <h1 className="truncate text-sm font-semibold md:text-base">{title}</h1>
            <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
          </div>
        )}
      </header>
    );
  }

  return (
    <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border bg-background px-4">
      <MobileMenuButton onOpenMobileSidebar={onOpenMobileSidebar} />

      <div className="flex min-w-0 flex-1 flex-col justify-center">
        <h1 className="truncate text-sm font-semibold md:text-base">{title}</h1>
        <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>

      <div className="flex shrink-0 items-center gap-1">
        <ThemeToggleButton />
      </div>
    </header>
  );
}
