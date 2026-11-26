package httpdelivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"monoops/backend/ent/auditlog"
	"monoops/backend/internal/domain"

	"github.com/go-chi/chi/v5"
)

// recordingPower stands for logind: tests never reboot the machine.
type recordingPower struct {
	requested []domain.PowerAction
	err       error
}

func (r *recordingPower) Request(_ context.Context, action domain.PowerAction) error {
	r.requested = append(r.requested, action)
	return r.err
}

func TestPowerActionValidatesBeforeAnyChange(t *testing.T) {
	// A nil usecase proves validation happens before any change.
	router := chi.NewRouter()
	router.Post("/api/v1/machine/power/{action}", powerActionHandler(nil))
	cases := []struct{ action, body string }{
		{"suspend", `{"confirmHostname":"monoops"}`},
		{"Reboot", `{"confirmHostname":"monoops"}`},
		{"reboot", `{}`},
		{"reboot", `{"confirmHostname":""}`},
		{"reboot", `{"confirmHostname":"` + strings.Repeat("a", 254) + `"}`},
		{"reboot", `{"confirmHostname":"monoops","force":true}`},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/machine/power/"+c.action, strings.NewReader(c.body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("POST %s %s returned %d, want 400", c.action, c.body, recorder.Code)
		}
	}
}

func TestPowerActionEndToEnd(t *testing.T) {
	controller := &recordingPower{}
	router, database := newTestRouter(t, controller)
	admin := signIn(t, router, "admin", "admin-password-1")

	var settings struct {
		Enabled bool     `json:"enabled"`
		Actions []string `json:"actions"`
	}
	decodeBody(t, call(router, http.MethodGet, "/api/v1/machine/power", "", admin), &settings)
	if !settings.Enabled || len(settings.Actions) != 2 {
		t.Fatalf("power settings = %+v", settings)
	}

	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"other"}`, admin); response.Code != http.StatusBadRequest || errorCode(t, response) != "HOSTNAME_MISMATCH" {
		t.Fatalf("wrong host = %d %s, want 400 HOSTNAME_MISMATCH", response.Code, response.Body)
	}
	controller.err = domain.ErrPowerActionRefused
	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"monoops"}`, admin); response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "POWER_ACTION_REFUSED" {
		t.Fatalf("polkit refusal = %d %s, want 503 POWER_ACTION_REFUSED", response.Code, response.Body)
	}
	controller.err = nil
	if response := call(router, http.MethodPost, "/api/v1/machine/power/reboot", `{"confirmHostname":"monoops"}`, admin); response.Code != http.StatusAccepted {
		t.Fatalf("reboot = %d %s, want 202", response.Code, response.Body)
	}
	if response := call(router, http.MethodPost, "/api/v1/machine/power/poweroff", `{"confirmHostname":"monoops"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "ACTION_IN_PROGRESS" {
		t.Fatalf("second request = %d %s, want 409 ACTION_IN_PROGRESS", response.Code, response.Body)
	}
	if len(controller.requested) != 2 {
		t.Fatalf("requested = %v, want the refused then the accepted reboot", controller.requested)
	}
	// refused: requested + failure; accepted: requested.
	count, err := database.Client().AuditLog.Query().Where(auditlog.TargetType("host"), auditlog.Action("reboot")).Count(context.Background())
	if err != nil || count != 3 {
		t.Fatalf("audit rows = (%d, %v), want 3", count, err)
	}
}

func TestPowerActionsDisabledByConfiguration(t *testing.T) {
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPost, "/api/v1/machine/power/poweroff", `{"confirmHostname":"monoops"}`, admin); response.Code != http.StatusConflict || errorCode(t, response) != "POWER_ACTIONS_DISABLED" {
		t.Fatalf("disabled = %d %s, want 409 POWER_ACTIONS_DISABLED", response.Code, response.Body)
	}
}
