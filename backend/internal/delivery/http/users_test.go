package httpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	argon2adapter "homelab-manager/backend/internal/adapters/argon2"
	"homelab-manager/backend/internal/adapters/qrcode"
	"homelab-manager/backend/internal/adapters/secretbox"
	"homelab-manager/backend/internal/adapters/sqlite"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func withSessionUser(r *http.Request, user domain.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), sessionUserKey{}, user))
}

var noContent = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

func TestRequirePermissionAppliesTheRolePolicy(t *testing.T) {
	cases := []struct {
		role domain.Role
		want int
	}{
		{domain.RoleViewer, http.StatusForbidden},
		{domain.RoleOperator, http.StatusForbidden},
		{domain.RoleAdmin, http.StatusNoContent},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		request := withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), domain.User{Username: "u", Role: c.role})
		requirePermission(nil, domain.PermManageUsers)(noContent).ServeHTTP(recorder, request)
		if recorder.Code != c.want {
			t.Errorf("role %s got %d, want %d", c.role, recorder.Code, c.want)
		}
	}
	recorder := httptest.NewRecorder()
	requirePermission(nil, domain.PermManageUsers)(noContent).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("without a session got %d, want 401", recorder.Code)
	}
}

func TestRequirePasswordChangedBlocksUntilChanged(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil), domain.User{Role: domain.RoleAdmin, MustChangePassword: true})
	requirePasswordChanged(noContent).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("got %d %s, want 403 PASSWORD_CHANGE_REQUIRED", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	request = withSessionUser(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil), domain.User{Role: domain.RoleViewer})
	requirePasswordChanged(noContent).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", recorder.Code)
	}
}

