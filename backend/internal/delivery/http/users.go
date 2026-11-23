package httpdelivery

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

var userIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// userDTO is an account as shown to administrators; never the password hash.
type userDTO struct {
	ID                 uuid.UUID  `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Role               string     `json:"role"`
	Disabled           bool       `json:"disabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	LastLoginAt        *time.Time `json:"lastLoginAt"`
	LastLoginIP        *string    `json:"lastLoginIp"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func toUserDTO(user domain.User) userDTO {
	dto := userDTO{
		ID: user.ID, Username: user.Username, DisplayName: user.DisplayName,
		Role: string(user.Role), Disabled: user.Disabled,
		MustChangePassword: user.MustChangePassword, LastLoginAt: user.LastLoginAt,
		CreatedAt: user.CreatedAt,
	}
	if user.LastLoginIP != "" {
		ip := user.LastLoginIP
		dto.LastLoginIP = &ip
	}
	return dto
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Password    string `json:"password"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Role        *string `json:"role"`
	Disabled    *bool   `json:"disabled"`
}

func listUsersHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := accounts.List(r.Context())
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "USERS_UNAVAILABLE", "Could not list the accounts.")
			return
		}
		dtos := make([]userDTO, 0, len(users))
		for _, user := range users {
			dtos = append(dtos, toUserDTO(user))
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": dtos})
	}
}

func createUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createUserRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		input, err := domain.NewUser{
			Username: request.Username, DisplayName: request.DisplayName, Role: domain.Role(request.Role),
		}.Normalize()
		if err == nil {
			err = domain.ValidateNewPassword(request.Password)
		}
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		created, err := accounts.Create(r.Context(), actor, input, request.Password, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"user": toUserDTO(created)})
	}
}

func updateUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDParam(w, r)
		if !ok {
			return
		}
		var request updateUserRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The request body is invalid.")
			return
		}
		change := domain.UserChange{DisplayName: request.DisplayName, Disabled: request.Disabled}
		if request.Role != nil {
			role := domain.Role(*request.Role)
			change.Role = &role
		}
		change, err := change.Normalize()
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		updated, err := accounts.Update(r.Context(), actor, id, change, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(updated)})
	}
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

func resetPasswordHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDParam(w, r)
		if !ok {
			return
		}
		var request resetPasswordRequest
		if err := decodeJSON(r, &request); err != nil || domain.ValidateNewPassword(request.Password) != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "password must be 12 to 1024 bytes.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		reset, err := accounts.ResetPassword(r.Context(), actor, id, request.Password, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(reset)})
	}
}

func deleteUserHandler(accounts *usecase.AccountUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := userIDParam(w, r)
		if !ok {
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := accounts.Delete(r.Context(), actor, id, middleware.GetReqID(r.Context())); err != nil {
			writeAccountError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

// userIDParam validates the {userID} path parameter (canonical lowercase UUID).
func userIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return uuidParam(w, r, "userID")
}

func writeAccountError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case domain.IsValidationError(err):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The account values are invalid.")
	case errors.Is(err, domain.ErrUserNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The account does not exist.")
	case errors.Is(err, domain.ErrUsernameTaken):
		writeError(w, r, http.StatusConflict, "USERNAME_TAKEN", "The username is already used.")
	case errors.Is(err, domain.ErrOwnPasswordReset):
		writeError(w, r, http.StatusConflict, "OWN_PASSWORD_RESET", "Change your own password instead of resetting it.")
	case errors.Is(err, domain.ErrLastAdmin):
		writeError(w, r, http.StatusConflict, "LAST_ADMIN", "The last active administrator must remain.")
	default:
		writeError(w, r, http.StatusInternalServerError, "ACCOUNT_UPDATE_FAILED", "Could not update the account.")
	}
}
