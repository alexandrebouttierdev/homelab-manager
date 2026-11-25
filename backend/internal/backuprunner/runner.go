// Package backuprunner is homelab-backup-runner, the program that
// homelab-backup@.service runs as root (SPEC_ADMINISTRATION.md § 7.1). It
// reads the job the Manager wrote, validates it again, translates it into
// restic arguments by a fixed schema and runs restic without a shell. No
// value of the job becomes an option: sources follow "--", exclusions go
// through a file, secrets through the environment of restic alone.
package backuprunner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"
)

// Exit codes of restic (restic 0.17 and later).
const (
	resticPartialBackup = 3
	// maxLineBytes bounds one JSON line of restic's output.
	maxLineBytes = 1 << 20
	// maxStderrBytes bounds what is kept of restic's error output.
	maxStderrBytes = 64 << 10
	// resultMode lets the Manager's group read the result.
	resultMode = 0o640
)

// Config locates the files and the programs; Defaults gives the deployment
// values.
type Config struct {
	JobsDir  string
	RunsDir  string
	CacheDir string
	TempDir  string
	Restic   string
	// Path is the PATH given to restic (rest-server and S3 need none; it
	// is kept fixed).
	Path string
	// RestoreRoot holds the quarantine directories; the job names
	// domain.BackupRestoreRoot, mapped here (tests use a temporary one).
	RestoreRoot string
	Now         func() time.Time
}

func Defaults() Config {
	return Config{
		JobsDir: "/var/lib/homelab-backup/jobs", RunsDir: "/var/lib/homelab-backup/runs",
		CacheDir: "/var/cache/homelab-backup", TempDir: os.TempDir(), Restic: "/usr/bin/restic",
		Path: "/usr/sbin:/usr/bin:/sbin:/bin", RestoreRoot: domain.BackupRestoreRoot, Now: time.Now,
	}
}

// Run executes the operation and writes its result; it returns the exit
// status of the program: 0 on success (or a snapshot with unreadable
// files), 1 otherwise, 2 when not even a result could be written.
func Run(ctx context.Context, config Config, operation string, stderr io.Writer) int {
	op := domain.BackupOperation(operation)
	if !op.Valid() {
		fmt.Fprintf(stderr, "unknown operation %q\n", operation)
		return 2
	}
	result := backupjob.Result{Version: domain.BackupJobVersion, Operation: op, StartedAt: config.Now().UTC()}
	var secrets []string
	err := execute(ctx, config, op, &result, &secrets)
	result.FinishedAt = config.Now().UTC()
	if err != nil {
		result.Success = false
		if result.Message == "" {
			result.Message = domain.TruncateBackupMessage(err.Error())
		}
		fmt.Fprintf(stderr, "%s failed: %v\n", op, scrub(err.Error(), secrets))
	}
	// restic hides the passwords it prints; the runner makes sure none
	// reaches the Manager.
	result.Message = scrub(result.Message, secrets)
	if writeErr := backupjob.WriteAtomic(backupjob.ResultPath(config.RunsDir, op), result, resultMode); writeErr != nil {
		fmt.Fprintf(stderr, "write the result: %v\n", writeErr)
		return 2
	}
	if !result.Success {
		return 1
	}
	return 0
}

