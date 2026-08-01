import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// Dev workflow (two terminals):
//   Terminal 1: make run-dashboard          → Go API on :8081
//   Terminal 2: npm run dev                 → Vite UI on :8080 → http://localhost:8080
// Use 127.0.0.1 (IPv4) to match dashboard tcp4 bind; "localhost" can hit a stale IPv6 listener.
const apiTarget = process.env.VITE_API_PROXY_TARGET ?? "http://127.0.0.1:8081";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: "127.0.0.1", // IPv4 — matches browser localhost; avoids split with Go on [::1]:8080
    port: Number(process.env.VITE_DEV_PORT ?? 8080),
    strictPort: true,
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
});
