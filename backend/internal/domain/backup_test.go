package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateBackupRepository(t *testing.T) {
	valid := []BackupRepository{
		{RepositoryLocal, "/mnt/backup/restic"},
		{RepositoryRest, "https://backup.lan:8000/homelab"},
		{RepositoryS3, "https://s3.eu-central-003.backblazeb2.com/my-bucket/homelab"},
	}
	for _, repository := range valid {
		if err := ValidateBackupRepository(repository); err != nil {
			t.Errorf("%+v rejected: %v", repository, err)
		}
	}
	invalid := []BackupRepository{
		{RepositoryLocal, ""},
		{RepositoryLocal, "/"},
		{RepositoryLocal, "relative/path"},
		{RepositoryLocal, "/mnt/../etc"},
		{RepositoryLocal, "/mnt/backup/"},
		{RepositoryLocal, "/mnt/back\nup"},
		{RepositoryRest, "http://backup.lan:8000/"},
		{RepositoryRest, "https://user:pass@backup.lan/"},
		{RepositoryRest, "https://backup.lan/?x=1"},
		{RepositoryRest, "/mnt/backup"},
		{RepositoryS3, "https://s3.example.com"},
		{RepositoryS3, "https://s3.example.com/"},
		{"sftp", "sftp:user@host:/srv"},
	}
	for _, repository := range invalid {
		var validation *BackupValidationError
		if err := ValidateBackupRepository(repository); !errors.As(err, &validation) {
			t.Errorf("%+v accepted (%v)", repository, err)
		}
	}
}

func TestResticRepository(t *testing.T) {
	cases := map[BackupRepository]string{
		{RepositoryLocal, "/mnt/backup"}:         "/mnt/backup",
		{RepositoryRest, "https://backup.lan/r"}: "rest:https://backup.lan/r",
		{RepositoryS3, "https://s3.host/bucket"}: "s3:https://s3.host/bucket",
	}
	for repository, want := range cases {
		if got := repository.ResticRepository(); got != want {
			t.Errorf("%+v = %q, want %q", repository, got, want)
		}
	}
}

