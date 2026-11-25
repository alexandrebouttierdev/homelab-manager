package httpdelivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

// The cookie of a login with a passkey alone (decision D-14) holds the
// token of its ceremony, sent back with the answer of the browser.
const (
	passkeyCookieName = "homelab_passkey"
	passkeyCookiePath = "/api/v1/auth/login/passkey"
)

func passkeyCookie(value string, expiresAt time.Time, secure bool) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if value == "" {
		maxAge = -1
	}
	return &http.Cookie{
		Name: passkeyCookieName, Value: value, Path: passkeyCookiePath, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expiresAt, MaxAge: maxAge,
	}
}

// passkeyLoginStatusHandler tells the login page whether to offer the
// login with a passkey alone.
func passkeyLoginStatusHandler(auth *usecase.AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"available": auth != nil && auth.PasskeyLoginAvailable()})
	}
}

func beginPasskeyLoginHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		token, options, expiresAt, err := auth.BeginPasskeyLogin()
		if err != nil {
			writePasskeyError(w, r, err)
			return
		}
		http.SetCookie(w, passkeyCookie(token, expiresAt, secure))
		writeOptions(w, options)
	}
}

type passkeyLoginRequest struct {
	Response json.RawMessage `json:"response"`
}

func completePasskeyLoginHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWebAuthnBody)
		var request passkeyLoginRequest
		if err := decodeJSON(r, &request); err != nil || len(request.Response) == 0 || len(request.Response) > domain.MaxWebAuthnResponseBytes {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "response must hold the answer of the browser.")
			return
		}
		cookie, err := r.Cookie(passkeyCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "PASSKEY_CEREMONY_EXPIRED", "Start the passkey login again.")
			return
		}
		// The ceremony answers once: its cookie goes whatever the outcome.
		http.SetCookie(w, passkeyCookie("", time.Unix(0, 0), secure))
		result, err := auth.CompletePasskeyLogin(r.Context(), cookie.Value, request.Response, clientIP(r), r.UserAgent())
		if err != nil {
			writePasskeyError(w, r, err)
			return
		}
		http.SetCookie(w, sessionCookie(sessionCookieName, result.Token, result.ExpiresAt, secure))
		writeJSON(w, http.StatusOK, map[string]any{"user": toSessionUserDTO(result.User)})
	}
}

func writePasskeyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrWebAuthnUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "WEBAUTHN_UNAVAILABLE", "Security keys are not configured.")
	case errors.Is(err, domain.ErrPasskeyLoginDisabled):
		writeError(w, r, http.StatusForbidden, "PASSKEY_LOGIN_DISABLED", "The security policy does not allow the login with a passkey alone.")
	case errors.Is(err, usecase.ErrTooManyLoginChallenges):
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many pending logins.")
	case errors.Is(err, domain.ErrLoginChallengeExpired):
		writeError(w, r, http.StatusUnauthorized, "PASSKEY_CEREMONY_EXPIRED", "Start the passkey login again.")
	case errors.Is(err, domain.ErrInvalidPasskey):
		writeError(w, r, http.StatusUnauthorized, "INVALID_PASSKEY", "The passkey cannot sign in.")
	case errors.Is(err, domain.ErrPasskeyPasswordChange):
		writeError(w, r, http.StatusConflict, "PASSKEY_PASSWORD_CHANGE", "Sign in with the password to change it.")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The authentication request could not be completed.")
	}
}
