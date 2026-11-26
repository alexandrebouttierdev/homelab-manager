package httpdelivery

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/usecase"
)

type journalEntryDTO struct {
	Cursor     string    `json:"cursor"`
	Time       time.Time `json:"time"`
	Priority   *int      `json:"priority"`
	Unit       string    `json:"unit"`
	Identifier string    `json:"identifier"`
	PID        string    `json:"pid"`
	Hostname   string    `json:"hostname"`
	BootID     string    `json:"bootId"`
	Message    string    `json:"message"`
	Truncated  bool      `json:"truncated"`
}

func toJournalEntryDTO(entry domain.JournalEntry) journalEntryDTO {
	return journalEntryDTO{
		Cursor: entry.Cursor, Time: entry.Time, Priority: entry.Priority, Unit: entry.Unit,
		Identifier: entry.Identifier, PID: entry.PID, Hostname: entry.Hostname,
		BootID: entry.BootID, Message: entry.Message, Truncated: entry.Truncated,
	}
}

type journalPageDTO struct {
	Entries          []journalEntryDTO `json:"entries"`
	OlderCursor      *string           `json:"olderCursor"`
	NewerCursor      *string           `json:"newerCursor"`
	Scanned          int               `json:"scanned"`
	ScanLimitReached bool              `json:"scanLimitReached"`
	Skipped          int               `json:"skipped"`
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

type journalFieldDTO struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Masked bool   `json:"masked"`
}

type journalBootDTO struct {
	Index      int       `json:"index"`
	BootID     string    `json:"bootId"`
	FirstEntry time.Time `json:"firstEntry"`
	LastEntry  time.Time `json:"lastEntry"`
}

type journalBucketDTO struct {
	Start  time.Time `json:"start"`
	Counts [8]int    `json:"counts"`
}

type journalSummaryDTO struct {
	Start          time.Time          `json:"start"`
	BucketMinutes  int                `json:"bucketMinutes"`
	Buckets        []journalBucketDTO `json:"buckets"`
	Total          int                `json:"total"`
	Truncated      bool               `json:"truncated"`
	DiskUsageBytes *int64             `json:"diskUsageBytes"`
	FullAccess     bool               `json:"fullAccess"`
	GeneratedAt    time.Time          `json:"generatedAt"`
}

// writeJournalError maps the journal errors; the detail of a journalctl
// failure goes to the Manager log only.
func writeJournalError(logger *zap.Logger, w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidJournalFilter):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The journal filter is invalid.")
	case errors.Is(err, domain.ErrJournalBootNotFound):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "The boot is not in the journal.")
	case errors.Is(err, domain.ErrJournalEntryNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "The journal entry does not exist.")
	case errors.Is(err, domain.ErrJournalUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "JOURNAL_UNAVAILABLE", "The system journal cannot be read.")
	default:
		logger.Error("Could not read the system journal",
			zap.String("request_id", middleware.GetReqID(r.Context())), zap.Error(err))
		writeError(w, r, http.StatusInternalServerError, "JOURNAL_READ_FAILED", "Could not read the system journal.")
	}
}

// journalQuery parses the query string strictly: url.Values drops a pair
// holding an unescaped ";", which every cursor contains.
func journalQuery(r *http.Request) (url.Values, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, domain.ErrInvalidJournalFilter
	}
	return query, nil
}

// journalFilter reads the filter query parameters; the domain validates
// them.
func journalFilter(query url.Values) (domain.JournalFilter, error) {
	filter := domain.JournalFilter{
		Source: domain.JournalSource(query.Get("source")), Unit: query.Get("unit"),
		BootID: query.Get("boot"), Search: query.Get("q"), Priority: domain.DefaultJournalPriority,
	}
	if name := query.Get("priority"); name != "" {
		priority, ok := domain.ParseJournalPriority(name)
		if !ok {
			return domain.JournalFilter{}, domain.ErrInvalidJournalFilter
		}
		filter.Priority = priority
	}
	for name, destination := range map[string]**time.Time{"from": &filter.From, "to": &filter.To} {
		if value := query.Get(name); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return domain.JournalFilter{}, domain.ErrInvalidJournalFilter
			}
			*destination = &parsed
		}
	}
	return filter, nil
}

func listJournalHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := journalQuery(r)
		var filter domain.JournalFilter
		if err == nil {
			filter, err = journalFilter(query)
		}
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		request := usecase.JournalQuery{Filter: filter, Before: query.Get("before"), After: query.Get("after")}
		if value := query.Get("limit"); value != "" {
			if request.Limit, err = strconv.Atoi(value); err != nil || request.Limit <= 0 {
				writeJournalError(logger, w, r, domain.ErrInvalidJournalFilter)
				return
			}
		}
		page, err := journal.List(r.Context(), request)
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		entries := make([]journalEntryDTO, 0, len(page.Entries))
		for _, entry := range page.Entries {
			entries = append(entries, toJournalEntryDTO(entry))
		}
		writeJSON(w, http.StatusOK, journalPageDTO{
			Entries: entries, OlderCursor: optionalString(page.OlderCursor), NewerCursor: optionalString(page.NewerCursor),
			Scanned: page.Scanned, ScanLimitReached: page.ScanLimitReached, Skipped: page.Skipped,
		})
	}
}

func journalEntryHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := journalQuery(r)
		var detail usecase.JournalEntryDetail
		if err == nil {
			detail, err = journal.Entry(r.Context(), query.Get("cursor"))
		}
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		fields := make([]journalFieldDTO, 0, len(detail.Fields))
		for _, field := range detail.Fields {
			fields = append(fields, journalFieldDTO{Name: field.Name, Value: field.Value, Masked: field.Masked})
		}
		writeJSON(w, http.StatusOK, map[string]any{"entry": toJournalEntryDTO(detail.Entry), "fields": fields})
	}
}

func journalBootsHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boots, err := journal.Boots(r.Context())
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		dtos := make([]journalBootDTO, 0, len(boots))
		for _, boot := range boots {
			dtos = append(dtos, journalBootDTO{Index: boot.Index, BootID: boot.ID, FirstEntry: boot.First, LastEntry: boot.Last})
		}
		writeJSON(w, http.StatusOK, map[string]any{"boots": dtos})
	}
}

func journalUnitsHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		units, err := journal.Units(r.Context())
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"units": units})
	}
}

func journalSummaryHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := journal.Summary(r.Context())
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		buckets := make([]journalBucketDTO, 0, domain.JournalSummaryBuckets)
		for i, counts := range summary.Counts {
			buckets = append(buckets, journalBucketDTO{Start: summary.Start.Add(time.Duration(i) * domain.JournalSummaryBucket), Counts: counts})
		}
		writeJSON(w, http.StatusOK, journalSummaryDTO{
			Start: summary.Start, BucketMinutes: int(domain.JournalSummaryBucket / time.Minute), Buckets: buckets,
			Total: summary.Total, Truncated: summary.Truncated, DiskUsageBytes: summary.DiskUsageBytes,
			FullAccess: summary.FullAccess, GeneratedAt: summary.GeneratedAt,
		})
	}
}

// exportJournalHandler streams the export: an error after the first byte
// can only end the response early, and is logged.
func exportJournalHandler(logger *zap.Logger, journal *usecase.JournalUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := journalQuery(r)
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		format := query.Get("format")
		if format != "log" && format != "json" {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "format must be log or json.")
			return
		}
		filter, err := journalFilter(query)
		if err == nil {
			filter, err = journal.CheckExport(r.Context(), filter)
		}
		if err != nil {
			writeJournalError(logger, w, r, err)
			return
		}
		name := "journal-" + time.Now().UTC().Format("20060102T150405Z") + "." + format
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		w.Header().Set("Cache-Control", "no-store")
		written := 0
		write := func(entry domain.JournalEntry) error {
			_, err := fmt.Fprintln(w, entry.ShortLine())
			return err
		}
		if format == "json" {
			w.Header().Set("Content-Type", "application/json")
			write = func(entry domain.JournalEntry) error {
				encoded, err := json.Marshal(toJournalEntryDTO(entry))
				if err != nil {
					return err
				}
				separator := ",\n"
				if written == 0 {
					separator = "[\n"
				}
				written++
				_, err = fmt.Fprintf(w, "%s%s", separator, encoded)
				return err
			}
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(http.StatusOK)
		export, err := journal.Export(r.Context(), filter, write)
		if err != nil {
			// The JSON stays unterminated, so that a cut export is not
			// mistaken for a complete one.
			logger.Error("Journal export ended early",
				zap.String("request_id", middleware.GetReqID(r.Context())), zap.Int("written", export.Written), zap.Error(err))
			return
		}
		if format == "json" {
			closing := "\n]\n"
			if written == 0 {
				closing = "[]\n"
			}
			_, _ = fmt.Fprint(w, closing)
		}
	}
}