func TestValidateBackupCredentials(t *testing.T) {
	if err := ValidateBackupCredentials(RepositoryLocal, BackupCredentials{Password: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackupCredentials(RepositoryRest, BackupCredentials{Password: "p"}); err != nil {
		t.Fatalf("rest-server without user: %v", err)
	}
	if err := ValidateBackupCredentials(RepositoryS3, BackupCredentials{Password: "p", Username: "key", Secret: "secret"}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		kind        BackupRepositoryType
		credentials BackupCredentials
	}{
		"no password":       {RepositoryLocal, BackupCredentials{}},
		"local with user":   {RepositoryLocal, BackupCredentials{Password: "p", Username: "u"}},
		"s3 without secret": {RepositoryS3, BackupCredentials{Password: "p", Username: "key"}},
		"newline":           {RepositoryRest, BackupCredentials{Password: "p\nq"}},
		"too long":          {RepositoryLocal, BackupCredentials{Password: strings.Repeat("x", 1025)}},
	} {
		if err := ValidateBackupCredentials(tc.kind, tc.credentials); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestValidateBackupSelection(t *testing.T) {
	valid := BackupSelection{
		Sources:          []string{"/etc", "/var/lib/docker/volumes", "/opt/stacks"},
		Excludes:         []string{"*.tmp", "/var/lib/docker/volumes/*/_data/cache", "**/node_modules", "!keep.tmp"},
		ExcludeCaches:    true,
		ExcludeIfPresent: ".nobackup",
	}
	if err := ValidateBackupSelection(valid); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackupSelection(BackupSelection{}); err != nil {
		t.Fatalf("an empty selection is a valid draft: %v", err)
	}
	for name, selection := range map[string]BackupSelection{
		"relative source":   {Sources: []string{"etc"}},
		"dot-dot source":    {Sources: []string{"/etc/../root"}},
		"option source":     {Sources: []string{"--password-file=/x"}},
		"duplicate source":  {Sources: []string{"/etc", "/etc"}},
		"bad glob":          {Excludes: []string{"[a-"}},
		"comment":           {Excludes: []string{"#x"}},
		"multi-line":        {Excludes: []string{"a\nb"}},
		"marker with slash": {ExcludeIfPresent: "a/b"},
		"too many sources":  {Sources: manyPaths(MaxBackupSources + 1)},
	} {
		if err := ValidateBackupSelection(selection); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func manyPaths(count int) []string {
	paths := make([]string, count)
	for i := range paths {
		paths[i] = "/srv/" + strings.Repeat("a", i+1)
	}
	return paths
}

func TestBackupScheduleNext(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone database")
	}
	daily := BackupSchedule{Enabled: true, Frequency: BackupDaily, Hour: 3, Minute: 30}
	at := time.Date(2026, 11, 24, 2, 0, 0, 0, paris)
	if next, ok := daily.Next(at); !ok || !next.Equal(time.Date(2026, 11, 24, 3, 30, 0, 0, paris)) {
		t.Fatalf("daily before the hour = %v", next)
	}
	if next, _ := daily.Next(time.Date(2026, 11, 24, 3, 30, 0, 0, paris)); !next.Equal(time.Date(2026, 11, 25, 3, 30, 0, 0, paris)) {
		t.Fatalf("daily at the hour = %v, want the next day", next)
	}
	weekly := BackupSchedule{Enabled: true, Frequency: BackupWeekly, Weekday: time.Sunday, Hour: 4}
	if next, _ := weekly.Next(at); !next.Equal(time.Date(2026, 11, 29, 4, 0, 0, 0, paris)) {
		t.Fatalf("weekly = %v, want Sunday 29 at 04:00", next)
	}
	// The night of the switch to summer time keeps the wall clock.
	if next, _ := daily.Next(time.Date(2027, 3, 27, 12, 0, 0, 0, paris)); next.Hour() != 3 && next.Hour() != 4 {
		t.Fatalf("daylight saving = %v", next)
	}
	if _, ok := (BackupSchedule{Frequency: BackupDaily}).Next(at); ok {
		t.Fatal("a disabled schedule has no next run")
	}
}

func TestValidateBackupSchedule(t *testing.T) {
	if err := ValidateBackupSchedule(DefaultBackupSchedule); err != nil {
		t.Fatal(err)
	}
	for _, schedule := range []BackupSchedule{
		{Frequency: "hourly"},
		{Frequency: BackupDaily, Hour: 24},
		{Frequency: BackupDaily, Minute: -1},
		{Frequency: BackupWeekly, Weekday: 7},
	} {
		if err := ValidateBackupSchedule(schedule); err == nil {
			t.Errorf("%+v accepted", schedule)
		}
	}
}

func TestBackupConfigReady(t *testing.T) {
	now := time.Now()
	ready := BackupConfig{
		Repository:        &BackupRepository{RepositoryLocal, "/mnt/backup"},
		SealedCredentials: "sealed", InitializedAt: &now,
		Selection: BackupSelection{Sources: []string{"/etc"}},
	}
	if err := ready.Ready(); err != nil {
		t.Fatal(err)
	}
	notInitialized := ready
	notInitialized.InitializedAt = nil
	if err := notInitialized.Ready(); !errors.Is(err, ErrBackupNotInitialized) {
		t.Fatalf("not initialized = %v", err)
	}
	if err := (BackupConfig{}).Ready(); !errors.Is(err, ErrBackupNotConfigured) {
		t.Fatalf("empty = %v", err)
	}
	noSource := ready
	noSource.Selection.Sources = nil
	if err := noSource.Ready(); err == nil {
		t.Fatal("a backup without source must not run")
	}
}

func TestTruncateBackupMessage(t *testing.T) {
	long := strings.Repeat("é", MaxBackupMessageBytes)
	got := TruncateBackupMessage(long)
	if len(got) > MaxBackupMessageBytes+len("…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated to %d bytes", len(got))
	}
	if TruncateBackupMessage("  Fatal: wrong password \n") != "Fatal: wrong password" {
		t.Fatal("message not trimmed")
	}
}

func TestBackupOperationUnit(t *testing.T) {
	if BackupOpBackup.BackupUnit() != "homelab-backup@backup.service" || !ValidUnitName(BackupOpInit.BackupUnit()) {
		t.Fatal("unit name")
	}
	for _, op := range BackupOperations {
		if !op.Valid() || !ValidUnitName(op.BackupUnit()) {
			t.Errorf("%q should be a valid operation and unit", op)
		}
	}
	if BackupOperation("diff").Valid() || BackupOperation("copy").Valid() {
		t.Fatal("diff and copy are not operations of this version")
	}
}

func TestValidateBackupRetentionAndCheck(t *testing.T) {
	if err := ValidateBackupRetention(DefaultBackupRetention); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBackupRetention(BackupRetention{Enabled: false}); err != nil {
		t.Fatalf("a disabled empty policy is valid: %v", err)
	}
	for _, retention := range []BackupRetention{
		{Enabled: true},
		{KeepDaily: -1},
		{KeepYearly: 1001},
	} {
		if err := ValidateBackupRetention(retention); err == nil {
			t.Errorf("%+v accepted", retention)
		}
	}
	if err := ValidateBackupCheck(DefaultBackupCheck); err != nil {
		t.Fatal(err)
	}
	if ValidateBackupCheck(BackupCheck{SubsetPercent: 0}) == nil || ValidateBackupCheck(BackupCheck{SubsetPercent: 101}) == nil {
		t.Fatal("subset out of bounds accepted")
	}
}

func TestValidateBackupRestore(t *testing.T) {
	if err := ValidateBackupRestore(BackupRestore{SnapshotID: "3a253ad8", Include: []string{"/etc/hostname"}}); err != nil {
		t.Fatal(err)
	}
	for _, restore := range []BackupRestore{
		{SnapshotID: "latest"},
		{SnapshotID: "3A253AD8"},
		{SnapshotID: "--target=/"},
		{SnapshotID: "3a253ad8", Include: []string{"etc"}},
		{SnapshotID: "3a253ad8", Include: []string{"/etc/../root"}},
	} {
		if err := ValidateBackupRestore(restore); err == nil {
			t.Errorf("%+v accepted", restore)
		}
	}
	id := uuid.MustParse("7c9e6679-7425-40de-944b-e07fc1f90ae7")
	if RestoreTarget(id) != "/var/lib/homelab-backup/restore/7c9e6679-7425-40de-944b-e07fc1f90ae7" {
		t.Fatal(RestoreTarget(id))
	}
}
