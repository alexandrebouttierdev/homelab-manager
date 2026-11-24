// Package secretbox encrypts the secrets of the Manager with AES-256-GCM
// and a master key provided by systemd (SPEC_ADMINISTRATION.md § 2.5).
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"homelab-manager/backend/internal/ports"
)

const (
	keyBytes = 32
	// prefix versions the format, to allow another algorithm later.
	prefix = "v1."
)

var (
	ErrInvalidKey = errors.New("the secrets key must be 32 bytes encoded in base64")
	ErrCannotOpen = errors.New("the sealed value cannot be opened")
)

var _ ports.SecretBox = (*Box)(nil)

type Box struct {
	aead cipher.AEAD
}

func New(key []byte) (*Box, error) {
	if len(key) != keyBytes {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Box{aead: aead}, nil
}

// LoadKeyFile reads a key written as base64 (surrounding spaces ignored).
func LoadKeyFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secrets key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(content)))
	if err != nil || len(key) != keyBytes {
		return nil, ErrInvalidKey
	}
	return key, nil
}

// Seal encrypts plaintext with a random nonce; associated is authenticated
// but not stored.
func (b *Box) Seal(plaintext []byte, associated string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := b.aead.Seal(nonce, nonce, plaintext, []byte(associated))
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (b *Box) Open(sealed, associated string) ([]byte, error) {
	encoded, ok := strings.CutPrefix(sealed, prefix)
	if !ok {
		return nil, ErrCannotOpen
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return nil, ErrCannotOpen
	}
	nonce, ciphertext := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, []byte(associated))
	if err != nil {
		return nil, ErrCannotOpen
	}
	return plaintext, nil
}
