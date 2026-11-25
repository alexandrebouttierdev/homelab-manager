package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"homelab-manager/backend/ent"
	entconfig "homelab-manager/backend/ent/backupconfig"
	entrun "homelab-manager/backend/ent/backuprun"
	entsnapshot "homelab-manager/backend/ent/backupsnapshot"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

// snapshotBatch bounds one bulk insert of snapshots.
const snapshotBatch = 200

var _ ports.BackupStore = (*BackupRepository)(nil)

// BackupRepository stores the configuration, runs and snapshots of the
// backups module.
type BackupRepository struct {
	client *ent.Client
}

func NewBackupRepository(client *ent.Client) *BackupRepository {
	return &BackupRepository{client: client}
}

func (r *BackupRepository) BackupConfig(ctx context.Context) (domain.BackupConfig, error) {
	record, err := r.client.BackupConfig.Query().Where(entconfig.SingletonKey(settingsSingletonKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.BackupConfig{
			Selection: domain.BackupSelection{ExcludeCaches: true}, Schedule: domain.DefaultBackupSchedule,
			Retention: domain.DefaultBackupRetention, Check: domain.DefaultBackupCheck,
		}, nil
	}
	if err != nil {
		return domain.BackupConfig{}, fmt.Errorf("read backup configuration: %w", err)
	}
	config := domain.BackupConfig{
		SealedCredentials: record.SealedCredentials, InitializedAt: utcPointer(record.InitializedAt),
		Selection: domain.BackupSelection{
			Sources: record.Sources, Excludes: record.Excludes,
			ExcludeCaches: record.ExcludeCaches, ExcludeIfPresent: record.ExcludeIfPresent,
			SQLiteDatabases: record.SqliteDatabases,
		},
		Schedule: domain.BackupSchedule{
			Enabled: record.ScheduleEnabled, Frequency: domain.BackupFrequency(record.ScheduleFrequency),
			Weekday: time.Weekday(record.ScheduleWeekday), Hour: record.ScheduleHour, Minute: record.ScheduleMinute,
		},
		Retention: domain.BackupRetention{
			Enabled: record.RetentionEnabled, KeepDaily: record.KeepDaily, KeepWeekly: record.KeepWeekly,
			KeepMonthly: record.KeepMonthly, KeepYearly: record.KeepYearly,
		},
		Check:     domain.BackupCheck{Enabled: record.CheckEnabled, SubsetPercent: record.CheckSubsetPercent},
		UpdatedBy: record.UpdatedBy,
	}
	updated := record.UpdatedAt.UTC()
	config.UpdatedAt = &updated
	if record.RepositoryType != nil && record.RepositoryLocation != nil {
		config.Repository = &domain.BackupRepository{
			Type: domain.BackupRepositoryType(*record.RepositoryType), Location: *record.RepositoryLocation,
			HostKey: record.RepositoryHostKey,
		}
	}
	if record.CopyRepositoryType != nil && record.CopyRepositoryLocation != nil {
		config.Copy = &domain.BackupCopyTarget{
			Repository: domain.BackupRepository{
				Type: domain.BackupRepositoryType(*record.CopyRepositoryType), Location: *record.CopyRepositoryLocation,
				HostKey: record.CopyRepositoryHostKey,
			},
			SealedCredentials: record.CopySealedCredentials, InitializedAt: utcPointer(record.CopyInitializedAt),
			AfterBackup: record.CopyAfterBackup,
		}
	}
	if record.SSHPublicKey != "" && record.SSHKeyCreatedAt != nil {
		config.SSHKey = &domain.BackupSSHKey{
			PublicKey: record.SSHPublicKey, SealedPrivateKey: record.SSHSealedPrivateKey, CreatedAt: record.SSHKeyCreatedAt.UTC(),
		}
	}
	return config, nil
}

func utcPointer(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func (r *BackupRepository) SaveBackupConfig(ctx context.Context, config domain.BackupConfig, audit domain.AuditEntry) error {
	updated := time.Now().UTC()
	if config.UpdatedAt != nil {
		updated = config.UpdatedAt.UTC()
	}
	sources, excludes := config.Selection.Sources, config.Selection.Excludes
	if sources == nil {
		sources = []string{}
	}
	if excludes == nil {
		excludes = []string{}
	}
	databases := config.Selection.SQLiteDatabases
	if databases == nil {
		databases = []string{}
	}
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		id, err := tx.BackupConfig.Query().Where(entconfig.SingletonKey(settingsSingletonKey)).OnlyID(ctx)
		if ent.IsNotFound(err) {
			id = uuid.New()
			err = tx.BackupConfig.Create().SetID(id).SetSingletonKey(settingsSingletonKey).
				SetSources([]string{}).SetExcludes([]string{}).SetUpdatedAt(updated).Exec(ctx)
		}
		if err != nil {
			return fmt.Errorf("find backup configuration: %w", err)
		}
		update := tx.BackupConfig.UpdateOneID(id).
			SetSealedCredentials(config.SealedCredentials).
			SetSources(sources).SetExcludes(excludes).SetSqliteDatabases(databases).
			SetExcludeCaches(config.Selection.ExcludeCaches).SetExcludeIfPresent(config.Selection.ExcludeIfPresent).
			SetScheduleEnabled(config.Schedule.Enabled).SetScheduleFrequency(string(config.Schedule.Frequency)).
			SetScheduleWeekday(int(config.Schedule.Weekday)).SetScheduleHour(config.Schedule.Hour).
			SetScheduleMinute(config.Schedule.Minute).SetUpdatedAt(updated).SetUpdatedBy(config.UpdatedBy).
			SetRetentionEnabled(config.Retention.Enabled).SetKeepDaily(config.Retention.KeepDaily).
			SetKeepWeekly(config.Retention.KeepWeekly).SetKeepMonthly(config.Retention.KeepMonthly).
			SetKeepYearly(config.Retention.KeepYearly).SetCheckEnabled(config.Check.Enabled)
		if config.Check.SubsetPercent > 0 {
			update.SetCheckSubsetPercent(config.Check.SubsetPercent)
		}
		if config.Repository != nil {
			update.SetRepositoryType(string(config.Repository.Type)).SetRepositoryLocation(config.Repository.Location).
				SetRepositoryHostKey(config.Repository.HostKey)
		} else {
			update.ClearRepositoryType().ClearRepositoryLocation().SetRepositoryHostKey("")
		}
		if key := config.SSHKey; key != nil {
			update.SetSSHPublicKey(key.PublicKey).SetSSHSealedPrivateKey(key.SealedPrivateKey).
				SetSSHKeyCreatedAt(key.CreatedAt.UTC())
		} else {
			update.SetSSHPublicKey("").SetSSHSealedPrivateKey("").ClearSSHKeyCreatedAt()
		}
		if config.InitializedAt != nil {
			update.SetInitializedAt(config.InitializedAt.UTC())
		} else {
			update.ClearInitializedAt()
		}
		if copy := config.Copy; copy != nil {
			update.SetCopyRepositoryType(string(copy.Repository.Type)).SetCopyRepositoryLocation(copy.Repository.Location).
				SetCopyRepositoryHostKey(copy.Repository.HostKey).
				SetCopySealedCredentials(copy.SealedCredentials).SetCopyAfterBackup(copy.AfterBackup)
			if copy.InitializedAt != nil {
				update.SetCopyInitializedAt(copy.InitializedAt.UTC())
			} else {
				update.ClearCopyInitializedAt()
			}
		} else {
			update.ClearCopyRepositoryType().ClearCopyRepositoryLocation().SetCopyRepositoryHostKey("").SetCopySealedCredentials("").
				ClearCopyInitializedAt().SetCopyAfterBackup(false)
		}
		if err := update.Exec(ctx); err != nil {
			return fmt.Errorf("save backup configuration: %w", err)
		}
		return writeAudit(ctx, tx, audit)
	})
}

