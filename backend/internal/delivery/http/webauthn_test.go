package httpdelivery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// fakeVerifier accepts {"answer":"ok:<session>"} to a registration, which
// creates the credential "cred-<session>", and
// {"answer":"ok:<session>:<credential>"} to a login.
type fakeVerifier struct{ ceremonies int }

func (f *fakeVerifier) next() ([]byte, []byte) {
	f.ceremonies++
	session := fmt.Sprintf("s%d", f.ceremonies)
	return []byte(`{"publicKey":{"challenge":"` + session + `"}}`), []byte(session)
}

func answerOf(response []byte) string {
	var parsed struct {
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal(response, &parsed)
	return parsed.Answer
}

func (f *fakeVerifier) BeginRegistration(domain.User, []domain.WebAuthnCredential) ([]byte, []byte, error) {
	options, session := f.next()
	return options, session, nil
}
func (f *fakeVerifier) FinishRegistration(_ domain.User, _ []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	if answerOf(response) != "ok:"+string(session) {
		return nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	return []byte("cred-" + string(session)), []byte("data"), nil
}
func (f *fakeVerifier) BeginLogin(_ domain.User, existing []domain.WebAuthnCredential) ([]byte, []byte, error) {
	if len(existing) == 0 {
		return nil, nil, domain.ErrWebAuthnKeyNotFound
	}
	options, session := f.next()
	return options, session, nil
}
func (f *fakeVerifier) FinishLogin(_ domain.User, existing []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	id, ok := strings.CutPrefix(answerOf(response), "ok:"+string(session)+":")
	if !ok {
		return nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	for _, key := range existing {
		if bytes.Equal(key.CredentialID, []byte(id)) {
			return key.CredentialID, []byte("data-used"), nil
		}
	}
	return nil, nil, domain.ErrInvalidWebAuthnAnswer
}

func (f *fakeVerifier) BeginPasskeyLogin() ([]byte, []byte, error) {
	options, session := f.next()
	return options, session, nil
}

// FinishPasskeyLogin accepts {"answer":"ok:<session>:<user UUID>:<credential>"}.
func (f *fakeVerifier) FinishPasskeyLogin(session, response []byte, lookup ports.PasskeyLookup) (domain.User, []byte, []byte, error) {
	rest, ok := strings.CutPrefix(answerOf(response), "ok:"+string(session)+":")
	handle, credential, found := strings.Cut(rest, ":")
	id, err := uuid.Parse(handle)
	if !ok || !found || err != nil {
		return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	user, existing, err := lookup(id[:])
	if errors.Is(err, domain.ErrWebAuthnKeyNotFound) {
		return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
	}
	if err != nil {
		return domain.User{}, nil, nil, err
	}
	for _, key := range existing {
		if bytes.Equal(key.CredentialID, []byte(credential)) {
			return user, key.CredentialID, []byte("data-passkey"), nil
		}
	}
	return domain.User{}, nil, nil, domain.ErrInvalidWebAuthnAnswer
}

func challengeOfOptions(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Options struct {
			PublicKey struct {
				Challenge string `json:"challenge"`
			} `json:"publicKey"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Options.PublicKey.Challenge == "" {
		t.Fatalf("options = %s (%v)", body, err)
	}
	return parsed.Options.PublicKey.Challenge
}

func TestWebAuthnEndToEnd(t *testing.T) {
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	verifier := &fakeVerifier{}
	keys := sqlite.NewSecondFactorRepository(database.Client())
	services.Auth.WithWebAuthn(verifier, keys)
	services.SecondFactors.WithWebAuthn(verifier, keys)
	router := NewRouter(zap.NewNop(), services, false, "")
	admin := signIn(t, router, "admin", "admin-password-1")

	var status struct {
		Keys              []struct{ ID, Name string } `json:"keys"`
		WebAuthnAvailable bool                        `json:"webauthnAvailable"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/second-factor", "", admin), &status)
	if !status.WebAuthnAvailable || status.Keys == nil || len(status.Keys) != 0 {
		t.Fatalf("status = %+v", status)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn", `{"password":"wrong"}`, admin); errorCode(t, response) != "INVALID_CREDENTIALS" {
		t.Fatalf("wrong password = %d %s", response.Code, response.Body)
	}
	begin := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn", `{"password":"admin-password-1"}`, admin)
	session := challengeOfOptions(t, begin.Body.Bytes())
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/confirm", `{"name":"YubiKey"}`, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("without response = %d", response.Code)
	}
	huge := fmt.Sprintf(`{"name":"YubiKey","response":{"answer":%q}}`, strings.Repeat("a", domain.MaxWebAuthnResponseBytes))
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/confirm", huge, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized response = %d", response.Code)
	}
	var created struct {
		Key           struct{ ID, Name string } `json:"key"`
		RecoveryCodes []string                  `json:"recoveryCodes"`
	}
	confirm := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/confirm", fmt.Sprintf(`{"name":"YubiKey","response":{"answer":"ok:%s"}}`, session), admin)
	if confirm.Code != http.StatusCreated {
		t.Fatalf("confirm = %d %s", confirm.Code, confirm.Body)
	}
	decodeBody(t, confirm, &created)
	if created.Key.Name != "YubiKey" || len(created.RecoveryCodes) != domain.RecoveryCodeCount || strings.Contains(confirm.Body.String(), "cred-") {
		t.Fatalf("created = %s", confirm.Body)
	}

	login := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin-password-1"}`, nil)
	if !strings.Contains(login.Body.String(), `"methods":["webauthn","recovery"]`) {
		t.Fatalf("password step = %s", login.Body)
	}
	challenge := cookieNamed(t, login, challengeCookieName)
	if response := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", `{"method":"webauthn","code":"123456"}`, challenge); response.Code != http.StatusBadRequest {
		t.Fatalf("webauthn with a code = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login/second-factor/webauthn", "", nil); errorCode(t, response) != "LOGIN_CHALLENGE_EXPIRED" {
		t.Fatalf("options without challenge = %d %s", response.Code, response.Body)
	}
	options := call(router, http.MethodPost, "/api/v1/auth/login/second-factor/webauthn", "", challenge)
	loginSession := challengeOfOptions(t, options.Body.Bytes())
	answer := fmt.Sprintf(`{"method":"webauthn","response":{"answer":"ok:%s:cred-%s"}}`, loginSession, session)
	completed := call(router, http.MethodPost, "/api/v1/auth/login/second-factor", answer, challenge)
	if completed.Code != http.StatusOK {
		t.Fatalf("key login = %d %s", completed.Code, completed.Body)
	}
	signedIn := cookieNamed(t, completed, sessionCookieName)

	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/not-a-uuid/delete", `{"password":"admin-password-1"}`, signedIn); response.Code != http.StatusNotFound {
		t.Fatalf("delete an invalid id = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/"+created.Key.ID+"/delete", `{"password":"admin-password-1"}`, signedIn); response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body)
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/auth/second-factor", "", signedIn), &status)
	if len(status.Keys) != 0 {
		t.Fatalf("keys after delete = %+v", status.Keys)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn/"+created.Key.ID+"/delete", `{"password":"admin-password-1"}`, signedIn); errorCode(t, response) != "WEBAUTHN_KEY_NOT_FOUND" {
		t.Fatalf("delete twice = %d %s", response.Code, response.Body)
	}
}

func TestWebAuthnUnconfigured(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/second-factor/webauthn", `{"password":"admin-password-1"}`, admin); errorCode(t, response) != "WEBAUTHN_UNAVAILABLE" {
		t.Fatalf("begin without WebAuthn = %d %s", response.Code, response.Body)
	}
	if !strings.Contains(call(router, http.MethodGet, "/api/v1/auth/second-factor", "", admin).Body.String(), `"webauthnAvailable":false`) {
		t.Fatal("WebAuthn reported available")
	}
}
