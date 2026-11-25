package crontab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// debianCrontab is /etc/crontab of Debian 13.
const debianCrontab = `# /etc/crontab: system-wide crontab
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin

# Example of job definition:
# m h dom mon dow user	command
17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
25 6	* * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }
47 6	* * 7	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.weekly; }
52 6	1 * *	root	test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.monthly; }
#
`

func write(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestReadTheSystemCrontabs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/crontab", debianCrontab, 0o644)
	write(t, root, "etc/cron.d/e2scrub_all", "30 3 * * 0 root test -e /run/systemd/system || SERVICE_MODE=1 /usr/lib/x86_64-linux-gnu/e2fsprogs/e2scrub_all_cron\n10 3 * * * root test -e /run/systemd/system || SERVICE_MODE=1 /sbin/e2scrub_all -A -r\n", 0o644)
	write(t, root, "etc/cron.d/backup", "MAILTO=\"\"\n@daily   alice  /opt/backup.sh --password=hunter2 token='abc def' --verbose\n@reboot root /usr/local/bin/warm-up\n0 0 31 2 * root never\n61 * * * * root broken\n", 0o644)
	write(t, root, "etc/cron.d/backup.dpkg-old", "* * * * * root ignored\n", 0o644)
	write(t, root, "etc/cron.daily/logrotate", "#!/bin/sh\n", 0o755)
	write(t, root, "etc/cron.daily/dpkg", "#!/bin/sh\n", 0o755)
	write(t, root, "etc/cron.daily/README", "not executable\n", 0o644)
	write(t, root, "etc/cron.daily/apt.disabled", "#!/bin/sh\n", 0o755)
	write(t, root, "etc/anacrontab", "", 0o644)

	inventory, err := New(root).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Four in /etc/crontab, four in backup, two in e2scrub_all.
	if len(inventory.Entries) != 10 {
		t.Fatalf("%d entries, want 10: %+v", len(inventory.Entries), inventory.Entries)
	}
	hourly := inventory.Entries[0]
	if hourly.Source != "/etc/crontab" || hourly.Line != 7 || hourly.Expression != "17 * * * *" || hourly.User != "root" ||
		hourly.Command != "cd / && run-parts --report /etc/cron.hourly" || !hourly.Valid() {
		t.Fatalf("hourly = %+v", hourly)
	}
	var daily, reboot, never, broken bool
	for _, entry := range inventory.Entries {
		switch {
		case entry.User == "alice":
			daily = true
			if entry.Expression != "@daily" || strings.Contains(entry.Command, "hunter2") || strings.Contains(entry.Command, "abc def") ||
				entry.Command != "/opt/backup.sh --password=•••• token=•••• --verbose" {
				t.Errorf("masked command = %q", entry.Command)
			}
		case entry.Expression == "@reboot":
			reboot = entry.Schedule.Reboot
		case entry.Command == "never":
			never = entry.Valid()
		case entry.Command == "broken":
			broken = !entry.Valid() && entry.Error != ""
		case strings.Contains(entry.Command, "ignored"):
			t.Error("a file with a dot in its name is read")
		}
	}
	if !daily || !reboot || !never || !broken {
		t.Fatalf("daily %v reboot %v never %v broken %v", daily, reboot, never, broken)
	}
	if len(inventory.Directories) != 1 || inventory.Directories[0].Period != "daily" ||
		strings.Join(inventory.Directories[0].Scripts, ",") != "dpkg,logrotate" {
		t.Fatalf("directories = %+v", inventory.Directories)
	}
	if !inventory.Anacron || len(inventory.Unreadable) != 0 {
		t.Fatalf("anacron %v unreadable %v", inventory.Anacron, inventory.Unreadable)
	}
}

func TestReadWithoutCron(t *testing.T) {
	inventory, err := New(t.TempDir()).Read(context.Background())
	if err != nil || len(inventory.Entries) != 0 || len(inventory.Directories) != 0 || inventory.Anacron {
		t.Fatalf("empty machine = %+v, %v", inventory, err)
	}
}

func TestReadReportsAnUnreadableCrontab(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	root := t.TempDir()
	write(t, root, "etc/crontab", debianCrontab, 0o000)
	write(t, root, "etc/cron.d/big", strings.Repeat("# padding\n", 7000), 0o644)
	inventory, err := New(root).Read(context.Background())
	if err != nil || len(inventory.Unreadable) != 2 {
		t.Fatalf("unreadable = %v, %v", inventory.Unreadable, err)
	}
}
