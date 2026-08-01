import { Navigate, Route } from "react-router-dom";
import { AppShell } from "@/components/layout";
import {
  AgentsPage,
  DashboardPage,
  DeployAgentPage,
  SettingsPage,
} from "@/routes/pages";

/** Protected app shell routes — compose inside App with auth guards. */
export const appShellRoutes = (
  <>
    <Route element={<AppShell />}>
      <Route index element={<DashboardPage />} />
      <Route path="dashboard" element={<Navigate to="/" replace />} />
      <Route path="agents" element={<AgentsPage />} />
      <Route path="deploy" element={<DeployAgentPage />} />
      <Route path="settings" element={<SettingsPage />} />
    </Route>
  </>
);
