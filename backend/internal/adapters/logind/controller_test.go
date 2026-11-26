package logind

import (
	"context"
	"errors"
	"testing"

	"monoops/backend/internal/domain"

	"github.com/godbus/dbus/v5"
)

func TestTranslateClassifiesRefusals(t *testing.T) {
	for _, name := range []string{
		"org.freedesktop.DBus.Error.AccessDenied",
		"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired",
		"org.freedesktop.login1.OperationInProgress",
	} {
		if err := translate(dbus.Error{Name: name}); !errors.Is(err, domain.ErrPowerActionRefused) {
			t.Errorf("%s = %v, want ErrPowerActionRefused", name, err)
		}
	}
	other := translate(dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"})
	if other == nil || errors.Is(other, domain.ErrPowerActionRefused) {
		t.Fatalf("an unavailable logind = %v, want a plain failure", other)
	}
	if err := translate(nil); err != nil {
		t.Fatalf("translate(nil) = %v", err)
	}
}

// An unknown action must be refused before any connection to the system
// bus: this test never reaches logind.
func TestRequestRejectsUnknownActionsWithoutCallingLogind(t *testing.T) {
	if err := New().Request(context.Background(), "suspend"); !errors.Is(err, domain.ErrInvalidPowerAction) {
		t.Fatalf("Request(suspend) = %v, want ErrInvalidPowerAction", err)
	}
}
