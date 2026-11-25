// Package backupunit runs a backup operation in homelab-backup@.service:
// it writes the job and its one-use secrets file, starts the unit by D-Bus
// (allowed by polkit for these instances only) and reads the result the
// runner wrote (SPEC_ADMINISTRATION.md § 7.1).
package backupunit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

// Defaults of the deployment (deployments/tmpfiles/homelab-backup.conf and
// deployments/systemd/homelab-backup@.service).
const (
	DefaultJobsDir = "/var/lib/homelab-backup/jobs"
	DefaultRunsDir = "/var/lib/homelab-backup/runs"
	// The waits leave the unit its TimeoutStartSec=, plus the time systemd
	// needs to stop it.
	DefaultInitTimeout   = 11 * time.Minute
	DefaultBackupTimeout = 6*time.Hour + 5*time.Minute
	// jobMode keeps the job and the secrets to the Manager (and root).
	jobMode = 0o600
)

// Config locates the exchange directories and bounds the runs.
type Config struct {
	JobsDir       string
	RunsDir       string
	InitTimeout   time.Duration
	BackupTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{JobsDir: DefaultJobsDir, RunsDir: DefaultRunsDir, InitTimeout: DefaultInitTimeout, BackupTimeout: DefaultBackupTimeout}
}

// unitController starts a unit and waits for it (systemd.Controller).
type unitController interface {
	Act(ctx context.Context, name string, verb domain.UnitVerb) (domain.UnitStableState, error)
}

var _ ports.BackupExecutor = (*Executor)(nil)

type Executor struct {
	config Config
	units  unitController
	now    func() time.Time
}

func NewExecutor(config Config, units unitController) *Executor {
	return &Executor{config: config, units: units, now: time.Now}
}

func (e *Executor) Execute(ctx context.Context, job ports.BackupJob, given ports.BackupSecrets) (domain.BackupResult, error) {
	op := job.Operation
	// Some file systems keep modification times to the second.
	started := e.now().UTC().Truncate(time.Second)
	file := backupjob.Job{
		Version: domain.BackupJobVersion, Operation: op,
		RepositoryType: job.Repository.Type, Repository: job.Repository.Location, HostKey: job.Repository.HostKey,
		Sources: orEmpty(job.Selection.Sources), Excludes: orEmpty(job.Selection.Excludes),
		ExcludeCaches: job.Selection.ExcludeCaches, ExcludeIfPresent: job.Selection.ExcludeIfPresent,
		SQLiteDatabases: orEmpty(job.Selection.SQLiteDatabases),
	}
	if op != domain.BackupOpBackup {
		file.Sources, file.Excludes, file.ExcludeCaches, file.ExcludeIfPresent = []string{}, []string{}, false, ""
		file.SQLiteDatabases = []string{}
	}
	switch op {
	case domain.BackupOpForgetPrune:
		file.Retention = &backupjob.Retention{
			KeepDaily: job.Retention.KeepDaily, KeepWeekly: job.Retention.KeepWeekly,
			KeepMonthly: job.Retention.KeepMonthly, KeepYearly: job.Retention.KeepYearly,
		}
	case domain.BackupOpCheck:
		file.CheckSubsetPercent = job.CheckSubsetPercent
	case domain.BackupOpRestore:
		if job.Restore == nil {
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: errors.New("a restore needs its request")}
		}
		file.Restore = &backupjob.Restore{
			SnapshotID: job.Restore.SnapshotID, Include: orEmpty(job.Restore.Include), DryRun: job.Restore.DryRun,
			Target: domain.RestoreTarget(job.RunID),
		}
	case domain.BackupOpDiff:
		if job.Diff == nil {
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: errors.New("a diff needs its snapshots")}
		}
		file.Diff = &backupjob.Diff{From: job.Diff.From, To: job.Diff.To}
	case domain.BackupOpCopy:
		if job.Copy == nil || given.Copy == nil {
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: errors.New("a copy needs its destination")}
		}
		file.Copy = &backupjob.Copy{
			RepositoryType: job.Copy.Type, Repository: job.Copy.Location, HostKey: job.Copy.HostKey, Init: job.CopyInit,
		}
		if r := job.CopyRetention; r != nil {
			file.Copy.Retention = &backupjob.Retention{
				KeepDaily: r.KeepDaily, KeepWeekly: r.KeepWeekly, KeepMonthly: r.KeepMonthly, KeepYearly: r.KeepYearly,
			}
		}
	}
	secrets := backupjob.Secrets{BackupCredentials: given.Credentials}
	if op == domain.BackupOpCopy {
		secrets.Copy = given.Copy
	}
	if file.UsesSFTP() {
		secrets.SSHKey = given.SSHKey
	}
	if err := file.Validate(); err != nil {
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: err}
	}
	if err := file.ValidateSecrets(secrets); err != nil {
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: err}
	}
	secretsPath := backupjob.SecretsPath(e.config.JobsDir, op)
	if err := backupjob.WriteAtomic(backupjob.JobPath(e.config.JobsDir, op), file, jobMode); err != nil {
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: err}
	}
	if err := backupjob.WriteAtomic(secretsPath, secrets, jobMode); err != nil {
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureJob, Err: err}
	}
	// The runner removes the secrets once read; they never outlive a run
	// that did not start.
	defer func() { _ = os.Remove(secretsPath) }()

	timeout := e.config.BackupTimeout
	switch op {
	case domain.BackupOpInit, domain.BackupOpSnapshots, domain.BackupOpUnlock, domain.BackupOpDiff:
		timeout = e.config.InitTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := e.units.Act(runCtx, op.BackupUnit(), domain.UnitStart); err != nil {
		var actionErr *domain.UnitActionError
		if !errors.As(err, &actionErr) {
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureUnavailable, Err: err}
		}
		switch actionErr.Failure {
		case domain.ActionUnitFailed:
			// The runner reports the failure in its result.
		case domain.ActionNotAllowed:
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureNotAllowed, Err: err}
		case domain.ActionNotFound:
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureRunner, Err: err}
		case domain.ActionTimeout:
			if ctx.Err() != nil {
				return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureInterrupted, Err: err}
			}
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureTimeout, Err: err}
		default:
			return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureUnavailable, Err: err}
		}
	}
	return e.readResult(op, started)
}

