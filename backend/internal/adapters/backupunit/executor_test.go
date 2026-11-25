package backupunit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/google/uuid"
)

// fakeUnits plays the runner: it checks the job and the secrets, then
// writes the result it was given.
type fakeUnits struct {
	t        *testing.T
	config   Config
	result   *backupjob.Result
	err      error
	units    []string
	sawJob   backupjob.Job
	sawCreds backupjob.Secrets
}

func (f *fakeUnits) Act(ctx context.Context, name string, verb domain.UnitVerb) (domain.UnitStableState, error) {
	f.units = append(f.units, string(verb)+" "+name)
	op := domain.BackupOperation(name[len("homelab-backup@") : len(name)-len(".service")])
	if err := backupjob.ReadStrict(backupjob.JobPath(f.config.JobsDir, op), backupjob.MaxJobBytes, &f.sawJob); err != nil {
		f.t.Fatalf("job: %v", err)
	}
	if err := backupjob.ReadStrict(backupjob.SecretsPath(f.config.JobsDir, op), backupjob.MaxSecretsBytes, &f.sawCreds); err != nil {
		f.t.Fatalf("secrets: %v", err)
	}
	if info, _ := os.Stat(backupjob.SecretsPath(f.config.JobsDir, op)); info.Mode().Perm() != 0o600 {
		f.t.Fatalf("secrets mode = %v", info.Mode())
	}
	if f.result != nil {
		if err := backupjob.WriteAtomic(backupjob.ResultPath(f.config.RunsDir, op), f.result, 0o640); err != nil {
			f.t.Fatal(err)
		}
	}
	return domain.UnitStableState{ActiveState: "inactive"}, f.err
}

func newExecutor(t *testing.T, units *fakeUnits) *Executor {
	t.Helper()
	root := t.TempDir()
	config := Config{JobsDir: filepath.Join(root, "jobs"), RunsDir: filepath.Join(root, "runs"), InitTimeout: time.Minute, BackupTimeout: time.Minute}
	for _, dir := range []string{config.JobsDir, config.RunsDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	units.t, units.config = t, config
	executor := NewExecutor(config, units)
	executor.now = func() time.Time { return time.Date(2026, 11, 24, 3, 0, 0, 500, time.UTC) }
	return executor
}

var job = ports.BackupJob{
	Operation:  domain.BackupOpBackup,
	Repository: domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"},
	Selection:  domain.BackupSelection{Sources: []string{"/etc"}, Excludes: []string{"*.tmp"}, ExcludeCaches: true},
}

func failureOf(err error) domain.BackupFailure {
	var runErr *domain.BackupRunError
	if errors.As(err, &runErr) {
		return runErr.Failure
	}
	return ""
}

func TestExecuteWritesTheJobStartsTheUnitAndReadsTheResult(t *testing.T) {
	units := &fakeUnits{result: &backupjob.Result{
		Version: 1, Operation: domain.BackupOpBackup, StartedAt: time.Date(2026, 11, 24, 3, 0, 1, 0, time.UTC), Success: true,
		SnapshotID: "abc", Summary: &backupjob.Summary{FilesNew: 2, DataAdded: 10},
		Snapshots: []backupjob.Snapshot{{ID: "abc", ShortID: "abc", Paths: []string{"/etc"}}},
	}}
	executor := newExecutor(t, units)
	result, err := executor.Execute(context.Background(), job, domain.BackupCredentials{Password: "p"}, nil)
	if err != nil || result.SnapshotID != "abc" || result.Stats.FilesNew != 2 || len(result.Snapshots) != 1 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if len(units.units) != 1 || units.units[0] != "start homelab-backup@backup.service" {
		t.Fatalf("units = %v", units.units)
	}
	if units.sawJob.Repository != "/mnt/backup" || units.sawJob.Sources[0] != "/etc" || units.sawCreds.Password != "p" {
		t.Fatalf("job = %+v, creds = %+v", units.sawJob, units.sawCreds)
	}
	if _, err := os.Stat(backupjob.SecretsPath(executor.config.JobsDir, domain.BackupOpBackup)); !os.IsNotExist(err) {
		t.Fatal("the secrets file must not outlive the run")
	}
}

func TestExecuteReportsEachFailure(t *testing.T) {
	fresh := time.Date(2026, 11, 24, 3, 0, 1, 0, time.UTC)
	cases := map[string]struct {
		units *fakeUnits
		want  domain.BackupFailure
	}{
		"polkit":    {&fakeUnits{err: &domain.UnitActionError{Failure: domain.ActionNotAllowed}}, domain.BackupFailureNotAllowed},
		"no unit":   {&fakeUnits{err: &domain.UnitActionError{Failure: domain.ActionNotFound}}, domain.BackupFailureRunner},
		"timeout":   {&fakeUnits{err: &domain.UnitActionError{Failure: domain.ActionTimeout}}, domain.BackupFailureTimeout},
		"no bus":    {&fakeUnits{err: &domain.UnitActionError{Failure: domain.ActionUnavailable}}, domain.BackupFailureUnavailable},
		"no result": {&fakeUnits{}, domain.BackupFailureResult},
		"stale result": {&fakeUnits{result: &backupjob.Result{
			Version: 1, Operation: domain.BackupOpBackup, StartedAt: time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC), Success: true,
		}}, domain.BackupFailureResult},
		"restic failed": {&fakeUnits{
			err:    &domain.UnitActionError{Failure: domain.ActionUnitFailed},
			result: &backupjob.Result{Version: 1, Operation: domain.BackupOpBackup, StartedAt: fresh, Message: "Fatal: wrong password"},
		}, domain.BackupFailureRestic},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			executor := newExecutor(t, tc.units)
			_, err := executor.Execute(context.Background(), job, domain.BackupCredentials{Password: "p"}, nil)
			if failureOf(err) != tc.want {
				t.Fatalf("failure = %q (%v), want %q", failureOf(err), err, tc.want)
			}
			var runErr *domain.BackupRunError
			if name == "restic failed" && (!errors.As(err, &runErr) || runErr.Message != "Fatal: wrong password") {
				t.Fatalf("message = %v", err)
			}
		})
	}
}

