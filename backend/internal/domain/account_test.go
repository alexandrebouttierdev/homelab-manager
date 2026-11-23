package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewUserNormalizeFillsTheDisplayName(t *testing.T) {
	user, err := NewUser{Username: "alice", DisplayName: "  ", Role: RoleViewer}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "alice" {
		t.Fatalf("display name = %q, want the username", user.DisplayName)
	}
	trimmed, err := NewUser{Username: "bob", DisplayName: "  Bob  ", Role: RoleAdmin}.Normalize()
	if err != nil || trimmed.DisplayName != "Bob" {
		t.Fatalf("Normalize = (%+v, %v), want trimmed display name", trimmed, err)
	}
}

func TestNewUserNormalizeRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input NewUser
		want  error
	}{
		{"uppercase username", NewUser{Username: "Alice", Role: RoleViewer}, ErrInvalidUsername},
		{"empty username", NewUser{Role: RoleViewer}, ErrInvalidUsername},
		{"username too long", NewUser{Username: strings.Repeat("a", 65), Role: RoleViewer}, ErrInvalidUsername},
		{"unknown role", NewUser{Username: "alice", Role: "root"}, ErrInvalidRole},
		{"control character", NewUser{Username: "alice", DisplayName: "Al\x00ice", Role: RoleViewer}, ErrInvalidDisplayName},
		{"display name too long", NewUser{Username: "alice", DisplayName: strings.Repeat("é", 65), Role: RoleViewer}, ErrInvalidDisplayName},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.input.Normalize()
			if !errors.Is(err, c.want) {
				t.Fatalf("Normalize error = %v, want %v", err, c.want)
			}
			if !IsValidationError(err) {
				t.Fatalf("%v should be a validation error", err)
			}
		})
	}
}

func TestUserChangeNormalize(t *testing.T) {
	if _, err := (UserChange{}).Normalize(); !errors.Is(err, ErrEmptyChange) {
		t.Fatalf("empty change error = %v, want ErrEmptyChange", err)
	}
	blank := "   "
	if _, err := (UserChange{DisplayName: &blank}).Normalize(); !errors.Is(err, ErrInvalidDisplayName) {
		t.Fatalf("blank display name error = %v, want ErrInvalidDisplayName", err)
	}
	root := Role("root")
	if _, err := (UserChange{Role: &root}).Normalize(); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("unknown role error = %v, want ErrInvalidRole", err)
	}
	padded := "  Alice "
	change, err := (UserChange{DisplayName: &padded}).Normalize()
	if err != nil || *change.DisplayName != "Alice" {
		t.Fatalf("Normalize = (%v, %v), want trimmed display name", change.DisplayName, err)
	}
}

func TestUserChangeApplyAndActiveAdmin(t *testing.T) {
	admin := User{Username: "admin", Role: RoleAdmin}
	if !admin.IsActiveAdmin() {
		t.Fatal("an enabled admin is an active admin")
	}
	viewer := RoleViewer
	if (UserChange{Role: &viewer}).Apply(admin).IsActiveAdmin() {
		t.Fatal("a demoted admin is no longer an active admin")
	}
	disabled := true
	if (UserChange{Disabled: &disabled}).Apply(admin).IsActiveAdmin() {
		t.Fatal("a disabled admin is no longer an active admin")
	}
	name := "Root"
	if got := (UserChange{DisplayName: &name}).Apply(admin); !got.IsActiveAdmin() || got.DisplayName != "Root" {
		t.Fatalf("renaming must keep the role: %+v", got)
	}
}

func TestValidateNewPassword(t *testing.T) {
	cases := []struct {
		password string
		valid    bool
	}{
		{strings.Repeat("a", 11), false},
		{strings.Repeat("a", 12), true},
		{strings.Repeat("a", 1024), true},
		{strings.Repeat("a", 1025), false},
	}
	for _, c := range cases {
		err := ValidateNewPassword(c.password)
		if (err == nil) != c.valid {
			t.Errorf("ValidateNewPassword(%d bytes) = %v, want valid=%v", len(c.password), err, c.valid)
		}
	}
}
