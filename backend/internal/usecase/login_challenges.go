package usecase

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Bounds of the second login step (SPEC_ADMINISTRATION.md § 3.3).
const (
	loginChallengeTTL     = 5 * time.Minute
	maxChallengeAttempts  = 5
	maxPendingChallenges  = 1000
	challengeTokenEntropy = 32 // bytes, like session tokens
)

// ErrTooManyLoginChallenges refuses a login while the bounded store of
// pending logins is full.
var ErrTooManyLoginChallenges = errors.New("too many pending login challenges")

// pendingLogin is a password accepted for an account that still has to
// give its second factor.
type pendingLogin struct {
	userID    uuid.UUID
	expiresAt time.Time
	attempts  int
	// webauthn is the session of the key ceremony in progress, if any.
	webauthn []byte
}

// loginChallenges keeps the pending logins in memory, keyed by the hash of
// their token: they last 5 minutes and a restart only asks the password
// again. The map is bounded and expired entries are dropped on insertion.
type loginChallenges struct {
	mu      sync.Mutex
	pending map[string]*pendingLogin
}

func newLoginChallenges() *loginChallenges {
	return &loginChallenges{pending: make(map[string]*pendingLogin)}
}

func (c *loginChallenges) create(userID uuid.UUID, now time.Time) (token string, expiresAt time.Time, err error) {
	token, err = randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, login := range c.pending {
		if !now.Before(login.expiresAt) {
			delete(c.pending, key)
		}
	}
	if len(c.pending) >= maxPendingChallenges {
		return "", time.Time{}, ErrTooManyLoginChallenges
	}
	expiresAt = now.Add(loginChallengeTTL)
	c.pending[sha256Hex(token)] = &pendingLogin{userID: userID, expiresAt: expiresAt}
	return token, expiresAt, nil
}

// find returns the pending login of a token that has not expired.
func (c *loginChallenges) find(token string, now time.Time) (pendingLogin, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	login, ok := c.pending[sha256Hex(token)]
	if !ok || !now.Before(login.expiresAt) {
		return pendingLogin{}, false
	}
	return *login, true
}

// fail counts a wrong code; the challenge ends after maxChallengeAttempts.
func (c *loginChallenges) fail(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := sha256Hex(token)
	if login, ok := c.pending[key]; ok {
		login.attempts++
		if login.attempts >= maxChallengeAttempts {
			delete(c.pending, key)
		}
	}
}

func (c *loginChallenges) remove(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, sha256Hex(token))
}

// setSession attaches a key ceremony to a pending login.
func (c *loginChallenges) setSession(token string, session []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	login, ok := c.pending[sha256Hex(token)]
	if ok {
		login.webauthn = session
	}
	return ok
}

// takeSession returns the key ceremony of a pending login and clears it.
func (c *loginChallenges) takeSession(token string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	login, ok := c.pending[sha256Hex(token)]
	if !ok {
		return nil
	}
	session := login.webauthn
	login.webauthn = nil
	return session
}