func TestExecuteRefusesAnInvalidJobBeforeWritingIt(t *testing.T) {
	units := &fakeUnits{}
	executor := newExecutor(t, units)
	bad := job
	bad.Selection.Sources = []string{"relative"}
	if _, err := executor.Execute(context.Background(), bad, domain.BackupCredentials{Password: "p"}, nil); failureOf(err) != domain.BackupFailureJob {
		t.Fatalf("err = %v", err)
	}
	if len(units.units) != 0 {
		t.Fatal("the unit started for an invalid job")
	}
}

func TestExecuteWritesTheJobOfEachOperation(t *testing.T) {
	fresh := time.Date(2026, 11, 24, 3, 0, 1, 0, time.UTC)
	runID := uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
	cases := map[domain.BackupOperation]ports.BackupJob{
		domain.BackupOpForgetPrune: {Retention: domain.BackupRetention{Enabled: true, KeepDaily: 7}},
		domain.BackupOpCheck:       {CheckSubsetPercent: 5},
		domain.BackupOpRestore:     {RunID: runID, Restore: &domain.BackupRestore{SnapshotID: "3a253ad8", DryRun: true}},
		domain.BackupOpSnapshots:   {},
		domain.BackupOpUnlock:      {},
	}
	for op, job := range cases {
		t.Run(string(op), func(t *testing.T) {
			units := &fakeUnits{result: &backupjob.Result{
				Version: 1, Operation: op, StartedAt: fresh, Success: true, Details: &domain.BackupRunDetails{SubsetPercent: 5},
			}}
			executor := newExecutor(t, units)
			job.Operation = op
			job.Repository = domain.BackupRepository{Type: domain.RepositoryLocal, Location: "/mnt/backup"}
			job.Selection = domain.BackupSelection{Sources: []string{"/etc"}}
			result, err := executor.Execute(context.Background(), job, domain.BackupCredentials{Password: "p"}, nil)
			if err != nil || result.Details == nil {
				t.Fatalf("result = %+v, %v", result, err)
			}
			if len(units.sawJob.Sources) != 0 {
				t.Fatal("only a backup carries the sources")
			}
			switch op {
			case domain.BackupOpForgetPrune:
				if units.sawJob.Retention == nil || units.sawJob.Retention.KeepDaily != 7 {
					t.Fatalf("retention = %+v", units.sawJob.Retention)
				}
			case domain.BackupOpRestore:
				if units.sawJob.Restore == nil || units.sawJob.Restore.Target != domain.RestoreTarget(runID) || !units.sawJob.Restore.DryRun {
					t.Fatalf("restore = %+v", units.sawJob.Restore)
				}
			}
		})
	}
}

