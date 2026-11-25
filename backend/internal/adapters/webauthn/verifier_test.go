package webauthn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"homelab-manager/backend/internal/domain"

	"github.com/google/uuid"
)

const (
	testRPID   = "homelab.test"
	testOrigin = "https://homelab.test:8443"
)

// cbor writes the few CBOR items an authenticator answers with.
type cbor struct{ bytes.Buffer }

func (c *cbor) head(major byte, n uint64) {
	switch {
	case n < 24:
		c.WriteByte(major<<5 | byte(n))
	case n < 256:
		c.WriteByte(major<<5 | 24)
		c.WriteByte(byte(n))
	default:
		c.WriteByte(major<<5 | 25)
		_ = binary.Write(c, binary.BigEndian, uint16(n))
	}
}

func (c *cbor) integer(v int) {
	if v >= 0 {
		c.head(0, uint64(v))
	} else {
		c.head(1, uint64(-1-v))
	}
}

func (c *cbor) bytes(b []byte) {
	c.head(2, uint64(len(b)))
	c.Write(b)
}

func (c *cbor) text(s string) {
	c.head(3, uint64(len(s)))
	c.WriteString(s)
}

// authenticator is a software FIDO2 authenticator with one P-256 key.
type authenticator struct {
	key          *ecdsa.PrivateKey
	credentialID []byte
	signCount    uint32
	origin       string
	// withoutUV answers without user verification (no PIN, no biometrics).
	withoutUV bool
}

func newAuthenticator(t *testing.T) *authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &authenticator{key: key, credentialID: id, origin: testOrigin}
}

var b64 = base64.RawURLEncoding

func (a *authenticator) coseKey() []byte {
	var c cbor
	c.head(5, 5)
	c.integer(1)
	c.integer(2) // kty: EC2
	c.integer(3)
	c.integer(-7) // alg: ES256
	c.integer(-1)
	c.integer(1) // crv: P-256
	x, y := make([]byte, 32), make([]byte, 32)
	a.key.PublicKey.X.FillBytes(x)
	a.key.PublicKey.Y.FillBytes(y)
	c.integer(-2)
	c.bytes(x)
	c.integer(-3)
	c.bytes(y)
	return c.Bytes()
}

func (a *authenticator) authData(attested bool) []byte {
	rpHash := sha256.Sum256([]byte(testRPID))
	data := append([]byte{}, rpHash[:]...)
	flags := byte(0x01 | 0x04) // user present, user verified
	if a.withoutUV {
		flags = 0x01
	}
	if attested {
		flags |= 0x40
	}
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, a.signCount)
	if attested {
		data = append(data, make([]byte, 16)...) // AAGUID
		data = binary.BigEndian.AppendUint16(data, uint16(len(a.credentialID)))
		data = append(data, a.credentialID...)
		data = append(data, a.coseKey()...)
	}
	return data
}

func challengeOf(t *testing.T, options []byte) string {
	t.Helper()
	var parsed struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil || parsed.PublicKey.Challenge == "" {
		t.Fatalf("options %s: %v", options, err)
	}
	return parsed.PublicKey.Challenge
}

func (a *authenticator) clientData(kind, challenge string) []byte {
	data, _ := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": a.origin, "crossOrigin": false})
	return data
}

func (a *authenticator) create(t *testing.T, options []byte) []byte {
	t.Helper()
	clientData := a.clientData("webauthn.create", challengeOf(t, options))
	var object cbor
	object.head(5, 3)
	object.text("fmt")
	object.text("none")
	object.text("attStmt")
	object.head(5, 0)
	object.text("authData")
	object.bytes(a.authData(true))
	response, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credentialID), "rawId": b64.EncodeToString(a.credentialID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(clientData), "attestationObject": b64.EncodeToString(object.Bytes()),
		},
	})
	return response
}

