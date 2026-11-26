package sqlite

import (
	"context"
	"testing"
	"time"

	"monoops/backend/internal/domain"
)

func TestFailedUnitsAndTheirEventsCommitTogether(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	units := NewSystemUnitRepository(database.Client())
	monitoring := NewMonitoringRepository(database)
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

	failed, err := units.FailedUnits(ctx)
	if err != nil || len(failed) != 0 {
		t.Fatalf("FailedUnits on a new database = (%v, %v)", failed, err)
	}

	first := domain.UnitFailureTransitions(failed, []domain.SystemUnit{
		{Name: "backup.service", ActiveState: "failed", SubState: "failed", Result: "exit-code"},
		{Name: "web.service", ActiveState: "failed", SubState: "failed", Result: "signal"},
	}, at)
	if err := units.ApplyFailureTransitions(ctx, first, at); err != nil {
		t.Fatalf("ApplyFailureTransitions: %v", err)
	}
	failed, err = units.FailedUnits(ctx)
	if err != nil || len(failed) != 2 {
		t.Fatalf("FailedUnits = (%v, %v), want two units", failed, err)
	}

	// A restart of the Manager reads the same set: no duplicate event.
	again := domain.UnitFailureTransitions(failed, []domain.SystemUnit{
		{Name: "backup.service", ActiveState: "failed"},
		{Name: "web.service", ActiveState: "failed"},
	}, at.Add(time.Minute))
	if len(again.Events) != 0 {
		t.Fatalf("a known failure produced events: %+v", again.Events)
	}

	later := at.Add(2 * time.Minute)
	second := domain.UnitFailureTransitions(failed, []domain.SystemUnit{
		{Name: "web.service", ActiveState: "active", SubState: "running", Result: "success"},
	}, later)
	if err := units.ApplyFailureTransitions(ctx, second, later); err != nil {
		t.Fatalf("ApplyFailureTransitions: %v", err)
	}
	failed, err = units.FailedUnits(ctx)
	if err != nil || len(failed) != 0 {
		t.Fatalf("after recovery and disappearance, FailedUnits = (%v, %v), want none", failed, err)
	}

	events, err := monitoring.ListEvents(ctx, nil, at.Add(-time.Hour), at.Add(time.Hour), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %+v, want two failures and one recovery", events)
	}
	newest := events[0]
	if newest.Type != domain.EventServiceRecovered || newest.Severity != "info" || newest.ContainerID != nil ||
		newest.Metadata["unit"] != "web.service" || newest.Metadata["result"] != "success" {
		t.Errorf("recovery event = %+v", newest)
	}
	for _, event := range events[1:] {
		if event.Type != domain.EventServiceFailed || event.Severity != "error" || event.Metadata["activeState"] != "failed" {
			t.Errorf("failure event = %+v", event)
		}
	}
}

func TestFailedUnitsSurviveTheEventPurge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	units := NewSystemUnitRepository(database.Client())
	monitoring := NewMonitoringRepository(database)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	transitions := domain.UnitFailureTransitions(nil, []domain.SystemUnit{{Name: "old.service", ActiveState: "failed"}}, at)
	if err := units.ApplyFailureTransitions(ctx, transitions, at); err != nil {
		t.Fatal(err)
	}
	if _, err := monitoring.DeleteEventsBefore(ctx, at.Add(time.Hour), 1000); err != nil {
		t.Fatal(err)
	}
	failed, err := units.FailedUnits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := failed["old.service"]; !ok {
		t.Fatalf("the failed set lost its unit with the event purge: %v", failed)
	}
}

func TestApplyFailureTransitionsWithoutChangeWritesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database := openTestDatabase(t, ctx)
	units := NewSystemUnitRepository(database.Client())
	if err := units.ApplyFailureTransitions(ctx, domain.FailureTransitions{}, time.Now()); err != nil {
		t.Fatal(err)
	}
}
