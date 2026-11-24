package httpdelivery

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

type backupRepositoryDTO struct {
	Type     string `json:"type"`
	Location string `json:"location"`
}

type backupScheduleDTO struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency"`
	Weekday   int    `json:"weekday"`
	// Time is "HH:MM" in the time zone of the machine.
	Time string `json:"time"`
}

type backupCredentialsDTO struct {
	Password bool `json:"password"`
	Username bool `json:"username"`
	Secret   bool `json:"secret"`
}

type backupConfigDTO struct {
	Repository       *backupRepositoryDTO `json:"repository"`
	Initialized      bool                 `json:"initialized"`
	InitializedAt    *time.Time           `json:"initializedAt"`
	Credentials      backupCredentialsDTO `json:"credentials"`
	SecretsAvailable bool                 `json:"secretsAvailable"`
	Sources          []string             `json:"sources"`
	Excludes         []string             `json:"excludes"`
	ExcludeCaches    bool                 `json:"excludeCaches"`
	ExcludeIfPresent string               `json:"excludeIfPresent"`
	Schedule         backupScheduleDTO    `json:"schedule"`
	UpdatedAt        *time.Time           `json:"updatedAt"`
	UpdatedBy        *string              `json:"updatedBy"`
}

// backupConfigRequest replaces the configuration. username and secret:
// absent or null keeps them, "" removes them.
type backupConfigRequest struct {
	Repository       *backupRepositoryDTO `json:"repository"`
	Username         *string              `json:"username"`
	Secret           *string              `json:"secret"`
	Sources          []string             `json:"sources"`
	Excludes         []string             `json:"excludes"`
	ExcludeCaches    bool                 `json:"excludeCaches"`
	ExcludeIfPresent string               `json:"excludeIfPresent"`
	Schedule         backupScheduleDTO    `json:"schedule"`
}

type backupStatsDTO struct {
	FilesNew            int64   `json:"filesNew"`
	FilesChanged        int64   `json:"filesChanged"`
	FilesUnmodified     int64   `json:"filesUnmodified"`
	DirsNew             int64   `json:"dirsNew"`
	DirsChanged         int64   `json:"dirsChanged"`
	DirsUnmodified      int64   `json:"dirsUnmodified"`
	DataAdded           int64   `json:"dataAdded"`
	TotalFilesProcessed int64   `json:"totalFilesProcessed"`
	TotalBytesProcessed int64   `json:"totalBytesProcessed"`
	DurationSeconds     float64 `json:"durationSeconds"`
}

type backupRunDTO struct {
	ID          uuid.UUID       `json:"id"`
	Operation   string          `json:"operation"`
	Origin      string          `json:"origin"`
	RequestedBy *string         `json:"requestedBy"`
	State       string          `json:"state"`
	Failure     *string         `json:"failure"`
	Message     *string         `json:"message"`
	Warning     bool            `json:"warning"`
	SnapshotID  *string         `json:"snapshotId"`
	Stats       *backupStatsDTO `json:"stats"`
	StartedAt   time.Time       `json:"startedAt"`
	FinishedAt  *time.Time      `json:"finishedAt"`
}

type backupSnapshotDTO struct {
	ID         string    `json:"id"`
	ShortID    string    `json:"shortId"`
	Time       time.Time `json:"time"`
	Hostname   string    `json:"hostname"`
	Paths      []string  `json:"paths"`
	Tags       []string  `json:"tags"`
	TotalFiles int64     `json:"totalFiles"`
	TotalBytes int64     `json:"totalBytes"`
	DataAdded  int64     `json:"dataAdded"`
}

type backupSummaryDTO struct {
	Configured  bool          `json:"configured"`
	Initialized bool          `json:"initialized"`
	Running     *backupRunDTO `json:"running"`
	LastRun     *backupRunDTO `json:"lastRun"`
	LastSuccess *backupRunDTO `json:"lastSuccess"`
	NextRunAt   *time.Time    `json:"nextRunAt"`
}

