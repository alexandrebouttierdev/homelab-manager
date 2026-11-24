package backuprunner

import (
	"bytes"
	"context"
	"encoding/json"
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
const fakeRestic = `#!/bin/sh
dir=$(dirname "$0")
printf '%s\n' "$@" > "$dir/args.$1"
env | sort > "$dir/env.$1"
previous=""
for arg in "$@"; do
  if [ "$previous" = "--exclude-file" ]; then cp "$arg" "$dir/excludes"; fi
  previous="$arg"
done
[ -f "$dir/$1.out" ] && cat "$dir/$1.out"
[ -f "$dir/$1.err" ] && cat "$dir/$1.err" >&2
code=0
[ -f "$dir/$1.code" ] && code=$(cat "$dir/$1.code")
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
		TempDir: filepath.Join(root, "tmp"), Restic: restic, Path: "/usr/bin:/bin",
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
	if status, _, _ := f.run("restore"); status != 2 {
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
	f.job(backupjob.Job{
		Version: 1, Operation: domain.BackupOpBackup, RepositoryType: domain.RepositoryLocal, Repository: repository,
		Sources: []string{source},
	}, domain.BackupCredentials{Password: "wrong-password"})
	if status, result, _ := f.run("backup"); status != 1 || result.Message == "" {
		t.Fatalf("wrong password: %d %+v", status, result)
	}
}
