package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	HTTPAddress string
	DatabaseURL string
	StartedAt   time.Time
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

	_ = v.ReadInConfig()

	databaseURL := v.GetString("database_url")
	if databaseURL == "" {
		databaseURL = "postgres://homelab_manager:change-me@127.0.0.1:5432/homelab_manager?sslmode=disable"
	}

	return Config{
		HTTPAddress: v.GetString("http_address"),
		DatabaseURL: databaseURL,
		StartedAt:   time.Now().UTC(),
	}, nil
}

func (c Config) Validate() error {
	if c.HTTPAddress == "" {
		return fmt.Errorf("HTTP address is required")
	}
	return nil
}
