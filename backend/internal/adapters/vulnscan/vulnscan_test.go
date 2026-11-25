package vulnscan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/dbustest"
	"homelab-manager/backend/internal/adapters/packages"
	"homelab-manager/backend/internal/domain"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

var ubuntu = domain.Distro{
	ID: "ubuntu", VersionID: "24.04", VersionCodename: "noble", Name: "Ubuntu", PrettyName: "Ubuntu 24.04.5 LTS", IDLike: []string{"debian"},
}

// Packages and PURLs as Syft v1.57.1 wrote them for the same dpkg base.
var syftPURLs = []struct {
	pkg  domain.InstalledPackage
	purl string
}{
	{domain.InstalledPackage{Name: "libc6", Version: "2.39-0ubuntu8.12", Architecture: "amd64", Source: "glibc"},
		"pkg:deb/ubuntu/libc6@2.39-0ubuntu8.12?arch=amd64&distro=ubuntu-24.04&upstream=glibc"},
	{domain.InstalledPackage{Name: "bison", Version: "2:3.8.2+dfsg-1ubuntu0.24.04.1", Architecture: "amd64"},
		"pkg:deb/ubuntu/bison@2%3A3.8.2%2Bdfsg-1ubuntu0.24.04.1?arch=amd64&distro=ubuntu-24.04"},
	{domain.InstalledPackage{Name: "bsdutils", Version: "1:2.39.3-9ubuntu6.6", Architecture: "amd64", Source: "util-linux", SourceVersion: "2.39.3-9ubuntu6.6"},
		"pkg:deb/ubuntu/bsdutils@1%3A2.39.3-9ubuntu6.6?arch=amd64&distro=ubuntu-24.04&upstream=util-linux%402.39.3-9ubuntu6.6"},
	{domain.InstalledPackage{Name: "clang", Version: "1:18.0-59~exp2", Architecture: "amd64", Source: "llvm-defaults", SourceVersion: "0.59~exp2"},
		"pkg:deb/ubuntu/clang@1%3A18.0-59~exp2?arch=amd64&distro=ubuntu-24.04&upstream=llvm-defaults%400.59~exp2"},
	{domain.InstalledPackage{Name: "g++", Version: "4:13.2.0-7ubuntu1", Architecture: "amd64", Source: "gcc-defaults", SourceVersion: "1.214ubuntu1"},
		"pkg:deb/ubuntu/g%2B%2B@4%3A13.2.0-7ubuntu1?arch=amd64&distro=ubuntu-24.04&upstream=gcc-defaults%401.214ubuntu1"},
}

