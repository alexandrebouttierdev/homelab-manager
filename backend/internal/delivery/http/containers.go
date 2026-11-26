package httpdelivery

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type containerLogLineDTO struct {
	Stream  string     `json:"stream"`
	Time    *time.Time `json:"time"`
	Message string     `json:"message"`
	Cut     bool       `json:"cut"`
}

type containerActionDTO struct {
	ContainerID uuid.UUID  `json:"containerId"`
	Name        string     `json:"name"`
	Action      string     `json:"action"`
	State       string     `json:"state"`
	Failure     *string    `json:"failure"`
	RequestedBy string     `json:"requestedBy"`
	RequestedAt time.Time  `json:"requestedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
}

type containerActionRequest struct {
	Action string `json:"action"`
}

func toContainerActionDTO(action domain.ContainerAction) containerActionDTO {
	return containerActionDTO{
		ContainerID: action.ContainerID, Name: action.Name, Action: string(action.Verb), State: string(action.State),
		Failure: optionalString(string(action.Failure)), RequestedBy: action.RequestedBy,
		RequestedAt: action.RequestedAt, FinishedAt: action.FinishedAt,
	}
}

// requireContainers answers 503 without the usecase (no database).
func requireContainers(containers *usecase.ContainerUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if containers == nil {
				writeError(w, r, http.StatusServiceUnavailable, "CONTAINERS_UNAVAILABLE", "Container logs and actions are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// parseLogQuery reads tail (1 to 2000, 200 by default) and since (RFC 3339).
func parseLogQuery(r *http.Request) (domain.ContainerLogQuery, bool) {
	query := domain.ContainerLogQuery{Tail: domain.DefaultContainerLogLines}
	values := r.URL.Query()
	for key, items := range values {
		if (key != "tail" && key != "since") || len(items) != 1 {
			return query, false
		}
	}
	if raw := values.Get("tail"); raw != "" {
		tail, err := strconv.Atoi(raw)
		if err != nil {
			return query, false
		}
		query.Tail = tail
	}
	if raw := values.Get("since"); raw != "" {
		since, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return query, false
		}
		query.Since = &since
	}
	return query, query.Validate() == nil
}

func containerLogsHandler(containers *usecase.ContainerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := containerIDParam(w, r)
		if !ok {
			return
		}
		query, ok := parseLogQuery(r)
		if !ok {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "tail must be 1 to 2000 and since an RFC 3339 time in the past.")
			return
		}
		container, logs, err := containers.Logs(r.Context(), id, query)
		switch {
		case errors.Is(err, domain.ErrContainerNotFound):
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
			return
		case errors.Is(err, domain.ErrContainerRemoved):
			writeError(w, r, http.StatusConflict, "CONTAINER_REMOVED", "The container was removed from the Docker host.")
			return
		case errors.Is(err, domain.ErrInvalidLogQuery):
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "tail must be 1 to 2000 and since an RFC 3339 time in the past.")
			return
		case err != nil:
			writeError(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "The Docker log could not be read.")
			return
		}
		lines := make([]containerLogLineDTO, 0, len(logs.Lines))
		for _, line := range logs.Lines {
			lines = append(lines, containerLogLineDTO{Stream: string(line.Stream), Time: line.Time, Message: line.Message, Cut: line.Cut})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"container": map[string]any{"id": container.ID, "name": container.Name},
			"lines":     lines, "truncated": logs.Truncated, "tail": query.Tail,
		})
	}
}

func containerActionsSettingsHandler(containers *usecase.ContainerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		verbs := make([]string, 0, len(domain.ContainerVerbs))
		for _, verb := range domain.ContainerVerbs {
			verbs = append(verbs, string(verb))
		}
		writeJSON(w, http.StatusOK, map[string]any{"enabled": containers.ActionsEnabled(), "actions": verbs})
	}
}

func requestContainerActionHandler(containers *usecase.ContainerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := containerIDParam(w, r)
		if !ok {
			return
		}
		var request containerActionRequest
		if err := decodeJSON(r, &request); err != nil || !domain.ContainerVerb(request.Action).Valid() {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "action must be start, stop or restart.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		action, err := containers.Request(r.Context(), actor, id, domain.ContainerVerb(request.Action), middleware.GetReqID(r.Context()))
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, map[string]any{"action": toContainerActionDTO(action)})
		case errors.Is(err, domain.ErrContainerActionsDisabled):
			writeError(w, r, http.StatusConflict, "CONTAINER_ACTIONS_DISABLED", "Container actions are disabled in the service configuration.")
		case errors.Is(err, domain.ErrContainerNotFound):
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
		case errors.Is(err, domain.ErrContainerRemoved):
			writeError(w, r, http.StatusConflict, "CONTAINER_REMOVED", "The container was removed from the Docker host.")
		case errors.Is(err, domain.ErrActionInProgress):
			writeError(w, r, http.StatusConflict, "ACTION_IN_PROGRESS", "An action is already in progress on this container.")
		case errors.Is(err, domain.ErrContainerActionsLimited):
			writeError(w, r, http.StatusTooManyRequests, "CONTAINER_ACTION_RATE_LIMITED", "Too many container actions in the last minute.")
		case errors.Is(err, domain.ErrContainerActionsUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "CONTAINERS_UNAVAILABLE", "Container actions are unavailable.")
		default:
			writeError(w, r, http.StatusInternalServerError, "CONTAINER_ACTION_FAILED", "The action could not be requested.")
		}
	}
}

func lastContainerActionHandler(containers *usecase.ContainerUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := containerIDParam(w, r)
		if !ok {
			return
		}
		action, found, err := containers.Last(r.Context(), id)
		switch {
		case errors.Is(err, domain.ErrContainerNotFound):
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Container not found.")
		case err != nil:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Could not load the container.")
		case !found:
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "No action was requested on this container since the Manager started.")
		default:
			writeJSON(w, http.StatusOK, map[string]any{"action": toContainerActionDTO(action)})
		}
	}
}
