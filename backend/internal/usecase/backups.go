package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	auditTargetBackupConfig = "backup_config"
	auditTargetBackupRun    = "backup_run"
	// backupTick is how often the schedule is checked.
	backupTick = 30 * time.Second
	// backupOnTime: a scheduled backup started later than this after its
	// time is reported as a catch-up.
	backupOnTime = 5 * time.Minute
	// backupStoreTimeout bounds the database writes around a run, including
	// the last one while the Manager stops.
	backupStoreTimeout = 10 * time.Second
	backupPurgeEvery   = time.Hour
)

// BackupConfigView is the configuration shown to the interface: never a
// secret, only whether each one is set.
type BackupConfigView struct {
	Repository     *domain.BackupRepository
	Initialized    bool
	InitializedAt  *time.Time
	HasPassword    bool
	HasUsername    bool
	HasSecret      bool
	Selection      domain.BackupSelection
	Schedule       domain.BackupSchedule
	UpdatedAt      *time.Time
	UpdatedBy      string
	SecretsEnabled bool
}

// BackupConfigUpdate replaces the configuration. Username and Secret are
// the repository access keys: nil keeps them, "" removes them. Changing
// the repository asks for a new initialisation.
type BackupConfigUpdate struct {
	Repository *domain.BackupRepository
	Username   *string
	Secret     *string
	Selection  domain.BackupSelection
	Schedule   domain.BackupSchedule
}

// BackupUsecase configures and runs the backups (SPEC_ADMINISTRATION.md
// § 7). Run owns the goroutine that executes every operation, one at a
// time, and starts the scheduled backups itself (decision D-12).
type BackupUsecase struct {
	store    ports.BackupStore
	executor ports.BackupExecutor
	// box is nil without the secrets key: backups are then unavailable.
	box       ports.SecretBox
	logger    *zap.Logger
	now       func() time.Time
	location  *time.Location
	tick      time.Duration
	startedAt time.Time

	mu      sync.Mutex
	running bool
	// requests holds one run already recorded and claimed.
	requests chan pendingBackup
}

type pendingBackup struct {
	run domain.BackupRun
}

func NewBackupUsecase(store ports.BackupStore, executor ports.BackupExecutor, box ports.SecretBox, logger *zap.Logger) *BackupUsecase {
	now := time.Now()
	return &BackupUsecase{
		store: store, executor: executor, box: box, logger: logger,
		now: time.Now, location: time.Local, tick: backupTick, startedAt: now,
		requests: make(chan pendingBackup, 1),
	}
}

// Run closes the runs left running by a stop, then checks the schedule
// every 30 s and executes the requested runs until ctx is cancelled.
func (u *BackupUsecase) Run(ctx context.Context) {
	u.startedAt = u.now()
	if err := u.store.InterruptRunningBackupRuns(ctx, u.now().UTC()); err != nil {
		u.logger.Warn("could not close the interrupted backup runs", zap.Error(err))
	}
	ticker := time.NewTicker(u.tick)
	defer ticker.Stop()
	lastPurge := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case pending := <-u.requests:
			u.execute(ctx, pending.run)
		case <-ticker.C:
			if u.now().Sub(lastPurge) >= backupPurgeEvery {
				lastPurge = u.now()
				u.purge(ctx)
			}
			u.checkSchedule(ctx)
		}
	}
}

func (u *BackupUsecase) claim() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.running {
		return false
	}
	u.running = true
	return true
}

func (u *BackupUsecase) release() {
	u.mu.Lock()
	u.running = false
	u.mu.Unlock()
}

// Config returns the configuration without its secrets.
func (u *BackupUsecase) Config(ctx context.Context) (BackupConfigView, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return BackupConfigView{}, err
	}
	view := BackupConfigView{
		Repository: config.Repository, Initialized: config.InitializedAt != nil, InitializedAt: config.InitializedAt,
		HasPassword: config.SealedCredentials != "", Selection: config.Selection, Schedule: config.Schedule,
		UpdatedAt: config.UpdatedAt, UpdatedBy: config.UpdatedBy, SecretsEnabled: u.box != nil,
	}
	if config.SealedCredentials != "" && u.box != nil {
		if credentials, err := u.openCredentials(config.SealedCredentials); err == nil {
			view.HasPassword, view.HasUsername, view.HasSecret =
				credentials.Password != "", credentials.Username != "", credentials.Secret != ""
		}
	}
	return view, nil
}

