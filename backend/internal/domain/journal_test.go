package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testCursor = "s=7019c775012c48e3983412859bc0cd93;i=1e;b=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb;m=fb388;t=660e88af11b00;x=ce9aaf6266ac73cd"

func TestJournalFilterValidation(t *testing.T) {
	from := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	cases := []struct {
		name   string
		filter JournalFilter
		valid  bool
	}{
		{"defaults", JournalFilter{Priority: DefaultJournalPriority}, true},
		{"unit and range", JournalFilter{Unit: "ssh.service", Priority: 3, From: &from, To: &to}, true},
		{"template instance and scope", JournalFilter{Unit: "getty@tty1.service"}, true},
		{"user scope", JournalFilter{Unit: "session-12.scope"}, true},
		{"kernel source", JournalFilter{Source: JournalSourceKernel}, true},
		{"boot", JournalFilter{BootID: strings.Repeat("a", 32)}, true},
		{"unknown source", JournalFilter{Source: "everything"}, false},
		{"priority too high", JournalFilter{Priority: 8}, false},
		{"negative priority", JournalFilter{Priority: -1}, false},
		{"unit without type", JournalFilter{Unit: "ssh"}, false},
		{"unit as an option", JournalFilter{Unit: "--output=x.service"}, false},
		{"unit with a space", JournalFilter{Unit: "a b.service"}, false},
		{"unit with a glob", JournalFilter{Unit: "*.service"}, false},
		{"docker source with unit", JournalFilter{Source: JournalSourceDocker, Unit: "ssh.service"}, false},
		{"boot in upper case", JournalFilter{BootID: strings.Repeat("A", 32)}, false},
		{"boot as an offset", JournalFilter{BootID: "-1"}, false},
		{"empty range", JournalFilter{From: &from, To: &from}, false},
		{"search too long", JournalFilter{Search: strings.Repeat("x", 201)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.filter.Validate()
			if c.valid != (err == nil) {
				t.Fatalf("Validate() error = %v, want valid %v", err, c.valid)
			}
			if !c.valid && !errors.Is(err, ErrInvalidJournalFilter) {
				t.Fatalf("error = %v, want ErrInvalidJournalFilter", err)
			}
			if c.valid && got.Source == "" {
				t.Fatal("an empty source becomes all")
			}
		})
	}
}

func TestJournalSearchIsACaseInsensitiveSubstring(t *testing.T) {
	filter, err := JournalFilter{Search: "  Accepted  "}.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if !filter.Matches(JournalEntry{Message: "sshd: accepted publickey"}) {
		t.Fatal("the message matches regardless of case")
	}
	if !filter.Matches(JournalEntry{Unit: "accepted.service"}) || !filter.Matches(JournalEntry{Identifier: "ACCEPTED"}) {
		t.Fatal("the unit and the identifier are searched too")
	}
	if filter.Matches(JournalEntry{Message: "refused"}) {
		t.Fatal("no match")
	}
	if regexLike, _ := (JournalFilter{Search: "a.*b"}).Validate(); regexLike.Matches(JournalEntry{Message: "axxb"}) {
		t.Fatal("the search is not a regular expression")
	}
}

func TestJournalCursorValidation(t *testing.T) {
	if !ValidJournalCursor(testCursor) {
		t.Fatal("a cursor printed by journalctl is valid")
	}
	for _, cursor := range []string{"", "s=0;i=1", testCursor + ";", "--cursor=x", strings.ToUpper(testCursor), testCursor + "\n"} {
		if ValidJournalCursor(cursor) {
			t.Fatalf("cursor %q accepted", cursor)
		}
	}
}

func TestJournalPriorities(t *testing.T) {
	if p, ok := ParseJournalPriority("err"); !ok || p != 3 {
		t.Fatalf("err = %d %v", p, ok)
	}
	if _, ok := ParseJournalPriority("error"); ok {
		t.Fatal("only the syslog names are accepted")
	}
}

