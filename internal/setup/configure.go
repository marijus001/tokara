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
	// Patch .vscode/settings.json in the current directory
	// This sets terminal.integrated.env.* so any terminal opened in the IDE
	// (including Claude Code, Codex, etc.) routes through the proxy.
	settingsPath := filepath.Join(".vscode", "settings.json")

	// Read existing settings or start fresh
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

	// Set env vars for all platforms
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

	// Write back
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

func configureFile(tool detect.Tool, gatewayURL string) ConfigResult {
	configPath := tool.ConfigPath

	switch tool.ID {
	case "openclaw":
		return patchOpenClaw(configPath, gatewayURL)
	case "opencode":
		return patchOpenCode(configPath, gatewayURL)
	case "continue":
		return patchContinue(configPath, gatewayURL)
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

	// Parse existing config or start fresh
	var config map[string]interface{}
	if err == nil {
		// Backup existing
		backup := configPath + ".tokara-backup"
		os.WriteFile(backup, existing, 0600)

		if jsonErr := json.Unmarshal(existing, &config); jsonErr != nil {
			config = make(map[string]interface{})
		}
	} else {
		config = make(map[string]interface{})
		os.MkdirAll(filepath.Dir(configPath), 0755)
	}

	// Set provider baseURLs to route through proxy.
	// OpenCode uses provider.<name>.options.baseURL.
	// Patch ALL known providers so any model routes through the proxy.
	proxyURL := gatewayURL + "/v1" // OpenCode expects /v1 suffix
	providers := []string{
		"anthropic", "openai", "google", "openrouter",
		"groq", "mistral", "fireworks", "together", "deepseek",
		"xai", "moonshot", "minimax",
	}

	providerBlock, ok := config["provider"].(map[string]interface{})
	if !ok {
		providerBlock = make(map[string]interface{})
	}
	for _, name := range providers {
		entry, ok := providerBlock[name].(map[string]interface{})
		if !ok {
			entry = make(map[string]interface{})
		}
		opts, ok := entry["options"].(map[string]interface{})
		if !ok {
			opts = make(map[string]interface{})
		}
		opts["baseURL"] = proxyURL
		entry["options"] = opts
		providerBlock[name] = entry
	}
	config["provider"] = providerBlock

	data, _ := json.MarshalIndent(config, "", "  ")
	if writeErr := os.WriteFile(configPath, data, 0600); writeErr != nil {
		return ConfigResult{Tool: "OpenCode", Success: false, Details: fmt.Sprintf("Write error: %v", writeErr)}
	}

	backup := ""
	if err == nil {
		backup = configPath + ".tokara-backup"
	}
	return ConfigResult{
		Tool:    "OpenCode",
		Success: true,
		Details: fmt.Sprintf("Patched %s (provider.*.options.baseURL)", configPath),
		Backup:  backup,
	}
}

// Unconfigure removes Tokara configuration for env-based tools.
func Unconfigure(tool detect.Tool) error {
	if tool.ConfigType != detect.ConfigEnv {
		return nil
	}

	settingsPath := filepath.Join(".vscode", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil // no settings file, nothing to unconfigure
	}

	var settings map[string]interface{}
	if jsonErr := json.Unmarshal(data, &settings); jsonErr != nil {
		return nil
	}

	// Remove our env vars from all platforms
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
			// Remove the key entirely if empty
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

func patchContinue(configPath, gatewayURL string) ConfigResult {
	existing, err := os.ReadFile(configPath)

	backup := ""
	if err == nil && len(existing) > 0 {
		backup = configPath + ".tokara-backup"
		os.WriteFile(backup, existing, 0600)
	}

	os.MkdirAll(filepath.Dir(configPath), 0755)

	// Continue.dev uses YAML config with models[].apiBase
	// Write a simple config that routes through the proxy
	content := fmt.Sprintf(`# Tokara proxy configuration
models:
  - name: "Claude (via Tokara)"
    provider: anthropic
    model: claude-sonnet-4-20250514
    apiBase: "%s"
  - name: "GPT-4o (via Tokara)"
    provider: openai
    model: gpt-4o
    apiBase: "%s/v1"
`, gatewayURL, gatewayURL)

	// If existing config has content, append our models
	if err == nil && len(existing) > 0 {
		existingStr := string(existing)
		if strings.Contains(existingStr, "Tokara proxy") {
			return ConfigResult{Tool: "Continue.dev", Success: true, Details: "Already configured"}
		}
		content = existingStr + "\n" + content
	}

	if writeErr := os.WriteFile(configPath, []byte(content), 0600); writeErr != nil {
		return ConfigResult{Tool: "Continue.dev", Success: false, Details: fmt.Sprintf("Write error: %v", writeErr)}
	}

	return ConfigResult{
		Tool:    "Continue.dev",
		Success: true,
		Details: fmt.Sprintf("Patched %s (models[].apiBase)", configPath),
		Backup:  backup,
	}
}

// IsToolConfigured checks if a tool is currently configured to use the tokara gateway.
func IsToolConfigured(tool detect.Tool, gatewayURL string) bool {
	switch tool.ConfigType {
	case detect.ConfigEnv:
		// Check .vscode/settings.json for terminal env vars
		settingsPath := filepath.Join(".vscode", "settings.json")
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			return false
		}
		var settings map[string]interface{}
		if json.Unmarshal(data, &settings) != nil {
			return false
		}
		// Check any platform's env block
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
