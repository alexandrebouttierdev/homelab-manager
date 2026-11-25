package backuprunner

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"homelab-manager/backend/internal/adapters/sshkey"
	"homelab-manager/backend/internal/backupjob"
	"homelab-manager/backend/internal/domain"

	"golang.org/x/crypto/ssh"
)

func hostKeyOf(t *testing.T, signer ssh.Signer) string {
	t.Helper()
	key, err := (sshkey.Keys{}).NormalizeHostKey(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func managerKey(t *testing.T) (private, public string) {
	t.Helper()
	private, public, err := (sshkey.Keys{}).Generate(domain.BackupSSHKeyComment)
	if err != nil {
		t.Fatal(err)
	}
	return private, public
}

func sftpCopyJob(mainHostKey, copyHostKey string) backupjob.Job {
	job := repositoryJob(domain.BackupOpCopy)
	job.RepositoryType, job.Repository, job.HostKey = domain.RepositorySFTP, "sftp://backup@nas.lan:2222/srv/restic", mainHostKey
	job.Copy = &backupjob.Copy{
		RepositoryType: domain.RepositorySFTP, Repository: "sftp://copy@backup.example.org/srv/copy", HostKey: copyHostKey,
	}
	return job
}

func TestSFTPGivesSSHTheManagerKeyAndThePinnedHostKeysOnly(t *testing.T) {
	f := newFixture(t)
	f.file("snapshots.out", snapshotsJSON)
	private, _ := managerKey(t)
	mainHost, copyHost := hostKeyOf(t, newSigner(t)), hostKeyOf(t, newSigner(t))
	f.job(sftpCopyJob(mainHost, copyHost), domain.BackupCredentials{})
	f.secrets(domain.BackupOpCopy, backupjob.Secrets{
		BackupCredentials: domain.BackupCredentials{Password: "main-pass"},
		Copy:              &domain.BackupCredentials{Password: "copy-pass"}, SSHKey: private,
	})

	status, result, stderr := f.run("copy")
	if status != 0 || !result.Success {
		t.Fatalf("copy: %d %+v %s", status, result, stderr)
	}
	args := strings.Split(strings.TrimSpace(f.read("args.copy")), "\n")
	if len(args) < 3 || args[0] != "-o" || !strings.HasPrefix(args[1], "sftp.args=-F /dev/null -i ") || args[2] != "copy" {
		t.Fatalf("args = %q", args)
	}
	for _, option := range []string{
		"IdentitiesOnly=yes", "IdentityAgent=none", "BatchMode=yes", "PasswordAuthentication=no",
		"StrictHostKeyChecking=yes", "GlobalKnownHostsFile=/dev/null", "UpdateHostKeys=no", "ProxyCommand=none",
		"ForwardAgent=no", "ClearAllForwardings=yes", "PermitLocalCommand=no",
	} {
		if !strings.Contains(args[1], " -o "+option) {
			t.Errorf("sftp.args misses %s: %s", option, args[1])
		}
	}
	if f.read("ssh.identity") != private {
		t.Fatal("the identity is not the key of the Manager")
	}
	if got, want := f.read("ssh.known_hosts"), "[nas.lan]:2222 "+mainHost+"\nbackup.example.org "+copyHost+"\n"; got != want {
		t.Fatalf("known_hosts = %q, want %q", got, want)
	}
	if modes := f.read("ssh.modes"); modes != "-rw-------\n-rw-------\n" {
		t.Fatalf("modes = %q", modes)
	}
	env := f.read("env.copy")
	if !strings.Contains(env, "RESTIC_REPOSITORY=sftp://copy@backup.example.org//srv/copy\n") ||
		!strings.Contains(env, "RESTIC_FROM_REPOSITORY=sftp://backup@nas.lan:2222//srv/restic\n") {
		t.Fatalf("env = %s", env)
	}
	if left, _ := filepath.Glob(filepath.Join(f.config.TempDir, "homelab-ssh-*")); len(left) != 0 {
		t.Fatalf("the SSH directory stays: %v", left)
	}
}

func TestSFTPRefusals(t *testing.T) {
	private, _ := managerKey(t)
	hostKey := hostKeyOf(t, newSigner(t))
	init := backupjob.Job{
		Version: 1, Operation: domain.BackupOpInit, RepositoryType: domain.RepositorySFTP,
		Repository: "sftp://backup@nas.lan/srv/restic", HostKey: hostKey,
	}
	local := backupjob.Job{Version: 1, Operation: domain.BackupOpInit, RepositoryType: domain.RepositoryLocal, Repository: "/mnt/backup"}
	withComment := init
	withComment.HostKey = hostKey + " root@nas"
	for name, c := range map[string]struct {
		job    backupjob.Job
		sshKey string
		want   string
	}{
		"no SSH key":               {init, "", "SSH key belongs"},
		"SSH key without SFTP":     {local, private, "SSH key belongs"},
		"not an OpenSSH key":       {init, "-----BEGIN RSA PRIVATE KEY-----\n", "OpenSSH private key"},
		"unreadable key":           {init, "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n", "cannot be read"},
		"host key not in its form": {withComment, private, "invalid host key"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.job(c.job, domain.BackupCredentials{})
			f.secrets(domain.BackupOpInit, backupjob.Secrets{BackupCredentials: domain.BackupCredentials{Password: "p"}, SSHKey: c.sshKey})
			status, result, stderr := f.run("init")
			if status == 0 || result.Success || !strings.Contains(stderr, c.want) {
				t.Fatalf("status %d, %+v, %s", status, result, stderr)
			}
			if _, err := os.Stat(filepath.Join(f.bin, "args.init")); err == nil {
				t.Fatal("restic ran")
			}
			if strings.Contains(stderr, "OPENSSH PRIVATE KEY-----\nAAAA") {
				t.Fatal("the key leaks")
			}
		})
	}
}

// sshServer is an SSH server on the loopback that accepts one client key
// and answers the "sftp" subsystem with a fixed line.
type sshServer struct {
	listener net.Listener
	logins   chan string
	wg       sync.WaitGroup
}

const sftpGreeting = "sftp subsystem reached"

func startSSHServer(t *testing.T, hostKey ssh.Signer, allowed ssh.PublicKey) *sshServer {
	t.Helper()
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() == "backup" && bytes.Equal(key.Marshal(), allowed.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown key")
		},
	}
	config.AddHostKey(hostKey)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &sshServer{listener: listener, logins: make(chan string, 10)}
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			server.wg.Add(1)
			go func() {
				defer server.wg.Done()
				server.serve(conn, config)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		server.wg.Wait()
	})
	return server
}

