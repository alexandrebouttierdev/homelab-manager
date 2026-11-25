package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// MaintenanceKind names a task of the fixed catalog (decision D-1). The
// catalog replaces the editing of Cron, which would run any command as
// root (rule 3): each task runs a unit shipped with the project, with
// parameters chosen from a list.
type MaintenanceKind string

const (
	// MaintenanceJournalVacuum keeps the systemd journal under a size
	// (journalctl --vacuum-size, in homelab-journal-vacuum@<size>.service).
	MaintenanceJournalVacuum MaintenanceKind = "journal-vacuum"
)

// MaintenanceKinds lists the catalog in the order shown by the interface.
var MaintenanceKinds = []MaintenanceKind{MaintenanceJournalVacuum}

// JournalVacuumSizes are the sizes the journal may be kept under, in MiB;
// the polkit rule grants exactly their units.
var JournalVacuumSizes = []int{256, 512, 1024, 2048}

// MaintenanceTimeout bounds one run of a task.
const MaintenanceTimeout = 10 * time.Minute

var (
	ErrUnknownMaintenanceTask = errors.New("maintenance task is not in the catalog")
	ErrInvalidMaintenanceTask = errors.New("maintenance task settings are invalid")
	// ErrMaintenanceUnavailable: the worker is not running.
	ErrMaintenanceUnavailable = errors.New("maintenance tasks are unavailable")
)

func (k MaintenanceKind) Valid() bool { return slices.Contains(MaintenanceKinds, k) }

// MaintenanceOrigin tells who started a run.
type MaintenanceOrigin string

const (
	MaintenanceManual   MaintenanceOrigin = "manual"
	MaintenanceSchedule MaintenanceOrigin = "schedule"
)

// MaintenanceRun is the last run of a task.
type MaintenanceRun struct {
	At        time.Time
	Origin    MaintenanceOrigin
	Succeeded bool
	// Message: the cause of a failure.
	Message string
}

// MaintenanceTask is a task of the catalog and its settings. Its schedule
// uses the clock of the machine, like the backups.
type MaintenanceTask struct {
	Kind       MaintenanceKind
	Schedule   BackupSchedule
	MaxSizeMiB int
	LastRun    *MaintenanceRun
	UpdatedAt  *time.Time
	UpdatedBy  string
}

// DefaultMaintenanceTask is a task never set: disabled, weekly on Sunday
// at 04:00, journal kept under 512 MiB.
func DefaultMaintenanceTask(kind MaintenanceKind) MaintenanceTask {
	return MaintenanceTask{
		Kind: kind, Schedule: BackupSchedule{Frequency: BackupWeekly, Weekday: time.Sunday, Hour: 4}, MaxSizeMiB: 512,
	}
}

// Validate checks the kind, the schedule and the parameters.
func (t MaintenanceTask) Validate() error {
	if !t.Kind.Valid() {
		return ErrUnknownMaintenanceTask
	}
	if err := ValidateBackupSchedule(t.Schedule); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMaintenanceTask, err)
	}
	if t.Kind == MaintenanceJournalVacuum && !slices.Contains(JournalVacuumSizes, t.MaxSizeMiB) {
		return fmt.Errorf("%w: maxSizeMiB must be 256, 512, 1024 or 2048", ErrInvalidMaintenanceTask)
	}
	return nil
}

// Unit is the unit a run of the task starts.
func (t MaintenanceTask) Unit() string {
	return fmt.Sprintf("homelab-journal-vacuum@%dM.service", t.MaxSizeMiB)
}

// MaintenanceUnits lists every unit of the catalog: the polkit rule grants
// the verb start on these only.
func MaintenanceUnits() []string {
	units := make([]string, 0, len(JournalVacuumSizes))
	for _, size := range JournalVacuumSizes {
		units = append(units, MaintenanceTask{Kind: MaintenanceJournalVacuum, MaxSizeMiB: size}.Unit())
	}
	return units
}
