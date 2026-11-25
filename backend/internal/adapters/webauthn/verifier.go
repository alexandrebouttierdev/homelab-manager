// Package webauthn runs the WebAuthn ceremonies with
// github.com/go-webauthn/webauthn (SPEC_ADMINISTRATION.md § 3.3): passkeys
// and FIDO2 security keys as a second factor, user verification required,
// no attestation requested.
package webauthn

import (
	"encoding/json"
	"errors"
	"fmt"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/go-webauthn/webauthn/protocol"
	library "github.com/go-webauthn/webauthn/webauthn"
)

var _ ports.WebAuthnVerifier = (*Verifier)(nil)

// Config is the relying party: its identifier (the host name of the
// interface) and the origins the browser may answer from.
type Config struct {
	RPID          string
	RPDisplayName string
	Origins       []string
}

type Verifier struct {
	webauthn *library.WebAuthn
}

func New(config Config) (*Verifier, error) {
	timeout := library.TimeoutConfig{Enforce: true, Timeout: domain.WebAuthnCeremonyTTL, TimeoutUVD: domain.WebAuthnCeremonyTTL}
	webauthn, err := library.New(&library.Config{
		RPID: config.RPID, RPDisplayName: config.RPDisplayName, RPOrigins: config.Origins,
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationRequired,
		},
		Timeouts: library.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("configure WebAuthn: %w", err)
	}
	return &Verifier{webauthn: webauthn}, nil
}

// account adapts an account to the user of the library: its handle is
// the UUID of the account, never its name.
type account struct {
	user        domain.User
	credentials []library.Credential
}

func (a account) WebAuthnID() []byte { return a.user.ID[:] }

func (a account) WebAuthnName() string { return a.user.Username }

func (a account) WebAuthnDisplayName() string {
	if a.user.DisplayName != "" {
		return a.user.DisplayName
	}
	return a.user.Username
}

func (a account) WebAuthnCredentials() []library.Credential { return a.credentials }

func newAccount(user domain.User, existing []domain.WebAuthnCredential) (account, error) {
	credentials := make([]library.Credential, 0, len(existing))
	for _, stored := range existing {
		var credential library.Credential
		if err := json.Unmarshal(stored.Data, &credential); err != nil {
			return account{}, fmt.Errorf("decode stored credential: %w", err)
		}
		credentials = append(credentials, credential)
	}
	return account{user: user, credentials: credentials}, nil
}

func (v *Verifier) BeginRegistration(user domain.User, existing []domain.WebAuthnCredential) ([]byte, []byte, error) {
	acc, err := newAccount(user, existing)
	if err != nil {
		return nil, nil, err
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(acc.credentials))
	for _, credential := range acc.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}
	creation, session, err := v.webauthn.BeginRegistration(acc, library.WithExclusions(exclusions))
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration: %w", err)
	}
	return encodePair(creation, session)
}

func (v *Verifier) FinishRegistration(user domain.User, existing []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	acc, err := newAccount(user, existing)
	if err != nil {
		return nil, nil, err
	}
	var data library.SessionData
	if err := json.Unmarshal(session, &data); err != nil {
		return nil, nil, fmt.Errorf("decode the session: %w", err)
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrInvalidWebAuthnAnswer, err)
	}
	credential, err := v.webauthn.CreateCredential(acc, data, parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrInvalidWebAuthnAnswer, err)
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		return nil, nil, fmt.Errorf("encode the credential: %w", err)
	}
	return credential.ID, stored, nil
}

func (v *Verifier) BeginLogin(user domain.User, existing []domain.WebAuthnCredential) ([]byte, []byte, error) {
	acc, err := newAccount(user, existing)
	if err != nil {
		return nil, nil, err
	}
	if len(acc.credentials) == 0 {
		return nil, nil, domain.ErrWebAuthnKeyNotFound
	}
	assertion, session, err := v.webauthn.BeginLogin(acc, library.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, nil, fmt.Errorf("begin login: %w", err)
	}
	return encodePair(assertion, session)
}

func (v *Verifier) FinishLogin(user domain.User, existing []domain.WebAuthnCredential, session, response []byte) ([]byte, []byte, error) {
	acc, err := newAccount(user, existing)
	if err != nil {
		return nil, nil, err
	}
	var data library.SessionData
	if err := json.Unmarshal(session, &data); err != nil {
		return nil, nil, fmt.Errorf("decode the session: %w", err)
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrInvalidWebAuthnAnswer, err)
	}
	credential, err := v.webauthn.ValidateLogin(acc, data, parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrInvalidWebAuthnAnswer, err)
	}
	// A sign count that went backwards betrays a cloned authenticator.
	if credential.Authenticator.CloneWarning {
		return nil, nil, fmt.Errorf("%w: the sign count went backwards", domain.ErrInvalidWebAuthnAnswer)
	}
	stored, err := json.Marshal(credential)
	if err != nil {
		return nil, nil, fmt.Errorf("encode the credential: %w", err)
	}
	return credential.ID, stored, nil
}

func encodePair(options any, session *library.SessionData) ([]byte, []byte, error) {
	if session == nil {
		return nil, nil, errors.New("no WebAuthn session")
	}
	encodedOptions, err := json.Marshal(options)
	if err != nil {
		return nil, nil, fmt.Errorf("encode options: %w", err)
	}
	encodedSession, err := json.Marshal(session)
	if err != nil {
		return nil, nil, fmt.Errorf("encode session: %w", err)
	}
	return encodedOptions, encodedSession, nil
}
