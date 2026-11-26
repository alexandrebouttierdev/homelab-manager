package httpdelivery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func callWithToken(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = testClientAddress + ":4000"
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func createToken(t *testing.T, router http.Handler, cookie *http.Cookie, body string) (string, string) {
	t.Helper()
	response := call(router, http.MethodPost, "/api/v1/auth/tokens", body, cookie)
	if response.Code != http.StatusCreated || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create token = %d %s", response.Code, response.Body)
	}
	var created struct {
		Token    string `json:"token"`
		APIToken struct {
			ID     string `json:"id"`
			Prefix string `json:"prefix"`
		} `json:"apiToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, "mop_") || !strings.HasPrefix(created.Token, created.APIToken.Prefix) {
		t.Fatalf("token = %+v", created)
	}
	return created.Token, created.APIToken.ID
}

func TestAPITokens(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	viewer := signInWithRole(t, router, admin, "vera", "viewer")

	token, id := createToken(t, router, viewer, `{"name":"Home Assistant","scopes":["services:read","events:read"],"expiresInDays":30}`)
	if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", token); response.Code != http.StatusOK {
		t.Fatalf("units with token = %d %s", response.Code, response.Body)
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/api/v1/containers", http.StatusForbidden},
		{http.MethodGet, "/api/v1/dashboard", http.StatusForbidden},
		{http.MethodPost, "/api/v1/system/units/nginx.service/actions", http.StatusForbidden},
		{http.MethodGet, "/api/v1/users", http.StatusForbidden},
		{http.MethodGet, "/api/v1/auth/tokens", http.StatusForbidden},
		{http.MethodGet, "/api/v1/audit", http.StatusForbidden},
		{http.MethodPost, "/api/v1/auth/logout", http.StatusForbidden},
		{http.MethodPut, "/api/v1/settings/retention", http.StatusForbidden},
	} {
		response := callWithToken(router, tc.method, tc.path, token)
		if response.Code != tc.code || !strings.Contains(response.Body.String(), "TOKEN_SCOPE_DENIED") {
			t.Errorf("%s %s = %d %s, want %d TOKEN_SCOPE_DENIED", tc.method, tc.path, response.Code, response.Body, tc.code)
		}
	}
	for _, bad := range []string{"mop_unknown", token[:len(token)-4] + "AAAA", ""} {
		if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", bad); response.Code != http.StatusUnauthorized {
			t.Errorf("token %q = %d", bad, response.Code)
		}
	}

	list := call(router, http.MethodGet, "/api/v1/auth/tokens", "", viewer).Body.String()
	if !strings.Contains(list, `"name":"Home Assistant"`) || strings.Contains(list, token) || !strings.Contains(list, `"lastUsedAt":"`) {
		t.Fatalf("list = %s", list)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/tokens?scope=all", "", viewer); response.Code != http.StatusForbidden {
		t.Fatalf("viewer list all = %d", response.Code)
	}
	adminToken, adminID := createToken(t, router, admin, `{"name":"Grafana","scopes":["services:read"]}`)
	all := call(router, http.MethodGet, "/api/v1/auth/tokens?scope=all", "", admin).Body.String()
	if !strings.Contains(all, `"username":"vera"`) || !strings.Contains(all, `"username":"admin"`) {
		t.Fatalf("all = %s", all)
	}
	if response := call(router, http.MethodDelete, "/api/v1/auth/tokens/"+adminID, "", viewer); response.Code != http.StatusNotFound {
		t.Fatalf("viewer revokes admin token = %d", response.Code)
	}
	if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", adminToken); response.Code != http.StatusOK {
		t.Fatalf("admin token = %d", response.Code)
	}
	// An administrator revokes the token of another account.
	if response := call(router, http.MethodDelete, "/api/v1/auth/tokens/"+id, "", admin); response.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", response.Code, response.Body)
	}
	if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", token); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d", response.Code)
	}
	audit := call(router, http.MethodGet, "/api/v1/audit?targetType=api_token", "", admin).Body.String()
	if !strings.Contains(audit, `"action":"revoke"`) || !strings.Contains(audit, `"action":"create"`) {
		t.Fatalf("audit = %s", audit)
	}
}

func TestAPITokenOfADisabledAccount(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	operator := signInWithRole(t, router, admin, "olivia", "operator")
	token, _ := createToken(t, router, operator, `{"name":"script","scopes":["services:read"]}`)
	if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", token); response.Code != http.StatusOK {
		t.Fatalf("units = %d %s", response.Code, response.Body)
	}
	users := call(router, http.MethodGet, "/api/v1/users", "", admin).Body.String()
	start := strings.Index(users, `"username":"olivia"`)
	idStart := strings.LastIndex(users[:start], `"id":"`) + 6
	userID := users[idStart : idStart+36]
	if response := call(router, http.MethodPatch, "/api/v1/users/"+userID, `{"disabled":true}`, admin); response.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", response.Code, response.Body)
	}
	if response := callWithToken(router, http.MethodGet, "/api/v1/system/units", token); response.Code != http.StatusUnauthorized {
		t.Fatalf("token of a disabled account = %d", response.Code)
	}
}

func TestAPITokenValidation(t *testing.T) {
	resetLoginRateLimit(t)
	services, _ := newTestServices(t, nil)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, body := range []string{
		`{"name":"","scopes":["metrics:read"]}`,
		`{"name":"x","scopes":[]}`,
		`{"name":"x","scopes":["services:manage"]}`,
		`{"name":"x","scopes":["metrics:read"],"expiresInDays":366}`,
		`{"name":"x","scopes":["metrics:read"],"extra":1}`,
	} {
		if response := call(router, http.MethodPost, "/api/v1/auth/tokens", body, admin); response.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", body, response.Code)
		}
	}
	for i := 0; i < 20; i++ {
		createToken(t, router, admin, `{"name":"t","scopes":["metrics:read"]}`)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/tokens", `{"name":"t","scopes":["metrics:read"]}`, admin); response.Code != http.StatusConflict {
		t.Fatalf("21st token = %d", response.Code)
	}
}

func TestTokenScopeFor(t *testing.T) {
	cases := map[string]string{
		"/api/v1/host":                      "metrics:read",
		"/api/v1/host/metrics/cpu.percent":  "metrics:read",
		"/api/v1/hostile":                   "",
		"/api/v1/containers/abc/events":     "containers:read",
		"/api/v1/system/units/ssh.service/": "services:read",
		"/api/v1/backups/runs/x":            "backups:read",
		"/api/v1/backups/config":            "",
		"/api/v1/backups/repository/init":   "",
		"/api/v1/users":                     "",
	}
	for path, want := range cases {
		scope, ok := tokenScopeFor(path)
		if string(scope) != want || ok != (want != "") {
			t.Errorf("%s = %q %v, want %q", path, scope, ok, want)
		}
	}
}
