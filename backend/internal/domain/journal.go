package domain

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Bounds of the system journal consultation (SPEC_ADMINISTRATION.md § 4).
const (
	DefaultJournalLimit = 200
	MaxJournalLimit     = 500
	// MaxJournalScan bounds the entries read by one request that searches
	// text: the search is done by the Manager, after journalctl.
	MaxJournalScan           = 5000
	MaxJournalExport         = 50000
	MaxJournalSummaryEntries = 200000
	JournalSummaryBuckets    = 96
	JournalSummaryBucket     = 15 * time.Minute
	// MaxJournalMessageBytes truncates a message in lists; the entry
	// detail shows it whole.
	MaxJournalMessageBytes = 8192
	DefaultJournalPriority = 6 // info
	maxJournalSearchBytes  = 200
	maxJournalUnitBytes    = 255
)

// JournalMaskedValue replaces the value of a sensitive field.
const JournalMaskedValue = "[masqué]"

var (
	ErrInvalidJournalFilter = errors.New("journal filter values are out of bounds")
	ErrJournalUnavailable   = errors.New("the system journal cannot be read")
	ErrJournalEntryNotFound = errors.New("journal entry not found")
	ErrJournalBootNotFound  = errors.New("journal boot not found")
)

// JournalSource narrows the journal to a family of messages.
type JournalSource string

const (
	JournalSourceAll    JournalSource = "all"
	JournalSourceKernel JournalSource = "kernel"
	JournalSourceAuth   JournalSource = "auth"
	JournalSourceDocker JournalSource = "docker"
)

// JournalDockerUnit is the unit of the Docker daemon (source "docker").
const JournalDockerUnit = "docker.service"

func (s JournalSource) Valid() bool {
	switch s {
	case JournalSourceAll, JournalSourceKernel, JournalSourceAuth, JournalSourceDocker:
		return true
	}
	return false
}

// JournalPriorities are the syslog priority names, from 0 (emerg) to 7.
var JournalPriorities = []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"}

// ParseJournalPriority returns the number of a priority name.
func ParseJournalPriority(name string) (int, bool) {
	index := slices.Index(JournalPriorities, name)
	return index, index >= 0
}

var (
	journalUnitPattern   = regexp.MustCompile(`^[A-Za-z0-9@._:\\-]+\.(service|socket|timer|mount|automount|swap|path|scope|slice|target|device)$`)
	journalBootPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	journalCursorPattern = regexp.MustCompile(`^s=[0-9a-f]{32};i=[0-9a-f]{1,16};b=[0-9a-f]{32};m=[0-9a-f]{1,16};t=[0-9a-f]{1,16};x=[0-9a-f]{1,16}$`)
)

// ValidJournalUnit checks a unit name before it is given to journalctl.
func ValidJournalUnit(name string) bool {
	return len(name) <= maxJournalUnitBytes && !strings.HasPrefix(name, "-") && journalUnitPattern.MatchString(name)
}

// ValidJournalCursor checks a cursor in the form journalctl prints.
func ValidJournalCursor(cursor string) bool {
	return journalCursorPattern.MatchString(cursor)
}

// JournalFilter selects journal entries. The time range is [From, To);
// Priority is the least severe priority shown (6 shows info and above).
type JournalFilter struct {
	Source   JournalSource
	Unit     string
	Priority int
	BootID   string
	From     *time.Time
	To       *time.Time
	Search   string
	// Identifier keeps the entries of one syslog identifier (CRON). It is
	// set by the Manager only, never from a request.
	Identifier string
}

var journalIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Validate checks every value; an empty source means all.
func (f JournalFilter) Validate() (JournalFilter, error) {
	if f.Source == "" {
		f.Source = JournalSourceAll
	}
	f.Search = strings.TrimSpace(f.Search)
	switch {
	case !f.Source.Valid(),
		f.Priority < 0 || f.Priority >= len(JournalPriorities),
		f.Unit != "" && !ValidJournalUnit(f.Unit),
		// The Docker source is a unit filter itself.
		f.Unit != "" && f.Source == JournalSourceDocker,
		f.BootID != "" && !journalBootPattern.MatchString(f.BootID),
		f.Identifier != "" && !journalIdentifierPattern.MatchString(f.Identifier),
		f.From != nil && f.To != nil && !f.From.Before(*f.To),
		len(f.Search) > maxJournalSearchBytes:
		return JournalFilter{}, ErrInvalidJournalFilter
	}
	return f, nil
}

