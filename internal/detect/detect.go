package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ConfigType describes how a tool is configured.
type ConfigType int

const (
	ConfigEnv  ConfigType = iota // Set environment variables in shell profile
	ConfigFile                   // Patch a config file
	ConfigNote                   // Display a note (no auto-configuration)
)

// Tool represents a detectable AI coding tool.
type Tool struct {
	ID         string
	Name       string
	Desc       string
	ConfigType ConfigType
	EnvVars    map[string]string // For ConfigEnv: var name -> value
	KeyVar     string            // For ConfigEnv: the API key env var name
	ConfigPath string            // For ConfigFile: path to config file
	Note       string            // For ConfigNote: message to display
}

// AllTools returns all known AI tools.
func AllTools(gatewayURL string) []Tool {
	home, _ := os.UserHomeDir()

	return []Tool{
		{
			ID:         "claude",
			Name:       "Claude Code",
			Desc:       "Anthropic's official coding CLI",
			ConfigType: ConfigEnv,
			EnvVars:    map[string]string{"ANTHROPIC_BASE_URL": gatewayURL},
			KeyVar:     "ANTHROPIC_API_KEY",
		},
		{
			ID:         "codex",
			Name:       "OpenAI Codex",
			Desc:       "OpenAI's coding agent CLI",
			ConfigType: ConfigEnv,
			EnvVars:    map[string]string{"OPENAI_BASE_URL": gatewayURL},
			KeyVar:     "OPENAI_API_KEY",
		},
		{
			ID:         "openclaw",
			Name:       "OpenClaw",
			Desc:       "Autonomous code refactoring agent",
			ConfigType: ConfigFile,
			ConfigPath: filepath.Join(home, ".openclaw", "config.yaml"),
		},
		{
			ID:         "opencode",
			Name:       "OpenCode",
			Desc:       "Terminal-based AI coding assistant",
			ConfigType: ConfigEnv,
			EnvVars:    map[string]string{"OPENAI_BASE_URL": gatewayURL},
			KeyVar:     "OPENAI_API_KEY",
		},
		{
			ID:         "copilot",
			Name:       "GitHub Copilot",
			Desc:       "GitHub's AI pair programmer",
			ConfigType: ConfigNote,
			Note:       "GitHub Copilot doesn't support custom API endpoints natively.\n  Use Tokara's gateway with Copilot-compatible tools instead.\n  See: https://tokara.dev/docs/integrations",
		},
	}
}

// Detect checks if a tool is installed on this machine.
func Detect(tool Tool) bool {
	switch tool.ID {
	case "claude":
		return cmdExists("claude") || cmdExists("claude.cmd") || claudeInstalledWindows()
	case "codex":
		return cmdExists("codex") || cmdExists("openai")
	case "openclaw":
		return fileExists(tool.ConfigPath) || cmdExists("openclaw")
	case "opencode":
		return cmdExists("opencode") || fileExists(tool.ConfigPath)
	case "copilot":
		return copilotInstalled()
	default:
		return false
	}
}

// DetectAll returns all tools that are detected on this machine.
func DetectAll(tools []Tool) []Tool {
	var found []Tool
	for _, t := range tools {
		if Detect(t) {
			found = append(found, t)
		}
	}
	return found
}

// DetectSelected detects only the tools at the given indices.
func DetectSelected(tools []Tool, indices []int) []Tool {
	var found []Tool
	for _, i := range indices {
		if i >= 0 && i < len(tools) && Detect(tools[i]) {
			found = append(found, tools[i])
		}
	}
	return found
}

// ShellProfile returns the path to the user's shell profile file.
func ShellProfile() string {
	home, _ := os.UserHomeDir()
	shell := os.Getenv("SHELL")

	if runtime.GOOS == "windows" {
		// Windows: use PowerShell profile
		return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	}

	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	if strings.Contains(shell, "fish") {
		return filepath.Join(home, ".config", "fish", "config.fish")
	}
	return filepath.Join(home, ".bashrc")
}

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func claudeInstalledWindows() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	home, _ := os.UserHomeDir()
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	if appData == "" {
		appData = filepath.Join(home, "AppData", "Roaming")
	}

	paths := []string{
		// Standalone installer
		filepath.Join(localAppData, "Programs", "claude", "claude.exe"),
		filepath.Join(localAppData, "AnthropicClaude", "claude.exe"),
		// npm global install
		filepath.Join(appData, "npm", "claude.cmd"),
		filepath.Join(appData, "npm", "claude"),
		// Scoop / winget / chocolatey
		filepath.Join(home, "scoop", "shims", "claude.exe"),
		filepath.Join(home, "scoop", "shims", "claude.cmd"),
	}
	for _, p := range paths {
		if fileExists(p) {
			return true
		}
	}
	return false
}

func copilotInstalled() bool {
	home, _ := os.UserHomeDir()
	extDir := filepath.Join(home, ".vscode", "extensions")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "github.copilot") {
			return true
		}
	}
	return false
}
