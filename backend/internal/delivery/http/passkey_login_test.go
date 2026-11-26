package httpdelivery

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"monoops/backend/internal/adapters/sqlite"

	"go.uber.org/zap"
)

func TestPasskeyLoginEndToEnd(t *testing.T) {
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	verifier := &fakeVerifier{}
	keys := sqlite.NewSecondFactorRepository(database.Client())
	services.Auth.WithWebAuthn(verifier, keys)
	services.SecondFactors.WithWebAuthn(verifier, keys)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")
	var me struct {
		User struct{ ID string } `json:"user"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/session", "", admin), &me)

	begin := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn", `{"password":"admin-password-1"}`, admin)
	registration := challengeOfOptions(t, begin.Body.Bytes())
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/confirm", fmt.Sprintf(`{"name":"Téléphone","response":{"answer":"ok:%s"}}`, registration), admin); response.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", response.Code, response.Body)
	}

	// Off by default.
	if body := call(router, http.MethodGet, "/api/v1/auth/login/passkey", "", nil).Body.String(); !strings.Contains(body, `"available":false`) {
		t.Fatalf("status = %s", body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey", "", nil); errorCode(t, response) != "PASSKEY_LOGIN_DISABLED" {
		t.Fatalf("begin while disabled = %d %s", response.Code, response.Body)
	}
	policy := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":[],"allowPasskeyLogin":true}`, admin)
	if policy.Code != http.StatusOK || !strings.Contains(policy.Body.String(), `"allowPasskeyLogin":true`) {
		t.Fatalf("enable = %d %s", policy.Code, policy.Body)
	}
	// Absent from a later update, the setting is kept.
	if body := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":45,"allowedNetworks":[]}`, admin).Body.String(); !strings.Contains(body, `"allowPasskeyLogin":true`) {
		t.Fatalf("update without the setting = %s", body)
	}
	if body := call(router, http.MethodGet, "/api/v1/auth/login/passkey", "", nil).Body.String(); !strings.Contains(body, `"available":true`) {
		t.Fatalf("status = %s", body)
	}

	options := call(router, http.MethodPost, "/api/v1/auth/login/passkey", "", nil)
	ceremony := cookieNamed(t, options, passkeyCookieName)
	if !ceremony.HttpOnly || ceremony.SameSite != http.SameSiteStrictMode || ceremony.Path != passkeyCookiePath {
		t.Fatalf("ceremony cookie = %+v", ceremony)
	}
	session := challengeOfOptions(t, options.Body.Bytes())
	answer := fmt.Sprintf(`{"response":{"answer":"ok:%s:%s:cred-%s"}}`, session, me.User.ID, registration)
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey/finish", answer, nil); errorCode(t, response) != "PASSKEY_CEREMONY_EXPIRED" {
		t.Fatalf("answer without ceremony = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey/finish", `{}`, ceremony); response.Code != http.StatusBadRequest {
		t.Fatalf("answer without response = %d", response.Code)
	}
	completed := call(router, http.MethodPost, "/api/v1/auth/login/passkey/finish", answer, ceremony)
	if completed.Code != http.StatusOK || !strings.Contains(completed.Body.String(), `"username":"admin"`) {
		t.Fatalf("passkey login = %d %s", completed.Code, completed.Body)
	}
	if cleared := cookieNamed(t, completed, passkeyCookieName); cleared.MaxAge >= 0 {
		t.Fatalf("ceremony cookie kept: %+v", cleared)
	}
	signedIn := cookieNamed(t, completed, sessionCookieName)
	if body := call(router, http.MethodGet, "/api/v1/auth/sessions", "", signedIn).Body.String(); !strings.Contains(body, `"authMethod":"passkey"`) {
		t.Fatalf("sessions = %s", body)
	}
	// The same answer cannot open a second session.
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey/finish", answer, ceremony); errorCode(t, response) != "PASSKEY_CEREMONY_EXPIRED" {
		t.Fatalf("replay = %d %s", response.Code, response.Body)
	}

	options = call(router, http.MethodPost, "/api/v1/auth/login/passkey", "", nil)
	wrong := fmt.Sprintf(`{"response":{"answer":"ok:%s:%s:cred-other"}}`, challengeOfOptions(t, options.Body.Bytes()), me.User.ID)
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey/finish", wrong, cookieNamed(t, options, passkeyCookieName)); errorCode(t, response) != "INVALID_PASSKEY" {
		t.Fatalf("unknown key = %d %s", response.Code, response.Body)
	}
}

func TestPasskeyLoginWithoutWebAuthn(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":[],"allowPasskeyLogin":true}`, admin)
	if body := call(router, http.MethodGet, "/api/v1/auth/login/passkey", "", nil).Body.String(); !strings.Contains(body, `"available":false`) {
		t.Fatalf("status without WebAuthn = %s", body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/passkey", "", nil); errorCode(t, response) != "WEBAUTHN_UNAVAILABLE" {
		t.Fatalf("begin without WebAuthn = %d %s", response.Code, response.Body)
	}
}
