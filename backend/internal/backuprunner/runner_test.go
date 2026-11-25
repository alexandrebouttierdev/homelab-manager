package backuprunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"
)

// fakeRestic records its arguments, environment and exclusion file, then
// prints <command>.out and <command>.err and exits with <command>.code.
// The command is the first argument after the global "-o" options; the
// files that sftp.args names are copied with their mode.
const fakeRestic = `#!/bin/sh
dir=$(dirname "$0")
cmd=""
skip=0
for arg in "$@"; do
  if [ "$skip" = 1 ]; then
    skip=0
    case "$arg" in
      sftp.args=*)
        identity=$(printf '%s' "$arg" | sed 's/.* -i \([^ ]*\) .*/\1/')
        hosts=$(printf '%s' "$arg" | sed 's/.*UserKnownHostsFile=\([^ ]*\).*/\1/')
        cp "$identity" "$dir/ssh.identity"
        cp "$hosts" "$dir/ssh.known_hosts"
        ls -l "$identity" "$hosts" | cut -c1-10 > "$dir/ssh.modes"
        ;;
    esac
    continue
  fi
  if [ -z "$cmd" ] && [ "$arg" = "-o" ]; then skip=1; continue; fi
  [ -z "$cmd" ] && cmd="$arg"
done
printf '%s\n' "$@" > "$dir/args.$cmd"
env | sort > "$dir/env.$cmd"
previous=""
for arg in "$@"; do
  if [ "$previous" = "--exclude-file" ]; then cp "$arg" "$dir/excludes"; fi
  previous="$arg"
done
[ -f "$dir/$cmd.out" ] && cat "$dir/$cmd.out"
[ -f "$dir/$cmd.err" ] && cat "$dir/$cmd.err" >&2
code=0
[ -f "$dir/$cmd.code" ] && code=$(cat "$dir/$cmd.code")
exit "$code"
`

type fixture struct {
	t      *testing.T
	config Config
	bin    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, bin: filepath.Join(root, "bin")}
	for _, dir := range []string{"jobs", "runs", "cache", "tmp", "bin"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	restic := filepath.Join(f.bin, "restic")
	if err := os.WriteFile(restic, []byte(fakeRestic), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 24, 3, 0, 0, 0, time.UTC)
	f.config = Config{
		JobsDir: filepath.Join(root, "jobs"), RunsDir: filepath.Join(root, "runs"), CacheDir: filepath.Join(root, "cache"),
		TempDir: filepath.Join(root, "tmp"), Restic: restic, Path: "/usr/bin:/bin", RestoreRoot: filepath.Join(root, "restore"),
		Now: func() time.Time { now = now.Add(time.Second); return now },
	}
	return f
}

