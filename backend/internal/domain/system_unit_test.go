package domain

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidUnitName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"ssh.service", true},
		{"getty@tty1.service", true},
		{"apt-daily.timer", true},
		{"docker.socket", true},
		{"systemd-fsck@dev-disk-by\\x2duuid.service", false},
		{"dbus-org.freedesktop.login1.service", true},
		{"a:b_c.service", true},
		{".service", false},
		{"ssh", false},
		{"ssh.mount", false},
		{"ssh.target", false},
		{"ssh service", false},
		{"../etc.service", false},
		{"ssh.service\n", false},
		{strings.Repeat("a", 255) + ".service", true},
		{strings.Repeat("a", 256) + ".service", false},
	} {
		if got := ValidUnitName(tc.name); got != tc.want {
			t.Errorf("ValidUnitName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnitTypeOfName(t *testing.T) {
	for name, want := range map[string]SystemUnitType{
		"ssh.service": UnitTypeService, "apt-daily.timer": UnitTypeTimer,
		"docker.socket": UnitTypeSocket, "-.mount": "", "multi-user.target": "",
	} {
		if got := UnitTypeOf(name); got != want {
			t.Errorf("UnitTypeOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAbsentValuesAreNilNeverZero(t *testing.T) {
	if OptionalCounter(math.MaxUint64) != nil {
		t.Error("UINT64_MAX (accounting disabled) must be absent")
	}
	if got := OptionalCounter(0); got == nil || *got != 0 {
		t.Errorf("a real zero must stay zero, got %v", got)
	}
	if OptionalTimestamp(0) != nil || OptionalTimestamp(math.MaxUint64) != nil {
		t.Error("timestamps 0 and UINT64_MAX must be absent")
	}
	at := OptionalTimestamp(1_700_000_000_123_456)
	if at == nil || !at.Equal(time.UnixMicro(1_700_000_000_123_456)) || at.Location() != time.UTC {
		t.Errorf("timestamp = %v, want UTC microseconds", at)
	}
	if OptionalPID(0) != nil {
		t.Error("PID 0 (no main process) must be absent")
	}
	if pid := OptionalPID(812); pid == nil || *pid != 812 {
		t.Errorf("PID = %v", pid)
	}
}

func TestUnitLimitTellsUnlimitedFromUnknown(t *testing.T) {
	unlimited := UnitLimitFrom(math.MaxUint64, true)
	if !unlimited.Unlimited || unlimited.Value != nil {
		t.Errorf("infinity = %+v", unlimited)
	}
	unknown := UnitLimitFrom(0, false)
	if unknown.Unlimited || unknown.Value != nil {
		t.Errorf("unknown = %+v", unknown)
	}
	set := UnitLimitFrom(4096, true)
	if set.Unlimited || set.Value == nil || *set.Value != 4096 {
		t.Errorf("4096 = %+v", set)
	}
}

func TestCountUnits(t *testing.T) {
	units := []SystemUnit{
		{Name: "a.service", Type: UnitTypeService, ActiveState: "active"},
		{Name: "b.service", Type: UnitTypeService, ActiveState: "failed"},
		{Name: "c.service", Type: UnitTypeService, ActiveState: "inactive"},
		{Name: "d.timer", Type: UnitTypeTimer, ActiveState: "active"},
		{Name: "e.timer", Type: UnitTypeTimer, ActiveState: "inactive"},
		{Name: "f.socket", Type: UnitTypeSocket, ActiveState: "active"},
	}
	got := CountUnits(units)
	want := SystemUnitCounts{Total: 6, Active: 3, Failed: 1, ActiveTimers: 1}
	if got != want {
		t.Fatalf("CountUnits = %+v, want %+v", got, want)
	}
}

func failedSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

func TestUnitFailureTransitions(t *testing.T) {
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	previous := failedSet("recovered.service", "still-failed.service", "gone.service", "restarting.service")
	units := []SystemUnit{
		{Name: "new-failure.service", ActiveState: "failed", SubState: "failed", Result: "exit-code"},
		{Name: "still-failed.service", ActiveState: "failed", SubState: "failed", Result: "exit-code"},
		{Name: "recovered.service", ActiveState: "active", SubState: "running", Result: "success"},
		{Name: "restarting.service", ActiveState: "activating", SubState: "auto-restart", Result: "exit-code"},
		{Name: "healthy.service", ActiveState: "active", SubState: "running", Result: "success"},
	}

	got := UnitFailureTransitions(previous, units, at)

	if !slices.Equal(got.Added, []string{"new-failure.service"}) {
		t.Errorf("Added = %v", got.Added)
	}
	if !slices.Equal(got.Removed, []string{"gone.service", "recovered.service"}) {
		t.Errorf("Removed = %v", got.Removed)
	}
	if len(got.Events) != 2 {
		t.Fatalf("Events = %+v, want two", got.Events)
	}
	failed, recovered := got.Events[0], got.Events[1]
	if failed.Type != EventServiceFailed || failed.Severity != "error" || failed.ContainerID != nil ||
		!failed.OccurredAt.Equal(at) || failed.Message == "" {
		t.Errorf("failed event = %+v", failed)
	}
	wantMetadata := map[string]any{
		"unit": "new-failure.service", "activeState": "failed", "subState": "failed", "result": "exit-code",
	}
	for key, value := range wantMetadata {
		if failed.Metadata[key] != value {
			t.Errorf("failed metadata[%s] = %v, want %v", key, failed.Metadata[key], value)
		}
	}
	if recovered.Type != EventServiceRecovered || recovered.Severity != "info" ||
		recovered.Metadata["unit"] != "recovered.service" || recovered.Metadata["activeState"] != "active" {
		t.Errorf("recovered event = %+v", recovered)
	}
	if failed.ID == recovered.ID {
		t.Error("events need distinct identifiers")
	}
}

func TestUnitFailureTransitionsWithoutChange(t *testing.T) {
	got := UnitFailureTransitions(failedSet("a.service"), []SystemUnit{
		{Name: "a.service", ActiveState: "failed"},
		{Name: "b.service", ActiveState: "inactive"},
	}, time.Now())
	if len(got.Added) != 0 || len(got.Removed) != 0 || len(got.Events) != 0 {
		t.Fatalf("no transition expected, got %+v", got)
	}
}
