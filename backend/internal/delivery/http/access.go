package httpdelivery

import (
	"net/http"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// sessionUserDTO is the signed-in user returned by login and session.
type sessionUserDTO struct {
	ID                 uuid.UUID `json:"id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"displayName"`
	Role               string    `json:"role"`
	MustChangePassword bool      `json:"mustChangePassword"`
}

func toSessionUserDTO(user domain.User) sessionUserDTO {
	return sessionUserDTO{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		Role: string(user.Role), MustChangePassword: user.MustChangePassword,
	}
}

// requirePermission refuses the request unless the role of the session
// grants p. The policy itself is domain.Role.Can.
func requirePermission(p domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := sessionUser(r)
			if !ok {
				writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
				return
			}
			if !user.Role.Can(p) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "The account role does not allow this action.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requirePasswordChanged blocks the routes it guards until an account
// created by an administrator has chosen its own password. Logout and the
// password change itself stay outside this guard.
func requirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := sessionUser(r); ok && user.MustChangePassword {
			writeError(w, r, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "The password must be changed first.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
