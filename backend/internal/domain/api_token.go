package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Bounds of the API tokens (SPEC_ADMINISTRATION.md § 3.5).
const (
	APITokenPrefix          = "mop_"
	apiTokenBytes           = 32
	APITokenDisplayLength   = 12
	MaxAPITokensPerUser     = 20
	DefaultAPITokenLifetime = 90
	MaxAPITokenLifetime     = 365
	maxAPITokenNameBytes    = 100
	// APITokenTouchEvery spaces two writes of the last use.
	APITokenTouchEvery = time.Minute
)

var (
	ErrInvalidAPIToken      = errors.New("invalid API token request")
	ErrAPITokenNotFound     = errors.New("API token not found")
	ErrTooManyAPITokens     = errors.New("too many API tokens for this account")
	ErrAPITokenUnauthorized = errors.New("the API token is unknown, expired or revoked")
	// ErrAPITokenScopeNotAllowed: an action scope the role of the account
	// does not grant.
	ErrAPITokenScopeNotAllowed = errors.New("the role of the account does not allow this token scope")
	encodedAPITokenLength      = len(APITokenPrefix) + base64.RawURLEncoding.EncodedLen(apiTokenBytes)
)

// APITokenScope is a part of the API a token may reach: read-only scopes,
// and two action scopes (D-4) that also need the permission of the role.
type APITokenScope string

const (
	ScopeMetricsRead    APITokenScope = "metrics:read"
	ScopeContainersRead APITokenScope = "containers:read"
	ScopeEventsRead     APITokenScope = "events:read"
	ScopeServicesRead   APITokenScope = "services:read"
	ScopeBackupsRead    APITokenScope = "backups:read"
	// ScopeServicesOperate starts, stops, restarts or reloads a unit of
	// the allow list.
	ScopeServicesOperate APITokenScope = "services:operate"
	// ScopeContainersOperate starts, stops or restarts a container.
	ScopeContainersOperate APITokenScope = "containers:operate"
)

// APITokenScopes lists the scopes in the order shown by the interface.
var APITokenScopes = []APITokenScope{
	ScopeMetricsRead, ScopeContainersRead, ScopeEventsRead, ScopeServicesRead, ScopeBackupsRead,
	ScopeServicesOperate, ScopeContainersOperate,
}

// Permission returns the permission an action scope needs, checked when
// the token is created and at each use; false for a read-only scope.
func (s APITokenScope) Permission() (Permission, bool) {
	switch s {
	case ScopeServicesOperate:
		return PermManageServices, true
	case ScopeContainersOperate:
		return PermManageContainers, true
	}
	return "", false
}

// APIToken is a token without its secret: only its SHA-256 is stored.
type APIToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Username   string
	Name       string
	Prefix     string
	Scopes     []APITokenScope
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

// Allows reports whether the token holds the scope.
func (t APIToken) Allows(scope APITokenScope) bool {
	return slices.Contains(t.Scopes, scope)
}

// APITokenRequest is a token asked by a user.
type APITokenRequest struct {
	Name         string
	Scopes       []APITokenScope
	LifetimeDays int
}

// Validate checks the name, the scopes (at least one, no duplicate) and the
// lifetime; a lifetime of 0 becomes the default.
func (r APITokenRequest) Validate() (APITokenRequest, error) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || len(r.Name) > maxAPITokenNameBytes || !utf8.ValidString(r.Name) || strings.IndexFunc(r.Name, unicode.IsControl) >= 0 {
		return r, ErrInvalidAPIToken
	}
	if len(r.Scopes) == 0 {
		return r, ErrInvalidAPIToken
	}
	for i, scope := range r.Scopes {
		if !slices.Contains(APITokenScopes, scope) || slices.Contains(r.Scopes[:i], scope) {
			return r, ErrInvalidAPIToken
		}
	}
	if r.LifetimeDays == 0 {
		r.LifetimeDays = DefaultAPITokenLifetime
	}
	if r.LifetimeDays < 1 || r.LifetimeDays > MaxAPITokenLifetime {
		return r, ErrInvalidAPIToken
	}
	return r, nil
}

// NewAPITokenSecret returns a new token, "mop_" and 32 random bytes in
// base64url, and the hash stored in its place.
func NewAPITokenSecret() (string, string, error) {
	random := make([]byte, apiTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	token := APITokenPrefix + base64.RawURLEncoding.EncodeToString(random)
	return token, HashAPIToken(token), nil
}

// HashAPIToken is the SHA-256 of a token, in hexadecimal: the token has
// 256 bits of entropy, a slow hash adds nothing.
func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LooksLikeAPIToken checks the form of a token before any lookup.
func LooksLikeAPIToken(token string) bool {
	if len(token) != encodedAPITokenLength || !strings.HasPrefix(token, APITokenPrefix) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, APITokenPrefix))
	return err == nil
}
