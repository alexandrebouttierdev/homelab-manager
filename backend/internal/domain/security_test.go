package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewSecurityPolicyValidatesBounds(t *testing.T) {
	policy, err := NewSecurityPolicy(30, []string{" 192.168.1.17/24 ", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Networks(); len(got) != 2 || got[0] != "192.168.1.0/24" || got[1] != "fd00::/8" {
		t.Fatalf("networks = %v, want masked prefixes", got)
	}
	if policy.IdleTimeout() != 30*time.Minute {
		t.Fatalf("idle timeout = %s", policy.IdleTimeout())
	}
	tooMany := make([]string, MaxAllowedNetworks+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("10.0.%d.0/24", i)
	}
	for _, c := range []struct {
		minutes  int
		networks []string
	}{
		{4, nil}, {721, nil}, {60, []string{"192.168.1.0"}}, {60, []string{"not-a-network"}}, {60, tooMany},
	} {
		if _, err := NewSecurityPolicy(c.minutes, c.networks); !errors.Is(err, ErrInvalidSecurityPolicy) {
			t.Errorf("NewSecurityPolicy(%d, %v) = %v, want ErrInvalidSecurityPolicy", c.minutes, c.networks, err)
		}
	}
}

func TestSecurityPolicyAllows(t *testing.T) {
	open := DefaultSecurityPolicy()
	if !open.Allows("203.0.113.9") || !open.Allows("not-an-ip") || open.IdleTimeoutMinutes != 60 {
		t.Fatal("the default policy allows every address with a 60 minute idle timeout")
	}
	policy, err := NewSecurityPolicy(60, []string{"192.168.1.0/24", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"192.168.1.20":        true,
		"::ffff:192.168.1.20": true,
		"fd12::1":             true,
		"192.168.2.1":         false,
		"2001:db8::1":         false,
		"not-an-ip":           false,
		"":                    false,
	}
	for address, want := range cases {
		if got := policy.Allows(address); got != want {
			t.Errorf("Allows(%q) = %v, want %v", address, got, want)
		}
	}
}

func TestLoginFailuresLockAfterTheLimit(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	var failures LoginFailures
	for i := 1; i < MaxConsecutiveFailures; i++ {
		var locked bool
		failures, locked = failures.AfterFailure(now)
		if locked || failures.Locked(now) || failures.Count != i {
			t.Fatalf("after %d failures = %+v locked=%v", i, failures, locked)
		}
	}
	failures, locked := failures.AfterFailure(now)
	if !locked || !failures.Locked(now) || failures.Count != 0 {
		t.Fatalf("after %d failures = %+v locked=%v, want a fresh lock", MaxConsecutiveFailures, failures, locked)
	}
	if !failures.Locked(now.Add(LockoutDuration - time.Second)) {
		t.Fatal("the lock lasts the whole lockout duration")
	}
	if failures.Locked(now.Add(LockoutDuration)) {
		t.Fatal("the lock ends after the lockout duration")
	}
}

func TestSessionActivity(t *testing.T) {
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	policy := DefaultSecurityPolicy()
	session := Session{LastSeenAt: now.Add(-59 * time.Second)}
	if session.IdleExpired(now, policy) || session.NeedsTouch(now) {
		t.Fatal("a session seen 59 s ago is neither idle nor due for a touch")
	}
	session.LastSeenAt = now.Add(-time.Minute)
	if !session.NeedsTouch(now) {
		t.Fatal("the last activity is written once a minute")
	}
	session.LastSeenAt = now.Add(-61 * time.Minute)
	if !session.IdleExpired(now, policy) {
		t.Fatal("a session idle for longer than the policy has expired")
	}
}

func TestAuditFilterValidate(t *testing.T) {
	filter, err := AuditFilter{}.Validate(MaxAuditLimit)
	if err != nil || filter.Limit != DefaultAuditLimit {
		t.Fatalf("empty filter = (%+v, %v), want the default limit", filter, err)
	}
	from := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	long := make([]byte, 256)
	for i := range long {
		long[i] = 'a'
	}
	for _, invalid := range []AuditFilter{
		{Limit: MaxAuditLimit + 1},
		{Limit: -1},
		{From: &from, To: &to},
		{Actor: string(long)},
	} {
		if _, err := invalid.Validate(MaxAuditLimit); !errors.Is(err, ErrInvalidAuditFilter) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalidAuditFilter", invalid, err)
		}
	}
	if filter, err := (AuditFilter{Limit: MaxAuditExport}).Validate(MaxAuditExport); err != nil || filter.Limit != MaxAuditExport {
		t.Fatalf("export limit = (%+v, %v)", filter, err)
	}
}
