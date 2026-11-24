package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const journalBoot = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

var journalBase = time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)

func journalCursor(i int) string {
	return fmt.Sprintf("s=%032x;i=%x;b=%s;m=1;t=1;x=1", 0, i+1, journalBoot)
}

// memoryJournal stands for journalctl: records in chronological order,
// read in either direction from a cursor.
type memoryJournal struct {
	records    []domain.JournalRecord
	reads      []ports.JournalRead
	diskErr    error
	truncateAt int
}

func newMemoryJournal(count int, message func(i int) string) *memoryJournal {
	journal := &memoryJournal{}
	for i := range count {
		journal.records = append(journal.records, domain.JournalRecord{
			"__CURSOR":             journalCursor(i),
			"__REALTIME_TIMESTAMP": strconv.FormatInt(journalBase.Add(time.Duration(i)*time.Minute).UnixMicro(), 10),
			"PRIORITY":             strconv.Itoa(i % 8),
			"_SYSTEMD_UNIT":        "unit" + strconv.Itoa(i%2) + ".service",
			"MESSAGE":              message(i),
			"API_TOKEN":            "secret-" + strconv.Itoa(i),
		})
	}
	return journal
}

func (m *memoryJournal) FullAccess() bool { return true }

func (m *memoryJournal) Read(_ context.Context, request ports.JournalRead, visit func(domain.JournalRecord) bool) (ports.JournalReadStats, error) {
	m.reads = append(m.reads, request)
	position := slices.IndexFunc(m.records, func(r domain.JournalRecord) bool { return r["__CURSOR"] == request.After })
	var selected []domain.JournalRecord
	for i, record := range m.records {
		priority, _ := strconv.Atoi(record["PRIORITY"])
		if priority > request.Filter.Priority || (request.Filter.Unit != "" && record["_SYSTEMD_UNIT"] != request.Filter.Unit) {
			continue
		}
		if request.After != "" && ((request.Reverse && i >= position) || (!request.Reverse && i <= position)) {
			continue
		}
		selected = append(selected, record)
	}
	switch {
	case request.Reverse:
		slices.Reverse(selected)
	case request.After == "":
		selected = selected[max(0, len(selected)-request.Limit):]
	}
	var stats ports.JournalReadStats
	for i, record := range selected {
		if i == request.Limit {
			break
		}
		if m.truncateAt > 0 && i == m.truncateAt {
			stats.Truncated = true
			break
		}
		if !visit(record) {
			break
		}
	}
	return stats, nil
}

func (m *memoryJournal) Entry(_ context.Context, cursor string) (domain.JournalRecord, error) {
	for _, record := range m.records {
		if record["__CURSOR"] == cursor {
			return record, nil
		}
	}
	return nil, domain.ErrJournalEntryNotFound
}

func (m *memoryJournal) Boots(context.Context) ([]domain.JournalBoot, error) {
	return []domain.JournalBoot{{Index: 0, ID: journalBoot}}, nil
}

func (m *memoryJournal) Units(context.Context) ([]string, error) {
	return []string{"unit0.service", "unit1.service"}, nil
}

func (m *memoryJournal) DiskUsage(context.Context) (int64, error) { return 4096, m.diskErr }

func numbered(i int) string { return "message " + strconv.Itoa(i) }

func entryNumbers(entries []domain.JournalEntry) []int {
	numbers := make([]int, 0, len(entries))
	for _, entry := range entries {
		n, _ := strconv.Atoi(strings.Fields(entry.Message)[1])
		numbers = append(numbers, n)
	}
	return numbers
}

func span(from, to int) []int {
	var numbers []int
	for i := from; ; {
		numbers = append(numbers, i)
		if i == to {
			return numbers
		}
		if from < to {
			i++
		} else {
			i--
		}
	}
}

func allPriorities() domain.JournalFilter { return domain.JournalFilter{Priority: 7} }

func TestJournalPagesGoBackToTheFirstEntry(t *testing.T) {
	journal := newMemoryJournal(30, numbered)
	journals := NewJournalUsecase(journal)
	ctx := context.Background()

	page, err := journals.List(ctx, JournalQuery{Filter: allPriorities(), Limit: 10})
	if err != nil || !slices.Equal(entryNumbers(page.Entries), span(29, 20)) {
		t.Fatalf("newest page = %v, %v", entryNumbers(page.Entries), err)
	}
	if page.OlderCursor != journalCursor(20) || page.NewerCursor != journalCursor(29) || page.ScanLimitReached {
		t.Fatalf("cursors = older %q newer %q, limit %v", page.OlderCursor, page.NewerCursor, page.ScanLimitReached)
	}
	if got := journal.reads[0]; !got.Reverse || got.Limit != 11 || !slices.Equal(got.Fields, domain.JournalListFields) {
		t.Fatalf("read = %+v, want the newest 11 entries with the list fields", got)
	}

	page, _ = journals.List(ctx, JournalQuery{Filter: allPriorities(), Limit: 10, Before: page.OlderCursor})
	if !slices.Equal(entryNumbers(page.Entries), span(19, 10)) || page.NewerCursor != "" {
		t.Fatalf("second page = %v, newer %q", entryNumbers(page.Entries), page.NewerCursor)
	}
	page, _ = journals.List(ctx, JournalQuery{Filter: allPriorities(), Limit: 10, Before: page.OlderCursor})
	if !slices.Equal(entryNumbers(page.Entries), span(9, 0)) || page.OlderCursor != "" {
		t.Fatalf("last page = %v, older %q: no older entries remain", entryNumbers(page.Entries), page.OlderCursor)
	}
}