func TestInventoryFollowsTheSyftFormat(t *testing.T) {
	inventory := domain.PackageInventory{Distro: ubuntu, Manager: domain.PackageManagerDpkg}
	for _, item := range syftPURLs {
		inventory.Packages = append(inventory.Packages, item.pkg)
	}
	document, err := syftInventory(inventory, "2.9.0")
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range syftPURLs {
		if got := document.Artifacts[i].PURL; got != item.purl {
			t.Errorf("purl %d = %s, want %s", i, got, item.purl)
		}
	}
	libc := document.Artifacts[0]
	metadata := libc.Metadata.(dpkgEntry)
	if libc.Type != "deb" || libc.MetadataType != "dpkg-db-entry" || metadata.Source != "glibc" || metadata.Architecture != "amd64" || len(libc.ID) != 16 {
		t.Fatalf("libc6 artifact = %+v", libc)
	}
	if document.Distro.ID != "ubuntu" || document.Distro.VersionID != "24.04" || document.Source.Type != "directory" || document.Schema.Version != syftSchemaVersion {
		t.Fatalf("document = %+v %+v", document.Distro, document.Source)
	}
	apk, err := syftInventory(domain.PackageInventory{Distro: domain.Distro{ID: "alpine", VersionID: "3.20.3"}, Manager: domain.PackageManagerApk, Packages: []domain.InstalledPackage{
		{Name: "busybox", Version: "1.36.1-r29", Architecture: "x86_64", Source: "busybox"},
		{Name: "libcrypto3", Version: "3.3.2-r0", Architecture: "x86_64", Source: "openssl"},
	}}, "dev")
	if err != nil || apk.Artifacts[0].PURL != "pkg:apk/alpine/busybox@1.36.1-r29?arch=x86_64&distro=alpine-3.20.3" ||
		apk.Artifacts[1].PURL != "pkg:apk/alpine/libcrypto3@3.3.2-r0?arch=x86_64&distro=alpine-3.20.3&upstream=openssl" ||
		apk.Artifacts[1].Metadata.(apkEntry).OriginPackage != "openssl" {
		t.Fatalf("apk inventory = %+v, %v", apk.Artifacts, err)
	}
	one := 1
	rpm, err := syftInventory(domain.PackageInventory{Distro: domain.Distro{ID: "rocky", VersionID: "9.4"}, Manager: domain.PackageManagerRPM, Packages: []domain.InstalledPackage{
		{Name: "openssl-libs", Version: "3.0.7", Release: "27.el9", Epoch: &one, Architecture: "x86_64", Source: "openssl-3.0.7-27.el9.src.rpm"},
		{Name: "bash", Version: "5.1.8", Release: "9.el9", Architecture: "x86_64", Source: "bash-5.1.8-9.el9.src.rpm"},
		{Name: "nodejs", Version: "18.20.2", Release: "1.module+el9.4.0+21731+46b5c9f6", Architecture: "x86_64", Source: "nodejs-18.20.2-1.module+el9.4.0+21731+46b5c9f6.src.rpm", ModularityLabel: "nodejs:18:9040020240509:rhel9"},
	}}, "dev")
	if err != nil {
		t.Fatal(err)
	}
	ssl, bash, node := rpm.Artifacts[0], rpm.Artifacts[1], rpm.Artifacts[2]
	if ssl.Type != "rpm" || ssl.MetadataType != "rpm-db-entry" || ssl.Version != "1:3.0.7-27.el9" ||
		ssl.PURL != "pkg:rpm/rocky/openssl-libs@3.0.7-27.el9?arch=x86_64&distro=rocky-9.4&epoch=1&upstream=openssl-3.0.7-27.el9.src.rpm" {
		t.Fatalf("openssl-libs artifact = %+v", ssl)
	}
	sslMetadata := ssl.Metadata.(rpmEntry)
	if sslMetadata.Version != "3.0.7" || sslMetadata.Release != "27.el9" || *sslMetadata.Epoch != 1 ||
		sslMetadata.SourceRpm != "openssl-3.0.7-27.el9.src.rpm" || sslMetadata.ModularityLabel != nil {
		t.Fatalf("openssl-libs metadata = %+v", sslMetadata)
	}
	if bash.Version != "5.1.8-9.el9" || bash.Metadata.(rpmEntry).Epoch != nil || strings.Contains(bash.PURL, "epoch=") {
		t.Fatalf("bash artifact = %+v", bash)
	}
	if label := node.Metadata.(rpmEntry).ModularityLabel; label == nil || *label != "nodejs:18:9040020240509:rhel9" {
		t.Fatalf("nodejs metadata = %+v", node.Metadata)
	}
	encoded, _ := json.Marshal(bash)
	if !strings.Contains(string(encoded), `"epoch":null`) {
		t.Fatalf("an RPM without epoch keeps a null epoch, as Syft writes it: %s", encoded)
	}
	redhat, _ := syftInventory(domain.PackageInventory{Distro: domain.Distro{ID: "rhel", VersionID: "9.4"}, Manager: domain.PackageManagerRPM, Packages: []domain.InstalledPackage{
		{Name: "bash", Version: "5.1.8", Release: "9.el9", Architecture: "x86_64"},
	}}, "dev")
	if redhat.Artifacts[0].PURL != "pkg:rpm/redhat/bash@5.1.8-9.el9?arch=x86_64&distro=rhel-9.4" {
		t.Fatalf("rhel PURL = %s", redhat.Artifacts[0].PURL)
	}
	if _, err := syftInventory(domain.PackageInventory{Distro: domain.Distro{ID: "arch"}, Packages: inventory.Packages}, "dev"); !errors.Is(err, domain.ErrUnsupportedDistro) {
		t.Fatalf("unknown package manager = %v", err)
	}
}

