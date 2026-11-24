package vulnscan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"homelab-manager/backend/internal/domain"
)

// maxResultBytes bounds the Grype result read by the Manager.
const maxResultBytes = 64 << 20

var errInvalidResult = errors.New("invalid Grype result")

// grypeDocument holds the fields of a Grype JSON result the Manager uses
// (grype/presenter/models in Grype v0.121); the others are ignored.
type grypeDocument struct {
	Matches *[]struct {
		Vulnerability struct {
			ID          string   `json:"id"`
			DataSource  string   `json:"dataSource"`
			Severity    string   `json:"severity"`
			URLs        []string `json:"urls"`
			Description string   `json:"description"`
			Fix         struct {
				Versions []string `json:"versions"`
				State    string   `json:"state"`
			} `json:"fix"`
		} `json:"vulnerability"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Type    string `json:"type"`
		} `json:"artifact"`
	} `json:"matches"`
	Descriptor struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		DB      struct {
			Status struct {
				Built string `json:"built"`
			} `json:"status"`
		} `json:"db"`
	} `json:"descriptor"`
}

// readResult reads result.json once the unit finished: written after
// notBefore (a result left by an earlier run is stale), at most
// maxResultBytes, decoded into the expected types.
func readResult(path string, notBefore time.Time) (domain.ScanResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureStaleResult, Err: err}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureInvalidResult, Err: err}
	}
	if info.ModTime().Before(notBefore) {
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureStaleResult}
	}
	if !info.Mode().IsRegular() || info.Size() > maxResultBytes {
		return domain.ScanResult{}, &domain.ScanError{Failure: domain.FailureInvalidResult, Err: fmt.Errorf("%w: %d bytes", errInvalidResult, info.Size())}
	}
	return decodeResult(io.LimitReader(file, maxResultBytes))
}

func decodeResult(reader io.Reader) (domain.ScanResult, error) {
	invalid := func(err error) error {
		return &domain.ScanError{Failure: domain.FailureInvalidResult, Err: err}
	}
	var document grypeDocument
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&document); err != nil {
		return domain.ScanResult{}, invalid(fmt.Errorf("%w: %w", errInvalidResult, err))
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return domain.ScanResult{}, invalid(fmt.Errorf("%w: trailing data", errInvalidResult))
	}
	if document.Matches == nil || document.Descriptor.Name != "grype" {
		return domain.ScanResult{}, invalid(fmt.Errorf("%w: not a Grype JSON document", errInvalidResult))
	}
	if len(*document.Matches) > domain.MaxScanMatches {
		return domain.ScanResult{}, invalid(fmt.Errorf("%w: %d matches", errInvalidResult, len(*document.Matches)))
	}
	result := domain.ScanResult{GrypeVersion: document.Descriptor.Version, Matches: make([]domain.ScanMatch, 0, len(*document.Matches))}
	if len(result.GrypeVersion) > 64 {
		result.GrypeVersion = result.GrypeVersion[:64]
	}
	if built, err := time.Parse(time.RFC3339, document.Descriptor.DB.Status.Built); err == nil {
		built = built.UTC()
		result.DBBuiltAt = &built
	}
	for _, match := range *document.Matches {
		v := match.Vulnerability
		result.Matches = append(result.Matches, domain.ScanMatch{
			VulnerabilityID: v.ID, Severity: domain.NormalizeVulnSeverity(v.Severity),
			Package: match.Artifact.Name, Version: match.Artifact.Version, PackageType: match.Artifact.Type,
			FixState: domain.NormalizeFixState(v.Fix.State), FixedVersions: v.Fix.Versions,
			Description: v.Description, URLs: v.URLs, DataSource: v.DataSource,
		})
	}
	return result, nil
}
