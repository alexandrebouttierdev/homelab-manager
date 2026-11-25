package httpdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
)

const sessionCookieName = "homelab_session"

type sessionUserKey struct{}

// sessionUser returns the user authenticated by requireSession.
func sessionUser(r *http.Request) (domain.User, bool) {
	user, ok := r.Context().Value(sessionUserKey{}).(domain.User)
	return user, ok
}

var loginAttempts = struct {
	sync.Mutex
	byIP map[string]loginWindow
}{byIP: make(map[string]loginWindow)}

type loginWindow struct {
	started time.Time
	count   int
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type passwordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func loginHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		var request loginRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		request.Username = strings.TrimSpace(request.Username)
		if len(request.Username) < 1 || len(request.Username) > 255 || len(request.Password) < 1 || len(request.Password) > 1024 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The credentials are invalid.")
			return
		}
		result, err := auth.Login(r.Context(), request.Username, request.Password, clientIP(r), r.UserAgent())
		if err != nil {
			switch {
			case usecase.IsInvalidCredentials(err):
				writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The username or password is invalid.")
			case errors.Is(err, usecase.ErrTooManyLoginChallenges):
				writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many pending logins.")
			default:
				writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "The authentication request could not be completed.")
			}
			return
		}
		if result.Challenge != "" {
			http.SetCookie(w, challengeCookie(result.Challenge, result.ExpiresAt, secure))
			writeJSON(w, http.StatusOK, map[string]any{
				"secondFactorRequired": true,
				"methods":              result.Methods,
				"expiresAt":            result.ExpiresAt,
			})
			return
		}
		http.SetCookie(w, sessionCookie(sessionCookieName, result.Token, result.ExpiresAt, secure))
		writeJSON(w, http.StatusOK, map[string]any{"user": toSessionUserDTO(result.User)})
	}
}

func logoutHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			if err := auth.Logout(r.Context(), cookie.Value); err != nil {
				if err != domain.ErrSessionNotFound {
					writeError(w, r, http.StatusInternalServerError, "LOGOUT_FAILED", "Could not end the session.")
					return
				}
			}
		}
		http.SetCookie(w, sessionCookie(sessionCookieName, "", time.Unix(0, 0), secure))
		writeJSON(w, http.StatusOK, map[string]bool{"loggedOut": true})
	}
}

func sessionHandler(auth *usecase.AuthUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "No active session.")
			return
		}
		user, _, err := auth.Session(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "No active session.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": toSessionUserDTO(user)})
	}
}

func changePasswordHandler(auth *usecase.AuthUsecase, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "No active session.")
			return
		}
		var request passwordRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		if len(request.CurrentPassword) > 1024 || len(request.NewPassword) < 12 || len(request.NewPassword) > 1024 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The password values are invalid.")
			return
		}
		if err := auth.ChangePassword(r.Context(), cookie.Value, request.CurrentPassword, request.NewPassword); err != nil {
			if usecase.IsInvalidCredentials(err) {
				writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The current password is invalid.")
				return
			}
			if err == domain.ErrSessionNotFound {
				writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "No active session.")
				return
			}
			writeError(w, r, http.StatusInternalServerError, "PASSWORD_CHANGE_FAILED", "Could not change the password.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"changed": true})
	}
}

func sessionCookie(name, value string, expiresAt time.Time, secure bool) *http.Cookie {
	maxAge := 0
	if value != "" {
		maxAge = int(time.Until(expiresAt).Seconds())
	} else {
		maxAge = -1
	}
	return &http.Cookie{
		Name: name, Value: value, Path: "/", HttpOnly: true, Secure: secure,
		SameSite: http.SameSiteLaxMode, Expires: expiresAt, MaxAge: maxAge,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requireSession(auth *usecase.AuthUsecase, tokens *usecase.APITokenUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth == nil {
				writeError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is unavailable.")
				return
			}
			// An API token authenticates a script: Authorization: Bearer hlm_….
			if header := r.Header.Get("Authorization"); header != "" {
				secret, ok := strings.CutPrefix(header, "Bearer ")
				if !ok || tokens == nil {
					writeError(w, r, http.StatusUnauthorized, "TOKEN_INVALID", "The API token is invalid, expired or revoked.")
					return
				}
				user, token, err := tokens.Authenticate(r.Context(), strings.TrimSpace(secret))
				if err != nil {
					writeError(w, r, http.StatusUnauthorized, "TOKEN_INVALID", "The API token is invalid, expired or revoked.")
					return
				}
				ctx := context.WithValue(r.Context(), sessionUserKey{}, user)
				ctx = context.WithValue(ctx, apiTokenKey{}, token)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
				return
			}
			user, session, err := auth.Session(r.Context(), cookie.Value)
			if err != nil {
				writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
				return
			}
			ctx := context.WithValue(r.Context(), sessionUserKey{}, user)
			ctx = context.WithValue(ctx, currentSessionKey{}, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Host, r.Host) {
				writeError(w, r, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "Cross-origin requests are not allowed.")
				return
			}
		}
		if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeError(w, r, http.StatusForbidden, "CROSS_ORIGIN_REQUEST", "Cross-origin requests are not allowed.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func loginRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		key := clientIP(r)
		loginAttempts.Lock()
		window := loginAttempts.byIP[key]
		if window.started.IsZero() || now.Sub(window.started) >= time.Minute {
			window = loginWindow{started: now}
		}
		if _, exists := loginAttempts.byIP[key]; !exists && len(loginAttempts.byIP) >= 10_000 {
			for address, candidate := range loginAttempts.byIP {
				if now.Sub(candidate.started) >= time.Minute {
					delete(loginAttempts.byIP, address)
				}
			}
			if len(loginAttempts.byIP) >= 10_000 {
				loginAttempts.Unlock()
				writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many login attempts.")
				return
			}
		}
		window.count++
		loginAttempts.byIP[key] = window
		limited := window.count > 10
		loginAttempts.Unlock()
		if limited {
			writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many login attempts.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(r *http.Request, destination any) error {
	return decodeStrict(r.Body, destination)
}

func decodeStrict(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := middleware.GetReqID(r.Context())
	writeJSON(w, status, errorResponse{Error: errorBody{
		Code: code, Message: message, RequestID: requestID,
	}})
}