func (f *fixture) file(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(name string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.bin, name))
	if err != nil {
		f.t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func (f *fixture) job(job backupjob.Job, credentials domain.BackupCredentials) {
	f.t.Helper()
	if err := backupjob.WriteAtomic(backupjob.JobPath(f.config.JobsDir, job.Operation), job, 0o600); err != nil {
		f.t.Fatal(err)
	}
	if err := backupjob.WriteAtomic(backupjob.SecretsPath(f.config.JobsDir, job.Operation), credentials, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) run(operation string) (int, backupjob.Result, string) {
	f.t.Helper()
	var stderr bytes.Buffer
	status := Run(context.Background(), f.config, operation, &stderr)
	var result backupjob.Result
	if status != 2 {
		if err := backupjob.ReadStrict(backupjob.ResultPath(f.config.RunsDir, domain.BackupOperation(operation)), backupjob.MaxResultBytes, &result); err != nil {
			f.t.Fatalf("result: %v", err)
		}
	}
	return status, result, stderr.String()
}

func backupJob() backupjob.Job {
	return backupjob.Job{
		Version: domain.BackupJobVersion, Operation: domain.BackupOpBackup,
		RepositoryType: domain.RepositoryS3, Repository: "https://s3.example.com/bucket/homelab",
		Sources:  []string{"/etc", "/srv/data"},
		Excludes: []string{"*.tmp", "**/node_modules"}, ExcludeCaches: true, ExcludeIfPresent: ".nobackup",
	}
}

const summaryLine = `{"message_type":"summary","files_new":3,"files_changed":1,"files_unmodified":10,"dirs_new":1,"dirs_changed":0,"dirs_unmodified":4,"data_added":2048,"total_files_processed":14,"total_bytes_processed":65536,"total_duration":1.5,"snapshot_id":"abcdef0123456789"}`

const snapshotsJSON = `[{"time":"2026-11-23T03:00:00Z","paths":["/etc"],"hostname":"homelab","tags":["homelab-manager"],"id":"old","short_id":"old"},` +
	`{"time":"2026-11-24T03:00:01Z","paths":["/etc","/srv/data"],"hostname":"homelab","tags":["homelab-manager"],"id":"abcdef0123456789","short_id":"abcdef01","summary":{"total_files_processed":14,"total_bytes_processed":65536,"data_added":2048}}]`

func TestBackupBuildsFixedArgumentsAndReportsTheSummary(t *testing.T) {
	f := newFixture(t)
	f.file("backup.out", `{"message_type":"status","percent_done":0.5}`+"\nnot json\n"+summaryLine+"\n")
	f.file("snapshots.out", snapshotsJSON)
	f.job(backupJob(), domain.BackupCredentials{Password: "repo-pass", Username: "key-id", Secret: "key-secret"})

	status, result, stderr := f.run("backup")
	if status != 0 || !result.Success || result.Warning {
		t.Fatalf("status %d, result %+v, stderr %s", status, result, stderr)
	}
	wantArgs := []string{
		"backup", "--json", "--tag", "homelab-manager", "--exclude-caches", "--exclude-if-present", ".nobackup",
		"--exclude-file", "", "--", "/etc", "/srv/data",
	}
	args := strings.Split(strings.TrimSpace(f.read("args.backup")), "\n")
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %q", args)
	}
	for i, want := range wantArgs {
		if want != "" && args[i] != want {
			t.Fatalf("arg %d = %q, want %q (all %q)", i, args[i], want, args)
		}
	}
	if f.read("excludes") != "*.tmp\n**/node_modules\n" {
		t.Fatalf("exclusion file = %q", f.read("excludes"))
	}
	env := f.read("env.backup")
	for _, want := range []string{
		"RESTIC_REPOSITORY=s3:https://s3.example.com/bucket/homelab", "RESTIC_PASSWORD=repo-pass",
		"AWS_ACCESS_KEY_ID=key-id", "AWS_SECRET_ACCESS_KEY=key-secret", "RESTIC_CACHE_DIR=" + f.config.CacheDir,
	} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("environment lacks %s", want)
		}
	}
	if strings.Contains(strings.Join(args, " "), "repo-pass") || strings.Contains(f.read("args.snapshots"), "repo-pass") {
		t.Fatal("a secret is in the arguments")
	}
	if _, err := os.Stat(backupjob.SecretsPath(f.config.JobsDir, domain.BackupOpBackup)); !os.IsNotExist(err) {
		t.Fatalf("the secrets file must be removed: %v", err)
	}
	if result.SnapshotID != "abcdef0123456789" || result.Summary == nil || result.Summary.FilesNew != 3 || result.Summary.DataAdded != 2048 {
		t.Fatalf("summary = %+v", result)
	}
	if len(result.Snapshots) != 2 || result.Snapshots[0].ID != "abcdef0123456789" || result.Snapshots[0].TotalFiles != 14 {
		t.Fatalf("snapshots = %+v (newest first)", result.Snapshots)
	}
	if strings.TrimSpace(f.read("args.snapshots")) != "snapshots\n--json\n--tag\nhomelab-manager" {
		t.Fatalf("snapshots args = %q", f.read("args.snapshots"))
	}
}

func TestBackupWithUnreadableFilesIsAWarning(t *testing.T) {
	f := newFixture(t)
	f.file("backup.out", summaryLine+"\n")
	f.file("backup.err", `{"message_type":"error","error":{"message":"permission denied"},"during":"archival","item":"/srv/data/secret"}`+"\n")
	f.file("backup.code", "3")
	f.file("snapshots.out", "[]")
	job := backupJob()
	job.RepositoryType, job.Repository = domain.RepositoryLocal, "/mnt/backup"
	f.job(job, domain.BackupCredentials{Password: "p"})
	status, result, _ := f.run("backup")
	if status != 0 || !result.Success || !result.Warning || !strings.Contains(result.Message, "/srv/data/secret: permission denied") {
		t.Fatalf("status %d, result %+v", status, result)
	}
	if strings.Contains(f.read("env.backup"), "AWS_") {
		t.Fatal("a local repository gets no S3 credentials")
	}
}

