package sshkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func authorizedLine(t *testing.T, public any, comment string) string {
	t.Helper()
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) + " " + comment
}

func TestGenerateGivesAWorkingKeyPair(t *testing.T) {
	var keys Keys
	private, public, err := keys.Generate("homelab-manager-backup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(private, "-----BEGIN OPENSSH PRIVATE KEY-----") ||
		!strings.HasPrefix(public, "ssh-ed25519 ") || !strings.HasSuffix(public, " homelab-manager-backup") {
		t.Fatalf("key pair = %q / %q", private[:40], public)
	}
	signer, err := ssh.ParsePrivateKey([]byte(private))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(public))
	if err != nil || string(parsed.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatalf("public key does not match the private one: %v", err)
	}
	fingerprint, err := keys.Fingerprint(public)
	if err != nil || fingerprint != ssh.FingerprintSHA256(parsed) || !strings.HasPrefix(fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q, %v", fingerprint, err)
	}
	_, other, _ := keys.Generate("homelab-manager-backup")
	if other == public {
		t.Fatal("two generated keys are equal")
	}
}

func TestNormalizeHostKeyAcceptsBothFormats(t *testing.T) {
	var keys Keys
	_, public, _ := keys.Generate("root@server")
	fields := strings.Fields(public)
	want := fields[0] + " " + fields[1]
	for name, text := range map[string]string{
		"pub file":           public + "\n",
		"without comment":    want,
		"ssh-keyscan":        "# server:22 SSH-2.0-OpenSSH_10.0\nserver " + want + "\n",
		"keyscan with port":  "[server]:2222 " + want,
		"hashed known_hosts": "|1|c2FsdA==|aGFzaA== " + want,
	} {
		got, err := keys.NormalizeHostKey(text)
		if err != nil || got != want {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
	ecdsaKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if got, err := keys.NormalizeHostKey(authorizedLine(t, &ecdsaKey.PublicKey, "root@server")); err != nil ||
		!strings.HasPrefix(got, "ecdsa-sha2-nistp256 ") {
		t.Fatalf("ecdsa = %q, %v", got, err)
	}
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err := keys.NormalizeHostKey(authorizedLine(t, &rsaKey.PublicKey, "")); err != nil {
		t.Fatalf("rsa 2048 = %v", err)
	}
}

func TestNormalizeHostKeyRefusals(t *testing.T) {
	var keys Keys
	_, public, _ := keys.Generate("")
	_, second, _ := keys.Generate("")
	weak, _ := rsa.GenerateKey(rand.Reader, 1024)
	for name, text := range map[string]string{
		"empty":          " \n# comment\n",
		"garbage":        "not a key",
		"two keys":       public + "\n" + second,
		"cert authority": "@cert-authority *.lan " + public,
		"revoked":        "@revoked * " + public,
		"weak rsa":       authorizedLine(t, &weak.PublicKey, ""),
		"trailing data":  "server " + public + "\nserver",
	} {
		if got, err := keys.NormalizeHostKey(text); err == nil {
			t.Errorf("%s accepted as %q", name, got)
		}
	}
	if _, err := keys.Fingerprint("nope"); err == nil {
		t.Fatal("fingerprint of garbage")
	}
}