// UpdateConfig validates and stores the configuration, audited. Callers
// must hold domain.PermManageBackups.
func (u *BackupUsecase) UpdateConfig(ctx context.Context, actor domain.User, update BackupConfigUpdate, requestID string) (BackupConfigView, error) {
	if update.Repository != nil {
		if err := domain.ValidateBackupRepository(*update.Repository); err != nil {
			return BackupConfigView{}, err
		}
	}
	if err := domain.ValidateBackupSelection(update.Selection); err != nil {
		return BackupConfigView{}, err
	}
	if err := domain.ValidateBackupSchedule(update.Schedule); err != nil {
		return BackupConfigView{}, err
	}
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return BackupConfigView{}, err
	}
	changed := !sameRepository(config.Repository, update.Repository)
	if changed && u.isRunning() {
		return BackupConfigView{}, domain.ErrBackupRepositoryInUse
	}
	if update.Username != nil || update.Secret != nil || (changed && config.SealedCredentials != "") {
		if u.box == nil {
			return BackupConfigView{}, domain.ErrBackupSecretsMissing
		}
		var credentials domain.BackupCredentials
		if config.SealedCredentials != "" {
			if credentials, err = u.openCredentials(config.SealedCredentials); err != nil {
				return BackupConfigView{}, err
			}
		}
		if update.Username != nil {
			credentials.Username = *update.Username
		}
		if update.Secret != nil {
			credentials.Secret = *update.Secret
		}
		if changed {
			// A new repository gets a new password at its initialisation.
			credentials.Password = ""
		}
		if config.SealedCredentials, err = u.sealCredentials(credentials); err != nil {
			return BackupConfigView{}, err
		}
	}
	if changed {
		config.InitializedAt = nil
	}
	now := u.now().UTC()
	config.Repository, config.Selection, config.Schedule = update.Repository, update.Selection, update.Schedule
	config.UpdatedAt, config.UpdatedBy = &now, actor.Username
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupConfig, TargetID: "local", Action: "update",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveBackupConfig(ctx, config, audit); err != nil {
		return BackupConfigView{}, err
	}
	return u.Config(ctx)
}

func sameRepository(a, b *domain.BackupRepository) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (u *BackupUsecase) isRunning() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.running
}

// sealCredentials seals the credentials; empty credentials are stored as
// "".
func (u *BackupUsecase) sealCredentials(credentials domain.BackupCredentials) (string, error) {
	if credentials == (domain.BackupCredentials{}) {
		return "", nil
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("encode backup credentials: %w", err)
	}
	sealed, err := u.box.Seal(plaintext, domain.BackupSecretsAssociatedData)
	if err != nil {
		return "", fmt.Errorf("seal backup credentials: %w", err)
	}
	return sealed, nil
}

func (u *BackupUsecase) openCredentials(sealed string) (domain.BackupCredentials, error) {
	if u.box == nil {
		return domain.BackupCredentials{}, domain.ErrBackupSecretsMissing
	}
	plaintext, err := u.box.Open(sealed, domain.BackupSecretsAssociatedData)
	if err != nil {
		return domain.BackupCredentials{}, fmt.Errorf("%w: %v", domain.ErrBackupSecretsMissing, err)
	}
	var credentials domain.BackupCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return domain.BackupCredentials{}, fmt.Errorf("decode backup credentials: %w", err)
	}
	return credentials, nil
}

