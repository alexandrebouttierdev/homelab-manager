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
	Retention      domain.BackupRetention
	Check          domain.BackupCheck
	UpdatedAt      *time.Time
	UpdatedBy      string
	SecretsEnabled bool
	// Copy is the copy repository, nil when none is set.
	Copy *BackupCopyView
}

// BackupCopyView is the copy repository without its secrets.
type BackupCopyView struct {
	Repository    domain.BackupRepository
	Initialized   bool
	InitializedAt *time.Time
	HasPassword   bool
	HasUsername   bool
	HasSecret     bool
	AfterBackup   bool
}

// BackupCopyUpdate sets the copy repository. Username and Secret: nil
// keeps them, "" removes them. Changing the repository asks for a new
// initialisation or connection.
type BackupCopyUpdate struct {
	Repository  domain.BackupRepository
	Username    *string
	Secret      *string
	AfterBackup bool
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
	// Retention and Check: nil keeps the stored ones.
	Retention *domain.BackupRetention
	Check     *domain.BackupCheck
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
	// accepting is set once Run closed the runs left by a stop, and
	// cleared when it ends: no run starts outside of it.
	accepting bool
	// requests holds one run already recorded and claimed.
	requests chan pendingBackup
}

type pendingBackup struct {
	run   domain.BackupRun
	input operationInput
}

// operationInput is what an operation needs besides the configuration.
type operationInput struct {
	restore *domain.BackupRestore
	diff    *domain.BackupDiffRequest
	// copyInit creates the copy repository before the copy.
	copyInit bool
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
	u.setAccepting(true)
	defer u.setAccepting(false)
	ticker := time.NewTicker(u.tick)
	defer ticker.Stop()
	lastPurge := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case pending := <-u.requests:
			u.execute(ctx, pending.run, pending.input)
		case <-ticker.C:
			if u.now().Sub(lastPurge) >= backupPurgeEvery {
				lastPurge = u.now()
				u.purge(ctx)
			}
			u.checkSchedule(ctx)
		}
	}
}

func (u *BackupUsecase) setAccepting(accepting bool) {
	u.mu.Lock()
	u.accepting = accepting
	u.mu.Unlock()
}

