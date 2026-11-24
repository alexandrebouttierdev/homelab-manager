// Package dbustest runs a private D-Bus daemon for the adapter tests that
// talk to systemd or logind: no test touches the system bus.
package dbustest

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// maxSocketDir keeps the bus socket path (directory, "/dbus-", 10 random
// characters) under sun_path: 104 bytes on macOS, 108 on Linux.
const maxSocketDir = 80

// busConfig is a session bus that activates services only from its own
// directory: the host's activation files (systemd installs one for
// org.freedesktop.systemd1 that runs /bin/false) must not change a test.
const busConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:dir=%s</listen>
  <servicedir>%s</servicedir>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// StartBus runs a private dbus-daemon for the test and returns its address.
// activatable maps a bus name to the program the bus runs to activate it.
// The test is skipped when dbus-daemon is missing; CI installs it.
func StartBus(t *testing.T, activatable map[string]string) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed")
	}
	dir := t.TempDir()
	if len(dir) > maxSocketDir {
		// macOS temporary directories are too long for a socket path.
		dir, err = os.MkdirTemp("/tmp", "hm-dbus-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}
	services := filepath.Join(dir, "services")
	if err := os.Mkdir(services, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, program := range activatable {
		file := fmt.Sprintf("[D-BUS Service]\nName=%s\nExec=%s\n", name, program)
		if err := os.WriteFile(filepath.Join(services, name+".service"), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(config, []byte(fmt.Sprintf(busConfig, dir, services)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--print-address=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()
	select {
	case address := <-lines:
		if address == "" {
			t.Fatal("dbus-daemon printed no address")
		}
		return address
	case <-time.After(10 * time.Second):
		t.Fatal("dbus-daemon did not start")
	}
	return ""
}
