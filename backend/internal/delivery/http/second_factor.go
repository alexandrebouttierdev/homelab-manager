package httpdelivery

import (
	"errors"
	"net/http"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
)

const (
	// challengeCookieName holds the pending login between the password and
	// the second factor; it is only sent to the authentication routes.
	challengeCookieName = "homelab_login"
	challengeCookiePath = "/api/v1/auth"
	maxSecondFactorCode = 64
)

func challengeCookie(value string, expiresAt time.Time, secure bool) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if value == "" {
		maxAge = -1
	}
	return &http.Cookie{
		Name: challengeCookieName, Value: value, Path: challengeCookiePath, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expiresAt, MaxAge: maxAge,
	}
}

type secondFactorLoginRequest struct {
	Method string `json:"method"`
	Code   string `json:"code"`
}

func completeSecondFactorHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		var request secondFactorLoginRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		method := domain.SecondFactorMethod(request.Method)
		if !method.Valid() || request.Code == "" || len(request.Code) > maxSecondFactorCode {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "method must be totp or recovery, with a code.")
			return
		}
		cookie, err := r.Cookie(challengeCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
			return
		}
		result, err := auth.CompleteSecondFactor(r.Context(), cookie.Value, method, request.Code, clientIP(r), r.UserAgent())
		switch {
		case errors.Is(err, domain.ErrLoginChallengeExpired):
			http.SetCookie(w, challengeCookie("", time.Unix(0, 0), secure))
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
		case errors.Is(err, domain.ErrInvalidSecondFactor):
			writeError(w, r, http.StatusUnauthorized, "INVALID_SECOND_FACTOR", "The code is invalid.")
		case errors.Is(err, domain.ErrSecondFactorUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "SECOND_FACTOR_UNAVAILABLE", "TOTP is unavailable; use a recovery code.")
		case err != nil:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The authentication request could not be completed.")
		default:
			http.SetCookie(w, challengeCookie("", time.Unix(0, 0), secure))
			http.SetCookie(w, sessionCookie(sessionCookieName, result.Token, result.ExpiresAt, secure))
			writeJSON(w, http.StatusOK, map[string]any{"user": toSessionUserDTO(result.User)})
		}
	}
}

// requireSecondFactorEnrolled limits an administrator without a second
// factor, when the policy requires one, to its enrollment.
func requireSecondFactorEnrolled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := sessionUser(r); ok && user.MustEnrollSecondFactor {
			writeError(w, r, http.StatusForbidden, "SECOND_FACTOR_REQUIRED", "Enroll a second factor first.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type passwordConfirmation struct {
	Password string `json:"password"`
}

type totpConfirmation struct {
	Code string `json:"code"`
}

type secondFactorStatusDTO struct {
	Enabled                bool `json:"enabled"`
	Pending                bool `json:"pending"`
	RecoveryCodesRemaining int  `json:"recoveryCodesRemaining"`
	Available              bool `json:"available"`
}

func secondFactorStatusHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		status, err := factors.Status(r.Context(), user)
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, secondFactorStatusDTO{
			Enabled: status.Enabled, Pending: status.Pending,
			RecoveryCodesRemaining: status.RecoveryCodesRemaining, Available: status.Available,
		})
	}
}

func beginTOTPHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		password, ok := decodePasswordConfirmation(w, r)
		if !ok {
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		start, err := factors.BeginTOTP(r.Context(), user, password)
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"secret": start.Secret, "uri": start.URI,
			"qr": map[string]any{"size": start.QR.Size, "rows": start.QR.Rows},
		})
	}
}

func confirmTOTPHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request totpConfirmation
		if err := decodeJSON(r, &request); err != nil || request.Code == "" || len(request.Code) > maxSecondFactorCode {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "code is required.")
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		codes, err := factors.ConfirmTOTP(r.Context(), user, request.Code, middleware.GetReqID(r.Context()))
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
	}
}

func regenerateRecoveryCodesHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		password, ok := decodePasswordConfirmation(w, r)
		if !ok {
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		codes, err := factors.RegenerateRecoveryCodes(r.Context(), user, password, middleware.GetReqID(r.Context()))
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
	}
}

func disableSecondFactorHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		password, ok := decodePasswordConfirmation(w, r)
		if !ok {
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		if err := factors.Disable(r.Context(), user, password, middleware.GetReqID(r.Context())); err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"disabled": true})
	}
}

func decodePasswordConfirmation(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request passwordConfirmation
	if err := decodeJSON(r, &request); err != nil || request.Password == "" || len(request.Password) > domain.MaxPasswordBytes {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "password is required.")
		return "", false
	}
	return request.Password, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	user, ok := sessionUser(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
	}
	return user, ok
}

func writeSecondFactorError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The current password is invalid.")
	case errors.Is(err, domain.ErrSecondFactorUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "SECOND_FACTOR_UNAVAILABLE", "The secrets key is missing.")
	case errors.Is(err, domain.ErrSecondFactorAlreadyEnabled):
		writeError(w, r, http.StatusConflict, "SECOND_FACTOR_ALREADY_ENABLED", "A second factor is already enabled.")
	case errors.Is(err, domain.ErrSecondFactorNotEnabled):
		writeError(w, r, http.StatusConflict, "SECOND_FACTOR_NOT_ENABLED", "No second factor is enabled.")
	case errors.Is(err, domain.ErrNoPendingEnrollment):
		writeError(w, r, http.StatusConflict, "NO_PENDING_ENROLLMENT", "Start the enrollment first.")
	case errors.Is(err, domain.ErrInvalidSecondFactor):
		writeError(w, r, http.StatusBadRequest, "INVALID_SECOND_FACTOR", "The code is invalid.")
	default:
		writeError(w, r, http.StatusInternalServerError, "SECOND_FACTOR_UPDATE_FAILED", "Could not update the second factor.")
	}
}