// TestSyftDecodesTheInventory runs "syft convert" on the inventory when
// HOMELAB_TEST_SYFT names a Syft binary: Grype decodes "sbom:" documents
// with the same code.
func TestSyftDecodesTheInventory(t *testing.T) {
	syft := os.Getenv("HOMELAB_TEST_SYFT")
	if syft == "" {
		t.Skip("HOMELAB_TEST_SYFT is not set")
	}
	dir := t.TempDir()
	inventory := domain.PackageInventory{Distro: ubuntu, Manager: domain.PackageManagerDpkg}
	for _, item := range syftPURLs {
		inventory.Packages = append(inventory.Packages, item.pkg)
	}
	path := filepath.Join(dir, "inventory.json")
	if err := writeInventory(path, inventory, "dev"); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(syft, "convert", path, "-o", "syft-json", "-q").Output()
	if err != nil {
		t.Fatalf("syft convert: %v", err)
	}
	var decoded struct {
		Artifacts []struct {
			Name, Version, Type, PURL string
			Metadata                  struct{ Source, SourceVersion string }
		}
		Distro struct{ ID, VersionID string }
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Artifacts) != len(syftPURLs) || decoded.Distro.ID != "ubuntu" || decoded.Distro.VersionID != "24.04" {
		t.Fatalf("decoded = %+v", decoded)
	}
	for _, artifact := range decoded.Artifacts {
		if artifact.Type != "deb" || artifact.PURL == "" {
			t.Errorf("artifact = %+v", artifact)
		}
	}
}

func TestWriteInventoryIsAtomicAndGroupReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	if err := writeInventory(path, domain.PackageInventory{Distro: ubuntu, Manager: domain.PackageManagerDpkg, Packages: []domain.InstalledPackage{{Name: "a", Version: "1"}}}, "dev"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("inventory mode = %v, %v", info, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d files, want only the inventory", len(entries))
	}
}

const grypeResult = `{
 "matches": [
  {"vulnerability": {"id": "CVE-2025-0001", "dataSource": "https://ubuntu.com/security/CVE-2025-0001", "namespace": "ubuntu:distro:ubuntu:24.04",
    "severity": "High", "urls": ["https://ubuntu.com/security/CVE-2025-0001"], "description": "Overflow.", "cvss": [],
    "fix": {"versions": ["2.39-0ubuntu8.13"], "state": "fixed"}, "advisories": [], "risk": 1.2},
   "relatedVulnerabilities": [], "matchDetails": [],
   "artifact": {"id": "x", "name": "libc6", "version": "2.39-0ubuntu8.12", "type": "deb", "locations": [], "language": "", "licenses": [], "cpes": [], "purl": "", "upstreams": [{"name": "glibc"}]}},
  {"vulnerability": {"id": "CVE-2025-0002", "severity": "Negligible", "urls": [], "fix": {"versions": [], "state": "wont-fix"}},
   "artifact": {"name": "bash", "version": "5.2.21-2ubuntu4", "type": "deb"}}
 ],
 "source": {"type": "sbom", "target": {}},
 "distro": {"name": "ubuntu", "version": "24.04"},
 "descriptor": {"name": "grype", "version": "0.121.3", "db": {"status": {"schemaVersion": "v6.1.3", "built": "2026-11-24T04:12:00Z"}}}
}`

