package dashboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/api"
	dashauth "github.com/Skillz147/TrinityProxy/internal/dashboard/auth"
	"github.com/Skillz147/TrinityProxy/internal/dashboard/deployment"
	"github.com/Skillz147/TrinityProxy/internal/metrics"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

type Server struct {
	cfg        Config
	auth       *dashauth.Store
	deployment *deployment.Store
	middleware *dashauth.Middleware
	nodes      storage.NodeStore
	log        *slog.Logger
}

func NewServer(cfg Config, authStore *dashauth.Store, deployStore *deployment.Store, nodes storage.NodeStore, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:        cfg,
		auth:       authStore,
		deployment: deployStore,
		middleware: dashauth.NewMiddleware(authStore),
		nodes:      nodes,
		log:        log.With("component", "dashboard"),
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.middleware.RequireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/auth/me", s.middleware.RequireAuth(s.handleMe))
	mux.HandleFunc("POST /api/auth/change-password", s.middleware.RequireAuth(s.handleChangePassword))
	mux.HandleFunc("GET /api/dashboard/stats", s.middleware.RequirePasswordChanged(s.handleStats))
	mux.HandleFunc("GET /api/dashboard/nodes", s.middleware.RequirePasswordChanged(s.handleNodes))
	mux.HandleFunc("GET /api/dashboard/nodes/{id}/credentials", s.middleware.RequirePasswordChanged(s.handleNodeCredentials))
	mux.HandleFunc("GET /api/dashboard/bootstrap-script", s.middleware.RequirePasswordChanged(s.handleBootstrapScript))
	mux.HandleFunc("GET /api/dashboard/deploy-commands", s.middleware.RequirePasswordChanged(s.handleDeployCommands))
	mux.HandleFunc("GET /api/dashboard/deployment", s.middleware.RequirePasswordChanged(s.handleGetDeployment))
	mux.HandleFunc("PUT /api/dashboard/deployment", s.middleware.RequirePasswordChanged(s.handlePutDeployment))
	mux.HandleFunc("POST /api/dashboard/deployment/regenerate-agent-key", s.middleware.RequirePasswordChanged(s.handleRegenerateAgentKey))
	mux.HandleFunc("GET /api/dashboard/deployment/dns-hints", s.middleware.RequirePasswordChanged(s.handleDNSHints))
	mux.HandleFunc("GET /api/dashboard/deployment/dev-setup", s.middleware.RequirePasswordChanged(s.handleDevSetup))
	mux.HandleFunc("GET /api/dashboard/deployment/cloudflare-setup", s.middleware.RequirePasswordChanged(s.handleCloudflareSetup))
	mux.HandleFunc("POST /api/dashboard/deployment/provision-ssl", s.middleware.RequirePasswordChanged(s.handleProvisionSSL))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	result, err := s.auth.Login(req.Username, req.Password)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":                result.Token,
		"must_change_password": result.MustChangePassword,
		"user":                 result.User,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := s.middleware.ExtractToken(r)
	if err := s.auth.Logout(token); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := dashauth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := dashauth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := s.auth.ChangePassword(user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		switch err {
		case dashauth.ErrInvalidPassword:
			writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		case dashauth.ErrPasswordUnchanged:
			writeJSONError(w, http.StatusBadRequest, "new password must differ from current password")
		default:
			if isValidationError(err) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
			} else {
				s.log.Error("change password failed", "err", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to change password")
			}
		}
		return
	}

	updated, err := s.auth.GetUserByID(user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "password_changed",
		"must_change_password": updated.MustChangePassword,
		"user":                 updated,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if err := s.nodes.MarkOfflineNodes(); err != nil {
		s.log.Error("failed to mark offline nodes", "err", err)
	}

	nodes, err := s.nodes.GetAllNodes()
	if err != nil {
		s.log.Error("failed to get nodes for stats", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}

	stats := computeDashboardStats(nodes, metrics.Current())
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if err := s.nodes.MarkOfflineNodes(); err != nil {
		s.log.Error("failed to mark offline nodes", "err", err)
	}

	nodes, err := s.nodes.GetAllNodes()
	if err != nil {
		s.log.Error("failed to get nodes", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load nodes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": api.ToPublicSlice(nodes),
		"count": len(nodes),
	})
}

func (s *Server) handleNodeCredentials(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "node id is required")
		return
	}

	node, err := s.nodes.GetNodeByID(id)
	if err != nil {
		s.log.Error("failed to get node credentials", "id", id, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load node credentials")
		return
	}
	if node == nil {
		writeJSONError(w, http.StatusNotFound, "node not found")
		return
	}

	admin := api.ToAdmin(*node)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                admin.ID,
		"ip":                admin.IP,
		"port":              admin.Port,
		"username":          admin.Username,
		"password":          admin.Password,
		"connection_string": fmt.Sprintf("socks5://%s:%s@%s:%d", admin.Username, admin.Password, admin.IP, admin.Port),
	})
}

