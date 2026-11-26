package httpdelivery

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

const (
	// challengeCookieName holds the pending login between the password and
	// the second factor; it is only sent to the authentication routes.
	challengeCookieName = "monoops_login"
	challengeCookiePath = "/api/v1/auth"
	maxSecondFactorCode = 64
	// maxWebAuthnBody bounds a request that carries an answer of the
	// browser, with room for the other fields.
	maxWebAuthnBody = domain.MaxWebAuthnResponseBytes + 4<<10
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
	// Response is the answer of the browser for the method "webauthn".
	Response json.RawMessage `json:"response"`
}

// code returns the code of the request, or the answer of the browser for a
// key; ok is false when the field of the method is missing or too long.
func (r secondFactorLoginRequest) code(method domain.SecondFactorMethod) (string, bool) {
	if method == domain.SecondFactorWebAuthn {
		return string(r.Response), r.Code == "" && len(r.Response) > 0 && len(r.Response) <= domain.MaxWebAuthnResponseBytes
	}
	return r.Code, r.Code != "" && len(r.Code) <= maxSecondFactorCode && r.Response == nil
}

// beginWebAuthnLoginHandler returns the options of a key login for the
// pending login of the challenge cookie.
func beginWebAuthnLoginHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		cookie, err := r.Cookie(challengeCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
			return
		}
		options, err := auth.BeginWebAuthnLogin(r.Context(), cookie.Value)
		switch {
		case errors.Is(err, domain.ErrLoginChallengeExpired):
			http.SetCookie(w, challengeCookie("", time.Unix(0, 0), secure))
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
		case errors.Is(err, domain.ErrWebAuthnUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "WEBAUTHN_UNAVAILABLE", "Security keys are not configured.")
		case errors.Is(err, domain.ErrWebAuthnKeyNotFound):
			writeError(w, r, http.StatusConflict, "WEBAUTHN_KEY_NOT_FOUND", "This account has no security key.")
		case err != nil:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The authentication request could not be completed.")
		default:
			writeOptions(w, options)
		}
	}
}

// writeOptions sends the options of a ceremony, already in JSON.
func writeOptions(w http.ResponseWriter, options []byte) {
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"options": options})
}

func completeSecondFactorHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWebAuthnBody)
		var request secondFactorLoginRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		method := domain.SecondFactorMethod(request.Method)
		code, ok := request.code(method)
		if !method.Valid() || !ok {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "method must be totp or recovery with a code, or webauthn with a response.")
			return
		}
		cookie, err := r.Cookie(challengeCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
			return
		}
		result, err := auth.CompleteSecondFactor(r.Context(), cookie.Value, method, code, clientIP(r), r.UserAgent())
		switch {
		case errors.Is(err, domain.ErrLoginChallengeExpired):
			http.SetCookie(w, challengeCookie("", time.Unix(0, 0), secure))
			writeError(w, r, http.StatusUnauthorized, "LOGIN_CHALLENGE_EXPIRED", "Sign in with the password again.")
		case errors.Is(err, domain.ErrInvalidSecondFactor):
			writeError(w, r, http.StatusUnauthorized, "INVALID_SECOND_FACTOR", "The code is invalid.")
		case errors.Is(err, domain.ErrSecondFactorUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "SECOND_FACTOR_UNAVAILABLE", "TOTP is unavailable; use a recovery code.")
		case errors.Is(err, domain.ErrWebAuthnUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "WEBAUTHN_UNAVAILABLE", "Security keys are not configured; use a recovery code.")
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
	Enabled                bool             `json:"enabled"`
	Pending                bool             `json:"pending"`
	RecoveryCodesRemaining int              `json:"recoveryCodesRemaining"`
	Available              bool             `json:"available"`
	Keys                   []webAuthnKeyDTO `json:"keys"`
	WebAuthnAvailable      bool             `json:"webauthnAvailable"`
}

type webAuthnKeyDTO struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func toWebAuthnKeyDTO(key domain.WebAuthnCredential) webAuthnKeyDTO {
	return webAuthnKeyDTO{ID: key.ID, Name: key.Name, CreatedAt: key.CreatedAt, LastUsedAt: key.LastUsedAt}
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
		keys := make([]webAuthnKeyDTO, 0, len(status.Keys))
		for _, key := range status.Keys {
			keys = append(keys, toWebAuthnKeyDTO(key))
		}
		writeJSON(w, http.StatusOK, secondFactorStatusDTO{
			Enabled: status.Enabled, Pending: status.Pending,
			RecoveryCodesRemaining: status.RecoveryCodesRemaining, Available: status.Available,
			Keys: keys, WebAuthnAvailable: status.WebAuthnAvailable,
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

func beginWebAuthnHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		password, ok := decodePasswordConfirmation(w, r)
		if !ok {
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		options, err := factors.BeginWebAuthn(r.Context(), user, password)
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeOptions(w, options)
	}
}

type webAuthnConfirmation struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

func confirmWebAuthnHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWebAuthnBody)
		var request webAuthnConfirmation
		if err := decodeJSON(r, &request); err != nil || len(request.Response) == 0 || len(request.Response) > domain.MaxWebAuthnResponseBytes {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "name and response are required.")
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		key, codes, err := factors.FinishWebAuthn(r.Context(), user, request.Name, request.Response, middleware.GetReqID(r.Context()))
		if err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		if codes == nil {
			codes = []string{}
		}
		writeJSON(w, http.StatusCreated, map[string]any{"key": toWebAuthnKeyDTO(key), "recoveryCodes": codes})
	}
}

func deleteWebAuthnHandler(factors *usecase.SecondFactorUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "keyID"))
		if err != nil {
			writeError(w, r, http.StatusNotFound, "WEBAUTHN_KEY_NOT_FOUND", "The security key does not exist.")
			return
		}
		password, ok := decodePasswordConfirmation(w, r)
		if !ok {
			return
		}
		user, ok := requireUser(w, r)
		if !ok {
			return
		}
		if err := factors.DeleteWebAuthn(r.Context(), user, password, id, middleware.GetReqID(r.Context())); err != nil {
			writeSecondFactorError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
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
	case errors.Is(err, domain.ErrWebAuthnUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "WEBAUTHN_UNAVAILABLE", "Security keys are not configured.")
	case errors.Is(err, domain.ErrInvalidWebAuthnName):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "name must have 1 to 64 characters.")
	case errors.Is(err, domain.ErrTooManyWebAuthnKeys):
		writeError(w, r, http.StatusConflict, "TOO_MANY_WEBAUTHN_KEYS", "This account already has the maximum number of security keys.")
	case errors.Is(err, domain.ErrWebAuthnKeyNotFound):
		writeError(w, r, http.StatusNotFound, "WEBAUTHN_KEY_NOT_FOUND", "The security key does not exist.")
	case errors.Is(err, domain.ErrWebAuthnKeyExists):
		writeError(w, r, http.StatusConflict, "WEBAUTHN_KEY_EXISTS", "This security key is already registered.")
	case errors.Is(err, domain.ErrNoWebAuthnCeremony):
		writeError(w, r, http.StatusConflict, "NO_PENDING_ENROLLMENT", "Start the registration of the key again.")
	case errors.Is(err, domain.ErrInvalidWebAuthnAnswer):
		writeError(w, r, http.StatusBadRequest, "INVALID_WEBAUTHN_RESPONSE", "The answer of the security key is invalid.")
	default:
		writeError(w, r, http.StatusInternalServerError, "SECOND_FACTOR_UPDATE_FAILED", "Could not update the second factor.")
	}
}