func (r *BackupRepository) StartBackupRun(ctx context.Context, run domain.BackupRun, audit *domain.AuditEntry) error {
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		create := tx.BackupRun.Create().SetID(run.ID).SetOperation(string(run.Operation)).SetOrigin(string(run.Origin)).
			SetState(string(domain.BackupRunning)).SetStartedAt(run.StartedAt.UTC())
		if run.RequestedBy != "" {
			create.SetRequestedBy(run.RequestedBy)
		}
		if err := create.Exec(ctx); err != nil {
			return fmt.Errorf("record backup run: %w", err)
		}
		if audit == nil {
			return nil
		}
		return writeAudit(ctx, tx, *audit)
	})
}

func (r *BackupRepository) FinishBackupRun(ctx context.Context, run domain.BackupRun, finish ports.BackupFinish) error {
	snapshots, initializedAt := finish.Snapshots, finish.InitializedAt
	return withTx(ctx, r.client, func(tx *ent.Tx) error {
		update := tx.BackupRun.UpdateOneID(run.ID).SetState(string(run.State)).SetMessage(run.Message).
			SetWarning(run.Warning).SetSnapshotID(run.SnapshotID)
		if run.FinishedAt != nil {
			update.SetFinishedAt(run.FinishedAt.UTC())
		}
		if run.Failure != "" {
			update.SetFailure(string(run.Failure))
		}
		if stats := run.Stats; stats != nil {
			update.SetFilesNew(stats.FilesNew).SetFilesChanged(stats.FilesChanged).SetFilesUnmodified(stats.FilesUnmodified).
				SetDirsNew(stats.DirsNew).SetDirsChanged(stats.DirsChanged).SetDirsUnmodified(stats.DirsUnmodified).
				SetDataAdded(stats.DataAdded).SetTotalFilesProcessed(stats.TotalFilesProcessed).
				SetTotalBytesProcessed(stats.TotalBytesProcessed).SetDurationSeconds(stats.DurationSeconds)
		}
		if run.Details != nil {
			details, err := json.Marshal(run.Details)
			if err != nil {
				return fmt.Errorf("encode run details: %w", err)
			}
			update.SetDetails(string(details))
		}
		if err := update.Exec(ctx); err != nil {
			return fmt.Errorf("finish backup run: %w", err)
		}
		if initializedAt != nil {
			if _, err := tx.BackupConfig.Update().Where(entconfig.SingletonKey(settingsSingletonKey)).
				SetInitializedAt(initializedAt.UTC()).Save(ctx); err != nil {
				return fmt.Errorf("mark the repository initialized: %w", err)
			}
		}
		if finish.CopyInitializedAt != nil {
			// The copy repository may have been removed meanwhile: only a
			// configured one is marked.
			if _, err := tx.BackupConfig.Update().
				Where(entconfig.SingletonKey(settingsSingletonKey), entconfig.CopyRepositoryLocationNotNil()).
				SetCopyInitializedAt(finish.CopyInitializedAt.UTC()).Save(ctx); err != nil {
				return fmt.Errorf("mark the copy repository initialized: %w", err)
			}
		}
		if snapshots == nil {
			return nil
		}
		if _, err := tx.BackupSnapshot.Delete().Exec(ctx); err != nil {
			return fmt.Errorf("clear snapshots: %w", err)
		}
		for start := 0; start < len(snapshots); start += snapshotBatch {
			batch := snapshots[start:min(start+snapshotBatch, len(snapshots))]
			creates := make([]*ent.BackupSnapshotCreate, 0, len(batch))
			for _, s := range batch {
				creates = append(creates, tx.BackupSnapshot.Create().SetID(s.ID).SetShortID(s.ShortID).
					SetTakenAt(s.Time.UTC()).SetHostname(s.Hostname).SetPaths(orEmptyList(s.Paths)).
					SetTags(orEmptyList(s.Tags)).SetTotalFiles(s.TotalFiles).SetTotalBytes(s.TotalBytes).
					SetDataAdded(s.DataAdded))
			}
			if err := tx.BackupSnapshot.CreateBulk(creates...).Exec(ctx); err != nil {
				return fmt.Errorf("store snapshots: %w", err)
			}
		}
		return nil
	})
}

