package httpdelivery

import (
	"errors"
	"net/http"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var powerActions = []domain.PowerAction{domain.PowerReboot, domain.PowerPowerOff}

type powerRequest struct {
	ConfirmHostname string `json:"confirmHostname"`
}

func powerSettingsHandler(power *usecase.PowerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": power.Enabled(), "actions": powerActions})
	}
}

func powerActionHandler(power *usecase.PowerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := domain.PowerAction(chi.URLParam(r, "action"))
		if !action.Valid() {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "action must be reboot or poweroff.")
			return
		}
		var request powerRequest
		if err := decodeJSON(r, &request); err != nil || request.ConfirmHostname == "" || len(request.ConfirmHostname) > domain.MaxHostnameBytes {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "confirmHostname is required.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		err := power.Request(r.Context(), actor, action, request.ConfirmHostname, middleware.GetReqID(r.Context()))
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]any{"requested": true, "action": action})
		case errors.Is(err, domain.ErrHostnameMismatch):
			writeError(w, r, http.StatusBadRequest, "HOSTNAME_MISMATCH", "The confirmation does not match the host name.")
		case errors.Is(err, domain.ErrPowerActionsDisabled):
			writeError(w, r, http.StatusConflict, "POWER_ACTIONS_DISABLED", "Power actions are disabled in the service configuration.")
		case errors.Is(err, domain.ErrActionInProgress):
			writeError(w, r, http.StatusConflict, "ACTION_IN_PROGRESS", "A power action is already in progress.")
		case errors.Is(err, domain.ErrPowerActionRefused):
			writeError(w, r, http.StatusServiceUnavailable, "POWER_ACTION_REFUSED", "The system refused the power action.")
		default:
			writeError(w, r, http.StatusInternalServerError, "POWER_ACTION_FAILED", "The power action could not be requested.")
		}
	}
}
