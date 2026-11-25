package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"go.uber.org/zap"
)

func (f *fakeBackupExecutor) lastJob() (ports.BackupJob, *domain.BackupCredentials) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[len(f.jobs)-1], f.copies[len(f.copies)-1]
}

func TestBackupDiff(t *testing.T) {
	diff := &domain.BackupDiff{From: "3a253ad8", To: "9f2c11b0", ChangedFiles: 1, Changes: []domain.BackupDiffChange{{Path: "/etc/hosts", Modifier: "M"}}}
	executor := &fakeBackupExecutor{result: domain.BackupResult{Details: &domain.BackupRunDetails{Diff: diff}}}
	backups, store := initializedBackups(t, executor)
	ctx := context.Background()

	var validation *domain.BackupValidationError
	for _, request := range []BackupOperationRequest{{}, {Diff: &domain.BackupDiffRequest{From: "3a253ad8", To: "3a253ad8"}}} {
		if _, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpDiff, request, "r"); !errors.As(err, &validation) {
			t.Fatalf("invalid diff %+v = %v", request, err)
		}
	}
	run, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpDiff,
		BackupOperationRequest{Diff: &domain.BackupDiffRequest{From: "3a253ad8", To: "9f2c11b0"}}, "req")
	if err != nil || run.Details == nil || run.Details.Diff == nil || run.Details.Diff.To != "9f2c11b0" {
		t.Fatalf("diff = %+v, %v", run, err)
	}
	done := finishedRun(t, store, run.ID)
	if done.State != domain.BackupSucceeded || done.Details.Diff.Changes[0].Path != "/etc/hosts" {
		t.Fatalf("diff run = %+v", done)
	}
	job, copyCredentials := executor.lastJob()
	if job.Diff == nil || job.Diff.From != "3a253ad8" || job.Copy != nil || copyCredentials != nil {
		t.Fatalf("diff job = %+v", job)
	}
	if audit := store.audits[len(store.audits)-1]; audit.Action != "diff" || audit.TargetType != auditTargetBackupRun {
		t.Fatalf("audit = %+v", audit)
	}
}

var copyRepository = domain.BackupRepository{Type: domain.RepositoryRest, Location: "https://nas.lan:8000/copy"}