func (s *sshServer) serve(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer serverConn.Close()
	s.logins <- serverConn.User()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			return
		}
		for request := range channelRequests {
			var payload struct{ Name string }
			if request.Type != "subsystem" || ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
				_ = request.Reply(false, nil)
				continue
			}
			_ = request.Reply(true, nil)
			_, _ = channel.Write([]byte(sftpGreeting + "\n"))
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			_ = channel.Close()
			return
		}
	}
}

// runSSH runs ssh as restic does: host, port and user, then sftp.args,
// then "-s sftp".
func runSSH(t *testing.T, port int, options []string) (string, error) {
	t.Helper()
	args := append([]string{"127.0.0.1", "-p", strconv.Itoa(port), "-l", "backup"}, options...)
	args = append(args, "-s", "sftp")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), errors.New(err.Error() + ": " + stderr.String())
	}
	return stdout.String(), nil
}

// TestSSHOptionsWithTheRealClient runs the ssh of the machine with the
// options the runner gives restic: the key of the Manager logs in to a
// server whose host key is pinned; another host key or another client
// key is refused.
func TestSSHOptionsWithTheRealClient(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh is not installed")
	}
	private, public := managerKey(t)
	clientKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(public))
	if err != nil {
		t.Fatal(err)
	}
	hostSigner := newSigner(t)
	server := startSSHServer(t, hostSigner, clientKey)
	port := server.listener.Addr().(*net.TCPAddr).Port

	prepare := func(hostKey, key string) []string {
		job := backupjob.Job{
			Version: 1, Operation: domain.BackupOpInit, RepositoryType: domain.RepositorySFTP,
			Repository: "sftp://backup@127.0.0.1:" + strconv.Itoa(port) + "/srv/restic", HostKey: hostKey,
		}
		options, cleanup, err := prepareSSH(Config{TempDir: t.TempDir()}, job, key)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cleanup)
		value, ok := strings.CutPrefix(options[1], "sftp.args=")
		if len(options) != 2 || options[0] != "-o" || !ok {
			t.Fatalf("options = %q", options)
		}
		return strings.Fields(value)
	}

	output, err := runSSH(t, port, prepare(hostKeyOf(t, hostSigner), private))
	if err != nil || strings.TrimSpace(output) != sftpGreeting {
		t.Fatalf("pinned host and Manager key: %q, %v", output, err)
	}
	if login := <-server.logins; login != "backup" {
		t.Fatalf("login = %q", login)
	}

	if output, err := runSSH(t, port, prepare(hostKeyOf(t, newSigner(t)), private)); err == nil || output != "" {
		t.Fatalf("another host key accepted: %q", output)
	}
	select {
	case login := <-server.logins:
		t.Fatalf("logged in as %s despite the host key", login)
	default:
	}

	otherPrivate, _ := managerKey(t)
	if output, err := runSSH(t, port, prepare(hostKeyOf(t, hostSigner), otherPrivate)); err == nil || output != "" {
		t.Fatalf("another client key accepted: %q", output)
	}
}
