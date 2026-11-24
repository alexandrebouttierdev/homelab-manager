package domain

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Bounds of the backups module (SPEC_ADMINISTRATION.md § 7).
const (
	MaxBackupSources        = 50
	MaxBackupExcludes       = 500
	maxBackupPathBytes      = 4096
	maxBackupLocationBytes  = 2048
	maxBackupCredentialSize = 1024
	// BackupPasswordBytes is the entropy of a generated repository password.
	BackupPasswordBytes = 32
	// MaxBackupMessageBytes bounds the error kept from a run.
	MaxBackupMessageBytes = 2048
	// MaxBackupSnapshots bounds the snapshots kept from a listing, newest
	// first.
	MaxBackupSnapshots = 500
	// BackupRunRetention keeps the history of the runs.
	BackupRunRetention = 365 * 24 * time.Hour
	// BackupCatchUpDelay leaves the Manager settle before running a
	// scheduled backup missed while it was stopped.
	BackupCatchUpDelay = 2 * time.Minute
	// DefaultBackupRunLimit and MaxBackupRunLimit bound a page of runs.
	DefaultBackupRunLimit = 50
	MaxBackupRunLimit     = 200
	// BackupTag marks the snapshots the Manager makes.
	BackupTag = "homelab-manager"
	// BackupJobVersion is the version of the job and result files
	// exchanged with homelab-backup-runner.
	BackupJobVersion = 1
)

