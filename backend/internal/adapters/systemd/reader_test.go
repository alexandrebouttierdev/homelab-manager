package systemd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// maxSocketDir keeps the bus socket path (directory, "/dbus-", 10 random
// characters) under sun_path: 104 bytes on macOS, 108 on Linux.
const maxSocketDir = 80

// startBus runs a private dbus-daemon for the test and returns its address.
// The test is skipped when dbus-daemon is missing; CI installs it.
func startBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed")
	}
	dir := t.TempDir()
	if len(dir) > maxSocketDir {
		// macOS temporary directories are too long for a socket path.
		dir, err = os.MkdirTemp("/tmp", "hm-dbus-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	cmd := exec.Command(daemon, "--session", "--nofork", "--print-address=1", "--address=unix:dir="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()
	select {
	case address := <-lines:
		if address == "" {
			t.Fatal("dbus-daemon printed no address")
		}
		return address
	case <-time.After(10 * time.Second):
		t.Fatal("dbus-daemon did not start")
	}
	return ""
}

func connector(address string) Connector {
	return func(ctx context.Context) (*dbus.Conn, error) {
		return dbus.Connect(address, dbus.WithContext(ctx))
	}
}

// execCommand has the D-Bus signature of ExecStart, (sasbttttuii).
type execCommand struct {
	Path                                                       string
	Argv                                                       []string
	IgnoreFailure                                              bool
	StartRealtime, StartMonotonic, ExitRealtime, ExitMonotonic uint64
	PID                                                        uint32
	Code, Status                                               int32
}

type fakeUnit struct {
	listed listedUnit
	// unit and kind are the exported properties; a nil unit leaves the
	// object unexported, like a unit unloaded after ListUnits.
	unit, kind map[string]any
}

// fakeSystemd exports ListUnits and GetUnit only and records every call:
// the reader must never call anything else.
type fakeSystemd struct {
	mu    sync.Mutex
	calls []string
	units []fakeUnit
}

func (f *fakeSystemd) record(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method)
}

func (f *fakeSystemd) ListUnits() ([]listedUnit, *dbus.Error) {
	f.record("ListUnits")
	listed := make([]listedUnit, 0, len(f.units))
	for _, unit := range f.units {
		listed = append(listed, unit.listed)
	}
	return listed, nil
}

func (f *fakeSystemd) GetUnit(name string) (dbus.ObjectPath, *dbus.Error) {
	f.record("GetUnit")
	for _, unit := range f.units {
		if unit.listed.Name == name {
			return unit.listed.Path, nil
		}
	}
	return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", []any{"Unit " + name + " not loaded."})
}

func toPropMap(values map[string]any) map[string]*prop.Prop {
	props := make(map[string]*prop.Prop, len(values))
	for name, value := range values {
		props[name] = &prop.Prop{Value: value, Emit: prop.EmitFalse}
	}
	return props
}

// serve exports the fake systemd on the bus under org.freedesktop.systemd1.
func serve(t *testing.T, address string, fake *fakeSystemd) {
	t.Helper()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatalf("connect the fake systemd: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Export(fake, managerPath, managerInterface); err != nil {
		t.Fatal(err)
	}
	for _, unit := range fake.units {
		if unit.unit == nil {
			continue
		}
		interfaces := map[string]map[string]*prop.Prop{unitInterface: toPropMap(unit.unit)}
		if unit.kind != nil {
			interfaces[typeInterfaces[domain.UnitTypeOf(unit.listed.Name)]] = toPropMap(unit.kind)
		}
		if _, err := prop.Export(conn, unit.listed.Path, interfaces); err != nil {
			t.Fatal(err)
		}
	}
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request %s: %v (%v)", busName, err, reply)
	}
}

func listing(name, active, sub string) listedUnit {
	return listedUnit{
		Name: name, Description: "Listed " + name, LoadState: "loaded", ActiveState: active, SubState: sub,
		Path:    dbus.ObjectPath("/org/freedesktop/systemd1/unit/" + strings.NewReplacer(".", "_2e", "-", "_2d", "@", "_40").Replace(name)),
		JobPath: "/",
	}
}

const changedAt = uint64(1_795_400_000_000_000)