// InitRepository generates the repository password, seals it and starts
// restic init. The password is returned once, to be shown to the
// administrator and never again. Callers must hold
// domain.PermManageBackups.
func (u *BackupUsecase) InitRepository(ctx context.Context, actor domain.User, requestID string) (string, domain.BackupRun, error) {
	if u.box == nil {
		return "", domain.BackupRun{}, domain.ErrBackupSecretsMissing
	}
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return "", domain.BackupRun{}, err
	}
	if config.Repository == nil {
		return "", domain.BackupRun{}, domain.ErrBackupNotConfigured
	}
	if config.InitializedAt != nil {
		return "", domain.BackupRun{}, domain.ErrBackupAlreadyInitialized
	}
	var credentials domain.BackupCredentials
	if config.SealedCredentials != "" {
		if credentials, err = u.openCredentials(config.SealedCredentials); err != nil {
			return "", domain.BackupRun{}, err
		}
	}
	random := make([]byte, domain.BackupPasswordBytes)
	if _, err := rand.Read(random); err != nil {
		return "", domain.BackupRun{}, fmt.Errorf("generate the repository password: %w", err)
	}
	credentials.Password = base64.RawURLEncoding.EncodeToString(random)
	if err := domain.ValidateBackupCredentials(config.Repository.Type, credentials); err != nil {
		return "", domain.BackupRun{}, err
	}
	if !u.claim() {
		return "", domain.BackupRun{}, domain.ErrBackupAlreadyRunning
	}
	if config.SealedCredentials, err = u.sealCredentials(credentials); err != nil {
		u.release()
		return "", domain.BackupRun{}, err
	}
	now := u.now().UTC()
	config.UpdatedAt, config.UpdatedBy = &now, actor.Username
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupConfig, TargetID: "local", Action: "init_repository",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveBackupConfig(ctx, config, audit); err != nil {
		u.release()
		return "", domain.BackupRun{}, err
	}
	run, err := u.enqueue(ctx, domain.BackupOpInit, domain.BackupManual, actor.Username, nil)
	if err != nil {
		return "", domain.BackupRun{}, err
	}
	return credentials.Password, run, nil
}

// RequestBackup records a manual backup as running and hands it to Run.
// Callers must hold domain.PermRunBackups.
func (u *BackupUsecase) RequestBackup(ctx context.Context, actor domain.User, requestID string) (domain.BackupRun, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupRun{}, err
	}
	if err := config.Ready(); err != nil {
		return domain.BackupRun{}, err
	}
	if u.box == nil {
		return domain.BackupRun{}, domain.ErrBackupSecretsMissing
	}
	if !u.claim() {
		return domain.BackupRun{}, domain.ErrBackupAlreadyRunning
	}
	id := uuid.New()
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupRun, TargetID: id.String(), Action: "start",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	return u.enqueueWithID(ctx, id, domain.BackupOpBackup, domain.BackupManual, actor.Username, &audit)
}

// enqueue records a claimed run and hands it to Run; it releases the claim
// on failure.
func (u *BackupUsecase) enqueue(ctx context.Context, op domain.BackupOperation, origin domain.BackupOrigin, actor string, audit *domain.AuditEntry) (domain.BackupRun, error) {
	return u.enqueueWithID(ctx, uuid.New(), op, origin, actor, audit)
}

func (u *BackupUsecase) enqueueWithID(ctx context.Context, id uuid.UUID, op domain.BackupOperation, origin domain.BackupOrigin, actor string, audit *domain.AuditEntry) (domain.BackupRun, error) {
	run := domain.BackupRun{ID: id, Operation: op, Origin: origin, RequestedBy: actor, State: domain.BackupRunning, StartedAt: u.now().UTC()}
	if err := u.store.StartBackupRun(ctx, run, audit); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	// The channel holds one run and only a claimed run is sent: it never
	// blocks.
	u.requests <- pendingBackup{run: run}
	return run, nil
}

// checkSchedule starts the scheduled backup once due. A backup missed while
// the Manager was stopped runs once, after the start-up delay.
func (u *BackupUsecase) checkSchedule(ctx context.Context) {
	due, origin, ok := u.nextScheduled(ctx)
	now := u.now()
	if !ok || now.Before(due) {
		return
	}
	if now.Sub(due) > backupOnTime {
		origin = domain.BackupCatchUp
		if now.Sub(u.startedAt) < domain.BackupCatchUpDelay {
			return
		}
	}
	if !u.claim() {
		return
	}
	storeCtx, cancel := context.WithTimeout(ctx, backupStoreTimeout)
	run := domain.BackupRun{ID: uuid.New(), Operation: domain.BackupOpBackup, Origin: origin, State: domain.BackupRunning, StartedAt: now.UTC()}
	err := u.store.StartBackupRun(storeCtx, run, nil)
	cancel()
	if err != nil {
		u.release()
		u.logger.Warn("could not record a scheduled backup", zap.Error(err))
		return
	}
	u.execute(ctx, run)
}

