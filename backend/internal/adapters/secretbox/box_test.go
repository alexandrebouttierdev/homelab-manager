package secretbox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey(fill byte) []byte { return bytes.Repeat([]byte{fill}, keyBytes) }

func TestSealAndOpen(t *testing.T) {
	box, err := New(testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal([]byte("totp secret"), "totp:alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1.") || strings.Contains(sealed, "totp secret") {
		t.Fatalf("sealed = %q", sealed)
	}
	again, err := box.Seal([]byte("totp secret"), "totp:alice")
	if err != nil || again == sealed {
		t.Fatal("each seal uses a fresh nonce")
	}
	opened, err := box.Open(sealed, "totp:alice")
	if err != nil || string(opened) != "totp secret" {
		t.Fatalf("Open = (%q, %v)", opened, err)
	}

	if _, err := box.Open(sealed, "totp:bob"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("other associated data = %v, want ErrCannotOpen", err)
	}
	other, err := New(testKey(2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Open(sealed, "totp:alice"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("other key = %v, want ErrCannotOpen", err)
	}
	tampered := sealed[:len(sealed)-2] + "AA"
	if _, err := box.Open(tampered, "totp:alice"); !errors.Is(err, ErrCannotOpen) {
		t.Fatalf("tampered = %v, want ErrCannotOpen", err)
	}
	for _, malformed := range []string{"", "v2." + sealed[3:], "v1.", "v1.!!!"} {
		if _, err := box.Open(malformed, "totp:alice"); !errors.Is(err, ErrCannotOpen) {
			t.Errorf("Open(%q) = %v, want ErrCannotOpen", malformed, err)
		}
	}
}

func TestNewRejectsAShortKey(t *testing.T) {
	if _, err := New(make([]byte, 16)); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("16-byte key = %v, want ErrInvalidKey", err)
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid")
	if err := os.WriteFile(valid, []byte(base64.StdEncoding.EncodeToString(testKey(7))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := LoadKeyFile(valid)
	if err != nil || !bytes.Equal(key, testKey(7)) {
		t.Fatalf("LoadKeyFile = (%v, %v)", key, err)
	}
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte(base64.StdEncoding.EncodeToString(make([]byte, 16))), 0o600); err != nil {
		t.Fatal(err)
	}
	notBase64 := filepath.Join(dir, "text")
	if err := os.WriteFile(notBase64, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{short, notBase64} {
		if _, err := LoadKeyFile(path); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("LoadKeyFile(%s) = %v, want ErrInvalidKey", filepath.Base(path), err)
		}
	}
	if _, err := LoadKeyFile(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file = %v, want os.ErrNotExist", err)
	}
}