func sampleSystemd() *fakeSystemd {
	requires := make([]string, 150)
	for i := range requires {
		requires[i] = fmt.Sprintf("dep%03d.service", i)
	}
	return &fakeSystemd{units: []fakeUnit{
		{
			listed: listing("web.service", "active", "running"),
			unit: map[string]any{
				"Description": "Web server", "LoadState": "loaded", "ActiveState": "active", "SubState": "running",
				"UnitFileState": "enabled", "StateChangeTimestamp": changedAt, "Triggers": []string{},
				"FragmentPath": "/etc/systemd/system/web.service", "Requires": requires,
				"After": []string{"network.target"},
			},
			kind: map[string]any{
				"MainPID": uint32(812), "MemoryCurrent": uint64(52_428_800), "CPUUsageNSec": uint64(1_500_000_000),
				"Result": "success", "Restart": "on-failure", "Slice": "system.slice",
				"TasksMax": uint64(math.MaxUint64), "LimitNOFILE": uint64(1024),
				"ExecStart": []execCommand{{Path: "/usr/bin/web", Argv: []string{"/usr/bin/web", "--port", "8080"}}},
			},
		},
		{
			listed: listing("backup.service", "failed", "failed"),
			unit: map[string]any{
				"Description": "Backup", "LoadState": "loaded", "ActiveState": "failed", "SubState": "failed",
				"UnitFileState": "static", "StateChangeTimestamp": changedAt,
			},
			kind: map[string]any{
				"MainPID": uint32(0), "MemoryCurrent": uint64(math.MaxUint64), "CPUUsageNSec": uint64(math.MaxUint64),
				"Result": "exit-code",
			},
		},
		{
			listed: listing("backup.timer", "active", "waiting"),
			unit: map[string]any{
				"Description": "Daily backup", "LoadState": "loaded", "ActiveState": "active", "SubState": "waiting",
				"UnitFileState": "enabled", "StateChangeTimestamp": changedAt, "Triggers": []string{"backup.service"},
			},
			kind: map[string]any{
				"NextElapseUSecRealtime": changedAt + 3_600_000_000, "LastTriggerUSec": uint64(0), "Result": "success",
			},
		},
		{
			listed: listing("docker.socket", "active", "listening"),
			unit: map[string]any{
				"Description": "Docker socket", "LoadState": "loaded", "ActiveState": "active", "SubState": "listening",
				"Triggers": []string{"docker.service"},
			},
			kind: map[string]any{"Result": "success"},
		},
		// Unloaded between ListUnits and the property read.
		{listed: listing("gone.service", "inactive", "dead")},
		// Types the Manager does not read.
		{listed: listing("multi-user.target", "active", "active")},
		{listed: listing("home.mount", "active", "mounted")},
	}}
}

func TestUnitsReadsTheInventoryFailedFirst(t *testing.T) {
	address := startBus(t)
	fake := sampleSystemd()
	serve(t, address, fake)

	units, err := New(connector(address)).Units(context.Background())
	if err != nil {
		t.Fatalf("Units: %v", err)
	}
	names := make([]string, len(units))
	for i, unit := range units {
		names[i] = unit.Name
	}
	want := []string{"backup.service", "backup.timer", "docker.socket", "gone.service", "web.service"}
	if !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	failed := units[0]
	if failed.ActiveState != "failed" || failed.Result != "exit-code" || failed.UnitFileState != "static" {
		t.Errorf("failed unit = %+v", failed)
	}
	if failed.MainPID != nil || failed.MemoryBytes != nil || failed.CPUNanoseconds != nil {
		t.Errorf("absent values must be nil: pid %v, memory %v, cpu %v", failed.MainPID, failed.MemoryBytes, failed.CPUNanoseconds)
	}

	timer := units[1]
	if timer.Type != domain.UnitTypeTimer || timer.NextElapse == nil || timer.LastTrigger != nil ||
		!slices.Equal(timer.Triggers, []string{"backup.service"}) {
		t.Errorf("timer = %+v", timer)
	}
	if !timer.NextElapse.Equal(time.UnixMicro(int64(changedAt + 3_600_000_000))) {
		t.Errorf("next elapse = %v", timer.NextElapse)
	}

	socket := units[2]
	if socket.Type != domain.UnitTypeSocket || !slices.Equal(socket.Triggers, []string{"docker.service"}) {
		t.Errorf("socket = %+v", socket)
	}

	gone := units[3]
	if gone.Description != "Listed gone.service" || gone.ActiveState != "inactive" || gone.StateChangedAt != nil {
		t.Errorf("a vanished unit keeps its listing only: %+v", gone)
	}

	web := units[4]
	if web.MainPID == nil || *web.MainPID != 812 || web.MemoryBytes == nil || *web.MemoryBytes != 52_428_800 ||
		web.CPUNanoseconds == nil || *web.CPUNanoseconds != 1_500_000_000 || web.Description != "Web server" {
		t.Errorf("web = %+v", web)
	}
	if web.StateChangedAt == nil || !web.StateChangedAt.Equal(time.UnixMicro(int64(changedAt))) {
		t.Errorf("state change = %v", web.StateChangedAt)
	}
	assertReadOnly(t, fake)
}