func TestBackupCopyLifecycle(t *testing.T) {
	executor := &fakeBackupExecutor{result: domain.BackupResult{Details: &domain.BackupRunDetails{Copy: &domain.BackupCopyResult{SnapshotsCopied: 1}}}}
	backups, store := initializedBackups(t, executor)
	ctx := context.Background()
	user, secret := "copy-user", "copy-secret"

	if _, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpCopy, BackupOperationRequest{}, "r"); !errors.Is(err, domain.ErrBackupCopyNotConfigured) {
		t.Fatalf("copy without a copy repository = %v", err)
	}
	var validation *domain.BackupValidationError
	same := BackupCopyUpdate{Repository: domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"}}
	if _, err := backups.SetCopy(ctx, backupAdmin, same, "r"); !errors.As(err, &validation) || validation.Field != "copy.location" {
		t.Fatalf("copy to the main repository = %v", err)
	}
	view, err := backups.SetCopy(ctx, backupAdmin, BackupCopyUpdate{Repository: copyRepository, Username: &user, Secret: &secret, AfterBackup: true}, "r")
	if err != nil || view.Copy == nil || view.Copy.Initialized || view.Copy.HasPassword || !view.Copy.HasUsername || !view.Copy.AfterBackup {
		t.Fatalf("set copy = %+v, %v", view.Copy, err)
	}
	if !strings.HasPrefix(store.config.Copy.SealedCredentials, domain.BackupCopySecretsAssociatedData+"|") {
		t.Fatal("the copy credentials are sealed for the copy repository")
	}
	if _, err := backups.RequestOperation(ctx, backupAdmin, domain.BackupOpCopy, BackupOperationRequest{}, "r"); !errors.Is(err, domain.ErrBackupCopyNotInitialized) {
		t.Fatalf("copy before the initialisation = %v", err)
	}

	password, run, err := backups.InitCopy(ctx, backupAdmin, "req-init")
	if err != nil || len(password) < 40 || run.Operation != domain.BackupOpCopy {
		t.Fatalf("init copy = %q %+v %v", password, run, err)
	}
	finishedRun(t, store, run.ID)
	job, copyCredentials := executor.lastJob()
	if job.Copy == nil || *job.Copy != copyRepository || !job.CopyInit || job.CopyRetention != nil {
		t.Fatalf("init job = %+v", job)
	}
	if copyCredentials == nil || copyCredentials.Password != password || copyCredentials.Username != user || copyCredentials.Secret != secret {
		t.Fatalf("copy credentials = %+v", copyCredentials)
	}
	if view, _ := backups.Config(ctx); view.Copy == nil || !view.Copy.Initialized || !view.Copy.HasPassword {
		t.Fatalf("after init = %+v", view.Copy)
	}
	if _, _, err := backups.InitCopy(ctx, backupAdmin, "r"); !errors.Is(err, domain.ErrBackupCopyAlreadyInitialized) {
		t.Fatalf("second init = %v", err)
	}

	run, err = backups.RequestOperation(ctx, backupAdmin, domain.BackupOpCopy, BackupOperationRequest{}, "req")
	if err != nil {
		t.Fatal(err)
	}
	if done := finishedRun(t, store, run.ID); done.State != domain.BackupSucceeded || done.Details.Copy.SnapshotsCopied != 1 {
		t.Fatalf("copy run = %+v", done)
	}
	if job, _ := executor.lastJob(); job.CopyInit {
		t.Fatal("a copy to an initialized repository does not initialize it")
	}

	// Keeping the repository keeps its password; another one loses it.
	if view, _ := backups.SetCopy(ctx, backupAdmin, BackupCopyUpdate{Repository: copyRepository}, "r"); !view.Copy.Initialized || view.Copy.AfterBackup {
		t.Fatalf("same repository = %+v", view.Copy)
	}
	other := domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/media/usb/copy"}
	empty := ""
	view, err = backups.SetCopy(ctx, backupAdmin, BackupCopyUpdate{Repository: other, Username: &empty, Secret: &empty}, "r")
	if err != nil || view.Copy.Initialized || view.Copy.HasPassword || view.Copy.HasUsername {
		t.Fatalf("other repository = %+v, %v", view.Copy, err)
	}

	// A failed connection leaves the repository to connect.
	executor.err = &domain.BackupRunError{Failure: domain.BackupFailureRestic, Message: "wrong password", Err: errors.New("restic")}
	run, err = backups.ConnectCopy(ctx, backupAdmin, "old-password", "r")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, run.ID)
	if job, credentials := executor.lastJob(); job.CopyInit || credentials.Password != "old-password" {
		t.Fatalf("connect job = %+v", job)
	}
	if view, _ := backups.Config(ctx); view.Copy.Initialized {
		t.Fatal("a failed connection must not initialize the copy repository")
	}
	executor.err = nil
	run, _ = backups.ConnectCopy(ctx, backupAdmin, "old-password", "r")
	finishedRun(t, store, run.ID)
	if view, _ := backups.Config(ctx); !view.Copy.Initialized {
		t.Fatal("a successful connection initializes the copy repository")
	}

	if _, err := backups.RemoveCopy(ctx, backupAdmin, "r"); err != nil || store.config.Copy != nil {
		t.Fatalf("remove = %v", err)
	}
	if _, err := backups.RemoveCopy(ctx, backupAdmin, "r"); !errors.Is(err, domain.ErrBackupCopyNotConfigured) {
		t.Fatalf("second remove = %v", err)
	}
	var actions []string
	for _, audit := range store.audits {
		actions = append(actions, audit.Action)
	}
	if got := strings.Join(actions, ","); !strings.Contains(got, "update_copy,init_copy,copy,update_copy,update_copy,connect_copy,connect_copy,remove_copy") {
		t.Fatalf("audits = %s", got)
	}
}

