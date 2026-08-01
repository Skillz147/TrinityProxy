package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dashauth "github.com/Skillz147/TrinityProxy/internal/dashboard/auth"
)

func TestHandleDeployCommandsLogLevelQuery(t *testing.T) {
	server := newTestServer(t)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	session := loginAndChangePassword(t, mux)

	for _, tc := range []struct {
		query    string
		contains string
	}{
		{"logLevel=silent", "TRINITY_LOG_LEVEL=silent"},
		{"logLevel=quiet", "TRINITY_LOG_LEVEL=quiet"},
		{"logLevel=debug", "TRINITY_LOG_LEVEL=debug"},
		{"", "TRINITY_LOG_LEVEL=info"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			path := "/api/dashboard/deploy-commands"
			if tc.query != "" {
				path += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(session)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}

			var payload struct {
				Platforms []struct {
					ID         string `json:"id"`
					Command    string `json:"command"`
					Operations []struct {
						ID      string `json:"id"`
						Command string `json:"command"`
					} `json:"operations"`
				} `json:"platforms"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode: %v", err)
			}

			linux := findDeployPlatform(payload.Platforms, "linux-vps")
			if linux == nil {
				t.Fatal("linux-vps platform missing")
			}
			install := findDeployOperation(linux.Operations, "install")
			if install == nil {
				t.Fatal("linux install operation missing")
			}
			if !strings.Contains(install.Command, tc.contains) {
				t.Errorf("linux install command missing %q: %s", tc.contains, install.Command)
			}

			win := findDeployPlatform(payload.Platforms, "windows")
			if win == nil {
				t.Fatal("windows platform missing")
			}
			winInstall := findDeployOperation(win.Operations, "install")
			if winInstall == nil {
				t.Fatal("windows install operation missing")
			}
			level := strings.TrimPrefix(tc.contains, "TRINITY_LOG_LEVEL=")
			if !strings.Contains(winInstall.Command, "$env:TRINITY_LOG_LEVEL='"+level+"'") {
				t.Errorf("windows install missing $env:TRINITY_LOG_LEVEL='%s': %s", level, winInstall.Command)
			}
		})
	}
}

func loginAndChangePassword(t *testing.T, mux *http.ServeMux) *http.Cookie {
	t.Helper()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"temp-pass"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}

	session := findCookie(loginRec.Result().Cookies(), dashauth.SessionCookieName)
	if session == nil {
		t.Fatal("expected session cookie")
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{"current_password":"temp-pass","new_password":"new-pass-123"}`))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(session)
	changeRec := httptest.NewRecorder()
	mux.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusOK {
		t.Fatalf("change-password status = %d, body = %s", changeRec.Code, changeRec.Body.String())
	}

	return session
}

func findDeployPlatform(platforms []struct {
	ID         string `json:"id"`
	Command    string `json:"command"`
	Operations []struct {
		ID      string `json:"id"`
		Command string `json:"command"`
	} `json:"operations"`
}, id string) *struct {
	ID         string `json:"id"`
	Command    string `json:"command"`
	Operations []struct {
		ID      string `json:"id"`
		Command string `json:"command"`
	} `json:"operations"`
} {
	for i := range platforms {
		if platforms[i].ID == id {
			return &platforms[i]
		}
	}
	return nil
}

func findDeployOperation(ops []struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}, id string) *struct {
	ID      string `json:"id"`
	Command string `json:"command"`
} {
	for i := range ops {
		if ops[i].ID == id {
			return &ops[i]
		}
	}
	return nil
}
