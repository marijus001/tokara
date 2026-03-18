package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                 int
	APIKey               string
	APIBase              string
	CompactionThreshold  float64
	PrecomputeThreshold  float64
	PreserveRecentTurns  int
	LogFile              string
}

func Defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Port:                18741,
		APIBase:             "https://api.tokara.dev",
		CompactionThreshold: 0.80,
		PrecomputeThreshold: 0.60,
		PreserveRecentTurns: 4,
		LogFile:             filepath.Join(home, ".tokara", "proxy.log"),
	}
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tokara", "config.toml")
}

func LoadFile(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch key {
		case "port":
			if v, err := strconv.Atoi(val); err == nil {
				cfg.Port = v
			}
		case "api_key":
			cfg.APIKey = val
		case "api_base":
			cfg.APIBase = val
		case "compaction_threshold":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				cfg.CompactionThreshold = v
			}
		case "precompute_threshold":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				cfg.PrecomputeThreshold = v
			}
		case "preserve_recent_turns":
			if v, err := strconv.Atoi(val); err == nil {
				cfg.PreserveRecentTurns = v
			}
		case "log_file":
			cfg.LogFile = val
		}
	}
	return cfg, scanner.Err()
}

func (c *Config) ApplyEnv() {
	if v := os.Getenv("TOKARA_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("TOKARA_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("TOKARA_API_BASE"); v != "" {
		c.APIBase = v
	}
}

func (c *Config) HasAPIKey() bool {
	return strings.HasPrefix(c.APIKey, "tk_live_") || strings.HasPrefix(c.APIKey, "tk_test_")
}
