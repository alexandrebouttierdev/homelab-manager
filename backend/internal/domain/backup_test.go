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
		{Type: RepositoryLocal, Location: "/mnt/backup/restic"},
		{Type: RepositoryRest, Location: "https://backup.lan:8000/homelab"},
		{Type: RepositoryS3, Location: "https://s3.eu-central-003.backblazeb2.com/my-bucket/homelab"},
	}
	for _, repository := range valid {
		if err := ValidateBackupRepository(repository); err != nil {
			t.Errorf("%+v rejected: %v", repository, err)
		}
	}
	invalid := []BackupRepository{
		{Type: RepositoryLocal, Location: ""},
		{Type: RepositoryLocal, Location: "/"},
		{Type: RepositoryLocal, Location: "relative/path"},
		{Type: RepositoryLocal, Location: "/mnt/../etc"},
		{Type: RepositoryLocal, Location: "/mnt/backup/"},
		{Type: RepositoryLocal, Location: "/mnt/back\nup"},
		{Type: RepositoryRest, Location: "http://backup.lan:8000/"},
		{Type: RepositoryRest, Location: "https://user:pass@backup.lan/"},
		{Type: RepositoryRest, Location: "https://backup.lan/?x=1"},
		{Type: RepositoryRest, Location: "/mnt/backup"},
		{Type: RepositoryS3, Location: "https://s3.example.com"},
		{Type: RepositoryS3, Location: "https://s3.example.com/"},
		{Type: RepositorySFTP, Location: "sftp:user@host:/srv", HostKey: testHostKey},
		{Type: RepositoryLocal, Location: "/mnt/backup", HostKey: testHostKey},
		{Type: "ftp", Location: "/mnt/backup"},
	}
	for _, repository := range invalid {
		var validation *BackupValidationError
		if err := ValidateBackupRepository(repository); !errors.As(err, &validation) {
			t.Errorf("%+v accepted (%v)", repository, err)
		}
	}
}

const testHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"

func TestValidateSFTPRepository(t *testing.T) {
	for _, location := range []string{
		"sftp://backup@nas.lan/srv/restic",
		"sftp://backup@nas.lan:2222/srv/restic/homelab",
		"sftp://u_1@192.168.1.10/volume1/backup",
		"sftp://backup@[fd00::10]:22/srv/restic",
	} {
		if err := ValidateBackupRepository(BackupRepository{Type: RepositorySFTP, Location: location, HostKey: testHostKey}); err != nil {
			t.Errorf("%s rejected: %v", location, err)
		}
	}
	for _, location := range []string{
		"sftp://nas.lan/srv/restic",               // no user
		"sftp://backup:pass@nas.lan/srv/restic",   // password
		"sftp://-oProxyCommand=x@nas.lan/srv",     // option as user
		"sftp://backup@-oProxyCommand=x/srv",      // option as host
		"sftp://backup@nas.lan:0/srv/restic",      // port
		"sftp://backup@nas.lan:99999/srv/restic",  // port
		"sftp://backup@nas.lan:022/srv/restic",    // port
		"sftp://backup@nas.lan/",                  // root
		"sftp://backup@nas.lan",                   // no path
		"sftp://backup@nas.lan/srv/../etc",        // not clean
		"sftp://backup@nas.lan/srv/a%20b",         // escape
		"sftp://backup@nas.lan/srv/a b",           // blank
		"sftp://backup@nas.lan/srv/restic?x=1",    // query
		"sftp://backup@nas.lan/srv/restic#x",      // fragment
		"https://backup@nas.lan/srv/restic",       // scheme
		"sftp://backup@nas_lan/srv/restic",        // host
		"sftp://backup@fe80::1%25eth0/srv/restic", // zone
	} {
		var validation *BackupValidationError
		if err := ValidateBackupRepository(BackupRepository{Type: RepositorySFTP, Location: location, HostKey: testHostKey}); !errors.As(err, &validation) || validation.Field != "location" {
			t.Errorf("%s accepted (%v)", location, err)
		}
	}
	for _, hostKey := range []string{"", "ssh-ed25519 AAAA\nssh-rsa BBBB", strings.Repeat("a", MaxBackupHostKeyBytes+1)} {
		var validation *BackupValidationError
		err := ValidateBackupRepository(BackupRepository{Type: RepositorySFTP, Location: "sftp://b@nas.lan/srv/r", HostKey: hostKey})
		if !errors.As(err, &validation) || validation.Field != "hostKey" {
			t.Errorf("host key %.20q accepted (%v)", hostKey, err)
		}
	}
	if err := ValidateBackupCredentials(RepositorySFTP, BackupCredentials{Password: "p", Username: "u"}); err == nil {
		t.Fatal("SFTP takes no access keys")
	}
}

