package domain

import (
	"errors"
	"net/netip"
	"strings"
	"time"
)

// Bounds of the security policy and of the account lockout
// (SPEC_ADMINISTRATION.md § 3.4).
const (
	MinIdleTimeoutMinutes     = 5
	MaxIdleTimeoutMinutes     = 720 // the 12 h maximum lifetime of a session
	DefaultIdleTimeoutMinutes = 60
	MaxAllowedNetworks        = 32
	MaxConsecutiveFailures    = 5
	LockoutDuration           = 15 * time.Minute
)

var (
	ErrInvalidSecurityPolicy        = errors.New("idle timeout must be 5 to 720 minutes and networks at most 32 CIDR prefixes")
	ErrPolicyExcludesCurrentAddress = errors.New("the policy would exclude the address of the current administrator")
)

// SecurityPolicy holds the security settings an administrator can change.
type SecurityPolicy struct {
	IdleTimeoutMinutes int
	// RequireAdminSecondFactor limits administrators without a second
	// factor to its enrollment.
	RequireAdminSecondFactor bool
	// AllowedNetworks limits the addresses that may use the interface; an
	// empty list allows every address.
	AllowedNetworks []netip.Prefix
	// AllowPasskeyLogin lets an account sign in with a passkey alone,
	// without its password (decision D-14); off by default.
	AllowPasskeyLogin bool
}

func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{IdleTimeoutMinutes: DefaultIdleTimeoutMinutes}
}

// NewSecurityPolicy validates the settings; prefixes are stored masked.
func NewSecurityPolicy(idleTimeoutMinutes int, networks []string) (SecurityPolicy, error) {
	if idleTimeoutMinutes < MinIdleTimeoutMinutes || idleTimeoutMinutes > MaxIdleTimeoutMinutes || len(networks) > MaxAllowedNetworks {
		return SecurityPolicy{}, ErrInvalidSecurityPolicy
	}
	prefixes := make([]netip.Prefix, 0, len(networks))
	for _, network := range networks {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(network))
		if err != nil {
			return SecurityPolicy{}, ErrInvalidSecurityPolicy
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return SecurityPolicy{IdleTimeoutMinutes: idleTimeoutMinutes, AllowedNetworks: prefixes}, nil
}

func (p SecurityPolicy) IdleTimeout() time.Duration {
	return time.Duration(p.IdleTimeoutMinutes) * time.Minute
}

// Allows reports whether address may use the interface. With a list of
// networks, an address that cannot be parsed is refused.
func (p SecurityPolicy) Allows(address string) bool {
	if len(p.AllowedNetworks) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range p.AllowedNetworks {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// Networks returns the allowed networks in CIDR notation.
func (p SecurityPolicy) Networks() []string {
	networks := make([]string, 0, len(p.AllowedNetworks))
	for _, prefix := range p.AllowedNetworks {
		networks = append(networks, prefix.String())
	}
	return networks
}

// LoginFailures counts the consecutive failed passwords of an account.
type LoginFailures struct {
	Count       int
	LockedUntil *time.Time
}

func (f LoginFailures) Locked(now time.Time) bool {
	return f.LockedUntil != nil && now.Before(*f.LockedUntil)
}

// AfterFailure counts one more failed password. At MaxConsecutiveFailures
// the account is locked for LockoutDuration and the counter restarts;
// locked reports such a new lock.
func (f LoginFailures) AfterFailure(now time.Time) (next LoginFailures, locked bool) {
	count := f.Count + 1
	if count >= MaxConsecutiveFailures {
		until := now.UTC().Add(LockoutDuration)
		return LoginFailures{LockedUntil: &until}, true
	}
	return LoginFailures{Count: count, LockedUntil: f.LockedUntil}, false
}
