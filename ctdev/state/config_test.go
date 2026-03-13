package state

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfigDefaults(t *testing.T) {
	viper.Reset()
	cfg := LoadConfig()
	if cfg.UpdateInterval != "weekly" {
		t.Errorf("expected weekly, got %s", cfg.UpdateInterval)
	}
	if len(cfg.DefaultComponents) != 0 {
		t.Errorf("expected no default components, got %v", cfg.DefaultComponents)
	}
}

func TestLoadConfigFromViper(t *testing.T) {
	viper.Reset()
	viper.Set("update_interval", "daily")
	viper.Set("default_components", []string{"zsh", "git"})
	cfg := LoadConfig()
	if cfg.UpdateInterval != "daily" {
		t.Errorf("expected daily, got %s", cfg.UpdateInterval)
	}
	if len(cfg.DefaultComponents) != 2 {
		t.Errorf("expected 2 default components, got %d", len(cfg.DefaultComponents))
	}
}
