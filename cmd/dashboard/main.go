package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/dashboard"
	dashauth "github.com/Skillz147/TrinityProxy/internal/dashboard/auth"
	"github.com/Skillz147/TrinityProxy/internal/dashboard/deployment"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func main() {
	initOnly := false
	for _, arg := range os.Args[1:] {
		if arg == "--init-only" {
			initOnly = true
		}
	}

	log := logutil.New("dashboard")
	cfg := dashboard.LoadConfig()

	authStore, err := dashauth.NewStore(cfg.DBPath, cfg.SessionTTL)
	if err != nil {
		logutil.Fatal(log, "failed to open dashboard auth store", "err", err, "db", cfg.DBPath)
	}
	defer authStore.Close()

	bootstrap, err := authStore.BootstrapAdmin(cfg.AdminUsername)
	if err != nil {
		logutil.Fatal(log, "failed to bootstrap dashboard admin", "err", err)
	}
	if bootstrap.Created {
		printBootstrapCredentials(cfg.DashboardURL, bootstrap.Username, bootstrap.TempPassword)
	} else if initOnly {
		fmt.Println("Dashboard admin already exists; no credentials generated.")
	}

	if initOnly {
		return
	}

	deployStore, err := deployment.NewStore(cfg.DBPath)
	if err != nil {
		logutil.Fatal(log, "failed to open deployment store", "err", err, "db", cfg.DBPath)
	}
	defer deployStore.Close()

	nodeStorage, err := storage.NewNodeStorage(cfg.NodesDBPath)
	if err != nil {
		logutil.Fatal(log, "failed to open node storage", "err", err, "db", cfg.NodesDBPath)
	}
	defer nodeStorage.Close()

	server := dashboard.NewServer(cfg, authStore, deployStore, nodeStorage, log)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	if !cfg.DevProxy {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "TrinityProxy Dashboard API (listening on %s)\n\n", cfg.ListenAddr())
			fmt.Fprintf(w, "This process serves /api/* and /health only.\n")
			fmt.Fprintf(w, "Open the UI at %s (Vite dev server on :8080)\n", cfg.DashboardURL)
			fmt.Fprintf(w, "Terminal 2: cd web/dashboard && npm run dev\n")
			fmt.Fprintf(w, "Setup help: make dashboard-dev | make run-dashboard (hyphen, not space)\n")
		})
	}

	if cfg.DevProxy {
		viteTarget, err := url.Parse(cfg.ViteDevURL)
		if err != nil {
			logutil.Fatal(log, "invalid DASHBOARD_VITE_URL", "err", err, "url", cfg.ViteDevURL)
		}
		proxy := httputil.NewSingleHostReverseProxy(viteTarget)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			proxy.ServeHTTP(w, r)
		})
		log.Info("frontend dev proxy enabled", "target", cfg.ViteDevURL)
	}

	addr := cfg.ListenAddr()
	log.Info("dashboard API listening (UI is a separate Vite dev server)",
		"addr", addr,
		"open_ui", cfg.DashboardURL,
		"ui_command", "cd web/dashboard && npm run dev",
		"make_hint", "make dashboard-dev",
		"auth_db", cfg.DBPath,
		"nodes_db", cfg.NodesDBPath,
	)
	log.Info("endpoints registered",
		"login", "POST /api/auth/login",
		"logout", "POST /api/auth/logout",
		"me", "GET /api/auth/me",
		"change_password", "POST /api/auth/change-password",
		"stats", "GET /api/dashboard/stats",
		"nodes", "GET /api/dashboard/nodes",
		"node_credentials", "GET /api/dashboard/nodes/{id}/credentials",
		"bootstrap_script", "GET /api/dashboard/bootstrap-script",
		"deploy_commands", "GET /api/dashboard/deploy-commands",
		"deployment", "GET/PUT /api/dashboard/deployment",
		"regenerate_agent_key", "POST /api/dashboard/deployment/regenerate-agent-key",
		"dns_hints", "GET /api/dashboard/deployment/dns-hints",
		"dev_setup", "GET /api/dashboard/deployment/dev-setup",
		"cloudflare_setup", "GET /api/dashboard/deployment/cloudflare-setup",
		"provision_ssl", "POST /api/dashboard/deployment/provision-ssl",
		"health", "GET /health",
	)

	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		logutil.Fatal(log, "dashboard port already in use (stop other dashboard/API processes on this port)", "err", err, "addr", addr)
	}
	if err := http.Serve(ln, mux); err != nil {
		logutil.Fatal(log, "dashboard server failed", "err", err)
	}
}

func printBootstrapCredentials(dashboardURL, username, tempPassword string) {
	sep := strings.Repeat("=", 60)
	fmt.Println()
	fmt.Println(sep)
	fmt.Println("TrinityProxy Master Dashboard — initial credentials")
	fmt.Println(sep)
	fmt.Printf("Dashboard URL:  %s\n", dashboardURL)
	fmt.Printf("Username:       %s\n", username)
	fmt.Printf("Temp password:  %s\n", tempPassword)
	fmt.Println()
	fmt.Println("First login requires a password change before accessing the dashboard.")
	fmt.Println(sep)
	fmt.Println()

	slog.Default().Info("dashboard admin bootstrapped",
		"dashboard_url", dashboardURL,
		"username", username,
	)
}
