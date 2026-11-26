package httpdelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"
	"monoops/backend/internal/usecase"
)

const testBoot = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func testJournalCursor(i int) string {
	return fmt.Sprintf("s=%032x;i=%x;b=%s;m=1;t=1;x=1", 0, i+1, testBoot)
}

// escapedCursor is a cursor as the interface sends it in a query string.
func escapedCursor(i int) string { return url.QueryEscape(testJournalCursor(i)) }

// fakeJournal holds five entries in chronological order and records the
// last read.
type fakeJournal struct {
	records []domain.JournalRecord
	last    ports.JournalRead
}

func newFakeJournal() *fakeJournal {
	journal := &fakeJournal{}
	base := time.Now().Add(-time.Hour).UTC()
	for i := range 5 {
		journal.records = append(journal.records, domain.JournalRecord{
			"__CURSOR":             testJournalCursor(i),
			"__REALTIME_TIMESTAMP": strconv.FormatInt(base.Add(time.Duration(i)*time.Minute).UnixMicro(), 10),
			"PRIORITY":             strconv.Itoa(i + 2),
			"_SYSTEMD_UNIT":        "ssh.service",
			"SYSLOG_IDENTIFIER":    "sshd",
			"_PID":                 "42",
			"_HOSTNAME":            "monoops",
			"MESSAGE":              "message " + strconv.Itoa(i),
			"_CMDLINE":             "sshd --password=hunter2",
		})
	}
	return journal
}

func (f *fakeJournal) FullAccess() bool { return false }

func (f *fakeJournal) Read(_ context.Context, request ports.JournalRead, visit func(domain.JournalRecord) bool) (ports.JournalReadStats, error) {
	f.last = request
	records := slices.Clone(f.records)
	if request.Reverse {
		slices.Reverse(records)
	}
	if request.After != "" {
		position := slices.IndexFunc(records, func(r domain.JournalRecord) bool { return r["__CURSOR"] == request.After })
		records = records[position+1:]
	}
	for i, record := range records {
		if i == request.Limit || !visit(record) {
			break
		}
	}
	return ports.JournalReadStats{}, nil
}

func (f *fakeJournal) Entry(_ context.Context, cursor string) (domain.JournalRecord, error) {
	for _, record := range f.records {
		if record["__CURSOR"] == cursor {
			return record, nil
		}
	}
	return nil, domain.ErrJournalEntryNotFound
}

func (f *fakeJournal) Boots(context.Context) ([]domain.JournalBoot, error) {
	return []domain.JournalBoot{{Index: 0, ID: testBoot, First: time.Unix(0, 0), Last: time.Unix(60, 0)}}, nil
}

func (f *fakeJournal) Units(context.Context) ([]string, error) { return []string{"ssh.service"}, nil }

func (f *fakeJournal) DiskUsage(context.Context) (int64, error) { return 8 << 20, nil }

func newJournalRouter(t *testing.T) (http.Handler, *fakeJournal) {
	t.Helper()
	services, _ := newTestServices(t, nil)
	journal := newFakeJournal()
	services.Journal = usecase.NewJournalUsecase(journal)
	return NewRouter(zap.NewNop(), services, false, ""), journal
}

type testJournalPage struct {
	Entries []struct {
		Cursor     string  `json:"cursor"`
		Time       string  `json:"time"`
		Priority   *int    `json:"priority"`
		Unit       string  `json:"unit"`
		Identifier string  `json:"identifier"`
		Message    string  `json:"message"`
		Truncated  bool    `json:"truncated"`
		BootID     string  `json:"bootId"`
		Hostname   string  `json:"hostname"`
		PID        string  `json:"pid"`
		Extra      *string `json:"_CMDLINE"`
	} `json:"entries"`
	OlderCursor      *string `json:"olderCursor"`
	NewerCursor      *string `json:"newerCursor"`
	Scanned          int     `json:"scanned"`
	ScanLimitReached bool    `json:"scanLimitReached"`
}

