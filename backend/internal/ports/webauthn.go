package ports

import (
	"context"
	"time"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

// WebAuthnVerifier runs the WebAuthn ceremonies (SPEC_ADMINISTRATION.md
// § 3.3). Options are the JSON sent to the browser, sessions the opaque
// state kept by the Manager between the two steps of a ceremony.
type WebAuthnVerifier interface {
	BeginRegistration(user domain.User, existing []domain.WebAuthnCredential) (options, session []byte, err error)
	// FinishRegistration checks the answer of the browser and returns the
	// identifier and the stored form of the new credential, or
	// domain.ErrInvalidWebAuthnAnswer.
	FinishRegistration(user domain.User, existing []domain.WebAuthnCredential, session, response []byte) (credentialID, data []byte, err error)
	BeginLogin(user domain.User, existing []domain.WebAuthnCredential) (options, session []byte, err error)
	// FinishLogin checks an assertion and returns the identifier of the
	// credential used and its stored form updated (sign count), or
	// domain.ErrInvalidWebAuthnAnswer.
	FinishLogin(user domain.User, existing []domain.WebAuthnCredential, session, response []byte) (credentialID, data []byte, err error)
	// BeginPasskeyLogin starts a login without a user name: the browser
	// offers the passkeys it holds for the site (decision D-14).
	BeginPasskeyLogin() (options, session []byte, err error)
	// FinishPasskeyLogin checks the assertion of a passkey. lookup returns
	// the account of the user handle and its keys. It returns that account,
	// the identifier of the credential used and its stored form updated, or
	// domain.ErrInvalidWebAuthnAnswer.
	FinishPasskeyLogin(session, response []byte, lookup PasskeyLookup) (user domain.User, credentialID, data []byte, err error)
}

// PasskeyLookup finds the account of a user handle, with its keys.
type PasskeyLookup func(userHandle []byte) (domain.User, []domain.WebAuthnCredential, error)

// WebAuthnRepository stores the passkeys and security keys of the accounts.
type WebAuthnRepository interface {
	WebAuthnCredentials(ctx context.Context, userID uuid.UUID) ([]domain.WebAuthnCredential, error)
	// AddWebAuthnCredential stores a key and its audit entry; recoveryHashes,
	// when not nil, replace the recovery codes in the same transaction.
	// domain.ErrTooManyWebAuthnKeys past domain.MaxWebAuthnCredentials.
	AddWebAuthnCredential(ctx context.Context, credential domain.WebAuthnCredential, recoveryHashes []string, audit domain.AuditEntry) error
	// UseWebAuthnCredential stores the credential updated by a login.
	UseWebAuthnCredential(ctx context.Context, id uuid.UUID, data []byte, at time.Time) error
	// DeleteWebAuthnCredential removes a key of the account; with no second
	// factor left, the recovery codes go too.
	DeleteWebAuthnCredential(ctx context.Context, userID, id uuid.UUID, audit domain.AuditEntry) error
}