func TestDecodeResult(t *testing.T) {
	result, err := decodeResult(strings.NewReader(grypeResult))
	if err != nil {
		t.Fatalf("decodeResult() = %v", err)
	}
	if result.GrypeVersion != "0.121.3" || result.DBBuiltAt == nil || !result.DBBuiltAt.Equal(time.Date(2026, 11, 24, 4, 12, 0, 0, time.UTC)) || len(result.Matches) != 2 {
		t.Fatalf("result = %+v", result)
	}
	first := result.Matches[0]
	if first.Severity != domain.VulnHigh || first.FixState != domain.FixAvailable || first.FixedVersions[0] != "2.39-0ubuntu8.13" ||
		first.Package != "libc6" || first.PackageType != "deb" || first.DataSource == "" {
		t.Fatalf("first match = %+v", first)
	}
	if second := result.Matches[1]; second.Severity != domain.VulnNegligible || second.FixState != domain.FixWontFix {
		t.Fatalf("second match = %+v", second)
	}
	for name, invalid := range map[string]string{
		"not json":       "{",
		"no matches":     `{"descriptor": {"name": "grype"}}`,
		"other tool":     `{"matches": [], "descriptor": {"name": "trivy"}}`,
		"wrong type":     `{"matches": [{"vulnerability": {"id": 5}}], "descriptor": {"name": "grype"}}`,
		"trailing value": `{"matches": [], "descriptor": {"name": "grype"}} {}`,
	} {
		_, err := decodeResult(strings.NewReader(invalid))
		var scanErr *domain.ScanError
		if !errors.As(err, &scanErr) || scanErr.Failure != domain.FailureInvalidResult {
			t.Errorf("%s: decodeResult() = %v, want invalid_result", name, err)
		}
	}
}

func TestReadResultRefusesAStaleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	var scanErr *domain.ScanError
	if _, err := readResult(path, time.Now()); !errors.As(err, &scanErr) || scanErr.Failure != domain.FailureStaleResult {
		t.Fatalf("missing result = %v, want stale_result", err)
	}
	if err := os.WriteFile(path, []byte(grypeResult), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := readResult(path, time.Now().Add(-time.Minute)); !errors.As(err, &scanErr) || scanErr.Failure != domain.FailureStaleResult {
		t.Fatalf("old result = %v, want stale_result", err)
	}
	if result, err := readResult(path, old.Add(-time.Second)); err != nil || len(result.Matches) != 2 {
		t.Fatalf("fresh result = (%+v, %v)", result, err)
	}
}

// fakeSystemd exports StartUnit and GetUnit; StartUnit creates a job that
// disappears after jobLife, then sets the unit state.
type fakeSystemd struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	calls    []string
	refuse   bool
	jobLife  time.Duration
	active   string
	result   string
	unitName string
	// unloaded: GetUnit answers NoSuchUnit, as systemd does once it has
	// unloaded a static unit that ended inactive.
	unloaded bool
}

func (f *fakeSystemd) StartUnit(name, mode string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	f.calls = append(f.calls, "StartUnit "+name+" "+mode)
	f.mu.Unlock()
	if f.refuse {
		return "", dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"denied"})
	}
	if name != f.unitName {
		return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", []any{"no unit"})
	}
	job := dbus.ObjectPath("/org/freedesktop/systemd1/job/1")
	props, err := prop.Export(f.conn, job, prop.Map{jobInterface: {"State": {Value: "running"}}})
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	_ = props
	go func() {
		time.Sleep(f.jobLife)
		f.conn.Export(nil, job, "org.freedesktop.DBus.Properties")
	}()
	return job, nil
}

func (f *fakeSystemd) GetUnit(name string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	f.calls = append(f.calls, "GetUnit "+name)
	f.mu.Unlock()
	if f.unloaded {
		return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", []any{"unit not loaded"})
	}
	return "/org/freedesktop/systemd1/unit/homelab_2dvulnscan_2eservice", nil
}

