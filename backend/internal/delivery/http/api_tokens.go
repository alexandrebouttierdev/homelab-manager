package httpdelivery

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

// apiTokenKey holds the token of a request authenticated by one.
type apiTokenKey struct{}

// tokenScopes maps the read-only routes a token may reach to their scope
// (SPEC_ADMINISTRATION.md § 3.5): the path itself or anything under it.
var tokenScopes = []struct {
	path  string
	scope domain.APITokenScope
}{
	{"/api/v1/dashboard", domain.ScopeMetricsRead},
	{"/api/v1/host", domain.ScopeMetricsRead},
	{"/api/v1/containers", domain.ScopeContainersRead},
	{"/api/v1/docker", domain.ScopeContainersRead},
	{"/api/v1/events", domain.ScopeEventsRead},
	{"/api/v1/system/units", domain.ScopeServicesRead},
	{"/api/v1/backups/summary", domain.ScopeBackupsRead},
	{"/api/v1/backups/runs", domain.ScopeBackupsRead},
	{"/api/v1/backups/snapshots", domain.ScopeBackupsRead},
}

// tokenScopeFor returns the scope that opens a path to a token. The log of
// a container is not opened to tokens, like the system journal: it may
// hold what an application writes about its users.
func tokenScopeFor(path string) (domain.APITokenScope, bool) {
	path = strings.TrimSuffix(path, "/")
	if strings.HasPrefix(path, "/api/v1/containers/") && strings.HasSuffix(path, "/logs") {
		return "", false
	}
	for _, route := range tokenScopes {
		if path == route.path || strings.HasPrefix(path, route.path+"/") {
			return route.scope, true
		}
	}
	return "", false
}

// tokenScopeGate lets a request authenticated by an API token read only
// the routes of its scopes, with GET; a session goes through untouched.
func tokenScopeGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := r.Context().Value(apiTokenKey{}).(domain.APIToken)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		scope, known := tokenScopeFor(r.URL.Path)
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !known || !token.Allows(scope) {
			writeError(w, r, http.StatusForbidden, "TOKEN_SCOPE_DENIED", "The API token does not allow this request.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type apiTokenDTO struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	Username   string     `json:"username"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func toAPITokenDTO(token domain.APIToken) apiTokenDTO {
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	return apiTokenDTO{
		ID: token.ID, Name: token.Name, Prefix: token.Prefix, Scopes: scopes, Username: token.Username,
		CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt, LastUsedAt: token.LastUsedAt,
	}
}

func writeAPITokenError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidAPIToken):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "name (1 to 100 characters), scopes and expiresInDays (1 to 365) are required.")
	case errors.Is(err, domain.ErrTooManyAPITokens):
		writeError(w, r, http.StatusConflict, "TOO_MANY_TOKENS", "An account has at most 20 API tokens.")
	case errors.Is(err, domain.ErrAPITokenNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The API token does not exist.")
	default:
		writeError(w, r, http.StatusInternalServerError, "TOKENS_FAILED", "Could not read or change the API tokens.")
	}
}

func requireAPITokens(tokens *usecase.APITokenUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokens == nil {
				writeError(w, r, http.StatusServiceUnavailable, "TOKENS_UNAVAILABLE", "API tokens are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func listAPITokensHandler(tokens *usecase.APITokenUsecase, deny func(*http.Request, domain.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		query := r.URL.Query()
		for key, values := range query {
			if key != "scope" || len(values) != 1 || (values[0] != "all" && values[0] != "mine") {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "scope must be all or mine.")
				return
			}
		}
		all := query.Get("scope") == "all"
		if all && !actor.Role.Can(domain.PermManageUsers) {
			deny(r, actor)
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Your role does not allow this action.")
			return
		}
		list, err := tokens.List(r.Context(), actor, all)
		if err != nil {
			writeAPITokenError(w, r, err)
			return
		}
		response := make([]apiTokenDTO, 0, len(list))
		for _, token := range list {
			response = append(response, toAPITokenDTO(token))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": response, "scopes": domain.APITokenScopes})
	}
}

type createAPITokenRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expiresInDays"`
}

func createAPITokenHandler(tokens *usecase.APITokenUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request createAPITokenRequest
		if err := decodeJSON(r, &request); err != nil {
			writeAPITokenError(w, r, domain.ErrInvalidAPIToken)
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		scopes := make([]domain.APITokenScope, 0, len(request.Scopes))
		for _, scope := range request.Scopes {
			scopes = append(scopes, domain.APITokenScope(scope))
		}
		secret, token, err := tokens.Create(r.Context(), actor, domain.APITokenRequest{
			Name: request.Name, Scopes: scopes, LifetimeDays: request.ExpiresInDays,
		}, middleware.GetReqID(r.Context()))
		if err != nil {
			writeAPITokenError(w, r, err)
			return
		}
		// The secret is shown once: it is never served again.
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, map[string]any{"token": secret, "apiToken": toAPITokenDTO(token)})
	}
}

func revokeAPITokenHandler(tokens *usecase.APITokenUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "tokenID"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The token identifier is invalid.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := tokens.Revoke(r.Context(), actor, id, middleware.GetReqID(r.Context())); err != nil {
			writeAPITokenError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
