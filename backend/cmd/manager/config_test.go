package main

import "testing"

func TestConfigValidateCollectionBounds(t *testing.T) {
	config := Config{
		HTTPAddress: ":8080", AdminUsername: "admin", DatabasePath: "/var/lib/homelab-manager/homelab-manager.db", DockerProxySocket: "/run/homelab-docker-proxy/docker.sock",
		SystemIntervalSeconds: 10, DockerStatsIntervalSeconds: 15,
		DockerDiscoveryIntervalSeconds: 30, MetricsRetentionDays: 30,
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