var (
	ErrBackupNotConfigured      = errors.New("no backup repository is configured")
	ErrBackupNotInitialized     = errors.New("the backup repository is not initialized")
	ErrBackupAlreadyInitialized = errors.New("the backup repository is already initialized")
	ErrBackupAlreadyRunning     = errors.New("a backup operation is already running")
	ErrBackupSecretsMissing     = errors.New("the backup secrets cannot be opened without the secrets key")
	ErrBackupRunNotFound        = errors.New("backup run not found")
	ErrInvalidBackupRunFilter   = errors.New("backup run filter values are out of bounds")
	ErrBackupRepositoryInUse    = errors.New("the repository cannot change while an operation is running")
	// ErrBackupsNotReady: the Manager is starting or stopping.
	ErrBackupsNotReady         = errors.New("backups are starting or stopping")
	backupExcludeMarkerPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,255}$`)
)

// BackupValidationError names the invalid field of a backup setting.
type BackupValidationError struct {
	Field   string
	Message string
}

func (e *BackupValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func invalidBackup(field, format string, args ...any) error {
	return &BackupValidationError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// BackupRepositoryType is where restic stores the snapshots.
type BackupRepositoryType string

const (
	// RepositoryLocal: a directory of the machine, possibly a mounted share.
	RepositoryLocal BackupRepositoryType = "local"
	// RepositoryRest: a rest-server over HTTPS.
	RepositoryRest BackupRepositoryType = "rest"
	// RepositoryS3: an S3-compatible bucket over HTTPS (Backblaze B2 too).
	RepositoryS3 BackupRepositoryType = "s3"
)

func (t BackupRepositoryType) Valid() bool {
	return t == RepositoryLocal || t == RepositoryRest || t == RepositoryS3
}

// BackupRepository locates the repository. Location is an absolute path
// for a local repository, an https URL otherwise.
type BackupRepository struct {
	Type     BackupRepositoryType
	Location string
}

// ValidateBackupRepository checks a repository received from a client or
// from a job file. Remote repositories use HTTPS: the data is encrypted
// by restic, but the credentials are not.
func ValidateBackupRepository(repository BackupRepository) error {
	location := repository.Location
	if location == "" || len(location) > maxBackupLocationBytes || !utf8.ValidString(location) || hasControl(location) {
		return invalidBackup("location", "must be 1 to %d bytes without control characters", maxBackupLocationBytes)
	}
	switch repository.Type {
	case RepositoryLocal:
		if err := validateAbsolutePath(location); err != nil {
			return invalidBackup("location", "%s", err.Error())
		}
		if location == "/" {
			return invalidBackup("location", "must not be the root directory")
		}
	case RepositoryRest, RepositoryS3:
		parsed, err := url.Parse(location)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
			return invalidBackup("location", "must be an https URL without credentials, query or fragment")
		}
		if repository.Type == RepositoryS3 && strings.Trim(parsed.Path, "/") == "" {
			return invalidBackup("location", "an S3 location names the bucket: https://host/bucket[/prefix]")
		}
	default:
		return invalidBackup("type", "must be local, rest or s3")
	}
	return nil
}

// ResticRepository is the repository as restic names it.
func (r BackupRepository) ResticRepository() string {
	switch r.Type {
	case RepositoryRest:
		return "rest:" + r.Location
	case RepositoryS3:
		return "s3:" + r.Location
	default:
		return r.Location
	}
}

// BackupCredentials are the secrets of a repository, sealed in the vault.
// Username and Secret are the rest-server user and password, or the S3
// access key and secret key; a local repository has neither.
type BackupCredentials struct {
	Password string `json:"password"`
	Username string `json:"username,omitempty"`
	Secret   string `json:"secret,omitempty"`
}

// BackupSecretsAssociatedData binds the sealed credentials to their use.
const BackupSecretsAssociatedData = "backup-repository"

// ValidateBackupCredentials checks the secrets of a repository type.
func ValidateBackupCredentials(repositoryType BackupRepositoryType, credentials BackupCredentials) error {
	check := func(field, value string, required bool) error {
		if value == "" {
			if required {
				return invalidBackup(field, "is required")
			}
			return nil
		}
		if len(value) > maxBackupCredentialSize || !utf8.ValidString(value) || hasControl(value) {
			return invalidBackup(field, "must be at most %d bytes without control characters", maxBackupCredentialSize)
		}
		return nil
	}
	if err := check("password", credentials.Password, true); err != nil {
		return err
	}
	remote := repositoryType == RepositoryRest || repositoryType == RepositoryS3
	if !remote && (credentials.Username != "" || credentials.Secret != "") {
		return invalidBackup("username", "a local repository takes no credentials")
	}
	if err := check("username", credentials.Username, repositoryType == RepositoryS3); err != nil {
		return err
	}
	return check("secret", credentials.Secret, repositoryType == RepositoryS3)
}

// BackupSelection is what a backup reads.
type BackupSelection struct {
	Sources       []string
	Excludes      []string
	ExcludeCaches bool
	// ExcludeIfPresent skips the directories holding a file of this name;
	// "" disables it.
	ExcludeIfPresent string
}

// ValidateBackupSelection checks the sources and the exclusions (§ 7.3).
func ValidateBackupSelection(selection BackupSelection) error {
	if len(selection.Sources) > MaxBackupSources {
		return invalidBackup("sources", "at most %d sources", MaxBackupSources)
	}
	seen := make(map[string]struct{}, len(selection.Sources))
	for _, source := range selection.Sources {
		if err := validateAbsolutePath(source); err != nil {
			return invalidBackup("sources", "%q %s", source, err.Error())
		}
		if _, ok := seen[source]; ok {
			return invalidBackup("sources", "%q is listed twice", source)
		}
		seen[source] = struct{}{}
	}
	if len(selection.Excludes) > MaxBackupExcludes {
		return invalidBackup("excludes", "at most %d patterns", MaxBackupExcludes)
	}
	for _, pattern := range selection.Excludes {
		if err := validateExcludePattern(pattern); err != nil {
			return invalidBackup("excludes", "%q %s", pattern, err.Error())
		}
	}
	if selection.ExcludeIfPresent != "" && !backupExcludeMarkerPattern.MatchString(selection.ExcludeIfPresent) {
		return invalidBackup("excludeIfPresent", "must be a file name of letters, digits, '.', '_' or '-'")
	}
	return nil
}

// validateAbsolutePath accepts a clean absolute path: no "..", no "//",
// no trailing slash, no control character.
func validateAbsolutePath(value string) error {
	switch {
	case value == "" || len(value) > maxBackupPathBytes:
		return fmt.Errorf("must be 1 to %d bytes", maxBackupPathBytes)
	case !utf8.ValidString(value) || hasControl(value):
		return errors.New("must not contain control characters")
	case !filepath.IsAbs(value) || !strings.HasPrefix(value, "/"):
		return errors.New("must be an absolute path")
	case path.Clean(value) != value:
		return errors.New("must be a clean path, without '..', '//' or a trailing '/'")
	}
	return nil
}

// validateExcludePattern accepts a restic exclusion pattern: one line, no
// control character, a valid glob. A leading '!' (re-inclusion) is
// allowed, as restic allows it.
func validateExcludePattern(pattern string) error {
	if pattern == "" || len(pattern) > maxBackupPathBytes {
		return fmt.Errorf("must be 1 to %d bytes", maxBackupPathBytes)
	}
	if !utf8.ValidString(pattern) || hasControl(pattern) {
		return errors.New("must not contain control characters")
	}
	if strings.HasPrefix(pattern, "#") {
		return errors.New("must not start with '#', which restic reads as a comment")
	}
	glob := strings.TrimPrefix(pattern, "!")
	for _, part := range strings.Split(glob, "/") {
		if part == "**" {
			continue
		}
		if _, err := filepath.Match(part, ""); err != nil {
			return errors.New("is not a valid glob")
		}
	}
	return nil
}

func hasControl(value string) bool {
	return strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// BackupFrequency is how often the scheduled backup runs.
type BackupFrequency string

const (
	BackupDaily  BackupFrequency = "daily"
	BackupWeekly BackupFrequency = "weekly"
)

// BackupSchedule is the schedule kept by the Manager (decision D-12): it
// starts the backup itself at the scheduled time, in the time zone of the
// machine.
type BackupSchedule struct {
	Enabled   bool
	Frequency BackupFrequency
	// Weekday is used by a weekly schedule.
	Weekday time.Weekday
	Hour    int
	Minute  int
}

// DefaultBackupSchedule: every day at 03:00, disabled until a repository
// and sources exist.
var DefaultBackupSchedule = BackupSchedule{Frequency: BackupDaily, Hour: 3}

func ValidateBackupSchedule(schedule BackupSchedule) error {
	if schedule.Frequency != BackupDaily && schedule.Frequency != BackupWeekly {
		return invalidBackup("frequency", "must be daily or weekly")
	}
	if schedule.Weekday < time.Sunday || schedule.Weekday > time.Saturday {
		return invalidBackup("weekday", "must be 0 (Sunday) to 6")
	}
	if schedule.Hour < 0 || schedule.Hour > 23 || schedule.Minute < 0 || schedule.Minute > 59 {
		return invalidBackup("time", "must be a time of day")
	}
	return nil
}

// Next returns the first scheduled time strictly after the given time, in
// the location of that time. A disabled schedule has none.
func (s BackupSchedule) Next(after time.Time) (time.Time, bool) {
	if !s.Enabled {
		return time.Time{}, false
	}
	year, month, day := after.Date()
	candidate := time.Date(year, month, day, s.Hour, s.Minute, 0, 0, after.Location())
	for i := 0; i < 9; i++ {
		if candidate.After(after) && (s.Frequency == BackupDaily || candidate.Weekday() == s.Weekday) {
			return candidate, true
		}
		// AddDate keeps the wall clock across daylight saving changes.
		candidate = candidate.AddDate(0, 0, 1)
		candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), s.Hour, s.Minute, 0, 0, after.Location())
	}
	return time.Time{}, false
}

// BackupConfig is the whole configuration of the module.
type BackupConfig struct {
	// Repository is nil until an administrator sets one.
	Repository *BackupRepository
	// SealedCredentials holds the BackupCredentials sealed by the vault;
	// "" when none were given.
	SealedCredentials string
	// InitializedAt is set once restic init succeeded, or once the
	// administrator connected an existing repository.
	InitializedAt *time.Time
	Selection     BackupSelection
	Schedule      BackupSchedule
	UpdatedAt     *time.Time
	UpdatedBy     string
}

// Ready reports whether a backup can run.
func (c BackupConfig) Ready() error {
	switch {
	case c.Repository == nil:
		return ErrBackupNotConfigured
	case c.InitializedAt == nil || c.SealedCredentials == "":
		return ErrBackupNotInitialized
	case len(c.Selection.Sources) == 0:
		return invalidBackup("sources", "at least one source is required")
	}
	return nil
}

// BackupOperation is one instance of homelab-backup@.service.
type BackupOperation string

const (
	BackupOpInit   BackupOperation = "init"
	BackupOpBackup BackupOperation = "backup"
)

func (o BackupOperation) Valid() bool {
	return o == BackupOpInit || o == BackupOpBackup
}

// BackupUnit is the systemd unit that runs the operation.
func (o BackupOperation) BackupUnit() string {
	return "homelab-backup@" + string(o) + ".service"
}

// BackupOrigin tells what started a run.
type BackupOrigin string

const (
	BackupManual    BackupOrigin = "manual"
	BackupScheduled BackupOrigin = "schedule"
	// BackupCatchUp runs a scheduled backup missed while the Manager was
	// stopped.
	BackupCatchUp BackupOrigin = "catchup"
)

type BackupRunState string

const (
	BackupRunning   BackupRunState = "running"
	BackupSucceeded BackupRunState = "succeeded"
	BackupFailed    BackupRunState = "failed"
)

// BackupFailure tells why a run failed.
type BackupFailure string

const (
	// BackupFailureRunner: the unit or homelab-backup-runner is missing.
	BackupFailureRunner BackupFailure = "runner_not_installed"
	// BackupFailureNotAllowed: polkit refused to start the unit.
	BackupFailureNotAllowed BackupFailure = "not_allowed"
	// BackupFailureRestic: restic failed; Message holds its error.
	BackupFailureRestic  BackupFailure = "restic_failed"
	BackupFailureTimeout BackupFailure = "timeout"
	// BackupFailureResult: no result, a stale one, or one that is invalid.
	BackupFailureResult BackupFailure = "invalid_result"
	// BackupFailureSecrets: the secrets key is missing or the credentials
	// cannot be opened.
	BackupFailureSecrets BackupFailure = "secrets_unavailable"
	// BackupFailureJob: the job file could not be written.
	BackupFailureJob         BackupFailure = "job_failed"
	BackupFailureUnavailable BackupFailure = "systemd_unavailable"
	// BackupFailureInterrupted: the Manager stopped during the run.
	BackupFailureInterrupted BackupFailure = "interrupted"
)

// BackupRunError is a failure of the run adapter.
type BackupRunError struct {
	Failure BackupFailure
	// Message is the error restic printed, shown to the administrator.
	Message string
	Err     error
}

func (e *BackupRunError) Error() string {
	return fmt.Sprintf("backup run %s: %v", e.Failure, e.Err)
}

func (e *BackupRunError) Unwrap() error { return e.Err }

// BackupStats are the figures restic gives at the end of a backup.
type BackupStats struct {
	FilesNew            int64
	FilesChanged        int64
	FilesUnmodified     int64
	DirsNew             int64
	DirsChanged         int64
	DirsUnmodified      int64
	DataAdded           int64
	TotalFilesProcessed int64
	TotalBytesProcessed int64
	DurationSeconds     float64
}

// BackupSnapshot is one snapshot of the repository.
type BackupSnapshot struct {
	ID       string
	ShortID  string
	Time     time.Time
	Hostname string
	Paths    []string
	Tags     []string
	// Totals of the snapshot when restic recorded them (0.17 and later).
	TotalFiles int64
	TotalBytes int64
	DataAdded  int64
}

// BackupResult is what an operation returns.
type BackupResult struct {
	SnapshotID string
	// Warning: restic made the snapshot but could not read some files.
	Warning   bool
	Message   string
	Stats     *BackupStats
	Snapshots []BackupSnapshot
}

// BackupRun is one execution of an operation.
type BackupRun struct {
	ID          uuid.UUID
	Operation   BackupOperation
	Origin      BackupOrigin
	RequestedBy string
	State       BackupRunState
	Failure     BackupFailure
	Message     string
	Warning     bool
	SnapshotID  string
	Stats       *BackupStats
	StartedAt   time.Time
	FinishedAt  *time.Time
}

// TruncateBackupMessage bounds a message from restic to
// MaxBackupMessageBytes, cut on a character boundary.
func TruncateBackupMessage(message string) string {
	message = strings.ToValidUTF8(strings.TrimSpace(message), "�")
	if len(message) <= MaxBackupMessageBytes {
		return message
	}
	cut := MaxBackupMessageBytes
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut] + "…"
}

// BackupRunFilter selects runs of the history, newest first.
type BackupRunFilter struct {
	Limit int
}

func (f BackupRunFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxBackupRunLimit {
		return ErrInvalidBackupRunFilter
	}
	return nil
}

// BackupSummary is the state shown on the Backups page and the dashboard.
type BackupSummary struct {
	Configured  bool
	Initialized bool
	Running     *BackupRun
	LastRun     *BackupRun
	LastSuccess *BackupRun
	NextRunAt   *time.Time
}
