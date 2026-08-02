package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/api"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func (s *Server) commandsStore() storage.CommandStore {
	if cs, ok := s.nodes.(storage.CommandStore); ok {
		return cs
	}
	return nil
}

func (s *Server) handleEnqueueNodeCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cs := s.commandsStore()
	if cs == nil {
		writeJSONError(w, http.StatusInternalServerError, "command store unavailable")
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		writeJSONError(w, http.StatusBadRequest, "node id is required")
		return
	}

	node, err := s.nodes.GetNodeByID(nodeID)
	if err != nil {
		s.log.Error("failed to load node for command", "id", nodeID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load node")
		return
	}
	if node == nil {
		writeJSONError(w, http.StatusNotFound, "node not found")
		return
	}

	var req struct {
		Action   string            `json:"action"`
		LogLevel string            `json:"log_level"`
		Params   map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	params := req.Params
	if params == nil {
		params = map[string]string{}
	}
	if req.LogLevel != "" {
		params["log_level"] = req.LogLevel
	}

	cmd, err := cs.EnqueueCommand(nodeID, action, params)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "queued",
		"command": cmd,
	})
}

func enrichNodesWithCommands(nodes []storage.ProxyNode, cs storage.CommandStore) []map[string]any {
	if len(nodes) == 0 {
		return []map[string]any{}
	}

	nodeIDs := make([]string, len(nodes))
	for i, node := range nodes {
		nodeIDs[i] = node.ID
	}

	latest := map[string]storage.AgentCommand{}
	if cs != nil {
		if m, err := cs.GetLatestCommandsForNodes(nodeIDs); err == nil {
			latest = m
		}
	}

	out := make([]map[string]any, len(nodes))
	for i, node := range nodes {
		pub := api.ToPublic(node)
		data, _ := json.Marshal(pub)
		var item map[string]any
		_ = json.Unmarshal(data, &item)

		if cmd, ok := latest[node.ID]; ok {
			item["remote_command"] = map[string]any{
				"id":           cmd.ID,
				"action":       cmd.Action,
				"status":       cmd.Status,
				"result":       cmd.Result,
				"created_at":   cmd.CreatedAt,
				"updated_at":   cmd.UpdatedAt,
				"completed_at": cmd.CompletedAt,
			}
		}
		item["has_node_token"] = node.HasNodeToken
		out[i] = item
	}
	return out
}