func orEmptyList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (r *BackupRepository) InterruptRunningBackupRuns(ctx context.Context, at time.Time) error {
	if _, err := r.client.BackupRun.Update().Where(entrun.State(string(domain.BackupRunning))).
		SetState(string(domain.BackupFailed)).SetFailure(string(domain.BackupFailureInterrupted)).
		SetFinishedAt(at.UTC()).Save(ctx); err != nil {
		return fmt.Errorf("interrupt backup runs: %w", err)
	}
	return nil
}

func (r *BackupRepository) LatestBackupRuns(ctx context.Context) (ports.BackupLatest, error) {
	var latest ports.BackupLatest
	first := func(query *ent.BackupRunQuery) (*domain.BackupRun, error) {
		record, err := query.Order(ent.Desc(entrun.FieldStartedAt)).First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read backup runs: %w", err)
		}
		run := toBackupRun(record)
		return &run, nil
	}
	var err error
	backups := func() *ent.BackupRunQuery {
		return r.client.BackupRun.Query().Where(entrun.Operation(string(domain.BackupOpBackup)))
	}
	if latest.Running, err = first(r.client.BackupRun.Query().Where(entrun.State(string(domain.BackupRunning)))); err != nil {
		return latest, err
	}
	if latest.LastBackup, err = first(backups()); err != nil {
		return latest, err
	}
	if latest.LastSuccess, err = first(backups().Where(entrun.State(string(domain.BackupSucceeded)))); err != nil {
		return latest, err
	}
	if latest.LastScheduled, err = first(backups().Where(entrun.OriginIn(string(domain.BackupScheduled), string(domain.BackupCatchUp)))); err != nil {
		return latest, err
	}
	if latest.LastCheck, err = first(r.client.BackupRun.Query().Where(
		entrun.Operation(string(domain.BackupOpCheck)), entrun.State(string(domain.BackupSucceeded)))); err != nil {
		return latest, err
	}
	latest.LastCopy, err = first(r.client.BackupRun.Query().Where(entrun.Operation(string(domain.BackupOpCopy))))
	return latest, err
}

