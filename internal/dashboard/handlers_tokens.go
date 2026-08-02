package dashboard

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func (s *Server) tokenStore() storage.NodeTokenStore {
	if ts, ok := s.nodes.(storage.NodeTokenStore); ok {
		return ts
	}
	return nil
}

func (s *Server) handleNodeTokenStatus(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		writeJSONError(w, http.StatusBadRequest, "node id is required")
		return
	}

	node, err := s.nodes.GetNodeByID(nodeID)
	if err != nil {
		s.log.Error("failed to load node token status", "id", nodeID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load node")
		return
	}
	if node == nil {
		writeJSONError(w, http.StatusNotFound, "node not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":          node.ID,
		"has_node_token":   node.HasNodeToken,
		"token_created_at": node.TokenCreatedAt,
	})
}

func (s *Server) handleRegenerateNodeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ts := s.tokenStore()
	if ts == nil {
		writeJSONError(w, http.StatusInternalServerError, "token store unavailable")
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		writeJSONError(w, http.StatusBadRequest, "node id is required")
		return
	}

	node, err := s.nodes.GetNodeByID(nodeID)
	if err != nil {
		s.log.Error("failed to load node for token regeneration", "id", nodeID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load node")
		return
	}
	if node == nil {
		writeJSONError(w, http.StatusNotFound, "node not found")
		return
	}

	token, err := ts.IssueNodeToken(nodeID)
	if err != nil {
		s.log.Error("failed to regenerate node token", "id", nodeID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to regenerate node token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "node_token_regenerated",
		"node_id":    nodeID,
		"node_token": token,
		"message":    "Copy this token now — it will not be shown again. Update the agent config or revoke to force re-enrollment.",
	})
}

func (s *Server) handleRevokeNodeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ts := s.tokenStore()
	if ts == nil {
		writeJSONError(w, http.StatusInternalServerError, "token store unavailable")
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		writeJSONError(w, http.StatusBadRequest, "node id is required")
		return
	}

	if err := ts.RevokeNodeToken(nodeID); err != nil {
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		s.log.Error("failed to revoke node token", "id", nodeID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to revoke node token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "node_token_revoked",
		"node_id": nodeID,
		"message": "Node token revoked. The agent must re-enroll with the enrollment key on its next heartbeat.",
	})
}
