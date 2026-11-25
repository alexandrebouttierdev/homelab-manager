package backuprunner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"homelab-manager/backend/internal/adapters/sshkey"
	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"

	"golang.org/x/crypto/ssh"
)

// sshOptions are given to every ssh that restic starts for an SFTP
// repository (decision D-13): only the key of the Manager and the pinned
// host keys count. No configuration file, agent, password or unknown host,
// no forwarding and no proxy; restic adds the host, the port, the user and
// "-s sftp" itself.
func sshOptions(identity, knownHosts string) []string {
	return []string{
		"-F", "/dev/null",
		"-i", identity,
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "BatchMode=yes",
		"-o", "PubkeyAuthentication=yes",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHosts,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "UpdateHostKeys=no",
		"-o", "CheckHostIP=no",
		"-o", "ConnectTimeout=30",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=4",
		"-o", "ForwardAgent=no",
		"-o", "ForwardX11=no",
		"-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no",
		"-o", "ProxyCommand=none",
		"-o", "ProxyJump=none",
		"-o", "ControlMaster=no",
		"-o", "ControlPath=none",
		"-o", "LogLevel=ERROR",
	}
}

// prepareSSH writes the key of the Manager and the pinned host keys of the
// SFTP repositories of the job into a new private directory, and returns
// the restic options that point ssh to them. cleanup removes the
// directory; it is a no-op without SFTP.
func prepareSSH(config Config, job backupjob.Job, privateKey string) (options []string, cleanup func(), err error) {
	cleanup = func() {}
	if !job.UsesSFTP() {
		return nil, cleanup, nil
	}
	if _, err := ssh.ParsePrivateKey([]byte(privateKey)); err != nil {
		return nil, cleanup, errors.New("the SSH key of the Manager cannot be read")
	}
	repositories := []domain.BackupRepository{job.BackupRepository()}
	if job.Copy != nil {
		repositories = append(repositories, job.Copy.BackupRepository())
	}
	var hosts strings.Builder
	var keys sshkey.Keys
	for _, repository := range repositories {
		_, host, port, ok := repository.SFTPEndpoint()
		if !ok {
			continue
		}
		normalized, err := keys.NormalizeHostKey(repository.HostKey)
		if err != nil || normalized != repository.HostKey {
			return nil, cleanup, fmt.Errorf("invalid host key for %s", host)
		}
		name := host
		if port != "" && port != "22" {
			name = "[" + host + "]:" + port
		}
		fmt.Fprintf(&hosts, "%s %s\n", name, normalized)
	}
	// restic splits sftp.args on blanks.
	if strings.ContainsAny(config.TempDir, " \t\n'\"\\") {
		return nil, cleanup, errors.New("the temporary directory must not contain blanks or quotes")
	}
	dir, err := os.MkdirTemp(config.TempDir, "homelab-ssh-")
	if err != nil {
		return nil, cleanup, fmt.Errorf("create the SSH directory: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	identity, knownHosts := filepath.Join(dir, "id_ed25519"), filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(identity, []byte(privateKey), 0o600); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("write the SSH key: %w", err)
	}
	if err := os.WriteFile(knownHosts, []byte(hosts.String()), 0o600); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("write the host keys: %w", err)
	}
	return []string{"-o", "sftp.args=" + strings.Join(sshOptions(identity, knownHosts), " ")}, cleanup, nil
}