// claim reserves the single operation slot.
func (u *BackupUsecase) claim() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	switch {
	case !u.accepting:
		return domain.ErrBackupsNotReady
	case u.running:
		return domain.ErrBackupAlreadyRunning
	}
	u.running = true
	return nil
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
		Retention: config.Retention, Check: config.Check,
		UpdatedAt: config.UpdatedAt, UpdatedBy: config.UpdatedBy, SecretsEnabled: u.box != nil,
	}
	if config.SealedCredentials != "" && u.box != nil {
		if credentials, err := u.openCredentials(config.SealedCredentials); err == nil {
			view.HasPassword, view.HasUsername, view.HasSecret =
				credentials.Password != "", credentials.Username != "", credentials.Secret != ""
		}
	}
	if copy := config.Copy; copy != nil {
		view.Copy = &BackupCopyView{
			Repository: copy.Repository, Initialized: copy.InitializedAt != nil, InitializedAt: copy.InitializedAt,
			HasPassword: copy.SealedCredentials != "", AfterBackup: copy.AfterBackup,
		}
		if copy.SealedCredentials != "" && u.box != nil {
			if credentials, err := u.openCopyCredentials(copy.SealedCredentials); err == nil {
				view.Copy.HasPassword, view.Copy.HasUsername, view.Copy.HasSecret =
					credentials.Password != "", credentials.Username != "", credentials.Secret != ""
			}
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
	if update.Retention != nil {
		if err := domain.ValidateBackupRetention(*update.Retention); err != nil {
			return BackupConfigView{}, err
		}
	}
	if update.Check != nil {
		if err := domain.ValidateBackupCheck(*update.Check); err != nil {
			return BackupConfigView{}, err
		}
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
	config.Repository = update.Repository
	if err := u.checkCopy(config); err != nil {
		return BackupConfigView{}, err
	}
	now := u.now().UTC()
	config.Selection, config.Schedule = update.Selection, update.Schedule
	if update.Retention != nil {
		config.Retention = *update.Retention
	}
	if update.Check != nil {
		config.Check = *update.Check
	}
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

// sealCredentials seals the credentials of the repository; empty
// credentials are stored as "".
func (u *BackupUsecase) sealCredentials(credentials domain.BackupCredentials) (string, error) {
	return u.sealWith(credentials, domain.BackupSecretsAssociatedData)
}

func (u *BackupUsecase) openCredentials(sealed string) (domain.BackupCredentials, error) {
	return u.openWith(sealed, domain.BackupSecretsAssociatedData)
}

// sealCopyCredentials and openCopyCredentials are bound to the copy
// repository: its secrets cannot pass for those of the main one.
func (u *BackupUsecase) sealCopyCredentials(credentials domain.BackupCredentials) (string, error) {
	return u.sealWith(credentials, domain.BackupCopySecretsAssociatedData)
}

func (u *BackupUsecase) openCopyCredentials(sealed string) (domain.BackupCredentials, error) {
	return u.openWith(sealed, domain.BackupCopySecretsAssociatedData)
}

func (u *BackupUsecase) sealWith(credentials domain.BackupCredentials, associatedData string) (string, error) {
	if credentials == (domain.BackupCredentials{}) {
		return "", nil
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("encode backup credentials: %w", err)
	}
	sealed, err := u.box.Seal(plaintext, associatedData)
	if err != nil {
		return "", fmt.Errorf("seal backup credentials: %w", err)
	}
	return sealed, nil
}

func (u *BackupUsecase) openWith(sealed, associatedData string) (domain.BackupCredentials, error) {
	if u.box == nil {
		return domain.BackupCredentials{}, domain.ErrBackupSecretsMissing
	}
	plaintext, err := u.box.Open(sealed, associatedData)
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
	if err := u.claim(); err != nil {
		return "", domain.BackupRun{}, err
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
	run, err := u.enqueue(ctx, domain.BackupOpInit, domain.BackupManual, actor.Username, nil, operationInput{})
	if err != nil {
		return "", domain.BackupRun{}, err
	}
	return credentials.Password, run, nil
}

// RequestBackup records a manual backup as running and hands it to Run.
// Callers must hold domain.PermRunBackups.
func (u *BackupUsecase) RequestBackup(ctx context.Context, actor domain.User, requestID string) (domain.BackupRun, error) {
	if err := u.claim(); err != nil {
		return domain.BackupRun{}, err
	}
	config, err := u.store.BackupConfig(ctx)
	if err == nil {
		err = config.Ready()
	}
	if err == nil && u.box == nil {
		err = domain.ErrBackupSecretsMissing
	}
	if err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	id := uuid.New()
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupRun, TargetID: id.String(), Action: "start",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	return u.enqueueWithID(ctx, id, domain.BackupOpBackup, domain.BackupManual, actor.Username, &audit, operationInput{})
}

// RequestOperation starts an operation of an administrator: forget-prune
// (with the configured policy), check, unlock or restore (into the
// quarantine directory of the run). Callers must hold
// domain.PermManageBackups.
func (u *BackupUsecase) RequestOperation(ctx context.Context, actor domain.User, op domain.BackupOperation, request BackupOperationRequest, requestID string) (domain.BackupRun, error) {
	input := operationInput{}
	switch op {
	case domain.BackupOpForgetPrune, domain.BackupOpCheck, domain.BackupOpUnlock, domain.BackupOpCopy:
	case domain.BackupOpRestore:
		if request.Restore == nil {
			return domain.BackupRun{}, invalidOperation("a restore needs a snapshot")
		}
		if err := domain.ValidateBackupRestore(*request.Restore); err != nil {
			return domain.BackupRun{}, err
		}
		input.restore = request.Restore
	case domain.BackupOpDiff:
		if request.Diff == nil {
			return domain.BackupRun{}, invalidOperation("a diff needs two snapshots")
		}
		if err := domain.ValidateBackupDiff(*request.Diff); err != nil {
			return domain.BackupRun{}, err
		}
		input.diff = request.Diff
	default:
		return domain.BackupRun{}, invalidOperation("operation must be forget-prune, check, unlock, restore, diff or copy")
	}
	if err := u.claim(); err != nil {
		return domain.BackupRun{}, err
	}
	config, err := u.store.BackupConfig(ctx)
	if err == nil {
		err = config.Operations()
	}
	if err == nil && u.box == nil {
		err = domain.ErrBackupSecretsMissing
	}
	if err == nil && op == domain.BackupOpForgetPrune {
		policy := config.Retention
		policy.Enabled = true
		err = domain.ValidateBackupRetention(policy)
	}
	if err == nil && op == domain.BackupOpCopy {
		err = config.CopyReady()
	}
	if err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	id := uuid.New()
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupRun, TargetID: id.String(), Action: string(op),
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	return u.enqueueWithID(ctx, id, op, domain.BackupManual, actor.Username, &audit, input)
}

// BackupOperationRequest holds what a restore or a diff needs.
type BackupOperationRequest struct {
	Restore *domain.BackupRestore
	Diff    *domain.BackupDiffRequest
}

func invalidOperation(message string) error {
	return &domain.BackupValidationError{Field: "operation", Message: message}
}

// ConnectRepository attaches an existing repository with its password: the
// password is sealed and restic lists the snapshots; the repository is
// initialized once the listing succeeds. Callers must hold
// domain.PermManageBackups.
func (u *BackupUsecase) ConnectRepository(ctx context.Context, actor domain.User, password, requestID string) (domain.BackupRun, error) {
	if u.box == nil {
		return domain.BackupRun{}, domain.ErrBackupSecretsMissing
	}
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupRun{}, err
	}
	if config.Repository == nil {
		return domain.BackupRun{}, domain.ErrBackupNotConfigured
	}
	if config.InitializedAt != nil {
		return domain.BackupRun{}, domain.ErrBackupAlreadyInitialized
	}
	var credentials domain.BackupCredentials
	if config.SealedCredentials != "" {
		if credentials, err = u.openCredentials(config.SealedCredentials); err != nil {
			return domain.BackupRun{}, err
		}
	}
	credentials.Password = password
	if err := domain.ValidateBackupCredentials(config.Repository.Type, credentials); err != nil {
		return domain.BackupRun{}, err
	}
	if err := u.claim(); err != nil {
		return domain.BackupRun{}, err
	}
	if config.SealedCredentials, err = u.sealCredentials(credentials); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	now := u.now().UTC()
	config.UpdatedAt, config.UpdatedBy = &now, actor.Username
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupConfig, TargetID: "local", Action: "connect_repository",
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveBackupConfig(ctx, config, audit); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	return u.enqueue(ctx, domain.BackupOpSnapshots, domain.BackupManual, actor.Username, nil, operationInput{})
}

// enqueue records a claimed run and hands it to Run; it releases the claim
// on failure.
func (u *BackupUsecase) enqueue(ctx context.Context, op domain.BackupOperation, origin domain.BackupOrigin, actor string, audit *domain.AuditEntry, input operationInput) (domain.BackupRun, error) {
	return u.enqueueWithID(ctx, uuid.New(), op, origin, actor, audit, input)
}

func (u *BackupUsecase) enqueueWithID(ctx context.Context, id uuid.UUID, op domain.BackupOperation, origin domain.BackupOrigin, actor string, audit *domain.AuditEntry, input operationInput) (domain.BackupRun, error) {
	run := domain.BackupRun{ID: id, Operation: op, Origin: origin, RequestedBy: actor, State: domain.BackupRunning, StartedAt: u.now().UTC()}
	// Known before the run, kept if it fails.
	run.Details = input.initialDetails(id)
	if err := u.store.StartBackupRun(ctx, run, audit); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	// The channel holds one run and only a claimed run is sent: it never
	// blocks.
	u.requests <- pendingBackup{run: run, input: input}
	return run, nil
}

// initialDetails are the details known before the run: the snapshot and
// the quarantine directory of a restore, the snapshots of a diff.
func (i operationInput) initialDetails(runID uuid.UUID) *domain.BackupRunDetails {
	switch {
	case i.restore != nil:
		return &domain.BackupRunDetails{SnapshotID: i.restore.SnapshotID, Target: domain.RestoreTarget(runID), DryRun: i.restore.DryRun}
	case i.diff != nil:
		return &domain.BackupRunDetails{Diff: &domain.BackupDiff{From: i.diff.From, To: i.diff.To, Changes: []domain.BackupDiffChange{}}}
	}
	return nil
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
	if u.claim() != nil {
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
	if done := u.execute(ctx, run, operationInput{}); done.State == domain.BackupSucceeded {
		u.maintain(ctx, origin)
	}
}

// maintain follows a successful scheduled backup: the copy when enabled,
// the retention policy when enabled, then the check of the repository when
// enabled and due.
func (u *BackupUsecase) maintain(ctx context.Context, origin domain.BackupOrigin) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return
	}
	var operations []domain.BackupOperation
	if config.Copy != nil && config.Copy.AfterBackup && config.CopyReady() == nil {
		operations = append(operations, domain.BackupOpCopy)
	}
	if config.Retention.Enabled {
		operations = append(operations, domain.BackupOpForgetPrune)
	}
	if config.Check.Enabled {
		latest, err := u.store.LatestBackupRuns(ctx)
		if err == nil && (latest.LastCheck == nil || u.now().Sub(latest.LastCheck.StartedAt) >= domain.BackupCheckEvery) {
			operations = append(operations, domain.BackupOpCheck)
		}
	}
	for _, op := range operations {
		if ctx.Err() != nil || u.claim() != nil {
			return
		}
		storeCtx, cancel := context.WithTimeout(ctx, backupStoreTimeout)
		run := domain.BackupRun{ID: uuid.New(), Operation: op, Origin: origin, State: domain.BackupRunning, StartedAt: u.now().UTC()}
		err := u.store.StartBackupRun(storeCtx, run, nil)
		cancel()
		if err != nil {
			u.release()
			u.logger.Warn("could not record a backup maintenance", zap.Error(err))
			return
		}
		u.execute(ctx, run, operationInput{})
	}
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

// execute runs a run already recorded as running and claimed, records its
// outcome and returns the finished run.
func (u *BackupUsecase) execute(ctx context.Context, run domain.BackupRun, input operationInput) domain.BackupRun {
	defer u.release()
	if run.Details == nil {
		run.Details = input.initialDetails(run.ID)
	}
	result, copyConnects, err := u.runOperation(ctx, run, input)
	finished := u.now().UTC()
	run.FinishedAt = &finished
	var finish ports.BackupFinish
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
		run.SnapshotID, run.Stats, finish.Snapshots = result.SnapshotID, result.Stats, result.Snapshots
		if result.Details != nil {
			run.Details = result.Details
		}
		// init creates the repository; snapshots connects an existing one.
		if run.Operation == domain.BackupOpInit || run.Operation == domain.BackupOpSnapshots {
			finish.InitializedAt = &finished
		}
		// The first copy creates or connects the copy repository.
		if copyConnects {
			finish.CopyInitializedAt = &finished
		}
		u.logger.Info("Backup operation done", zap.String("operation", string(run.Operation)), zap.String("snapshot", run.SnapshotID))
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backupStoreTimeout)
	defer cancel()
	if err := u.store.FinishBackupRun(storeCtx, run, finish); err != nil {
		u.logger.Error("could not record the outcome of a backup run", zap.Error(err))
	}
	return run
}

// runOperation executes the run; copyConnects reports a copy to a copy
// repository not yet initialized, which its success initializes.
func (u *BackupUsecase) runOperation(ctx context.Context, run domain.BackupRun, input operationInput) (domain.BackupResult, bool, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupResult{}, false, err
	}
	if config.Repository == nil || config.SealedCredentials == "" {
		return domain.BackupResult{}, false, domain.ErrBackupNotConfigured
	}
	credentials, err := u.openCredentials(config.SealedCredentials)
	if err != nil {
		return domain.BackupResult{}, false, err
	}
	job := ports.BackupJob{
		RunID: run.ID, Operation: run.Operation, Repository: *config.Repository, Selection: config.Selection,
		Retention: config.Retention, CheckSubsetPercent: config.Check.SubsetPercent, Restore: input.restore,
		Diff: input.diff,
	}
	if run.Operation != domain.BackupOpCopy {
		result, err := u.executor.Execute(ctx, job, credentials, nil)
		return result, false, err
	}
	target := config.Copy
	if target == nil || target.SealedCredentials == "" {
		return domain.BackupResult{}, false, domain.ErrBackupCopyNotConfigured
	}
	copyCredentials, err := u.openCopyCredentials(target.SealedCredentials)
	if err != nil {
		return domain.BackupResult{}, false, err
	}
	repository := target.Repository
	job.Copy, job.CopyInit = &repository, input.copyInit && target.InitializedAt == nil
	if config.Retention.Enabled {
		// The copy keeps the snapshots the main repository keeps.
		retention := config.Retention
		job.CopyRetention = &retention
	}
	result, err := u.executor.Execute(ctx, job, credentials, &copyCredentials)
	return result, err == nil && target.InitializedAt == nil, err
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
		Running: latest.Running, LastRun: latest.LastBackup, LastSuccess: latest.LastSuccess, LastCheck: latest.LastCheck,
		LastCopy: latest.LastCopy,
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

// checkCopy validates the copy repository against the main one, with
// their credentials when the secrets key can open them.
func (u *BackupUsecase) checkCopy(config domain.BackupConfig) error {
	if config.Copy == nil || config.Repository == nil {
		return nil
	}
	var main, copy *domain.BackupCredentials
	if config.SealedCredentials != "" && u.box != nil {
		if credentials, err := u.openCredentials(config.SealedCredentials); err == nil {
			main = &credentials
		}
	}
	if config.Copy.SealedCredentials != "" && u.box != nil {
		if credentials, err := u.openCopyCredentials(config.Copy.SealedCredentials); err == nil {
			copy = &credentials
		}
	}
	return domain.ValidateBackupCopy(*config.Repository, config.Copy.Repository, main, copy)
}

// SetCopy sets the copy repository, audited. A new repository loses the
// password of the previous one and must be initialized or connected.
// Callers must hold domain.PermManageBackups.
func (u *BackupUsecase) SetCopy(ctx context.Context, actor domain.User, update BackupCopyUpdate, requestID string) (BackupConfigView, error) {
	if err := domain.ValidateBackupRepository(update.Repository); err != nil {
		var validation *domain.BackupValidationError
		if errors.As(err, &validation) {
			return BackupConfigView{}, &domain.BackupValidationError{Field: "copy." + validation.Field, Message: validation.Message}
		}
		return BackupConfigView{}, err
	}
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return BackupConfigView{}, err
	}
	if config.Repository == nil {
		return BackupConfigView{}, domain.ErrBackupNotConfigured
	}
	if u.isRunning() {
		return BackupConfigView{}, domain.ErrBackupRepositoryInUse
	}
	target := domain.BackupCopyTarget{Repository: update.Repository, AfterBackup: update.AfterBackup}
	changed := config.Copy == nil || config.Copy.Repository != update.Repository
	if !changed {
		target.SealedCredentials, target.InitializedAt = config.Copy.SealedCredentials, config.Copy.InitializedAt
	}
	if update.Username != nil || update.Secret != nil || (changed && config.Copy != nil && config.Copy.SealedCredentials != "") {
		if u.box == nil {
			return BackupConfigView{}, domain.ErrBackupSecretsMissing
		}
		var credentials domain.BackupCredentials
		if config.Copy != nil && config.Copy.SealedCredentials != "" {
			if credentials, err = u.openCopyCredentials(config.Copy.SealedCredentials); err != nil {
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
			credentials.Password = ""
		}
		if err := validateCopyKeys(update.Repository.Type, credentials); err != nil {
			return BackupConfigView{}, err
		}
		if target.SealedCredentials, err = u.sealCopyCredentials(credentials); err != nil {
			return BackupConfigView{}, err
		}
	}
	config.Copy = &target
	if err := u.checkCopy(config); err != nil {
		return BackupConfigView{}, err
	}
	return u.saveCopy(ctx, actor, config, "update_copy", requestID)
}

// validateCopyKeys checks the access keys of the copy repository; its
// password comes later, at its initialisation or connection.
func validateCopyKeys(repositoryType domain.BackupRepositoryType, credentials domain.BackupCredentials) error {
	probe := credentials
	if probe.Password == "" {
		probe.Password = "-"
	}
	if repositoryType == domain.RepositoryS3 && probe.Username == "" && probe.Secret == "" {
		// The keys may come in a later update.
		return nil
	}
	if err := domain.ValidateBackupCredentials(repositoryType, probe); err != nil {
		var validation *domain.BackupValidationError
		if errors.As(err, &validation) {
			return &domain.BackupValidationError{Field: "copy." + validation.Field, Message: validation.Message}
		}
		return err
	}
	return nil
}

// RemoveCopy forgets the copy repository, audited; its data stays where
// it is. Callers must hold domain.PermManageBackups.
func (u *BackupUsecase) RemoveCopy(ctx context.Context, actor domain.User, requestID string) (BackupConfigView, error) {
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return BackupConfigView{}, err
	}
	if config.Copy == nil {
		return BackupConfigView{}, domain.ErrBackupCopyNotConfigured
	}
	if u.isRunning() {
		return BackupConfigView{}, domain.ErrBackupRepositoryInUse
	}
	config.Copy = nil
	return u.saveCopy(ctx, actor, config, "remove_copy", requestID)
}

func (u *BackupUsecase) saveCopy(ctx context.Context, actor domain.User, config domain.BackupConfig, action, requestID string) (BackupConfigView, error) {
	now := u.now().UTC()
	config.UpdatedAt, config.UpdatedBy = &now, actor.Username
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupConfig, TargetID: "local", Action: action,
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveBackupConfig(ctx, config, audit); err != nil {
		return BackupConfigView{}, err
	}
	return u.Config(ctx)
}

// InitCopy generates the password of the copy repository, seals it and
// starts a copy that creates the repository first. The password is
// returned once. Callers must hold domain.PermManageBackups.
func (u *BackupUsecase) InitCopy(ctx context.Context, actor domain.User, requestID string) (string, domain.BackupRun, error) {
	random := make([]byte, domain.BackupPasswordBytes)
	if _, err := rand.Read(random); err != nil {
		return "", domain.BackupRun{}, fmt.Errorf("generate the copy repository password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(random)
	run, err := u.startCopy(ctx, actor, password, true, "init_copy", requestID)
	if err != nil {
		return "", domain.BackupRun{}, err
	}
	return password, run, nil
}

// ConnectCopy attaches an existing copy repository with its password: the
// first copy opens it, and its success marks it initialized. Callers must
// hold domain.PermManageBackups.
func (u *BackupUsecase) ConnectCopy(ctx context.Context, actor domain.User, password, requestID string) (domain.BackupRun, error) {
	return u.startCopy(ctx, actor, password, false, "connect_copy", requestID)
}

func (u *BackupUsecase) startCopy(ctx context.Context, actor domain.User, password string, init bool, action, requestID string) (domain.BackupRun, error) {
	if u.box == nil {
		return domain.BackupRun{}, domain.ErrBackupSecretsMissing
	}
	config, err := u.store.BackupConfig(ctx)
	if err != nil {
		return domain.BackupRun{}, err
	}
	if err := config.Operations(); err != nil {
		return domain.BackupRun{}, err
	}
	if config.Copy == nil {
		return domain.BackupRun{}, domain.ErrBackupCopyNotConfigured
	}
	if config.Copy.InitializedAt != nil {
		return domain.BackupRun{}, domain.ErrBackupCopyAlreadyInitialized
	}
	var credentials domain.BackupCredentials
	if config.Copy.SealedCredentials != "" {
		if credentials, err = u.openCopyCredentials(config.Copy.SealedCredentials); err != nil {
			return domain.BackupRun{}, err
		}
	}
	credentials.Password = password
	if err := domain.ValidateBackupCredentials(config.Copy.Repository.Type, credentials); err != nil {
		var validation *domain.BackupValidationError
		if errors.As(err, &validation) {
			return domain.BackupRun{}, &domain.BackupValidationError{Field: "copy." + validation.Field, Message: validation.Message}
		}
		return domain.BackupRun{}, err
	}
	if err := u.claim(); err != nil {
		return domain.BackupRun{}, err
	}
	if config.Copy.SealedCredentials, err = u.sealCopyCredentials(credentials); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	if err := u.checkCopy(config); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	now := u.now().UTC()
	config.UpdatedAt, config.UpdatedBy = &now, actor.Username
	audit := domain.AuditEntry{
		Actor: actor.Username, TargetType: auditTargetBackupConfig, TargetID: "local", Action: action,
		RequestID: requestID, Outcome: domain.AuditSuccess,
	}
	if err := u.store.SaveBackupConfig(ctx, config, audit); err != nil {
		u.release()
		return domain.BackupRun{}, err
	}
	return u.enqueue(ctx, domain.BackupOpCopy, domain.BackupManual, actor.Username, nil, operationInput{copyInit: init})
}
