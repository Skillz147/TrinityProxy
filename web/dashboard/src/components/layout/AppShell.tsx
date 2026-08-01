import { useState } from "react";
import { Outlet } from "react-router-dom";
import { Header } from "@/components/layout/Header";
import { SIDEBAR_STORAGE_KEY, Sidebar } from "@/components/layout/Sidebar";
import { HeaderSlotProvider } from "@/context/HeaderSlotContext";
import { useLocalStorage } from "@/hooks/useLocalStorage";
import { cn } from "@/lib/utils";

export function AppShell() {
  const [sidebarCollapsed, setSidebarCollapsed] = useLocalStorage(
    SIDEBAR_STORAGE_KEY,
    false,
  );
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  const toggleSidebarCollapse = () => {
    setSidebarCollapsed((prev) => !prev);
  };

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <Sidebar
        collapsed={sidebarCollapsed}
        onToggleCollapse={toggleSidebarCollapse}
        mobileOpen={mobileSidebarOpen}
        onMobileClose={() => setMobileSidebarOpen(false)}
      />

      <HeaderSlotProvider>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <Header onOpenMobileSidebar={() => setMobileSidebarOpen(true)} />

          <main
            className={cn(
              "min-h-0 flex-1 overflow-y-auto p-4 md:p-6",
              sidebarCollapsed ? "md:pl-4" : "md:pl-6",
            )}
          >
            <Outlet />
          </main>
        </div>
      </HeaderSlotProvider>
    </div>
  );
}
