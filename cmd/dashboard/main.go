package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/dashboard"
	dashauth "github.com/Skillz147/TrinityProxy/internal/dashboard/auth"
	"github.com/Skillz147/TrinityProxy/internal/dashboard/deployment"
	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/health"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func main() {
	initOnly := false
	resetAdmin := false
	syncDeployment := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--init-only":
			initOnly = true
		case "--reset-admin":
			resetAdmin = true
		case "--sync-deployment":
			syncDeployment = true
		}
	}
	modes := 0
	if initOnly {
		modes++
	}
	if resetAdmin {
		modes++
	}
	if syncDeployment {
		modes++
	}
	if modes > 1 {
		fmt.Fprintln(os.Stderr, "error: use only one of --init-only, --reset-admin, or --sync-deployment")
		os.Exit(2)
	}

	log := logutil.New("dashboard")
	cfg := dashboard.LoadConfig()

	if resetAdmin {
		runResetAdmin(log, cfg)
		return
	}

	if syncDeployment {
		runSyncDeployment(log, cfg)
		return
	}

	authStore, err := dashauth.NewStore(cfg.DBPath, cfg.SessionTTL)
	if err != nil {
		logutil.Fatal(log, "failed to open dashboard auth store", "err", err, "db", cfg.DBPath)
	}
	defer authStore.Close()

	bootstrap, err := authStore.BootstrapAdmin(cfg.AdminUsername)
	if err != nil {
		logutil.Fatal(log, "failed to bootstrap dashboard admin", "err", err)
	}

	if initOnly {
		deployStore, err := deployment.NewStore(cfg.DBPath)
		if err != nil {
			logutil.Fatal(log, "failed to open deployment store for init", "err", err, "db", cfg.DBPath)
		}
		agentKey, err := deployStore.EnsureAgentKey(cfg.AgentKey)
		deployStore.Close()
		if err != nil {
			logutil.Fatal(log, "failed to ensure agent key", "err", err)
		}
		if agentKey != "" && cfg.AgentKey == "" {
			fmt.Printf("TRINITY_AGENT_KEY=%s\n", agentKey)
		}

		if bootstrap.Created {
			printBootstrapCredentials(cfg.DashboardURL, bootstrap.Username, bootstrap.TempPassword, os.Getenv("TRINITY_BOOTSTRAP_DEFER_PRINT") == "1")
		} else {
			fmt.Println("Dashboard admin already exists; no credentials generated.")
		}
		return
	}

	deployStore, err := deployment.NewStore(cfg.DBPath)
	if err != nil {
		logutil.Fatal(log, "failed to open deployment store", "err", err, "db", cfg.DBPath)
	}
	defer deployStore.Close()

	if result, err := deployStore.SyncFromExternal(deploymentSyncOptions(false)); err != nil {
		log.Warn("deployment settings sync skipped", "err", err)
	} else if result.Updated {
		log.Info("deployment settings synced from host config",
			"public_domain", result.PublicDomain,
			"controller_url", result.ControllerURL,
			"ssl_mode", result.SSLMode,
		)
	}

	nodeStorage, err := storage.NewNodeStorage(cfg.NodesDBPath)
	if err != nil {
		logutil.Fatal(log, "failed to open node storage", "err", err, "db", cfg.NodesDBPath)
	}
	defer nodeStorage.Close()

	runtimeCfg := config.Load()
	prober := health.NewProberFromConfig(runtimeCfg)
	health.NewBackgroundProber(nodeStorage, prober, runtimeCfg.ProbeInterval, log).Start()

	server := dashboard.NewServer(cfg, authStore, deployStore, nodeStorage, log)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	if cfg.StaticDir != "" {
		registerStaticUI(mux, cfg.StaticDir, log)
	} else if embeddedUIAvailable() {
		ui, err := embeddedUIFS()
		if err != nil {
			logutil.Fatal(log, "failed to load embedded dashboard UI", "err", err)
		}
		registerEmbeddedUI(mux, ui, log)
	} else if !cfg.DevProxy {
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
			fmt.Fprintf(w, "Setup help: make start-dev | make dashboard-dev\n")
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
	if cfg.StaticDir != "" {
		log.Info("dashboard listening with built UI",
			"addr", addr,
			"static_dir", cfg.StaticDir,
			"auth_db", cfg.DBPath,
			"nodes_db", cfg.NodesDBPath,
		)
	} else if embeddedUIAvailable() {
		log.Info("dashboard listening with embedded UI",
			"addr", addr,
			"auth_db", cfg.DBPath,
			"nodes_db", cfg.NodesDBPath,
		)
	} else {
		log.Info("dashboard API listening (UI is a separate Vite dev server)",
			"addr", addr,
			"open_ui", cfg.DashboardURL,
			"ui_command", "cd web/dashboard && npm run dev",
			"make_hint", "make start-dev",
			"auth_db", cfg.DBPath,
			"nodes_db", cfg.NodesDBPath,
		)
	}
	log.Info("endpoints registered",
		"login", "POST /api/auth/login",
		"logout", "POST /api/auth/logout",
		"me", "GET /api/auth/me",
		"change_password", "POST /api/auth/change-password",
		"stats", "GET /api/dashboard/stats",
		"nodes", "GET /api/dashboard/nodes",
		"delete_node", "DELETE /api/dashboard/nodes/{id}",
		"node_credentials", "GET /api/dashboard/nodes/{id}/credentials",
		"bootstrap_script", "GET /api/dashboard/bootstrap-script",
		"deploy_commands", "GET /api/dashboard/deploy-commands",
		"deployment", "GET/PUT /api/dashboard/deployment",
		"regenerate_agent_key", "POST /api/dashboard/deployment/regenerate-agent-key",
		"dns_hints", "GET /api/dashboard/deployment/dns-hints",
		"dev_setup", "GET /api/dashboard/deployment/dev-setup",
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

func registerStaticUI(mux *http.ServeMux, staticDir string, log *slog.Logger) {
	absDir, err := filepath.Abs(staticDir)
	if err != nil {
		logutil.Fatal(log, "invalid DASHBOARD_STATIC_DIR", "err", err, "dir", staticDir)
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		logutil.Fatal(log, "DASHBOARD_STATIC_DIR is missing or not a directory", "dir", absDir)
	}
	indexPath := filepath.Join(absDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		logutil.Fatal(log, "DASHBOARD_STATIC_DIR missing index.html — run: cd web/dashboard && npm run build", "dir", absDir)
	}

	fileServer := http.FileServer(http.Dir(absDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/" {
			path := filepath.Join(absDir, filepath.Clean("/"+r.URL.Path))
			if rel, err := filepath.Rel(absDir, path); err != nil || strings.HasPrefix(rel, "..") {
				http.NotFound(w, r)
				return
			}
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, indexPath)
	})
	log.Info("serving built dashboard UI", "dir", absDir)
}





func deploymentSyncOptions(force bool) deployment.SyncOptions {
	return deployment.SyncOptions{
		Sources: deployment.ExternalSources{
			CaddySitePath:     envStringDefault("TRINITY_CADDY_SITE_PATH", deployment.DefaultCaddySitePath),
			ControllerEnvPath: envStringDefault("TRINITY_CONTROLLER_ENV_PATH", deployment.DefaultControllerEnvPath),
		},
		OverrideDomain:        strings.TrimSpace(os.Getenv("TRINITY_SYNC_PUBLIC_DOMAIN")),
		OverrideSSLMode:       strings.TrimSpace(os.Getenv("TRINITY_SYNC_SSL_MODE")),
		OverrideControllerURL: strings.TrimSpace(os.Getenv("TRINITY_SYNC_CONTROLLER_URL")),
		Force:                 force || strings.TrimSpace(os.Getenv("TRINITY_SYNC_FORCE")) == "1",
	}
}

func envStringDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func runSyncDeployment(log *slog.Logger, cfg dashboard.Config) {
	deployStore, err := deployment.NewStore(cfg.DBPath)
	if err != nil {
		logutil.Fatal(log, "failed to open deployment store", "err", err, "db", cfg.DBPath)
	}
	defer deployStore.Close()

	force := strings.TrimSpace(os.Getenv("TRINITY_SYNC_FORCE")) == "1" || strings.TrimSpace(os.Getenv("TRINITY_SYNC_PUBLIC_DOMAIN")) != ""
	result, err := deployStore.SyncFromExternal(deploymentSyncOptions(force))
	if err != nil {
		logutil.Fatal(log, "failed to sync deployment settings", "err", err)
	}

	if result.Updated {
		fmt.Println("TRINITY_DEPLOYMENT_SYNCED=1")
	} else {
		fmt.Println("TRINITY_DEPLOYMENT_SYNCED=0")
	}
	fmt.Printf("TRINITY_PUBLIC_DOMAIN=%s\n", result.PublicDomain)
	fmt.Printf("TRINITY_CONTROLLER_URL=%s\n", result.ControllerURL)
	fmt.Printf("TRINITY_SSL_MODE=%s\n", result.SSLMode)
}

func runResetAdmin(log *slog.Logger, cfg dashboard.Config) {
	if _, err := os.Stat(cfg.DBPath); err != nil {
		if os.IsNotExist(err) {
			logutil.Fatal(log, "dashboard database not found — install or bootstrap first",
				"db", cfg.DBPath,
				"hint", "run: sudo make start (production) or make dashboard-init (dev)",
			)
		}
		logutil.Fatal(log, "cannot access dashboard database", "err", err, "db", cfg.DBPath)
	}

	authStore, err := dashauth.NewStore(cfg.DBPath, cfg.SessionTTL)
	if err != nil {
		logutil.Fatal(log, "failed to open dashboard auth store", "err", err, "db", cfg.DBPath)
	}
	defer authStore.Close()

	bootstrap, err := authStore.ResetAdmin(cfg.AdminUsername)
	if err != nil {
		logutil.Fatal(log, "failed to reset dashboard admin", "err", err)
	}

	printBootstrapCredentials(cfg.DashboardURL, bootstrap.Username, bootstrap.TempPassword, false)
}

func printBootstrapCredentials(dashboardURL, username, tempPassword string, deferPrint bool) {
	if deferPrint {
		fmt.Printf("TRINITY_BOOTSTRAP_CREATED=1\n")
		fmt.Printf("TRINITY_BOOTSTRAP_URL=%s\n", dashboardURL)
		fmt.Printf("TRINITY_BOOTSTRAP_USERNAME=%s\n", username)
		fmt.Printf("TRINITY_BOOTSTRAP_PASSWORD=%s\n", tempPassword)
		return
	}

	sep := strings.Repeat("=", 60)
	lines := []string{
		"",
		sep,
		"TrinityProxy Master Dashboard — initial credentials",
		sep,
		fmt.Sprintf("Dashboard URL:  %s", dashboardURL),
		fmt.Sprintf("Username:       %s", username),
		fmt.Sprintf("Temp password:  %s", tempPassword),
		"",
		"First login requires a password change before accessing the dashboard.",
		sep,
		"",
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	slog.Default().Info("dashboard admin bootstrapped",
		"dashboard_url", dashboardURL,
		"username", username,
	)
}
