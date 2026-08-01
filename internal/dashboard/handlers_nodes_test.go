package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Skillz147/TrinityProxy/internal/storage"
)

func TestHandleDeleteNode(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	node := &storage.ProxyNode{
		IP: "203.0.113.55", Port: 1080,
		Username: "proxy", Password: "secret", Country: "US",
	}
	if err := server.nodes.UpsertNode(node); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	token := loginWithPasswordChanged(t, server, mux)
	nodeID := "203.0.113.55:1080"

	req := httptest.NewRequest(http.MethodDelete, "/api/dashboard/nodes/"+nodeID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "removed" {
		t.Fatalf("status = %q, want removed", resp["status"])
	}

	got, err := server.nodes.GetNodeByID(nodeID)
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if got != nil {
		t.Fatal("expected node to be deleted from storage")
	}
}

func TestHandleDeleteNodeNotFound(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	token := loginWithPasswordChanged(t, server, mux)

	req := httptest.NewRequest(http.MethodDelete, "/api/dashboard/nodes/missing:1080", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d, want 404", rec.Code)
	}
}

func loginWithPasswordChanged(t *testing.T, server *Server, mux *http.ServeMux) string {
	t.Helper()

	login, err := server.auth.Login("admin", "temp-pass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := server.auth.ChangePassword(login.User.ID, "temp-pass", "permanent-password-123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "permanent-password-123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected login token")
	}
	return loginResp.Token
}

func loginAndGetToken(t *testing.T, mux *http.ServeMux) string {
	t.Helper()

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "temp-pass",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected login token")
	}
	return loginResp.Token
}
