package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Skillz147/TrinityProxy/internal/api"
	"github.com/Skillz147/TrinityProxy/internal/auth"
	"github.com/Skillz147/TrinityProxy/internal/health"
	"github.com/Skillz147/TrinityProxy/internal/storage"
)

type mockNodeStore struct {
	upsertErr      error
	getOnlineErr   error
	getCountryErr  error
	markOfflineErr error
	nodes          []storage.ProxyNode
	countryNodes   map[string][]storage.ProxyNode
	lastUpsert     *storage.ProxyNode
	tokenHashes    map[string]string
}

func (m *mockNodeStore) UpsertNode(node *storage.ProxyNode) error {
	m.lastUpsert = node
	return m.upsertErr
}

func (m *mockNodeStore) GetOnlineNodes() ([]storage.ProxyNode, error) {
	if m.getOnlineErr != nil {
		return nil, m.getOnlineErr
	}
	return m.nodes, nil
}

func (m *mockNodeStore) GetNodeByID(id string) (*storage.ProxyNode, error) {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			node := m.nodes[i]
			return &node, nil
		}
	}
	return nil, nil
}

func (m *mockNodeStore) GetAllNodes() ([]storage.ProxyNode, error) {
	if m.getOnlineErr != nil {
		return nil, m.getOnlineErr
	}
	return m.nodes, nil
}

func (m *mockNodeStore) GetNodesByCountry(country string) ([]storage.ProxyNode, error) {
	if m.getCountryErr != nil {
		return nil, m.getCountryErr
	}
	if m.countryNodes != nil {
		if nodes, ok := m.countryNodes[country]; ok {
			return nodes, nil
		}
	}
	return nil, nil
}

func (m *mockNodeStore) MarkOfflineNodes() error {
	return m.markOfflineErr
}

func (m *mockNodeStore) DeleteNode(id string) error {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			m.nodes = append(m.nodes[:i], m.nodes[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockNodeStore) UpdateNodeHealth(id string, healthy bool, probedAt time.Time) error {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			m.nodes[i].IsHealthy = healthy
			t := probedAt
			m.nodes[i].LastProbeAt = &t
			break
		}
	}
	return nil
}

func (m *mockNodeStore) GetNodeTokenHash(nodeID string) (string, error) {
	if m.tokenHashes == nil {
		return "", nil
	}
	return m.tokenHashes[nodeID], nil
}

func (m *mockNodeStore) IssueNodeToken(nodeID string) (string, error) {
	token, err := auth.GenerateNodeToken()
	if err != nil {
		return "", err
	}
	if m.tokenHashes == nil {
		m.tokenHashes = map[string]string{}
	}
	m.tokenHashes[nodeID] = auth.HashNodeToken(token)
	return token, nil
}

func (m *mockNodeStore) RevokeNodeToken(nodeID string) error {
	if m.tokenHashes != nil {
		delete(m.tokenHashes, nodeID)
	}
	return nil
}

func (m *mockNodeStore) NodeHasToken(nodeID string) (bool, error) {
	hash, err := m.GetNodeTokenHash(nodeID)
	return hash != "", err
}

func (m *mockNodeStore) ValidateNodeToken(nodeID, token string) (bool, error) {
	hash, err := m.GetNodeTokenHash(nodeID)
	if err != nil || hash == "" {
		return false, err
	}
	return auth.ValidNodeToken(token, hash), nil
}

func (m *mockNodeStore) ClearCommandsForNode(nodeID string) error {
	return nil
}

