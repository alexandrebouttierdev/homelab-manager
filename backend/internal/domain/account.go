package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds of the account fields (SPEC_ADMINISTRATION.md § 3.1).
const (
	MinPasswordBytes    = 12
	MaxPasswordBytes    = 1024
	MaxDisplayNameBytes = 128
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

var (
	ErrInvalidUsername    = errors.New("username must match ^[a-z0-9._-]{1,64}$")
	ErrInvalidDisplayName = errors.New("display name must be at most 128 bytes without control characters")
	ErrInvalidRole        = errors.New("role must be viewer, operator or admin")
	ErrInvalidPassword    = errors.New("password must be 12 to 1024 bytes")
	ErrEmptyChange        = errors.New("at least one field must change")
)

// NewUser is an account created by an administrator.
type NewUser struct {
	Username    string
	DisplayName string
	Role        Role
}

// Normalize validates the account and uses the username as display name
// when none is given.
func (n NewUser) Normalize() (NewUser, error) {
	if !usernamePattern.MatchString(n.Username) {
		return NewUser{}, ErrInvalidUsername
	}
	if !n.Role.Valid() {
		return NewUser{}, ErrInvalidRole
	}
	name, err := normalizeDisplayName(n.DisplayName)
	if err != nil {
		return NewUser{}, err
	}
	if name == "" {
		name = n.Username
	}
	n.DisplayName = name
	return n, nil
}

// UserChange lists the account fields to modify; nil leaves a field as is.
type UserChange struct {
	DisplayName *string
	Role        *Role
	Disabled    *bool
}

// Normalize validates the change; a display name cannot be blanked.
func (c UserChange) Normalize() (UserChange, error) {
	if c.DisplayName == nil && c.Role == nil && c.Disabled == nil {
		return UserChange{}, ErrEmptyChange
	}
	if c.Role != nil && !c.Role.Valid() {
		return UserChange{}, ErrInvalidRole
	}
	if c.DisplayName != nil {
		name, err := normalizeDisplayName(*c.DisplayName)
		if err != nil {
			return UserChange{}, err
		}
		if name == "" {
			return UserChange{}, ErrInvalidDisplayName
		}
		c.DisplayName = &name
	}
	return c, nil
}

// Apply returns u with the change applied.
func (c UserChange) Apply(u User) User {
	if c.DisplayName != nil {
		u.DisplayName = *c.DisplayName
	}
	if c.Role != nil {
		u.Role = *c.Role
	}
	if c.Disabled != nil {
		u.Disabled = *c.Disabled
	}
	return u
}

// IsActiveAdmin reports whether u can administer the Manager now.
func (u User) IsActiveAdmin() bool {
	return u.Role == RoleAdmin && !u.Disabled
}

// ValidateNewPassword checks the length of a password being set; logging
// in accepts shorter passwords such as the default admin/admin.
func ValidateNewPassword(password string) error {
	if len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

// IsValidationError reports whether err rejects user input.
func IsValidationError(err error) bool {
	for _, target := range []error{ErrInvalidUsername, ErrInvalidDisplayName, ErrInvalidRole, ErrInvalidPassword, ErrEmptyChange} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func normalizeDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if len(name) > MaxDisplayNameBytes || !utf8.ValidString(name) {
		return "", ErrInvalidDisplayName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidDisplayName
		}
	}
	return name, nil
}