func (e *Executor) readResult(op domain.BackupOperation, started time.Time) (domain.BackupResult, error) {
	var result backupjob.Result
	if err := backupjob.ReadStrict(backupjob.ResultPath(e.config.RunsDir, op), backupjob.MaxResultBytes, &result); err != nil {
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureResult, Err: err}
	}
	switch {
	case result.Version != domain.BackupJobVersion || result.Operation != op:
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureResult, Err: fmt.Errorf("result of %q, version %d", result.Operation, result.Version)}
	case result.StartedAt.Before(started):
		return domain.BackupResult{}, &domain.BackupRunError{Failure: domain.BackupFailureResult, Err: errors.New("the result is older than the run")}
	case !result.Success:
		return domain.BackupResult{}, &domain.BackupRunError{
			Failure: domain.BackupFailureRestic, Message: domain.TruncateBackupMessage(result.Message),
			Err: errors.New("restic failed"),
		}
	}
	converted := domain.BackupResult{
		SnapshotID: result.SnapshotID, Warning: result.Warning, Message: domain.TruncateBackupMessage(result.Message),
		Details: result.Details,
	}
	if s := result.Summary; s != nil {
		converted.Stats = &domain.BackupStats{
			FilesNew: s.FilesNew, FilesChanged: s.FilesChanged, FilesUnmodified: s.FilesUnmodified,
			DirsNew: s.DirsNew, DirsChanged: s.DirsChanged, DirsUnmodified: s.DirsUnmodified,
			DataAdded: s.DataAdded, TotalFilesProcessed: s.TotalFilesProcessed,
			TotalBytesProcessed: s.TotalBytesProcessed, DurationSeconds: s.DurationSeconds,
		}
	}
	if result.Snapshots != nil {
		converted.Snapshots = make([]domain.BackupSnapshot, 0, min(len(result.Snapshots), domain.MaxBackupSnapshots))
		for _, s := range result.Snapshots[:min(len(result.Snapshots), domain.MaxBackupSnapshots)] {
			converted.Snapshots = append(converted.Snapshots, domain.BackupSnapshot{
				ID: s.ID, ShortID: s.ShortID, Time: s.Time, Hostname: s.Hostname, Paths: s.Paths, Tags: s.Tags,
				TotalFiles: s.TotalFiles, TotalBytes: s.TotalBytes, DataAdded: s.DataAdded,
			})
		}
	}
	return converted, nil
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