// nextScheduled returns the scheduled time of the next backup: the first
// one after the last scheduled run, or after the configuration was saved.
func (u *BackupUsecase) nextScheduled(ctx context.Context) (time.Time, domain.BackupOrigin, bool) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil || config.Ready() != nil || !config.Schedule.Enabled {
		return time.Time{}, "", false
	}
	latest, err := u.store.LatestBackupRuns(ctx)
	if err != nil {
		u.logger.Warn("could not read the backup runs", zap.Error(err))
		return time.Time{}, "", false
	}
	reference := u.now()
	switch {
	case latest.LastScheduled != nil:
		reference = latest.LastScheduled.StartedAt
	case config.UpdatedAt != nil:
		reference = *config.UpdatedAt
	}
	next, ok := config.Schedule.Next(reference.In(u.location))
	return next, domain.BackupScheduled, ok
}

// execute runs a run already recorded as running and claimed, and records
// its outcome.
func (u *BackupUsecase) execute(ctx context.Context, run domain.BackupRun) {
	defer u.release()
	result, err := u.runOperation(ctx, run)
	finished := u.now().UTC()
	run.FinishedAt = &finished
	var snapshots []domain.BackupSnapshot
	var initializedAt *time.Time
	if err != nil {
		run.State = domain.BackupFailed
		var runErr *domain.BackupRunError
		switch {
		case ctx.Err() != nil:
			run.Failure = domain.BackupFailureInterrupted
		case errors.As(err, &runErr):
			run.Failure, run.Message = runErr.Failure, runErr.Message
		case errors.Is(err, domain.ErrBackupSecretsMissing):
			run.Failure = domain.BackupFailureSecrets
		default:
			run.Failure = domain.BackupFailureJob
		}
		u.logger.Warn("Backup operation failed", zap.String("operation", string(run.Operation)),
			zap.String("failure", string(run.Failure)), zap.Error(err))
	} else {
		run.State, run.Warning, run.Message = domain.BackupSucceeded, result.Warning, result.Message
		run.SnapshotID, run.Stats, snapshots = result.SnapshotID, result.Stats, result.Snapshots
		if run.Operation == domain.BackupOpInit {
			initializedAt = &finished
		}
		u.logger.Info("Backup operation done", zap.String("operation", string(run.Operation)), zap.String("snapshot", run.SnapshotID))
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupStoreTimeout)
	defer cancel()
	if err := u.store.FinishBackupRun(storeCtx, run, snapshots, initializedAt); err != nil {
		u.logger.Error("could not record the outcome of a backup run", zap.Error(err))
	}
}

func (u *BackupUsecase) runOperation(ctx context.Context, run domain.BackupRun) (domain.BackupResult, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupResult{}, err
	}
	if config.Repository == nil || config.SealedCredentials == "" {
		return domain.BackupResult{}, domain.ErrBackupNotConfigured
	}
	credentials, err := u.openCredentials(config.SealedCredentials)
	if err != nil {
		return domain.BackupResult{}, err
	}
	job := ports.BackupJob{Operation: run.Operation, Repository: *config.Repository, Selection: config.Selection}
	return u.executor.Execute(ctx, job, credentials)
}

func (u *BackupUsecase) purge(ctx context.Context) {
	if err := u.store.PurgeBackupRuns(ctx, u.now().Add(-domain.BackupRunRetention)); err != nil {
		u.logger.Warn("could not purge the backup runs", zap.Error(err))
	}
}

// Summary gives the state of the module, with the next scheduled backup.
func (u *BackupUsecase) Summary(ctx context.Context) (domain.BackupSummary, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupSummary{}, err
	}
	latest, err := u.store.LatestBackupRuns(ctx)
	if err != nil {
		return domain.BackupSummary{}, err
	}
	summary := domain.BackupSummary{
		Configured: config.Repository != nil, Initialized: config.InitializedAt != nil,
		Running: latest.Running, LastRun: latest.LastBackup, LastSuccess: latest.LastSuccess,
	}
	if next, _, ok := u.nextScheduled(ctx); ok {
		if next.Before(u.now()) {
			next = u.now()
		}
		utc := next.UTC()
		summary.NextRunAt = &utc
	}
	return summary, nil
}

func (u *BackupUsecase) Runs(ctx context.Context, filter domain.BackupRunFilter) ([]domain.BackupRun, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	return u.store.BackupRuns(ctx, filter)
}

func (u *BackupUsecase) BackupRun(ctx context.Context, id uuid.UUID) (domain.BackupRun, error) {
	return u.store.BackupRun(ctx, id)
}

func (u *BackupUsecase) Snapshots(ctx context.Context) ([]domain.BackupSnapshot, error) {
	return u.store.BackupSnapshots(ctx)
}
