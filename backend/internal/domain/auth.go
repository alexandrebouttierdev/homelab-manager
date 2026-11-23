package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrForbidden          = errors.New("the account role does not allow this action")
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrLastAdmin          = errors.New("the last active administrator cannot be removed, demoted or disabled")
	ErrOwnPasswordReset   = errors.New("administrators change their own password instead of resetting it")
)

// User is a Manager account. Accounts are never Linux accounts.
type User struct {
	ID                 uuid.UUID
	Username           string
	DisplayName        string
	Role               Role
	Disabled           bool
	MustChangePassword bool
	// MustEnrollSecondFactor is set when the policy requires a second
	// factor from administrators and this one has none.
	MustEnrollSecondFactor bool
	LastLoginAt            *time.Time
	LastLoginIP            string
	CreatedAt              time.Time
}

// SessionTouchInterval bounds how often the last activity of a session is
// written, to keep SQLite writes rare.
const SessionTouchInterval = time.Minute

// Session is a signed-in browser. Only the hash of its token is stored.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Username   string
	IPAddress  string
	UserAgent  string
	AuthMethod string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// IdleExpired reports whether the session has been inactive for longer
// than the policy allows.
func (s Session) IdleExpired(now time.Time, policy SecurityPolicy) bool {
	return now.Sub(s.LastSeenAt) > policy.IdleTimeout()
}

// NeedsTouch reports whether the last activity should be written again.
func (s Session) NeedsTouch(now time.Time) bool {
	return now.Sub(s.LastSeenAt) >= SessionTouchInterval
}
