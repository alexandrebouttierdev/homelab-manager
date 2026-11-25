package httpdelivery

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type maintenanceRunDTO struct {
	At        time.Time `json:"at"`
	Origin    string    `json:"origin"`
	Succeeded bool      `json:"succeeded"`
	Message   *string   `json:"message"`
}

type maintenanceTaskDTO struct {
	Kind       string             `json:"kind"`
	Schedule   backupScheduleDTO  `json:"schedule"`
	MaxSizeMiB int                `json:"maxSizeMiB"`
	Sizes      []int              `json:"sizes"`
	Unit       string             `json:"unit"`
	Running    bool               `json:"running"`
	NextRunAt  *time.Time         `json:"nextRunAt"`
	LastRun    *maintenanceRunDTO `json:"lastRun"`
	UpdatedAt  *time.Time         `json:"updatedAt"`
	UpdatedBy  *string            `json:"updatedBy"`
}

type maintenanceTaskRequest struct {
	Schedule   backupScheduleDTO `json:"schedule"`
	MaxSizeMiB int               `json:"maxSizeMiB"`
}

func toMaintenanceTaskDTO(view usecase.MaintenanceTaskView) maintenanceTaskDTO {
	dto := maintenanceTaskDTO{
		Kind: string(view.Kind),
		Schedule: backupScheduleDTO{
			Enabled: view.Schedule.Enabled, Frequency: string(view.Schedule.Frequency), Weekday: int(view.Schedule.Weekday),
			Time: fmt.Sprintf("%02d:%02d", view.Schedule.Hour, view.Schedule.Minute),
		},
		MaxSizeMiB: view.MaxSizeMiB, Sizes: domain.JournalVacuumSizes, Unit: view.Unit(), Running: view.Running,
		NextRunAt: view.NextRun, UpdatedAt: view.UpdatedAt, UpdatedBy: optionalString(view.UpdatedBy),
	}
	if run := view.LastRun; run != nil {
		dto.LastRun = &maintenanceRunDTO{At: run.At, Origin: string(run.Origin), Succeeded: run.Succeeded, Message: optionalString(run.Message)}
	}
	return dto
}

func requireMaintenance(maintenance *usecase.MaintenanceUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if maintenance == nil {
				writeError(w, r, http.StatusServiceUnavailable, "MAINTENANCE_UNAVAILABLE", "Maintenance tasks are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeMaintenanceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrUnknownMaintenanceTask):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The maintenance task is not in the catalog.")
	case errors.Is(err, domain.ErrInvalidMaintenanceTask):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, domain.ErrActionInProgress):
		writeError(w, r, http.StatusConflict, "ACTION_IN_PROGRESS", "A maintenance task is already running.")
	case errors.Is(err, domain.ErrMaintenanceUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "MAINTENANCE_UNAVAILABLE", "Maintenance tasks are unavailable.")
	default:
		writeError(w, r, http.StatusInternalServerError, "MAINTENANCE_FAILED", "Could not read or change the maintenance tasks.")
	}
}

func listMaintenanceTasksHandler(maintenance *usecase.MaintenanceUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		views, err := maintenance.Tasks(r.Context())
		if err != nil {
			writeMaintenanceError(w, r, err)
			return
		}
		tasks := make([]maintenanceTaskDTO, 0, len(views))
		for _, view := range views {
			tasks = append(tasks, toMaintenanceTaskDTO(view))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
	}
}

func updateMaintenanceTaskHandler(maintenance *usecase.MaintenanceUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request maintenanceTaskRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "schedule and maxSizeMiB are required.")
			return
		}
		var hour, minute int
		if _, err := fmt.Sscanf(request.Schedule.Time, "%02d:%02d", &hour, &minute); err != nil || len(request.Schedule.Time) != 5 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "schedule.time must be HH:MM.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		task := domain.MaintenanceTask{
			Kind: domain.MaintenanceKind(chi.URLParam(r, "kind")),
			Schedule: domain.BackupSchedule{
				Enabled: request.Schedule.Enabled, Frequency: domain.BackupFrequency(request.Schedule.Frequency),
				Weekday: time.Weekday(request.Schedule.Weekday), Hour: hour, Minute: minute,
			},
			MaxSizeMiB: request.MaxSizeMiB,
		}
		view, err := maintenance.Update(r.Context(), actor, task, middleware.GetReqID(r.Context()))
		if err != nil {
			writeMaintenanceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": toMaintenanceTaskDTO(view)})
	}
}

func runMaintenanceTaskHandler(maintenance *usecase.MaintenanceUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		if err := maintenance.Request(r.Context(), actor, domain.MaintenanceKind(chi.URLParam(r, "kind")), middleware.GetReqID(r.Context())); err != nil {
			writeMaintenanceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"requested": true})
	}
}
