package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Port != 18741 {
		t.Errorf("expected port 18741, got %d", cfg.Port)
	}
	if cfg.CompactionThreshold != 0.80 {
		t.Errorf("expected threshold 0.80, got %f", cfg.CompactionThreshold)
	}
	if cfg.PrecomputeThreshold != 0.60 {
		t.Errorf("expected precompute 0.60, got %f", cfg.PrecomputeThreshold)
	}
}

func TestLoadFromTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	toml := "port = 9999\napi_key = \"tk_live_test123\"\ncompaction_threshold = 0.75\n"
	os.WriteFile(path, []byte(toml), 0644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Port)
	}
	if cfg.APIKey != "tk_live_test123" {
		t.Errorf("expected api key, got %s", cfg.APIKey)
	}
	if cfg.CompactionThreshold != 0.75 {
		t.Errorf("expected threshold 0.75, got %f", cfg.CompactionThreshold)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("TOKARA_PORT", "7777")
	t.Setenv("TOKARA_API_KEY", "tk_live_envkey")
	cfg := Defaults()
	cfg.ApplyEnv()
	if cfg.Port != 7777 {
		t.Errorf("expected port 7777, got %d", cfg.Port)
	}
	if cfg.APIKey != "tk_live_envkey" {
		t.Errorf("expected env api key, got %s", cfg.APIKey)
	}
}