func TestBackupFailureKeepsResticMessage(t *testing.T) {
	f := newFixture(t)
	f.file("backup.err", "repository is already locked\nFatal: wrong password or no key found\n")
	f.file("backup.code", "12")
	f.job(backupJob(), domain.BackupCredentials{Password: "p", Username: "k", Secret: "s"})
	status, result, _ := f.run("backup")
	if status != 1 || result.Success || result.Message != "Fatal: wrong password or no key found" {
		t.Fatalf("status %d, result %+v", status, result)
	}
	f2 := newFixture(t)
	f2.file("backup.err", `{"message_type":"exit_error","code":10,"message":"repository does not exist"}`+"\n")
	f2.file("backup.code", "10")
	f2.job(backupJob(), domain.BackupCredentials{Password: "p", Username: "k", Secret: "s"})
	if _, result, _ := f2.run("backup"); result.Message != "repository does not exist" {
		t.Fatalf("exit error = %+v", result)
	}
}

func TestInit(t *testing.T) {
	f := newFixture(t)
	f.file("init.out", `{"message_type":"initialized","id":"r1","repository":"/mnt/backup"}`)
	job := backupjob.Job{Version: 1, Operation: domain.BackupOpInit, RepositoryType: domain.RepositoryRest, Repository: "https://backup.lan/homelab"}
	f.job(job, domain.BackupCredentials{Password: "p", Username: "u", Secret: "s"})
	status, result, stderr := f.run("init")
	if status != 0 || !result.Success {
		t.Fatalf("status %d result %+v %s", status, result, stderr)
	}
	env := f.read("env.init")
	if !strings.Contains(env, "RESTIC_REPOSITORY=rest:https://backup.lan/homelab\n") || !strings.Contains(env, "RESTIC_REST_USERNAME=u\n") {
		t.Fatalf("env = %s", env)
	}
	if strings.TrimSpace(f.read("args.init")) != "init\n--json" {
		t.Fatalf("args = %q", f.read("args.init"))
	}
}

func TestRunnerRefusesAnInvalidJob(t *testing.T) {
	cases := map[string]func(*backupjob.Job){
		"option as source":   func(j *backupjob.Job) { j.Sources = []string{"--password-command=/bin/sh"} },
		"relative source":    func(j *backupjob.Job) { j.Sources = []string{"etc"} },
		"no source":          func(j *backupjob.Job) { j.Sources = nil },
		"plain http":         func(j *backupjob.Job) { j.Repository = "http://s3.example.com/bucket" },
		"other operation":    func(j *backupjob.Job) { j.Operation = domain.BackupOpInit },
		"unsupported format": func(j *backupjob.Job) { j.Version = 2 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			job := backupJob()
			change(&job)
			// The file name follows the unit instance, whatever the job says.
			if err := backupjob.WriteAtomic(backupjob.JobPath(f.config.JobsDir, domain.BackupOpBackup), job, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := backupjob.WriteAtomic(backupjob.SecretsPath(f.config.JobsDir, domain.BackupOpBackup), domain.BackupCredentials{Password: "p", Username: "k", Secret: "s"}, 0o600); err != nil {
				t.Fatal(err)
			}
			status, result, _ := f.run("backup")
			if status != 1 || result.Success {
				t.Fatalf("status %d, result %+v", status, result)
			}
			if _, err := os.Stat(filepath.Join(f.bin, "args.backup")); !os.IsNotExist(err) {
				t.Fatal("restic ran for an invalid job")
			}
		})
	}
}

func TestRunnerRefusesUnknownFieldsAndOperations(t *testing.T) {
	f := newFixture(t)
	path := backupjob.JobPath(f.config.JobsDir, domain.BackupOpBackup)
	raw, _ := json.Marshal(backupJob())
	raw = append(raw[:len(raw)-1], []byte(`,"passwordCommand":"/bin/sh"}`)...)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if status, result, _ := f.run("backup"); status != 1 || !strings.Contains(result.Message, "unknown field") {
		t.Fatalf("unknown field: status %d, %+v", status, result)
	}
	if status, _, _ := f.run("mount"); status != 2 {
		t.Fatalf("unknown operation = %d, want 2", status)
	}
	if status, result, _ := f.run("init"); status != 1 || result.Success {
		t.Fatalf("missing job: status %d, %+v", status, result)
	}
}