func TestExecuteWritesADiffAndACopy(t *testing.T) {
	started := time.Date(2026, 11, 24, 3, 0, 1, 0, time.UTC)
	diff := &domain.BackupDiff{From: "3a253ad8", To: "9f2c11b0", Changes: []domain.BackupDiffChange{{Path: "/etc/hosts", Modifier: "M"}}}
	units := &fakeUnits{result: &backupjob.Result{
		Version: 1, Operation: domain.BackupOpDiff, StartedAt: started, Success: true,
		Details: &domain.BackupRunDetails{Diff: diff},
	}}
	executor := newExecutor(t, units)
	diffJob := ports.BackupJob{
		Operation: domain.BackupOpDiff, Repository: job.Repository,
		Diff: &domain.BackupDiffRequest{From: "3a253ad8", To: "9f2c11b0"},
	}
	result, err := executor.Execute(context.Background(), diffJob, domain.BackupCredentials{Password: "p"}, nil)
	if err != nil || result.Details == nil || result.Details.Diff.Changes[0].Path != "/etc/hosts" {
		t.Fatalf("diff = %+v, %v", result, err)
	}
	if units.sawJob.Diff == nil || units.sawJob.Diff.From != "3a253ad8" || len(units.sawJob.Sources) != 0 || units.sawCreds.Copy != nil {
		t.Fatalf("diff job = %+v", units.sawJob)
	}

	units.result = &backupjob.Result{
		Version: 1, Operation: domain.BackupOpCopy, StartedAt: started, Success: true,
		Details: &domain.BackupRunDetails{Copy: &domain.BackupCopyResult{SnapshotsCopied: 2}},
	}
	copyJob := ports.BackupJob{
		Operation: domain.BackupOpCopy, Repository: job.Repository, CopyInit: true,
		Copy:          &domain.BackupRepository{Type: domain.RepositoryRest, Location: "https://nas.lan:8000/copy"},
		CopyRetention: &domain.BackupRetention{Enabled: true, KeepDaily: 7},
	}
	copyCredentials := &domain.BackupCredentials{Password: "copy-pass", Username: "u", Secret: "s"}
	result, err = executor.Execute(context.Background(), copyJob, domain.BackupCredentials{Password: "p"}, copyCredentials)
	if err != nil || result.Details.Copy.SnapshotsCopied != 2 {
		t.Fatalf("copy = %+v, %v", result, err)
	}
	if c := units.sawJob.Copy; c == nil || !c.Init || c.Repository != "https://nas.lan:8000/copy" || c.Retention == nil || c.Retention.KeepDaily != 7 {
		t.Fatalf("copy job = %+v", units.sawJob.Copy)
	}
	if units.sawCreds.Password != "p" || units.sawCreds.Copy == nil || units.sawCreds.Copy.Password != "copy-pass" {
		t.Fatalf("copy secrets = %+v", units.sawCreds)
	}
	if _, err := executor.Execute(context.Background(), copyJob, domain.BackupCredentials{Password: "p"}, nil); failureOf(err) != domain.BackupFailureJob {
		t.Fatalf("copy without its credentials = %v", err)
	}
	copyJob.Copy = &job.Repository
	if _, err := executor.Execute(context.Background(), copyJob, domain.BackupCredentials{Password: "p"}, copyCredentials); failureOf(err) != domain.BackupFailureJob {
		t.Fatalf("copy to the same repository = %v", err)
	}
}

// The polkit rule shipped with the project allows exactly the operations
// of the Manager.
func TestThePolkitRuleListsEveryOperation(t *testing.T) {
	rule, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "deployments", "polkit", "50-homelab-manager-backup.rules"))
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, match := range regexp.MustCompile(`"(homelab-backup@[a-z-]+\.service)"`).FindAllStringSubmatch(string(rule), -1) {
		listed = append(listed, match[1])
	}
	var want []string
	for _, op := range domain.BackupOperations {
		want = append(want, op.BackupUnit())
	}
	slices.Sort(listed)
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Fatalf("polkit rule lists %v, want %v", listed, want)
	}
}
