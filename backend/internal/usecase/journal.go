package usecase

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
)

// journalCacheLifetime bounds how often the summary and the boot list are
// computed again.
const journalCacheLifetime = time.Minute

// JournalUsecase consults the systemd journal, read-only
// (SPEC_ADMINISTRATION.md § 4). Every role may use it.
type JournalUsecase struct {
	// reader is nil when journalctl was not found.
	reader ports.JournalReader
	now    func() time.Time

	summaryMu sync.Mutex
	summary   *domain.JournalSummary

	bootsMu sync.Mutex
	boots   []domain.JournalBoot
	bootsAt time.Time
}

func NewJournalUsecase(reader ports.JournalReader) *JournalUsecase {
	return &JournalUsecase{reader: reader, now: time.Now}
}

// JournalQuery is one page request: the newest entries, the entries older
// than Before, or the entries newer than After (live follow).
type JournalQuery struct {
	Filter domain.JournalFilter
	Before string
	After  string
	Limit  int
}

// JournalPage holds entries from the newest to the oldest.
type JournalPage struct {
	Entries []domain.JournalEntry
	// OlderCursor continues towards older entries; empty when there are
	// none. Not set for a follow page.
	OlderCursor string
	// NewerCursor is the cursor to follow from; empty when the page read
	// nothing and no After was given. Not set for an older page.
	NewerCursor string
	Scanned     int
	// ScanLimitReached is set when the read stopped at its bound (text
	// search budget or output size) before the end of the journal.
	ScanLimitReached bool
	Skipped          int
}

func (u *JournalUsecase) available() error {
	if u.reader == nil {
		return domain.ErrJournalUnavailable
	}
	return nil
}

func (q JournalQuery) validate() (JournalQuery, error) {
	filter, err := q.Filter.Validate()
	if err != nil {
		return JournalQuery{}, err
	}
	q.Filter = filter
	if q.Limit == 0 {
		q.Limit = domain.DefaultJournalLimit
	}
	switch {
	case q.Limit < 0 || q.Limit > domain.MaxJournalLimit,
		q.Before != "" && q.After != "",
		q.Before != "" && !domain.ValidJournalCursor(q.Before),
		q.After != "" && !domain.ValidJournalCursor(q.After):
		return JournalQuery{}, domain.ErrInvalidJournalFilter
	}
	return q, nil
}

// List reads one page. Without text search it reads one entry more than
// the limit, to know whether older entries remain; with a search it
// examines at most domain.MaxJournalScan entries.
func (u *JournalUsecase) List(ctx context.Context, query JournalQuery) (JournalPage, error) {
	if err := u.available(); err != nil {
		return JournalPage{}, err
	}
	query, err := query.validate()
	if err != nil {
		return JournalPage{}, err
	}
	if err := u.checkBoot(ctx, query.Filter.BootID); err != nil {
		return JournalPage{}, err
	}
	follow := query.After != ""
	budget := query.Limit + 1
	if query.Filter.Search != "" {
		budget = domain.MaxJournalScan
	}
	request := ports.JournalRead{
		Filter: query.Filter, Reverse: !follow, After: query.After + query.Before,
		Limit: budget, Fields: domain.JournalListFields,
	}

	var page JournalPage
	more := false
	lastScanned, firstScanned := "", ""
	stats, err := u.reader.Read(ctx, request, func(record domain.JournalRecord) bool {
		entry, ok := domain.JournalEntryFrom(record)
		if !ok {
			page.Skipped++
			return true
		}
		page.Scanned++
		if query.Filter.Matches(entry) {
			if len(page.Entries) == query.Limit {
				more = true
				return false
			}
			page.Entries = append(page.Entries, entry)
		}
		if firstScanned == "" {
			firstScanned = entry.Cursor
		}
		lastScanned = entry.Cursor
		return true
	})
	if err != nil {
		return JournalPage{}, err
	}
	page.Skipped += stats.Skipped
	exhausted := !more && !stats.Truncated && page.Scanned+page.Skipped < budget
	page.ScanLimitReached = !more && !exhausted

	// The continuation cursor is the last entry examined: an entry passed
	// over by the search is not read again.
	continuation := ""
	if !exhausted {
		continuation = lastScanned
	}
	if follow {
		slices.Reverse(page.Entries)
		page.NewerCursor = query.After
		if lastScanned != "" {
			page.NewerCursor = lastScanned
		}
		return page, nil
	}
	page.OlderCursor = continuation
	if query.Before == "" {
		page.NewerCursor = firstScanned
	}
	return page, nil
}

// JournalEntryDetail is the inspector view of one entry.
type JournalEntryDetail struct {
	Entry  domain.JournalEntry
	Fields []domain.JournalField
}

// Entry returns every field of one entry, sensitive ones masked.
func (u *JournalUsecase) Entry(ctx context.Context, cursor string) (JournalEntryDetail, error) {
	if err := u.available(); err != nil {
		return JournalEntryDetail{}, err
	}
	if !domain.ValidJournalCursor(cursor) {
		return JournalEntryDetail{}, domain.ErrInvalidJournalFilter
	}
	record, err := u.reader.Entry(ctx, cursor)
	if err != nil {
		return JournalEntryDetail{}, err
	}
	entry, ok := domain.JournalEntryFrom(domain.MaskJournalRecord(record))
	if !ok {
		return JournalEntryDetail{}, domain.ErrJournalEntryNotFound
	}
	return JournalEntryDetail{Entry: entry, Fields: domain.JournalFields(record)}, nil
}

