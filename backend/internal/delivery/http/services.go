package httpdelivery

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

type unitDTO struct {
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Type           string     `json:"type"`
	LoadState      string     `json:"loadState"`
	ActiveState    string     `json:"activeState"`
	SubState       string     `json:"subState"`
	UnitFileState  *string    `json:"unitFileState"`
	StateChangedAt *time.Time `json:"stateChangedAt"`
	MainPID        *uint32    `json:"mainPid"`
	MemoryBytes    *uint64    `json:"memoryBytes"`
	CPUNanoseconds *uint64    `json:"cpuNanoseconds"`
	Result         *string    `json:"result"`
	NextElapse     *time.Time `json:"nextElapse"`
	LastTrigger    *time.Time `json:"lastTrigger"`
	Triggers       []string   `json:"triggers"`
}

func toUnitDTO(unit domain.SystemUnit) unitDTO {
	triggers := unit.Triggers
	if triggers == nil {
		triggers = []string{}
	}
	return unitDTO{
		Name: unit.Name, Description: unit.Description, Type: string(unit.Type),
		LoadState: unit.LoadState, ActiveState: unit.ActiveState, SubState: unit.SubState,
		UnitFileState: optionalString(unit.UnitFileState), StateChangedAt: unit.StateChangedAt,
		MainPID: unit.MainPID, MemoryBytes: unit.MemoryBytes, CPUNanoseconds: unit.CPUNanoseconds,
		Result: optionalString(unit.Result), NextElapse: unit.NextElapse, LastTrigger: unit.LastTrigger,
		Triggers: triggers,
	}
}

type unitCountsDTO struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	Failed       int `json:"failed"`
	ActiveTimers int `json:"activeTimers"`
}

type unitListDTO struct {
	Status      string        `json:"status"`
	CollectedAt *time.Time    `json:"collectedAt"`
	Counts      unitCountsDTO `json:"counts"`
	Units       []unitDTO     `json:"units"`
}

// unitLimitDTO tells "infinity" (unlimited) from an unknown limit (value
// null, unlimited false).
type unitLimitDTO struct {
	Value     *uint64 `json:"value"`
	Unlimited bool    `json:"unlimited"`
}

type unitDefinitionDTO struct {
	FragmentPath *string      `json:"fragmentPath"`
	ExecStart    []string     `json:"execStart"`
	Requires     []string     `json:"requires"`
	After        []string     `json:"after"`
	Restart      *string      `json:"restart"`
	Slice        *string      `json:"slice"`
	TasksMax     unitLimitDTO `json:"tasksMax"`
	LimitNOFILE  unitLimitDTO `json:"limitNofile"`
	MemoryMax    unitLimitDTO `json:"memoryMax"`
}

type unitJournalDTO struct {
	Available bool              `json:"available"`
	Entries   []journalEntryDTO `json:"entries"`
}

type unitDetailDTO struct {
	Unit       unitDTO           `json:"unit"`
	Definition unitDefinitionDTO `json:"definition"`
	Journal    unitJournalDTO    `json:"journal"`
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func toUnitLimitDTO(limit domain.UnitLimit) unitLimitDTO {
	return unitLimitDTO{Value: limit.Value, Unlimited: limit.Unlimited}
}

func listUnitsHandler(units *usecase.ServiceUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inventory := units.Inventory()
		response := unitListDTO{
			Status: string(inventory.Status), CollectedAt: inventory.CollectedAt,
			Counts: unitCountsDTO{
				Total: inventory.Counts.Total, Active: inventory.Counts.Active,
				Failed: inventory.Counts.Failed, ActiveTimers: inventory.Counts.ActiveTimers,
			},
			Units: make([]unitDTO, 0, len(inventory.Units)),
		}
		for _, unit := range inventory.Units {
			response.Units = append(response.Units, toUnitDTO(unit))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func unitDetailHandler(logger *zap.Logger, units *usecase.ServiceUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// chi matches the escaped path: "getty%40tty1.service" arrives
		// escaped and is decoded once here.
		name, err := url.PathUnescape(chi.URLParam(r, "unitName"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The unit name is invalid.")
			return
		}
		detail, err := units.Detail(r.Context(), name)
		switch {
		case errors.Is(err, domain.ErrInvalidUnitName):
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The unit name is invalid.")
			return
		case errors.Is(err, domain.ErrUnitNotFound):
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The unit is not in the inventory.")
			return
		case errors.Is(err, domain.ErrSystemdUnavailable):
			writeError(w, r, http.StatusServiceUnavailable, "SYSTEMD_UNAVAILABLE", "systemd cannot be reached.")
			return
		case err != nil:
			logger.Error("Could not read a system unit",
				zap.String("request_id", middleware.GetReqID(r.Context())), zap.String("unit", name), zap.Error(err))
			writeError(w, r, http.StatusInternalServerError, "SYSTEMD_READ_FAILED", "Could not read the unit.")
			return
		}
		unit := detail.Unit
		response := unitDetailDTO{
			Unit: toUnitDTO(unit.SystemUnit),
			Definition: unitDefinitionDTO{
				FragmentPath: optionalString(unit.FragmentPath), ExecStart: orEmpty(unit.ExecStart),
				Requires: orEmpty(unit.Requires), After: orEmpty(unit.After),
				Restart: optionalString(unit.Restart), Slice: optionalString(unit.Slice),
				TasksMax: toUnitLimitDTO(unit.TasksMax), LimitNOFILE: toUnitLimitDTO(unit.LimitNOFILE),
				MemoryMax: toUnitLimitDTO(unit.MemoryMax),
			},
			Journal: unitJournalDTO{Available: detail.JournalAvailable, Entries: make([]journalEntryDTO, 0, len(detail.Journal))},
		}
		for _, entry := range detail.Journal {
			response.Journal.Entries = append(response.Journal.Entries, toJournalEntryDTO(entry))
		}
		writeJSON(w, http.StatusOK, response)
	}
}
