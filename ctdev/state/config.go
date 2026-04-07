package state

import (
	"github.com/spf13/viper"
)

type Config struct {
	DefaultComponents []string `mapstructure:"default_components"`
}

func LoadConfig() Config {
	var cfg Config
	cfg.DefaultComponents = viper.GetStringSlice("default_components")
	return cfg
}