func (a *authenticator) get(t *testing.T, options []byte, userHandle []byte) []byte {
	t.Helper()
	a.signCount++
	clientData := a.clientData("webauthn.get", challengeOf(t, options))
	authData := a.authData(false)
	digest := sha256.Sum256(clientData)
	signed := sha256.Sum256(append(append([]byte{}, authData...), digest[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, signed[:])
	if err != nil {
		t.Fatal(err)
	}
	response, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(a.credentialID), "rawId": b64.EncodeToString(a.credentialID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(clientData), "authenticatorData": b64.EncodeToString(authData),
			"signature": b64.EncodeToString(signature), "userHandle": b64.EncodeToString(userHandle),
		},
	})
	return response
}

func newVerifier(t *testing.T) *Verifier {
	t.Helper()
	verifier, err := New(Config{RPID: testRPID, RPDisplayName: "Homelab Manager", Origins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func register(t *testing.T, verifier *Verifier, user domain.User, key *authenticator) domain.WebAuthnCredential {
	t.Helper()
	options, session, err := verifier.BeginRegistration(user, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(options, []byte(`"userVerification":"required"`)) || !bytes.Contains(options, []byte(`"id":"homelab.test"`)) {
		t.Fatalf("creation options = %s", options)
	}
	id, data, err := verifier.FinishRegistration(user, nil, session, key.create(t, options))
	if err != nil {
		t.Fatalf("finish registration: %v", err)
	}
	if !bytes.Equal(id, key.credentialID) {
		t.Fatal("credential identifier")
	}
	return domain.WebAuthnCredential{ID: uuid.New(), UserID: user.ID, Name: "key", CredentialID: id, Data: data}
}

func TestRegisterThenLogIn(t *testing.T) {
	verifier := newVerifier(t)
	user := domain.User{ID: uuid.New(), Username: "alice", DisplayName: "Alice"}
	key := newAuthenticator(t)
	stored := register(t, verifier, user, key)

	options, session, err := verifier.BeginLogin(user, []domain.WebAuthnCredential{stored})
	if err != nil {
		t.Fatal(err)
	}
	id, updated, err := verifier.FinishLogin(user, []domain.WebAuthnCredential{stored}, session, key.get(t, options, user.ID[:]))
	if err != nil || !bytes.Equal(id, key.credentialID) || bytes.Equal(updated, stored.Data) {
		t.Fatalf("login = %v (updated %v)", err, !bytes.Equal(updated, stored.Data))
	}
	// The same answer cannot be replayed against a new challenge.
	options2, session2, _ := verifier.BeginLogin(user, []domain.WebAuthnCredential{stored})
	_ = options2
	replay := key.get(t, options, user.ID[:])
	if _, _, err := verifier.FinishLogin(user, []domain.WebAuthnCredential{stored}, session2, replay); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("replayed answer = %v", err)
	}
}

func TestLoginRefusesAnotherOriginOrKey(t *testing.T) {
	verifier := newVerifier(t)
	user := domain.User{ID: uuid.New(), Username: "alice"}
	key := newAuthenticator(t)
	stored := register(t, verifier, user, key)

	key.origin = "https://evil.test"
	options, session, _ := verifier.BeginLogin(user, []domain.WebAuthnCredential{stored})
	if _, _, err := verifier.FinishLogin(user, []domain.WebAuthnCredential{stored}, session, key.get(t, options, user.ID[:])); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("other origin = %v", err)
	}
	key.origin = testOrigin
	other := newAuthenticator(t)
	other.credentialID = key.credentialID // same identifier, other private key
	options, session, _ = verifier.BeginLogin(user, []domain.WebAuthnCredential{stored})
	if _, _, err := verifier.FinishLogin(user, []domain.WebAuthnCredential{stored}, session, other.get(t, options, user.ID[:])); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("forged signature = %v", err)
	}
	if _, _, err := verifier.BeginLogin(user, nil); !errors.Is(err, domain.ErrWebAuthnKeyNotFound) {
		t.Fatalf("login without key = %v", err)
	}
}

func TestRegistrationRefusesAnInvalidAnswer(t *testing.T) {
	verifier := newVerifier(t)
	user := domain.User{ID: uuid.New(), Username: "alice"}
	key := newAuthenticator(t)
	_, session, _ := verifier.BeginRegistration(user, nil)
	otherOptions, _, _ := verifier.BeginRegistration(user, nil)
	if _, _, err := verifier.FinishRegistration(user, nil, session, key.create(t, otherOptions)); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("answer to another challenge = %v", err)
	}
	if _, _, err := verifier.FinishRegistration(user, nil, session, []byte(`{"id":"x"}`)); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("malformed answer = %v", err)
	}
}

