package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Bounds of the passkeys and security keys (SPEC_ADMINISTRATION.md § 3.3).
const (
	MaxWebAuthnCredentials = 10
	maxWebAuthnNameBytes   = 64
	// MaxWebAuthnResponseBytes bounds a response of the browser.
	MaxWebAuthnResponseBytes = 64 << 10
	// WebAuthnCeremonyTTL bounds a registration started and not finished.
	WebAuthnCeremonyTTL = 5 * time.Minute
)

var (
	ErrWebAuthnUnavailable   = errors.New("WebAuthn is not configured (webauthn_rp_id)")
	ErrInvalidWebAuthnName   = errors.New("the key name must be 1 to 64 characters")
	ErrTooManyWebAuthnKeys   = errors.New("an account has at most 10 keys")
	ErrWebAuthnKeyNotFound   = errors.New("security key not found")
	ErrNoWebAuthnCeremony    = errors.New("no key registration is pending")
	ErrInvalidWebAuthnAnswer = errors.New("the answer of the key is invalid")
	ErrWebAuthnKeyExists     = errors.New("this security key is already registered")
	// Passkey login (decision D-14).
	ErrPasskeyLoginDisabled  = errors.New("the security policy does not allow the login with a passkey alone")
	ErrInvalidPasskey        = errors.New("the passkey is unknown, invalid or its account cannot sign in")
	ErrPasskeyPasswordChange = errors.New("the account must change its password: sign in with it")
)

// WebAuthnCredential is a passkey or a security key of an account. Data is
// the credential as the WebAuthn adapter stores it (public key, sign
// count, flags); the domain does not read it.
type WebAuthnCredential struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Name         string
	CredentialID []byte
	Data         []byte
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// ValidWebAuthnName trims and checks the name of a key.
func ValidWebAuthnName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxWebAuthnNameBytes || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", ErrInvalidWebAuthnName
	}
	return name, nil
}
