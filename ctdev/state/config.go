package state

import (
	"github.com/spf13/viper"
)

type Config struct {
	DefaultComponents []string `mapstructure:"default_components"`
	UpdateInterval    string   `mapstructure:"update_interval"`
}

func LoadConfig() Config {
	var cfg Config
	cfg.DefaultComponents = viper.GetStringSlice("default_components")
	cfg.UpdateInterval = viper.GetString("update_interval")
	if cfg.UpdateInterval == "" {
		cfg.UpdateInterval = "weekly"
	}
	return cfg
}
