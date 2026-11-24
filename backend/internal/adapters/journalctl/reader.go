// Package journalctl reads the systemd journal by running journalctl, the
// only external program allowed for it (SPEC_ADMINISTRATION.md § 2.3).
// Arguments are built from validated values, in the --option=value form, and
// the program runs without a shell and with an environment of its own.
package journalctl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const (
	callTimeout = 10 * time.Second
	// maxProcesses bounds the journalctl processes running at once.
	maxProcesses = 4
	// maxLineBytes bounds one JSON entry; a larger one is skipped.
	maxLineBytes = 1 << 20
	// maxOutputBytes bounds the output of one call.
	maxOutputBytes = 64 << 20
	maxStderrBytes = 4096
	maxUnits       = 1000
	maxBoots       = 100
)

// environment is the whole environment of journalctl: the Manager's own
// variables, secrets included, are not passed on.
var environment = []string{"LANG=C.UTF-8", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=", "SYSTEMD_LESS="}

// Reader implements ports.JournalReader.
type Reader struct {
	binary     string
	fullAccess bool
	slots      chan struct{}
	timeout    time.Duration
	// directory reads journal files from a directory instead of the
	// system journal; tests only.
	directory string
}

var _ ports.JournalReader = (*Reader)(nil)

// Find returns the path of journalctl in the service PATH.
func Find() (string, error) {
	path, err := exec.LookPath("journalctl")
	if err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrJournalUnavailable, err)
	}
	return path, nil
}

// New returns a reader running binary.
func New(binary string) *Reader {
	return &Reader{binary: binary, fullAccess: hasFullAccess(), slots: make(chan struct{}, maxProcesses), timeout: callTimeout}
}

func (r *Reader) FullAccess() bool { return r.fullAccess }

