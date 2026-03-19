// Package setup implements the tool configuration logic for the setup wizard.
package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marijus001/tokara/internal/detect"
)

// ConfigResult holds the outcome of configuring a tool.
type ConfigResult struct {
	Tool    string
	Success bool
	Details string
	Backup  string
}

// ConfigureTool applies Tokara's gateway configuration to a detected tool.
// For env-based tools, patches .vscode/settings.json with SDK env vars.
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

// configureEnv patches .vscode/settings.json with terminal.integrated.env.*
// so any terminal opened in VS Code/Cursor/Windsurf routes SDK calls through the proxy.
func configureEnv(tool detect.Tool, gatewayURL string) ConfigResult {
	settingsPath := filepath.Join(".vscode", "settings.json")

	var settings map[string]interface{}
	existing, err := os.ReadFile(settingsPath)
	if err == nil {
		if jsonErr := json.Unmarshal(existing, &settings); jsonErr != nil {
			settings = make(map[string]interface{})
		}
	} else {
		settings = make(map[string]interface{})
	}

	// Check if already configured
	for _, platform := range []string{"osx", "linux", "windows"} {
		key := "terminal.integrated.env." + platform
		if env, ok := settings[key].(map[string]interface{}); ok {
			for varName := range tool.EnvVars {
				if v, exists := env[varName]; exists && v == gatewayURL {
					return ConfigResult{Tool: tool.Name, Success: true, Details: "Already configured in .vscode/settings.json"}
				}
			}
		}
	}

	// Backup existing settings
	backup := ""
	if err == nil && len(existing) > 0 {
		backup = settingsPath + ".tokara-backup"
		os.WriteFile(backup, existing, 0644)
	}

	// Set SDK env vars for all platforms
	for _, platform := range []string{"osx", "linux", "windows"} {
		key := "terminal.integrated.env." + platform
		env, ok := settings[key].(map[string]interface{})
		if !ok {
			env = make(map[string]interface{})
		}
		for varName, val := range tool.EnvVars {
			env[varName] = val
		}
		settings[key] = env
	}

	os.MkdirAll(".vscode", 0755)
	data, _ := json.MarshalIndent(settings, "", "  ")
	if writeErr := os.WriteFile(settingsPath, append(data, '\n'), 0644); writeErr != nil {
		return ConfigResult{Tool: tool.Name, Success: false, Details: fmt.Sprintf("Cannot write %s: %v", settingsPath, writeErr)}
	}

	return ConfigResult{
		Tool:    tool.Name,
		Success: true,
		Details: "Patched .vscode/settings.json (restart IDE terminal to apply)",
		Backup:  backup,
	}
}

// Unconfigure removes Tokara SDK env vars from .vscode/settings.json.
func Unconfigure(tool detect.Tool) error {
	if tool.ConfigType != detect.ConfigEnv {
		return nil
	}

	settingsPath := filepath.Join(".vscode", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings map[string]interface{}
	if json.Unmarshal(data, &settings) != nil {
		return nil
	}

	changed := false
	for _, platform := range []string{"osx", "linux", "windows"} {
		key := "terminal.integrated.env." + platform
		if env, ok := settings[key].(map[string]interface{}); ok {
			for varName := range tool.EnvVars {
				if _, exists := env[varName]; exists {
					delete(env, varName)
					changed = true
				}
			}
			if len(env) == 0 {
				delete(settings, key)
			} else {
				settings[key] = env
			}
		}
	}

	if !changed {
		return nil
	}

	out, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
}

// IsToolConfigured checks if a tool is currently configured to use the tokara gateway.
func IsToolConfigured(tool detect.Tool, gatewayURL string) bool {
	if tool.ConfigType != detect.ConfigEnv {
		return false
	}

	settingsPath := filepath.Join(".vscode", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}
	var settings map[string]interface{}
	if json.Unmarshal(data, &settings) != nil {
		return false
	}
	for _, platform := range []string{"osx", "linux", "windows"} {
		key := "terminal.integrated.env." + platform
		if env, ok := settings[key].(map[string]interface{}); ok {
			for varName := range tool.EnvVars {
				if v, exists := env[varName]; exists && v == gatewayURL {
					return true
				}
			}
		}
	}
	return false
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
	var lines []string
	lines = append(lines, "  File: .vscode/settings.json")
	for varName, val := range tool.EnvVars {
		lines = append(lines, fmt.Sprintf("  Set:  %s = %s", varName, val))
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
