package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPAddress                    string
	DatabasePath                   string
	StartedAt                      time.Time
	AdminUsername                  string
	AdminPassword                  string
	SessionSecure                  bool
	TLSCertFile                    string
	TLSKeyFile                     string
	DockerProxySocket              string
	WebRoot                        string
	SystemIntervalSeconds          int
	DockerStatsIntervalSeconds     int
	DockerDiscoveryIntervalSeconds int
	MetricsRetentionDays           int
	PowerActionsEnabled            bool
}

func LoadConfig() (Config, error) {
	v := viper.New()
	v.SetConfigName("manager")
	v.SetConfigType("yaml")
	// First match wins: system configuration, then a repository checkout.
	v.AddConfigPath("/etc/homelab-manager")
	v.AddConfigPath("./deployments/config")
	v.AddConfigPath(".")
	v.SetEnvPrefix("HOMELAB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	v.SetDefault("http_address", ":8080")
	v.SetDefault("database_path", "./data/homelab-manager.db")
	v.SetDefault("admin_username", "admin")
	v.SetDefault("admin_password", "")
	v.SetDefault("session_secure", false)
	v.SetDefault("tls_cert_file", "")
	v.SetDefault("tls_key_file", "")
	v.SetDefault("docker_proxy_socket", "/run/homelab-docker-proxy/docker.sock")
	v.SetDefault("web_root", "./frontend/dist")
	v.SetDefault("system_collect_interval_seconds", 10)
	v.SetDefault("docker_collect_interval_seconds", 15)
	v.SetDefault("docker_discovery_interval_seconds", 30)
	v.SetDefault("metrics_retention_days", 7)
	// Rebooting and powering off also need the polkit rule of
	// deployments/polkit; both are off by default.
	v.SetDefault("power_actions_enabled", false)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return Config{}, fmt.Errorf("read Manager configuration: %w", err)
		}
	}

	config := Config{
		HTTPAddress:                    v.GetString("http_address"),
		DatabasePath:                   v.GetString("database_path"),
		StartedAt:                      time.Now().UTC(),
		AdminUsername:                  v.GetString("admin_username"),
		AdminPassword:                  v.GetString("admin_password"),
		SessionSecure:                  v.GetBool("session_secure"),
		TLSCertFile:                    v.GetString("tls_cert_file"),
		TLSKeyFile:                     v.GetString("tls_key_file"),
		DockerProxySocket:              v.GetString("docker_proxy_socket"),
		WebRoot:                        v.GetString("web_root"),
		SystemIntervalSeconds:          v.GetInt("system_collect_interval_seconds"),
		DockerStatsIntervalSeconds:     v.GetInt("docker_collect_interval_seconds"),
		DockerDiscoveryIntervalSeconds: v.GetInt("docker_discovery_interval_seconds"),
		MetricsRetentionDays:           v.GetInt("metrics_retention_days"),
		PowerActionsEnabled:            v.GetBool("power_actions_enabled"),
	}
	if config.TLSCertFile != "" {
		config.SessionSecure = true
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// defaultAdminPassword is used only to create the administrator on a fresh
// database when HOMELAB_ADMIN_PASSWORD is empty. It must be changed from the
// Settings page; an existing account is never reset to it.
const defaultAdminPassword = "admin"

// BootstrapAdminPassword returns the password used to create the initial
// administrator and whether it is the built-in default.
func (c Config) BootstrapAdminPassword() (string, bool) {
	if c.AdminPassword == "" {
		return defaultAdminPassword, true
	}
	return c.AdminPassword, false
}

func (c Config) Validate() error {
	if c.HTTPAddress == "" {
		return fmt.Errorf("HTTP address is required")
	}
	if c.AdminUsername == "" {
		return fmt.Errorf("administrator username is required")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database path is required")
	}
	if c.AdminPassword != "" && (len(c.AdminPassword) < 12 || len(c.AdminPassword) > 1024) {
		return fmt.Errorf("administrator password must be between 12 and 1024 bytes")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("TLS certificate and key must be configured together")
	}
	if c.DockerProxySocket == "" {
		return fmt.Errorf("Docker proxy socket is required")
	}
	if c.SystemIntervalSeconds < 5 || c.SystemIntervalSeconds > 300 {
		return fmt.Errorf("system collection interval must be between 5 and 300 seconds")
	}
	if c.DockerStatsIntervalSeconds < 5 || c.DockerStatsIntervalSeconds > 300 {
		return fmt.Errorf("Docker statistics interval must be between 5 and 300 seconds")
	}
	if c.DockerDiscoveryIntervalSeconds < 10 || c.DockerDiscoveryIntervalSeconds > 600 {
		return fmt.Errorf("Docker discovery interval must be between 10 and 600 seconds")
	}
	if c.MetricsRetentionDays < 1 || c.MetricsRetentionDays > 365 {
		return fmt.Errorf("metrics retention must be between 1 and 365 days")
	}
	return nil
}
