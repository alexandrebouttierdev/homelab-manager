package ports

import "homelab-manager/backend/internal/domain"

// SecretBox encrypts the secrets the Manager must reuse
// (SPEC_ADMINISTRATION.md § 2.5). The associated data binds a sealed value
// to its use: opening it with other associated data fails.
type SecretBox interface {
	Seal(plaintext []byte, associated string) (string, error)
	Open(sealed, associated string) ([]byte, error)
}

// QREncoder draws a QR code.
type QREncoder interface {
	Encode(text string) (domain.QRMatrix, error)
}
