package packages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monoops/backend/internal/domain"
)

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const debianRelease = `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION_CODENAME=trixie
ID=debian
`

const dpkgStatus = `Package: libc6
Status: install ok installed
Priority: optional
Architecture: amd64
Multi-Arch: same
Source: glibc
Version: 2.41-12
Description: GNU C Library: Shared libraries
 Contains the standard libraries that are used by nearly all programs on
 the system.
Package: not-a-field-because-continuation

Package: openssl
Status: install ok installed
Architecture: amd64
Source: openssl (3.5.1-1)
Version: 3.5.1-1+b1

Package: removed-package
Status: deinstall ok config-files
Architecture: amd64
Version: 1.0

Package: unpacked
Status: install ok unpacked
Architecture: amd64
Version: 1.5

Package: purged
Status: purge ok not-installed
Version: 1.0

Package: held
Status: hold ok installed
Architecture: all
Version: 2.0

Package: no-version
Status: install ok installed
`

func TestDebianInventory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", debianRelease)
	writeFile(t, root, "var/lib/dpkg/status", dpkgStatus)
	inventory, err := New(root).Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() = %v", err)
	}
	if inventory.Distro.ID != "debian" || inventory.Distro.VersionID != "13" || inventory.Distro.PrettyName != "Debian GNU/Linux 13 (trixie)" ||
		inventory.Distro.VersionCodename != "trixie" || inventory.Manager != domain.PackageManagerDpkg {
		t.Fatalf("distro = %+v, manager %q", inventory.Distro, inventory.Manager)
	}
	want := []domain.InstalledPackage{
		{Name: "libc6", Version: "2.41-12", Architecture: "amd64", Source: "glibc"},
		{Name: "openssl", Version: "3.5.1-1+b1", Architecture: "amd64", Source: "openssl", SourceVersion: "3.5.1-1"},
		{Name: "unpacked", Version: "1.5", Architecture: "amd64"},
		{Name: "held", Version: "2.0", Architecture: "all"},
	}
	if len(inventory.Packages) != len(want) {
		t.Fatalf("packages = %+v", inventory.Packages)
	}
	for i := range want {
		if inventory.Packages[i] != want[i] {
			t.Errorf("package %d = %+v, want %+v", i, inventory.Packages[i], want[i])
		}
	}
}

func TestAlpineInventoryAndOSReleaseFallback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "usr/lib/os-release", "ID=alpine\nVERSION_ID=3.20.3\nPRETTY_NAME='Alpine Linux v3.20'\n")
	writeFile(t, root, "lib/apk/db/installed", "C:Q1abc=\nP:busybox\nV:1.36.1-r29\nA:x86_64\no:busybox\nT:Size optimized toolbox\n\nP:libcrypto3\nV:3.3.2-r0\nA:x86_64\no:openssl\n")
	inventory, err := New(root).Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() = %v", err)
	}
	if inventory.Manager != domain.PackageManagerApk || inventory.Distro.PrettyName != "Alpine Linux v3.20" || len(inventory.Packages) != 2 ||
		inventory.Packages[0].Source != "busybox" || inventory.Packages[1].Source != "openssl" {
		t.Fatalf("inventory = %+v", inventory)
	}
}

func TestUnsupportedAndMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := New(t.TempDir()).Inventory(context.Background()); err == nil {
		t.Fatal("Inventory() without os-release succeeded")
	}
	writeFile(t, root, "etc/os-release", debianRelease)
	if _, err := New(root).Inventory(context.Background()); err == nil {
		t.Fatal("Inventory() without the dpkg database succeeded")
	}
	writeFile(t, root, "etc/os-release", "NAME=nothing\n")
	if _, err := New(root).Inventory(context.Background()); err == nil {
		t.Fatal("Inventory() of an os-release without ID succeeded")
	}
}

func TestBoundsOfTheDatabase(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", debianRelease)
	writeFile(t, root, "var/lib/dpkg/status", "Package: a\nDescription: "+strings.Repeat("x", maxLineBytes+1)+"\n")
	if _, err := New(root).Inventory(context.Background()); !errors.Is(err, errTooLarge) {
		t.Fatalf("an overlong line = %v, want errTooLarge", err)
	}
	var many strings.Builder
	for i := range domain.MaxInstalledPackages + 1 {
		many.WriteString("Package: p" + strings.Repeat("x", i%3) + "\nStatus: install ok installed\nVersion: 1\n\n")
	}
	writeFile(t, root, "var/lib/dpkg/status", many.String())
	if _, err := New(root).Inventory(context.Background()); !errors.Is(err, errTooLarge) {
		t.Fatalf("too many packages = %v, want errTooLarge", err)
	}
	writeFile(t, root, "var/lib/dpkg/status", "Package: a\x07b\nStatus: install ok installed\nVersion: "+strings.Repeat("9", 300)+"\n")
	inventory, err := New(root).Inventory(context.Background())
	if err != nil || inventory.Packages[0].Name != "ab" || len(inventory.Packages[0].Version) != maxFieldBytes {
		t.Fatalf("cleaned package = (%+v, %v)", inventory.Packages, err)
	}
}
