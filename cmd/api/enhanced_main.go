// cmd/api/enhanced_main.go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/api"
	"github.com/Skillz147/TrinityProxy/internal/config"
	"github.com/Skillz147/TrinityProxy/internal/geo"
	"github.com/Skillz147/TrinityProxy/internal/health"
	"github.com/Skillz147/TrinityProxy/internal/logutil"
	"github.com/Skillz147/TrinityProxy/internal/metrics"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

type NodeMetadata struct {
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Country     string `json:"country"`
	Region      string `json:"region"`
	City        string `json:"city"`
	Zip         string `json:"zip"`
	Platform    string `json:"platform"`
	DeviceClass string `json:"device_class"`
	NetworkType string `json:"network_type"`
}

type APIServer struct {
	storage  storage.NodeStore
	tokens   storage.NodeTokenStore
	commands storage.CommandStore
	agentAuth api.AgentAuthConfig
	prober   *health.Prober
	log      *slog.Logger
}

func NewAPIServer(dbPath string, log *slog.Logger) (*APIServer, error) {
	nodeStorage, err := storage.NewNodeStorage(dbPath)
	if err != nil {
		return nil, err
	}

	cfg := config.Load()
	return NewAPIServerWithStore(nodeStorage, log, newProber(cfg)), nil
}

func newProber(cfg config.Config) *health.Prober {
	return health.NewProberFromConfig(cfg)
}

func NewAPIServerWithStore(store storage.NodeStore, log *slog.Logger, prober *health.Prober) *APIServer {
	return NewAPIServerWithAuth(store, api.AgentAuthConfig{Log: log}, log, prober)
}

func NewAPIServerWithAuth(store storage.NodeStore, agentAuth api.AgentAuthConfig, log *slog.Logger, prober *health.Prober) *APIServer {
	if log == nil {
		log = slog.Default()
	}
	if agentAuth.Log == nil {
		agentAuth.Log = log
	}
	if prober == nil {
		prober = health.NewProber()
	}
	var commands storage.CommandStore
	if cs, ok := store.(storage.CommandStore); ok {
		commands = cs
	}
	var tokens storage.NodeTokenStore
	if ts, ok := store.(storage.NodeTokenStore); ok {
		tokens = ts
	}
	return &APIServer{
		storage:   store,
		tokens:    tokens,
		commands:  commands,
		agentAuth: agentAuth,
		prober:    prober,
		log:       log.With("component", "api"),
	}
}

func (s *APIServer) probeNodeHealth(node *storage.ProxyNode) {
	nodeID := fmt.Sprintf("%s:%d", node.IP, node.Port)
	healthy := s.prober.ProbeFresh(node.IP, node.Port, node.Username, node.Password)
	if err := s.storage.UpdateNodeHealth(nodeID, healthy, time.Now()); err != nil {
		s.log.Error("failed to update node health after heartbeat", "err", err, "node_id", nodeID)
	}
}

func (s *APIServer) refreshNodesOnlineMetric() {
	nodes, err := s.storage.GetOnlineNodes()
	if err != nil {
		s.log.Error("failed to refresh nodes_online metric", "err", err)
		return
	}
	metrics.SetNodesOnline(len(nodes))
}