func TestJournalRoutesForEveryRole(t *testing.T) {
	resetLoginRateLimit(t)
	router, journal := newJournalRouter(t)
	if response := call(router, http.MethodGet, "/api/v1/journal/entries", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", response.Code)
	}
	admin := signIn(t, router, "admin", "admin-password-1")
	if response := call(router, http.MethodPost, "/api/v1/users", `{"username":"vera","displayName":"Vera","role":"viewer","password":"initial-password-1"}`, admin); response.Code != http.StatusCreated {
		t.Fatalf("create viewer = %d %s", response.Code, response.Body)
	}
	vera := signIn(t, router, "vera", "initial-password-1")
	if response := call(router, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"initial-password-1","newPassword":"vera-own-password"}`, vera); response.Code != http.StatusOK {
		t.Fatalf("viewer password = %d", response.Code)
	}
	vera = signIn(t, router, "vera", "vera-own-password")

	response := call(router, http.MethodGet, "/api/v1/journal/entries?priority=debug&limit=2&source=all", "", vera)
	if response.Code != http.StatusOK {
		t.Fatalf("viewer entries = %d %s", response.Code, response.Body)
	}
	var page testJournalPage
	decodeBody(t, response, &page)
	if len(page.Entries) != 2 || page.Entries[0].Message != "message 4" || page.Entries[0].Unit != "ssh.service" || page.Entries[0].Identifier != "sshd" {
		t.Fatalf("page = %+v", page.Entries)
	}
	if page.Entries[0].Priority == nil || *page.Entries[0].Priority != 6 || page.Entries[0].BootID != "" {
		t.Fatalf("entry priority %v boot %q (the fake journal has no boot field)", page.Entries[0].Priority, page.Entries[0].BootID)
	}
	if page.OlderCursor == nil || *page.OlderCursor != testJournalCursor(3) || page.NewerCursor == nil || *page.NewerCursor != testJournalCursor(4) {
		t.Fatalf("cursors = %v %v", page.OlderCursor, page.NewerCursor)
	}
	if strings.Contains(response.Body.String(), "hunter2") {
		t.Fatal("the command line leaked into the list")
	}
	if journal.last.Filter.Priority != 7 || !journal.last.Reverse || journal.last.Limit != 3 {
		t.Fatalf("read = %+v", journal.last)
	}

	response = call(router, http.MethodGet, "/api/v1/journal/entries?after="+escapedCursor(2), "", vera)
	decodeBody(t, response, &page)
	if response.Code != http.StatusOK || len(page.Entries) != 2 || page.Entries[0].Message != "message 4" || page.OlderCursor != nil {
		t.Fatalf("follow = %d %+v older %v", response.Code, page.Entries, page.OlderCursor)
	}
	if journal.last.Filter.Priority != domain.DefaultJournalPriority {
		t.Fatalf("default priority = %d, want info", journal.last.Filter.Priority)
	}

	response = call(router, http.MethodGet, "/api/v1/journal/entry?cursor="+escapedCursor(1), "", vera)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "hunter2") || !strings.Contains(response.Body.String(), `"name":"_CMDLINE","value":"[masqué]","masked":true`) {
		t.Fatalf("entry = %d %s", response.Code, response.Body)
	}
	if response := call(router, http.MethodGet, "/api/v1/journal/entry?cursor="+escapedCursor(9), "", vera); response.Code != http.StatusNotFound {
		t.Fatalf("unknown entry = %d", response.Code)
	}

	response = call(router, http.MethodGet, "/api/v1/journal/boots", "", vera)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"bootId":"`+testBoot+`"`) {
		t.Fatalf("boots = %d %s", response.Code, response.Body)
	}
	response = call(router, http.MethodGet, "/api/v1/journal/units", "", vera)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"units":["ssh.service"]}` {
		t.Fatalf("units = %d %s", response.Code, response.Body)
	}

	response = call(router, http.MethodGet, "/api/v1/journal/summary", "", vera)
	var summary struct {
		BucketMinutes  int                       `json:"bucketMinutes"`
		Buckets        []struct{ Counts [8]int } `json:"buckets"`
		Total          int                       `json:"total"`
		DiskUsageBytes *int64                    `json:"diskUsageBytes"`
		FullAccess     bool                      `json:"fullAccess"`
	}
	decodeBody(t, response, &summary)
	if response.Code != http.StatusOK || summary.BucketMinutes != 15 || len(summary.Buckets) != 96 || summary.Total != 5 || summary.DiskUsageBytes == nil || summary.FullAccess {
		t.Fatalf("summary = %d %+v", response.Code, summary)
	}
}

func TestJournalParametersAreValidated(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newJournalRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, query := range []string{
		"priority=error", "priority=7", "source=everything", "unit=ssh", "unit=--all.service",
		"boot=-1", "boot=" + strings.Repeat("c", 32), "from=yesterday", "from=2026-11-23T10:00:00Z&to=2026-11-23T09:00:00Z",
		"limit=0", "limit=501", "limit=ten", "before=s=1", "before=" + escapedCursor(1) + "&after=" + escapedCursor(0),
		// An unescaped cursor is refused, not silently dropped.
		"after=" + testJournalCursor(0),
		"q=" + strings.Repeat("x", 201), "source=docker&unit=ssh.service",
	} {
		response := call(router, http.MethodGet, "/api/v1/journal/entries?"+query, "", admin)
		if response.Code != http.StatusBadRequest || errorCode(t, response) != "VALIDATION_ERROR" {
			t.Fatalf("%s = %d %s", query, response.Code, response.Body)
		}
	}
	if response := call(router, http.MethodGet, "/api/v1/journal/entry?cursor=x", "", admin); response.Code != http.StatusBadRequest {
		t.Fatalf("malformed cursor = %d", response.Code)
	}
	if response := call(router, http.MethodGet, "/api/v1/journal/export?format=csv", "", admin); response.Code != http.StatusBadRequest {
		t.Fatalf("export format = %d", response.Code)
	}
	if response := call(router, http.MethodPost, "/api/v1/journal/entries", "", admin); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d, the journal is read-only", response.Code)
	}
}

func TestJournalExport(t *testing.T) {
	resetLoginRateLimit(t)
	router, journal := newJournalRouter(t)
	admin := signIn(t, router, "admin", "admin-password-1")

	response := call(router, http.MethodGet, "/api/v1/journal/export?format=log&priority=debug&q=MESSAGE%203", "", admin)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Disposition"), `attachment; filename="journal-`) || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("log export = %d %v", response.Code, response.Header())
	}
	lines := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	if len(lines) != 1 || !strings.HasSuffix(lines[0], " monoops sshd[42]: message 3") {
		t.Fatalf("log lines = %q", lines)
	}
	if journal.last.Reverse || journal.last.Limit != domain.MaxJournalExport {
		t.Fatalf("export read = %+v, want the most recent entries in order", journal.last)
	}

	response = call(router, http.MethodGet, "/api/v1/journal/export?format=json&priority=debug", "", admin)
	var entries []struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &entries); err != nil || len(entries) != 5 || entries[0].Message != "message 0" {
		t.Fatalf("json export = %v %s", err, response.Body)
	}
	if strings.Contains(response.Body.String(), "hunter2") {
		t.Fatal("the command line leaked into the export")
	}
	response = call(router, http.MethodGet, "/api/v1/journal/export?format=json&q=nothing", "", admin)
	if strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("empty json export = %q", response.Body)
	}
}

func TestJournalUnavailableWithoutJournalctl(t *testing.T) {
	resetLoginRateLimit(t)
	router, _ := newTestRouter(t, nil)
	admin := signIn(t, router, "admin", "admin-password-1")
	for _, path := range []string{"/api/v1/journal/entries", "/api/v1/journal/summary", "/api/v1/journal/export?format=log"} {
		response := call(router, http.MethodGet, path, "", admin)
		if response.Code != http.StatusServiceUnavailable || errorCode(t, response) != "JOURNAL_UNAVAILABLE" {
			t.Fatalf("%s = %d %s", path, response.Code, response.Body)
		}
	}
}
