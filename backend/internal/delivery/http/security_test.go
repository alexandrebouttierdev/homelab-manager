package httpdelivery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/go-chi/chi/v5"
)

func TestSessionAndAuditRoutesValidateTheirInput(t *testing.T) {
	// Nil usecases prove validation happens before any change.
	router := chi.NewRouter()
	router.Delete("/api/v1/auth/sessions/{sessionID}", revokeSessionHandler(nil, false))
	router.Put("/api/v1/security/policy", updateSecurityPolicyHandler(nil))
	router.Get("/api/v1/audit", listAuditHandler(nil))
	router.Get("/api/v1/audit/export", exportAuditHandler(nil))
	cases := []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/auth/sessions/not-a-uuid", ""},
		{http.MethodPut, "/api/v1/security/policy", `{}`},
		{http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60}`},
		{http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":[],"extra":1}`},
		{http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":4,"allowedNetworks":[]}`},
		{http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":["10.0.0.1"]}`},
		{http.MethodGet, "/api/v1/audit?from=yesterday", ""},
		{http.MethodGet, "/api/v1/audit?limit=5000", ""},
		{http.MethodGet, "/api/v1/audit?limit=abc", ""},
		{http.MethodGet, "/api/v1/audit?from=2026-11-23T10:00:00Z&to=2026-11-23T09:00:00Z", ""},
		{http.MethodGet, "/api/v1/audit/export", ""},
		{http.MethodGet, "/api/v1/audit/export?format=xml", ""},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s returned %d, want 400", c.method, c.path, c.body, recorder.Code)
		}
	}
}

