package argon2

import "testing"

func TestHasherVerify(t *testing.T) {
	hasher := New()
	hash, err := hasher.Hash("a-very-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := hasher.Verify("a-very-secure-password", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("expected password to verify")
	}
	valid, err = hasher.Verify("wrong-password", hash)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("wrong password unexpectedly verified")
	}
}