func execute(ctx context.Context, config Config, op domain.BackupOperation, result *backupjob.Result, secrets *[]string) error {
	var job backupjob.Job
	if err := backupjob.ReadStrict(backupjob.JobPath(config.JobsDir, op), backupjob.MaxJobBytes, &job); err != nil {
		return fmt.Errorf("read the job: %w", err)
	}
	if job.Operation != op {
		return fmt.Errorf("the job is for %q, not %q", job.Operation, op)
	}
	if err := job.Validate(); err != nil {
		return fmt.Errorf("invalid job: %w", err)
	}
	credentials, err := readSecrets(config, op, job)
	if err != nil {
		return err
	}
	*secrets = credentials.Values()
	if op == domain.BackupOpCopy {
		return copySnapshots(ctx, config, job, credentials, result)
	}
	env := resticEnv(config, job, credentials.BackupCredentials)
	switch op {
	case domain.BackupOpInit:
		output, err := runCaptured(ctx, config, env, []string{"init", "--json"})
		if err != nil {
			result.Message = output.message()
			return err
		}
		result.Success = true
		return nil
	case domain.BackupOpBackup:
		if err := backup(ctx, config, env, job, result); err != nil {
			return err
		}
		snapshots, err := listSnapshots(ctx, config, env)
		if err != nil {
			// The snapshot was made: the backup succeeded, the listing is
			// reported in the message.
			result.Message = domain.TruncateBackupMessage("snapshot list unavailable: " + err.Error())
			return nil
		}
		result.Snapshots = snapshots
		return nil
	case domain.BackupOpSnapshots:
		snapshots, err := listSnapshots(ctx, config, env)
		if err != nil {
			result.Message = domain.TruncateBackupMessage(err.Error())
			return err
		}
		result.Snapshots, result.Success = snapshots, true
		return nil
	case domain.BackupOpUnlock:
		output, err := runCaptured(ctx, config, env, []string{"unlock", "--remove-all"})
		if err != nil {
			result.Message = output.message()
			return err
		}
		result.Success = true
		return nil
	case domain.BackupOpCheck:
		percent := strconv.Itoa(job.CheckSubsetPercent) + "%"
		output, err := runCaptured(ctx, config, env, []string{"check", "--read-data-subset", percent})
		result.Details = &domain.BackupRunDetails{SubsetPercent: job.CheckSubsetPercent}
		if err != nil {
			result.Message = output.message()
			return err
		}
		result.Success = true
		return nil
	case domain.BackupOpForgetPrune:
		return forgetPrune(ctx, config, env, job, result)
	case domain.BackupOpRestore:
		return restore(ctx, config, env, job, result)
	case domain.BackupOpDiff:
		return diff(ctx, config, env, job, result)
	}
	return fmt.Errorf("operation %q is not supported", op)
}

// forgetPrune applies the retention policy to the snapshots of the
// Manager, prunes, then lists what is left.
func forgetPrune(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	args := []string{"forget", "--json", "--tag", domain.BackupTag}
	for _, keep := range []struct {
		flag  string
		count int
	}{
		{"--keep-daily", job.Retention.KeepDaily}, {"--keep-weekly", job.Retention.KeepWeekly},
		{"--keep-monthly", job.Retention.KeepMonthly}, {"--keep-yearly", job.Retention.KeepYearly},
	} {
		if keep.count > 0 {
			args = append(args, keep.flag, strconv.Itoa(keep.count))
		}
	}
	args = append(args, "--prune")
	output, err := runCaptured(ctx, config, env, args)
	if err != nil {
		result.Message = output.message()
		return err
	}
	removed := 0
	for _, line := range bytes.Split(output.stdout, []byte("\n")) {
		var groups []struct {
			Remove []json.RawMessage `json:"remove"`
		}
		if json.Unmarshal(bytes.TrimSpace(line), &groups) == nil {
			for _, group := range groups {
				removed += len(group.Remove)
			}
		}
	}
	result.Details = &domain.BackupRunDetails{SnapshotsRemoved: removed}
	result.Success = true
	if snapshots, err := listSnapshots(ctx, config, env); err == nil {
		result.Snapshots = snapshots
	} else {
		result.Message = domain.TruncateBackupMessage("snapshot list unavailable: " + err.Error())
	}
	return nil
}

// restore brings a snapshot back into the quarantine directory of the run,
// which it creates (0700); a dry run leaves it empty and removes it.
func restore(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	request := job.Restore
	target := filepath.Join(config.RestoreRoot, filepath.Base(request.Target))
	result.Details = &domain.BackupRunDetails{SnapshotID: request.SnapshotID, Target: request.Target, DryRun: request.DryRun}
	if err := os.MkdirAll(config.RestoreRoot, 0o700); err != nil {
		return fmt.Errorf("create the quarantine: %w", err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		return fmt.Errorf("create the restore directory: %w", err)
	}
	if request.DryRun {
		defer func() { _ = os.Remove(target) }()
	}
	args := []string{"restore", "--json", "--target", target}
	for _, include := range request.Include {
		args = append(args, "--include", include)
	}
	if request.DryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, "--", request.SnapshotID)
	summary, output, err := runStreaming(ctx, config, env, args)
	if summary != nil {
		result.Details.TotalFiles, result.Details.FilesRestored = summary.TotalFiles, summary.FilesRestored
		result.Details.TotalBytes, result.Details.BytesRestored = summary.TotalBytes, summary.BytesRestored
	}
	if err != nil {
		result.Message = output.message()
		return err
	}
	result.Success = true
	return nil
}