// lookupOf finds the accounts of a test like the Manager does: by the user
// handle, which is the UUID of the account.
func lookupOf(accounts map[uuid.UUID][]domain.WebAuthnCredential, users ...domain.User) func([]byte) (domain.User, []domain.WebAuthnCredential, error) {
	return func(handle []byte) (domain.User, []domain.WebAuthnCredential, error) {
		id, err := uuid.FromBytes(handle)
		if err != nil {
			return domain.User{}, nil, domain.ErrWebAuthnKeyNotFound
		}
		for _, user := range users {
			if user.ID == id {
				return user, accounts[id], nil
			}
		}
		return domain.User{}, nil, domain.ErrWebAuthnKeyNotFound
	}
}

func TestPasskeyLoginFindsTheAccountOfTheKey(t *testing.T) {
	verifier := newVerifier(t)
	alice := domain.User{ID: uuid.New(), Username: "alice"}
	bob := domain.User{ID: uuid.New(), Username: "bob"}
	aliceKey, bobKey := newAuthenticator(t), newAuthenticator(t)
	accounts := map[uuid.UUID][]domain.WebAuthnCredential{
		alice.ID: {register(t, verifier, alice, aliceKey)},
		bob.ID:   {register(t, verifier, bob, bobKey)},
	}
	lookup := lookupOf(accounts, alice, bob)

	options, session, err := verifier.BeginPasskeyLogin()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(options, []byte(`"userVerification":"required"`)) || bytes.Contains(options, []byte(`allowCredentials`)) {
		t.Fatalf("passkey options = %s", options)
	}
	user, id, data, err := verifier.FinishPasskeyLogin(session, bobKey.get(t, options, bob.ID[:]), lookup)
	if err != nil || user.ID != bob.ID || !bytes.Equal(id, bobKey.credentialID) || len(data) == 0 {
		t.Fatalf("passkey login = %+v, %v", user, err)
	}

	refused := map[string]func(options []byte) []byte{
		// The key of Bob claims to be Alice.
		"other account": func(options []byte) []byte { return bobKey.get(t, options, alice.ID[:]) },
		"unknown account": func(options []byte) []byte {
			stranger := uuid.New()
			return aliceKey.get(t, options, stranger[:])
		},
		"no user handle": func(options []byte) []byte { return aliceKey.get(t, options, nil) },
		"no verification": func(options []byte) []byte {
			aliceKey.withoutUV = true
			defer func() { aliceKey.withoutUV = false }()
			return aliceKey.get(t, options, alice.ID[:])
		},
		"other origin": func(options []byte) []byte {
			aliceKey.origin = "https://evil.test"
			defer func() { aliceKey.origin = testOrigin }()
			return aliceKey.get(t, options, alice.ID[:])
		},
		"malformed answer": func([]byte) []byte { return []byte(`{"id":"x"}`) },
	}
	for name, answer := range refused {
		options, session, err := verifier.BeginPasskeyLogin()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := verifier.FinishPasskeyLogin(session, answer(options), lookup); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
			t.Fatalf("%s = %v", name, err)
		}
	}

	// A session of a login with a known user is not a passkey session.
	options, session, _ = verifier.BeginLogin(alice, accounts[alice.ID])
	if _, _, _, err := verifier.FinishPasskeyLogin(session, aliceKey.get(t, options, alice.ID[:]), lookup); !errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("answer to a second-factor ceremony = %v", err)
	}
	// A storage failure is reported as such.
	failing := func([]byte) (domain.User, []domain.WebAuthnCredential, error) {
		return domain.User{}, nil, errors.New("disk failure")
	}
	options, session, _ = verifier.BeginPasskeyLogin()
	if _, _, _, err := verifier.FinishPasskeyLogin(session, aliceKey.get(t, options, alice.ID[:]), failing); err == nil || errors.Is(err, domain.ErrInvalidWebAuthnAnswer) {
		t.Fatalf("storage failure = %v", err)
	}
}
