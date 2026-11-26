package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"monoops/backend/internal/domain"
	"monoops/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// scanGuard bounds a whole scan, inventory and result included, above
	// the scanner's own 16 min limit.
	scanGuard = 20 * time.Minute
	// scanStoreTimeout bounds the database work of one scan.
	scanStoreTimeout = time.Minute
	// planRetry is the wait after the plan could not be read.
	planRetry               = time.Minute
	maxScansListed          = 100
	defaultScansListed      = 20
	auditTargetVulnScan     = "vulnerability_scan"
	vulnerabilityPurgeEvery = time.Hour
)

// VulnerabilitySummary is the state of the module for the interface.
type VulnerabilitySummary struct {
	domain.VulnerabilityBrief
	Running    bool
	NextScanAt *time.Time
	IntervalSettings
}

// IntervalSettings is the effective interval of the automatic scans, the
// configured one and where the effective one comes from ("config" or
// "user").
type IntervalSettings struct {
	Interval        time.Duration
	DefaultInterval time.Duration
	Source          string
}

// VulnerabilityUsecase scans the system packages with Grype
// (SPEC_ADMINISTRATION.md § 8 ter). Run owns the goroutine that runs every
// scan, automatic or manual: at most one at a time.
type VulnerabilityUsecase struct {
	repo      ports.VulnerabilityRepository
	settings  ports.VulnerabilitySettings
	inventory ports.PackageInventory
	scanner   ports.VulnerabilityScanner
	logger    *zap.Logger
	now       func() time.Time
	started   time.Time

	mu       sync.Mutex
	interval time.Duration
	// configured is the interval of the service configuration; fromUser
	// tells that interval comes from the interface.
	configured time.Duration
	fromUser   bool
	running    bool
	// requests carries the manual scans already recorded as running.
	requests chan domain.Scan
	// changed wakes Run when the interval changes.
	changed chan struct{}
}

func NewVulnerabilityUsecase(repo ports.VulnerabilityRepository, inventory ports.PackageInventory, scanner ports.VulnerabilityScanner,
	interval time.Duration, logger *zap.Logger) *VulnerabilityUsecase {
	now := time.Now().UTC()
	return &VulnerabilityUsecase{
		repo: repo, inventory: inventory, scanner: scanner, logger: logger, now: time.Now, started: now,
		interval: interval, configured: interval, requests: make(chan domain.Scan, 1), changed: make(chan struct{}, 1),
	}
}

// WithSettings reads and stores the interval chosen in the interface.
func (u *VulnerabilityUsecase) WithSettings(settings ports.VulnerabilitySettings) *VulnerabilityUsecase {
	u.settings = settings
	return u
}

// loadInterval applies the interval stored from the interface, if any.
func (u *VulnerabilityUsecase) loadInterval(ctx context.Context) {
	if u.settings == nil {
		return
	}
	hours, err := u.settings.VulnerabilityScanIntervalHours(ctx)
	if err != nil {
		u.logger.Warn("could not load the vulnerability scan interval; using the configured one", zap.Error(err))
		return
	}
	if hours != nil && domain.ValidateScanInterval(*hours) == nil {
		u.mu.Lock()
		u.interval, u.fromUser = time.Duration(*hours)*time.Hour, true
		u.mu.Unlock()
	}
}

// Intervals returns the effective and the configured intervals.
func (u *VulnerabilityUsecase) Intervals() IntervalSettings {
	u.mu.Lock()
	defer u.mu.Unlock()
	source := "config"
	if u.fromUser {
		source = "user"
	}
	return IntervalSettings{Interval: u.interval, DefaultInterval: u.configured, Source: source}
}

// UpdateInterval stores the interval of the automatic scans chosen by an
// administrator, audited, and applies it at once.
func (u *VulnerabilityUsecase) UpdateInterval(ctx context.Context, actor domain.User, hours int, requestID string) (IntervalSettings, error) {
	if err := domain.ValidateScanInterval(hours); err != nil {
		return IntervalSettings{}, err
	}
	if u.settings == nil {
		return IntervalSettings{}, errors.New("settings storage is unavailable")
	}
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: "settings", TargetID: "vulnerability_scan_interval_hours", Action: "update",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.settings.SetVulnerabilityScanIntervalHours(ctx, hours, audit); err != nil {
		return IntervalSettings{}, err
	}
	u.mu.Lock()
	u.fromUser = true
	u.mu.Unlock()
	u.SetInterval(time.Duration(hours) * time.Hour)
	return u.Intervals(), nil
}

// Interval returns the period of the automatic scans.
func (u *VulnerabilityUsecase) Interval() time.Duration {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.interval
}