func toBackupRunDTO(run *domain.BackupRun) *backupRunDTO {
	if run == nil {
		return nil
	}
	dto := &backupRunDTO{
		ID: run.ID, Operation: string(run.Operation), Origin: string(run.Origin),
		RequestedBy: optionalString(run.RequestedBy), State: string(run.State),
		Failure: optionalString(string(run.Failure)), Message: optionalString(run.Message), Warning: run.Warning,
		SnapshotID: optionalString(run.SnapshotID), StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}
	if s := run.Stats; s != nil {
		dto.Stats = &backupStatsDTO{
			FilesNew: s.FilesNew, FilesChanged: s.FilesChanged, FilesUnmodified: s.FilesUnmodified,
			DirsNew: s.DirsNew, DirsChanged: s.DirsChanged, DirsUnmodified: s.DirsUnmodified,
			DataAdded: s.DataAdded, TotalFilesProcessed: s.TotalFilesProcessed,
			TotalBytesProcessed: s.TotalBytesProcessed, DurationSeconds: s.DurationSeconds,
		}
	}
	return dto
}

func toBackupConfigDTO(view usecase.BackupConfigView) backupConfigDTO {
	dto := backupConfigDTO{
		Initialized: view.Initialized, InitializedAt: view.InitializedAt,
		Credentials:      backupCredentialsDTO{Password: view.HasPassword, Username: view.HasUsername, Secret: view.HasSecret},
		SecretsAvailable: view.SecretsEnabled,
		Sources:          orEmpty(view.Selection.Sources), Excludes: orEmpty(view.Selection.Excludes),
		ExcludeCaches: view.Selection.ExcludeCaches, ExcludeIfPresent: view.Selection.ExcludeIfPresent,
		Schedule: backupScheduleDTO{
			Enabled: view.Schedule.Enabled, Frequency: string(view.Schedule.Frequency), Weekday: int(view.Schedule.Weekday),
			Time: fmt.Sprintf("%02d:%02d", view.Schedule.Hour, view.Schedule.Minute),
		},
		UpdatedAt: view.UpdatedAt, UpdatedBy: optionalString(view.UpdatedBy),
	}
	if view.Repository != nil {
		dto.Repository = &backupRepositoryDTO{Type: string(view.Repository.Type), Location: view.Repository.Location}
	}
	return dto
}

// parseScheduleTime reads "HH:MM".
func parseScheduleTime(value string) (int, int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || len(value) != 5 {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}

// requireBackups answers 503 without the backups module (no database).
func requireBackups(backups *usecase.BackupUsecase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if backups == nil {
				writeError(w, r, http.StatusServiceUnavailable, "BACKUPS_UNAVAILABLE", "Backups are unavailable.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeBackupError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *domain.BackupValidationError
	switch {
	case errors.As(err, &validation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid "+validation.Error()+".")
	case errors.Is(err, domain.ErrInvalidBackupRunFilter):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be 1 to 200.")
	case errors.Is(err, domain.ErrBackupRunNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The backup run does not exist.")
	case errors.Is(err, domain.ErrBackupNotConfigured):
		writeError(w, r, http.StatusConflict, "BACKUP_NOT_CONFIGURED", "No backup repository is configured.")
	case errors.Is(err, domain.ErrBackupNotInitialized):
		writeError(w, r, http.StatusConflict, "BACKUP_NOT_INITIALIZED", "The backup repository is not initialized.")
	case errors.Is(err, domain.ErrBackupAlreadyInitialized):
		writeError(w, r, http.StatusConflict, "BACKUP_ALREADY_INITIALIZED", "The backup repository is already initialized.")
	case errors.Is(err, domain.ErrBackupAlreadyRunning), errors.Is(err, domain.ErrBackupRepositoryInUse):
		writeError(w, r, http.StatusConflict, "BACKUP_IN_PROGRESS", "A backup operation is already running.")
	case errors.Is(err, domain.ErrBackupsNotReady):
		writeError(w, r, http.StatusServiceUnavailable, "BACKUPS_UNAVAILABLE", "Backups are starting or stopping.")
	case errors.Is(err, domain.ErrBackupSecretsMissing):
		writeError(w, r, http.StatusServiceUnavailable, "SECRETS_UNAVAILABLE", "The secrets key is unavailable.")
	default:
		writeError(w, r, http.StatusInternalServerError, "BACKUPS_FAILED", "Could not read or run the backups.")
	}
}

func backupSummaryHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := backups.Summary(r.Context())
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, backupSummaryDTO{
			Configured: summary.Configured, Initialized: summary.Initialized,
			Running: toBackupRunDTO(summary.Running), LastRun: toBackupRunDTO(summary.LastRun),
			LastSuccess: toBackupRunDTO(summary.LastSuccess), NextRunAt: summary.NextRunAt,
		})
	}
}

func backupConfigHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := backups.Config(r.Context())
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toBackupConfigDTO(view))
	}
}

func updateBackupConfigHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request backupConfigRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The backup configuration is invalid.")
			return
		}
		hour, minute, ok := parseScheduleTime(request.Schedule.Time)
		if !ok {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "schedule.time must be HH:MM.")
			return
		}
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		update := usecase.BackupConfigUpdate{
			Username: request.Username, Secret: request.Secret,
			Selection: domain.BackupSelection{
				Sources: request.Sources, Excludes: request.Excludes,
				ExcludeCaches: request.ExcludeCaches, ExcludeIfPresent: request.ExcludeIfPresent,
			},
			Schedule: domain.BackupSchedule{
				Enabled: request.Schedule.Enabled, Frequency: domain.BackupFrequency(request.Schedule.Frequency),
				Weekday: time.Weekday(request.Schedule.Weekday), Hour: hour, Minute: minute,
			},
		}
		if request.Repository != nil {
			update.Repository = &domain.BackupRepository{
				Type: domain.BackupRepositoryType(request.Repository.Type), Location: request.Repository.Location,
			}
		}
		view, err := backups.UpdateConfig(r.Context(), actor, update, middleware.GetReqID(r.Context()))
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toBackupConfigDTO(view))
	}
}

func initBackupRepositoryHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		password, run, err := backups.InitRepository(r.Context(), actor, middleware.GetReqID(r.Context()))
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		// The password is shown once: it is never served again.
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusAccepted, map[string]any{"password": password, "run": toBackupRunDTO(&run)})
	}
}

func requestBackupHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := sessionUser(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "SESSION_NOT_FOUND", "Authentication is required.")
			return
		}
		run, err := backups.RequestBackup(r.Context(), actor, middleware.GetReqID(r.Context()))
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run": toBackupRunDTO(&run)})
	}
}

func listBackupRunsHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit := domain.DefaultBackupRunLimit
		for key, values := range query {
			if key != "limit" || len(values) != 1 {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Only limit is accepted, once.")
				return
			}
		}
		if value := query.Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be 1 to 200.")
				return
			}
			limit = parsed
		}
		runs, err := backups.Runs(r.Context(), domain.BackupRunFilter{Limit: limit})
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		response := make([]*backupRunDTO, 0, len(runs))
		for i := range runs {
			response = append(response, toBackupRunDTO(&runs[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": response})
	}
}

func backupRunHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "runID"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The run identifier is invalid.")
			return
		}
		run, err := backups.BackupRun(r.Context(), id)
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": toBackupRunDTO(&run)})
	}
}

func listBackupSnapshotsHandler(backups *usecase.BackupUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshots, err := backups.Snapshots(r.Context())
		if err != nil {
			writeBackupError(w, r, err)
			return
		}
		response := make([]backupSnapshotDTO, 0, len(snapshots))
		for _, s := range snapshots {
			response = append(response, backupSnapshotDTO{
				ID: s.ID, ShortID: s.ShortID, Time: s.Time, Hostname: s.Hostname, Paths: orEmpty(s.Paths),
				Tags: orEmpty(s.Tags), TotalFiles: s.TotalFiles, TotalBytes: s.TotalBytes, DataAdded: s.DataAdded,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": response})
	}
}