func TestSFTPEndpointAndSameLocation(t *testing.T) {
	repository := BackupRepository{Type: RepositorySFTP, Location: "sftp://backup@[fd00::10]:2222/srv/r", HostKey: testHostKey}
	user, host, port, ok := repository.SFTPEndpoint()
	if !ok || user != "backup" || host != "fd00::10" || port != "2222" {
		t.Fatalf("endpoint = %q %q %q %v", user, host, port, ok)
	}
	if _, _, _, ok := (BackupRepository{Type: RepositoryLocal, Location: "/srv"}).SFTPEndpoint(); ok {
		t.Fatal("endpoint of a local repository")
	}
	other := repository
	other.HostKey = "ssh-ed25519 other"
	if !repository.SameLocation(other) {
		t.Fatal("a new host key changes the repository")
	}
	other.Location = "sftp://backup@[fd00::10]:2222/srv/s"
	if repository.SameLocation(other) {
		t.Fatal("another path is the same repository")
	}
	config := BackupConfig{Repository: &BackupRepository{Type: RepositoryLocal, Location: "/srv"}}
	if config.UsesSFTP(BackupOpBackup) {
		t.Fatal("local repository uses SFTP")
	}
	config.Copy = &BackupCopyTarget{Repository: repository}
	if config.UsesSFTP(BackupOpBackup) || !config.UsesSFTP(BackupOpCopy) {
		t.Fatal("an SFTP copy repository is used by the copy only")
	}
}

