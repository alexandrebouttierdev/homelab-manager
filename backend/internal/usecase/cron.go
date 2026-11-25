package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"go.uber.org/zap"
)

const (
	// cronIdentifier is the syslog identifier of the child processes of
	// the cron daemon, which log "(user) CMD (command)". They run in a PAM
	// session scope, not in cron.service: a unit filter would miss them.
	cronIdentifier = "CRON"
	// cronJournalEntries bounds the journal lines read for the last runs.
	cronJournalEntries = domain.MaxJournalLimit
	cronReadTimeout    = 10 * time.Second
)

var cronCommandLine = regexp.MustCompile(`^\((\S+)\) CMD \((.*)\)$`)

// CronEntryView is a crontab line with its schedule in French and its next
// runs in the time zone of the machine.
type CronEntryView struct {
	Entry       domain.CronEntry
	Description string
	NextRuns    []time.Time
	// LastRun comes from the CRON lines of the journal; nil when none was
	// found (cron does not log the exit code).
	LastRun *time.Time
}

// CronDirectoryView is a run-parts directory with the line that runs it.
type CronDirectoryView struct {
	Directory domain.CronScriptDirectory
	// ScheduledBy is the crontab line that runs the directory, nil when
	// none does.
	ScheduledBy *CronEntryView
}

// CronTimerView is a systemd timer with the service it starts.
type CronTimerView struct {
	Timer   domain.SystemUnit
	Service string
	// ServiceResult is the result of the last run of the service
	// ("success", "exit-code"…), "" when unknown.
	ServiceResult string
}

// CronView is the scheduled tasks page (SPEC_ADMINISTRATION.md § 8).
type CronView struct {
	Entries          []CronEntryView
	Directories      []CronDirectoryView
	Anacron          bool
	Unreadable       []string
	Timers           []CronTimerView
	TimersAvailable  bool
	JournalAvailable bool
	TimeZone         string
}

// CronInspection is the result of the expression inspector: nothing is
// saved.
type CronInspection struct {
	Description string
	NextRuns    []time.Time
}

// CronJournal lists journal entries (JournalUsecase).
type CronJournal interface {
	List(ctx context.Context, query JournalQuery) (JournalPage, error)
}

// CronUsecase reads the scheduled tasks, read-only: creating, changing or
// running a Cron task would run code as root (rule 3, D-1).
type CronUsecase struct {
	reader   ports.CronReader
	units    *ServiceUsecase
	journal  CronJournal
	logger   *zap.Logger
	now      func() time.Time
	location *time.Location
}

// NewCronUsecase takes the journal as nil when it is unavailable.
func NewCronUsecase(reader ports.CronReader, units *ServiceUsecase, journal CronJournal, logger *zap.Logger) *CronUsecase {
	return &CronUsecase{reader: reader, units: units, journal: journal, logger: logger, now: time.Now, location: time.Local}
}

func (u *CronUsecase) Inventory(ctx context.Context) (CronView, error) {
	ctx, cancel := context.WithTimeout(ctx, cronReadTimeout)
	defer cancel()
	inventory, err := u.reader.Read(ctx)
	if err != nil {
		return CronView{}, err
	}
	now := u.now().In(u.location)
	zone, _ := now.Zone()
	view := CronView{
		Entries: make([]CronEntryView, 0, len(inventory.Entries)), Directories: []CronDirectoryView{},
		Anacron: inventory.Anacron, Unreadable: inventory.Unreadable, Timers: []CronTimerView{}, TimeZone: zone,
	}
	lastRuns, available := u.lastRuns(ctx)
	view.JournalAvailable = available
	for _, entry := range inventory.Entries {
		item := CronEntryView{Entry: entry, NextRuns: []time.Time{}}
		if entry.Valid() {
			item.Description = entry.Schedule.Describe()
			item.NextRuns = entry.Schedule.Next(now, domain.CronNextRuns)
		}
		if at, ok := lastRuns[entry.User+"\x00"+entry.Command]; ok {
			item.LastRun = &at
		}
		view.Entries = append(view.Entries, item)
	}
	for _, directory := range inventory.Directories {
		item := CronDirectoryView{Directory: directory}
		for i := range view.Entries {
			if runsDirectory(view.Entries[i].Entry.Command, directory.Path) {
				entry := view.Entries[i]
				item.ScheduledBy = &entry
				break
			}
		}
		view.Directories = append(view.Directories, item)
	}
	if u.units != nil {
		units := u.units.Inventory()
		view.TimersAvailable = units.CollectedAt != nil
		results := make(map[string]string, len(units.Units))
		for _, unit := range units.Units {
			results[unit.Name] = unit.Result
		}
		for _, unit := range units.Units {
			if unit.Type != domain.UnitTypeTimer {
				continue
			}
			timer := CronTimerView{Timer: unit}
			if len(unit.Triggers) > 0 {
				timer.Service = unit.Triggers[0]
				timer.ServiceResult = results[timer.Service]
			}
			view.Timers = append(view.Timers, timer)
		}
	}
	return view, nil
}

// runsDirectory reports a command that runs the scripts of a directory
// ("run-parts --report /etc/cron.daily").
func runsDirectory(command, directory string) bool {
	if !strings.Contains(command, "run-parts") {
		return false
	}
	for _, field := range strings.FieldsFunc(command, func(r rune) bool { return r == ' ' || r == ';' || r == '\t' }) {
		if field == directory || field == directory+"/" {
			return true
		}
	}
	return false
}

// lastRuns reads the newest CRON lines of the journal: the last start of
// each (account, command). available is false without journal.
func (u *CronUsecase) lastRuns(ctx context.Context) (map[string]time.Time, bool) {
	runs := map[string]time.Time{}
	if u.journal == nil {
		return runs, false
	}
	page, err := u.journal.List(ctx, JournalQuery{
		Filter: domain.JournalFilter{Identifier: cronIdentifier, Priority: len(domain.JournalPriorities) - 1},
		Limit:  cronJournalEntries,
	})
	if errors.Is(err, domain.ErrJournalUnavailable) {
		return runs, false
	}
	if err != nil {
		u.logger.Warn("could not read the cron lines of the journal", zap.Error(err))
		return runs, false
	}
	for _, entry := range page.Entries {
		match := cronCommandLine.FindStringSubmatch(entry.Message)
		if match == nil {
			continue
		}
		key := match[1] + "\x00" + domain.MaskCronSecrets(match[2])
		// Entries go from the newest to the oldest: keep the first.
		if _, seen := runs[key]; !seen {
			runs[key] = entry.Time
		}
	}
	return runs, true
}

// Inspect describes an expression and gives its next runs, without saving
// anything.
func (u *CronUsecase) Inspect(expression string) (CronInspection, error) {
	schedule, err := domain.ParseCron(expression)
	if err != nil {
		return CronInspection{}, err
	}
	return CronInspection{
		Description: schedule.Describe(),
		NextRuns:    schedule.Next(u.now().In(u.location), domain.CronNextRuns),
	}, nil
}