func serveSystemd(t *testing.T, fake *fakeSystemd) Connector {
	t.Helper()
	address := dbustest.StartBus(t, nil)
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fake.conn = conn
	if err := conn.Export(fake, managerPath, managerInterface); err != nil {
		t.Fatal(err)
	}
	if _, err := prop.Export(conn, "/org/freedesktop/systemd1/unit/homelab_2dvulnscan_2eservice", prop.Map{
		unitInterface:                      {"ActiveState": {Value: fake.active}},
		"org.freedesktop.systemd1.Service": {"Result": {Value: fake.result}},
	}); err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	return func(ctx context.Context) (*dbus.Conn, error) { return dbus.Connect(address, dbus.WithContext(ctx)) }
}

func runUnit(t *testing.T, fake *fakeSystemd, unit string, timeout time.Duration) error {
	t.Helper()
	starter := NewUnitStarter(serveSystemd(t, fake))
	starter.poll = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return starter.Run(ctx, unit)
}

func failure(err error) domain.ScanFailure {
	var scanErr *domain.ScanError
	if errors.As(err, &scanErr) {
		return scanErr.Failure
	}
	return ""
}

func TestUnitStarterWaitsForTheStartJob(t *testing.T) {
	fake := &fakeSystemd{unitName: DefaultUnit, jobLife: 150 * time.Millisecond, active: "inactive", result: "success"}
	started := time.Now()
	if err := runUnit(t, fake, DefaultUnit, 5*time.Second); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if time.Since(started) < 150*time.Millisecond {
		t.Fatal("Run() returned before the start job ended")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.calls) != 2 || fake.calls[0] != "StartUnit homelab-vulnscan.service replace" || fake.calls[1] != "GetUnit homelab-vulnscan.service" {
		t.Fatalf("calls = %v, want StartUnit then GetUnit only", fake.calls)
	}
}

// A static Type=oneshot unit that ends inactive is unloaded by systemd as
// soon as its job is gone: GetUnit then answers NoSuchUnit. Seen on Debian
// 13 with Grype 0.121.3, where every scan was reported "not installed"
// although Grype had written its result.
func TestUnitStarterAcceptsAUnitUnloadedAfterItsRun(t *testing.T) {
	fake := &fakeSystemd{unitName: DefaultUnit, jobLife: 10 * time.Millisecond, unloaded: true}
	if err := runUnit(t, fake, DefaultUnit, 5*time.Second); err != nil {
		t.Fatalf("Run() = %v, want nil: the unit ran and was unloaded", err)
	}
}

func TestUnitStarterReportsEachFailure(t *testing.T) {
	for name, test := range map[string]struct {
		fake    *fakeSystemd
		unit    string
		timeout time.Duration
		want    domain.ScanFailure
	}{
		"polkit refusal": {&fakeSystemd{unitName: DefaultUnit, refuse: true}, DefaultUnit, 5 * time.Second, domain.FailureNotAllowed},
		"no such unit":   {&fakeSystemd{unitName: "other.service"}, DefaultUnit, 5 * time.Second, domain.FailureNotInstalled},
		"unit failed":    {&fakeSystemd{unitName: DefaultUnit, jobLife: 10 * time.Millisecond, active: "failed", result: "exit-code"}, DefaultUnit, 5 * time.Second, domain.FailureScan},
		"unit timed out": {&fakeSystemd{unitName: DefaultUnit, jobLife: 10 * time.Millisecond, active: "failed", result: "timeout"}, DefaultUnit, 5 * time.Second, domain.FailureTimeout},
		"wait exceeded":  {&fakeSystemd{unitName: DefaultUnit, jobLife: time.Minute, active: "inactive", result: "success"}, DefaultUnit, 200 * time.Millisecond, domain.FailureTimeout},
	} {
		if got := failure(runUnit(t, test.fake, test.unit, test.timeout)); got != test.want {
			t.Errorf("%s: failure = %q, want %q", name, got, test.want)
		}
	}
	unreachable := NewUnitStarter(func(context.Context) (*dbus.Conn, error) { return nil, errors.New("no bus") })
	if got := failure(unreachable.Run(context.Background(), DefaultUnit)); got != domain.FailureNotInstalled {
		t.Fatalf("unreachable bus = %q, want vulnscan_not_installed", got)
	}
}

