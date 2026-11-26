package usecase

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"monoops/backend/internal/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// memoryVulnerabilities stands for the scan and finding tables.
type memoryVulnerabilities struct {
	mu          sync.Mutex
	scans       []domain.Scan
	findings    []domain.Finding
	audits      []domain.AuditEntry
	interrupted int
	purged      []time.Time
}

func (m *memoryVulnerabilities) StartScan(_ context.Context, scan domain.Scan, audit *domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scans = append(m.scans, scan)
	if audit != nil {
		m.audits = append(m.audits, *audit)
	}
	return nil
}

func (m *memoryVulnerabilities) update(id uuid.UUID, change func(*domain.Scan)) {
	for i := range m.scans {
		if m.scans[i].ID == id {
			change(&m.scans[i])
		}
	}
}

func (m *memoryVulnerabilities) FailScan(_ context.Context, id uuid.UUID, failure domain.ScanFailure, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.update(id, func(s *domain.Scan) { s.State, s.Failure, s.FinishedAt = domain.ScanFailed, failure, &at })
	return nil
}

func (m *memoryVulnerabilities) CompleteScan(_ context.Context, scan domain.Scan, changes domain.FindingChanges) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	scan.Totals, scan.NewCount, scan.ResolvedCount = changes.Totals, len(changes.New), len(changes.Resolved)
	m.update(scan.ID, func(s *domain.Scan) { *s = scan })
	for _, id := range changes.Resolved {
		for i := range m.findings {
			if m.findings[i].ID == id {
				at := *scan.FinishedAt
				m.findings[i].ResolvedAt = &at
			}
		}
	}
	for _, seen := range changes.Seen {
		for i := range m.findings {
			if m.findings[i].ID == seen.ID {
				m.findings[i] = seen
			}
		}
	}
	m.findings = append(m.findings, changes.New...)
	return nil
}

func (m *memoryVulnerabilities) InterruptRunningScans(_ context.Context, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.scans {
		if m.scans[i].State == domain.ScanRunning {
			m.scans[i].State, m.scans[i].Failure, m.scans[i].FinishedAt = domain.ScanFailed, domain.FailureInterrupted, &at
			m.interrupted++
		}
	}
	return nil
}

func (m *memoryVulnerabilities) PresentFindings(context.Context) ([]domain.Finding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var present []domain.Finding
	for _, finding := range m.findings {
		if finding.ResolvedAt == nil {
			present = append(present, finding)
		}
	}
	return present, nil
}

func (m *memoryVulnerabilities) latest(keep func(domain.Scan) bool) *domain.Scan {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.scans) - 1; i >= 0; i-- {
		if keep(m.scans[i]) {
			scan := m.scans[i]
			return &scan
		}
	}
	return nil
}

func (m *memoryVulnerabilities) LatestScan(context.Context) (*domain.Scan, error) {
	return m.latest(func(domain.Scan) bool { return true }), nil
}

func (m *memoryVulnerabilities) LatestSuccessfulScan(context.Context) (*domain.Scan, error) {
	return m.latest(func(s domain.Scan) bool { return s.State == domain.ScanSucceeded }), nil
}

func (m *memoryVulnerabilities) LatestManualScanAt(context.Context) (*time.Time, error) {
	if scan := m.latest(func(s domain.Scan) bool { return s.Trigger == domain.ScanManual }); scan != nil {
		return &scan.StartedAt, nil
	}
	return nil, nil
}

func (m *memoryVulnerabilities) Scans(_ context.Context, limit int) ([]domain.Scan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	scans := slices.Clone(m.scans)
	slices.Reverse(scans)
	return scans[:min(limit, len(scans))], nil
}

func (m *memoryVulnerabilities) Findings(context.Context, domain.FindingFilter) (domain.FindingPage, error) {
	present, _ := m.PresentFindings(context.Background())
	return domain.FindingPage{Findings: present, Total: len(present)}, nil
}

