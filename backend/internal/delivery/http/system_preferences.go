package httpdelivery

import (
	"errors"
	"net/http"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
)

type systemPreferencesDTO struct {
	Hostname string `json:"hostname"`
	Timezone string `json:"timezone"`
	Locale   string `json:"locale"`
}

func requireSystemPreferences(preferences *usecase.SystemPreferencesUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if preferences == nil {
				writeError(w, r, http.StatusServiceUnavailable, "SYSTEM_PREFERENCES_UNAVAILABLE", "System preferences are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeSystemPreferences(w http.ResponseWriter, r *http.Request, preferences *usecase.SystemPreferencesUsecase, status int, changed []domain.SystemSetting) {
	current, zones, err := preferences.Preferences(r.Context())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "SYSTEM_PREFERENCES_UNAVAILABLE", "The system preferences could not be read.")
		return
	}
	if zones == nil {
		zones = []string{}
	}
	body := map[string]any{
		"enabled":     preferences.Enabled(),
		"preferences": systemPreferencesDTO{Hostname: current.Hostname, Timezone: current.Timezone, Locale: current.Locale},
		"timezones":   zones,
	}
	if changed != nil {
		names := make([]string, 0, len(changed))
		for _, setting := range changed {
			names = append(names, string(setting))
		}
		body["changed"] = names
	}
	writeJSON(w, status, body)
}

func systemPreferencesHandler(preferences *usecase.SystemPreferencesUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeSystemPreferences(w, r, preferences, http.StatusOK, nil)
	}
}

func updateSystemPreferencesHandler(preferences *usecase.SystemPreferencesUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request systemPreferencesDTO
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "hostname, timezone and locale are required.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		changed, err := preferences.Update(r.Context(), actor, domain.SystemPreferencesChange{
			Hostname: request.Hostname, Timezone: request.Timezone, Locale: request.Locale,
		}, middleware.GetReqID(r.Context()))
		if err == nil {
			if changed == nil {
				changed = []domain.SystemSetting{}
			}
			writeSystemPreferences(w, r, preferences, http.StatusOK, changed)
			return
		}
		var changeErr *domain.SystemChangeError
		setting := ""
		if errors.As(err, &changeErr) {
			setting = " (" + string(changeErr.Setting) + ")"
		}
		switch {
		case errors.Is(err, domain.ErrInvalidHostname), errors.Is(err, domain.ErrInvalidTimezone), errors.Is(err, domain.ErrInvalidLocale):
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		case errors.Is(err, domain.ErrSystemPreferencesDisabled):
			writeError(w, r, http.StatusConflict, "SYSTEM_PREFERENCES_DISABLED", "System preferences are disabled in the service configuration.")
		case errors.Is(err, domain.ErrActionInProgress):
			writeError(w, r, http.StatusConflict, "ACTION_IN_PROGRESS", "A change of the system preferences is already in progress.")
		case errors.Is(err, domain.ErrSystemValueRejected):
			writeError(w, r, http.StatusBadRequest, "SYSTEM_VALUE_REJECTED", "The system refused the value"+setting+".")
		case errors.Is(err, domain.ErrSystemChangeRefused):
			writeError(w, r, http.StatusServiceUnavailable, "SYSTEM_CHANGE_REFUSED", "polkit refused the change"+setting+".")
		case errors.Is(err, domain.ErrSystemSettingsUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "SYSTEM_PREFERENCES_UNAVAILABLE", "The system services could not be reached"+setting+".")
		default:
			writeError(w, r, http.StatusInternalServerError, "SYSTEM_CHANGE_FAILED", "The change could not be recorded"+setting+".")
		}
	}
}
