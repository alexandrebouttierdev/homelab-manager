package journalctl

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

func TestArguments(t *testing.T) {
	from := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	boot := strings.Repeat("b", 32)
	reader := &Reader{}
	cases := []struct {
		name    string
		request ports.JournalRead
		want    []string
	}{
		{
			"newest page",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 6}, Reverse: true, Limit: 200, Fields: []string{"MESSAGE", "PRIORITY"}},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--output-fields=MESSAGE,PRIORITY", "--reverse", "--lines=200", "--priority=info"},
		},
		{
			"older page of a unit in a boot",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 3, Unit: "ssh.service", BootID: boot}, Reverse: true, After: "s=x", Limit: 5},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--reverse", "--after-cursor=s=x", "--lines=5", "--priority=err", "--unit=ssh.service", "--boot=" + boot},
		},
		{
			"follow the kernel",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 7, Source: domain.JournalSourceKernel}, After: "s=y", Limit: 500},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--after-cursor=s=y", "--lines=500", "--priority=debug", "_TRANSPORT=kernel"},
		},
		{
			"authentication in a range",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 6, Source: domain.JournalSourceAuth, From: &from, To: &to}, Limit: 1},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--lines=1", "--priority=info", "--facility=auth,authpriv", "--since=@1795428000.000000", "--until=@1795431599.999999"},
		},
		{
			"cursor in a range: the Manager applies it",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 6, From: &from, To: &to}, Reverse: true, After: "s=z", Limit: 2},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--reverse", "--after-cursor=s=z", "--lines=2", "--priority=info"},
		},
		{
			"docker daemon",
			ports.JournalRead{Filter: domain.JournalFilter{Priority: 6, Source: domain.JournalSourceDocker}, Limit: 1},
			[]string{"--no-pager", "--quiet", "--output=json", "--all", "--lines=1", "--priority=info", "--unit=docker.service"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reader.arguments(c.request); !slices.Equal(got, c.want) {
				t.Fatalf("arguments =\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestDecodeRecord(t *testing.T) {
	record, err := decodeRecord([]byte(`{"__CURSOR":"c","MESSAGE":[27,91,51,49,109,114,101,100],"NOTE":null,"TAG":["a","b"],"BLOB":[255,0],"BAD_TEXT_MESSAGE":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if record["MESSAGE"] != "\x1b[31mred" || record["NOTE"] != "" || record["TAG"] != "a\nb" || record["BLOB"] != "[données binaires, 2 octets]" {
		t.Fatalf("record = %q", record)
	}
	record, err = decodeRecord([]byte(`{"MESSAGE":[104,255,105]}`))
	if err != nil || record["MESSAGE"] != "h�i" {
		t.Fatalf("invalid UTF-8 message = %q, %v", record["MESSAGE"], err)
	}
	for _, line := range []string{`not json`, `{"A":1}`, `{"A":[300]}`} {
		if _, err := decodeRecord([]byte(line)); err == nil {
			t.Fatalf("%s decoded", line)
		}
	}
}

func TestReadLines(t *testing.T) {
	input := "short\n" + strings.Repeat("x", 300) + "\nlast"
	var lines []string
	var stats ports.JournalReadStats
	stopped, err := readLines(strings.NewReader(input), 100, 10_000, func(line []byte) bool {
		lines = append(lines, string(line))
		return true
	}, &stats)
	if err != nil || stopped || !slices.Equal(lines, []string{"short", "last"}) || stats.Skipped != 1 {
		t.Fatalf("lines %q, stopped %v, skipped %d, err %v", lines, stopped, stats.Skipped, err)
	}

	lines, stats = nil, ports.JournalReadStats{}
	stopped, _ = readLines(strings.NewReader("a\nb\nc\n"), 100, 10_000, func(line []byte) bool {
		lines = append(lines, string(line))
		return len(lines) < 2
	}, &stats)
	if !stopped || len(lines) != 2 {
		t.Fatalf("stop: lines %q stopped %v", lines, stopped)
	}

	stats = ports.JournalReadStats{}
	stopped, _ = readLines(strings.NewReader(strings.Repeat("line\n", 100)), 100, 50, func([]byte) bool { return true }, &stats)
	if !stopped || !stats.Truncated {
		t.Fatal("the output bound stops the read and marks it truncated")
	}
}

func TestParseDiskUsage(t *testing.T) {
	for text, want := range map[string]int64{
		"Archived and active journals take up 8.0M in the file system.":   8 << 20,
		"Archived and active journals take up 0B in the file system.":     0,
		"Archived and active journals take up 1.5G in the file system.\n": 3 << 29,
	} {
		if got, err := parseDiskUsage(text); err != nil || got != want {
			t.Fatalf("%q = %d, %v; want %d", text, got, err, want)
		}
	}
	if _, err := parseDiskUsage("nothing"); err == nil {
		t.Fatal("unexpected output is an error")
	}
}

// fakeJournalctl writes a shell script standing for journalctl.
func fakeJournalctl(t *testing.T, script string) *Reader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journalctl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	reader := New(path)
	reader.timeout = 2 * time.Second
	return reader
}

func readAll(t *testing.T, reader *Reader, request ports.JournalRead) ([]domain.JournalRecord, error) {
	t.Helper()
	var records []domain.JournalRecord
	_, err := reader.Read(context.Background(), request, func(record domain.JournalRecord) bool {
		records = append(records, record)
		return true
	})
	return records, err
}

func TestTheManagerEnvironmentIsNotPassedOn(t *testing.T) {
	t.Setenv("HOMELAB_ADMIN_PASSWORD", "must-not-leak")
	reader := fakeJournalctl(t, `printf '{"__CURSOR":"c","MESSAGE":"%s|%s"}\n' "$HOMELAB_ADMIN_PASSWORD" "$LANG"`)
	records, err := readAll(t, reader, ports.JournalRead{Filter: domain.JournalFilter{Priority: 6}, Limit: 1})
	if err != nil || len(records) != 1 {
		t.Fatalf("records %v, err %v", records, err)
	}
	if records[0]["MESSAGE"] != "|C.UTF-8" {
		t.Fatalf("journalctl saw %q, want only its own environment", records[0]["MESSAGE"])
	}
}

func TestACallIsBoundedInTime(t *testing.T) {
	reader := fakeJournalctl(t, "sleep 5")
	reader.timeout = 100 * time.Millisecond
	started := time.Now()
	_, err := readAll(t, reader, ports.JournalRead{Filter: domain.JournalFilter{Priority: 6}, Limit: 1})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("the process was not stopped at the deadline")
	}
}

func TestStoppingEarlyEndsTheProcess(t *testing.T) {
	reader := fakeJournalctl(t, `while true; do echo '{"__CURSOR":"c","__REALTIME_TIMESTAMP":"1"}'; done`)
	count := 0
	_, err := reader.Read(context.Background(), ports.JournalRead{Filter: domain.JournalFilter{Priority: 6}, Limit: 10}, func(domain.JournalRecord) bool {
		count++
		return count < 3
	})
	if err != nil || count != 3 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestAFailureReportsTheErrorOutput(t *testing.T) {
	reader := fakeJournalctl(t, "echo 'Failed to seek to cursor: Invalid argument' >&2; exit 1")
	if _, err := readAll(t, reader, ports.JournalRead{Filter: domain.JournalFilter{Priority: 6}, Limit: 1}); err == nil || !strings.Contains(err.Error(), "Failed to seek") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(filepath.Join(t.TempDir(), "missing")).Units(context.Background()); !errors.Is(err, domain.ErrJournalUnavailable) {
		t.Fatalf("missing program = %v, want ErrJournalUnavailable", err)
	}
}

// Integration with the real journalctl, on journal files written by
// systemd-journal-remote from the export format.

const (
	baseMicros = int64(1795400000000000)
	bootA      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bootB      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var testUnits = []string{"ssh.service", "docker.service", "cron.service"}

// testJournal writes 30 entries, one a minute: entries 0–19 in boot A,
// 20–29 in boot B; entry i has priority i%8, unit testUnits[i%3], kernel
// transport when i%5 == 0, auth facility when i%7 == 0. Entry 3 holds a
// token; entry 4 has a binary message.
func testJournal(t *testing.T) *Reader {
	t.Helper()
	binaryPath, err := exec.LookPath("journalctl")
	remote := "/usr/lib/systemd/systemd-journal-remote"
	if _, statErr := os.Stat(remote); err != nil || statErr != nil {
		t.Skip("journalctl and systemd-journal-remote are needed")
	}
	var export bytes.Buffer
	for i := range 30 {
		boot := bootA
		if i >= 20 {
			boot = bootB
		}
		fmt.Fprintf(&export, "__REALTIME_TIMESTAMP=%d\n__MONOTONIC_TIMESTAMP=%d\n_BOOT_ID=%s\n", baseMicros+int64(i)*60_000_000, 1_000_000+i*1000, boot)
		fmt.Fprintf(&export, "PRIORITY=%d\n_SYSTEMD_UNIT=%s\n_PID=%d\n_COMM=test\n", i%8, testUnits[i%3], 100+i)
		if i%5 == 0 {
			export.WriteString("_TRANSPORT=kernel\n")
		} else {
			export.WriteString("_TRANSPORT=journal\n")
		}
		facility := 3
		if i%7 == 0 {
			facility = 4
		}
		fmt.Fprintf(&export, "SYSLOG_FACILITY=%d\n", facility)
		if i == 3 {
			export.WriteString("API_TOKEN=supersecret\n")
		}
		if i == 4 {
			message := []byte("\x1b[31mred alert")
			export.WriteString("MESSAGE\n")
			_ = binary.Write(&export, binary.LittleEndian, uint64(len(message)))
			export.Write(message)
			export.WriteString("\n")
		} else {
			fmt.Fprintf(&export, "MESSAGE=message %d from %s\n", i, testUnits[i%3])
		}
		export.WriteString("\n")
	}
	directory := t.TempDir()
	command := exec.Command(remote, "--output="+filepath.Join(directory, "test.journal"), "-")
	command.Stdin = &export
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("systemd-journal-remote: %v %s", err, output)
	}
	reader := New(binaryPath)
	reader.directory = directory
	return reader
}

func messageNumbers(records []domain.JournalRecord) []string {
	numbers := make([]string, 0, len(records))
	for _, record := range records {
		fields := strings.Fields(record["MESSAGE"])
		if len(fields) > 1 && fields[0] == "message" {
			numbers = append(numbers, fields[1])
		} else {
			numbers = append(numbers, record["MESSAGE"])
		}
	}
	return numbers
}

func TestJournalctlPagination(t *testing.T) {
	reader := testJournal(t)
	all := domain.JournalFilter{Priority: 7}

	newest, err := readAll(t, reader, ports.JournalRead{Filter: all, Reverse: true, Limit: 3, Fields: domain.JournalListFields})
	if err != nil || !slices.Equal(messageNumbers(newest), []string{"29", "28", "27"}) {
		t.Fatalf("newest = %v, %v", messageNumbers(newest), err)
	}
	if newest[0]["__CURSOR"] == "" || newest[0]["__REALTIME_TIMESTAMP"] == "" || newest[0]["_SYSTEMD_UNIT"] != "cron.service" || newest[0]["SYSLOG_FACILITY"] != "" {
		t.Fatalf("list fields = %v, want the cursor, the timestamp and only the list fields", newest[0])
	}
	if !domain.ValidJournalCursor(newest[0]["__CURSOR"]) {
		t.Fatalf("cursor %q does not pass the domain validation", newest[0]["__CURSOR"])
	}

	older, err := readAll(t, reader, ports.JournalRead{Filter: all, Reverse: true, After: newest[2]["__CURSOR"], Limit: 3})
	if err != nil || !slices.Equal(messageNumbers(older), []string{"26", "25", "24"}) {
		t.Fatalf("older = %v, %v", messageNumbers(older), err)
	}
	newer, err := readAll(t, reader, ports.JournalRead{Filter: all, After: older[1]["__CURSOR"], Limit: 500})
	if err != nil || !slices.Equal(messageNumbers(newer), []string{"26", "27", "28", "29"}) {
		t.Fatalf("newer = %v, %v", messageNumbers(newer), err)
	}
	last, _ := readAll(t, reader, ports.JournalRead{Filter: all, Limit: 2})
	if !slices.Equal(messageNumbers(last), []string{"28", "29"}) {
		t.Fatalf("most recent in order = %v", messageNumbers(last))
	}
}

func TestJournalctlFilters(t *testing.T) {
	reader := testJournal(t)
	from := time.UnixMicro(baseMicros + 10*60_000_000)
	to := time.UnixMicro(baseMicros + 13*60_000_000)
	cases := []struct {
		name   string
		filter domain.JournalFilter
		want   []string
	}{
		{"priority err and above of ssh", domain.JournalFilter{Priority: 3, Unit: "ssh.service"}, []string{"27", "24", "18", "9", "3"}},
		{"kernel", domain.JournalFilter{Priority: 7, Source: domain.JournalSourceKernel}, []string{"25", "20", "15", "10", "5"}},
		{"authentication", domain.JournalFilter{Priority: 7, Source: domain.JournalSourceAuth}, []string{"28", "21", "14", "7", "0"}},
		{"previous boot", domain.JournalFilter{Priority: 7, BootID: bootA}, []string{"19", "18", "17", "16", "15"}},
		{"range excludes its end", domain.JournalFilter{Priority: 7, From: &from, To: &to}, []string{"12", "11", "10"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			records, err := readAll(t, reader, ports.JournalRead{Filter: c.filter, Reverse: true, Limit: 5})
			if err != nil || !slices.Equal(messageNumbers(records), c.want) {
				t.Fatalf("got %v, %v; want %v", messageNumbers(records), err, c.want)
			}
		})
	}
}

func TestJournalctlEntryBootsUnitsAndDiskUsage(t *testing.T) {
	reader := testJournal(t)
	ctx := context.Background()
	records, _ := readAll(t, reader, ports.JournalRead{Filter: domain.JournalFilter{Priority: 7}, Reverse: true, Limit: 30})
	var withToken, binaryMessage domain.JournalRecord
	for _, record := range records {
		if record["API_TOKEN"] != "" {
			withToken = record
		}
		if strings.HasSuffix(record["MESSAGE"], "red alert") {
			binaryMessage = record
		}
	}
	if withToken == nil || binaryMessage == nil || binaryMessage["MESSAGE"] != "\x1b[31mred alert" {
		t.Fatalf("token entry %v, binary message %q", withToken != nil, binaryMessage["MESSAGE"])
	}

	entry, err := reader.Entry(ctx, withToken["__CURSOR"])
	if err != nil || entry["API_TOKEN"] != "supersecret" || entry["_PID"] != "103" {
		t.Fatalf("Entry = %v, %v (the adapter returns raw fields, the domain masks them)", entry, err)
	}
	gone := strings.Replace(withToken["__CURSOR"], "i=4;", "i=ff;", 1)
	if _, err := reader.Entry(ctx, gone); !errors.Is(err, domain.ErrJournalEntryNotFound) {
		t.Fatalf("unknown cursor = %v, want ErrJournalEntryNotFound", err)
	}

	boots, err := reader.Boots(ctx)
	if err != nil || len(boots) != 2 || boots[0].ID != bootB || boots[0].Index != 0 || boots[1].ID != bootA {
		t.Fatalf("Boots = %+v, %v", boots, err)
	}
	if !boots[1].First.Equal(time.UnixMicro(baseMicros)) {
		t.Fatalf("first entry of boot A = %s", boots[1].First)
	}
	units, err := reader.Units(ctx)
	if err != nil || !slices.Equal(units, []string{"cron.service", "docker.service", "ssh.service"}) {
		t.Fatalf("Units = %v, %v", units, err)
	}
	usage, err := reader.DiskUsage(ctx)
	if err != nil || usage <= 0 {
		t.Fatalf("DiskUsage = %d, %v", usage, err)
	}
}

// journalctl refuses --since with a cursor: pages after the first one in a
// range must still stop at its start.
func TestJournalctlRangeWithACursor(t *testing.T) {
	reader := testJournal(t)
	from := time.UnixMicro(baseMicros + 10*60_000_000)
	to := time.UnixMicro(baseMicros + 25*60_000_000)
	filter := domain.JournalFilter{Priority: 7, From: &from, To: &to}

	first, err := readAll(t, reader, ports.JournalRead{Filter: filter, Reverse: true, Limit: 3})
	if err != nil || !slices.Equal(messageNumbers(first), []string{"24", "23", "22"}) {
		t.Fatalf("first page = %v, %v", messageNumbers(first), err)
	}
	older, err := readAll(t, reader, ports.JournalRead{Filter: filter, Reverse: true, After: first[2]["__CURSOR"], Limit: 50})
	if err != nil || len(older) != 12 || messageNumbers(older)[0] != "21" || messageNumbers(older)[11] != "10" {
		t.Fatalf("older pages = %v, %v; want 21 down to 10, the start of the range", messageNumbers(older), err)
	}
	newer, err := readAll(t, reader, ports.JournalRead{Filter: filter, After: older[11]["__CURSOR"], Limit: 50})
	if err != nil || len(newer) != 14 || messageNumbers(newer)[13] != "24" {
		t.Fatalf("follow in a range = %v, %v; want 11 up to 24, the end of the range", messageNumbers(newer), err)
	}
}
