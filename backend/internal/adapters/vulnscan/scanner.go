package vulnscan

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

// Defaults of the deployment (deployments/systemd/homelab-vulnscan.service,
// deployments/tmpfiles/homelab-vulnscan.conf).
const (
	DefaultUnit          = "homelab-vulnscan.service"
	DefaultInventoryPath = "/var/lib/homelab-vulnscan/inventory/inventory.json"
	DefaultResultPath    = "/var/lib/homelab-vulnscan/result/result.json"
	// DefaultTimeout leaves the unit its TimeoutStartSec=15min, plus the
	// time systemd needs to stop it.
	DefaultTimeout = 16 * time.Minute
)

// Config locates the exchange files and the unit.
type Config struct {
	Unit          string
	InventoryPath string
	ResultPath    string
	Timeout       time.Duration
	// Version of the Manager, written in the inventory descriptor.
	Version string
}

// unitRunner starts a unit and waits for it (UnitStarter).
type unitRunner interface {
	Run(ctx context.Context, unit string) error
}

var _ ports.VulnerabilityScanner = (*Scanner)(nil)

// Scanner writes the inventory, starts the unit and reads the result.
type Scanner struct {
	config Config
	units  unitRunner
	now    func() time.Time
}

func NewScanner(config Config, units unitRunner) *Scanner {
	return &Scanner{config: config, units: units, now: time.Now}
}

func (s *Scanner) Scan(ctx context.Context, inventory domain.PackageInventory) (domain.ScanResult, error) {
	// Some file systems keep modification times to the second.
	started := s.now().Truncate(time.Second)
	if err := writeInventory(s.config.InventoryPath, inventory, s.config.Version); err != nil {
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureInventory, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	if err := s.units.Run(ctx, s.config.Unit); err != nil {
		return domain.ScanResult{}, err
	}
	return readResult(s.config.ResultPath, started)
}
