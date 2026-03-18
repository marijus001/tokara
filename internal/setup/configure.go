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
func ConfigureTool(tool detect.Tool, gatewayURL string) ConfigResult {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		return configureEnv(tool, gatewayURL)
	case detect.ConfigFile:
		return configureFile(tool, gatewayURL)
	case detect.ConfigNote:
		return ConfigResult{Tool: tool.Name, Success: true, Details: tool.Note}
	default:
		return ConfigResult{Tool: tool.Name, Success: false, Details: "unknown config type"}
	}
}

func configureEnv(tool detect.Tool, gatewayURL string) ConfigResult {
	profile := detect.ShellProfile()

	// Read existing profile
	existing, _ := os.ReadFile(profile)
	content := string(existing)

	// Build the lines to add
	var lines []string
	lines = append(lines, fmt.Sprintf("\n# Tokara proxy — %s", tool.Name))
	for varName := range tool.EnvVars {
		lines = append(lines, fmt.Sprintf("export %s=\"%s\"", varName, gatewayURL))
	}

	addition := strings.Join(lines, "\n") + "\n"

	// Check if already configured
	if strings.Contains(content, "# Tokara proxy") && strings.Contains(content, tool.Name) {
		return ConfigResult{
			Tool:    tool.Name,
			Success: true,
			Details: fmt.Sprintf("Already configured in %s", profile),
		}
	}

	// Backup existing profile
	backup := ""
	if len(existing) > 0 {
		backup = profile + ".tokara-backup"
		if err := os.WriteFile(backup, existing, 0600); err != nil {
			return ConfigResult{Tool: tool.Name, Success: false, Details: fmt.Sprintf("Cannot backup %s: %v", profile, err)}
		}
	}

	// Append to profile
	f, err := os.OpenFile(profile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return ConfigResult{Tool: tool.Name, Success: false, Details: fmt.Sprintf("Cannot write to %s: %v", profile, err)}
	}
	defer f.Close()

	if _, err := f.WriteString(addition); err != nil {
		return ConfigResult{Tool: tool.Name, Success: false, Details: fmt.Sprintf("Write error: %v", err)}
	}

	return ConfigResult{
		Tool:    tool.Name,
		Success: true,
		Details: fmt.Sprintf("Added to %s", profile),
		Backup:  backup,
	}
}

func configureFile(tool detect.Tool, gatewayURL string) ConfigResult {
	configPath := tool.ConfigPath

	switch tool.ID {
	case "openclaw":
		return patchOpenClaw(configPath, gatewayURL)
	case "opencode":
		return patchOpenCode(configPath, gatewayURL)
	default:
		return ConfigResult{Tool: tool.Name, Success: false, Details: "no patch function for this tool"}
	}
}

func patchOpenClaw(configPath, gatewayURL string) ConfigResult {
	existing, err := os.ReadFile(configPath)
	if err != nil {
		// Config file doesn't exist — create it
		dir := filepath.Dir(configPath)
		os.MkdirAll(dir, 0755)
		content := fmt.Sprintf("api_base: %s\n", gatewayURL)
		if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
			return ConfigResult{Tool: "OpenClaw", Success: false, Details: fmt.Sprintf("Cannot create %s: %v", configPath, err)}
		}
		return ConfigResult{Tool: "OpenClaw", Success: true, Details: fmt.Sprintf("Created %s", configPath)}
	}

	// Backup
	backup := configPath + ".tokara-backup"
	os.WriteFile(backup, existing, 0600)

	content := string(existing)

	// Replace or add api_base line
	if strings.Contains(content, "api_base:") {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "api_base:") {
				lines[i] = fmt.Sprintf("api_base: %s", gatewayURL)
			}
		}
		content = strings.Join(lines, "\n")
	} else {
		content += fmt.Sprintf("\napi_base: %s\n", gatewayURL)
	}

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return ConfigResult{Tool: "OpenClaw", Success: false, Details: fmt.Sprintf("Write error: %v", err)}
	}

	return ConfigResult{
		Tool:    "OpenClaw",
		Success: true,
		Details: fmt.Sprintf("Patched %s", configPath),
		Backup:  backup,
	}
}

