package domain

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B: SHA-1 secret "12345678901234567890", 8 digits.
var rfc6238 = []struct {
	unix  int64
	code8 string
}{
	{59, "94287082"},
	{1111111109, "07081804"},
	{1111111111, "14050471"},
	{1234567890, "89005924"},
	{2000000000, "69279037"},
	{20000000000, "65353130"},
}

func TestTOTPMatchesTheRFCVectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	for _, vector := range rfc6238 {
		step := TOTPStep(time.Unix(vector.unix, 0))
		if got := hotp(secret, step, 8); got != vector.code8 {
			t.Errorf("8 digits at %d = %s, want %s", vector.unix, got, vector.code8)
		}
		if got := TOTPCode(secret, step); got != vector.code8[2:] {
			t.Errorf("6 digits at %d = %s, want %s", vector.unix, got, vector.code8[2:])
		}
	}
}

func TestMatchTOTPAcceptsOnePeriodOfSkew(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0)
	step := TOTPStep(now)
	for offset := int64(-1); offset <= 1; offset++ {
		code := TOTPCode(secret, step+offset)
		matched, ok := MatchTOTP(secret, code, now)
		if !ok || matched != step+offset {
			t.Errorf("offset %d = (%d, %v), want (%d, true)", offset, matched, ok, step+offset)
		}
	}
	for _, code := range []string{TOTPCode(secret, step+2), TOTPCode(secret, step-2), "", "12345", "1234567", "abcdef"} {
		if _, ok := MatchTOTP(secret, code, now); ok {
			t.Errorf("code %q must be refused", code)
		}
	}
	if _, ok := MatchTOTP(secret, " "+TOTPCode(secret, step)[:3]+" "+TOTPCode(secret, step)[3:]+" ", now); !ok {
		t.Error("spaces typed inside the code are ignored")
	}
}

func TestTOTPURI(t *testing.T) {
	uri := TOTPURI("MonoOps", "alice", []byte("12345678901234567890"))
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" || parsed.Path != "/MonoOps:alice" ||
		query.Get("secret") != "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" || query.Get("issuer") != "MonoOps" ||
		query.Get("digits") != "6" || query.Get("period") != "30" || query.Get("algorithm") != "SHA1" {
		t.Fatalf("uri = %s", uri)
	}
	if TOTPSecretText([]byte("12345678901234567890")) != "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" {
		t.Fatal("the secret is shown in unpadded base32")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("%d codes, want %d", len(codes), RecoveryCodeCount)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if len(code) != 14 || code[4] != '-' || code[9] != '-' || strings.ContainsAny(code, "01ilo") || seen[code] {
			t.Fatalf("code %q is malformed or repeated", code)
		}
		seen[code] = true
	}
	code := codes[0]
	typed := " " + strings.ToUpper(strings.ReplaceAll(code, "-", " ")) + " "
	if RecoveryCodeHash(typed) != RecoveryCodeHash(code) {
		t.Fatal("case, spaces and dashes do not matter")
	}
	if RecoveryCodeHash(code) == RecoveryCodeHash(codes[1]) || len(RecoveryCodeHash(code)) != 64 {
		t.Fatal("hashes are distinct SHA-256 hex digests")
	}
}