func (r *BackupRepository) BackupRuns(ctx context.Context, filter domain.BackupRunFilter) ([]domain.BackupRun, error) {
	records, err := r.client.BackupRun.Query().Order(ent.Desc(entrun.FieldStartedAt)).Limit(filter.Limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backup runs: %w", err)
	}
	runs := make([]domain.BackupRun, 0, len(records))
	for _, record := range records {
		runs = append(runs, toBackupRun(record))
	}
	return runs, nil
}

func (r *BackupRepository) BackupRun(ctx context.Context, id uuid.UUID) (domain.BackupRun, error) {
	record, err := r.client.BackupRun.Get(ctx, id)
	if ent.IsNotFound(err) {
		return domain.BackupRun{}, domain.ErrBackupRunNotFound
	}
	if err != nil {
		return domain.BackupRun{}, fmt.Errorf("read backup run: %w", err)
	}
	return toBackupRun(record), nil
}

func (r *BackupRepository) BackupSnapshots(ctx context.Context) ([]domain.BackupSnapshot, error) {
	records, err := r.client.BackupSnapshot.Query().Order(ent.Desc(entsnapshot.FieldTakenAt)).
		Limit(domain.MaxBackupSnapshots).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	snapshots := make([]domain.BackupSnapshot, 0, len(records))
	for _, record := range records {
		snapshots = append(snapshots, domain.BackupSnapshot{
			ID: record.ID, ShortID: record.ShortID, Time: record.TakenAt.UTC(), Hostname: record.Hostname,
			Paths: record.Paths, Tags: record.Tags, TotalFiles: record.TotalFiles, TotalBytes: record.TotalBytes,
			DataAdded: record.DataAdded,
		})
	}
	return snapshots, nil
}

func (r *BackupRepository) PurgeBackupRuns(ctx context.Context, before time.Time) error {
	if _, err := r.client.BackupRun.Delete().
		Where(entrun.StartedAtLT(before.UTC()), entrun.StateNEQ(string(domain.BackupRunning))).Exec(ctx); err != nil {
		return fmt.Errorf("purge backup runs: %w", err)
	}
	return nil
}

func toBackupRun(record *ent.BackupRun) domain.BackupRun {
	run := domain.BackupRun{
		ID: record.ID, Operation: domain.BackupOperation(record.Operation), Origin: domain.BackupOrigin(record.Origin),
		State: domain.BackupRunState(record.State), Message: record.Message, Warning: record.Warning,
		SnapshotID: record.SnapshotID, StartedAt: record.StartedAt.UTC(), FinishedAt: utcPointer(record.FinishedAt),
	}
	if record.RequestedBy != nil {
		run.RequestedBy = *record.RequestedBy
	}
	if record.Failure != nil {
		run.Failure = domain.BackupFailure(*record.Failure)
	}
	if record.Details != nil {
		var details domain.BackupRunDetails
		if json.Unmarshal([]byte(*record.Details), &details) == nil {
			run.Details = &details
		}
	}
	if record.TotalFilesProcessed != nil {
		value := func(v *int64) int64 {
			if v == nil {
				return 0
			}
			return *v
		}
		run.Stats = &domain.BackupStats{
			FilesNew: value(record.FilesNew), FilesChanged: value(record.FilesChanged),
			FilesUnmodified: value(record.FilesUnmodified), DirsNew: value(record.DirsNew),
			DirsChanged: value(record.DirsChanged), DirsUnmodified: value(record.DirsUnmodified),
			DataAdded: value(record.DataAdded), TotalFilesProcessed: value(record.TotalFilesProcessed),
			TotalBytesProcessed: value(record.TotalBytesProcessed),
		}
		if record.DurationSeconds != nil {
			run.Stats.DurationSeconds = *record.DurationSeconds
		}
	}
	return run
}

// BackupAlertStatus gives what the backup alert rule watches; known is
// false while no repository is initialized.
func (r *BackupRepository) BackupAlertStatus(ctx context.Context) (domain.BackupAlertStatus, bool, error) {
	config, err := r.BackupConfig(ctx)
	if err != nil {
		return domain.BackupAlertStatus{}, false, err
	}
	if config.Repository == nil || config.InitializedAt == nil {
		return domain.BackupAlertStatus{}, false, nil
	}
	latest, err := r.LatestBackupRuns(ctx)
	if err != nil {
		return domain.BackupAlertStatus{}, false, err
	}
	status := domain.BackupAlertStatus{Reference: *config.InitializedAt}
	if latest.LastSuccess != nil {
		status.Reference = latest.LastSuccess.StartedAt
		if latest.LastSuccess.FinishedAt != nil {
			status.Reference = *latest.LastSuccess.FinishedAt
		}
	}
	status.LastRunFailed = latest.LastBackup != nil && latest.LastBackup.State == domain.BackupFailed
	return status, true, nil
}