func (s *Server) handleBootstrapScript(w http.ResponseWriter, r *http.Request) {
	controllerURL, err := s.deployment.EffectiveControllerURL(s.cfg.ControllerURL)
	if err != nil {
		s.log.Error("failed to load deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}
	controllerURL = strings.TrimRight(controllerURL, "/")
	if controllerURL == "" {
		writeJSONError(w, http.StatusBadRequest, "controller URL is not configured — set it in Settings → Deployment")
		return
	}

	agentKey, err := s.deployment.EnsureAgentKey(s.cfg.AgentKey)
	if err != nil {
		s.log.Error("failed to ensure agent key", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to prepare agent key")
		return
	}

	script := fmt.Sprintf(
		`curl -fsSL https://raw.githubusercontent.com/Skillz147/TrinityProxy/main/scripts/install-agent-service.sh | CONTROLLER_URL=%q TRINITY_AGENT_KEY=%q TRINITY_NONINTERACTIVE=1 bash`,
		controllerURL,
		agentKey,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"script":         script,
		"controller_url": controllerURL,
		"has_agent_key":  agentKey != "",
	})
}

func (s *Server) handleDeployCommands(w http.ResponseWriter, r *http.Request) {
	settings, err := s.deployment.Get()
	if err != nil {
		s.log.Error("failed to load deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}

	agentKey, err := s.deployment.EnsureAgentKey(s.cfg.AgentKey)
	if err != nil {
		s.log.Error("failed to ensure agent key", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to prepare agent key")
		return
	}

	commands := deployment.BuildDeployCommands(settings, s.cfg.ControllerURL, agentKey)
	writeJSON(w, http.StatusOK, commands)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	view, err := s.deployment.PublicView()
	if err != nil {
		s.log.Error("failed to get deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handlePutDeployment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicDomain        string `json:"public_domain"`
		ControllerPublicURL string `json:"controller_public_url"`
		SSLMode             string `json:"ssl_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	view, err := s.deployment.Update(req.PublicDomain, req.ControllerPublicURL, req.SSLMode)
	if err != nil {
		if err == deployment.ErrInvalidSSLMode {
			writeJSONError(w, http.StatusBadRequest, "invalid ssl_mode")
			return
		}
		if err == deployment.ErrInvalidDomain {
			writeJSONError(w, http.StatusBadRequest, "invalid public_domain — use a bare domain like trinityproxy.local (no https://)")
			return
		}
		s.log.Error("failed to update deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to update deployment config")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleRegenerateAgentKey(w http.ResponseWriter, r *http.Request) {
	if _, err := s.deployment.RegenerateAgentKey(); err != nil {
		s.log.Error("failed to regenerate agent key", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to regenerate agent key")
		return
	}

	view, err := s.deployment.PublicView()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "agent_key_regenerated",
		"has_agent_key": view.HasAgentKey,
		"message":       "Agent key rotated. Update TRINITY_AGENT_KEY on the controller and re-copy the bootstrap script for new agents.",
	})
}

func (s *Server) handleDNSHints(w http.ResponseWriter, r *http.Request) {
	settings, err := s.deployment.Get()
	if err != nil {
		s.log.Error("failed to load deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}

	domain := settings.PublicDomain
	sslMode := settings.SSLMode
	if q := strings.TrimSpace(r.URL.Query().Get("domain")); q != "" {
		domain = q
	}
	if q := strings.TrimSpace(r.URL.Query().Get("ssl_mode")); q != "" {
		sslMode = q
	}

	serverIP := s.serverPublicIP(r)
	hints := deployment.BuildDNSHints(domain, serverIP, sslMode)
	writeJSON(w, http.StatusOK, hints)
}

func (s *Server) handleDevSetup(w http.ResponseWriter, r *http.Request) {
	settings, err := s.deployment.Get()
	if err != nil {
		s.log.Error("failed to load deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}

	serverIP := s.serverPublicIP(r)
	setup := deployment.BuildDevSetup(settings.PublicDomain, serverIP)
	writeJSON(w, http.StatusOK, setup)
}

func (s *Server) handleCloudflareSetup(w http.ResponseWriter, r *http.Request) {
	settings, err := s.deployment.Get()
	if err != nil {
		s.log.Error("failed to load deployment config", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load deployment config")
		return
	}

	domain := settings.PublicDomain
	if q := strings.TrimSpace(r.URL.Query().Get("domain")); q != "" {
		domain = q
	}

	serverIP := s.serverPublicIP(r)
	setup := deployment.BuildCloudflareSetup(domain, serverIP)
	writeJSON(w, http.StatusOK, setup)
}

func (s *Server) handleProvisionSSL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain             string `json:"domain"`
		CloudflareAPIToken string `json:"cloudflare_api_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	req.Domain = strings.TrimSpace(req.Domain)
	req.CloudflareAPIToken = strings.TrimSpace(req.CloudflareAPIToken)

	if req.Domain == "" {
		writeJSONError(w, http.StatusBadRequest, "domain is required")
		return
	}

	normalized := deployment.NormalizeDomain(req.Domain)
	if normalized == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid domain")
		return
	}

	if req.CloudflareAPIToken == "" {
		writeJSONError(w, http.StatusBadRequest, "cloudflare_api_token is required")
		return
	}

	email := "ssl@" + normalized
	serverIP := s.serverPublicIP(r)

	scriptPath := "./scripts/setup-ssl-caddy-cloudflare.sh"
	if _, err := os.Stat(scriptPath); err != nil {
		s.log.Error("setup script not found", "path", scriptPath, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "SSL setup script not found")
		return
	}

	cmd := exec.Command("sudo", "env",
		"PUBLIC_DOMAIN="+normalized,
		"EMAIL="+email,
		"SERVER_IP="+serverIP,
		"CLOUDFLARE_API_TOKEN="+req.CloudflareAPIToken,
		"SKIP_DNS_WAIT=1",
		scriptPath,
	)

	s.log.Info("running Cloudflare SSL provision script", "domain", normalized, "email", email, "ip", serverIP)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.log.Error("SSL provisioning failed", "err", err, "output", string(out))
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("SSL provisioning failed: %v\nOutput:\n%s", err, string(out)))
		return
	}

	s.log.Info("SSL provisioning succeeded", "domain", normalized)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "success",
		"output": string(out),
	})
}

func (s *Server) serverPublicIP(r *http.Request) string {
	if ip := strings.TrimSpace(os.Getenv("SERVER_PUBLIC_IP")); ip != "" {
		return ip
	}
	host := strings.TrimSpace(r.Host)
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return deployment.ResolveServerIP(host, r.RemoteAddr)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func isValidationError(err error) bool {
	switch err {
	case dashauth.ErrEmptyPassword, dashauth.ErrPasswordTooShort, dashauth.ErrPasswordTooLong:
		return true
	default:
		return false
	}
}
