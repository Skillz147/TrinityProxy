// cmd/api/enhanced_main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

type NodeMetadata struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	City     string `json:"city"`
	Zip      string `json:"zip"`
}

type APIServer struct {
	storage *storage.NodeStorage
}

func NewAPIServer(dbPath string) (*APIServer, error) {
	nodeStorage, err := storage.NewNodeStorage(dbPath)
	if err != nil {
		return nil, err
	}

	return &APIServer{
		storage: nodeStorage,
	}, nil
}

func (api *APIServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var meta NodeMetadata
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert to storage format
	node := &storage.ProxyNode{
		IP:       meta.IP,
		Port:     meta.Port,
		Username: meta.Username,
		Password: meta.Password,
		Country:  meta.Country,
		Region:   meta.Region,
		City:     meta.City,
	}

	// Store/update node
	if err := api.storage.UpsertNode(node); err != nil {
		log.Printf("[-] Failed to store node: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	log.Printf("[+] Received heartbeat: %s:%d (%s, %s)", meta.IP, meta.Port, meta.City, meta.Country)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (api *APIServer) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Mark offline nodes before returning
	api.storage.MarkOfflineNodes()

	nodes, err := api.storage.GetOnlineNodes()
	if err != nil {
		log.Printf("[-] Failed to get nodes: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

func (api *APIServer) handleGetNodesByCountry(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	country := r.URL.Query().Get("country")
	if country == "" {
		http.Error(w, "country parameter required", http.StatusBadRequest)
		return
	}

	nodes, err := api.storage.GetNodesByCountry(country)
	if err != nil {
		log.Printf("[-] Failed to get nodes by country: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes":   nodes,
		"country": country,
		"count":   len(nodes),
	})
}

func (api *APIServer) handleGetRandomNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes, err := api.storage.GetOnlineNodes()
	if err != nil {
		log.Printf("[-] Failed to get nodes: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	if len(nodes) == 0 {
		http.Error(w, "no nodes available", http.StatusNotFound)
		return
	}

	// Select random node
	randomNode := nodes[rand.Intn(len(nodes))]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(randomNode)
}

func (api *APIServer) startCleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			if err := api.storage.MarkOfflineNodes(); err != nil {
				log.Printf("[-] Cleanup error: %v", err)
			}
		}
	}()
}

func main() {
	api, err := NewAPIServer("./trinityproxy.db")
	if err != nil {
		log.Fatalf("[-] Failed to initialize API server: %v", err)
	}

	// Start cleanup routine
	api.startCleanupRoutine()

	// Routes
	http.HandleFunc("/api/heartbeat", api.handleHeartbeat)
	http.HandleFunc("/api/nodes", api.handleGetNodes)
	http.HandleFunc("/api/nodes/country", api.handleGetNodesByCountry)
	http.HandleFunc("/api/nodes/random", api.handleGetRandomNode)

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	log.Println("[*] Enhanced API server listening on :3100")
	log.Println("[*] Available endpoints:")
	log.Println("    POST /api/heartbeat     - Node heartbeat")
	log.Println("    GET  /api/nodes         - List all online nodes")
	log.Println("    GET  /api/nodes/country?country=US - Filter by country")
	log.Println("    GET  /api/nodes/random  - Get random node")
	log.Println("    GET  /health            - Health check")

	if err := http.ListenAndServe(":3100", nil); err != nil {
		log.Fatalf("[-] API server failed: %v", err)
	}
}