// hasFullAccess tells whether this process can read the whole journal:
// root, or a member of systemd-journal or adm.
func hasFullAccess() bool {
	if os.Geteuid() == 0 {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, name := range []string{"systemd-journal", "adm"} {
		group, err := user.LookupGroup(name)
		if err != nil {
			continue
		}
		if gid, err := strconv.Atoi(group.Gid); err == nil && (slices.Contains(groups, gid) || os.Getegid() == gid) {
			return true
		}
	}
	return false
}

// arguments builds the journalctl arguments of a read.
func (r *Reader) arguments(request ports.JournalRead) []string {
	args := r.base("--output=json", "--all")
	if len(request.Fields) > 0 {
		args = append(args, "--output-fields="+strings.Join(request.Fields, ","))
	}
	if request.Reverse {
		args = append(args, "--reverse")
	}
	if request.After != "" {
		args = append(args, "--after-cursor="+request.After)
	}
	args = append(args, "--lines="+strconv.Itoa(request.Limit))
	filter := request.Filter
	args = append(args, "--priority="+domain.JournalPriorities[filter.Priority])
	switch filter.Source {
	case domain.JournalSourceKernel:
		args = append(args, "_TRANSPORT=kernel")
	case domain.JournalSourceAuth:
		args = append(args, "--facility=auth,authpriv")
	case domain.JournalSourceDocker:
		args = append(args, "--unit="+domain.JournalDockerUnit)
	}
	if filter.Unit != "" {
		args = append(args, "--unit="+filter.Unit)
	}
	if filter.Identifier != "" {
		args = append(args, "--identifier="+filter.Identifier)
	}
	if filter.BootID != "" {
		args = append(args, "--boot="+filter.BootID)
	}
	// With a cursor, journalctl refuses --since and ignores --until: Read
	// applies the range itself.
	if request.After == "" && filter.From != nil {
		args = append(args, "--since="+timestamp(*filter.From))
	}
	if request.After == "" && filter.To != nil {
		// --until includes its bound; the range is [From, To).
		args = append(args, "--until="+timestamp(filter.To.Add(-time.Microsecond)))
	}
	return args
}

func (r *Reader) base(args ...string) []string {
	base := []string{"--no-pager", "--quiet"}
	if r.directory != "" {
		base = append(base, "--directory="+r.directory)
	}
	return append(base, args...)
}

// timestamp writes a time as journalctl's @seconds.microseconds.
func timestamp(at time.Time) string {
	micros := at.UnixMicro()
	sign := ""
	if micros < 0 {
		sign, micros = "-", -micros
	}
	return fmt.Sprintf("@%s%d.%06d", sign, micros/1_000_000, micros%1_000_000)
}

func (r *Reader) Read(ctx context.Context, request ports.JournalRead, visit func(domain.JournalRecord) bool) (ports.JournalReadStats, error) {
	if request.Limit <= 0 {
		return ports.JournalReadStats{}, nil
	}
	// With a cursor the range is applied here (see arguments): reading
	// stops at the first entry past the bound it moves towards, and passes
	// over entries past the other one.
	ranged := request.After != "" && (request.Filter.From != nil || request.Filter.To != nil)
	var stats ports.JournalReadStats
	err := r.run(ctx, r.arguments(request), func(line []byte) bool {
		record, err := decodeRecord(line)
		if err != nil {
			stats.Skipped++
			return true
		}
		if ranged {
			if at, err := strconv.ParseInt(record["__REALTIME_TIMESTAMP"], 10, 64); err == nil {
				before := request.Filter.From != nil && at < request.Filter.From.UnixMicro()
				after := request.Filter.To != nil && at >= request.Filter.To.UnixMicro()
				switch {
				case before && request.Reverse, after && !request.Reverse:
					return false
				case before, after:
					return true
				}
			}
		}
		return visit(record)
	}, &stats)
	return stats, err
}

func (r *Reader) Entry(ctx context.Context, cursor string) (domain.JournalRecord, error) {
	var found domain.JournalRecord
	var stats ports.JournalReadStats
	err := r.run(ctx, r.base("--output=json", "--all", "--cursor="+cursor, "--lines=1"), func(line []byte) bool {
		if record, err := decodeRecord(line); err == nil {
			found = record
		}
		return false
	}, &stats)
	if err != nil {
		return nil, err
	}
	// A cursor whose entry was removed seeks to a neighbour.
	if found == nil || found["__CURSOR"] != cursor {
		return nil, domain.ErrJournalEntryNotFound
	}
	return found, nil
}

func (r *Reader) Boots(ctx context.Context) ([]domain.JournalBoot, error) {
	var output bytes.Buffer
	var stats ports.JournalReadStats
	err := r.run(ctx, r.base("--list-boots", "--output=json"), func(line []byte) bool {
		output.Write(line)
		return true
	}, &stats)
	if err != nil || output.Len() == 0 {
		return nil, err
	}
	var listed []struct {
		Index      int    `json:"index"`
		BootID     string `json:"boot_id"`
		FirstEntry int64  `json:"first_entry"`
		LastEntry  int64  `json:"last_entry"`
	}
	if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
		return nil, fmt.Errorf("decode journal boots: %w", err)
	}
	boots := make([]domain.JournalBoot, 0, len(listed))
	for _, boot := range listed {
		boots = append(boots, domain.JournalBoot{
			Index: boot.Index, ID: boot.BootID,
			First: time.UnixMicro(boot.FirstEntry).UTC(), Last: time.UnixMicro(boot.LastEntry).UTC(),
		})
	}
	slices.SortFunc(boots, func(a, b domain.JournalBoot) int { return b.Index - a.Index })
	return boots[:min(len(boots), maxBoots)], nil
}

func (r *Reader) Units(ctx context.Context) ([]string, error) {
	var units []string
	var stats ports.JournalReadStats
	err := r.run(ctx, r.base("--field=_SYSTEMD_UNIT"), func(line []byte) bool {
		if name := string(line); domain.ValidJournalUnit(name) {
			units = append(units, name)
		}
		return len(units) < maxUnits
	}, &stats)
	slices.Sort(units)
	return units, err
}

var diskUsagePattern = regexp.MustCompile(`take up ([0-9]+(?:\.[0-9]+)?)([BKMGTPE])`)

func (r *Reader) DiskUsage(ctx context.Context) (int64, error) {
	var output bytes.Buffer
	var stats ports.JournalReadStats
	if err := r.run(ctx, r.base("--disk-usage"), func(line []byte) bool {
		output.Write(line)
		return true
	}, &stats); err != nil {
		return 0, err
	}
	return parseDiskUsage(output.String())
}