func TestCreateUserValidatesTheBody(t *testing.T) {
	// A nil usecase proves validation happens before any change.
	handler := createUserHandler(nil)
	for _, body := range []string{
		``,
		`{}`,
		`{"username":"Alice","displayName":"","role":"viewer","password":"initial-password-1"}`,
		`{"username":"alice","displayName":"","role":"root","password":"initial-password-1"}`,
		`{"username":"alice","displayName":"","role":"viewer","password":"short"}`,
		`{"username":"alice","displayName":"","role":"viewer","password":"initial-password-1","extra":1}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q returned %d, want 400", body, recorder.Code)
		}
	}
}

func TestUpdateUserValidatesPathAndBody(t *testing.T) {
	router := chi.NewRouter()
	router.Patch("/api/v1/users/{userID}", updateUserHandler(nil))
	const id = "9369be23-5ad3-467d-b954-470b622a1442"
	cases := []struct{ path, body string }{
		{"/api/v1/users/not-a-uuid", `{"role":"viewer"}`},
		{"/api/v1/users/9369BE23-5AD3-467D-B954-470B622A1442", `{"role":"viewer"}`},
		{"/api/v1/users/" + id, `{}`},
		{"/api/v1/users/" + id, `{"role":"root"}`},
		{"/api/v1/users/" + id, `{"displayName":"   "}`},
		{"/api/v1/users/" + id, `{"disabled":"yes"}`},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, c.path, strings.NewReader(c.body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s %s returned %d, want 400", c.path, c.body, recorder.Code)
		}
	}
}

func TestAccountAdministrationEndToEnd(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")

	const aliceBody = `{"username":"alice","displayName":"Alice","role":"viewer","password":"initial-password-1"}`
	created := call(router, http.MethodPost, "/api/v1/users", aliceBody, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body)
	}
	var createdBody struct {
		User userDTO `json:"user"`
	}
	decodeBody(t, created, &createdBody)
	if !createdBody.User.MustChangePassword || createdBody.User.Role != "viewer" || createdBody.User.LastLoginAt != nil {
		t.Fatalf("created account = %+v", createdBody.User)
	}
	aliceID := createdBody.User.ID.String()
	if response := call(router, http.MethodPost, "/api/v1/users", aliceBody, admin); response.Code != http.StatusConflict || errorCode(t, response) != "USERNAME_TAKEN" {
		t.Fatalf("duplicate = %d %s, want 409 USERNAME_TAKEN", response.Code, response.Body)
	}

	alice := signIn(t, router, "alice", "initial-password-1")
	if response := call(router, http.MethodGet, "/api/v1/settings", "", alice); errorCode(t, response) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("before the change = %d %s, want PASSWORD_CHANGE_REQUIRED", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"alice-own-password"}`, alice); response.Code != http.StatusOK {
		t.Fatalf("password change = %d %s", response.Code, response.Body)
	}
	alice = signIn(t, router, "alice", "alice-own-password")
	if response := call(router, http.MethodGet, "/api/v1/users", "", alice); response.Code != http.StatusForbidden || errorCode(t, response) != "FORBIDDEN" {
		t.Fatalf("viewer listing accounts = %d %s, want 403 FORBIDDEN", response.Code, response.Body)
	}
	if response := call(router, http.MethodPut, "/api/v1/settings/retention", `{"metricsRetentionDays":3}`, alice); response.Code != http.StatusForbidden {
		t.Fatalf("viewer changing the retention = %d, want 403", response.Code)
	}

	var list struct {
		Users []userDTO `json:"users"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/users", "", admin), &list)
	if len(list.Users) != 2 || list.Users[0].Username != "admin" {
		t.Fatalf("listed accounts = %+v", list.Users)
	}
	adminID := list.Users[0].ID.String()
	if response := call(router, http.MethodPatch, "/api/v1/users/"+adminID, `{"role":"viewer"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "LAST_ADMIN" {
		t.Fatalf("demoting the last admin = %d %s, want 409 LAST_ADMIN", response.Code, response.Body)
	}

	if response := call(router, http.MethodPatch, "/api/v1/users/"+aliceID, `{"disabled":true}`, admin); response.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/session", "", alice); response.Code != http.StatusUnauthorized {
		t.Fatalf("session of a disabled account = %d, want 401", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"alice-own-password"}`, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("login of a disabled account = %d, want 401", response.Code)
	}

	if response := call(router, http.MethodDelete, "/api/v1/users/"+aliceID, "", admin); response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodDelete, "/api/v1/users/"+aliceID, "", admin); response.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", response.Code)
	}
}

// newTestRouter builds the HTTP stack on a fresh SQLite file with an
// "admin" account. A nil controller leaves the power actions disabled.
func newTestRouter(t *testing.T, controller ports.PowerController) (http.Handler, *sqlite.Database) {
	t.Helper()
	services, database := newTestServices(t, controller)
	return NewRouter(zap.NewNop(), services, false, ""), database
}

// newTestServices builds the usecases of newTestRouter, for a test that
// adds its own.
func newTestServices(t *testing.T, controller ports.PowerController) (Services, *sqlite.Database) {
	t.Helper()
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	hasher := argon2adapter.New()
	hash, err := hasher.Hash("admin-password-1")
	if err != nil {
		t.Fatal(err)
	}
	authRepository := sqlite.NewAuthRepository(database.Client())
	if _, err := authRepository.EnsureAdmin(ctx, "admin", hash); err != nil {
		t.Fatal(err)
	}
	auditRepository := sqlite.NewAuditRepository(database.Client())
	securityRepository := sqlite.NewSecurityRepository(database.Client())
	factorRepository := sqlite.NewSecondFactorRepository(database.Client())
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	security := usecase.NewSecurityUsecase(securityRepository).WithSecondFactors(factorRepository)
	if err := security.Load(ctx); err != nil {
		t.Fatal(err)
	}
	services := Services{
		Health: usecase.NewHealthUsecase(time.Now()),
		Auth: usecase.NewAuthUsecase(authRepository, authRepository, hasher).
			WithSecurity(securityRepository, auditRepository, security).WithSecondFactor(factorRepository, box),
		Accounts:      usecase.NewAccountUsecase(sqlite.NewAccountRepository(database.Client()), authRepository, hasher).WithAudit(auditRepository),
		Power:         usecase.NewPowerUsecase(controller, auditRepository, func() (string, error) { return "homelab", nil }, controller != nil),
		Security:      security,
		Audit:         usecase.NewAuditUsecase(auditRepository, auditRepository),
		SecondFactors: usecase.NewSecondFactorUsecase(authRepository, factorRepository, box, qrcode.New(), hasher),
		Tokens:        usecase.NewAPITokenUsecase(sqlite.NewAPITokenRepository(database.Client()), authRepository, zap.NewNop()),
	}
	return services, database
}

// testClientAddress is the address of every request sent by call.
const testClientAddress = "198.51.100.7"

// call sends a request from testClientAddress.
func call(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return callFrom(handler, testClientAddress, method, path, body, cookie)
}

func callFrom(handler http.Handler, address, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = address + ":4000"
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// resetLoginRateLimit empties the per-address login counter shared by the
// tests of the package, so that each end-to-end test starts afresh.
func resetLoginRateLimit(t *testing.T) {
	t.Helper()
	loginAttempts.Lock()
	loginAttempts.byIP = make(map[string]loginWindow)
	loginAttempts.Unlock()
}

func signIn(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	response := call(handler, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("login %s = %d %s", username, response.Code, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("login %s set no session cookie", username)
	return nil
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body errorResponse
	decodeBody(t, recorder, &body)
	return body.Error.Code
}

func TestAdministratorResetsAPassword(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	created := call(router, http.MethodPost, "/api/v1/users", `{"username":"alice","displayName":"Alice","role":"viewer","password":"initial-password-1"}`, admin)
	var body struct {
		User userDTO `json:"user"`
	}
	decodeBody(t, created, &body)
	aliceID := body.User.ID.String()
	alice := signIn(t, router, "alice", "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"alice-own-password"}`, alice); response.Code != http.StatusOK {
		t.Fatalf("alice password = %d", response.Code)
	}
	alice = signIn(t, router, "alice", "alice-own-password")

	if response := call(router, http.MethodPost, "/api/v1/users/"+aliceID+"/password", `{"password":"short"}`, admin); response.Code != http.StatusBadRequest {
		t.Fatalf("short password = %d, want 400", response.Code)
	}
	reset := call(router, http.MethodPost, "/api/v1/users/"+aliceID+"/password", `{"password":"reset-password-1"}`, admin)
	decodeBody(t, reset, &body)
	if reset.Code != http.StatusOK || !body.User.MustChangePassword {
		t.Fatalf("reset = %d %s", reset.Code, reset.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/auth/session", "", alice); response.Code != http.StatusUnauthorized {
		t.Fatalf("alice session after a reset = %d, want 401", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"alice-own-password"}`, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("old password after a reset = %d, want 401", response.Code)
	}
	alice = signIn(t, router, "alice", "reset-password-1")
	if response := call(router, http.MethodGet, "/api/v1/settings", "", alice); errorCode(t, response) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("after a reset = %d %s, want PASSWORD_CHANGE_REQUIRED", response.Code, response.Body)
	}

	var list struct {
		Users []userDTO `json:"users"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/users", "", admin), &list)
	adminID := list.Users[0].ID.String()
	if response := call(router, http.MethodPost, "/api/v1/users/"+adminID+"/password", `{"password":"reset-password-1"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "OWN_PASSWORD_RESET" {
		t.Fatalf("own reset = %d %s, want 409 OWN_PASSWORD_RESET", response.Code, response.Body)
	}
}
