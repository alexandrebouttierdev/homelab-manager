package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// BackupLatest are the runs the schedule and the summary need.
type BackupLatest struct {
	// Running is the run in progress, if any.
	Running *domain.BackupRun
	// LastBackup is the newest backup run, whatever its outcome.
	LastBackup *domain.BackupRun
	// LastSuccess is the newest successful backup.
	LastSuccess *domain.BackupRun
	// LastScheduled is the newest backup started by the schedule (or its
	// catch-up), whatever its outcome.
	LastScheduled *domain.BackupRun
	// LastCheck is the newest successful check of the repository.
	LastCheck *domain.BackupRun
	// LastCopy is the newest copy to the copy repository, whatever its
	// outcome.
	LastCopy *domain.BackupRun
}

// BackupFinish is what a finished run changes besides itself.
type BackupFinish struct {
	// Snapshots replace the stored ones when not nil.
	Snapshots []domain.BackupSnapshot
	// InitializedAt marks the repository initialized when not nil.
	InitializedAt *time.Time
	// CopyInitializedAt marks the copy repository initialized when not nil.
	CopyInitializedAt *time.Time
}

// BackupStore keeps the configuration, the runs and the snapshots of the
// backups module (SPEC_ADMINISTRATION.md § 7).
type BackupStore interface {
	// BackupConfig returns the stored configuration, or an empty one with
	// the default schedule.
	BackupConfig(ctx context.Context) (domain.BackupConfig, error)
	// SaveBackupConfig stores the configuration and its audit entry in one
	// transaction.
	SaveBackupConfig(ctx context.Context, config domain.BackupConfig, audit domain.AuditEntry) error
	// StartBackupRun records a running run, with its audit entry when a
	// user asked for it.
	StartBackupRun(ctx context.Context, run domain.BackupRun, audit *domain.AuditEntry) error
	// FinishBackupRun records the outcome and what it changes in one
	// transaction.
	FinishBackupRun(ctx context.Context, run domain.BackupRun, finish BackupFinish) error
	// InterruptRunningBackupRuns fails the runs left running by a stop.
	InterruptRunningBackupRuns(ctx context.Context, at time.Time) error
	LatestBackupRuns(ctx context.Context) (BackupLatest, error)
	BackupRuns(ctx context.Context, filter domain.BackupRunFilter) ([]domain.BackupRun, error)
	// BackupRun returns one run or domain.ErrBackupRunNotFound.
	BackupRun(ctx context.Context, id uuid.UUID) (domain.BackupRun, error)
	// BackupSnapshots lists the stored snapshots, newest first.
	BackupSnapshots(ctx context.Context) ([]domain.BackupSnapshot, error)
	// PurgeBackupRuns removes the finished runs started before the time.
	PurgeBackupRuns(ctx context.Context, before time.Time) error
}

// BackupJob is what one operation needs; the secrets travel apart.
type BackupJob struct {
	// RunID names the quarantine directory of a restore.
	RunID      uuid.UUID
	Operation  domain.BackupOperation
	Repository domain.BackupRepository
	// Selection: backup only.
	Selection domain.BackupSelection
	// Retention: forget-prune only.
	Retention domain.BackupRetention
	// CheckSubsetPercent: check only.
	CheckSubsetPercent int
	// Restore: restore only.
	Restore *domain.BackupRestore
	// Diff: diff only.
	Diff *domain.BackupDiffRequest
	// Copy: copy only, the destination; CopyInit creates it first and
	// CopyRetention, when not nil, applies to it after the copy.
	Copy          *domain.BackupRepository
	CopyInit      bool
	CopyRetention *domain.BackupRetention
}

// BackupExecutor runs an operation in homelab-backup@.service and returns
// its result. Its failures are *domain.BackupRunError; the context bounds
// the run.
type BackupExecutor interface {
	// copyCredentials are those of the copy repository, for a copy only.
	Execute(ctx context.Context, job BackupJob, credentials domain.BackupCredentials, copyCredentials *domain.BackupCredentials) (domain.BackupResult, error)
}
