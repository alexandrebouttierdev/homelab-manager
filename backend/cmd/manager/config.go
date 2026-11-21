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
	DatabaseURL                    string
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
}

func LoadConfig() (Config, error) {
	v := viper.New()
	v.SetConfigName("manager")
	v.SetConfigType("yaml")
	v.AddConfigPath("./deployments/config")
	v.AddConfigPath(".")
	v.SetEnvPrefix("HOMELAB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	v.SetDefault("http_address", ":8080")
	v.SetDefault("database_url", "")
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
	v.SetDefault("metrics_retention_days", 30)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return Config{}, fmt.Errorf("read Manager configuration: %w", err)
		}
	}

	databaseURL := v.GetString("database_url")
	if databaseURL == "" {
		databaseURL = "postgres://homelab_manager:change-me@127.0.0.1:5432/homelab_manager?sslmode=disable"
	}

	config := Config{
		HTTPAddress:                    v.GetString("http_address"),
		DatabaseURL:                    databaseURL,
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
	}
	if config.TLSCertFile != "" {
		config.SessionSecure = true
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.HTTPAddress == "" {
		return fmt.Errorf("HTTP address is required")
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
