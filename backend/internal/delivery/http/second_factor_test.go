package httpdelivery

import (
	"encoding/base32"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/domain"
)

func cookieNamed(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("no %s cookie in %v", name, response.Result().Cookies())
	return nil
}

func TestSecondFactorLoginValidatesTheBody(t *testing.T) {
	handler := completeSecondFactorHandler(nil, false)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/second-factor", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("without auth = %d, want 503", recorder.Code)
	}
}

func TestSecondFactorEndToEnd(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")

	var status struct {
		Enabled   bool `json:"enabled"`
		Available bool `json:"available"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/second-factor", "", admin), &status)
	if status.Enabled || !status.Available {
		t.Fatalf("status = %+v", status)
	}
	if response := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":[],"requireAdminSecondFactor":true}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "SECOND_FACTOR_NOT_ENROLLED" {
		t.Fatalf("requiring without an own factor = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/totp", `{"password":"wrong"}`, admin); response.Code != http.StatusUnauthorized {
		t.Fatalf("enroll with a wrong password = %d", response.Code)
	}
	var start struct {
		Secret string `json:"secret"`
		URI    string `json:"uri"`
		QR     struct {
			Size int      `json:"size"`
			Rows []string `json:"rows"`
		} `json:"qr"`
	}
	decodeBody(t, call(router, http.MethodPost, "/api/v1/auth/second-factor/totp", `{"password":"admin-password-1"}`, admin), &start)
	if start.QR.Size < 21 || len(start.QR.Rows) != start.QR.Size || !strings.HasPrefix(start.URI, "otpauth://totp/") {
		t.Fatalf("enrollment start = %+v", start)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(start.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/totp/confirm", `{"code":"000000"}`, admin); errorCode(t, response) != "INVALID_SECOND_FACTOR" {
		t.Fatalf("wrong confirmation = %d %s", response.Code, response.Body)
	}
	confirmCode := domain.TOTPCode(secret, domain.TOTPStep(time.Now()))
	var confirmed struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	decodeBody(t, call(router, http.MethodPost, "/api/v1/auth/second-factor/totp/confirm", fmt.Sprintf(`{"code":%q}`, confirmCode), admin), &confirmed)
	if len(confirmed.RecoveryCodes) != domain.RecoveryCodeCount {
		t.Fatalf("recovery codes = %v", confirmed.RecoveryCodes)
	}
	if response := call(router, http.MethodPut, "/api/v1/security/policy", `{"idleTimeoutMinutes":60,"allowedNetworks":[],"requireAdminSecondFactor":true}`, admin); response.Code != http.StatusOK {
		t.Fatalf("require the second factor = %d %s", response.Code, response.Body)
	}

	// Two-step login: the password gives a challenge, not a session.
	login := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin-password-1"}`, nil)
	if !strings.Contains(login.Body.String(), `"secondFactorRequired":true`) {
		t.Fatalf("password step = %d %s", login.Code, login.Body)
	}
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			t.Fatal("no session before the second factor")
		}
	}
	challenge := cookieNamed(t, login, challengeCookieName)
	if challenge.Path != challengeCookiePath || !challenge.HttpOnly {
		t.Fatalf("challenge cookie = %+v", challenge)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", `{"method":"totp","code":"000000"}`, challenge); errorCode(t, response) != "INVALID_SECOND_FACTOR" {
		t.Fatalf("wrong code = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", fmt.Sprintf(`{"method":"totp","code":%q}`, confirmCode), challenge); errorCode(t, response) != "INVALID_SECOND_FACTOR" {
		t.Fatalf("replayed confirmation code = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", `{"method":"recovery","code":"x"}`, nil); errorCode(t, response) != "LOGIN_CHALLENGE_EXPIRED" {
		t.Fatalf("without challenge = %d %s", response.Code, response.Body)
	}
	completed := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", fmt.Sprintf(`{"method":"recovery","code":%q}`, strings.ToUpper(confirmed.RecoveryCodes[0])), challenge)
	if completed.Code != http.StatusOK {
		t.Fatalf("recovery code = %d %s", completed.Code, completed.Body)
	}
	session := cookieNamed(t, completed, sessionCookieName)
	var sessions struct {
		Sessions []struct {
			AuthMethod string `json:"authMethod"`
			Current    bool   `json:"current"`
		} `json:"sessions"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/sessions", "", session), &sessions)
	found := false
	for _, s := range sessions.Sessions {
		if s.Current && s.AuthMethod == "password+recovery" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sessions = %+v, want the current one opened with a recovery code", sessions.Sessions)
	}

	// Another administrator without a factor is limited to the enrollment.
	resetLoginRateLimit(t)
	if response := call(router, http.MethodPost, "/api/v1/users", `{"username":"root2","displayName":"Root","role":"admin","password":"initial-password-1"}`, session); response.Code != http.StatusCreated {
		t.Fatalf("create root2 = %d %s", response.Code, response.Body)
	}
	root2 := signIn(t, router, "root2", "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"root2-own-password"}`, root2); response.Code != http.StatusOK {
		t.Fatalf("root2 password = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"root2","password":"root2-own-password"}`, nil); !strings.Contains(response.Body.String(), `"mustEnrollSecondFactor":true`) {
		t.Fatalf("root2 login = %d %s, want the enrollment obligation in the response", response.Code, response.Body)
	}
	root2 = signIn(t, router, "root2", "root2-own-password")
	if response := call(router, http.MethodGet, "/api/v1/settings", "", root2); response.Code != http.StatusForbidden || errorCode(t, response) != "SECOND_FACTOR_REQUIRED" {
		t.Fatalf("root2 without a factor = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/second-factor", "", root2); response.Code != http.StatusOK {
		t.Fatalf("root2 enrollment status = %d", response.Code)
	}
	if !strings.Contains(call(router, http.MethodGet, "/api/v1/auth/session", "", root2).Body.String(), `"mustEnrollSecondFactor":true`) {
		t.Fatal("the session tells the interface to enroll")
	}
}
