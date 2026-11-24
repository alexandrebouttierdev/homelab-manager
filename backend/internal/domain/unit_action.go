package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Bounds of the service actions (SPEC_ADMINISTRATION.md § 5.2 and § 2.2).
const (
	// MaxManageableUnits bounds the allow list of the service configuration.
	MaxManageableUnits = 200
	// UnitActionTimeout bounds an action, from the request to a stable state.
	UnitActionTimeout = 90 * time.Second
	// UnitActionsPerMinute bounds the actions requested by one user.
	UnitActionsPerMinute = 30
)

var (
	ErrInvalidUnitVerb    = errors.New("unit action must be start, stop, restart or reload")
	ErrUnitNotManageable  = errors.New("unit is not in the allow list of the service configuration")
	ErrUnitActionsLimited = errors.New("too many unit actions in the last minute")
	// ErrUnitActionsUnavailable: the action worker is not running (Manager
	// starting or stopping).
	ErrUnitActionsUnavailable = errors.New("unit actions are unavailable")
)

// UnitVerb is one of the four changes the Manager may ask systemd for.
type UnitVerb string

const (
	UnitStart   UnitVerb = "start"
	UnitStop    UnitVerb = "stop"
	UnitRestart UnitVerb = "restart"
	UnitReload  UnitVerb = "reload"
)

// UnitVerbs lists the verbs in the order shown by the interface; they are
// also the verbs the generated polkit rule grants.
var UnitVerbs = []UnitVerb{UnitStart, UnitStop, UnitRestart, UnitReload}

func (v UnitVerb) Valid() bool {
	switch v {
	case UnitStart, UnitStop, UnitRestart, UnitReload:
		return true
	}
	return false
}

// UnitActionState is where an action stands.
type UnitActionState string

const (
	ActionRunning   UnitActionState = "running"
	ActionSucceeded UnitActionState = "succeeded"
	ActionFailed    UnitActionState = "failed"
)

// UnitActionFailure tells why an action failed.
type UnitActionFailure string

const (
	// ActionUnitFailed: the unit is in the failed state once stable.
	ActionUnitFailed UnitActionFailure = "failed"
	// ActionNotAllowed: polkit refused (no rule, or a rule without the unit).
	ActionNotAllowed UnitActionFailure = "not_allowed"
	// ActionNotFound: systemd knows no such unit.
	ActionNotFound UnitActionFailure = "not_found"
	// ActionRejected: systemd refused the job (verb not applicable, unit
	// masked, manual start refused…).
	ActionRejected UnitActionFailure = "rejected"
	// ActionTimeout: no stable state within UnitActionTimeout.
	ActionTimeout UnitActionFailure = "timeout"
	// ActionUnavailable: the system bus or systemd cannot be reached.
	ActionUnavailable UnitActionFailure = "unavailable"
	// ActionInterrupted: the Manager stopped while following the action.
	ActionInterrupted UnitActionFailure = "interrupted"
)

// UnitActionError is a failure reported by the unit controller.
type UnitActionError struct {
	Failure UnitActionFailure
	Err     error
}

func (e *UnitActionError) Error() string {
	return fmt.Sprintf("unit action %s: %v", e.Failure, e.Err)
}

func (e *UnitActionError) Unwrap() error { return e.Err }

// UnitStableState is the state of a unit read at the end of an action.
type UnitStableState struct {
	ActiveState string
	SubState    string
}

// UnitAction is the last action requested on a unit.
type UnitAction struct {
	Unit        string
	Verb        UnitVerb
	State       UnitActionState
	Failure     UnitActionFailure
	RequestedBy string
	RequestedAt time.Time
	FinishedAt  *time.Time
	// Final is the unit state once stable; nil while running and when it
	// could not be read.
	Final *UnitStableState
}

// ManageableUnits is the allow list of SPEC_ADMINISTRATION.md § 5.2: the
// only units the Manager may start, stop, restart or reload.
type ManageableUnits struct {
	names map[string]struct{}
}

// NewManageableUnits validates the allow list of the service
// configuration: each name follows the pattern of § 2.2 and names one
// unit (a template without instance, "name@.service", cannot be started).
// Duplicates are removed.
func NewManageableUnits(names []string) (ManageableUnits, error) {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !ValidUnitName(name) || strings.Contains(name, "@.") {
			return ManageableUnits{}, fmt.Errorf("%q is not a unit name the Manager can manage", name)
		}
		set[name] = struct{}{}
	}
	if len(set) > MaxManageableUnits {
		return ManageableUnits{}, fmt.Errorf("at most %d manageable units, got %d", MaxManageableUnits, len(set))
	}
	return ManageableUnits{names: set}, nil
}

// Contains reports whether the unit is in the allow list.
func (m ManageableUnits) Contains(name string) bool {
	_, ok := m.names[name]
	return ok
}

// Names returns the allow list sorted by name.
func (m ManageableUnits) Names() []string {
	names := make([]string, 0, len(m.names))
	for name := range m.names {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