func TestJournalEntryFromRecord(t *testing.T) {
	record := JournalRecord{
		"__CURSOR":             testCursor,
		"__REALTIME_TIMESTAMP": "1795401740000000",
		"PRIORITY":             "3",
		"_SYSTEMD_UNIT":        "ssh.service",
		"_COMM":                "sshd",
		"_PID":                 "812",
		"_HOSTNAME":            "homelab",
		"MESSAGE":              strings.Repeat("é", MaxJournalMessageBytes),
	}
	entry, ok := JournalEntryFrom(record)
	if !ok {
		t.Fatal("a record with cursor and timestamp is an entry")
	}
	if entry.Time != time.UnixMicro(1795401740000000).UTC() || entry.Priority == nil || *entry.Priority != 3 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Identifier != "sshd" || entry.Unit != "ssh.service" || entry.PID != "812" {
		t.Fatalf("identifier, unit, pid = %q %q %q", entry.Identifier, entry.Unit, entry.PID)
	}
	if !entry.Truncated || len(entry.Message) > MaxJournalMessageBytes || !strings.HasPrefix(entry.Message, "é") || strings.ContainsRune(entry.Message, '�') {
		t.Fatalf("message cut at %d bytes, truncated %v, valid runes expected", len(entry.Message), entry.Truncated)
	}

	record["SYSLOG_IDENTIFIER"], record["PRIORITY"] = "sshd-auth", "nine"
	entry, _ = JournalEntryFrom(record)
	if entry.Identifier != "sshd-auth" || entry.Priority != nil {
		t.Fatalf("identifier %q priority %v", entry.Identifier, entry.Priority)
	}
	if _, ok := JournalEntryFrom(JournalRecord{"MESSAGE": "x"}); ok {
		t.Fatal("a record without cursor is not an entry")
	}
}

func TestJournalShortLine(t *testing.T) {
	entry := JournalEntry{Time: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC), Hostname: "homelab", Identifier: "sshd", PID: "812", Message: "first\nsecond"}
	if got, want := entry.ShortLine(), "2026-11-23T09:00:00Z homelab sshd[812]: first\n    second"; got != want {
		t.Fatalf("ShortLine() = %q, want %q", got, want)
	}
	if got := (JournalEntry{Time: entry.Time, Message: "kernel"}).ShortLine(); got != "2026-11-23T09:00:00Z -: kernel" {
		t.Fatalf("ShortLine() = %q", got)
	}
}

func TestJournalMasking(t *testing.T) {
	record := JournalRecord{"MESSAGE": "hello", "_CMDLINE": "app --password=hunter2", "API_TOKEN": "abc", "db_password": "x", "MY_SECRET_KEY": "y", "_EXE": "/usr/bin/app"}
	for _, name := range []string{"_CMDLINE", "API_TOKEN", "db_password", "MY_SECRET_KEY"} {
		if !JournalFieldMasked(name) {
			t.Fatalf("%s must be masked", name)
		}
	}
	masked := MaskJournalRecord(record)
	if masked["_CMDLINE"] != JournalMaskedValue || masked["API_TOKEN"] != JournalMaskedValue || masked["MESSAGE"] != "hello" || masked["_EXE"] != "/usr/bin/app" {
		t.Fatalf("masked = %v", masked)
	}
	if record["_CMDLINE"] == JournalMaskedValue {
		t.Fatal("the original record is not changed")
	}
	fields := JournalFields(record)
	if fields[0].Name != "API_TOKEN" || !fields[0].Masked || fields[0].Value != JournalMaskedValue {
		t.Fatalf("fields = %+v, want sorted and masked", fields)
	}
	for _, field := range fields {
		if strings.Contains(field.Value, "hunter2") {
			t.Fatal("a secret leaked through the inspector")
		}
	}
}

func TestJournalSummaryBuckets(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 7, 0, 0, time.UTC)
	summary := NewJournalSummary(now)
	if got := summary.End(); !got.Equal(time.Date(2026, 11, 23, 10, 15, 0, 0, time.UTC)) {
		t.Fatalf("End() = %s, want the end of the current bucket", got)
	}
	if got := summary.Start; !got.Equal(time.Date(2026, 11, 22, 10, 15, 0, 0, time.UTC)) {
		t.Fatalf("Start = %s, want 96 buckets before", got)
	}
	summary.Add(now, 3)
	summary.Add(summary.Start, 6)
	summary.Add(summary.Start.Add(-time.Nanosecond), 6) // too old
	summary.Add(summary.End(), 6)                       // in the future
	summary.Add(now, 42)                                // no valid priority: info
	if summary.Counts[JournalSummaryBuckets-1][3] != 1 || summary.Counts[0][6] != 1 || summary.Counts[JournalSummaryBuckets-1][6] != 1 {
		t.Fatalf("counts = first %v last %v", summary.Counts[0], summary.Counts[JournalSummaryBuckets-1])
	}
	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
}
