package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"router-app/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("providers: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen: %q", cfg.Listen)
	}
	if cfg.VirtualModel != "auto" {
		t.Fatalf("virtual: %q", cfg.VirtualModel)
	}
}
