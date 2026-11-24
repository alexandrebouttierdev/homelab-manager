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
	"os"
	"os/exec"
	"sort"
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
	Now  func() time.Time
}

func Defaults() Config {
	return Config{
		JobsDir: "/var/lib/homelab-backup/jobs", RunsDir: "/var/lib/homelab-backup/runs",
		CacheDir: "/var/cache/homelab-backup", TempDir: os.TempDir(), Restic: "/usr/bin/restic",
		Path: "/usr/sbin:/usr/bin:/sbin:/bin", Now: time.Now,
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
	err := execute(ctx, config, op, &result)
	result.FinishedAt = config.Now().UTC()
	if err != nil {
		result.Success = false
		if result.Message == "" {
			result.Message = domain.TruncateBackupMessage(err.Error())
		}
		fmt.Fprintf(stderr, "%s failed: %v\n", op, err)
	}
	if writeErr := backupjob.WriteAtomic(backupjob.ResultPath(config.RunsDir, op), result, resultMode); writeErr != nil {
		fmt.Fprintf(stderr, "write the result: %v\n", writeErr)
		return 2
	}
	if !result.Success {
		return 1
	}
	return 0
}

func execute(ctx context.Context, config Config, op domain.BackupOperation, result *backupjob.Result) error {
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
	credentials, err := readSecrets(config, op, job.RepositoryType)
	if err != nil {
		return err
	}
	env := resticEnv(config, job, credentials)
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
	}
	return fmt.Errorf("operation %q is not supported", op)
}

// readSecrets reads then removes the one-use secrets file: it never stays
// on disk after the run.
func readSecrets(config Config, op domain.BackupOperation, kind domain.BackupRepositoryType) (domain.BackupCredentials, error) {
	path := backupjob.SecretsPath(config.JobsDir, op)
	var credentials domain.BackupCredentials
	readErr := backupjob.ReadStrict(path, backupjob.MaxSecretsBytes, &credentials)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return domain.BackupCredentials{}, fmt.Errorf("remove the secrets file: %w", err)
	}
	if readErr != nil {
		return domain.BackupCredentials{}, fmt.Errorf("read the secrets: %w", readErr)
	}
	if err := domain.ValidateBackupCredentials(kind, credentials); err != nil {
		return domain.BackupCredentials{}, fmt.Errorf("invalid secrets: %w", err)
	}
	return credentials, nil
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

	cmd := exec.CommandContext(ctx, config.Restic, args...)
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
	summary, scanErr := readBackupStream(stdout, &output)
	waitErr := cmd.Wait()
	output.absorbAll(stderr.Bytes())
	if summary != nil {
		result.Summary = summary.toSummary()
		result.SnapshotID = summary.SnapshotID
	}
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil && scanErr == nil && summary != nil:
		result.Success = true
		return nil
	case errors.As(waitErr, &exitErr) && exitErr.ExitCode() == resticPartialBackup && summary != nil:
		result.Success, result.Warning = true, true
		result.Message = domain.TruncateBackupMessage("some files could not be read: " + output.message())
		return nil
	}
	result.Message = output.message()
	return errors.Join(waitErr, scanErr, errorIf(summary == nil && waitErr == nil, "restic gave no summary"))
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
