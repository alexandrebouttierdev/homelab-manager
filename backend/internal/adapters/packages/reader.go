// Package packages reads the distribution (/etc/os-release) and the
// installed system packages from the package database files, which every
// account may read: no command is run and no privilege is needed
// (SPEC_ADMINISTRATION.md § 8 ter.1).
package packages

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"
)

const (
	// maxDatabaseBytes bounds a package database file.
	maxDatabaseBytes = 64 << 20
	// maxOSReleaseBytes bounds /etc/os-release.
	maxOSReleaseBytes = 64 << 10
	// maxLineBytes bounds one line; dpkg descriptions and file lists stay
	// far below.
	maxLineBytes = 1 << 20
	// maxFieldBytes bounds a kept field (name, version, architecture).
	maxFieldBytes = 255
)

var (
	errTooLarge  = errors.New("package database too large")
	errMalformed = errors.New("package database malformed")
)

var _ ports.PackageInventory = (*Reader)(nil)

// Reader reads below root, "/" on the machine and a directory in tests.
type Reader struct {
	root string
}

func New(root string) *Reader { return &Reader{root: root} }

func (r *Reader) path(name string) string { return filepath.Join(r.root, name) }

// Inventory reads the distribution then its package database.
func (r *Reader) Inventory(ctx context.Context) (domain.PackageInventory, error) {
	distro, err := r.distro()
	if err != nil {
		return domain.PackageInventory{}, err
	}
	inventory := domain.PackageInventory{Distro: distro, Manager: distro.PackageManager()}
	if !distro.Supported() {
		return inventory, fmt.Errorf("%w: %s", domain.ErrUnsupportedDistro, distro.ID)
	}
	var file string
	var parse func(io.Reader) ([]domain.InstalledPackage, error)
	switch inventory.Manager {
	case domain.PackageManagerDpkg:
		file, parse = "var/lib/dpkg/status", parseDpkgStatus
	case domain.PackageManagerApk:
		file, parse = "lib/apk/db/installed", parseApkInstalled
	}
	if err := ctx.Err(); err != nil {
		return domain.PackageInventory{}, err
	}
	inventory.Packages, err = readBounded(r.path(file), maxDatabaseBytes, parse)
	if err != nil {
		return domain.PackageInventory{}, fmt.Errorf("read %s: %w", file, err)
	}
	return inventory, nil
}

func (r *Reader) distro() (domain.Distro, error) {
	var lastErr error
	for _, name := range []string{"etc/os-release", "usr/lib/os-release"} {
		distro, err := readBounded(r.path(name), maxOSReleaseBytes, parseOSRelease)
		if err == nil {
			return distro, nil
		}
		lastErr = err
	}
	return domain.Distro{}, fmt.Errorf("read os-release: %w", lastErr)
}

func readBounded[T any](path string, limit int64, parse func(io.Reader) (T, error)) (T, error) {
	var zero T
	file, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return zero, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return zero, errTooLarge
	}
	return parse(io.LimitReader(file, limit))
}

// parseOSRelease reads KEY=value lines, values optionally quoted
// (os-release(5)).
func parseOSRelease(reader io.Reader) (domain.Distro, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else {
			value = strings.Trim(value, `"'`)
		}
		values[key] = clean(value)
	}
	if err := scanner.Err(); err != nil {
		return domain.Distro{}, err
	}
	if values["ID"] == "" {
		return domain.Distro{}, fmt.Errorf("%w: os-release has no ID", errMalformed)
	}
	return domain.Distro{
		ID: strings.ToLower(values["ID"]), VersionID: values["VERSION_ID"], VersionCodename: values["VERSION_CODENAME"],
		Name: values["NAME"], PrettyName: values["PRETTY_NAME"], IDLike: strings.Fields(strings.ToLower(values["ID_LIKE"])),
	}, nil
}

// clean keeps a printable value of at most maxFieldBytes.
func clean(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(strings.TrimSpace(value), ""))
	if len(value) > maxFieldBytes {
		cut := maxFieldBytes
		for cut > 0 && !utf8.RuneStart(value[cut]) {
			cut--
		}
		value = value[:cut]
	}
	return value
}

// stanzas calls handle with the fields of each paragraph separated by an
// empty line; continuation lines (starting with a space) are skipped.
func stanzas(reader io.Reader, separator string, handle func(map[string]string) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	fields := map[string]string{}
	flush := func() error {
		if len(fields) == 0 {
			return nil
		}
		err := handle(fields)
		fields = map[string]string{}
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}
		key, value, ok := strings.Cut(line, separator)
		if !ok {
			continue
		}
		if _, seen := fields[key]; !seen {
			fields[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return errTooLarge
		}
		return err
	}
	return flush()
}

func appendPackage(packages []domain.InstalledPackage, pkg domain.InstalledPackage) ([]domain.InstalledPackage, error) {
	if pkg.Name == "" || pkg.Version == "" {
		return packages, nil
	}
	if len(packages) == domain.MaxInstalledPackages {
		return nil, errTooLarge
	}
	return append(packages, pkg), nil
}

// parseDpkgStatus reads /var/lib/dpkg/status as Syft does: every package
// whose files are on the disk, that is not removed ("deinstall") and not
// reduced to its configuration files. "Source: name (version)" gives the
// source package, kept even when it has the package name.
func parseDpkgStatus(reader io.Reader) ([]domain.InstalledPackage, error) {
	var packages []domain.InstalledPackage
	err := stanzas(reader, ":", func(fields map[string]string) error {
		status := strings.Fields(fields["Status"])
		if len(status) != 3 || status[0] == "deinstall" || status[0] == "purge" || status[2] == "not-installed" || status[2] == "config-files" {
			return nil
		}
		pkg := domain.InstalledPackage{
			Name: clean(fields["Package"]), Version: clean(fields["Version"]), Architecture: clean(fields["Architecture"]),
		}
		source := strings.TrimSpace(fields["Source"])
		if name, version, ok := strings.Cut(source, " "); ok {
			pkg.Source = clean(name)
			pkg.SourceVersion = clean(strings.Trim(strings.TrimSpace(version), "()"))
		} else {
			pkg.Source = clean(source)
		}
		var err error
		packages, err = appendPackage(packages, pkg)
		return err
	})
	return packages, err
}

// parseApkInstalled reads /lib/apk/db/installed: P (name), V (version),
// A (architecture), o (origin, the source package).
func parseApkInstalled(reader io.Reader) ([]domain.InstalledPackage, error) {
	var packages []domain.InstalledPackage
	err := stanzas(reader, ":", func(fields map[string]string) error {
		var err error
		packages, err = appendPackage(packages, domain.InstalledPackage{
			Name: clean(fields["P"]), Version: clean(fields["V"]), Architecture: clean(fields["A"]), Source: clean(fields["o"]),
		})
		return err
	})
	return packages, err
}