func (m *memoryVulnerabilities) Finding(_ context.Context, id uuid.UUID) (domain.Finding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, finding := range m.findings {
		if finding.ID == id {
			return finding, nil
		}
	}
	return domain.Finding{}, domain.ErrFindingNotFound
}

func (m *memoryVulnerabilities) Brief(ctx context.Context) (domain.VulnerabilityBrief, error) {
	brief := domain.VulnerabilityBrief{Totals: domain.NewSeverityTotals()}
	present, _ := m.PresentFindings(ctx)
	for _, finding := range present {
		brief.Totals[finding.Severity]++
	}
	brief.LatestScan, _ = m.LatestScan(ctx)
	brief.LastSuccess, _ = m.LatestSuccessfulScan(ctx)
	return brief, nil
}

func (m *memoryVulnerabilities) DeleteVulnerabilityHistoryBefore(_ context.Context, before time.Time, _ int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purged = append(m.purged, before)
	return 0, nil
}

func (m *memoryVulnerabilities) scan(id uuid.UUID) domain.Scan {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, scan := range m.scans {
		if scan.ID == id {
			return scan
		}
	}
	return domain.Scan{}
}

type fakeInventory struct {
	inventory domain.PackageInventory
	err       error
}

func (f fakeInventory) Inventory(context.Context) (domain.PackageInventory, error) {
	return f.inventory, f.err
}

// fakeScanner returns its result or error; block makes it wait for the
// context, like a scan stopped with the Manager.
type fakeScanner struct {
	mu     sync.Mutex
	result domain.ScanResult
	err    error
	block  bool
	calls  int
}

func (f *fakeScanner) Scan(ctx context.Context, _ domain.PackageInventory) (domain.ScanResult, error) {
	f.mu.Lock()
	f.calls++
	block, result, err := f.block, f.result, f.err
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureTimeout, Err: ctx.Err()}
	}
	return result, err
}

var vulnBase = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func newVulnerabilityFixture(inventory fakeInventory, scanner *fakeScanner) (*VulnerabilityUsecase, *memoryVulnerabilities, *time.Time) {
	repo := &memoryVulnerabilities{}
	now := vulnBase
	usecase := NewVulnerabilityUsecase(repo, inventory, scanner, domain.DefaultScanInterval, zap.NewNop())
	usecase.now = func() time.Time { return now }
	usecase.started = vulnBase
	return usecase, repo, &now
}

var debianInventory = fakeInventory{inventory: domain.PackageInventory{
	Distro: domain.Distro{ID: "debian", VersionID: "13"}, Manager: domain.PackageManagerDpkg,
	Packages: []domain.InstalledPackage{{Name: "openssl", Version: "3.5"}, {Name: "bash", Version: "5.2"}},
}}

func TestPlannedScanStoresTheFindings(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{GrypeVersion: "0.121.3", Matches: []domain.ScanMatch{
		{VulnerabilityID: "CVE-1", Severity: domain.VulnCritical, Package: "openssl", Version: "3.5", FixState: domain.FixAvailable},
	}}}
	usecase, repo, now := newVulnerabilityFixture(debianInventory, scanner)
	*now = vulnBase.Add(domain.StartupScanDelay)
	usecase.startPlanned(context.Background(), domain.ScanStartup)
	stored := repo.scans[0]
	if stored.State != domain.ScanSucceeded || stored.Trigger != domain.ScanStartup || stored.PackageCount != 2 ||
		stored.Distro.ID != "debian" || stored.Totals[domain.VulnCritical] != 1 || stored.NewCount != 1 || stored.GrypeVersion != "0.121.3" {
		t.Fatalf("scan = %+v", stored)
	}
	summary, err := usecase.Summary(context.Background())
	if err != nil || summary.Running || summary.NextScanAt == nil || !summary.NextScanAt.Equal(now.Add(24*time.Hour)) || summary.Totals[domain.VulnCritical] != 1 {
		t.Fatalf("Summary() = (%+v, %v)", summary, err)
	}
}

