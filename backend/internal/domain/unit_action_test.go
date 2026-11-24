package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func TestUnitVerbValid(t *testing.T) {
	for _, verb := range UnitVerbs {
		if !verb.Valid() {
			t.Errorf("%q should be valid", verb)
		}
	}
	for _, verb := range []UnitVerb{"", "enable", "daemon-reload", "kill", "mask", "Start", "reload-or-restart"} {
		if verb.Valid() {
			t.Errorf("%q should be refused", verb)
		}
	}
}

func TestNewManageableUnits(t *testing.T) {
	units, err := NewManageableUnits([]string{" nginx.service", "backup.timer", "nginx.service", "", "getty@tty1.service"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := units.Names(), []string{"backup.timer", "getty@tty1.service", "nginx.service"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if !units.Contains("nginx.service") || units.Contains("ssh.service") {
		t.Fatal("Contains does not follow the list")
	}
	empty, err := NewManageableUnits(nil)
	if err != nil || len(empty.Names()) != 0 || empty.Contains("nginx.service") {
		t.Fatalf("empty list = %v, %v", empty.Names(), err)
	}
}

func TestNewManageableUnitsRefusesInvalidNames(t *testing.T) {
	for _, name := range []string{
		"nginx", "nginx.mount", "../nginx.service", "a b.service", `dev-disk-by\x2duuid.service`,
		"getty@.service", "nginx.service;rm", "\"nginx.service\"",
	} {
		if _, err := NewManageableUnits([]string{name}); err == nil {
			t.Errorf("%q should be refused", name)
		}
	}
}

func TestNewManageableUnitsIsBounded(t *testing.T) {
	names := make([]string, MaxManageableUnits+1)
	for i := range names {
		names[i] = fmt.Sprintf("unit%d.service", i)
	}
	if _, err := NewManageableUnits(names[:MaxManageableUnits]); err != nil {
		t.Fatalf("%d units should be accepted: %v", MaxManageableUnits, err)
	}
	if _, err := NewManageableUnits(names); err == nil {
		t.Fatal("one unit over the bound should be refused")
	}
}
