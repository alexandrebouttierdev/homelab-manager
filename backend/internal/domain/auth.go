package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrLastAdmin          = errors.New("the last active administrator cannot be removed, demoted or disabled")
)

// User is a Manager account. Accounts are never Linux accounts.
type User struct {
	ID                 uuid.UUID
	Username           string
	DisplayName        string
	Role               Role
	Disabled           bool
	MustChangePassword bool
	LastLoginAt        *time.Time
	LastLoginIP        string
	CreatedAt          time.Time
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
}