// Matches applies the text search, a case-insensitive substring of the
// message, the unit or the identifier.
func (f JournalFilter) Matches(entry JournalEntry) bool {
	if f.Search == "" {
		return true
	}
	needle := strings.ToLower(f.Search)
	for _, value := range []string{entry.Message, entry.Unit, entry.Identifier} {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

// JournalRecord is one journal entry as journalctl gives it: field names
// and decoded values.
type JournalRecord map[string]string

// JournalListFields are the fields read for a list; journalctl adds the
// cursor, the timestamps and the boot ID.
var JournalListFields = []string{"MESSAGE", "PRIORITY", "_SYSTEMD_UNIT", "_SYSTEMD_USER_UNIT", "SYSLOG_IDENTIFIER", "_COMM", "_PID", "SYSLOG_PID", "_HOSTNAME"}

// JournalEntry is an entry as lists show it.
type JournalEntry struct {
	Cursor     string
	Time       time.Time
	Priority   *int
	Unit       string
	Identifier string
	PID        string
	Hostname   string
	BootID     string
	Message    string
	// Truncated is set when the message was cut to MaxJournalMessageBytes.
	Truncated bool
}

// JournalEntryFrom builds a list entry; a record without cursor or
// timestamp is not an entry.
func JournalEntryFrom(record JournalRecord) (JournalEntry, bool) {
	cursor := record["__CURSOR"]
	micros, err := strconv.ParseInt(record["__REALTIME_TIMESTAMP"], 10, 64)
	if cursor == "" || err != nil {
		return JournalEntry{}, false
	}
	entry := JournalEntry{
		Cursor:     cursor,
		Time:       time.UnixMicro(micros).UTC(),
		Unit:       firstNonEmpty(record["_SYSTEMD_UNIT"], record["_SYSTEMD_USER_UNIT"]),
		Identifier: firstNonEmpty(record["SYSLOG_IDENTIFIER"], record["_COMM"]),
		PID:        firstNonEmpty(record["_PID"], record["SYSLOG_PID"]),
		Hostname:   record["_HOSTNAME"],
		BootID:     record["_BOOT_ID"],
		Message:    record["MESSAGE"],
	}
	if priority, err := strconv.Atoi(record["PRIORITY"]); err == nil && priority >= 0 && priority < len(JournalPriorities) {
		entry.Priority = &priority
	}
	if len(entry.Message) > MaxJournalMessageBytes {
		cut := MaxJournalMessageBytes
		for cut > 0 && !utf8.RuneStart(entry.Message[cut]) {
			cut--
		}
		entry.Message, entry.Truncated = entry.Message[:cut], true
	}
	return entry, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ShortLine formats an entry like journalctl's short-iso output; the
// lines after the first are indented.
func (e JournalEntry) ShortLine() string {
	var line strings.Builder
	line.WriteString(e.Time.Format(time.RFC3339))
	if e.Hostname != "" {
		line.WriteString(" " + e.Hostname)
	}
	identifier := firstNonEmpty(e.Identifier, "-")
	line.WriteString(" " + identifier)
	if e.PID != "" {
		line.WriteString("[" + e.PID + "]")
	}
	line.WriteString(": ")
	line.WriteString(strings.ReplaceAll(e.Message, "\n", "\n    "))
	return line.String()
}

// JournalFieldMasked tells whether a field may hold a secret: the command
// line and any field whose name mentions a password, token or secret.
func JournalFieldMasked(name string) bool {
	upper := strings.ToUpper(name)
	return upper == "_CMDLINE" || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET")
}

// MaskJournalRecord returns a copy whose sensitive values are replaced.
func MaskJournalRecord(record JournalRecord) JournalRecord {
	masked := make(JournalRecord, len(record))
	for name, value := range record {
		if JournalFieldMasked(name) {
			value = JournalMaskedValue
		}
		masked[name] = value
	}
	return masked
}

// JournalField is one field of the entry inspector.
type JournalField struct {
	Name   string
	Value  string
	Masked bool
}

// JournalFields lists the fields of a record by name, masked.
func JournalFields(record JournalRecord) []JournalField {
	fields := make([]JournalField, 0, len(record))
	for name, value := range record {
		masked := JournalFieldMasked(name)
		if masked {
			value = JournalMaskedValue
		}
		fields = append(fields, JournalField{Name: name, Value: value, Masked: masked})
	}
	slices.SortFunc(fields, func(a, b JournalField) int { return strings.Compare(a.Name, b.Name) })
	return fields
}

// JournalBoot is a boot recorded in the journal; Index 0 is the current one.
type JournalBoot struct {
	Index int
	ID    string
	First time.Time
	Last  time.Time
}

// JournalSummary counts entries by priority over the last 24 hours, in
// 15-minute buckets ending with the one that holds the current time.
type JournalSummary struct {
	Start  time.Time
	Counts [JournalSummaryBuckets][8]int
	Total  int
	// Truncated is set when more than MaxJournalSummaryEntries entries
	// were in range: only the most recent ones are counted.
	Truncated      bool
	DiskUsageBytes *int64
	FullAccess     bool
	GeneratedAt    time.Time
}

// NewJournalSummary prepares the buckets around now.
func NewJournalSummary(now time.Time) JournalSummary {
	now = now.UTC()
	start := now.Truncate(JournalSummaryBucket).Add(-(JournalSummaryBuckets - 1) * JournalSummaryBucket)
	return JournalSummary{Start: start, GeneratedAt: now}
}

// End is the end of the last bucket.
func (s JournalSummary) End() time.Time {
	return s.Start.Add(JournalSummaryBuckets * JournalSummaryBucket)
}

// Add counts one entry; an entry out of range is ignored, and one without
// a valid priority counts as info.
func (s *JournalSummary) Add(at time.Time, priority int) {
	if at.Before(s.Start) || !at.Before(s.End()) {
		return
	}
	if priority < 0 || priority >= len(JournalPriorities) {
		priority = DefaultJournalPriority
	}
	s.Counts[int(at.Sub(s.Start)/JournalSummaryBucket)][priority]++
	s.Total++
}