// readSecrets reads then removes the one-use secrets file: it never stays
// on disk after the run.
func readSecrets(config Config, op domain.BackupOperation, job backupjob.Job) (backupjob.Secrets, error) {
	path := backupjob.SecretsPath(config.JobsDir, op)
	var secrets backupjob.Secrets
	readErr := backupjob.ReadStrict(path, backupjob.MaxSecretsBytes, &secrets)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return backupjob.Secrets{}, fmt.Errorf("remove the secrets file: %w", err)
	}
	if readErr != nil {
		return backupjob.Secrets{}, fmt.Errorf("read the secrets: %w", readErr)
	}
	if err := job.ValidateSecrets(secrets); err != nil {
		return backupjob.Secrets{}, fmt.Errorf("invalid secrets: %w", err)
	}
	return secrets, nil
}

// scrub replaces every secret found in a message.
func scrub(message string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 4 {
			message = strings.ReplaceAll(message, secret, "***")
		}
	}
	return message
}

// resticEnv is the whole environment of restic: nothing is inherited.
func resticEnv(config Config, job backupjob.Job, credentials domain.BackupCredentials) []string {
	env := []string{
		"PATH=" + config.Path,
		"HOME=" + config.CacheDir,
		"TMPDIR=" + config.TempDir,
		"RESTIC_CACHE_DIR=" + config.CacheDir,
		"RESTIC_REPOSITORY=" + job.BackupRepository().ResticRepository(),
		"RESTIC_PASSWORD=" + credentials.Password,
		"RESTIC_PROGRESS_FPS=0.1",
	}
	switch job.RepositoryType {
	case domain.RepositoryRest:
		if credentials.Username != "" {
			env = append(env, "RESTIC_REST_USERNAME="+credentials.Username, "RESTIC_REST_PASSWORD="+credentials.Secret)
		}
	case domain.RepositoryS3:
		env = append(env, "AWS_ACCESS_KEY_ID="+credentials.Username, "AWS_SECRET_ACCESS_KEY="+credentials.Secret)
	}
	return env
}

// resticOutput keeps what restic said about a failure. In JSON mode,
// restic prints its errors as JSON lines, on stdout or stderr depending on
// the version; both are read.
type resticOutput struct {
	exitError string
	errors    []string
	plain     string
	stdout    []byte
}

func (o resticOutput) message() string {
	switch {
	case o.exitError != "":
		return domain.TruncateBackupMessage(o.exitError)
	case o.plain != "":
		return domain.TruncateBackupMessage(o.plain)
	case len(o.errors) > 0:
		return domain.TruncateBackupMessage(o.errors[len(o.errors)-1])
	}
	return ""
}

// absorb reads one line of restic output: a JSON error, an exit error, or
// plain text (the last non-empty plain line is kept).
func (o *resticOutput) absorb(line []byte) *backupMessage {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	var message backupMessage
	if json.Unmarshal(line, &message) != nil || message.MessageType == "" {
		if line[0] != '{' && line[0] != '[' {
			o.plain = string(line)
		}
		return nil
	}
	switch message.MessageType {
	case "error":
		text := message.Item
		if message.Error != nil {
			text = strings.TrimSpace(strings.TrimPrefix(message.Item+": "+message.Error.Message, ": "))
		}
		o.errors = append(o.errors, text)
		if len(o.errors) > 20 {
			o.errors = o.errors[1:]
		}
	case "exit_error":
		o.exitError = message.Message
	}
	return &message
}

func (o *resticOutput) absorbAll(data []byte) {
	for _, line := range bytes.Split(data, []byte("\n")) {
		o.absorb(line)
	}
}