func TestJournalFollowReturnsNewerEntries(t *testing.T) {
	journals := NewJournalUsecase(newMemoryJournal(30, numbered))
	ctx := context.Background()

	page, err := journals.List(ctx, JournalQuery{Filter: allPriorities(), After: journalCursor(25)})
	if err != nil || !slices.Equal(entryNumbers(page.Entries), span(29, 26)) || page.NewerCursor != journalCursor(29) || page.OlderCursor != "" {
		t.Fatalf("follow = %v newer %q older %q, %v", entryNumbers(page.Entries), page.NewerCursor, page.OlderCursor, err)
	}
	page, _ = journals.List(ctx, JournalQuery{Filter: allPriorities(), After: journalCursor(29)})
	if len(page.Entries) != 0 || page.NewerCursor != journalCursor(29) {
		t.Fatalf("nothing new = %v, newer %q: the cursor is kept", entryNumbers(page.Entries), page.NewerCursor)
	}
	page, _ = journals.List(ctx, JournalQuery{Filter: allPriorities(), After: journalCursor(5), Limit: 3})
	if !slices.Equal(entryNumbers(page.Entries), span(8, 6)) || page.NewerCursor != journalCursor(8) {
		t.Fatalf("follow with a backlog = %v, newer %q: the next poll continues", entryNumbers(page.Entries), page.NewerCursor)
	}
}

func TestJournalSearchIsBoundedAndContinues(t *testing.T) {
	journal := newMemoryJournal(domain.MaxJournalScan+1000, func(i int) string {
		if i%1000 == 0 {
			return "message " + strconv.Itoa(i) + " NEEDLE"
		}
		return numbered(i)
	})
	journals := NewJournalUsecase(journal)
	ctx := context.Background()
	filter := allPriorities()
	filter.Search = "needle"

	page, err := journals.List(ctx, JournalQuery{Filter: filter})
	if err != nil || !slices.Equal(entryNumbers(page.Entries), []int{5000, 4000, 3000, 2000, 1000}) {
		t.Fatalf("first search page = %v, %v", entryNumbers(page.Entries), err)
	}
	if !page.ScanLimitReached || page.Scanned != domain.MaxJournalScan || page.OlderCursor != journalCursor(1000) {
		t.Fatalf("budget: reached %v scanned %d older %q", page.ScanLimitReached, page.Scanned, page.OlderCursor)
	}
	page, _ = journals.List(ctx, JournalQuery{Filter: filter, Before: page.OlderCursor})
	if !slices.Equal(entryNumbers(page.Entries), []int{0}) || page.OlderCursor != "" || page.ScanLimitReached {
		t.Fatalf("continued search = %v older %q reached %v", entryNumbers(page.Entries), page.OlderCursor, page.ScanLimitReached)
	}
}

func TestJournalSearchStopsAtTheLimit(t *testing.T) {
	journals := NewJournalUsecase(newMemoryJournal(30, func(i int) string {
		if i%3 == 0 {
			return "message " + strconv.Itoa(i) + " match"
		}
		return numbered(i)
	}))
	filter := allPriorities()
	filter.Search = "MATCH"
	page, err := journals.List(context.Background(), JournalQuery{Filter: filter, Limit: 2})
	if err != nil || !slices.Equal(entryNumbers(page.Entries), []int{27, 24}) || page.ScanLimitReached {
		t.Fatalf("page = %v reached %v, %v", entryNumbers(page.Entries), page.ScanLimitReached, err)
	}
	// Entries 23 and 22 were examined; 21, the next match, was not.
	if page.OlderCursor != journalCursor(22) {
		t.Fatalf("older = %q, want the last entry examined", page.OlderCursor)
	}
}

func TestJournalTruncatedOutputIsReported(t *testing.T) {
	journal := newMemoryJournal(30, numbered)
	journal.truncateAt = 4
	page, err := NewJournalUsecase(journal).List(context.Background(), JournalQuery{Filter: allPriorities(), Limit: 10})
	if err != nil || len(page.Entries) != 4 || !page.ScanLimitReached || page.OlderCursor != journalCursor(26) {
		t.Fatalf("page = %v reached %v older %q, %v", entryNumbers(page.Entries), page.ScanLimitReached, page.OlderCursor, err)
	}
}

