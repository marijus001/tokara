// Package setup implements the setup wizard and config management.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigResult holds the outcome of a configuration operation.
type ConfigResult struct {
	Tool    string
	Success bool
	Details string
}

// SaveTokaraConfig writes the tokara config file.
func SaveTokaraConfig(apiKey string, port int) error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tokara")
	os.MkdirAll(dir, 0755)

	configPath := filepath.Join(dir, "config.toml")

	var lines []string
	lines = append(lines, fmt.Sprintf("port = %d", port))
	if apiKey != "" {
		lines = append(lines, fmt.Sprintf("api_key = \"%s\"", apiKey))
	}
	lines = append(lines, "compaction_threshold = 0.80")
	lines = append(lines, "precompute_threshold = 0.60")
	lines = append(lines, "preserve_recent_turns = 4")
	lines = append(lines, "")

	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0600)
}