func backup(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	args := []string{"backup", "--json", "--tag", domain.BackupTag}
	if job.ExcludeCaches {
		args = append(args, "--exclude-caches")
	}
	if job.ExcludeIfPresent != "" {
		args = append(args, "--exclude-if-present", job.ExcludeIfPresent)
	}
	if len(job.Excludes) > 0 {
		file, err := os.CreateTemp(config.TempDir, "excludes-*.txt")
		if err != nil {
			return fmt.Errorf("create the exclusion file: %w", err)
		}
		defer func() { _ = os.Remove(file.Name()) }()
		_, writeErr := file.WriteString(strings.Join(job.Excludes, "\n") + "\n")
		if closeErr := file.Close(); writeErr != nil || closeErr != nil {
			return fmt.Errorf("write the exclusion file: %w", errors.Join(writeErr, closeErr))
		}
		args = append(args, "--exclude-file", file.Name())
	}
	args = append(args, "--")
	args = append(args, job.Sources...)

	summary, output, runErr := runStreaming(ctx, config, env, args)
	if summary != nil {
		result.Summary = summary.toSummary()
		result.SnapshotID = summary.SnapshotID
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		result.Success = true
		return nil
	case errors.As(runErr, &exitErr) && exitErr.ExitCode() == resticPartialBackup && summary != nil:
		result.Success, result.Warning = true, true
		result.Message = domain.TruncateBackupMessage("some files could not be read: " + output.message())
		return nil
	}
	result.Message = output.message()
	return runErr
}

