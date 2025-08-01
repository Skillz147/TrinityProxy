// cmd/api-server/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

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