func (s *APIServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "heartbeat")
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hbMeta, body, err := api.ParseHeartbeatMeta(r)
	if err != nil {
		log.Warn("invalid heartbeat payload", "err", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	nodeID := api.NodeIDFromMeta(hbMeta)
	providedToken := api.ExtractAgentToken(r)
	enroll, ok := s.agentAuth.ValidateAgentAccessHTTP(w, s.tokens, nodeID, providedToken)
	if !ok {
		return
	}

	var meta NodeMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		log.Warn("invalid heartbeat payload", "err", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	node := &storage.ProxyNode{
		IP:          meta.IP,
		Port:        meta.Port,
		Username:    meta.Username,
		Password:    meta.Password,
		Country:     geo.NormalizeCountry(meta.Country),
		Region:      meta.Region,
		City:        meta.City,
		Zip:         meta.Zip,
		Platform:    meta.Platform,
		DeviceClass: meta.DeviceClass,
		NetworkType: meta.NetworkType,
	}

	nodeID = fmt.Sprintf("%s:%d", meta.IP, meta.Port)
	if s.commands != nil {
		existing, err := s.storage.GetNodeByID(nodeID)
		if err != nil {
			log.Error("failed to check node before registration", "err", err, "node_id", nodeID)
		} else if existing == nil {
			if err := s.commands.ClearCommandsForNode(nodeID); err != nil {
				log.Warn("failed to clear stale commands on re-registration", "err", err, "node_id", nodeID)
			}
		}
	}

	if err := s.storage.UpsertNode(node); err != nil {
		log.Error("failed to store node", "err", err, "ip", meta.IP, "port", meta.Port)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	metrics.IncHeartbeatsReceived()
	s.refreshNodesOnlineMetric()
	s.probeNodeHealth(node)

	log.Info("heartbeat received",
		"ip", meta.IP,
		"port", meta.Port,
		"city", meta.City,
		"country", meta.Country,
		"platform", meta.Platform,
		"device_class", meta.DeviceClass,
		"network_type", meta.NetworkType,
	)

	var pending []map[string]any
	if s.commands != nil {
		commands, err := s.commands.GetPendingCommands(nodeID)
		if err != nil {
			log.Error("failed to load pending commands", "err", err, "node_id", nodeID)
		} else if len(commands) > 0 {
			ids := make([]string, len(commands))
			for i, cmd := range commands {
				ids[i] = cmd.ID
				pending = append(pending, map[string]any{
					"id":     cmd.ID,
					"action": cmd.Action,
					"params": cmd.Params,
				})
			}
			if err := s.commands.MarkCommandsRunning(ids); err != nil {
				log.Error("failed to mark commands running", "err", err, "node_id", nodeID)
			}
		}
	}

	resp := map[string]any{
		"status":            "ok",
		"pending_commands": pending,
	}
	if enroll && s.tokens != nil {
		nodeToken, err := s.tokens.IssueNodeToken(nodeID)
		if err != nil {
			log.Error("failed to issue node token", "err", err, "node_id", nodeID)
		} else {
			resp["node_token"] = nodeToken
			log.Info("issued per-node token", "node_id", nodeID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *APIServer) handleDeregister(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "deregister")
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hbMeta, body, err := api.ParseHeartbeatMeta(r)
	if err != nil {
		log.Warn("invalid deregister payload", "err", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	nodeID := api.NodeIDFromMeta(hbMeta)
	providedToken := api.ExtractAgentToken(r)
	if _, ok := s.agentAuth.ValidateAgentAccessHTTP(w, s.tokens, nodeID, providedToken); !ok {
		return
	}

	var meta NodeMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		log.Warn("invalid deregister payload", "err", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	nodeID = fmt.Sprintf("%s:%d", meta.IP, meta.Port)
	if err := s.storage.DeleteNode(nodeID); err != nil {
		if err == sql.ErrNoRows {
			log.Info("deregister for unknown node (already removed)", "node_id", nodeID)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
			return
		}
		log.Error("failed to delete node on deregister", "err", err, "node_id", nodeID)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	s.refreshNodesOnlineMetric()
	log.Info("agent deregistered", "node_id", nodeID, "ip", meta.IP, "port", meta.Port)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (s *APIServer) handleAgentCommands(w http.ResponseWriter, r *http.Request, nodeID string) {
	log := s.log.With("component", "agent-commands")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.commands == nil {
		writeJSON(w, http.StatusOK, map[string]any{"pending_commands": []any{}})
		return
	}

	commands, err := s.commands.GetPendingCommands(nodeID)
	if err != nil {
		log.Error("failed to get pending commands", "err", err, "node_id", nodeID)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	pending := make([]map[string]any, 0, len(commands))
	ids := make([]string, len(commands))
	for i, cmd := range commands {
		ids[i] = cmd.ID
		pending = append(pending, map[string]any{
			"id":     cmd.ID,
			"action": cmd.Action,
			"params": cmd.Params,
		})
	}
	if len(ids) > 0 {
		if err := s.commands.MarkCommandsRunning(ids); err != nil {
			log.Error("failed to mark commands running", "err", err, "node_id", nodeID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"pending_commands": pending})
}

func (s *APIServer) handleCommandResult(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "command-result")
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.commands == nil {
		http.Error(w, "command store unavailable", http.StatusInternalServerError)
		return
	}

	var req struct {
		CommandID string `json:"command_id"`
		NodeID    string `json:"node_id"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		Status    string `json:"status"`
		Result    string `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.NodeID = strings.TrimSpace(req.NodeID)
	if req.CommandID == "" {
		http.Error(w, "command_id required", http.StatusBadRequest)
		return
	}
	if req.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}

	providedToken := api.ExtractAgentToken(r)
	if _, ok := s.agentAuth.ValidateAgentAccessHTTP(w, s.tokens, req.NodeID, providedToken); !ok {
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != "success" && status != "failure" {
		http.Error(w, "status must be success or failure", http.StatusBadRequest)
		return
	}

	cmd, err := s.commands.GetCommandByID(req.CommandID)
	if err != nil {
		log.Error("failed to load command", "err", err, "command_id", req.CommandID)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if cmd == nil {
		http.Error(w, "command not found", http.StatusNotFound)
		return
	}
	if cmd.NodeID != req.NodeID {
		log.Warn("command result node mismatch", "command_id", req.CommandID, "expected_node", cmd.NodeID, "provided_node", req.NodeID)
		http.Error(w, "command does not belong to node", http.StatusForbidden)
		return
	}

	node, err := s.storage.GetNodeByID(req.NodeID)
	if err != nil {
		log.Error("failed to load node for command result", "err", err, "node_id", req.NodeID)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if node == nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if req.Username == "" || req.Password == "" || node.Username != req.Username || node.Password != req.Password {
		log.Warn("command result credential mismatch", "command_id", req.CommandID, "node_id", req.NodeID)
		http.Error(w, "node credentials mismatch", http.StatusForbidden)
		return
	}

	if err := s.commands.CompleteCommandForNode(req.CommandID, req.NodeID, status, req.Result); err != nil {
		log.Error("failed to complete command", "err", err, "command_id", req.CommandID)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	if status == storage.CommandStatusSuccess && cmd.Action == storage.CommandActionRepair {
		if err := s.commands.ClearCommandsForNode(cmd.NodeID); err != nil {
			log.Warn("failed to clear commands after repair", "err", err, "node_id", cmd.NodeID)
		}
	}

	if status == storage.CommandStatusSuccess && cmd.Action == storage.CommandActionUninstall {
		if err := s.storage.DeleteNode(cmd.NodeID); err != nil && err != sql.ErrNoRows {
			log.Error("failed to delete node after uninstall", "err", err, "node_id", cmd.NodeID)
		} else {
			s.refreshNodesOnlineMetric()
			log.Info("node removed after uninstall", "node_id", cmd.NodeID)
		}
	}

	log.Info("command completed", "command_id", req.CommandID, "status", status, "action", cmd.Action)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *APIServer) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "nodes")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.storage.MarkOfflineNodes()
	s.refreshNodesOnlineMetric()

	nodes, err := s.storage.GetOnlineNodes()
	if err != nil {
		log.Error("failed to get nodes", "err", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": api.ToPublicSlice(nodes),
		"count": len(nodes),
	})
}

func (s *APIServer) handleGetNodesByCountry(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "nodes")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	countryQuery := r.URL.Query().Get("country")
	if countryQuery == "" {
		http.Error(w, "country parameter required", http.StatusBadRequest)
		return
	}

	nodes, err := s.storage.GetNodesByCountry(countryQuery)
	if err != nil {
		log.Error("failed to get nodes by country", "err", err, "country", geo.NormalizeCountry(countryQuery))
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes":   api.ToPublicSlice(nodes),
		"country": geo.NormalizeCountry(countryQuery),
		"count":   len(nodes),
	})
}

func (s *APIServer) handleGetRandomNode(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "nodes")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes, err := s.storage.GetOnlineNodes()
	if err != nil {
		log.Error("failed to get nodes", "err", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	if len(nodes) == 0 {
		http.Error(w, "no nodes available", http.StatusNotFound)
		return
	}

	var healthy []storage.ProxyNode
	for _, node := range nodes {
		if node.IsHealthy {
			healthy = append(healthy, node)
		}
	}

	if len(healthy) == 0 {
		for _, node := range nodes {
			if s.prober.IsHealthy(node.IP, node.Port, node.Username, node.Password) {
				healthy = append(healthy, node)
			}
		}
	}

	if len(healthy) == 0 {
		http.Error(w, "no healthy nodes available", http.StatusNotFound)
		return
	}

	randomNode := healthy[rand.Intn(len(healthy))]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.ToPublic(randomNode))
}

func (s *APIServer) handleGetNodesAdmin(w http.ResponseWriter, r *http.Request) {
	log := s.log.With("component", "nodes")
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.storage.MarkOfflineNodes()
	s.refreshNodesOnlineMetric()

	nodes, err := s.storage.GetOnlineNodes()
	if err != nil {
		log.Error("failed to get nodes", "err", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": api.ToAdminSlice(nodes),
		"count": len(nodes),
	})
}

func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.storage.MarkOfflineNodes()
	s.refreshNodesOnlineMetric()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics.Current())
}

func (s *APIServer) startCleanupRoutine() {
	log := s.log.With("component", "cleanup")
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			if err := s.storage.MarkOfflineNodes(); err != nil {
				log.Error("cleanup error", "err", err)
				continue
			}
			s.refreshNodesOnlineMetric()
		}
	}()
}

func (s *APIServer) startBackgroundProbes(interval time.Duration) {
	health.NewBackgroundProber(s.storage, s.prober, interval, s.log).Start()
}

func adminKey(cfg config.Config) string {
	if k := os.Getenv("TRINITY_ADMIN_KEY"); k != "" {
		return k
	}
	return cfg.APIKey
}

func main() {
	log := logutil.New("server")
	cfg := config.Load()
	cfg.LogSecurityWarnings(log)

	server, err := NewAPIServer(cfg.DBPath, log)
	if err != nil {
		log.Error("failed to initialize API server", "err", err)
		os.Exit(1)
	}

	agentAuth := api.AgentAuthConfig{
		EnrollmentKey:  cfg.EnrollmentKey,
		LegacyAgentKey: cfg.AgentKey,
		Log:            log,
	}
	server.agentAuth = agentAuth

	server.startCleanupRoutine()
	server.startBackgroundProbes(cfg.ProbeInterval)
	server.refreshNodesOnlineMetric()

	http.HandleFunc("/api/heartbeat", server.handleHeartbeat)
	http.HandleFunc("/api/deregister", server.handleDeregister)
	http.HandleFunc("/api/agent/commands", api.WithNodeAgentAuth(agentAuth, server.tokens, server.handleAgentCommands))
	http.HandleFunc("/api/agent/command-result", server.handleCommandResult)
	http.HandleFunc("/api/nodes", api.WithAPIKey(cfg.APIKey, "trinity-api", server.handleGetNodes))
	http.HandleFunc("/api/nodes/admin", api.WithAPIKey(adminKey(cfg), "trinity-admin", server.handleGetNodesAdmin))
	http.HandleFunc("/api/nodes/country", api.WithAPIKey(cfg.APIKey, "trinity-api", server.handleGetNodesByCountry))
	http.HandleFunc("/api/nodes/random", api.WithAPIKey(cfg.APIKey, "trinity-api", server.handleGetRandomNode))

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	http.HandleFunc("/metrics", server.handleMetrics)

	log.Info("API server listening", "addr", cfg.ListenAddr(), "db", cfg.DBPath)
	log.Info("endpoints registered",
		"heartbeat", "POST /api/heartbeat",
		"deregister", "POST /api/deregister",
		"agent_commands", "GET /api/agent/commands",
		"command_result", "POST /api/agent/command-result",
		"nodes", "GET /api/nodes",
		"nodes_admin", "GET /api/nodes/admin",
		"nodes_country", "GET /api/nodes/country?country=US",
		"nodes_random", "GET /api/nodes/random",
		"health", "GET /health",
		"metrics", "GET /metrics",
	)

	addr := cfg.ListenAddr()
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		log.Error("API port already in use or unavailable (IPv4 bind failed)", "err", err, "addr", addr)
		os.Exit(1)
	}
	if err := http.Serve(ln, nil); err != nil {
		log.Error("API server failed", "err", err)
		os.Exit(1)
	}
}