// runStreaming runs a long restic command whose JSON lines carry a
// summary; it fails when restic fails or gives no summary.
func runStreaming(ctx context.Context, config Config, env, args []string) (*backupMessage, resticOutput, error) {
	cmd := exec.CommandContext(ctx, config.Restic, args...)
	cmd.Env = env
	stderr := &boundedBuffer{limit: maxStderrBytes}
	cmd.Stderr = stderr
	output := resticOutput{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, output, fmt.Errorf("start restic: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, output, fmt.Errorf("start restic: %w", err)
	}
	summary, scanErr := readBackupStream(stdout, &output)
	waitErr := cmd.Wait()
	output.absorbAll(stderr.Bytes())
	if waitErr != nil {
		return summary, output, waitErr
	}
	return summary, output, errors.Join(scanErr, errorIf(summary == nil, "restic gave no summary"))
}

func errorIf(condition bool, message string) error {
	if condition {
		return errors.New(message)
	}
	return nil
}

// backupMessage is the part of restic's JSON lines the runner reads.
type backupMessage struct {
	MessageType string `json:"message_type"`
	Message     string `json:"message"`
	Error       *struct {
		Message string `json:"message"`
	} `json:"error"`
	Item                string  `json:"item"`
	FilesNew            int64   `json:"files_new"`
	FilesChanged        int64   `json:"files_changed"`
	FilesUnmodified     int64   `json:"files_unmodified"`
	DirsNew             int64   `json:"dirs_new"`
	DirsChanged         int64   `json:"dirs_changed"`
	DirsUnmodified      int64   `json:"dirs_unmodified"`
	DataAdded           int64   `json:"data_added"`
	TotalFilesProcessed int64   `json:"total_files_processed"`
	TotalBytesProcessed int64   `json:"total_bytes_processed"`
	TotalDuration       float64 `json:"total_duration"`
	SnapshotID          string  `json:"snapshot_id"`
	// Restore summary.
	TotalFiles    int64 `json:"total_files"`
	FilesRestored int64 `json:"files_restored"`
	TotalBytes    int64 `json:"total_bytes"`
	BytesRestored int64 `json:"bytes_restored"`
}

func (m backupMessage) toSummary() *backupjob.Summary {
	return &backupjob.Summary{
		FilesNew: m.FilesNew, FilesChanged: m.FilesChanged, FilesUnmodified: m.FilesUnmodified,
		DirsNew: m.DirsNew, DirsChanged: m.DirsChanged, DirsUnmodified: m.DirsUnmodified,
		DataAdded: m.DataAdded, TotalFilesProcessed: m.TotalFilesProcessed,
		TotalBytesProcessed: m.TotalBytesProcessed, DurationSeconds: m.TotalDuration,
	}
}

// readBackupStream reads the JSON lines of restic backup: progress lines
// are dropped, file errors and the exit error kept (the last 20), the
// summary returned. Lines that are not JSON are ignored.
func readBackupStream(stdout io.Reader, output *resticOutput) (*backupMessage, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	var summary *backupMessage
	for scanner.Scan() {
		if message := output.absorb(scanner.Bytes()); message != nil && message.MessageType == "summary" {
			summary = message
		}
	}
	if err := scanner.Err(); err != nil {
		// Drain so that restic does not block on a full pipe.
		_, _ = io.Copy(io.Discard, stdout)
		return summary, fmt.Errorf("read restic output: %w", err)
	}
	return summary, nil
}

// runCaptured runs a short restic command and keeps its output, bounded.
func runCaptured(ctx context.Context, config Config, env, args []string) (resticOutput, error) {
	cmd := exec.CommandContext(ctx, config.Restic, args...)
	cmd.Env = env
	stdout := &boundedBuffer{limit: backupjob.MaxResultBytes}
	stderr := &boundedBuffer{limit: maxStderrBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	output := resticOutput{}
	output.absorbAll(stderr.Bytes())
	if err != nil {
		output.absorbAll(stdout.Bytes())
	}
	if stdout.truncated {
		return output, errors.Join(err, errors.New("restic output exceeds its bound"))
	}
	if err != nil {
		return output, fmt.Errorf("restic %s: %w", args[0], err)
	}
	output.stdout = stdout.Bytes()
	return output, nil
}

type resticSnapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Paths    []string  `json:"paths"`
	Tags     []string  `json:"tags"`
	Summary  *struct {
		TotalFilesProcessed int64 `json:"total_files_processed"`
		TotalBytesProcessed int64 `json:"total_bytes_processed"`
		DataAdded           int64 `json:"data_added"`
	} `json:"summary"`
}

// listSnapshots lists the snapshots the Manager made, newest first, at
// most domain.MaxBackupSnapshots.
func listSnapshots(ctx context.Context, config Config, env []string) ([]backupjob.Snapshot, error) {
	output, err := runCaptured(ctx, config, env, []string{"snapshots", "--json", "--tag", domain.BackupTag})
	if err != nil {
		if message := output.message(); message != "" {
			return nil, errors.New(message)
		}
		return nil, err
	}
	var listed []resticSnapshot
	if err := json.Unmarshal(bytes.TrimSpace(output.stdout), &listed); err != nil {
		return nil, fmt.Errorf("decode the snapshots: %w", err)
	}
	sort.SliceStable(listed, func(i, j int) bool { return listed[i].Time.After(listed[j].Time) })
	if len(listed) > domain.MaxBackupSnapshots {
		listed = listed[:domain.MaxBackupSnapshots]
	}
	snapshots := make([]backupjob.Snapshot, 0, len(listed))
	for _, s := range listed {
		snapshot := backupjob.Snapshot{
			ID: s.ID, ShortID: s.ShortID, Time: s.Time.UTC(), Hostname: s.Hostname,
			Paths: s.Paths, Tags: s.Tags,
		}
		if s.Summary != nil {
			snapshot.TotalFiles, snapshot.TotalBytes, snapshot.DataAdded =
				s.Summary.TotalFilesProcessed, s.Summary.TotalBytesProcessed, s.Summary.DataAdded
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// boundedBuffer keeps at most limit bytes; the rest is counted as
// truncated and dropped.
type boundedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	room := b.limit - b.Len()
	if room <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		b.truncated = true
		_, _ = b.Buffer.Write(p[:room])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

// diff compares two snapshots: every change is counted, the first ones
// are kept with their path, within domain.MaxBackupDiffChanges and
// domain.MaxBackupDiffBytes.
func diff(ctx context.Context, config Config, env []string, job backupjob.Job, result *backupjob.Result) error {
	request := job.Diff
	report := &domain.BackupDiff{From: request.From, To: request.To, Changes: []domain.BackupDiffChange{}}
	result.Details = &domain.BackupRunDetails{Diff: report}
	cmd := exec.CommandContext(ctx, config.Restic, "diff", "--json", "--", request.From, request.To)
	cmd.Env = env
	stderr := &boundedBuffer{limit: maxStderrBytes}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("start restic: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start restic: %w", err)
	}
	output := resticOutput{}
	statistics, scanErr := readDiffStream(stdout, &output, report)
	waitErr := cmd.Wait()
	output.absorbAll(stderr.Bytes())
	if waitErr != nil {
		result.Message = output.message()
		return waitErr
	}
	if err := errors.Join(scanErr, errorIf(!statistics, "restic gave no statistics")); err != nil {
		return err
	}
	result.Success = true
	return nil
}

// diffMessage is a line of restic diff --json: a change or the statistics.
type diffMessage struct {
	MessageType  string `json:"message_type"`
	Path         string `json:"path"`
	Modifier     string `json:"modifier"`
	ChangedFiles int64  `json:"changed_files"`
	Added        struct {
		Files  int64 `json:"files"`
		Dirs   int64 `json:"dirs"`
		Others int64 `json:"others"`
		Bytes  int64 `json:"bytes"`
	} `json:"added"`
	Removed struct {
		Files  int64 `json:"files"`
		Dirs   int64 `json:"dirs"`
		Others int64 `json:"others"`
		Bytes  int64 `json:"bytes"`
	} `json:"removed"`
}

// diffModifiers are the modifiers restic prints.
var diffModifiers = map[string]bool{"+": true, "-": true, "M": true, "U": true, "T": true, "?": true}

func readDiffStream(stdout io.Reader, output *resticOutput, report *domain.BackupDiff) (bool, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	statistics, kept := false, 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if output.absorb(line) == nil {
			continue
		}
		var message diffMessage
		if json.Unmarshal(bytes.TrimSpace(line), &message) != nil {
			continue
		}
		switch message.MessageType {
		case "change":
			if !diffModifiers[message.Modifier] || message.Path == "" {
				continue
			}
			report.TotalChanges++
			path := strings.ToValidUTF8(message.Path, "�")
			if len(report.Changes) >= domain.MaxBackupDiffChanges || kept+len(path) > domain.MaxBackupDiffBytes {
				report.Truncated = true
				continue
			}
			kept += len(path)
			report.Changes = append(report.Changes, domain.BackupDiffChange{Path: path, Modifier: message.Modifier})
		case "statistics":
			statistics = true
			report.ChangedFiles = message.ChangedFiles
			report.Added = domain.BackupDiffCounts{
				Files: message.Added.Files, Dirs: message.Added.Dirs, Others: message.Added.Others, Bytes: message.Added.Bytes,
			}
			report.Removed = domain.BackupDiffCounts{
				Files: message.Removed.Files, Dirs: message.Removed.Dirs, Others: message.Removed.Others, Bytes: message.Removed.Bytes,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_, _ = io.Copy(io.Discard, stdout)
		return statistics, fmt.Errorf("read restic output: %w", err)
	}
	return statistics, nil
}

// copySnapshots copies the snapshots of the Manager from the repository of
// the job to the copy repository: restic opens the destination as its
// repository and the source as its "from" repository. It initializes the
// destination first when asked, with the chunker parameters of the source
// to keep the deduplication, and applies the retention policy after the
// copy when one is given.
func copySnapshots(ctx context.Context, config Config, job backupjob.Job, secrets backupjob.Secrets, result *backupjob.Result) error {
	destination := job.Copy
	report := &domain.BackupCopyResult{Repository: destination.BackupRepository()}
	result.Details = &domain.BackupRunDetails{Copy: report}
	env, err := copyEnv(config, job, secrets)
	if err != nil {
		return err
	}
	before := 0
	if destination.Init {
		output, err := runCaptured(ctx, config, env, []string{"init", "--copy-chunker-params", "--json"})
		if err != nil {
			result.Message = output.message()
			return err
		}
		report.Initialized = true
	} else if before, err = countSnapshots(ctx, config, env); err != nil {
		result.Message = domain.TruncateBackupMessage(err.Error())
		return err
	}
	// Once the repository is created, a later failure is a warning: the
	// Manager keeps its password, and the next copy resumes.
	partial := func(step string, err error, message string) error {
		if message == "" {
			message = err.Error()
		}
		result.Message = domain.TruncateBackupMessage(step + ": " + message)
		if !report.Initialized {
			return err
		}
		result.Success, result.Warning = true, true
		return nil
	}
	output, err := runCaptured(ctx, config, env, []string{"copy", "--tag", domain.BackupTag})
	if err != nil {
		return partial("the copy failed", err, output.message())
	}
	after, err := countSnapshots(ctx, config, env)
	if err != nil {
		return partial("the copy succeeded, its snapshots could not be counted", err, "")
	}
	report.SnapshotsCopied, report.SnapshotsInTarget = max(after-before, 0), after
	result.Success = true
	if r := destination.Retention; r != nil {
		retention := job
		retention.Retention = r
		forget := backupjob.Result{}
		if err := forgetPrune(ctx, config, env, retention, &forget); err != nil {
			// The copy is done: its retention failing is a warning.
			result.Warning = true
			result.Message = domain.TruncateBackupMessage("the copy succeeded, its retention failed: " + forget.Message)
			return nil
		}
		report.SnapshotsRemoved = forget.Details.SnapshotsRemoved
		report.SnapshotsInTarget = max(after-report.SnapshotsRemoved, 0)
	}
	return nil
}

// copyEnv is the environment of a copy. restic reads a single set of
// rest-server and S3 credentials for both repositories: the rest-server
// credentials go into each repository URL, which restic hides in its
// messages, and both S3 repositories must share their key.
func copyEnv(config Config, job backupjob.Job, secrets backupjob.Secrets) ([]string, error) {
	source, destination := job.BackupRepository(), job.Copy.BackupRepository()
	env := []string{
		"PATH=" + config.Path,
		"HOME=" + config.CacheDir,
		"TMPDIR=" + config.TempDir,
		"RESTIC_CACHE_DIR=" + config.CacheDir,
		"RESTIC_REPOSITORY=" + withRestCredentials(destination, *secrets.Copy),
		"RESTIC_PASSWORD=" + secrets.Copy.Password,
		"RESTIC_FROM_REPOSITORY=" + withRestCredentials(source, secrets.BackupCredentials),
		"RESTIC_FROM_PASSWORD=" + secrets.Password,
		"RESTIC_PROGRESS_FPS=0.1",
	}
	var s3 *domain.BackupCredentials
	for _, side := range []struct {
		repository  domain.BackupRepository
		credentials domain.BackupCredentials
	}{{source, secrets.BackupCredentials}, {destination, *secrets.Copy}} {
		if side.repository.Type != domain.RepositoryS3 {
			continue
		}
		if s3 != nil && (s3.Username != side.credentials.Username || s3.Secret != side.credentials.Secret) {
			return nil, errors.New("restic copies between two S3 repositories with the same access key only")
		}
		credentials := side.credentials
		s3 = &credentials
	}
	if s3 != nil {
		env = append(env, "AWS_ACCESS_KEY_ID="+s3.Username, "AWS_SECRET_ACCESS_KEY="+s3.Secret)
	}
	return env, nil
}

// withRestCredentials puts the rest-server credentials into the URL of a
// rest repository; other repositories are named as usual.
func withRestCredentials(repository domain.BackupRepository, credentials domain.BackupCredentials) string {
	if repository.Type != domain.RepositoryRest || credentials.Username == "" {
		return repository.ResticRepository()
	}
	parsed, err := url.Parse(repository.Location)
	if err != nil {
		return repository.ResticRepository()
	}
	parsed.User = url.UserPassword(credentials.Username, credentials.Secret)
	return "rest:" + parsed.String()
}

// countSnapshots counts the snapshots of the Manager in the repository.
func countSnapshots(ctx context.Context, config Config, env []string) (int, error) {
	output, err := runCaptured(ctx, config, env, []string{"snapshots", "--json", "--tag", domain.BackupTag})
	if err != nil {
		if message := output.message(); message != "" {
			return 0, errors.New(message)
		}
		return 0, err
	}
	var listed []json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.stdout), &listed); err != nil {
		return 0, fmt.Errorf("decode the snapshots: %w", err)
	}
	return len(listed), nil
}