func TestCSVExportNeutralisesFormulas(t *testing.T) {
	for value, want := range map[string]string{
		"=cmd":    "'=cmd",
		"+1":      "'+1",
		"-1":      "'-1",
		"@SUM":    "'@SUM",
		"GET /x":  "GET /x",
		"":        "",
		"\tvalue": "'\tvalue",
	} {
		if got := csvCell(value); got != want {
			t.Errorf("csvCell(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestSecurityEndToEnd(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")

	// Policy: default, self-exclusion refused, then restricted networks.
	var policy struct {
		IdleTimeoutMinutes int      `json:"idleTimeoutMinutes"`
		AllowedNetworks    []string `json:"allowedNetworks"`
		CurrentAddress     string   `json:"currentAddress"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/security/policy", "", admin), &policy)
	if policy.IdleTimeoutMinutes != 60 || len(policy.AllowedNetworks) != 0 || policy.CurrentAddress != testClientAddress {
		t.Fatalf("default policy = %+v", policy)
	}
	if response := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":["10.0.0.0/8"]}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "POLICY_EXCLUDES_CURRENT_ADDRESS" {
		t.Fatalf("self-exclusion = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":30,"allowedNetworks":["198.51.100.0/24"]}`, admin); response.Code != http.StatusOK {
		t.Fatalf("update policy = %d %s", response.Code, response.Body)
	}
	if response := callFrom(router, "203.0.113.5", http.MethodGet, "/api/v1/health", "", nil); response.Code != http.StatusForbidden || errorCode(t, response) != "NETWORK_NOT_ALLOWED" {
		t.Fatalf("request from outside = %d %s, want 403 NETWORK_NOT_ALLOWED", response.Code, response.Body)
	}
	if response := callFrom(router, "203.0.113.5", http.MethodGet, "/health", "", nil); response.Code != http.StatusOK {
		t.Fatalf("/health from outside = %d, want 200", response.Code)
	}

	// Lockout: five wrong passwords lock alice, even for the right one.
	if response := call(router, http.MethodPost, "/api/v1/users", `{"username":"alice","displayName":"Alice","role":"viewer","password":"initial-password-1"}`, admin); response.Code != http.StatusCreated {
		t.Fatalf("create alice = %d %s", response.Code, response.Body)
	}
	for range domain.MaxConsecutiveFailures {
		if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"wrong-password"}`, nil); response.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password = %d", response.Code)
		}
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"initial-password-1"}`, nil); response.Code != http.StatusUnauthorized || errorCode(t, response) != "INVALID_CREDENTIALS" {
		t.Fatalf("locked account = %d %s, want 401 INVALID_CREDENTIALS", response.Code, response.Body)
	}

	// Sessions: two admin browsers, the other one revoked.
	second := signIn(t, router, "admin", "admin-password-1")
	var sessions struct {
		Sessions []struct {
			ID         string `json:"id"`
			Username   string `json:"username"`
			AuthMethod string `json:"authMethod"`
			Current    bool   `json:"current"`
		} `json:"sessions"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/sessions", "", second), &sessions)
	current := 0
	for _, session := range sessions.Sessions {
		if session.Current {
			current++
		}
		if session.Username != "admin" || session.AuthMethod != "password" {
			t.Fatalf("session = %+v", session)
		}
	}
	if len(sessions.Sessions) != 2 || current != 1 {
		t.Fatalf("admin sessions = %+v, want 2 with one current", sessions.Sessions)
	}
	var revoked struct {
		Revoked int `json:"revoked"`
	}
	decodeBody(t, call(router, http.MethodPost, "/api/v1/auth/sessions/revoke-others", "", second), &revoked)
	if revoked.Revoked != 1 {
		t.Fatalf("revoke others = %+v", revoked)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/session", "", admin); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked browser = %d, want 401", response.Code)
	}

	// A viewer asking for every session is refused and the refusal audited.
	resetLoginRateLimit(t)
	bobBody := `{"username":"bob","displayName":"Bob","role":"viewer","password":"initial-password-1"}`
	if response := call(router, http.MethodPost, "/api/v1/users", bobBody, second); response.Code != http.StatusCreated {
		t.Fatalf("create bob = %d %s", response.Code, response.Body)
	}
	bob := signIn(t, router, "bob", "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"bob-own-password"}`, bob); response.Code != http.StatusOK {
		t.Fatalf("bob password = %d", response.Code)
	}
	bob = signIn(t, router, "bob", "bob-own-password")
	if response := call(router, http.MethodGet, "/api/v1/auth/sessions?scope=all", "", bob); response.Code != http.StatusForbidden || errorCode(t, response) != "FORBIDDEN" {
		t.Fatalf("viewer listing every session = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/audit", "", bob); response.Code != http.StatusForbidden {
		t.Fatalf("viewer reading the audit = %d, want 403", response.Code)
	}

	// Audit: denied entries, lock and export.
	var audit struct {
		Entries []struct {
			Actor      string `json:"actor"`
			TargetType string `json:"targetType"`
			Action     string `json:"action"`
			Outcome    string `json:"outcome"`
		} `json:"entries"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/audit?outcome=denied", "", second), &audit)
	if len(audit.Entries) != 2 || audit.Entries[0].Actor != "bob" || audit.Entries[0].TargetType != "route" {
		t.Fatalf("denied entries = %+v, want bob's two refusals", audit.Entries)
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/audit?action=lock", "", second), &audit)
	if len(audit.Entries) != 1 || audit.Entries[0].Actor != "system" {
		t.Fatalf("lock entries = %+v", audit.Entries)
	}
	csvResponse := call(router, http.MethodGet, "/api/v1/audit/export?format=csv", "", second)
	if csvResponse.Code != http.StatusOK || !strings.HasPrefix(csvResponse.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(csvResponse.Header().Get("Content-Disposition"), "attachment") ||
		!strings.HasPrefix(csvResponse.Body.String(), "createdAt,actor,targetType,targetId,action,outcome,requestId\n") {
		t.Fatalf("CSV export = %d %v %q", csvResponse.Code, csvResponse.Header(), csvResponse.Body.String())
	}
	jsonResponse := call(router, http.MethodGet, "/api/v1/audit/export?format=json", "", second)
	var exported []map[string]any
	if err := json.Unmarshal(jsonResponse.Body.Bytes(), &exported); err != nil || len(exported) < 5 {
		t.Fatalf("JSON export = (%d entries, %v)", len(exported), err)
	}
}