func TestScanFailuresAreClassified(t *testing.T) {
	for name, test := range map[string]struct {
		inventory fakeInventory
		scanErr   error
		want      domain.ScanFailure
	}{
		"unsupported distro": {fakeInventory{err: domain.ErrUnsupportedDistro}, nil, domain.FailureUnsupportedDistro},
		"unreadable dpkg":    {fakeInventory{err: errors.New("permission denied")}, nil, domain.FailureInventory},
		"polkit refusal":     {debianInventory, &domain.ScanError{Failure: domain.FailureNotAllowed}, domain.FailureNotAllowed},
		"other error":        {debianInventory, errors.New("boom"), domain.FailureScan},
	} {
		usecase, repo, _ := newVulnerabilityFixture(test.inventory, &fakeScanner{err: test.scanErr})
		usecase.startPlanned(context.Background(), domain.ScanSchedule)
		if scan := repo.scans[0]; scan.State != domain.ScanFailed || scan.Failure != test.want || scan.FinishedAt == nil {
			t.Errorf("%s: scan = %+v, want failed %q", name, scan, test.want)
		}
		if usecase.running {
			t.Errorf("%s: the lock is still held", name)
		}
	}
}

func TestManualScanIsLockedRateLimitedAuditedAndRun(t *testing.T) {
	scanner := &fakeScanner{result: domain.ScanResult{}}
	usecase, repo, now := newVulnerabilityFixture(debianInventory, scanner)
	operator := domain.User{Username: "otto"}
	usecase.running = true
	if _, err := usecase.RequestScan(context.Background(), operator, "req"); !errors.Is(err, domain.ErrScanInProgress) {
		t.Fatalf("RequestScan during a scan = %v", err)
	}
	usecase.running = false
	scan, err := usecase.RequestScan(context.Background(), operator, "req-1")
	if err != nil || scan.State != domain.ScanRunning || scan.RequestedBy != "otto" {
		t.Fatalf("RequestScan() = (%+v, %v)", scan, err)
	}
	if len(repo.audits) != 1 || repo.audits[0].TargetType != "vulnerability_scan" || repo.audits[0].Action != "start" || repo.audits[0].RequestID != "req-1" {
		t.Fatalf("audits = %+v", repo.audits)
	}
	if _, err := usecase.RequestScan(context.Background(), operator, "req"); !errors.Is(err, domain.ErrScanInProgress) {
		t.Fatalf("second request before the first ran = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		usecase.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for repo.scan(scan.ID).State == domain.ScanRunning && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if state := repo.scan(scan.ID).State; state != domain.ScanSucceeded {
		t.Fatalf("manual scan state = %q, want succeeded", state)
	}
	*now = now.Add(time.Minute)
	if _, err := usecase.RequestScan(context.Background(), operator, "req"); !errors.Is(err, domain.ErrScanRateLimited) {
		t.Fatalf("request 1 min after = %v, want ErrScanRateLimited", err)
	}
	*now = now.Add(domain.ManualScanCooldown)
	if _, err := usecase.RequestScan(context.Background(), operator, "req"); err != nil {
		t.Fatalf("request after the cooldown = %v", err)
	}
}

func TestRunInterruptsLeftoversAndAScanStoppedWithTheManager(t *testing.T) {
	scanner := &fakeScanner{block: true}
	usecase, repo, _ := newVulnerabilityFixture(debianInventory, scanner)
	leftover := domain.Scan{ID: uuid.New(), Trigger: domain.ScanSchedule, State: domain.ScanRunning, StartedAt: vulnBase.Add(-time.Hour)}
	repo.scans = []domain.Scan{leftover}
	// The leftover scan ran before this start: a startup scan is due at
	// once (the clock is past the startup delay).
	usecase.started = vulnBase.Add(-domain.StartupScanDelay)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		usecase.Run(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		scanner.mu.Lock()
		calls := scanner.calls
		scanner.mu.Unlock()
		if calls > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if repo.interrupted != 1 || repo.scan(leftover.ID).Failure != domain.FailureInterrupted {
		t.Fatalf("leftover = %+v", repo.scan(leftover.ID))
	}
	running := repo.scans[len(repo.scans)-1]
	if running.ID == leftover.ID || running.Trigger != domain.ScanStartup || running.State != domain.ScanFailed || running.Failure != domain.FailureInterrupted {
		t.Fatalf("scan stopped with the Manager = %+v", running)
	}
	if len(repo.purged) != 1 || !repo.purged[0].Equal(vulnBase.Add(-domain.VulnerabilityRetention)) {
		t.Fatalf("purged = %v", repo.purged)
	}
}

func TestScansLimit(t *testing.T) {
	usecase, _, _ := newVulnerabilityFixture(debianInventory, &fakeScanner{})
	if _, err := usecase.Scans(context.Background(), 101); !errors.Is(err, domain.ErrInvalidFindingFilter) {
		t.Fatalf("Scans(101) = %v", err)
	}
	if scans, err := usecase.Scans(context.Background(), 0); err != nil || len(scans) != 0 {
		t.Fatalf("Scans(0) = (%v, %v)", scans, err)
	}
	usecase.SetInterval(48 * time.Hour)
	if usecase.Interval() != 48*time.Hour {
		t.Fatal("SetInterval did not apply")
	}
}

type memoryScanSettings struct {
	hours  *int
	audits []domain.AuditEntry
}

func (m *memoryScanSettings) VulnerabilityScanIntervalHours(context.Context) (*int, error) {
	return m.hours, nil
}

func (m *memoryScanSettings) SetVulnerabilityScanIntervalHours(_ context.Context, hours int, audit domain.AuditEntry) error {
	m.hours, m.audits = &hours, append(m.audits, audit)
	return nil
}

func TestScanIntervalFromTheInterface(t *testing.T) {
	usecase, _, _ := newVulnerabilityFixture(debianInventory, &fakeScanner{})
	stored := 72
	settings := &memoryScanSettings{hours: &stored}
	usecase.WithSettings(settings)
	usecase.loadInterval(context.Background())
	if got := usecase.Intervals(); got.Interval != 72*time.Hour || got.DefaultInterval != 24*time.Hour || got.Source != "user" {
		t.Fatalf("Intervals() after loading = %+v", got)
	}
	if _, err := usecase.UpdateInterval(context.Background(), domain.User{Username: "admin"}, 5, "req"); !errors.Is(err, domain.ErrInvalidScanInterval) {
		t.Fatalf("UpdateInterval(5) = %v", err)
	}
	got, err := usecase.UpdateInterval(context.Background(), domain.User{Username: "admin"}, 12, "req")
	if err != nil || got.Interval != 12*time.Hour || *settings.hours != 12 || settings.audits[0].TargetID != "vulnerability_scan_interval_hours" {
		t.Fatalf("UpdateInterval(12) = (%+v, %v)", got, err)
	}
	select {
	case <-usecase.changed:
	default:
		t.Fatal("the scheduler was not woken")
	}
}

type countSource struct {
	counts []domain.VulnerabilityCount
	known  bool
}

func (c countSource) VulnerabilityCounts(context.Context) ([]domain.VulnerabilityCount, bool, error) {
	return c.counts, c.known, nil
}

func TestAlertEngineReadsTheVulnerabilityCounts(t *testing.T) {
	usecase, repo, _, _ := newAlertFixture(t)
	usecase.sources.Vulnerabilities = countSource{known: true, counts: []domain.VulnerabilityCount{{Severity: domain.VulnCritical, Count: 1}}}
	repo.rules = []domain.AlertRule{{ID: uuid.New(), Name: "vulns", Type: domain.AlertRuleVulnerabilities, Severity: domain.AlertCritical, Enabled: true,
		Params: domain.AlertRuleParams{VulnSeverity: domain.VulnCritical}}}
	usecase.evaluate(context.Background())
	if open := repo.open(); len(open) != 1 || open[0].Target.Kind != domain.AlertTargetVulnerabilities {
		t.Fatalf("open = %+v", open)
	}
}