func TestUnitReadsTheDetail(t *testing.T) {
	address := startBus(t)
	fake := sampleSystemd()
	serve(t, address, fake)

	detail, err := New(connector(address)).Unit(context.Background(), "web.service")
	if err != nil {
		t.Fatalf("Unit: %v", err)
	}
	if detail.FragmentPath != "/etc/systemd/system/web.service" || detail.Restart != "on-failure" ||
		detail.Slice != "system.slice" {
		t.Errorf("detail = %+v", detail)
	}
	if !slices.Equal(detail.ExecStart, []string{"/usr/bin/web --port 8080"}) {
		t.Errorf("ExecStart = %q", detail.ExecStart)
	}
	if len(detail.Requires) != domain.MaxUnitDependencies || detail.Requires[0] != "dep000.service" {
		t.Errorf("Requires = %d names, want %d", len(detail.Requires), domain.MaxUnitDependencies)
	}
	if !slices.Equal(detail.After, []string{"network.target"}) {
		t.Errorf("After = %v", detail.After)
	}
	if !detail.TasksMax.Unlimited || detail.TasksMax.Value != nil {
		t.Errorf("TasksMax = %+v, want unlimited", detail.TasksMax)
	}
	if detail.LimitNOFILE.Value == nil || *detail.LimitNOFILE.Value != 1024 {
		t.Errorf("LimitNOFILE = %+v", detail.LimitNOFILE)
	}
	if detail.MemoryMax.Unlimited || detail.MemoryMax.Value != nil {
		t.Errorf("MemoryMax not given = %+v, want unknown", detail.MemoryMax)
	}
	assertReadOnly(t, fake)
}

func TestUnitReportsAnUnknownUnit(t *testing.T) {
	address := startBus(t)
	fake := sampleSystemd()
	serve(t, address, fake)
	reader := New(connector(address))

	for _, name := range []string{"missing.service", "gone.service"} {
		if _, err := reader.Unit(context.Background(), name); !errors.Is(err, domain.ErrUnitNotFound) {
			t.Errorf("Unit(%s) = %v, want ErrUnitNotFound", name, err)
		}
	}
}

func TestReaderReportsABusWithoutSystemd(t *testing.T) {
	address := startBus(t)
	reader := New(connector(address))
	if _, err := reader.Units(context.Background()); !errors.Is(err, domain.ErrSystemdUnavailable) {
		t.Fatalf("Units = %v, want ErrSystemdUnavailable", err)
	}
	if _, err := reader.Unit(context.Background(), "ssh.service"); !errors.Is(err, domain.ErrSystemdUnavailable) {
		t.Fatalf("Unit = %v, want ErrSystemdUnavailable", err)
	}
}

func TestReaderReportsAnUnreachableBus(t *testing.T) {
	reader := New(func(context.Context) (*dbus.Conn, error) { return nil, errors.New("no such socket") })
	if _, err := reader.Units(context.Background()); !errors.Is(err, domain.ErrSystemdUnavailable) {
		t.Fatalf("Units = %v, want ErrSystemdUnavailable", err)
	}
}

func TestUnitRejectsAnInvalidNameWithoutConnecting(t *testing.T) {
	connected := false
	reader := New(func(context.Context) (*dbus.Conn, error) {
		connected = true
		return nil, errors.New("unexpected connection")
	})
	if _, err := reader.Unit(context.Background(), "ssh.mount"); !errors.Is(err, domain.ErrInvalidUnitName) || connected {
		t.Fatalf("Unit(ssh.mount) = %v (connected %v), want ErrInvalidUnitName", err, connected)
	}
}

func TestSelectUnitsKeepsFailuresWithinTheBound(t *testing.T) {
	listed := make([]listedUnit, 0, domain.MaxSystemUnits+10)
	for i := range domain.MaxSystemUnits + 5 {
		listed = append(listed, listedUnit{Name: fmt.Sprintf("a%05d.service", i), ActiveState: "active"})
	}
	for i := range 5 {
		listed = append(listed, listedUnit{Name: fmt.Sprintf("z%d.service", i), ActiveState: "failed"})
	}
	kept := selectUnits(listed)
	if len(kept) != domain.MaxSystemUnits {
		t.Fatalf("kept %d units, want %d", len(kept), domain.MaxSystemUnits)
	}
	for i := range 5 {
		if kept[i].ActiveState != "failed" {
			t.Fatalf("unit %d = %+v, want the failures first", i, kept[i])
		}
	}
}

func assertReadOnly(t *testing.T, fake *fakeSystemd) {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, call := range fake.calls {
		if call != "ListUnits" && call != "GetUnit" {
			t.Errorf("unexpected manager call %s", call)
		}
	}
}