func patchOpenCode(configPath, gatewayURL string) ConfigResult {
	existing, err := os.ReadFile(configPath)
	if err != nil {
		// Config file doesn't exist — create it
		dir := filepath.Dir(configPath)
		os.MkdirAll(dir, 0755)
		config := map[string]interface{}{
			"providers": map[string]interface{}{
				"default": map[string]interface{}{
					"apiBase": gatewayURL,
				},
			},
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			return ConfigResult{Tool: "OpenCode", Success: false, Details: fmt.Sprintf("Cannot create %s: %v", configPath, err)}
		}
		return ConfigResult{Tool: "OpenCode", Success: true, Details: fmt.Sprintf("Created %s", configPath)}
	}

	// Backup
	backup := configPath + ".tokara-backup"
	os.WriteFile(backup, existing, 0600)

	var config map[string]interface{}
	if err := json.Unmarshal(existing, &config); err != nil {
		return ConfigResult{Tool: "OpenCode", Success: false, Details: fmt.Sprintf("Invalid JSON: %v", err)}
	}

	// Set the provider base URL
	if providers, ok := config["providers"].(map[string]interface{}); ok {
		for name, p := range providers {
			if prov, ok := p.(map[string]interface{}); ok {
				prov["apiBase"] = gatewayURL
				providers[name] = prov
			}
		}
	} else {
		config["providers"] = map[string]interface{}{
			"default": map[string]interface{}{
				"apiBase": gatewayURL,
			},
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ConfigResult{Tool: "OpenCode", Success: false, Details: fmt.Sprintf("Marshal error: %v", err)}
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return ConfigResult{Tool: "OpenCode", Success: false, Details: fmt.Sprintf("Write error: %v", err)}
	}

	return ConfigResult{
		Tool:    "OpenCode",
		Success: true,
		Details: fmt.Sprintf("Patched %s", configPath),
		Backup:  backup,
	}
}

// Unconfigure removes Tokara configuration for env-based tools.
func Unconfigure(tool detect.Tool) error {
	if tool.ConfigType != detect.ConfigEnv {
		return nil
	}

	profile := detect.ShellProfile()
	data, err := os.ReadFile(profile)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var cleaned []string
	skip := false
	for _, line := range lines {
		if strings.Contains(line, "# Tokara proxy") {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(strings.TrimSpace(line), "export ") {
			continue
		}
		skip = false
		cleaned = append(cleaned, line)
	}

	return os.WriteFile(profile, []byte(strings.Join(cleaned, "\n")), 0644)
}

// IsToolConfigured checks if a tool is currently configured to use the tokara gateway.
func IsToolConfigured(tool detect.Tool, gatewayURL string) bool {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		profile := detect.ShellProfile()
		data, err := os.ReadFile(profile)
		if err != nil {
			return false
		}
		content := string(data)
		return strings.Contains(content, "# Tokara proxy") &&
			strings.Contains(content, tool.Name) &&
			strings.Contains(content, gatewayURL)

	case detect.ConfigFile:
		data, err := os.ReadFile(tool.ConfigPath)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), gatewayURL)

	default:
		return false
	}
}

// UnconfigureTool restores a tool's original configuration by removing tokara settings.
func UnconfigureTool(tool detect.Tool) error {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		return Unconfigure(tool)

	case detect.ConfigFile:
		backup := tool.ConfigPath + ".tokara-backup"
		data, err := os.ReadFile(backup)
		if err != nil {
			// No backup = tokara created this file from scratch. Delete it.
			os.Remove(tool.ConfigPath)
			return nil
		}
		if err := os.WriteFile(tool.ConfigPath, data, 0600); err != nil {
			return fmt.Errorf("failed to restore %s: %w", tool.ConfigPath, err)
		}
		os.Remove(backup)
		return nil

	case detect.ConfigNote:
		return fmt.Errorf("%s cannot be automatically configured", tool.Name)

	default:
		return fmt.Errorf("unknown config type for %s", tool.Name)
	}
}

// DiffPreview returns a string showing what changes will be made.
func DiffPreview(tool detect.Tool, gatewayURL string) string {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		profile := detect.ShellProfile()
		var lines []string
		lines = append(lines, fmt.Sprintf("  File: %s", profile))
		lines = append(lines, fmt.Sprintf("  Add:  # Tokara proxy — %s", tool.Name))
		for varName := range tool.EnvVars {
			lines = append(lines, fmt.Sprintf("  Add:  export %s=\"%s\"", varName, gatewayURL))
		}
		return strings.Join(lines, "\n")
	case detect.ConfigFile:
		return fmt.Sprintf("  File: %s\n  Set:  api_base → %s", tool.ConfigPath, gatewayURL)
	default:
		return ""
	}
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
