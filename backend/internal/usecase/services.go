package usecase

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"go.uber.org/zap"
)

const (
	// unitsInterval is the collection period of SPEC_ADMINISTRATION.md
	// § 5.1.
	unitsInterval = 30 * time.Second
	// unitsCycleTimeout bounds one collection, D-Bus and database included,
	// below the period so that cycles never overlap.
	unitsCycleTimeout = 25 * time.Second
	// unitDetailTimeout bounds the on-demand reads of a unit detail.
	unitDetailTimeout = 10 * time.Second
)

// ServiceUsecase collects the systemd units, read-only (SPEC_ADMINISTRATION.md
// § 5.1). Run owns the collection goroutine; the inventory is kept in
// memory and served to every role.
type ServiceUsecase struct {
	reader ports.SystemUnitReader
	// failed is nil when the database is unavailable: the inventory is
	// still served, without events.
	failed  ports.FailedUnitRepository
	journal *JournalUsecase
	logger  *zap.Logger
	now     func() time.Time

	interval     time.Duration
	cycleTimeout time.Duration

	mu        sync.RWMutex
	inventory UnitInventory
	// failedSet mirrors failed_units; nil until it was read.
	failedSet map[string]struct{}
}

// UnitInventory is the last collected inventory. CollectedAt is the time
// of the last successful cycle: an error keeps the previous units.
type UnitInventory struct {
	Status      domain.SystemUnitsStatus
	CollectedAt *time.Time
	Counts      domain.SystemUnitCounts
	Units       []domain.SystemUnit
}

// UnitDetail is a unit definition with the newest entries of its journal.
type UnitDetail struct {
	Unit domain.SystemUnitDetail
	// Journal goes from the newest entry to the oldest.
	Journal          []domain.JournalEntry
	JournalAvailable bool
}

func NewServiceUsecase(reader ports.SystemUnitReader, failed ports.FailedUnitRepository, journal *JournalUsecase, logger *zap.Logger) *ServiceUsecase {
	return &ServiceUsecase{
		reader: reader, failed: failed, journal: journal, logger: logger, now: time.Now,
		interval: unitsInterval, cycleTimeout: unitsCycleTimeout,
		inventory: UnitInventory{Status: domain.UnitsPending, Units: []domain.SystemUnit{}},
	}
}

// Run collects at once, then every 30 s, until ctx is cancelled.
func (u *ServiceUsecase) Run(ctx context.Context) {
	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()
	for {
		u.collect(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Inventory returns the last inventory; its units must not be modified.
func (u *ServiceUsecase) Inventory() UnitInventory {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.inventory
}

func (u *ServiceUsecase) collect(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, u.cycleTimeout)
	defer cancel()
	units, err := u.reader.Units(ctx)
	now := u.now().UTC()
	if err != nil {
		status := domain.UnitsError
		if errors.Is(err, domain.ErrSystemdUnavailable) {
			status = domain.UnitsUnavailable
		}
		u.mu.Lock()
		previous := u.inventory.Status
		u.inventory.Status = status
		u.mu.Unlock()
		// An unavailable systemd (development machine, container) is logged
		// once, not every 30 s.
		if status != domain.UnitsUnavailable || previous != domain.UnitsUnavailable {
			u.logger.Warn("system units collection failed", zap.String("status", string(status)), zap.Error(err))
		}
		return
	}
	u.mu.Lock()
	u.inventory = UnitInventory{
		Status: domain.UnitsOK, CollectedAt: &now, Counts: domain.CountUnits(units), Units: units,
	}
	u.mu.Unlock()
	u.recordFailures(ctx, units, now)
}

// recordFailures compares the inventory with the recorded failed set and
// writes the transitions. On a database error the set is left unchanged:
// the next cycle computes the same transitions again.
func (u *ServiceUsecase) recordFailures(ctx context.Context, units []domain.SystemUnit, now time.Time) {
	if u.failed == nil {
		return
	}
	if u.failedSet == nil {
		set, err := u.failed.FailedUnits(ctx)
		if err != nil {
			u.logger.Warn("could not read the failed units", zap.Error(err))
			return
		}
		u.failedSet = set
	}
	transitions := domain.UnitFailureTransitions(u.failedSet, units, now)
	if len(transitions.Added) == 0 && len(transitions.Removed) == 0 {
		return
	}
	if err := u.failed.ApplyFailureTransitions(ctx, transitions, now); err != nil {
		u.logger.Warn("could not record the failed units", zap.Error(err))
		return
	}
	for _, name := range transitions.Removed {
		delete(u.failedSet, name)
	}
	for _, name := range transitions.Added {
		u.failedSet[name] = struct{}{}
	}
	for _, event := range transitions.Events {
		u.logger.Info("system unit state", zap.String("event", event.Type), zap.Any("unit", event.Metadata["unit"]))
	}
}

// Detail reads a unit of the last inventory and the newest entries of its
// journal. A journal that cannot be read leaves the excerpt empty.
func (u *ServiceUsecase) Detail(ctx context.Context, name string) (UnitDetail, error) {
	if !domain.ValidUnitName(name) {
		return UnitDetail{}, domain.ErrInvalidUnitName
	}
	inventory := u.Inventory()
	if inventory.CollectedAt == nil {
		return UnitDetail{}, domain.ErrSystemdUnavailable
	}
	if !slices.ContainsFunc(inventory.Units, func(unit domain.SystemUnit) bool { return unit.Name == name }) {
		return UnitDetail{}, domain.ErrUnitNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, unitDetailTimeout)
	defer cancel()
	unit, err := u.reader.Unit(ctx, name)
	if err != nil {
		return UnitDetail{}, err
	}
	detail := UnitDetail{Unit: unit, Journal: []domain.JournalEntry{}}
	if u.journal == nil {
		return detail, nil
	}
	page, err := u.journal.List(ctx, JournalQuery{
		Filter: domain.JournalFilter{Unit: name, Priority: len(domain.JournalPriorities) - 1},
		Limit:  domain.UnitJournalLines,
	})
	switch {
	case errors.Is(err, domain.ErrJournalUnavailable):
	case err != nil:
		u.logger.Warn("could not read the journal of a unit", zap.String("unit", name), zap.Error(err))
	default:
		detail.Journal, detail.JournalAvailable = page.Entries, true
	}
	return detail, nil
}
