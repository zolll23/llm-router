package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	ID         string `yaml:"id"`
	Type       string `yaml:"type"`
	BaseURL    string `yaml:"base_url"`
	APIKeyEnv  string `yaml:"api_key_env"`
	ConfigPath string `yaml:"config_path"`
	Command    string `yaml:"command"`
}

type Config struct {
	Listen       string           `yaml:"listen"`
	DataDir      string           `yaml:"data_dir"`
	VirtualModel string           `yaml:"virtual_model"`
	AdminToken   string           `yaml:"admin_token"`
	APIToken     string           `yaml:"api_token"`
	Providers    []ProviderConfig `yaml:"providers"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	cfg := &Config{
		Listen:       "127.0.0.1:8080",
		VirtualModel: "auto",
		DataDir:      filepath.Join(home, ".local/share/router"),
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8080"
	}
	if cfg.VirtualModel == "" {
		cfg.VirtualModel = "auto"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(home, ".local/share/router")
	}
	if strings.HasPrefix(cfg.DataDir, "~/") {
		cfg.DataDir = filepath.Join(home, cfg.DataDir[2:])
	}
	return cfg, nil
}
