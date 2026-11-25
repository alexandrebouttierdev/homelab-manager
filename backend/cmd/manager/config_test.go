package main

import "testing"

func TestConfigValidateCollectionBounds(t *testing.T) {
	config := Config{
		HTTPAddress: ":8080", AdminUsername: "admin", DatabasePath: "/var/lib/homelab-manager/homelab-manager.db", DockerProxySocket: "/run/homelab-docker-proxy/docker.sock",
		SystemIntervalSeconds: 10, DockerStatsIntervalSeconds: 15,
		DockerDiscoveryIntervalSeconds: 30, MetricsRetentionDays: 30, VulnerabilityScanIntervalHours: 24,
		VulnscanInventoryPath: "/var/lib/homelab-vulnscan/inventory/inventory.json", VulnscanResultPath: "/var/lib/homelab-vulnscan/result/result.json",
		BackupJobsDir: "/var/lib/homelab-backup/jobs", BackupRunsDir: "/var/lib/homelab-backup/runs",
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"system interval too short", func(c *Config) { c.SystemIntervalSeconds = 4 }},
		{"Docker stats interval too long", func(c *Config) { c.DockerStatsIntervalSeconds = 301 }},
		{"Docker discovery interval too short", func(c *Config) { c.DockerDiscoveryIntervalSeconds = 9 }},
		{"retention too long", func(c *Config) { c.MetricsRetentionDays = 366 }},
		{"missing administrator username", func(c *Config) { c.AdminUsername = "" }},
		{"missing database path", func(c *Config) { c.DatabasePath = "" }},
		{"missing proxy socket", func(c *Config) { c.DockerProxySocket = "" }},
		{"administrator password too short", func(c *Config) { c.AdminPassword = "short" }},
		{"scan interval too short", func(c *Config) { c.VulnerabilityScanIntervalHours = 5 }},
		{"scan interval too long", func(c *Config) { c.VulnerabilityScanIntervalHours = 169 }},
		{"relative inventory path", func(c *Config) { c.VulnscanInventoryPath = "inventory.json" }},
		{"relative backup jobs directory", func(c *Config) { c.BackupJobsDir = "jobs" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := config
			test.change(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestBootstrapAdminPassword(t *testing.T) {
	if password, isDefault := (Config{}).BootstrapAdminPassword(); password != "admin" || !isDefault {
		t.Fatalf("empty configuration = (%q, %v), want default admin password", password, isDefault)
	}
	configured := Config{AdminPassword: "a-strong-configured-password"}
	if password, isDefault := configured.BootstrapAdminPassword(); password != configured.AdminPassword || isDefault {
		t.Fatalf("configured password = (%q, %v), want configured value", password, isDefault)
	}
}

func TestDefaultRetentionIsSevenDays(t *testing.T) {
	t.Setenv("HOMELAB_ADMIN_PASSWORD", "")
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.MetricsRetentionDays != 7 {
		t.Fatalf("default retention = %d days, want 7", config.MetricsRetentionDays)
	}
}

func TestPowerActionsAreDisabledByDefault(t *testing.T) {
	t.Chdir(t.TempDir()) // no manager.yaml from the repository
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.PowerActionsEnabled {
		t.Fatal("power actions must be disabled unless explicitly enabled")
	}
	t.Setenv("HOMELAB_POWER_ACTIONS_ENABLED", "true")
	if config, err = LoadConfig(); err != nil || !config.PowerActionsEnabled {
		t.Fatalf("LoadConfig = (%v, %v), want power actions enabled", config.PowerActionsEnabled, err)
	}
}

func TestDockerActionsAreDisabledByDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	config, err := LoadConfig()
	if err != nil || config.DockerActionsEnabled {
		t.Fatalf("LoadConfig = (%v, %v), want container actions disabled", config.DockerActionsEnabled, err)
	}
	t.Setenv("HOMELAB_DOCKER_ACTIONS_ENABLED", "true")
	if config, err = LoadConfig(); err != nil || !config.DockerActionsEnabled {
		t.Fatalf("LoadConfig = (%v, %v), want container actions enabled", config.DockerActionsEnabled, err)
	}
}

func TestSecretsKeyPath(t *testing.T) {
	if got := (Config{SecretsKeyFile: "/etc/key"}).SecretsKeyPath("/run/credentials/x"); got != "/etc/key" {
		t.Fatalf("configured file = %q", got)
	}
	if got := (Config{}).SecretsKeyPath("/run/credentials/homelab-manager.service"); got != "/run/credentials/homelab-manager.service/homelab-manager-secrets-key" {
		t.Fatalf("systemd credential = %q", got)
	}
	if got := (Config{}).SecretsKeyPath(""); got != "" {
		t.Fatalf("no key = %q, want empty", got)
	}
}

func TestConfigValidateWebAuthn(t *testing.T) {
	base := Config{
		HTTPAddress: ":8080", AdminUsername: "admin", DatabasePath: "/var/lib/homelab-manager/homelab-manager.db", DockerProxySocket: "/run/homelab-docker-proxy/docker.sock",
		SystemIntervalSeconds: 10, DockerStatsIntervalSeconds: 15,
		DockerDiscoveryIntervalSeconds: 30, MetricsRetentionDays: 30, VulnerabilityScanIntervalHours: 24,
		VulnscanInventoryPath: "/var/lib/homelab-vulnscan/inventory/inventory.json", VulnscanResultPath: "/var/lib/homelab-vulnscan/result/result.json",
		BackupJobsDir: "/var/lib/homelab-backup/jobs", BackupRunsDir: "/var/lib/homelab-backup/runs",
		WebAuthnRPName: "Homelab Manager",
	}
	tests := []struct {
		name    string
		rpID    string
		origins []string
		valid   bool
	}{
		{"off", "", nil, true},
		{"domain and origin", "homelab.example", []string{"https://homelab.example:8443"}, true},
		{"subdomain origin", "example.org", []string{"https://manager.example.org"}, true},
		{"localhost over HTTP", "localhost", []string{"http://localhost:5173"}, true},
		{"origins without domain", "", []string{"https://homelab.example"}, false},
		{"IP address", "192.168.1.10", []string{"https://192.168.1.10"}, false},
		{"URL as domain", "https://homelab.example", []string{"https://homelab.example"}, false},
		{"no origin", "homelab.example", nil, false},
		{"other domain", "homelab.example", []string{"https://evil.example"}, false},
		{"suffix without dot", "homelab.example", []string{"https://myhomelab.example"}, false},
		{"plain HTTP", "homelab.example", []string{"http://homelab.example"}, false},
		{"origin with a path", "homelab.example", []string{"https://homelab.example/app"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base
			config.WebAuthnRPID, config.WebAuthnOrigins = test.rpID, test.origins
			if err := config.Validate(); (err == nil) != test.valid {
				t.Fatalf("Validate() = %v, want valid %v", err, test.valid)
			}
		})
	}
}