func TestJournalQueryValidation(t *testing.T) {
	journals := NewJournalUsecase(newMemoryJournal(3, numbered))
	ctx := context.Background()
	for name, query := range map[string]JournalQuery{
		"before and after": {Filter: allPriorities(), Before: journalCursor(1), After: journalCursor(0)},
		"malformed cursor": {Filter: allPriorities(), Before: "s=1"},
		"limit too high":   {Filter: allPriorities(), Limit: domain.MaxJournalLimit + 1},
		"invalid filter":   {Filter: domain.JournalFilter{Priority: 9}},
	} {
		if _, err := journals.List(ctx, query); !errors.Is(err, domain.ErrInvalidJournalFilter) {
			t.Fatalf("%s = %v, want ErrInvalidJournalFilter", name, err)
		}
	}
	filter := allPriorities()
	filter.BootID = strings.Repeat("c", 32)
	if _, err := journals.List(ctx, JournalQuery{Filter: filter}); !errors.Is(err, domain.ErrJournalBootNotFound) {
		t.Fatalf("unknown boot = %v", err)
	}
	filter.BootID = journalBoot
	if _, err := journals.List(ctx, JournalQuery{Filter: filter}); err != nil {
		t.Fatalf("known boot = %v", err)
	}
	if _, err := NewJournalUsecase(nil).List(ctx, JournalQuery{Filter: allPriorities()}); !errors.Is(err, domain.ErrJournalUnavailable) {
		t.Fatalf("without journalctl = %v", err)
	}
}

func TestJournalEntryIsMasked(t *testing.T) {
	journals := NewJournalUsecase(newMemoryJournal(3, numbered))
	detail, err := journals.Entry(context.Background(), journalCursor(1))
	if err != nil || detail.Entry.Message != "message 1" {
		t.Fatalf("detail = %+v, %v", detail.Entry, err)
	}
	for _, field := range detail.Fields {
		if strings.HasPrefix(field.Value, "secret-") {
			t.Fatalf("field %s leaked its value", field.Name)
		}
	}
	if _, err := journals.Entry(context.Background(), journalCursor(9)); !errors.Is(err, domain.ErrJournalEntryNotFound) {
		t.Fatalf("unknown entry = %v", err)
	}
	if _, err := journals.Entry(context.Background(), "--cursor=x"); !errors.Is(err, domain.ErrInvalidJournalFilter) {
		t.Fatalf("malformed cursor = %v", err)
	}
}

func TestJournalSummaryIsCachedForAMinute(t *testing.T) {
	journal := newMemoryJournal(30, numbered)
	journals := NewJournalUsecase(journal)
	now := journalBase.Add(40 * time.Minute)
	journals.now = func() time.Time { return now }
	ctx := context.Background()

	summary, err := journals.Summary(ctx)
	if err != nil || summary.Total != 30 || summary.DiskUsageBytes == nil || *summary.DiskUsageBytes != 4096 || !summary.FullAccess || summary.Truncated {
		t.Fatalf("summary = total %d disk %v full %v truncated %v, %v", summary.Total, summary.DiskUsageBytes, summary.FullAccess, summary.Truncated, err)
	}
	read := journal.reads[0]
	if read.Reverse || read.Limit != domain.MaxJournalSummaryEntries || read.Filter.From == nil || !read.Filter.From.Equal(summary.Start) || !slices.Equal(read.Fields, []string{"PRIORITY"}) {
		t.Fatalf("summary read = %+v", read)
	}
	if _, err := journals.Summary(ctx); err != nil || len(journal.reads) != 1 {
		t.Fatalf("second call read the journal again (%d reads)", len(journal.reads))
	}
	now = now.Add(time.Minute)
	journal.diskErr = errors.New("journalctl failed")
	summary, err = journals.Summary(ctx)
	if err != nil || len(journal.reads) != 2 || summary.DiskUsageBytes != nil {
		t.Fatalf("after a minute: %d reads, disk %v, %v", len(journal.reads), summary.DiskUsageBytes, err)
	}
}

func TestJournalExportIsChronologicalAndSearched(t *testing.T) {
	journals := NewJournalUsecase(newMemoryJournal(30, numbered))
	ctx := context.Background()
	filter, err := journals.CheckExport(ctx, domain.JournalFilter{Priority: 7, Search: "message 2"})
	if err != nil {
		t.Fatal(err)
	}
	var written []domain.JournalEntry
	export, err := journals.Export(ctx, filter, func(entry domain.JournalEntry) error {
		written = append(written, entry)
		return nil
	})
	if err != nil || export.Written != 11 || export.Truncated || !slices.Equal(entryNumbers(written)[:3], []int{2, 20, 21}) {
		t.Fatalf("export = %+v %v, %v", export, entryNumbers(written), err)
	}
	failure := errors.New("client gone")
	export, err = journals.Export(ctx, filter, func(domain.JournalEntry) error { return failure })
	if !errors.Is(err, failure) || export.Written != 0 {
		t.Fatalf("write failure = %+v, %v", export, err)
	}
	if _, err := journals.CheckExport(ctx, domain.JournalFilter{Priority: 7, BootID: strings.Repeat("d", 32)}); !errors.Is(err, domain.ErrJournalBootNotFound) {
		t.Fatalf("export of an unknown boot = %v", err)
	}
}