// SetInterval changes the period of the automatic scans.
func (u *VulnerabilityUsecase) SetInterval(interval time.Duration) {
	u.mu.Lock()
	u.interval = interval
	u.mu.Unlock()
	select {
	case u.changed <- struct{}{}:
	default:
	}
}

// Run marks the scans left running by a previous Manager as interrupted,
// then runs the planned and the requested scans until ctx is cancelled;
// a scan in progress is then recorded as interrupted.
func (u *VulnerabilityUsecase) Run(ctx context.Context) {
	u.loadInterval(ctx)
	if err := u.repo.InterruptRunningScans(ctx, u.now().UTC()); err != nil {
		u.logger.Warn("could not close the interrupted vulnerability scans", zap.Error(err))
	}
	purge := time.NewTicker(vulnerabilityPurgeEvery)
	defer purge.Stop()
	u.purge(ctx)
	for {
		wait := u.untilNextScan(ctx)
		timer := time.NewTimer(wait.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-purge.C:
			u.purge(ctx)
		case <-u.changed:
		case scan := <-u.requests:
			u.execute(ctx, scan)
		case <-timer.C:
			if wait.planned {
				u.startPlanned(ctx, wait.trigger)
			}
		}
		timer.Stop()
	}
}

type plannedWait struct {
	delay   time.Duration
	trigger domain.ScanTrigger
	planned bool
}

func (u *VulnerabilityUsecase) untilNextScan(ctx context.Context) plannedWait {
	readCtx, cancel := context.WithTimeout(ctx, scanStoreTimeout)
	defer cancel()
	at, trigger, ok, err := u.plan(readCtx)
	if err != nil {
		u.logger.Warn("could not plan the next vulnerability scan", zap.Error(err))
		return plannedWait{delay: planRetry}
	}
	if !ok {
		return plannedWait{delay: planRetry}
	}
	return plannedWait{delay: max(at.Sub(u.now()), 0), trigger: trigger, planned: true}
}

func (u *VulnerabilityUsecase) plan(ctx context.Context) (time.Time, domain.ScanTrigger, bool, error) {
	latest, err := u.repo.LatestScan(ctx)
	if err != nil {
		return time.Time{}, "", false, err
	}
	success, err := u.repo.LatestSuccessfulScan(ctx)
	if err != nil {
		return time.Time{}, "", false, err
	}
	at, trigger, ok := domain.PlanNextScan(latest, success != nil, u.started, u.Interval())
	return at, trigger, ok, nil
}

func (u *VulnerabilityUsecase) claim() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.running {
		return false
	}
	u.running = true
	return true
}

func (u *VulnerabilityUsecase) release() {
	u.mu.Lock()
	u.running = false
	u.mu.Unlock()
}

func (u *VulnerabilityUsecase) startPlanned(ctx context.Context, trigger domain.ScanTrigger) {
	if !u.claim() {
		return
	}
	scan := domain.Scan{ID: uuid.New(), Trigger: trigger, State: domain.ScanRunning, StartedAt: u.now().UTC()}
	storeCtx, cancel := context.WithTimeout(ctx, scanStoreTimeout)
	err := u.repo.StartScan(storeCtx, scan, nil)
	cancel()
	if err != nil {
		u.release()
		u.logger.Warn("could not record a vulnerability scan", zap.Error(err))
		return
	}
	u.execute(ctx, scan)
}

// execute runs a scan already recorded as running and claimed.
func (u *VulnerabilityUsecase) execute(ctx context.Context, scan domain.Scan) {
	defer u.release()
	scanCtx, cancel := context.WithTimeout(ctx, scanGuard)
	defer cancel()
	failure, err := u.scan(scanCtx, scan)
	if err == nil {
		return
	}
	if ctx.Err() != nil {
		failure = domain.FailureInterrupted
	}
	storeCtx, cancelStore := context.WithTimeout(context.WithoutCancel(ctx), scanStoreTimeout)
	defer cancelStore()
	if storeErr := u.repo.FailScan(storeCtx, scan.ID, failure, u.now().UTC()); storeErr != nil {
		u.logger.Warn("could not record a failed vulnerability scan", zap.Error(storeErr))
	}
	u.logger.Warn("vulnerability scan failed", zap.String("scan_id", scan.ID.String()),
		zap.String("failure", string(failure)), zap.Error(err))
}

