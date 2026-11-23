package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined on HMAC-SHA-1, which authenticator apps expect.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (SPEC_ADMINISTRATION.md § 3.3, RFC 6238).
const (
	TOTPDigits      = 6
	TOTPPeriod      = 30 * time.Second
	TOTPSecretBytes = 20
	// totpSkew is the number of periods accepted before and after now.
	totpSkew = 1
)

// Recovery codes (SPEC_ADMINISTRATION.md § 3.3).
const (
	RecoveryCodeCount = 10
	// recoveryAlphabet leaves out 0, 1, i, l and o, which are easy to misread.
	recoveryAlphabet    = "abcdefghjkmnpqrstuvwxyz23456789"
	recoveryGroups      = 3
	recoveryGroupLength = 4
)

var (
	ErrSecondFactorUnavailable    = errors.New("the secrets key is missing: TOTP is unavailable")
	ErrSecondFactorAlreadyEnabled = errors.New("a second factor is already enabled")
	ErrSecondFactorNotEnabled     = errors.New("no second factor is enabled")
	ErrNoPendingEnrollment        = errors.New("no TOTP enrollment is pending")
	ErrInvalidSecondFactor        = errors.New("the second factor code is invalid")
	ErrLoginChallengeExpired      = errors.New("the login challenge is missing or expired")
	ErrSecondFactorNotEnrolled    = errors.New("enroll your own second factor before requiring it")
)

// SecondFactorMethod names how a login completed its second step.
type SecondFactorMethod string

const (
	SecondFactorTOTP     SecondFactorMethod = "totp"
	SecondFactorRecovery SecondFactorMethod = "recovery"
)

func (m SecondFactorMethod) Valid() bool {
	return m == SecondFactorTOTP || m == SecondFactorRecovery
}

// TOTPEnrollment is the stored TOTP secret of an account, sealed by the
// secrets vault.
type TOTPEnrollment struct {
	Sealed       string
	Confirmed    bool
	LastUsedStep int64
}

// SecondFactorStatus describes the second factor of an account.
type SecondFactorStatus struct {
	Enabled                bool
	Pending                bool
	RecoveryCodesRemaining int
	// Available is false when the secrets key is missing.
	Available bool
}

// QRMatrix is a QR code as rows of "1" (dark) and "0" (light) modules.
type QRMatrix struct {
	Size int
	Rows []string
}

// TOTPStep returns the time step of t.
func TOTPStep(t time.Time) int64 {
	return t.Unix() / int64(TOTPPeriod/time.Second)
}

// TOTPCode returns the 6-digit code of a step.
func TOTPCode(secret []byte, step int64) string {
	return hotp(secret, step, TOTPDigits)
}

// hotp implements RFC 4226 with HMAC-SHA-1 and dynamic truncation.
func hotp(secret []byte, counter int64, digits int) string {
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], uint64(counter))
	mac := hmac.New(sha1.New, secret)
	mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulo := uint32(1)
	for range digits {
		modulo *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%modulo)
}

// MatchTOTP returns the step whose code matches, within one period before
// or after now. Spaces typed in the code are ignored. The caller must
// refuse a step already used (anti-replay).
func MatchTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != TOTPDigits {
		return 0, false
	}
	current := TOTPStep(now)
	for step := current - totpSkew; step <= current+totpSkew; step++ {
		if subtle.ConstantTimeCompare([]byte(TOTPCode(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// NewTOTPSecret returns a random secret of TOTPSecretBytes.
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, TOTPSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate TOTP secret: %w", err)
	}
	return secret, nil
}

// TOTPSecretText is the secret as typed in an authenticator app.
func TOTPSecretText(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// TOTPURI is the otpauth:// URI encoded in the enrollment QR code.
func TOTPURI(issuer, account string, secret []byte) string {
	query := url.Values{}
	query.Set("secret", TOTPSecretText(secret))
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprint(TOTPDigits))
	query.Set("period", fmt.Sprint(int(TOTPPeriod/time.Second)))
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// NewRecoveryCodes returns RecoveryCodeCount distinct codes such as
// "k7pq-2mzx-b9ht".
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	seen := make(map[string]bool, RecoveryCodeCount)
	limit := big.NewInt(int64(len(recoveryAlphabet)))
	for len(codes) < RecoveryCodeCount {
		var builder strings.Builder
		for group := range recoveryGroups {
			if group > 0 {
				builder.WriteByte('-')
			}
			for range recoveryGroupLength {
				index, err := rand.Int(rand.Reader, limit)
				if err != nil {
					return nil, fmt.Errorf("generate recovery code: %w", err)
				}
				builder.WriteByte(recoveryAlphabet[index.Int64()])
			}
		}
		if code := builder.String(); !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	return codes, nil
}

// RecoveryCodeHash hashes a recovery code as typed: case, spaces and
// dashes do not matter. The codes carry about 59 bits of entropy, enough
// for a fast hash, like session tokens.
func RecoveryCodeHash(code string) string {
	normalized := strings.NewReplacer(" ", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
