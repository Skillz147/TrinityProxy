// cmd/api/enhanced_main.go
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
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
	storage storage.NodeStore
	prober  *health.Prober
	log     *slog.Logger
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
	opts := []health.ProberOption{health.WithLocalFallback(!cfg.IsProduction())}
	return health.NewProber(opts...)
}

func NewAPIServerWithStore(store storage.NodeStore, log *slog.Logger, prober *health.Prober) *APIServer {
	if log == nil {
		log = slog.Default()
	}
	if prober == nil {
		prober = health.NewProber()
	}
	return &APIServer{
		storage: store,
		prober:  prober,
		log:     log.With("component", "api"),
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

	var meta NodeMetadata
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
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
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
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
	ticker := time.NewTicker(1 * time.Minute)
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

	server.startCleanupRoutine()
	server.startBackgroundProbes(cfg.ProbeInterval)
	server.refreshNodesOnlineMetric()

	http.HandleFunc("/api/heartbeat", api.WithAPIKey(cfg.AgentKey, "trinity-agent", server.handleHeartbeat))
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
