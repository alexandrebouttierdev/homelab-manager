package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"go.uber.org/zap"
)

// fakeUnitReader returns the inventory it is given, or its error.
type fakeUnitReader struct {
	mu      sync.Mutex
	units   []domain.SystemUnit
	err     error
	block   bool
	details map[string]domain.SystemUnitDetail
	calls   int
}

func (f *fakeUnitReader) set(units []domain.SystemUnit, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.units, f.err = units, err
}

func (f *fakeUnitReader) Units(ctx context.Context) ([]domain.SystemUnit, error) {
	f.mu.Lock()
	f.calls++
	units, err, block := f.units, f.err, f.block
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return units, err
}

func (f *fakeUnitReader) Unit(_ context.Context, name string) (domain.SystemUnitDetail, error) {
	detail, ok := f.details[name]
	if !ok {
		return domain.SystemUnitDetail{}, domain.ErrUnitNotFound
	}
	return detail, nil
}

// memoryFailedUnits stands for the failed_units table and its events.
type memoryFailedUnits struct {
	mu       sync.Mutex
	set      map[string]struct{}
	events   []domain.MonitorEvent
	writeErr error
}

func newMemoryFailedUnits(names ...string) *memoryFailedUnits {
	repository := &memoryFailedUnits{set: map[string]struct{}{}}
	for _, name := range names {
		repository.set[name] = struct{}{}
	}
	return repository
}

func (m *memoryFailedUnits) FailedUnits(context.Context) (map[string]struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make(map[string]struct{}, len(m.set))
	for name := range m.set {
		copied[name] = struct{}{}
	}
	return copied, nil
}

func (m *memoryFailedUnits) ApplyFailureTransitions(_ context.Context, transitions domain.FailureTransitions, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	for _, name := range transitions.Removed {
		delete(m.set, name)
	}
	for _, name := range transitions.Added {
		m.set[name] = struct{}{}
	}
	m.events = append(m.events, transitions.Events...)
	return nil
}

func (m *memoryFailedUnits) eventTypes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	types := make([]string, len(m.events))
	for i, event := range m.events {
		types[i] = event.Type + " " + fmt.Sprint(event.Metadata["unit"])
	}
	return types
}

var servicesNow = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

func newTestServices(reader *fakeUnitReader, failed *memoryFailedUnits, journal *JournalUsecase) *ServiceUsecase {
	services := NewServiceUsecase(reader, failed, journal, zap.NewNop())
	services.now = func() time.Time { return servicesNow }
	return services
}

func TestServiceCollectionReportsAFailureOnce(t *testing.T) {
	ctx := context.Background()
	reader := &fakeUnitReader{units: []domain.SystemUnit{
		{Name: "backup.service", Type: domain.UnitTypeService, ActiveState: "failed", SubState: "failed"},
		{Name: "web.service", Type: domain.UnitTypeService, ActiveState: "active", SubState: "running"},
		{Name: "backup.timer", Type: domain.UnitTypeTimer, ActiveState: "active"},
	}}
	failed := newMemoryFailedUnits()
	services := newTestServices(reader, failed, nil)

	if inventory := services.Inventory(); inventory.Status != domain.UnitsPending || inventory.CollectedAt != nil {
		t.Fatalf("before the first cycle = %+v, want pending", inventory)
	}
	services.collect(ctx)
	services.collect(ctx)

	inventory := services.Inventory()
	if inventory.Status != domain.UnitsOK || inventory.CollectedAt == nil || !inventory.CollectedAt.Equal(servicesNow) {
		t.Fatalf("inventory = %+v", inventory)
	}
	if inventory.Counts != (domain.SystemUnitCounts{Total: 3, Active: 2, Failed: 1, ActiveTimers: 1}) {
		t.Errorf("counts = %+v", inventory.Counts)
	}
	if got := failed.eventTypes(); len(got) != 1 || got[0] != "service.failed backup.service" {
		t.Fatalf("events = %v, want one service.failed", got)
	}

	reader.set([]domain.SystemUnit{{Name: "backup.service", ActiveState: "active", SubState: "running"}}, nil)
	services.collect(ctx)
	if got := failed.eventTypes(); len(got) != 2 || got[1] != "service.recovered backup.service" {
		t.Fatalf("events = %v, want service.recovered", got)
	}
}

func TestServiceCollectionReadsTheRecordedSetAfterARestart(t *testing.T) {
	reader := &fakeUnitReader{units: []domain.SystemUnit{{Name: "backup.service", ActiveState: "failed"}}}
	failed := newMemoryFailedUnits("backup.service")
	newTestServices(reader, failed, nil).collect(context.Background())
	if got := failed.eventTypes(); len(got) != 0 {
		t.Fatalf("a failure recorded before the restart was reported again: %v", got)
	}
}