func sampleNode(ip string, port int, country string) storage.ProxyNode {
	now := time.Now()
	return storage.ProxyNode{
		ID:        fmt.Sprintf("%s:%d", ip, port),
		IP:        ip,
		Port:      port,
		Username:  "proxyuser",
		Password:  "secret-pass",
		Country:   country,
		Region:    "TestRegion",
		City:      "TestCity",
		Zip:       "12345",
		IsOnline:  true,
		LastSeen:  now,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testProber() *health.Prober {
	return health.NewProber(health.WithDialFunc(func(network, address string) (net.Conn, error) {
		server, client := net.Pipe()
		go func() {
			defer server.Close()
			greeting := make([]byte, 3)
			if _, err := io.ReadFull(server, greeting); err != nil {
				return
			}
			if _, err := server.Write([]byte{0x05, 0x02}); err != nil {
				return
			}
			authHeader := make([]byte, 2)
			if _, err := io.ReadFull(server, authHeader); err != nil {
				return
			}
			user := make([]byte, authHeader[1])
			if _, err := io.ReadFull(server, user); err != nil {
				return
			}
			passLen := make([]byte, 1)
			if _, err := io.ReadFull(server, passLen); err != nil {
				return
			}
			pass := make([]byte, passLen[0])
			if _, err := io.ReadFull(server, pass); err != nil {
				return
			}
			_, _ = server.Write([]byte{0x01, 0x00})
		}()
		return client, nil
	}))
}

func newTestServer(store storage.NodeStore) *APIServer {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewAPIServerWithAuth(store, api.AgentAuthConfig{Log: log}, log, testProber())
	if ts, ok := store.(storage.NodeTokenStore); ok {
		server.tokens = ts
	}
	return server
}

func TestHandleHeartbeatSuccess(t *testing.T) {
	store := &mockNodeStore{}
	server := newTestServer(store)

	body := `{"ip":"203.0.113.1","port":1080,"username":"u","password":"p","country":"US","region":"CA","city":"LA","zip":"90001","platform":"linux","device_class":"vps","network_type":"datacenter"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var hbResp struct {
		Status          string `json:"status"`
		PendingCommands []any  `json:"pending_commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hbResp); err != nil {
		t.Fatalf("decode heartbeat response: %v body=%s", err, rec.Body.String())
	}
	if hbResp.Status != "ok" {
		t.Fatalf("status = %q, want ok", hbResp.Status)
	}
	if store.lastUpsert == nil {
		t.Fatal("expected UpsertNode to be called")
	}
	if store.lastUpsert.IP != "203.0.113.1" {
		t.Fatalf("upsert IP = %q, want 203.0.113.1", store.lastUpsert.IP)
	}
	if store.lastUpsert.Country != "US" {
		t.Fatalf("upsert country = %q, want US", store.lastUpsert.Country)
	}
	if store.lastUpsert.Platform != "linux" {
		t.Fatalf("upsert platform = %q, want linux", store.lastUpsert.Platform)
	}
	if store.lastUpsert.DeviceClass != "vps" {
		t.Fatalf("upsert device_class = %q, want vps", store.lastUpsert.DeviceClass)
	}
	if store.lastUpsert.NetworkType != "datacenter" {
		t.Fatalf("upsert network_type = %q, want datacenter", store.lastUpsert.NetworkType)
	}
}

func TestHandleHeartbeatStorageError(t *testing.T) {
	store := &mockNodeStore{upsertErr: errors.New("db down")}
	server := newTestServer(store)

	body := `{"ip":"203.0.113.1","port":1080,"username":"u","password":"p","country":"US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleHeartbeatInvalidJSON(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()

	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid JSON") {
		t.Fatalf("body = %q, want invalid JSON message", rec.Body.String())
	}
}

func TestHandleHeartbeatMethodNotAllowed(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/heartbeat", nil)
	rec := httptest.NewRecorder()

	server.handleHeartbeat(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleDeregisterDeletesNode(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewNodeStorage(filepath.Join(dir, "nodes.db"))
	if err != nil {
		t.Fatalf("NewNodeStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	node := &storage.ProxyNode{
		IP: "203.0.113.99", Port: 1080,
		Username: "u", Password: "p", Country: "US",
	}
	if err := store.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	server := newTestServer(store)
	body := `{"ip":"203.0.113.99","port":1080,"username":"u","password":"p","country":"US"}`
	req := httptest.NewRequest(http.MethodPost, "/api/deregister", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleDeregister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, err := store.GetNodeByID("203.0.113.99:1080")
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if got != nil {
		t.Fatal("expected node to be removed on deregister")
	}
}

func TestHandleGetNodesSuccess(t *testing.T) {
	nodes := []storage.ProxyNode{
		sampleNode("203.0.113.10", 1080, "US"),
		sampleNode("203.0.113.11", 1081, "DE"),
	}
	server := newTestServer(&mockNodeStore{nodes: nodes})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(resp["count"].(float64)) != 2 {
		t.Fatalf("count = %v, want 2", resp["count"])
	}
	if strings.Contains(rec.Body.String(), "secret-pass") {
		t.Fatal("public response must not contain password")
	}
	if strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatal("public response must not include password field")
	}
}

func TestHandleGetNodesStorageError(t *testing.T) {
	server := newTestServer(&mockNodeStore{getOnlineErr: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodes(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleGetNodesMethodNotAllowed(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/nodes", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodes(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGetNodesByCountrySuccess(t *testing.T) {
	usNodes := []storage.ProxyNode{sampleNode("203.0.113.20", 1080, "US")}
	store := &mockNodeStore{
		countryNodes: map[string][]storage.ProxyNode{"US": usNodes},
	}
	server := newTestServer(store)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/country?country=US", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodesByCountry(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["country"] != "US" {
		t.Fatalf("country = %v, want US", resp["country"])
	}
	if int(resp["count"].(float64)) != 1 {
		t.Fatalf("count = %v, want 1", resp["count"])
	}
	if strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatal("country filter response must not include password")
	}
}

func TestHandleGetNodesByCountryEmptyParam(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/country", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodesByCountry(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "country parameter required") {
		t.Fatalf("body = %q, want country parameter required", rec.Body.String())
	}
}

func TestHandleGetNodesByCountryMethodNotAllowed(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/nodes/country?country=US", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodesByCountry(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGetRandomNodePrefersCachedHealthy(t *testing.T) {
	nodes := []storage.ProxyNode{
		sampleNode("203.0.113.30", 1080, "US"),
		sampleNode("203.0.113.31", 1081, "DE"),
	}
	nodes[0].IsHealthy = true
	server := newTestServer(&mockNodeStore{nodes: nodes})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/random", nil)
	rec := httptest.NewRecorder()

	server.handleGetRandomNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var node map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &node); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if node["ip"] != "203.0.113.30" {
		t.Fatalf("random node ip = %v, want cached healthy node 203.0.113.30", node["ip"])
	}
}

func TestHandleGetRandomNodeSuccess(t *testing.T) {
	nodes := []storage.ProxyNode{
		sampleNode("203.0.113.30", 1080, "US"),
		sampleNode("203.0.113.31", 1081, "DE"),
	}
	server := newTestServer(&mockNodeStore{nodes: nodes})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/random", nil)
	rec := httptest.NewRecorder()

	server.handleGetRandomNode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var node map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &node); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	ip, ok := node["ip"].(string)
	if !ok {
		t.Fatalf("response missing ip: %v", node)
	}
	if ip != "203.0.113.30" && ip != "203.0.113.31" {
		t.Fatalf("random node ip = %q, want one of the online nodes", ip)
	}
	if _, hasPassword := node["password"]; hasPassword {
		t.Fatal("random node response must not include password")
	}
}

func TestHandleGetRandomNodeNotFound(t *testing.T) {
	server := newTestServer(&mockNodeStore{nodes: nil})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/random", nil)
	rec := httptest.NewRecorder()

	server.handleGetRandomNode(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetRandomNodeMethodNotAllowed(t *testing.T) {
	server := newTestServer(&mockNodeStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/nodes/random", nil)
	rec := httptest.NewRecorder()

	server.handleGetRandomNode(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGetNodesAdminIncludesPassword(t *testing.T) {
	nodes := []storage.ProxyNode{sampleNode("203.0.113.40", 1080, "US")}
	server := newTestServer(&mockNodeStore{nodes: nodes})

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/admin", nil)
	rec := httptest.NewRecorder()

	server.handleGetNodesAdmin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Nodes []map[string]interface{} `json:"nodes"`
		Count int                      `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("count = %d, want 1", resp.Count)
	}
	password, ok := resp.Nodes[0]["password"].(string)
	if !ok || password != "secret-pass" {
		t.Fatalf("admin response password = %v, want secret-pass", resp.Nodes[0]["password"])
	}
}

