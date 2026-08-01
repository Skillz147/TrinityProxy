import { Navigate, Route, Routes } from "react-router-dom";
import { ProtectedRoute, PublicOnlyRoute } from "@/components/ProtectedRoute";
import { ChangePasswordPage } from "@/pages/ChangePassword";
import { LoginPage } from "@/pages/Login";
import { appShellRoutes } from "@/routes";

export function App() {
  return (
    <Routes>
      <Route element={<PublicOnlyRoute />}>
        <Route path="/login" element={<LoginPage />} />
      </Route>

      <Route element={<ProtectedRoute requirePasswordChanged={false} />}>
        <Route path="/change-password" element={<ChangePasswordPage />} />
      </Route>

      <Route element={<ProtectedRoute requirePasswordChanged />}>
        {appShellRoutes}
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