type fakeRunner struct {
	err   error
	write func()
}

func (f fakeRunner) Run(context.Context, string) error {
	if f.write != nil {
		f.write()
	}
	return f.err
}

func TestScannerWritesRunsAndReads(t *testing.T) {
	dir := t.TempDir()
	config := Config{Unit: DefaultUnit, InventoryPath: filepath.Join(dir, "inventory.json"), ResultPath: filepath.Join(dir, "result.json"), Timeout: time.Minute, Version: "dev"}
	inventory := domain.PackageInventory{Distro: ubuntu, Manager: domain.PackageManagerDpkg, Packages: []domain.InstalledPackage{{Name: "libc6", Version: "1"}}}
	scanner := NewScanner(config, fakeRunner{write: func() {
		if _, err := os.Stat(config.InventoryPath); err != nil {
			t.Errorf("inventory not written before the unit: %v", err)
		}
		_ = os.WriteFile(config.ResultPath, []byte(grypeResult), 0o600)
	}})
	result, err := scanner.Scan(context.Background(), inventory)
	if err != nil || len(result.Matches) != 2 {
		t.Fatalf("Scan() = (%+v, %v)", result, err)
	}
	failing := NewScanner(config, fakeRunner{err: &domain.ScanError{Failure: domain.FailureNotAllowed}})
	if _, err := failing.Scan(context.Background(), inventory); failure(err) != domain.FailureNotAllowed {
		t.Fatalf("Scan() with a refused unit = %v", err)
	}
	config.InventoryPath = filepath.Join(dir, "missing", "inventory.json")
	if _, err := NewScanner(config, fakeRunner{}).Scan(context.Background(), inventory); failure(err) != domain.FailureInventory {
		t.Fatalf("Scan() without the exchange directory = %v", err)
	}
}

// TestInventoryMatchesSyftOnARealDatabase compares, package by package, the
// inventory with the one Syft builds from the same files. It runs when
// HOMELAB_TEST_SYFT names a Syft binary and HOMELAB_TEST_ROOT a directory
// holding etc/os-release and var/lib/dpkg/status (or lib/apk/db/installed).
func TestInventoryMatchesSyftOnARealDatabase(t *testing.T) {
	syft, root := os.Getenv("HOMELAB_TEST_SYFT"), os.Getenv("HOMELAB_TEST_ROOT")
	if syft == "" || root == "" {
		t.Skip("HOMELAB_TEST_SYFT and HOMELAB_TEST_ROOT are not set")
	}
	inventory, err := packages.New(root).Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ours, err := syftInventory(inventory, "dev")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(syft, "scan", "dir:"+root, "-o", "syft-json", "-q").Output()
	if err != nil {
		t.Fatalf("syft scan: %v", err)
	}
	var theirs struct {
		Artifacts []struct {
			Name, Version, PURL string
			Metadata            map[string]any
		}
	}
	if err := json.Unmarshal(output, &theirs); err != nil {
		t.Fatal(err)
	}
	type key struct{ name, version string }
	expected := map[key]string{}
	for _, artifact := range theirs.Artifacts {
		expected[key{artifact.Name, artifact.Version}] = artifact.PURL
	}
	if len(expected) != len(ours.Artifacts) {
		t.Errorf("Syft found %d packages, the inventory %d", len(expected), len(ours.Artifacts))
	}
	for _, artifact := range ours.Artifacts {
		purl, ok := expected[key{artifact.Name, artifact.Version}]
		if !ok {
			t.Errorf("%s %s is not in the Syft inventory", artifact.Name, artifact.Version)
			continue
		}
		if purl != artifact.PURL {
			t.Errorf("%s: purl %s, Syft %s", artifact.Name, artifact.PURL, purl)
		}
	}
	t.Logf("%d packages compared", len(ours.Artifacts))
}