func TestHandlerAuthMiddleware(t *testing.T) {
	const apiKey = "test-api-key"
	const agentKey = "test-agent-key"
	const adminKey = "test-admin-key"

	store := &mockNodeStore{
		nodes: []storage.ProxyNode{sampleNode("203.0.113.50", 1080, "US")},
		countryNodes: map[string][]storage.ProxyNode{
			"US": {sampleNode("203.0.113.50", 1080, "US")},
		},
	}
	server := newTestServer(store)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		key        string
		authHeader string // "agent" uses X-Agent-Token, default X-API-Key
		expected   int
		wantSubstr string
	}{
		{
			name:     "nodes without key",
			method:   http.MethodGet,
			path:     "/api/nodes",
			expected: http.StatusUnauthorized,
		},
		{
			name:     "nodes with valid key",
			method:   http.MethodGet,
			path:     "/api/nodes",
			key:      apiKey,
			expected: http.StatusOK,
		},
		{
			name:     "heartbeat without agent key",
			method:   http.MethodPost,
			path:     "/api/heartbeat",
			body:     `{"ip":"1.2.3.4","port":1080,"username":"u","password":"p","country":"US"}`,
			expected: http.StatusUnauthorized,
		},
		{
			name:     "heartbeat with enrollment key",
			method:   http.MethodPost,
			path:     "/api/heartbeat",
			body:     `{"ip":"1.2.3.4","port":1080,"username":"u","password":"p","country":"US"}`,
			key:      agentKey,
			authHeader: "agent",
			expected: http.StatusOK,
		},
		{
			name:     "admin without key",
			method:   http.MethodGet,
			path:     "/api/nodes/admin",
			expected: http.StatusUnauthorized,
		},
		{
			name:     "admin with valid key",
			method:   http.MethodGet,
			path:     "/api/nodes/admin",
			key:      adminKey,
			expected: http.StatusOK,
		},
	}

	mux := http.NewServeMux()
	agentAuth := api.AgentAuthConfig{EnrollmentKey: agentKey, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	authServer := NewAPIServerWithAuth(store, agentAuth, slog.New(slog.NewTextHandler(io.Discard, nil)), testProber())
	authServer.tokens = store
	mux.HandleFunc("/api/heartbeat", authServer.handleHeartbeat)
	mux.HandleFunc("/api/nodes", api.WithAPIKey(apiKey, "trinity-api", server.handleGetNodes))
	mux.HandleFunc("/api/nodes/admin", api.WithAPIKey(adminKey, "trinity-admin", server.handleGetNodesAdmin))
	mux.HandleFunc("/api/nodes/country", api.WithAPIKey(apiKey, "trinity-api", server.handleGetNodesByCountry))
	mux.HandleFunc("/api/nodes/random", api.WithAPIKey(apiKey, "trinity-api", server.handleGetRandomNode))

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			if tc.key != "" {
				if tc.authHeader == "agent" {
					req.Header.Set("X-Agent-Token", tc.key)
				} else {
					req.Header.Set("X-API-Key", tc.key)
				}
			}
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.expected {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.expected, rec.Body.String())
			}
			if tc.expected == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), "invalid or missing agent token") && !strings.Contains(rec.Body.String(), "invalid or missing API key") {
				t.Fatalf("body = %q, want auth error message", rec.Body.String())
			}
		})
	}
}

func TestPublicResponseOmitsPasswordField(t *testing.T) {
	nodes := []storage.ProxyNode{sampleNode("203.0.113.60", 1080, "US")}
	server := newTestServer(&mockNodeStore{nodes: nodes})

	endpoints := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"nodes", server.handleGetNodes, "/api/nodes"},
		{"random", server.handleGetRandomNode, "/api/nodes/random"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep.path, nil)
			rec := httptest.NewRecorder()
			ep.handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			raw := rec.Body.Bytes()
			if bytes.Contains(raw, []byte(`"password"`)) {
				t.Fatalf("%s response contains password field: %s", ep.name, raw)
			}
		})
	}
}
