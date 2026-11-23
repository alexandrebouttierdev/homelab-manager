package domain

import (
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Bounds of the system services module (SPEC_ADMINISTRATION.md § 5.1).
const (
	// MaxSystemUnits bounds the inventory; failed units are kept first
	// when a machine has more.
	MaxSystemUnits = 2000
	// MaxUnitDependencies bounds Requires and After in a unit detail.
	MaxUnitDependencies = 100
	// UnitJournalLines is the journal excerpt of a unit detail.
	UnitJournalLines = 50
)

var (
	ErrInvalidUnitName     = errors.New("unit name is invalid")
	ErrUnitNotFound        = errors.New("unit not found")
	ErrSystemdUnavailable  = errors.New("systemd cannot be reached on the system bus")
	unitNamePattern        = regexp.MustCompile(`^[A-Za-z0-9@._:\-]{1,255}\.(service|timer|socket)$`)
	unitNameSuffixesByType = map[string]SystemUnitType{
		".service": UnitTypeService, ".timer": UnitTypeTimer, ".socket": UnitTypeSocket,
	}
)

// Event types of the system services module (API_CONTRACTS.md § 6.2).
const (
	EventServiceFailed    = "service.failed"
	EventServiceRecovered = "service.recovered"
)

// SystemUnitType is one of the unit types the Manager reads.
type SystemUnitType string

const (
	UnitTypeService SystemUnitType = "service"
	UnitTypeTimer   SystemUnitType = "timer"
	UnitTypeSocket  SystemUnitType = "socket"
)

// UnitTypeOf returns the type of a unit name, or "" for a type the
// Manager does not read (mount, target…).
func UnitTypeOf(name string) SystemUnitType {
	for suffix, unitType := range unitNameSuffixesByType {
		if strings.HasSuffix(name, suffix) {
			return unitType
		}
	}
	return ""
}

// ValidUnitName checks a unit name received from a client
// (SPEC_ADMINISTRATION.md § 2.2). Escaped names (a backslash, as in
// "dev-disk-by\x2duuid") are outside the pattern.
func ValidUnitName(name string) bool {
	return unitNamePattern.MatchString(name)
}

// SystemUnit is one loaded unit of the inventory. A nil pointer is a value
// systemd does not give (accounting disabled, never triggered…), never 0.
type SystemUnit struct {
	Name           string
	Description    string
	Type           SystemUnitType
	LoadState      string
	ActiveState    string
	SubState       string
	UnitFileState  string
	StateChangedAt *time.Time
	MainPID        *uint32
	MemoryBytes    *uint64
	CPUNanoseconds *uint64
	// Result of the last run (service, timer, socket); "" when unknown.
	Result string
	// NextElapse and LastTrigger are set for timers only.
	NextElapse  *time.Time
	LastTrigger *time.Time
	// Triggers lists the units a timer or a socket starts.
	Triggers []string
}

// UnitLimit is a resource limit of a unit: Unlimited ("infinity") differs
// from an unknown limit (Value nil, Unlimited false).
type UnitLimit struct {
	Value     *uint64
	Unlimited bool
}

// SystemUnitDetail adds the definition of a unit, read on demand.
type SystemUnitDetail struct {
	SystemUnit
	FragmentPath string
	// ExecStart holds the command lines as systemd stores them, shown as
	// they are; nothing is ever run.
	ExecStart   []string
	Requires    []string
	After       []string
	Restart     string
	Slice       string
	TasksMax    UnitLimit
	LimitNOFILE UnitLimit
	MemoryMax   UnitLimit
}

// SystemUnitCounts summarizes an inventory.
type SystemUnitCounts struct {
	Total        int
	Active       int
	Failed       int
	ActiveTimers int
}

// SystemUnitsStatus tells how the last collection ended.
type SystemUnitsStatus string

const (
	// UnitsPending: no collection has ended yet.
	UnitsPending SystemUnitsStatus = "pending"
	UnitsOK      SystemUnitsStatus = "ok"
	// UnitsError: systemd answered with an error, or the cycle timed out.
	UnitsError SystemUnitsStatus = "error"
	// UnitsUnavailable: no system bus or no systemd on it.
	UnitsUnavailable SystemUnitsStatus = "unavailable"
)

// OptionalCounter maps systemd's "not available" (UINT64_MAX, for example
// with memory accounting disabled) to nil.
func OptionalCounter(value uint64) *uint64 {
	if value == math.MaxUint64 {
		return nil
	}
	return &value
}

// OptionalTimestamp maps a systemd timestamp in microseconds since the
// epoch to UTC; 0 (never) and UINT64_MAX are nil.
func OptionalTimestamp(microseconds uint64) *time.Time {
	if microseconds == 0 || microseconds > math.MaxInt64 {
		return nil
	}
	at := time.UnixMicro(int64(microseconds)).UTC()
	return &at
}

// OptionalPID maps PID 0 (no main process) to nil.
func OptionalPID(pid uint32) *uint32 {
	if pid == 0 {
		return nil
	}
	return &pid
}

// UnitLimitFrom reads a limit property; known is false when systemd did
// not give it. UINT64_MAX is systemd's "infinity".
func UnitLimitFrom(value uint64, known bool) UnitLimit {
	switch {
	case !known:
		return UnitLimit{}
	case value == math.MaxUint64:
		return UnitLimit{Unlimited: true}
	default:
		return UnitLimit{Value: &value}
	}
}

// CountUnits gives the counters shown above the inventory.
func CountUnits(units []SystemUnit) SystemUnitCounts {
	counts := SystemUnitCounts{Total: len(units)}
	for _, unit := range units {
		switch unit.ActiveState {
		case "active":
			counts.Active++
			if unit.Type == UnitTypeTimer {
				counts.ActiveTimers++
			}
		case "failed":
			counts.Failed++
		}
	}
	return counts
}

// FailureTransitions is the change to the set of failed units and the
// events it produces.
type FailureTransitions struct {
	Added   []string
	Removed []string
	Events  []MonitorEvent
}

// UnitFailureTransitions compares the units recorded as failed with a new
// inventory. A failed unit absent from the set is added (service.failed);
// a unit of the set that is active again is removed (service.recovered); a
// unit of the set that disappeared is removed without event. Any other
// unit of the set stays, so that a unit restarting after a failure does
// not produce a second service.failed.
func UnitFailureTransitions(previous map[string]struct{}, units []SystemUnit, at time.Time) FailureTransitions {
	var result FailureTransitions
	seen := make(map[string]struct{}, len(units))
	for _, unit := range units {
		seen[unit.Name] = struct{}{}
		_, recorded := previous[unit.Name]
		switch {
		case unit.ActiveState == "failed" && !recorded:
			result.Added = append(result.Added, unit.Name)
			result.Events = append(result.Events, unitEvent(unit, EventServiceFailed, "error", "Unit entered the failed state", at))
		case unit.ActiveState == "active" && recorded:
			result.Removed = append(result.Removed, unit.Name)
			result.Events = append(result.Events, unitEvent(unit, EventServiceRecovered, "info", "Unit is active again", at))
		}
	}
	for name := range previous {
		if _, ok := seen[name]; !ok {
			result.Removed = append(result.Removed, name)
		}
	}
	sort.Strings(result.Removed)
	return result
}

func unitEvent(unit SystemUnit, eventType, severity, message string, at time.Time) MonitorEvent {
	return MonitorEvent{
		ID: uuid.New(), Type: eventType, Severity: severity, Message: message, OccurredAt: at,
		Metadata: map[string]any{
			"unit": unit.Name, "activeState": unit.ActiveState, "subState": unit.SubState, "result": unit.Result,
		},
	}
}
