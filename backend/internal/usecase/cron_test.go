package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"go.uber.org/zap"
)

type fakeCronReader struct {
	inventory domain.CronInventory
}

func (f fakeCronReader) Read(context.Context) (domain.CronInventory, error) { return f.inventory, nil }

type fakeCronJournal struct {
	entries []domain.JournalEntry
	err     error
	query   JournalQuery
}

func (f *fakeCronJournal) List(_ context.Context, query JournalQuery) (JournalPage, error) {
	f.query = query
	return JournalPage{Entries: f.entries}, f.err
}

func cronEntry(t *testing.T, expression, user, command string) domain.CronEntry {
	t.Helper()
	schedule, err := domain.ParseCron(expression)
	if err != nil {
		t.Fatal(err)
	}
	return domain.CronEntry{Source: "/etc/crontab", Line: 1, Expression: expression, User: user, Command: command, Schedule: schedule}
}

func TestCronInventory(t *testing.T) {
	hourly := cronEntry(t, "17 * * * *", "root", "cd / && run-parts --report /etc/cron.hourly")
	backup := cronEntry(t, "@daily", "alice", "/opt/backup.sh --password=••••")
	broken := domain.CronEntry{Source: "/etc/cron.d/x", Line: 2, Expression: "61 * * * *", User: "root", Command: "x", Error: "invalid"}
	reader := fakeCronReader{inventory: domain.CronInventory{
		Entries: []domain.CronEntry{hourly, backup, broken},
		Directories: []domain.CronScriptDirectory{
			{Period: "hourly", Path: "/etc/cron.hourly", Scripts: []string{"a"}},
			{Period: "weekly", Path: "/etc/cron.weekly", Scripts: []string{}},
		},
	}}
	ran := time.Date(2026, 11, 24, 9, 17, 1, 0, time.UTC)
	journal := &fakeCronJournal{entries: []domain.JournalEntry{
		{Time: ran, Message: "(root) CMD (cd / && run-parts --report /etc/cron.hourly)"},
		{Time: ran.Add(-time.Hour), Message: "(root) CMD (cd / && run-parts --report /etc/cron.hourly)"},
		{Time: ran.Add(-2 * time.Hour), Message: "(alice) CMD (/opt/backup.sh --password=hunter2)"},
		{Time: ran, Message: "pam_unix(cron:session): session opened for user root"},
	}}
	units := NewServiceUsecase(nil, nil, nil, zap.NewNop())
	collected := ran
	units.inventory = UnitInventory{Status: domain.UnitsOK, CollectedAt: &collected, Units: []domain.SystemUnit{
		{Name: "apt-daily.timer", Type: domain.UnitTypeTimer, ActiveState: "active", Triggers: []string{"apt-daily.service"}},
		{Name: "apt-daily.service", Type: domain.UnitTypeService, ActiveState: "inactive", Result: "exit-code"},
	}}
	cron := NewCronUsecase(reader, units, journal, zap.NewNop())
	cron.now, cron.location = func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) }, time.UTC

	view, err := cron.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if journal.query.Filter.Identifier != "CRON" || !view.JournalAvailable || view.TimeZone != "UTC" {
		t.Fatalf("query = %+v, view = %+v", journal.query, view)
	}
	first := view.Entries[0]
	if first.Description != "Toutes les heures, à la minute 17" || len(first.NextRuns) != domain.CronNextRuns ||
		!first.NextRuns[0].Equal(time.Date(2026, 11, 24, 10, 17, 0, 0, time.UTC)) || first.LastRun == nil || !first.LastRun.Equal(ran) {
		t.Fatalf("hourly = %+v", first)
	}
	// The command logged in clear is matched once masked.
	if view.Entries[1].LastRun == nil || !view.Entries[1].LastRun.Equal(ran.Add(-2*time.Hour)) {
		t.Fatalf("masked match = %+v", view.Entries[1])
	}
	if view.Entries[2].Description != "" || len(view.Entries[2].NextRuns) != 0 {
		t.Fatalf("invalid entry = %+v", view.Entries[2])
	}
	if view.Directories[0].ScheduledBy == nil || view.Directories[0].ScheduledBy.Entry.Expression != "17 * * * *" || view.Directories[1].ScheduledBy != nil {
		t.Fatalf("directories = %+v", view.Directories)
	}
	if !view.TimersAvailable || len(view.Timers) != 1 || view.Timers[0].Service != "apt-daily.service" || view.Timers[0].ServiceResult != "exit-code" {
		t.Fatalf("timers = %+v", view.Timers)
	}
}

func TestCronInventoryWithoutJournal(t *testing.T) {
	cron := NewCronUsecase(fakeCronReader{}, nil, &fakeCronJournal{err: domain.ErrJournalUnavailable}, zap.NewNop())
	view, err := cron.Inventory(context.Background())
	if err != nil || view.JournalAvailable || view.TimersAvailable {
		t.Fatalf("view = %+v, %v", view, err)
	}
}

func TestCronInspect(t *testing.T) {
	cron := NewCronUsecase(fakeCronReader{}, nil, nil, zap.NewNop())
	cron.now, cron.location = func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) }, time.UTC
	inspection, err := cron.Inspect("0 9 * * 1-5")
	if err != nil || inspection.Description != "Du lundi au vendredi à 09:00" || len(inspection.NextRuns) != 5 ||
		!inspection.NextRuns[0].Equal(time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("inspect = %+v, %v", inspection, err)
	}
	if _, err := cron.Inspect("* * *"); !errors.Is(err, domain.ErrInvalidCronExpression) {
		t.Fatalf("invalid = %v", err)
	}
}
