package domain

import (
	"strings"
	"testing"
)

func TestSystemPreferenceValues(t *testing.T) {
	for name, want := range map[string]bool{
		"monoops": true, "nas-01": true, "nas.home.lan": true, "a": true,
		"": false, "-nas": false, "nas-": false, "nas..lan": false, "nas_01": false, "nas lan": false,
		strings.Repeat("a", 63): true, strings.Repeat("a", 64): false,
		strings.Repeat("a.", 32) + "b": false,
	} {
		if ValidHostname(name) != want {
			t.Errorf("ValidHostname(%q) = %t", name, !want)
		}
	}
	for name, want := range map[string]bool{
		"Europe/Paris": true, "UTC": true, "America/Argentina/Buenos_Aires": true, "Etc/GMT+5": true,
		"": false, "../etc/passwd": false, "/Europe/Paris": false, "Europe/": false, "Europe Paris": false,
	} {
		if ValidTimezone(name) != want {
			t.Errorf("ValidTimezone(%q) = %t", name, !want)
		}
	}
	for name, want := range map[string]bool{
		"fr_FR.UTF-8": true, "en_US.utf8": true, "C.UTF-8": true, "POSIX": true, "de_DE@euro": true, "fr": true,
		"": false, "fr_FR.UTF-8;rm": false, "LANG=fr_FR": false, "FR_fr": false,
	} {
		if ValidLocale(name) != want {
			t.Errorf("ValidLocale(%q) = %t", name, !want)
		}
	}
	if err := (SystemPreferencesChange{Hostname: "nas", Timezone: "UTC", Locale: "C.UTF-8"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SystemPreferencesChange{Hostname: "nas", Timezone: "UTC", Locale: "x y"}).Validate(); err != ErrInvalidLocale {
		t.Fatalf("invalid locale = %v", err)
	}
}
