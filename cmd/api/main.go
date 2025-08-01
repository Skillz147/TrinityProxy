// cmd/api-server/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var meta NodeMetadata
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[+] Received heartbeat: %s:%d (%s, %s)", meta.IP, meta.Port, meta.City, meta.Country)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func main() {
	http.HandleFunc("/api/heartbeat", handleHeartbeat)

	log.Println("[*] API server listening on :3000")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		log.Fatalf("[-] API server failed: %v", err)
	}
}
