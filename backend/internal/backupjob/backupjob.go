// Package backupjob defines the files the Manager and homelab-backup-runner
// exchange (SPEC_ADMINISTRATION.md § 7.1): a job and its secrets, written by
// the Manager, and a result, written by the runner. Both sides decode them
// strictly and validate them with the domain rules.
package backupjob

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"homelab-manager/backend/internal/domain"
)

// Bounds of the exchanged files.
const (
	MaxJobBytes     = 1 << 20
	MaxSecretsBytes = 16 << 10
	MaxResultBytes  = 32 << 20
)

// Job is what an operation must do. It never holds a secret.
type Job struct {
	Version          int                         `json:"version"`
	Operation        domain.BackupOperation      `json:"operation"`
	RepositoryType   domain.BackupRepositoryType `json:"repositoryType"`
	Repository       string                      `json:"repository"`
	Sources          []string                    `json:"sources"`
	Excludes         []string                    `json:"excludes"`
	ExcludeCaches    bool                        `json:"excludeCaches"`
	ExcludeIfPresent string                      `json:"excludeIfPresent"`
}

// Validate applies the domain rules again: the runner trusts nothing the
// Manager wrote.
func (j Job) Validate() error {
	if j.Version != domain.BackupJobVersion {
		return fmt.Errorf("job version %d is not supported", j.Version)
	}
	if !j.Operation.Valid() {
		return fmt.Errorf("operation %q is not supported", j.Operation)
	}
	if err := domain.ValidateBackupRepository(j.BackupRepository()); err != nil {
		return err
	}
	selection := domain.BackupSelection{
		Sources: j.Sources, Excludes: j.Excludes, ExcludeCaches: j.ExcludeCaches, ExcludeIfPresent: j.ExcludeIfPresent,
	}
	if err := domain.ValidateBackupSelection(selection); err != nil {
		return err
	}
	if j.Operation == domain.BackupOpBackup && len(j.Sources) == 0 {
		return errors.New("a backup needs at least one source")
	}
	return nil
}

func (j Job) BackupRepository() domain.BackupRepository {
	return domain.BackupRepository{Type: j.RepositoryType, Location: j.Repository}
}

// Summary holds the figures of restic's backup summary.
type Summary struct {
	FilesNew            int64   `json:"filesNew"`
	FilesChanged        int64   `json:"filesChanged"`
	FilesUnmodified     int64   `json:"filesUnmodified"`
	DirsNew             int64   `json:"dirsNew"`
	DirsChanged         int64   `json:"dirsChanged"`
	DirsUnmodified      int64   `json:"dirsUnmodified"`
	DataAdded           int64   `json:"dataAdded"`
	TotalFilesProcessed int64   `json:"totalFilesProcessed"`
	TotalBytesProcessed int64   `json:"totalBytesProcessed"`
	DurationSeconds     float64 `json:"durationSeconds"`
}

// Snapshot is one snapshot of the repository.
type Snapshot struct {
	ID         string    `json:"id"`
	ShortID    string    `json:"shortId"`
	Time       time.Time `json:"time"`
	Hostname   string    `json:"hostname"`
	Paths      []string  `json:"paths"`
	Tags       []string  `json:"tags"`
	TotalFiles int64     `json:"totalFiles"`
	TotalBytes int64     `json:"totalBytes"`
	DataAdded  int64     `json:"dataAdded"`
}

// Result is what the runner reports, success or not.
type Result struct {
	Version    int                    `json:"version"`
	Operation  domain.BackupOperation `json:"operation"`
	StartedAt  time.Time              `json:"startedAt"`
	FinishedAt time.Time              `json:"finishedAt"`
	Success    bool                   `json:"success"`
	// Warning: the snapshot exists but restic could not read some files.
	Warning    bool       `json:"warning"`
	Message    string     `json:"message"`
	SnapshotID string     `json:"snapshotId"`
	Summary    *Summary   `json:"summary"`
	Snapshots  []Snapshot `json:"snapshots"`
}

// Paths of the files of an operation in the exchange directories.
func JobPath(jobsDir string, operation domain.BackupOperation) string {
	return filepath.Join(jobsDir, string(operation)+".json")
}

func SecretsPath(jobsDir string, operation domain.BackupOperation) string {
	return filepath.Join(jobsDir, string(operation)+".secrets.json")
}

func ResultPath(runsDir string, operation domain.BackupOperation) string {
	return filepath.Join(runsDir, string(operation)+".json")
}

// ReadStrict decodes one JSON document of at most limit bytes, refusing
// unknown fields and trailing data.
func ReadStrict(path string, limit int64, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	if decoder.More() {
		return fmt.Errorf("%s has trailing data", filepath.Base(path))
	}
	return nil
}

// WriteAtomic writes a JSON document through a temporary file renamed in
// place, so that a reader never sees half a file.
func WriteAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	name := temporary.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("chmod %s: %w", filepath.Base(path), err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return fmt.Errorf("rename %s: %w", filepath.Base(path), err)
	}
	return nil
}
