package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"homelab-manager/backend/internal/domain"
)

func TestPolkitRuleListsTheUnitsAndVerbs(t *testing.T) {
	units, err := domain.NewManageableUnits([]string{"nginx.service", "backup.timer"})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := polkitRule(units)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`var units = ["backup.timer","nginx.service"];`,
		`var verbs = ["start","stop","restart","reload"];`,
		`action.id === "org.freedesktop.systemd1.manage-units"`,
		`subject.user === "homelab-manager"`,
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("rule lacks %s:\n%s", want, rule)
		}
	}
	empty, err := polkitRule(domain.ManageableUnits{})
	if err != nil || !strings.Contains(empty, "var units = [];") {
		t.Fatalf("empty rule = %v\n%s", err, empty)
	}
}

// evaluator runs a rule under node with a stand-in for polkit and prints
// the result of each case: the generated JavaScript is checked as polkit
// would run it.
const evaluator = `
const fs = require("fs");
let rule;
global.polkit = { Result: { YES: "yes" }, addRule: (f) => { rule = f; } };
eval(fs.readFileSync(process.argv[2], "utf8"));
const cases = JSON.parse(process.argv[3]);
for (const c of cases) {
  const action = { id: c.id, lookup: (k) => ({ unit: c.unit, verb: c.verb })[k] };
  console.log(String(rule(action, { user: c.user })));
}
`

func TestPolkitRuleGrantsOnlyTheListedActions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	units, _ := domain.NewManageableUnits([]string{"nginx.service", "getty@tty1.service"})
	rule, err := polkitRule(units)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rulePath, scriptPath := filepath.Join(dir, "rule.js"), filepath.Join(dir, "evaluate.js")
	if err := os.WriteFile(rulePath, []byte(rule), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte(evaluator), 0o600); err != nil {
		t.Fatal(err)
	}
	const manage = "org.freedesktop.systemd1.manage-units"
	cases := []struct {
		id, user, unit, verb, want string
	}{
		{manage, "homelab-manager", "nginx.service", "restart", "yes"},
		{manage, "homelab-manager", "getty@tty1.service", "start", "yes"},
		{manage, "homelab-manager", "nginx.service", "reload", "yes"},
		{manage, "homelab-manager", "ssh.service", "restart", "undefined"},
		{manage, "homelab-manager", "nginx.service", "enable", "undefined"},
		{manage, "alice", "nginx.service", "restart", "undefined"},
		{"org.freedesktop.systemd1.manage-unit-files", "homelab-manager", "nginx.service", "start", "undefined"},
		{"org.freedesktop.systemd1.reload-daemon", "homelab-manager", "", "", "undefined"},
	}
	var encoded []string
	for _, c := range cases {
		encoded = append(encoded, fmt.Sprintf(`{"id":%q,"user":%q,"unit":%q,"verb":%q}`, c.id, c.user, c.unit, c.verb))
	}
	output, err := exec.Command(node, scriptPath, rulePath, "["+strings.Join(encoded, ",")+"]").CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, output)
	}
	results := strings.Fields(string(output))
	if len(results) != len(cases) {
		t.Fatalf("results = %v", results)
	}
	for i, c := range cases {
		if results[i] != c.want {
			t.Errorf("%s %s %s %s = %s, want %s", c.id, c.user, c.unit, c.verb, results[i], c.want)
		}
	}
}

func TestRunCommandPrintsTheRule(t *testing.T) {
	units, _ := domain.NewManageableUnits([]string{"nginx.service"})
	var stdout, stderr bytes.Buffer
	status := runCommand([]string{"polkit-rule"}, &stdout, &stderr, func() (Config, error) {
		return Config{ManageableUnits: units}, nil
	})
	if status != 0 || !strings.Contains(stdout.String(), `["nginx.service"]`) || stderr.Len() != 0 {
		t.Fatalf("status %d, stdout %q, stderr %q", status, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if status := runCommand([]string{"serve"}, &stdout, &stderr, nil); status != 2 || stdout.Len() != 0 {
		t.Fatalf("unknown command = %d %q", status, stdout.String())
	}
	if status := runCommand([]string{"polkit-rule"}, &stdout, &stderr, func() (Config, error) {
		return Config{}, errors.New("bad unit")
	}); status != 1 || stdout.Len() != 0 {
		t.Fatalf("configuration error = %d %q", status, stdout.String())
	}
}

func TestLoadConfigReadsTheAllowList(t *testing.T) {
	t.Setenv("HOMELAB_SYSTEMD_MANAGEABLE_UNITS", "nginx.service  backup.timer")
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := config.ManageableUnits.Names(); len(got) != 2 || got[0] != "backup.timer" || got[1] != "nginx.service" {
		t.Fatalf("allow list = %v", got)
	}
	t.Setenv("HOMELAB_SYSTEMD_MANAGEABLE_UNITS", "nginx.service ../etc/passwd")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "systemd_manageable_units") {
		t.Fatalf("invalid name = %v, want an error", err)
	}
}

func TestLoadConfigDefaultsToAnEmptyAllowList(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if names := config.ManageableUnits.Names(); len(names) != 0 {
		t.Fatalf("default allow list = %v, want empty", names)
	}
}

// TestSystemRuleGrantsOnlyTheThreeSettings evaluates the rule shipped for
// decision D-6.
func TestSystemRuleGrantsOnlyTheThreeSettings(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "evaluate.js")
	if err := os.WriteFile(scriptPath, []byte(evaluator), 0o600); err != nil {
		t.Fatal(err)
	}
	rulePath := filepath.Join("..", "..", "..", "deployments", "polkit", "50-homelab-manager-system.rules")
	cases := []struct{ id, user, want string }{
		{"org.freedesktop.hostname1.set-static-hostname", "homelab-manager", "yes"},
		{"org.freedesktop.timedate1.set-timezone", "homelab-manager", "yes"},
		{"org.freedesktop.locale1.set-locale", "homelab-manager", "yes"},
		{"org.freedesktop.hostname1.set-static-hostname", "alice", "undefined"},
		{"org.freedesktop.hostname1.set-hostname", "homelab-manager", "undefined"},
		{"org.freedesktop.hostname1.set-machine-info", "homelab-manager", "undefined"},
		{"org.freedesktop.timedate1.set-time", "homelab-manager", "undefined"},
		{"org.freedesktop.timedate1.set-ntp", "homelab-manager", "undefined"},
		{"org.freedesktop.locale1.set-keyboard", "homelab-manager", "undefined"},
		{"org.freedesktop.login1.reboot", "homelab-manager", "undefined"},
	}
	var encoded []string
	for _, c := range cases {
		encoded = append(encoded, fmt.Sprintf(`{"id":%q,"user":%q}`, c.id, c.user))
	}
	output, err := exec.Command(node, scriptPath, rulePath, "["+strings.Join(encoded, ",")+"]").CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, output)
	}
	results := strings.Fields(string(output))
	if len(results) != len(cases) {
		t.Fatalf("results = %v", results)
	}
	for i, c := range cases {
		if results[i] != c.want {
			t.Errorf("%s %s = %s, want %s", c.id, c.user, results[i], c.want)
		}
	}
}
