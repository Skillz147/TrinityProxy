import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { ConfirmDialogProvider } from "@/components/ui/ConfirmDialog";
import { App } from "@/App";
import { AuthProvider } from "@/context/AuthContext";
import { ThemeProvider } from "@/context/ThemeProvider";
import { Toaster } from "@/lib/toast";
import "@/index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <BrowserRouter>
        <AuthProvider>
          <ConfirmDialogProvider>
            <App />
            <Toaster />
          </ConfirmDialogProvider>
        </AuthProvider>
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>,
);