func TestBackupCopyRefusesTwoS3Keys(t *testing.T) {
	backups, _ := initializedBackups(t, &fakeBackupExecutor{})
	ctx := context.Background()
	key, secret := "main-key", "main-secret"
	_, err := backups.UpdateConfig(ctx, backupAdmin, BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositoryS3, Location: "https://s3.example.com/main"},
		Username:   &key, Secret: &secret,
		Selection: domain.BackupSelection{Sources: []string{"/etc"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r")
	if err != nil {
		t.Fatal(err)
	}
	otherKey, otherSecret := "copy-key", "copy-secret"
	s3 := domain.BackupRepository{Type: domain.RepositoryS3, Location: "https://s3.example.com/copy"}
	var validation *domain.BackupValidationError
	if _, err := backups.SetCopy(ctx, backupAdmin, BackupCopyUpdate{Repository: s3, Username: &otherKey, Secret: &otherSecret}, "r"); !errors.As(err, &validation) || validation.Field != "copy.username" {
		t.Fatalf("two S3 keys = %v", err)
	}
	if _, err := backups.SetCopy(ctx, backupAdmin, BackupCopyUpdate{Repository: s3, Username: &key, Secret: &secret}, "r"); err != nil {
		t.Fatalf("same S3 key = %v", err)
	}
	// Changing the main repository to the copy one is refused too.
	if _, err := backups.UpdateConfig(ctx, backupAdmin, BackupConfigUpdate{
		Repository: &s3, Selection: domain.BackupSelection{Sources: []string{"/etc"}}, Schedule: domain.DefaultBackupSchedule,
	}, "r"); !errors.As(err, &validation) || validation.Field != "copy.location" {
		t.Fatalf("main repository on the copy = %v", err)
	}
}

func TestBackupCopyAfterTheScheduledBackup(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	initialized := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	store.config = domain.BackupConfig{
		Repository:        &domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"},
		SealedCredentials: domain.BackupSecretsAssociatedData + `|{"password":"p"}`, InitializedAt: &initialized,
		Selection: domain.BackupSelection{Sources: []string{"/etc"}},
		Schedule:  domain.BackupSchedule{Enabled: true, Frequency: domain.BackupDaily, Hour: 3},
		Retention: domain.BackupRetention{Enabled: true, KeepDaily: 7},
		Check:     domain.BackupCheck{SubsetPercent: 5},
		Copy: &domain.BackupCopyTarget{
			Repository: copyRepository, InitializedAt: &initialized, AfterBackup: true,
			SealedCredentials: domain.BackupCopySecretsAssociatedData + `|{"password":"c"}`,
		},
		UpdatedAt: &initialized,
	}
	var mu sync.Mutex
	now := time.Date(2026, 11, 21, 3, 0, 5, 0, time.UTC)
	backups := NewBackupUsecase(store, executor, fakeBox{}, nil, zap.NewNop())
	backups.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	backups.location, backups.tick = time.UTC, 5*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		backups.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	waitFor(t, func() bool { return executor.calls() == 3 })
	ops := executor.operations()
	if ops[0] != domain.BackupOpBackup || ops[1] != domain.BackupOpCopy || ops[2] != domain.BackupOpForgetPrune {
		t.Fatalf("operations = %v", ops)
	}
	executor.mu.Lock()
	copyJob, copyCredentials := executor.jobs[1], executor.copies[1]
	executor.mu.Unlock()
	if copyJob.CopyRetention == nil || copyJob.CopyRetention.KeepDaily != 7 || copyJob.CopyInit || copyCredentials.Password != "c" {
		t.Fatalf("copy job = %+v", copyJob)
	}
	summary, err := backups.Summary(ctx)
	if err != nil || summary.LastCopy == nil || summary.LastCopy.Origin != domain.BackupScheduled {
		t.Fatalf("summary = %+v, %v", summary.LastCopy, err)
	}
}