func TestResticRepository(t *testing.T) {
	cases := map[BackupRepository]string{
		{Type: RepositorySFTP, Location: "sftp://backup@nas.lan:2222/srv/restic", HostKey: testHostKey}: "sftp://backup@nas.lan:2222//srv/restic",
		{Type: RepositorySFTP, Location: "sftp://backup@[fd00::1]/srv/restic", HostKey: testHostKey}:    "sftp://backup@[fd00::1]//srv/restic",
		{Type: RepositoryLocal, Location: "/mnt/backup"}:                                                "/mnt/backup",
		{Type: RepositoryRest, Location: "https://backup.lan/r"}:                                        "rest:https://backup.lan/r",
		{Type: RepositoryS3, Location: "https://s3.host/bucket"}:                                        "s3:https://s3.host/bucket",
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
		Repository:        &BackupRepository{Type: RepositoryLocal, Location: "/mnt/backup"},
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
	if BackupOperation("mount").Valid() || BackupOperation("").Valid() {
		t.Fatal("mount is not an operation")
	}
}

func TestValidateBackupDiff(t *testing.T) {
	if err := ValidateBackupDiff(BackupDiffRequest{From: "3a253ad8", To: "9f2c11b0"}); err != nil {
		t.Fatal(err)
	}
	for _, request := range []BackupDiffRequest{
		{From: "3a253ad8", To: "3a253ad8"},
		{From: "latest", To: "9f2c11b0"},
		{From: "3a253ad8", To: "--help"},
		{From: "3a253ad8:/etc", To: "9f2c11b0"},
	} {
		if err := ValidateBackupDiff(request); err == nil {
			t.Errorf("%+v accepted", request)
		}
	}
}

func TestValidateBackupCopy(t *testing.T) {
	main := BackupRepository{Type: RepositoryLocal, Location: "/srv/backup"}
	if err := ValidateBackupCopy(main, BackupRepository{Type: RepositoryRest, Location: "https://nas.lan:8000/copy"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	var validation *BackupValidationError
	err := ValidateBackupCopy(main, BackupRepository{Type: RepositoryLocal, Location: "/srv/backup"}, nil, nil)
	if !errors.As(err, &validation) || validation.Field != "copy.location" {
		t.Fatalf("same repository = %v", err)
	}
	err = ValidateBackupCopy(main, BackupRepository{Type: RepositoryLocal, Location: "relative"}, nil, nil)
	if !errors.As(err, &validation) || validation.Field != "copy.location" {
		t.Fatalf("invalid copy = %v", err)
	}
	s3 := func(bucket string) BackupRepository {
		return BackupRepository{Type: RepositoryS3, Location: "https://s3.example.com/" + bucket}
	}
	one := &BackupCredentials{Password: "a", Username: "key", Secret: "secret"}
	other := &BackupCredentials{Password: "b", Username: "key2", Secret: "secret2"}
	if err := ValidateBackupCopy(s3("a"), s3("b"), one, one); err != nil {
		t.Fatalf("same S3 key: %v", err)
	}
	if err := ValidateBackupCopy(s3("a"), s3("b"), one, other); !errors.As(err, &validation) || validation.Field != "copy.username" {
		t.Fatalf("two S3 keys = %v", err)
	}
}

func TestCopyReady(t *testing.T) {
	now := time.Now()
	config := BackupConfig{Repository: &BackupRepository{Type: RepositoryLocal, Location: "/srv/b"}, InitializedAt: &now, SealedCredentials: "s"}
	if !errors.Is(config.CopyReady(), ErrBackupCopyNotConfigured) {
		t.Fatal("no copy")
	}
	config.Copy = &BackupCopyTarget{Repository: BackupRepository{Type: RepositoryLocal, Location: "/mnt/copy"}}
	if !errors.Is(config.CopyReady(), ErrBackupCopyNotInitialized) {
		t.Fatal("copy not initialized")
	}
	config.Copy.InitializedAt, config.Copy.SealedCredentials = &now, "s"
	if err := config.CopyReady(); err != nil {
		t.Fatal(err)
	}
	config.InitializedAt = nil
	if !errors.Is(config.CopyReady(), ErrBackupNotInitialized) {
		t.Fatal("main repository not initialized")
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

func TestBackupSQLiteDatabases(t *testing.T) {
	valid := BackupSelection{Sources: []string{"/srv"}, SQLiteDatabases: []string{"/srv/app/data.db", "/var/lib/docker/volumes/vault/_data/db.sqlite3"}}
	if err := ValidateBackupSelection(valid); err != nil {
		t.Fatal(err)
	}
	for _, databases := range [][]string{
		{"relative.db"}, {"/srv/../etc/shadow"}, {"/srv/app.db", "/srv/app.db"}, {BackupDumpRoot + "/srv/app.db"}, {"/srv/app/"},
	} {
		if err := ValidateBackupSelection(BackupSelection{SQLiteDatabases: databases}); err == nil {
			t.Errorf("%q accepted", databases)
		}
	}
	many := make([]string, MaxBackupSQLiteDatabases+1)
	for i := range many {
		many[i] = "/srv/app" + string(rune('a'+i)) + ".db"
	}
	if err := ValidateBackupSelection(BackupSelection{SQLiteDatabases: many}); err == nil {
		t.Fatal("too many databases accepted")
	}
	if got := BackupDumpPath("/dumps", "/srv/app/data.db"); got != "/dumps/srv/app/data.db" {
		t.Fatalf("dump path = %s", got)
	}
}