func (u *VulnerabilityUsecase) scan(ctx context.Context, scan domain.Scan) (domain.ScanFailure, error) {
	inventory, err := u.inventory.Inventory(ctx)
	if errors.Is(err, domain.ErrUnsupportedDistro) {
		return domain.FailureUnsupportedDistro, err
	}
	if err != nil {
		return domain.FailureInventory, err
	}
	result, err := u.scanner.Scan(ctx, inventory)
	if err != nil {
		var scanErr *domain.ScanError
		if errors.As(err, &scanErr) {
			return scanErr.Failure, err
		}
		return domain.FailureScan, err
	}
	present, err := u.repo.PresentFindings(ctx)
	if err != nil {
		return domain.FailureScan, err
	}
	finished := u.now().UTC()
	changes := domain.MergeScanResult(present, result, scan.ID, finished)
	scan.State, scan.FinishedAt = domain.ScanSucceeded, &finished
	scan.GrypeVersion, scan.DBBuiltAt = result.GrypeVersion, result.DBBuiltAt
	scan.Distro, scan.PackageCount = inventory.Distro, len(inventory.Packages)
	if err := u.repo.CompleteScan(ctx, scan, changes); err != nil {
		return domain.FailureScan, err
	}
	u.logger.Info("vulnerability scan succeeded", zap.String("scan_id", scan.ID.String()), zap.Int("packages", scan.PackageCount),
		zap.Int("findings", changes.Totals.Sum()), zap.Int("new", len(changes.New)), zap.Int("resolved", len(changes.Resolved)))
	return "", nil
}

func (u *VulnerabilityUsecase) purge(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, scanStoreTimeout)
	defer cancel()
	before := u.now().UTC().Add(-domain.VulnerabilityRetention)
	for {
		deleted, err := u.repo.DeleteVulnerabilityHistoryBefore(ctx, before, purgeBatchSize)
		if err != nil {
			u.logger.Warn("could not purge the vulnerability history", zap.Error(err))
			return
		}
		if deleted < purgeBatchSize {
			return
		}
	}
}

// RequestScan records a manual scan as running and hands it to Run. One
// scan at a time (domain.ErrScanInProgress), one manual scan per 5 min
// (domain.ErrScanRateLimited); the request is audited with the scan.
func (u *VulnerabilityUsecase) RequestScan(ctx context.Context, actor domain.User, requestID string) (domain.Scan, error) {
	if !u.claim() {
		return domain.Scan{}, domain.ErrScanInProgress
	}
	now := u.now().UTC()
	last, err := u.repo.LatestManualScanAt(ctx)
	if err != nil {
		u.release()
		return domain.Scan{}, err
	}
	if last != nil && now.Sub(*last) < domain.ManualScanCooldown {
		u.release()
		return domain.Scan{}, domain.ErrScanRateLimited
	}
	scan := domain.Scan{ID: uuid.New(), Trigger: domain.ScanManual, RequestedBy: actor.Username, State: domain.ScanRunning, StartedAt: now}
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetVulnScan, TargetID: scan.ID.String(), Action: "start",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.repo.StartScan(ctx, scan, &audit); err != nil {
		u.release()
		return domain.Scan{}, err
	}
	// The channel holds one scan and only a claimed scan is sent: it never
	// blocks.
	u.requests <- scan
	return scan, nil
}

func (u *VulnerabilityUsecase) Summary(ctx context.Context) (VulnerabilitySummary, error) {
	brief, err := u.repo.Brief(ctx)
	if err != nil {
		return VulnerabilitySummary{}, err
	}
	summary := VulnerabilitySummary{VulnerabilityBrief: brief, IntervalSettings: u.Intervals()}
	summary.Running = brief.LatestScan != nil && brief.LatestScan.State == domain.ScanRunning
	if at, _, ok := domain.PlanNextScan(brief.LatestScan, brief.LastSuccess != nil, u.started, summary.Interval); ok {
		at = at.UTC()
		summary.NextScanAt = &at
	}
	return summary, nil
}

func (u *VulnerabilityUsecase) Findings(ctx context.Context, filter domain.FindingFilter) (domain.FindingPage, error) {
	filter, err := filter.Validate()
	if err != nil {
		return domain.FindingPage{}, err
	}
	return u.repo.Findings(ctx, filter)
}

func (u *VulnerabilityUsecase) Finding(ctx context.Context, id uuid.UUID) (domain.Finding, error) {
	return u.repo.Finding(ctx, id)
}

// Scans lists the latest scans; limit 1 to 100, 0 for the default 20.
func (u *VulnerabilityUsecase) Scans(ctx context.Context, limit int) ([]domain.Scan, error) {
	if limit < 0 || limit > maxScansListed {
		return nil, domain.ErrInvalidFindingFilter
	}
	if limit == 0 {
		limit = defaultScansListed
	}
	return u.repo.Scans(ctx, limit)
}