// With HOMELAB_TEST_RESTIC=1 and restic installed, a real repository is
// initialised and backed up in a temporary directory.
func TestRunnerWithRealRestic(t *testing.T) {
	restic, err := exec.LookPath("restic")
	if err != nil || os.Getenv("HOMELAB_TEST_RESTIC") != "1" {
		t.Skip("set HOMELAB_TEST_RESTIC=1 with restic installed")
	}
	f := newFixture(t)
	f.config.Restic = restic
	root := filepath.Dir(f.config.JobsDir)
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "skip"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"a.txt": "hello", "b.tmp": "temporary", "skip/.nobackup": ""} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository := filepath.Join(root, "repository")
	credentials := domain.BackupCredentials{Password: "a-test-password"}
	f.job(backupjob.Job{Version: 1, Operation: domain.BackupOpInit, RepositoryType: domain.RepositoryLocal, Repository: repository}, credentials)
	if status, result, stderr := f.run("init"); status != 0 {
		t.Fatalf("init: %d %+v %s", status, result, stderr)
	}
	f.job(backupjob.Job{
		Version: 1, Operation: domain.BackupOpBackup, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{source}, Excludes: []string{"*.tmp"}, ExcludeIfPresent: ".nobackup",
	}, credentials)
	status, result, stderr := f.run("backup")
	// "hello" alone: b.tmp is excluded, skip/ holds only its empty marker.
	if status != 0 || !result.Success || result.Summary == nil || result.Summary.TotalBytesProcessed != 5 || len(result.Snapshots) != 1 {
		t.Fatalf("backup: %d %+v %s", status, result, stderr)
	}
	first := result.SnapshotID
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("hello again"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "c.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.job(backupjob.Job{
		Version: 1, Operation: domain.BackupOpBackup, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{source}, Excludes: []string{"*.tmp"}, ExcludeIfPresent: ".nobackup",
	}, credentials)
	status, result, stderr = f.run("backup")
	if status != 0 || result.SnapshotID == "" {
		t.Fatalf("second backup: %d %+v %s", status, result, stderr)
	}

	diffJob := backupjob.Job{
		Version: 1, Operation: domain.BackupOpDiff, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{}, Excludes: []string{}, Diff: &backupjob.Diff{From: first, To: result.SnapshotID},
	}
	f.job(diffJob, credentials)
	status, result, stderr = f.run("diff")
	if status != 0 || result.Details == nil || result.Details.Diff == nil {
		t.Fatalf("diff: %d %+v %s", status, result, stderr)
	}
	changes := map[string]string{}
	for _, change := range result.Details.Diff.Changes {
		changes[strings.TrimPrefix(change.Path, source)] = change.Modifier
	}
	if changes["/a.txt"] != "M" || changes["/c.txt"] != "+" || result.Details.Diff.Added.Files != 1 {
		t.Fatalf("diff = %+v (%v)", result.Details.Diff, changes)
	}

	copyRepository := filepath.Join(root, "copy")
	copyJob := backupjob.Job{
		Version: 1, Operation: domain.BackupOpCopy, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{}, Excludes: []string{},
		Copy: &backupjob.Copy{RepositoryType: domain.RepositoryLocal, Repository: copyRepository, Init: true},
	}
	copySecrets := backupjob.Secrets{BackupCredentials: credentials, Copy: &domain.BackupCredentials{Password: "another-password"}}
	f.job(copyJob, domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	status, result, stderr = f.run("copy")
	if status != 0 || result.Warning || result.Details.Copy.SnapshotsCopied != 2 || !result.Details.Copy.Initialized {
		t.Fatalf("first copy: %d %+v %s", status, result.Details, stderr)
	}
	copyJob.Copy.Init, copyJob.Copy.Retention = false, &backupjob.Retention{KeepDaily: 1}
	f.job(copyJob, domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	status, result, stderr = f.run("copy")
	// Both snapshots were made the same day: the retention keeps one.
	if status != 0 || result.Details.Copy.SnapshotsCopied != 0 || result.Details.Copy.SnapshotsRemoved != 1 ||
		result.Details.Copy.SnapshotsInTarget != 1 {
		t.Fatalf("second copy: %d %+v %s", status, result.Details.Copy, stderr)
	}
	copySecrets.Copy.Password = "wrong-password"
	f.job(copyJob, domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	if status, result, _ := f.run("copy"); status != 1 || result.Message == "" || strings.Contains(result.Message, "wrong-password") {
		t.Fatalf("copy with a wrong password: %d %+v", status, result)
	}

	f.job(backupjob.Job{
		Version: 1, Operation: domain.BackupOpBackup, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{source},
	}, domain.BackupCredentials{Password: "wrong-password"})
	if status, result, _ := f.run("backup"); status != 1 || result.Message == "" {
		t.Fatalf("wrong password: %d %+v", status, result)
	}
}

func repositoryJob(op domain.BackupOperation) backupjob.Job {
	return backupjob.Job{Version: 1, Operation: op, RepositoryType: domain.RepositoryLocal, Repository: "/mnt/backup", Sources: []string{}, Excludes: []string{}}
}

func TestMaintenanceOperations(t *testing.T) {
	f := newFixture(t)
	f.file("snapshots.out", snapshotsJSON)
	f.job(repositoryJob(domain.BackupOpSnapshots), domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("snapshots"); status != 0 || len(result.Snapshots) != 2 {
		t.Fatalf("snapshots: %d %+v", status, result)
	}

	f.job(repositoryJob(domain.BackupOpUnlock), domain.BackupCredentials{Password: "p"})
	if status, _, _ := f.run("unlock"); status != 0 || strings.TrimSpace(f.read("args.unlock")) != "unlock\n--remove-all" {
		t.Fatalf("unlock: %d %q", status, f.read("args.unlock"))
	}

	check := repositoryJob(domain.BackupOpCheck)
	check.CheckSubsetPercent = 5
	f.job(check, domain.BackupCredentials{Password: "p"})
	status, result, _ := f.run("check")
	if status != 0 || result.Details.SubsetPercent != 5 || strings.TrimSpace(f.read("args.check")) != "check\n--read-data-subset\n5%" {
		t.Fatalf("check: %d %+v %q", status, result, f.read("args.check"))
	}
	f.file("check.err", "Fatal: repository contains errors\n")
	f.file("check.code", "1")
	f.job(check, domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("check"); status != 1 || result.Message != "Fatal: repository contains errors" {
		t.Fatalf("failed check: %d %+v", status, result)
	}

	forget := repositoryJob(domain.BackupOpForgetPrune)
	forget.Retention = &backupjob.Retention{KeepDaily: 7, KeepMonthly: 6}
	f.file("forget.out", `[{"tags":["homelab-manager"],"host":"h","paths":["/etc"],"keep":[{"id":"a"}],"remove":[{"id":"b"},{"id":"c"}]}]`+"\nrepacking packs\n")
	f.job(forget, domain.BackupCredentials{Password: "p"})
	status, result, _ = f.run("forget-prune")
	if status != 0 || result.Details.SnapshotsRemoved != 2 || len(result.Snapshots) != 2 {
		t.Fatalf("forget: %d %+v", status, result)
	}
	if got := strings.TrimSpace(f.read("args.forget")); got != "forget\n--json\n--tag\nhomelab-manager\n--keep-daily\n7\n--keep-monthly\n6\n--prune" {
		t.Fatalf("forget args = %q", got)
	}
}

func restoreJob(dryRun bool) backupjob.Job {
	job := repositoryJob(domain.BackupOpRestore)
	job.Restore = &backupjob.Restore{
		SnapshotID: "3a253ad8", Include: []string{"/etc/hostname"}, DryRun: dryRun,
		Target: domain.BackupRestoreRoot + "/7c9e6679-7425-40de-944b-e07fc1f90ae7",
	}
	return job
}

func TestRestoreIntoTheQuarantine(t *testing.T) {
	f := newFixture(t)
	f.file("restore.out", `{"message_type":"summary","total_files":3,"files_restored":3,"total_bytes":42,"bytes_restored":42}`+"\n")
	f.job(restoreJob(false), domain.BackupCredentials{Password: "p"})
	status, result, stderr := f.run("restore")
	if status != 0 || result.Details.FilesRestored != 3 || result.Details.BytesRestored != 42 || result.Details.SnapshotID != "3a253ad8" {
		t.Fatalf("restore: %d %+v %s", status, result, stderr)
	}
	target := filepath.Join(f.config.RestoreRoot, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("quarantine = %v %v", info, err)
	}
	want := "restore\n--json\n--target\n" + target + "\n--include\n/etc/hostname\n--\n3a253ad8"
	if got := strings.TrimSpace(f.read("args.restore")); got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	// The same run directory is never written twice.
	f.job(restoreJob(false), domain.BackupCredentials{Password: "p"})
	if status, _, _ := f.run("restore"); status != 1 {
		t.Fatal("a second restore into the same directory must fail")
	}

	g := newFixture(t)
	g.file("restore.out", `{"message_type":"summary","total_files":3,"files_restored":0,"total_bytes":42,"bytes_restored":0}`+"\n")
	g.job(restoreJob(true), domain.BackupCredentials{Password: "p"})
	if status, result, _ := g.run("restore"); status != 0 || !result.Details.DryRun || result.Details.TotalFiles != 3 {
		t.Fatalf("dry run: %d %+v", status, result)
	}
	if !strings.Contains(g.read("args.restore"), "--dry-run") {
		t.Fatal("dry run flag missing")
	}
	if _, err := os.Stat(filepath.Join(g.config.RestoreRoot, "7c9e6679-7425-40de-944b-e07fc1f90ae7")); !os.IsNotExist(err) {
		t.Fatal("a dry run leaves no directory")
	}
}

func TestRestoreRefusesATargetOutsideTheQuarantine(t *testing.T) {
	for name, target := range map[string]string{
		"elsewhere":   "/etc",
		"not a run":   domain.BackupRestoreRoot + "/x",
		"nested":      domain.BackupRestoreRoot + "/7c9e6679-7425-40de-944b-e07fc1f90ae7/../../etc",
		"root itself": domain.BackupRestoreRoot,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			job := restoreJob(false)
			job.Restore.Target = target
			f.job(job, domain.BackupCredentials{Password: "p"})
			if status, _, _ := f.run("restore"); status != 1 {
				t.Fatalf("status %d", status)
			}
			if _, err := os.Stat(filepath.Join(f.bin, "args.restore")); !os.IsNotExist(err) {
				t.Fatal("restic ran")
			}
		})
	}
}

func (f *fixture) secrets(op domain.BackupOperation, secrets backupjob.Secrets) {
	f.t.Helper()
	if err := backupjob.WriteAtomic(backupjob.SecretsPath(f.config.JobsDir, op), secrets, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

const (
	snapshotA = "3a253ad8aaaaaaaa"
	snapshotB = "9f2c11b0bbbbbbbb"
)

func TestDiffKeepsTheFiguresAndTheFirstChanges(t *testing.T) {
	f := newFixture(t)
	var out strings.Builder
	for i := 0; i < domain.MaxBackupDiffChanges+5; i++ {
		fmt.Fprintf(&out, `{"message_type":"change","path":"/srv/file-%d","modifier":"+"}`+"\n", i)
	}
	out.WriteString(`{"message_type":"change","path":"/srv/odd","modifier":"X"}` + "\n")
	out.WriteString(`{"message_type":"statistics","source_snapshot":"a","target_snapshot":"b","changed_files":2,` +
		`"added":{"files":1005,"dirs":1,"others":0,"data_blobs":2,"tree_blobs":3,"bytes":4096},` +
		`"removed":{"files":1,"dirs":0,"others":0,"data_blobs":1,"tree_blobs":1,"bytes":12}}` + "\n")
	f.file("diff.out", out.String())
	job := repositoryJob(domain.BackupOpDiff)
	job.Diff = &backupjob.Diff{From: snapshotA, To: snapshotB}
	f.job(job, domain.BackupCredentials{Password: "p"})

	status, result, stderr := f.run("diff")
	if status != 0 || !result.Success || result.Details == nil || result.Details.Diff == nil {
		t.Fatalf("diff: %d %+v %s", status, result, stderr)
	}
	report := result.Details.Diff
	if report.From != snapshotA || report.To != snapshotB || report.ChangedFiles != 2 ||
		report.Added.Files != 1005 || report.Added.Bytes != 4096 || report.Removed.Files != 1 || report.Removed.Bytes != 12 {
		t.Fatalf("figures = %+v", report)
	}
	if report.TotalChanges != domain.MaxBackupDiffChanges+5 || !report.Truncated || len(report.Changes) != domain.MaxBackupDiffChanges ||
		report.Changes[0] != (domain.BackupDiffChange{Path: "/srv/file-0", Modifier: "+"}) {
		t.Fatalf("changes: total %d, kept %d, truncated %v", report.TotalChanges, len(report.Changes), report.Truncated)
	}
	if got := strings.TrimSpace(f.read("args.diff")); got != "diff\n--json\n--\n"+snapshotA+"\n"+snapshotB {
		t.Fatalf("args = %q", got)
	}

	f.file("diff.out", `{"message_type":"change","path":"/a","modifier":"M"}`+"\n")
	f.job(job, domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("diff"); status != 1 || result.Success {
		t.Fatalf("a diff without statistics must fail: %d %+v", status, result)
	}
	f.file("diff.out", "")
	f.file("diff.err", `{"message_type":"exit_error","code":1,"message":"Fatal: no matching ID found for prefix \"3a253ad8aaaaaaaa\""}`+"\n")
	f.file("diff.code", "1")
	f.job(job, domain.BackupCredentials{Password: "p"})
	if status, result, _ := f.run("diff"); status != 1 || !strings.Contains(result.Message, "no matching ID") {
		t.Fatalf("unknown snapshot: %d %+v", status, result)
	}
}

func TestDiffRefusesInvalidSnapshots(t *testing.T) {
	for name, request := range map[string]backupjob.Diff{
		"same":   {From: snapshotA, To: snapshotA},
		"option": {From: "--help", To: snapshotB},
		"short":  {From: "3a25", To: snapshotB},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			job := repositoryJob(domain.BackupOpDiff)
			job.Diff = &request
			f.job(job, domain.BackupCredentials{Password: "p"})
			if status, _, _ := f.run("diff"); status != 1 {
				t.Fatalf("status %d", status)
			}
			if _, err := os.Stat(filepath.Join(f.bin, "args.diff")); !os.IsNotExist(err) {
				t.Fatal("restic ran")
			}
		})
	}
}

func copyJob(init bool) backupjob.Job {
	job := repositoryJob(domain.BackupOpCopy)
	job.RepositoryType, job.Repository = domain.RepositoryRest, "https://backup.lan:8000/main"
	job.Copy = &backupjob.Copy{RepositoryType: domain.RepositoryS3, Repository: "https://s3.example.com/bucket/copy", Init: init}
	return job
}

var copySecrets = backupjob.Secrets{
	BackupCredentials: domain.BackupCredentials{Password: "main-pass", Username: "rest-user", Secret: "rest-secret"},
	Copy:              &domain.BackupCredentials{Password: "copy-pass", Username: "key-id", Secret: "key-secret"},
}

func TestCopyInitializesThenCopies(t *testing.T) {
	f := newFixture(t)
	f.file("snapshots.out", snapshotsJSON)
	f.job(copyJob(true), domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)

	status, result, stderr := f.run("copy")
	if status != 0 || !result.Success || result.Details == nil || result.Details.Copy == nil {
		t.Fatalf("copy: %d %+v %s", status, result, stderr)
	}
	report := result.Details.Copy
	if !report.Initialized || report.SnapshotsCopied != 2 || report.SnapshotsInTarget != 2 ||
		report.Repository.Location != "https://s3.example.com/bucket/copy" {
		t.Fatalf("report = %+v", report)
	}
	if got := strings.TrimSpace(f.read("args.init")); got != "init\n--copy-chunker-params\n--json" {
		t.Fatalf("init args = %q", got)
	}
	if got := strings.TrimSpace(f.read("args.copy")); got != "copy\n--tag\nhomelab-manager" {
		t.Fatalf("copy args = %q", got)
	}
	env := f.read("env.copy")
	for _, want := range []string{
		"RESTIC_REPOSITORY=s3:https://s3.example.com/bucket/copy\n", "RESTIC_PASSWORD=copy-pass\n",
		"RESTIC_FROM_REPOSITORY=rest:https://rest-user:rest-secret@backup.lan:8000/main\n", "RESTIC_FROM_PASSWORD=main-pass\n",
		"AWS_ACCESS_KEY_ID=key-id\n", "AWS_SECRET_ACCESS_KEY=key-secret\n",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("env misses %q:\n%s", want, env)
		}
	}
	if strings.Contains(env, "RESTIC_REST_") {
		t.Fatal("rest-server credentials go in the URL of their repository only")
	}
	if _, err := os.Stat(backupjob.SecretsPath(f.config.JobsDir, domain.BackupOpCopy)); !os.IsNotExist(err) {
		t.Fatal("the secrets file must be removed")
	}
}

func TestCopyAppliesTheRetentionAndHidesTheSecrets(t *testing.T) {
	f := newFixture(t)
	f.file("snapshots.out", snapshotsJSON)
	f.file("forget.out", `[{"remove":[{"id":"b"}]}]`+"\n")
	job := copyJob(false)
	job.Copy.Retention = &backupjob.Retention{KeepDaily: 3}
	f.job(job, domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	status, result, _ := f.run("copy")
	if status != 0 || result.Details.Copy.SnapshotsCopied != 0 || result.Details.Copy.SnapshotsRemoved != 1 ||
		result.Details.Copy.SnapshotsInTarget != 1 {
		t.Fatalf("copy: %d %+v", status, result.Details.Copy)
	}
	if _, err := os.Stat(filepath.Join(f.bin, "args.init")); !os.IsNotExist(err) {
		t.Fatal("init ran on a connected repository")
	}
	if got := strings.TrimSpace(f.read("args.forget")); got != "forget\n--json\n--tag\nhomelab-manager\n--keep-daily\n3\n--prune" {
		t.Fatalf("forget args = %q", got)
	}

	f.file("copy.err", "Fatal: wrong password copy-pass or key-secret\n")
	f.file("copy.code", "12")
	f.job(job, domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	status, result, stderr := f.run("copy")
	if status != 1 || strings.Contains(result.Message, "copy-pass") || strings.Contains(result.Message, "key-secret") ||
		!strings.Contains(result.Message, "wrong password ***") || strings.Contains(stderr, "copy-pass") {
		t.Fatalf("failed copy: %d %q %q", status, result.Message, stderr)
	}
}

func TestCopyRefusesInvalidSecrets(t *testing.T) {
	cases := map[string]func(*backupjob.Job, *backupjob.Secrets){
		"no copy credentials": func(_ *backupjob.Job, s *backupjob.Secrets) { s.Copy = nil },
		"two S3 keys": func(j *backupjob.Job, s *backupjob.Secrets) {
			j.RepositoryType, j.Repository = domain.RepositoryS3, "https://s3.example.com/bucket/main"
			s.BackupCredentials = domain.BackupCredentials{Password: "p", Username: "other", Secret: "other-secret"}
		},
		"same repository": func(j *backupjob.Job, _ *backupjob.Secrets) {
			j.Copy.RepositoryType, j.Copy.Repository = j.RepositoryType, j.Repository
		},
		"copy credentials elsewhere": func(j *backupjob.Job, _ *backupjob.Secrets) {
			*j = repositoryJob(domain.BackupOpCheck)
			j.Operation = domain.BackupOpCopy
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			job, secrets := copyJob(false), copySecrets
			copied := *copySecrets.Copy
			secrets.Copy = &copied
			change(&job, &secrets)
			f.job(job, domain.BackupCredentials{})
			f.secrets(domain.BackupOpCopy, secrets)
			if status, _, _ := f.run("copy"); status != 1 {
				t.Fatalf("status %d", status)
			}
			if _, err := os.Stat(filepath.Join(f.bin, "args.copy")); !os.IsNotExist(err) {
				t.Fatal("restic ran")
			}
		})
	}
}

func TestCopyFailingAfterTheInitialisationIsAWarning(t *testing.T) {
	f := newFixture(t)
	f.file("copy.err", "Fatal: connection reset\n")
	f.file("copy.code", "1")
	f.job(copyJob(true), domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	status, result, _ := f.run("copy")
	if status != 0 || !result.Success || !result.Warning || !result.Details.Copy.Initialized ||
		result.Message != "the copy failed: Fatal: connection reset" {
		t.Fatalf("copy: %d %+v", status, result)
	}
	f.job(copyJob(false), domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, copySecrets)
	if status, result, _ := f.run("copy"); status != 1 || result.Success {
		t.Fatalf("a failed copy to a connected repository fails: %d %+v", status, result)
	}
}