// Boots lists the recorded boots, the current one first; the list is kept
// for a minute.
func (u *JournalUsecase) Boots(ctx context.Context) ([]domain.JournalBoot, error) {
	if err := u.available(); err != nil {
		return nil, err
	}
	u.bootsMu.Lock()
	defer u.bootsMu.Unlock()
	now := u.now()
	if u.boots != nil && now.Sub(u.bootsAt) < journalCacheLifetime {
		return u.boots, nil
	}
	boots, err := u.reader.Boots(ctx)
	if err != nil {
		return nil, err
	}
	if boots == nil {
		boots = []domain.JournalBoot{}
	}
	u.boots, u.bootsAt = boots, now
	return boots, nil
}

// checkBoot refuses a boot the journal does not know: journalctl would
// fail on it.
func (u *JournalUsecase) checkBoot(ctx context.Context, bootID string) error {
	if bootID == "" {
		return nil
	}
	boots, err := u.Boots(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(boots, func(boot domain.JournalBoot) bool { return boot.ID == bootID }) {
		return domain.ErrJournalBootNotFound
	}
	return nil
}

// Units lists the units present in the journal.
func (u *JournalUsecase) Units(ctx context.Context) ([]string, error) {
	if err := u.available(); err != nil {
		return nil, err
	}
	units, err := u.reader.Units(ctx)
	if units == nil {
		units = []string{}
	}
	return units, err
}

// Summary counts the entries of the last 24 hours by priority and reads
// the journal disk usage; the result is kept for a minute.
func (u *JournalUsecase) Summary(ctx context.Context) (domain.JournalSummary, error) {
	if err := u.available(); err != nil {
		return domain.JournalSummary{}, err
	}
	u.summaryMu.Lock()
	defer u.summaryMu.Unlock()
	now := u.now()
	if u.summary != nil && now.Sub(u.summary.GeneratedAt) < journalCacheLifetime {
		return *u.summary, nil
	}
	summary := domain.NewJournalSummary(now)
	start := summary.Start
	request := ports.JournalRead{
		Filter: domain.JournalFilter{Source: domain.JournalSourceAll, Priority: len(domain.JournalPriorities) - 1, From: &start},
		Limit:  domain.MaxJournalSummaryEntries, Fields: []string{"PRIORITY"},
	}
	read := 0
	stats, err := u.reader.Read(ctx, request, func(record domain.JournalRecord) bool {
		read++
		micros, err := strconv.ParseInt(record["__REALTIME_TIMESTAMP"], 10, 64)
		if err != nil {
			return true
		}
		priority, err := strconv.Atoi(record["PRIORITY"])
		if err != nil {
			priority = -1
		}
		summary.Add(time.UnixMicro(micros), priority)
		return true
	})
	if err != nil {
		return domain.JournalSummary{}, err
	}
	summary.Truncated = stats.Truncated || read+stats.Skipped >= domain.MaxJournalSummaryEntries
	summary.FullAccess = u.reader.FullAccess()
	if usage, err := u.reader.DiskUsage(ctx); err == nil {
		summary.DiskUsageBytes = &usage
	} else if errors.Is(err, context.Canceled) {
		return domain.JournalSummary{}, err
	}
	u.summary = &summary
	return summary, nil
}

// JournalExport tells what an export wrote.
type JournalExport struct {
	Written int
	// Truncated is set when the filter matched more than
	// domain.MaxJournalExport entries: only the most recent are exported.
	Truncated bool
}

// CheckExport validates an export filter before the response starts.
func (u *JournalUsecase) CheckExport(ctx context.Context, filter domain.JournalFilter) (domain.JournalFilter, error) {
	if err := u.available(); err != nil {
		return domain.JournalFilter{}, err
	}
	filter, err := filter.Validate()
	if err != nil {
		return domain.JournalFilter{}, err
	}
	return filter, u.checkBoot(ctx, filter.BootID)
}

// Export gives the most recent domain.MaxJournalExport entries of the
// filter in chronological order; the text search applies to them. The
// filter must come from CheckExport.
func (u *JournalUsecase) Export(ctx context.Context, filter domain.JournalFilter, write func(domain.JournalEntry) error) (JournalExport, error) {
	if err := u.available(); err != nil {
		return JournalExport{}, err
	}
	var export JournalExport
	var writeErr error
	read := 0
	request := ports.JournalRead{Filter: filter, Limit: domain.MaxJournalExport, Fields: domain.JournalListFields}
	stats, err := u.reader.Read(ctx, request, func(record domain.JournalRecord) bool {
		read++
		entry, ok := domain.JournalEntryFrom(record)
		if !ok || !filter.Matches(entry) {
			return true
		}
		if writeErr = write(entry); writeErr != nil {
			return false
		}
		export.Written++
		return true
	})
	if writeErr != nil {
		return export, writeErr
	}
	export.Truncated = stats.Truncated || read+stats.Skipped >= domain.MaxJournalExport
	return export, err
}