func TestServiceCollectionRetriesAFailedWrite(t *testing.T) {
	ctx := context.Background()
	reader := &fakeUnitReader{units: []domain.SystemUnit{{Name: "backup.service", ActiveState: "failed"}}}
	failed := newMemoryFailedUnits()
	failed.writeErr = errors.New("database is locked")
	services := newTestServices(reader, failed, nil)

	services.collect(ctx)
	if services.Inventory().Status != domain.UnitsOK {
		t.Fatal("a database failure must not hide the inventory")
	}
	failed.mu.Lock()
	failed.writeErr = nil
	failed.mu.Unlock()
	services.collect(ctx)
	if got := failed.eventTypes(); len(got) != 1 {
		t.Fatalf("events = %v, want the failure written by the next cycle", got)
	}
}

func TestServiceCollectionStatus(t *testing.T) {
	ctx := context.Background()
	reader := &fakeUnitReader{units: []domain.SystemUnit{{Name: "web.service", ActiveState: "active"}}}
	services := newTestServices(reader, newMemoryFailedUnits(), nil)
	services.collect(ctx)

	reader.set(nil, errors.New("org.freedesktop.DBus.Error.AccessDenied"))
	services.now = func() time.Time { return servicesNow.Add(time.Minute) }
	services.collect(ctx)
	inventory := services.Inventory()
	if inventory.Status != domain.UnitsError || len(inventory.Units) != 1 || !inventory.CollectedAt.Equal(servicesNow) {
		t.Fatalf("after an error = %+v, want the last inventory with status error", inventory)
	}

	reader.set(nil, fmt.Errorf("%w: no bus", domain.ErrSystemdUnavailable))
	services.collect(ctx)
	if status := services.Inventory().Status; status != domain.UnitsUnavailable {
		t.Fatalf("status = %s, want unavailable", status)
	}
}

func TestServiceCollectionIsBoundedInTime(t *testing.T) {
	reader := &fakeUnitReader{block: true}
	services := newTestServices(reader, newMemoryFailedUnits(), nil)
	services.cycleTimeout = 20 * time.Millisecond
	started := time.Now()
	services.collect(context.Background())
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cycle took %v", elapsed)
	}
	if status := services.Inventory().Status; status != domain.UnitsError {
		t.Fatalf("status = %s, want error after the timeout", status)
	}
}

func TestServiceRunCollectsAtOnceAndStopsWithItsContext(t *testing.T) {
	reader := &fakeUnitReader{}
	services := newTestServices(reader, newMemoryFailedUnits(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		services.Run(ctx)
	}()
	deadline := time.After(5 * time.Second)
	for services.Inventory().Status == domain.UnitsPending {
		select {
		case <-deadline:
			t.Fatal("no collection at start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop with its context")
	}
}

func TestServiceDetail(t *testing.T) {
	ctx := context.Background()
	reader := &fakeUnitReader{
		units: []domain.SystemUnit{{Name: "unit0.service", ActiveState: "active"}, {Name: "escaped\\x2d.service"}},
		details: map[string]domain.SystemUnitDetail{
			"unit0.service": {SystemUnit: domain.SystemUnit{Name: "unit0.service"}, FragmentPath: "/lib/systemd/system/unit0.service"},
		},
	}
	journal := NewJournalUsecase(newMemoryJournal(300, func(i int) string { return fmt.Sprintf("line %d", i) }))
	services := newTestServices(reader, newMemoryFailedUnits(), journal)

	if _, err := services.Detail(ctx, "unit0.service"); !errors.Is(err, domain.ErrSystemdUnavailable) {
		t.Fatalf("Detail before any inventory = %v, want ErrSystemdUnavailable", err)
	}
	services.collect(ctx)

	for name, want := range map[string]error{
		"unit0.mount":          domain.ErrInvalidUnitName,
		"escaped\\x2d.service": domain.ErrInvalidUnitName,
		"other.service":        domain.ErrUnitNotFound,
	} {
		if _, err := services.Detail(ctx, name); !errors.Is(err, want) {
			t.Errorf("Detail(%s) = %v, want %v", name, err, want)
		}
	}

	detail, err := services.Detail(ctx, "unit0.service")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Unit.FragmentPath != "/lib/systemd/system/unit0.service" || !detail.JournalAvailable {
		t.Fatalf("detail = %+v", detail)
	}
	if len(detail.Journal) != domain.UnitJournalLines {
		t.Fatalf("journal excerpt has %d entries, want %d", len(detail.Journal), domain.UnitJournalLines)
	}
	for _, entry := range detail.Journal {
		if entry.Unit != "unit0.service" {
			t.Fatalf("entry of another unit: %+v", entry)
		}
	}
	if detail.Journal[0].Message != "line 298" {
		t.Errorf("newest entry = %q, want line 298 (every priority, newest first)", detail.Journal[0].Message)
	}

	withoutJournal := newTestServices(reader, newMemoryFailedUnits(), NewJournalUsecase(nil))
	withoutJournal.collect(ctx)
	detail, err = withoutJournal.Detail(ctx, "unit0.service")
	if err != nil || detail.JournalAvailable || len(detail.Journal) != 0 {
		t.Fatalf("without journalctl = (%+v, %v), want the detail without excerpt", detail, err)
	}
}
