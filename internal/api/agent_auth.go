package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/auth"
)

// NodeTokenStore validates and issues per-node agent tokens.
type NodeTokenStore interface {
	GetNodeTokenHash(nodeID string) (string, error)
	IssueNodeToken(nodeID string) (string, error)
}

// AgentAuthConfig holds fleet-wide keys used during enrollment and legacy transition.
type AgentAuthConfig struct {
	EnrollmentKey  string // TRINITY_ENROLLMENT_KEY (preferred)
	LegacyAgentKey string // TRINITY_AGENT_KEY (deprecated fleet-wide key)
	Log            *slog.Logger
}

// EnrollmentKeyEffective returns the enrollment key, falling back to the legacy fleet key.
func (c AgentAuthConfig) EnrollmentKeyEffective() string {
	if k := strings.TrimSpace(c.EnrollmentKey); k != "" {
		return k
	}
	return strings.TrimSpace(c.LegacyAgentKey)
}

// AuthDisabled reports whether agent endpoints allow unauthenticated access (dev mode).
func (c AgentAuthConfig) AuthDisabled() bool {
	return c.EnrollmentKeyEffective() == ""
}

// ExtractAgentToken reads the agent token from Authorization: Bearer, X-Agent-Token, or X-API-Key.
func ExtractAgentToken(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-Agent-Token")); key != "" {
		return key
	}
	return ExtractAPIKey(r)
}

func validFleetKey(provided, expected string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// HeartbeatMeta is the minimum node identity sent in heartbeat/deregister payloads.
type HeartbeatMeta struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// NodeIDFromMeta returns the canonical node identifier ip:port.
func NodeIDFromMeta(meta HeartbeatMeta) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(meta.IP), meta.Port)
}

var errInvalidAgentToken = fmt.Errorf("invalid or missing agent token")

// ValidateAgentAccess checks whether the request may proceed.
// When enroll is true, the caller must upsert the node then call IssueNodeToken.
func (c AgentAuthConfig) ValidateAgentAccess(store NodeTokenStore, nodeID, providedToken string) (enroll bool, err error) {
	if c.AuthDisabled() {
		return false, nil
	}

	providedToken = strings.TrimSpace(providedToken)
	storedHash, err := store.GetNodeTokenHash(nodeID)
	if err != nil {
		return false, err
	}

	if storedHash != "" {
		if !auth.ValidNodeToken(providedToken, storedHash) {
			return false, errInvalidAgentToken
		}
		return false, nil
	}

	fleetKey := c.EnrollmentKeyEffective()
	if !validFleetKey(providedToken, fleetKey) {
		return false, errInvalidAgentToken
	}

	if c.LegacyAgentKey != "" && strings.TrimSpace(c.EnrollmentKey) == "" && validFleetKey(providedToken, c.LegacyAgentKey) {
		if c.Log != nil {
			c.Log.Warn("agent authenticated with deprecated TRINITY_AGENT_KEY fleet key — migrate to TRINITY_ENROLLMENT_KEY and per-node tokens")
		}
	}
	return true, nil
}

// ValidateAgentAccessHTTP validates access and writes HTTP errors on failure.
func (c AgentAuthConfig) ValidateAgentAccessHTTP(w http.ResponseWriter, store NodeTokenStore, nodeID, providedToken string) (enroll bool, ok bool) {
	enroll, err := c.ValidateAgentAccess(store, nodeID, providedToken)
	if err == errInvalidAgentToken {
		w.Header().Set("WWW-Authenticate", `Bearer realm="trinity-agent"`)
		http.Error(w, "invalid or missing agent token", http.StatusUnauthorized)
		return false, false
	}
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return false, false
	}
	return enroll, true
}

// ParseHeartbeatMeta decodes heartbeat/deregister JSON into HeartbeatMeta.
func ParseHeartbeatMeta(r *http.Request) (HeartbeatMeta, []byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return HeartbeatMeta{}, nil, err
	}
	var meta HeartbeatMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return HeartbeatMeta{}, body, err
	}
	return meta, body, nil
}

// WithNodeAgentAuth wraps handlers that identify a node via query param node_id.
func WithNodeAgentAuth(cfg AgentAuthConfig, store NodeTokenStore, handler func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
		if nodeID == "" {
			http.Error(w, "node_id required", http.StatusBadRequest)
			return
		}
		token := ExtractAgentToken(r)
		if _, ok := cfg.ValidateAgentAccessHTTP(w, store, nodeID, token); !ok {
			return
		}
		handler(w, r, nodeID)
	}
}
