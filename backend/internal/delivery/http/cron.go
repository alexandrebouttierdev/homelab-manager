package httpdelivery

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"
)

type cronEntryDTO struct {
	Source      string      `json:"source"`
	Line        int         `json:"line"`
	Expression  string      `json:"expression"`
	User        string      `json:"user"`
	Command     string      `json:"command"`
	Valid       bool        `json:"valid"`
	Error       *string     `json:"error"`
	Description *string     `json:"description"`
	NextRuns    []time.Time `json:"nextRuns"`
	LastRun     *time.Time  `json:"lastRun"`
}

type cronDirectoryDTO struct {
	Period      string        `json:"period"`
	Path        string        `json:"path"`
	Scripts     []string      `json:"scripts"`
	ScheduledBy *cronEntryDTO `json:"scheduledBy"`
}

type cronTimerDTO struct {
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	ActiveState   string     `json:"activeState"`
	NextElapse    *time.Time `json:"nextElapse"`
	LastTrigger   *time.Time `json:"lastTrigger"`
	Service       *string    `json:"service"`
	ServiceResult *string    `json:"serviceResult"`
}

type cronDTO struct {
	TimeZone         string             `json:"timeZone"`
	Entries          []cronEntryDTO     `json:"entries"`
	Directories      []cronDirectoryDTO `json:"directories"`
	Anacron          bool               `json:"anacron"`
	Unreadable       []string           `json:"unreadable"`
	Timers           []cronTimerDTO     `json:"timers"`
	TimersAvailable  bool               `json:"timersAvailable"`
	JournalAvailable bool               `json:"journalAvailable"`
}

func toCronEntryDTO(view usecase.CronEntryView) cronEntryDTO {
	entry := view.Entry
	runs := view.NextRuns
	if runs == nil {
		runs = []time.Time{}
	}
	return cronEntryDTO{
		Source: entry.Source, Line: entry.Line, Expression: entry.Expression, User: entry.User, Command: entry.Command,
		Valid: entry.Valid(), Error: optionalString(entry.Error), Description: optionalString(view.Description),
		NextRuns: runs, LastRun: view.LastRun,
	}
}

func cronHandler(logger *zap.Logger, cron *usecase.CronUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view, err := cron.Inventory(r.Context())
		if err != nil {
			logger.Error("Could not read the scheduled tasks",
				zap.String("request_id", middleware.GetReqID(r.Context())), zap.Error(err))
			writeError(w, r, http.StatusInternalServerError, "CRON_READ_FAILED", "Could not read the scheduled tasks.")
			return
		}
		response := cronDTO{
			TimeZone: view.TimeZone, Entries: make([]cronEntryDTO, 0, len(view.Entries)),
			Directories: make([]cronDirectoryDTO, 0, len(view.Directories)), Anacron: view.Anacron,
			Unreadable: orEmpty(view.Unreadable), Timers: make([]cronTimerDTO, 0, len(view.Timers)),
			TimersAvailable: view.TimersAvailable, JournalAvailable: view.JournalAvailable,
		}
		for _, entry := range view.Entries {
			response.Entries = append(response.Entries, toCronEntryDTO(entry))
		}
		for _, directory := range view.Directories {
			item := cronDirectoryDTO{
				Period: directory.Directory.Period, Path: directory.Directory.Path, Scripts: orEmpty(directory.Directory.Scripts),
			}
			if directory.ScheduledBy != nil {
				scheduled := toCronEntryDTO(*directory.ScheduledBy)
				item.ScheduledBy = &scheduled
			}
			response.Directories = append(response.Directories, item)
		}
		for _, timer := range view.Timers {
			response.Timers = append(response.Timers, cronTimerDTO{
				Name: timer.Timer.Name, Description: timer.Timer.Description, ActiveState: timer.Timer.ActiveState,
				NextElapse: timer.Timer.NextElapse, LastTrigger: timer.Timer.LastTrigger,
				Service: optionalString(timer.Service), ServiceResult: optionalString(timer.ServiceResult),
			})
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func cronInspectHandler(cron *usecase.CronUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, values := range query {
			if key != "expression" || len(values) != 1 {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Only expression is accepted, once.")
				return
			}
		}
		inspection, err := cron.Inspect(query.Get("expression"))
		switch {
		case errors.Is(err, domain.ErrInvalidCronExpression):
			writeError(w, r, http.StatusBadRequest, "CRON_EXPRESSION_INVALID", err.Error())
			return
		case err != nil:
			writeError(w, r, http.StatusInternalServerError, "CRON_READ_FAILED", "Could not read the expression.")
			return
		}
		runs := inspection.NextRuns
		if runs == nil {
			runs = []time.Time{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"description": inspection.Description, "nextRuns": runs})
	}
}
