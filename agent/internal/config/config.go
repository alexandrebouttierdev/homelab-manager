package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	ManagerURL             string
	AgentToken             string
	CollectIntervalSeconds int
}

func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("agent")
	v.SetConfigType("yaml")
	v.AddConfigPath("/etc/homelab-manager")
	v.AddConfigPath(".")
	v.SetEnvPrefix("HOMELAB_AGENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	v.SetDefault("manager_url", "")
	v.SetDefault("agent_token", "")
	v.SetDefault("collect_interval_seconds", 30)

	_ = v.ReadInConfig()

	config := Config{
		ManagerURL:             v.GetString("manager_url"),
		AgentToken:             v.GetString("agent_token"),
		CollectIntervalSeconds: v.GetInt("collect_interval_seconds"),
	}
	if config.ManagerURL == "" {
		return Config{}, fmt.Errorf("manager URL is required")
	}
	if config.CollectIntervalSeconds < 5 {
		return Config{}, fmt.Errorf("collect interval must be at least 5 seconds")
	}
	return config, nil
}
