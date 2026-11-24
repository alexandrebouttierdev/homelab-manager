package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// PackageInventory reads the distribution and the installed system
// packages of the machine, without privilege. An unsupported distribution
// returns the inventory with its Distro and domain.ErrUnsupportedDistro.
type PackageInventory interface {
	Inventory(ctx context.Context) (domain.PackageInventory, error)
}

// VulnerabilityScanner hands an inventory to Grype through the dedicated
// systemd unit and decodes its result. A failure is a *domain.ScanError
// with its cause.
type VulnerabilityScanner interface {
	Scan(ctx context.Context, inventory domain.PackageInventory) (domain.ScanResult, error)
}

// VulnerabilityRepository stores the scans and the findings
// (SPEC_ADMINISTRATION.md § 8 ter.4).
type VulnerabilityRepository interface {
	// StartScan records a running scan; with audit (manual scan) both
	// commit together.
	StartScan(ctx context.Context, scan domain.Scan, audit *domain.AuditEntry) error
	// FailScan ends a running scan as failed.
	FailScan(ctx context.Context, id uuid.UUID, failure domain.ScanFailure, at time.Time) error
	// CompleteScan ends a running scan as succeeded and applies the
	// changes to the findings, in one transaction.
	CompleteScan(ctx context.Context, scan domain.Scan, changes domain.FindingChanges) error
	// InterruptRunningScans fails the scans left running by a stopped
	// Manager, with domain.FailureInterrupted.
	InterruptRunningScans(ctx context.Context, at time.Time) error
	// PresentFindings returns every finding not resolved.
	PresentFindings(ctx context.Context) ([]domain.Finding, error)
	// LatestScan returns the latest scan, nil when there is none.
	LatestScan(ctx context.Context) (*domain.Scan, error)
	// LatestSuccessfulScan returns the latest succeeded scan, or nil.
	LatestSuccessfulScan(ctx context.Context) (*domain.Scan, error)
	// LatestManualScanAt returns when the latest manual scan started, or nil.
	LatestManualScanAt(ctx context.Context) (*time.Time, error)
	// Scans returns the latest scans, most recent first.
	Scans(ctx context.Context, limit int) ([]domain.Scan, error)
	// Findings returns a page of the findings, most severe then most
	// recently found first; the filter is validated.
	Findings(ctx context.Context, filter domain.FindingFilter) (domain.FindingPage, error)
	Finding(ctx context.Context, id uuid.UUID) (domain.Finding, error)
	// Brief returns the totals, the fixable count, the latest scans and the
	// most recently found present findings.
	Brief(ctx context.Context) (domain.VulnerabilityBrief, error)
	// DeleteVulnerabilityHistoryBefore purges the scans and the resolved
	// findings that ended before the time, at most limit of each.
	DeleteVulnerabilityHistoryBefore(ctx context.Context, before time.Time, limit int) (int, error)
}
