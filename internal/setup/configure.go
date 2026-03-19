// Package setup implements the tool configuration logic for the setup wizard.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marijus001/tokara/internal/detect"
)

const (
	markerStart = "# >>> tokara >>>"
	markerEnd   = "# <<< tokara <<<"
)

// ConfigResult holds the outcome of configuring a tool.
type ConfigResult struct {
	Tool    string
	Success bool
	Details string
	Backup  string
}

// ConfigureTool applies Tokara's gateway configuration to a detected tool.
// For env-based tools, patches the user's shell profile with SDK env vars.
func ConfigureTool(tool detect.Tool, gatewayURL string) ConfigResult {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		return configureEnv(tool, gatewayURL)
	case detect.ConfigNote:
		return ConfigResult{Tool: tool.Name, Success: true, Details: tool.Note}
	default:
		return ConfigResult{Tool: tool.Name, Success: false, Details: "unknown config type"}
	}
}

// configureEnv adds SDK env var exports to the user's shell profile
// between tokara markers so they can be cleanly removed later.
func configureEnv(tool detect.Tool, gatewayURL string) ConfigResult {
	profilePath := detect.ShellProfile()

	existing, _ := os.ReadFile(profilePath)
	content := string(existing)

	// Already configured?
	if strings.Contains(content, markerStart) {
		// Check if the gateway URL matches
		if strings.Contains(content, gatewayURL) {
			return ConfigResult{Tool: tool.Name, Success: true, Details: "Already configured in " + profilePath}
		}
		// Different URL — replace the block
		content = removeMarkerBlock(content)
	}

	// Build the export block
	block := buildEnvBlock(tool.EnvVars)

	// Backup
	backup := ""
	if len(existing) > 0 {
		backup = profilePath + ".tokara-backup"
		os.WriteFile(backup, existing, 0644)
	}

	// Append to profile
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += block

	if err := os.WriteFile(profilePath, []byte(content), 0644); err != nil {
		return ConfigResult{Tool: tool.Name, Success: false, Details: fmt.Sprintf("Cannot write %s: %v", profilePath, err)}
	}

	return ConfigResult{
		Tool:    tool.Name,
		Success: true,
		Details: fmt.Sprintf("Added env vars to %s (open a new terminal to apply)", filepath.Base(profilePath)),
		Backup:  backup,
	}
}

// buildEnvBlock creates the marker-wrapped export block.
func buildEnvBlock(envVars map[string]string) string {
	var lines []string
	lines = append(lines, markerStart)
	for varName, val := range envVars {
		lines = append(lines, fmt.Sprintf("export %s=\"%s\"", varName, val))
	}
	lines = append(lines, markerEnd)
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

// removeMarkerBlock strips the tokara marker block from content.
func removeMarkerBlock(content string) string {
	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content, markerEnd)
	if startIdx < 0 || endIdx < 0 || endIdx < startIdx {
		return content
	}
	endIdx += len(markerEnd)
	// Also consume trailing newline
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	return content[:startIdx] + content[endIdx:]
}

// Unconfigure removes Tokara SDK env vars from the shell profile.
func Unconfigure(tool detect.Tool) error {
	if tool.ConfigType != detect.ConfigEnv {
		return nil
	}

	profilePath := detect.ShellProfile()
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil
	}

	content := string(data)
	if !strings.Contains(content, markerStart) {
		return nil
	}

	cleaned := removeMarkerBlock(content)
	return os.WriteFile(profilePath, []byte(cleaned), 0644)
}

// IsToolConfigured checks if a tool is currently configured to use the tokara gateway.
func IsToolConfigured(tool detect.Tool, gatewayURL string) bool {
	if tool.ConfigType != detect.ConfigEnv {
		return false
	}

	profilePath := detect.ShellProfile()
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return false
	}

	content := string(data)
	return strings.Contains(content, markerStart) && strings.Contains(content, gatewayURL)
}

// UnconfigureTool restores a tool's original configuration.
func UnconfigureTool(tool detect.Tool) error {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		return Unconfigure(tool)
	case detect.ConfigNote:
		return fmt.Errorf("%s requires manual configuration", tool.Name)
	default:
		return fmt.Errorf("unknown config type for %s", tool.Name)
	}
}

// DiffPreview returns a string showing what changes will be made.
func DiffPreview(tool detect.Tool, gatewayURL string) string {
	if tool.ConfigType != detect.ConfigEnv {
		return ""
	}
	profilePath := detect.ShellProfile()
	var lines []string
	lines = append(lines, "  File: "+profilePath)
	for varName, val := range tool.EnvVars {
		lines = append(lines, fmt.Sprintf("  Set:  export %s=\"%s\"", varName, val))
	}
	return strings.Join(lines, "\n")
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