// parseDiskUsage reads journalctl's size, written with base-1024 units.
func parseDiskUsage(text string) (int64, error) {
	match := diskUsagePattern.FindStringSubmatch(text)
	if match == nil {
		return 0, fmt.Errorf("unexpected disk usage output %q", text)
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, err
	}
	exponent := strings.Index("BKMGTPE", match[2])
	for range exponent {
		value *= 1024
	}
	return int64(value), nil
}

// run starts journalctl and gives each output line to onLine, until it
// returns false. Lines longer than maxLineBytes are skipped and counted.
func (r *Reader) run(ctx context.Context, args []string, onLine func([]byte) bool, stats *ports.JournalReadStats) error {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	command := exec.CommandContext(callCtx, r.binary, args...)
	command.Env = environment
	command.WaitDelay = time.Second
	stderr := &limitedBuffer{limit: maxStderrBytes}
	command.Stderr = stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("journalctl output: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrJournalUnavailable, err)
	}

	stopped, readErr := readLines(stdout, maxLineBytes, maxOutputBytes, onLine, stats)
	// The deadline or the caller's cancellation may end the read before
	// the end of the output: that is an error, not a stop.
	interrupted := callCtx.Err()
	if stopped || readErr != nil {
		cancel()
	}
	waitErr := command.Wait()
	switch {
	case interrupted != nil:
		return fmt.Errorf("journalctl: %w", interrupted)
	case stopped:
		return nil
	case readErr != nil:
		return fmt.Errorf("read journalctl output: %w", readErr)
	case waitErr != nil:
		return fmt.Errorf("journalctl: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// readLines splits r into lines. It reports whether it stopped before the
// end: onLine returned false or the output exceeded maxTotal.
func readLines(r io.Reader, maxLine, maxTotal int, onLine func([]byte) bool, stats *ports.JournalReadStats) (bool, error) {
	reader := bufio.NewReaderSize(r, 64<<10)
	var line []byte
	total, oversized := 0, false
	for {
		chunk, err := reader.ReadSlice('\n')
		total += len(chunk)
		if total > maxTotal {
			stats.Truncated = true
			return true, nil
		}
		if !oversized {
			line = append(line, chunk...)
			if len(line) > maxLine {
				oversized, line = true, line[:0]
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		complete := err == nil || (errors.Is(err, io.EOF) && len(line) > 0)
		if complete {
			if oversized {
				stats.Skipped++
			} else if text := bytes.TrimRight(line, "\r\n"); len(text) > 0 && !onLine(text) {
				return true, nil
			}
		}
		line, oversized = line[:0], false
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

// decodeRecord reads one entry of journalctl's JSON output. A value is a
// string, null, an array of bytes (binary or non-printable data) or an
// array of values (a field set several times).
func decodeRecord(line []byte) (domain.JournalRecord, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}
	record := make(domain.JournalRecord, len(raw))
	for name, value := range raw {
		text, err := decodeValue(name, value)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", name, err)
		}
		record[name] = text
	}
	return record, nil
}

func decodeValue(name string, value json.RawMessage) (string, error) {
	value = bytes.TrimSpace(value)
	switch {
	case len(value) == 0 || bytes.Equal(value, []byte("null")):
		return "", nil
	case value[0] == '"':
		var text string
		err := json.Unmarshal(value, &text)
		return text, err
	case value[0] != '[':
		return "", fmt.Errorf("unexpected value %.20s", value)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(value, &items); err != nil {
		return "", err
	}
	if len(items) > 0 && (items[0][0] == '"' || items[0][0] == '[' || items[0][0] == 'n') {
		values := make([]string, 0, len(items))
		for _, item := range items {
			text, err := decodeValue(name, item)
			if err != nil {
				return "", err
			}
			values = append(values, text)
		}
		return strings.Join(values, "\n"), nil
	}
	var data []byte
	for _, item := range items {
		number, err := strconv.ParseUint(string(item), 10, 8)
		if err != nil {
			return "", fmt.Errorf("unexpected byte %s", item)
		}
		data = append(data, byte(number))
	}
	switch {
	case utf8.Valid(data):
		return string(data), nil
	case name == "MESSAGE":
		return strings.ToValidUTF8(string(data), "�"), nil
	}
	return fmt.Sprintf("[données binaires, %d octets]", len(data)), nil
}

// limitedBuffer keeps the first bytes of journalctl's error output.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		b.Buffer.Write(data[:min(len(data), room)])
	}
	return len(data), nil
}
