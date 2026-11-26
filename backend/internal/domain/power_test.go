package domain

import "testing"

func TestPowerActionValid(t *testing.T) {
	for _, action := range []PowerAction{PowerReboot, PowerPowerOff} {
		if !action.Valid() {
			t.Errorf("%q should be valid", action)
		}
	}
	for _, action := range []PowerAction{"", "suspend", "halt", "Reboot"} {
		if action.Valid() {
			t.Errorf("%q should be invalid", action)
		}
	}
}

func TestConfirmsHostname(t *testing.T) {
	cases := []struct {
		confirmation, hostname string
		want                   bool
	}{
		{"monoops", "monoops", true},
		{"  monoops ", "monoops", true},
		{"Monoops", "monoops", false},
		{"monoops.local", "monoops", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := ConfirmsHostname(c.confirmation, c.hostname); got != c.want {
			t.Errorf("ConfirmsHostname(%q, %q) = %v, want %v", c.confirmation, c.hostname, got, c.want)
		}
	}
}
