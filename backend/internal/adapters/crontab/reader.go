// Package crontab reads the system crontabs, read-only
// (SPEC_ADMINISTRATION.md § 8): /etc/crontab, /etc/cron.d/* and the
// scripts of /etc/cron.hourly, daily, weekly and monthly. These files are
// readable by every account; the crontabs of the users
// (/var/spool/cron/crontabs) are not, and are not read.
package crontab

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

var _ ports.CronReader = (*Reader)(nil)

// runPartsName is the file name cron and run-parts accept: a file of
// /etc/cron.d with a dot ("x.dpkg-old") is ignored.
var runPartsName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var (
	envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\s*=`)
	periods       = []string{"hourly", "daily", "weekly", "monthly"}
)

// Reader reads the crontabs under a root directory ("/" in production).
type Reader struct {
	root string
}

func New(root string) *Reader {
	return &Reader{root: root}
}

func (r *Reader) path(parts ...string) string {
	return filepath.Join(append([]string{r.root}, parts...)...)
}

func (r *Reader) Read(ctx context.Context) (domain.CronInventory, error) {
	inventory := domain.CronInventory{Entries: []domain.CronEntry{}, Directories: []domain.CronScriptDirectory{}, Unreadable: []string{}}
	files := []string{"/etc/crontab"}
	names, err := os.ReadDir(r.path("etc", "cron.d"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		inventory.Unreadable = append(inventory.Unreadable, "/etc/cron.d")
	}
	for _, entry := range names {
		if runPartsName.MatchString(entry.Name()) && entry.Type().IsRegular() {
			files = append(files, "/etc/cron.d/"+entry.Name())
		}
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return domain.CronInventory{}, err
		}
		entries, err := r.readCrontab(file)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			inventory.Unreadable = append(inventory.Unreadable, file)
		}
		for _, entry := range entries {
			if len(inventory.Entries) >= domain.MaxCronEntries {
				break
			}
			inventory.Entries = append(inventory.Entries, entry)
		}
	}
	for _, period := range periods {
		directory := "/etc/cron." + period
		scripts, err := os.ReadDir(r.path("etc", "cron."+period))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			inventory.Unreadable = append(inventory.Unreadable, directory)
			continue
		}
		found := domain.CronScriptDirectory{Period: period, Path: directory, Scripts: []string{}}
		for _, script := range scripts {
			info, err := script.Info()
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || !runPartsName.MatchString(script.Name()) {
				continue
			}
			found.Scripts = append(found.Scripts, script.Name())
		}
		sort.Strings(found.Scripts)
		inventory.Directories = append(inventory.Directories, found)
	}
	if _, err := os.Stat(r.path("etc", "anacrontab")); err == nil {
		inventory.Anacron = true
	}
	return inventory, nil
}

// readCrontab reads a crontab of the system format: five fields (or a
// macro), the account, then the command.
func (r *Reader) readCrontab(name string) ([]domain.CronEntry, error) {
	file, err := os.Open(r.path(strings.TrimPrefix(name, "/")))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, domain.MaxCrontabBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > domain.MaxCrontabBytes {
		return nil, errors.New("crontab too large")
	}
	var entries []domain.CronEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 4096), domain.MaxCrontabBytes)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") || envAssignment.MatchString(text) {
			continue
		}
		scheduleFields := 5
		if strings.HasPrefix(text, "@") {
			scheduleFields = 1
		}
		fields, rest := splitFields(text, scheduleFields+1)
		entry := domain.CronEntry{Source: name, Line: line}
		if len(fields) < scheduleFields+1 || rest == "" {
			entry.Expression, entry.Command = text, ""
			entry.Error = "line without schedule, account and command"
			entries = append(entries, entry)
			continue
		}
		entry.Expression = strings.Join(fields[:scheduleFields], " ")
		entry.User = fields[scheduleFields]
		command := rest
		if len(command) > domain.MaxCronCommandBytes {
			command = command[:domain.MaxCronCommandBytes] + "…"
		}
		entry.Command = domain.MaskCronSecrets(strings.ToValidUTF8(command, "�"))
		schedule, err := domain.ParseCron(entry.Expression)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Schedule = schedule
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// splitFields returns the first n blank-separated fields of a line and the
// rest of it, trimmed.
func splitFields(text string, n int) ([]string, string) {
	fields := make([]string, 0, n)
	rest := text
	for len(fields) < n {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			fields = append(fields, rest)
			rest = ""
			break
		}
		fields = append(fields, rest[:end])
		rest = rest[end:]
	}
	return fields, strings.TrimSpace(rest)
}
