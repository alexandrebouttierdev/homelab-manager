package httpdelivery

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"monoops/backend/internal/adapters/sqlite"
	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"go.uber.org/zap"
)

type httpFakeSystem struct {
	current domain.SystemPreferences
	refuse  bool
	locked  bool
}

func (f *httpFakeSystem) LocaleEditable() bool { return !f.locked }

func (f *httpFakeSystem) Preferences(context.Context) (domain.SystemPreferences, error) {
	return f.current, nil
}

func (f *httpFakeSystem) Timezones(context.Context) ([]string, error) {
	return []string{"Europe/Paris", "UTC"}, nil
}

func (f *httpFakeSystem) SetHostname(_ context.Context, value string) error {
	if f.refuse {
		return domain.ErrSystemChangeRefused
	}
	f.current.Hostname = value
	return nil
}

func (f *httpFakeSystem) SetTimezone(_ context.Context, value string) error {
	if value == "Mars/Olympus" {
		return domain.ErrSystemValueRejected
	}
	f.current.Timezone = value
	return nil
}

func (f *httpFakeSystem) SetLocale(_ context.Context, value string) error {
	f.current.Locale = value
	return nil
}

func newSystemRouter(t *testing.T, enabled bool) (http.Handler, *httpFakeSystem, *http.Cookie) {
	t.Helper()
	resetLoginRateLimit(t)
	services, database := newTestServices(t, nil)
	fake := &httpFakeSystem{current: domain.SystemPreferences{Hostname: "monoops", Timezone: "Europe/Paris", Locale: "fr_FR.UTF-8"}}
	services.SystemPreferences = usecase.NewSystemPreferencesUsecase(fake, sqlite.NewAuditRepository(database.Client()), enabled, zap.NewNop())
	router := NewRouter(zap.NewNop(), services, false, "")
	return router, fake, signIn(t, router, "admin", "admin-password-1")
}

func TestSystemPreferencesRoutes(t *testing.T) {
	router, fake, admin := newSystemRouter(t, true)
	operator := signInWithRole(t, router, admin, "olivia", "operator")
	read := call(router, http.MethodGet, "/api/v1/system/preferences", "", operator)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"hostname":"monoops"`) ||
		!strings.Contains(read.Body.String(), `"timezones":["Europe/Paris","UTC"]`) || !strings.Contains(read.Body.String(), `"enabled":true`) {
		t.Fatalf("read = %d %s", read.Code, read.Body)
	}
	body := `{"hostname":"nas","timezone":"UTC","locale":"fr_FR.UTF-8"}`
	if response := call(router, http.MethodPut, "/api/v1/system/preferences", body, operator); response.Code != http.StatusForbidden {
		t.Fatalf("operator change = %d", response.Code)
	}
	response := call(router, http.MethodPut, "/api/v1/system/preferences", body, admin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"changed":["hostname","timezone"]`) || fake.current.Hostname != "nas" {
		t.Fatalf("change = %d %s", response.Code, response.Body)
	}
	for request, code := range map[string]string{
		`{"hostname":"-nas","timezone":"UTC","locale":"fr_FR.UTF-8"}`:             "VALIDATION_ERROR",
		`{"hostname":"nas","timezone":"Mars/Olympus","locale":"fr_FR.UTF-8"}`:     "SYSTEM_VALUE_REJECTED",
		`{"hostname":"nas","timezone":"UTC","locale":"fr_FR.UTF-8","extra":true}`: "VALIDATION_ERROR",
	} {
		if response := call(router, http.MethodPut, "/api/v1/system/preferences", request, admin); !strings.Contains(response.Body.String(), code) {
			t.Fatalf("%s = %d %s", request, response.Code, response.Body)
		}
	}
	fake.locked = true
	locked := call(router, http.MethodPut, "/api/v1/system/preferences", `{"hostname":"nas","timezone":"UTC","locale":"C.UTF-8"}`, admin)
	if locked.Code != http.StatusConflict || !strings.Contains(locked.Body.String(), "SYSTEM_SETTING_READ_ONLY") {
		t.Fatalf("locked locale = %d %s", locked.Code, locked.Body)
	}
	if read := call(router, http.MethodGet, "/api/v1/system/preferences", "", admin); !strings.Contains(read.Body.String(), `"localeEditable":false`) {
		t.Fatalf("read = %s", read.Body)
	}
	fake.locked = false
	fake.refuse = true
	refused := call(router, http.MethodPut, "/api/v1/system/preferences", `{"hostname":"other","timezone":"UTC","locale":"fr_FR.UTF-8"}`, admin)
	if refused.Code != http.StatusServiceUnavailable || !strings.Contains(refused.Body.String(), "SYSTEM_CHANGE_REFUSED") {
		t.Fatalf("refused = %d %s", refused.Code, refused.Body)
	}
	audit := call(router, http.MethodGet, "/api/v1/audit?targetType=system", "", admin)
	if !strings.Contains(audit.Body.String(), `"targetId":"hostname","action":"set"`) {
		t.Fatalf("audit = %s", audit.Body)
	}
}

func TestSystemPreferencesDisabled(t *testing.T) {
	router, fake, admin := newSystemRouter(t, false)
	response := call(router, http.MethodPut, "/api/v1/system/preferences", `{"hostname":"nas","timezone":"UTC","locale":"fr_FR.UTF-8"}`, admin)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "SYSTEM_PREFERENCES_DISABLED") || fake.current.Hostname != "monoops" {
		t.Fatalf("disabled = %d %s", response.Code, response.Body)
	}
}
