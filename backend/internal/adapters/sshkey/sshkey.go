// Package sshkey generates the SSH key of the Manager for its SFTP
// repositories and reads the host keys of SFTP servers (decision D-13),
// with golang.org/x/crypto/ssh.
package sshkey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// minRSABits refuses the RSA host keys OpenSSH itself no longer accepts.
const minRSABits = 2048

var errNoKey = errors.New("paste one public key: the content of /etc/ssh/ssh_host_ed25519_key.pub, or one line of ssh-keyscan")

// Keys implements ports.SSHKeys.
type Keys struct{}

// Generate returns a new Ed25519 key.
func (Keys) Generate(comment string) (string, string, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate the SSH key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(private, comment)
	if err != nil {
		return "", "", fmt.Errorf("encode the SSH key: %w", err)
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return "", "", fmt.Errorf("encode the SSH public key: %w", err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic)))
	if comment != "" {
		line += " " + comment
	}
	return string(pem.EncodeToMemory(block)), line, nil
}

// Fingerprint returns the SHA256 fingerprint of an authorized_keys line or
// of a "type base64" key.
func (Keys) Fingerprint(publicKey string) (string, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	if err != nil {
		return "", fmt.Errorf("read the public key: %w", err)
	}
	return ssh.FingerprintSHA256(key), nil
}

// NormalizeHostKey reads one host key and returns it as "type base64".
func (Keys) NormalizeHostKey(text string) (string, error) {
	var line string
	for candidate := range strings.SplitSeq(text, "\n") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}
		if line != "" {
			return "", errors.New("paste a single host key, preferably the ssh-ed25519 one")
		}
		line = candidate
	}
	if line == "" {
		return "", errNoKey
	}
	key, err := parseHostKey(line)
	if err != nil {
		return "", err
	}
	if err := acceptable(key); err != nil {
		return "", err
	}
	return key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal()), nil
}

// parseHostKey reads an authorized_keys line, or a known_hosts line
// without marker; the host names of the line are not kept.
func parseHostKey(line string) (ssh.PublicKey, error) {
	if key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(line)); err == nil && len(options) == 0 && len(rest) == 0 {
		return key, nil
	}
	marker, _, key, _, rest, err := ssh.ParseKnownHosts([]byte(line))
	if err != nil || len(rest) > 0 {
		return nil, errNoKey
	}
	if marker != "" {
		return nil, errors.New("a certificate authority or a revoked key is not a host key")
	}
	return key, nil
}

func acceptable(key ssh.PublicKey) error {
	switch key.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return nil
	case ssh.KeyAlgoRSA:
		crypto, ok := key.(ssh.CryptoPublicKey)
		if !ok {
			return errors.New("unreadable RSA key")
		}
		if rsaKey, ok := crypto.CryptoPublicKey().(*rsa.PublicKey); !ok || rsaKey.N.BitLen() < minRSABits {
			return fmt.Errorf("an RSA host key has at least %d bits", minRSABits)
		}
		return nil
	default:
		return fmt.Errorf("host keys of type %s are not accepted: use ssh-ed25519, ecdsa-sha2-nistp* or ssh-rsa", key.Type())
	}
}
