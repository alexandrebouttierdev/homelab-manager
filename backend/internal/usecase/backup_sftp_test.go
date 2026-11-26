package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"monoops/backend/internal/adapters/sshkey"
	"monoops/backend/internal/domain"

	"go.uber.org/zap"
)

func newSFTPBackups(t *testing.T, store *memoryBackupStore, executor *fakeBackupExecutor) *BackupUsecase {
	t.Helper()
	backups := NewBackupUsecase(store, executor, fakeBox{}, sshkey.Keys{}, zap.NewNop())
	backups.tick = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		backups.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitAccepting(t, backups)
	return backups
}

func serverHostKey(t *testing.T) (pasted, stored string) {
	t.Helper()
	_, public, err := (sshkey.Keys{}).Generate("root@nas")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(public)
	// As ssh-keyscan prints it.
	return "# nas.lan:22 SSH-2.0-OpenSSH_10.0\nnas.lan " + fields[0] + " " + fields[1] + "\n", fields[0] + " " + fields[1]
}

func sftpUpdate(hostKey string) BackupConfigUpdate {
	return BackupConfigUpdate{
		Repository: &domain.BackupRepository{Type: domain.RepositorySFTP, Location: "sftp://backup@nas.lan/srv/restic", HostKey: hostKey},
		Selection:  domain.BackupSelection{Sources: []string{"/etc"}},
		Schedule:   domain.DefaultBackupSchedule,
	}
}

func TestSFTPRepositoryNeedsTheSSHKeyOfTheManager(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	backups := newSFTPBackups(t, store, executor)
	ctx := context.Background()
	pasted, stored := serverHostKey(t)
	if _, err := backups.UpdateConfig(ctx, backupAdmin, sftpUpdate(pasted), "r"); !errors.Is(err, domain.ErrBackupSSHKeyMissing) {
		t.Fatalf("SFTP without SSH key = %v", err)
	}

	view, err := backups.GenerateSSHKey(ctx, backupAdmin, "req-key")
	if err != nil || view.SSHKey == nil || !strings.HasPrefix(view.SSHKey.PublicKey, "ssh-ed25519 ") ||
		!strings.HasSuffix(view.SSHKey.PublicKey, " "+domain.BackupSSHKeyComment) || !strings.HasPrefix(view.SSHKey.Fingerprint, "SHA256:") {
		t.Fatalf("generate = %+v, %v", view.SSHKey, err)
	}
	if audit := store.audits[len(store.audits)-1]; audit.Action != "generate_ssh_key" || audit.Actor != "alice" {
		t.Fatalf("audit = %+v", audit)
	}
	sealed := store.config.SSHKey.SealedPrivateKey
	if !strings.HasPrefix(sealed, domain.BackupSSHKeyAssociatedData+"|-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Fatalf("the private key is not sealed with its associated data: %.40q", sealed)
	}

	var validation *domain.BackupValidationError
	if _, err := backups.UpdateConfig(ctx, backupAdmin, sftpUpdate("not a key"), "r"); !errors.As(err, &validation) || validation.Field != "hostKey" {
		t.Fatalf("invalid host key = %v", err)
	}
	view, err = backups.UpdateConfig(ctx, backupAdmin, sftpUpdate(pasted), "r")
	if err != nil || view.Repository.HostKey != stored || !strings.HasPrefix(view.HostKeyFingerprint, "SHA256:") {
		t.Fatalf("SFTP config = %+v (%s), %v", view.Repository, view.HostKeyFingerprint, err)
	}

	password, run, err := backups.InitRepository(ctx, backupAdmin, "req-init")
	if err != nil || password == "" {
		t.Fatal(err)
	}
	if done := finishedRun(t, store, run.ID); done.State != domain.BackupSucceeded {
		t.Fatalf("init = %+v", done)
	}
	executor.mu.Lock()
	sshKey, job := executor.sshKeys[0], executor.jobs[0]
	executor.mu.Unlock()
	if !strings.HasPrefix(sshKey, "-----BEGIN OPENSSH PRIVATE KEY-----") || job.Repository.HostKey != stored {
		t.Fatalf("executor got %.40q, %+v", sshKey, job.Repository)
	}

	// A new host key of the same server keeps the initialisation.
	_, newHost := serverHostKey(t)
	view, err = backups.UpdateConfig(ctx, backupAdmin, sftpUpdate(newHost), "r")
	if err != nil || !view.Initialized || view.Repository.HostKey != newHost {
		t.Fatalf("new host key = %+v, %v", view, err)
	}

	// A new SSH key replaces the old one.
	previous := view.SSHKey.PublicKey
	view, err = backups.GenerateSSHKey(ctx, backupAdmin, "req-key-2")
	if err != nil || view.SSHKey.PublicKey == previous {
		t.Fatalf("regenerate = %+v, %v", view.SSHKey, err)
	}
}

func TestSFTPCopyRepositoryGetsTheKeyForTheCopyOnly(t *testing.T) {
	store, executor := newMemoryBackupStore(), &fakeBackupExecutor{}
	backups := newSFTPBackups(t, store, executor)
	ctx := context.Background()
	readyLocalConfig(t, backups)
	pasted, stored := serverHostKey(t)
	copyUpdate := BackupCopyUpdate{
		Repository: domain.BackupRepository{Type: domain.RepositorySFTP, Location: "sftp://copy@nas.lan:2222/srv/copy", HostKey: pasted},
	}
	if _, err := backups.SetCopy(ctx, backupAdmin, copyUpdate, "r"); !errors.Is(err, domain.ErrBackupSSHKeyMissing) {
		t.Fatalf("SFTP copy without SSH key = %v", err)
	}
	if _, err := backups.GenerateSSHKey(ctx, backupAdmin, "r"); err != nil {
		t.Fatal(err)
	}
	bad := copyUpdate
	bad.Repository.HostKey = ""
	var validation *domain.BackupValidationError
	if _, err := backups.SetCopy(ctx, backupAdmin, bad, "r"); !errors.As(err, &validation) || validation.Field != "copy.hostKey" {
		t.Fatalf("copy without host key = %v", err)
	}
	view, err := backups.SetCopy(ctx, backupAdmin, copyUpdate, "r")
	if err != nil || view.Copy.Repository.HostKey != stored || view.Copy.HostKeyFingerprint == "" {
		t.Fatalf("copy = %+v, %v", view.Copy, err)
	}

	if _, run, err := backups.InitRepository(ctx, backupAdmin, "r"); err != nil {
		t.Fatal(err)
	} else {
		finishedRun(t, store, run.ID)
	}
	_, run, err := backups.InitCopy(ctx, backupAdmin, "r")
	if err != nil {
		t.Fatal(err)
	}
	finishedRun(t, store, run.ID)
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.jobs[0].Operation != domain.BackupOpInit || executor.sshKeys[0] != "" {
		t.Fatal("the init of a local repository got the SSH key")
	}
	if executor.jobs[1].Operation != domain.BackupOpCopy || !strings.HasPrefix(executor.sshKeys[1], "-----BEGIN OPENSSH") ||
		executor.jobs[1].Copy.HostKey != stored {
		t.Fatalf("copy job = %+v", executor.jobs[1])
	}
}
