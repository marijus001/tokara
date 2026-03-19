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

// SDKEnvVars returns the env vars that intercept all LLM SDK traffic.
// Setting these routes requests from any tool using these SDKs through the proxy.
func SDKEnvVars(gatewayURL string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL": gatewayURL,           // Anthropic SDK
		"OPENAI_BASE_URL":    gatewayURL,            // OpenAI SDK
		"OPENAI_API_BASE":    gatewayURL,            // Aider / older OpenAI SDK
		"ANTHROPIC_API_BASE": gatewayURL,            // Aider / LiteLLM
	}
}

// AllTools returns all known AI tools.
func AllTools(gatewayURL string) []Tool {
	sdkVars := SDKEnvVars(gatewayURL)

	return []Tool{
		{
			ID:         "claude",
			Name:       "Claude Code",
			Desc:       "Anthropic's official coding CLI",
			ConfigType: ConfigEnv,
			EnvVars:    sdkVars,
		},
		{
			ID:         "codex",
			Name:       "OpenAI Codex",
			Desc:       "OpenAI's coding agent CLI",
			ConfigType: ConfigEnv,
			EnvVars:    sdkVars,
		},
		{
			ID:         "opencode",
			Name:       "OpenCode",
			Desc:       "Terminal-based AI coding assistant (WIP)",
			ConfigType: ConfigNote,
			Note:       "OpenCode routing is a work in progress.",
		},
		{
			ID:         "aider",
			Name:       "Aider",
			Desc:       "Terminal AI pair programming",
			ConfigType: ConfigEnv,
			EnvVars:    sdkVars,
		},
		{
			ID:         "continue",
			Name:       "Continue.dev",
			Desc:       "VS Code AI extension",
			ConfigType: ConfigEnv,
			EnvVars:    sdkVars,
		},
		{
			ID:         "cursor",
			Name:       "Cursor",
			Desc:       "AI code editor",
			ConfigType: ConfigNote,
			Note:       "Set proxy in Cursor > Settings > Models > Override OpenAI Base URL:\n  " + gatewayURL + "/v1",
		},
		{
			ID:         "windsurf",
			Name:       "Windsurf",
			Desc:       "Codeium's AI IDE",
			ConfigType: ConfigNote,
			Note:       "Set proxy in Windsurf Settings > Custom Model Provider:\n  Base URL: " + gatewayURL + "/v1",
		},
		{
			ID:         "copilot",
			Name:       "GitHub Copilot",
			Desc:       "GitHub's AI pair programmer",
			ConfigType: ConfigNote,
			Note:       "Copilot doesn't support custom API endpoints.",
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
	case "opencode":
		return cmdExists("opencode")
	case "aider":
		return cmdExists("aider")
	case "continue":
		home, _ := os.UserHomeDir()
		return fileExists(filepath.Join(home, ".continue", "config.yaml")) || continueInstalled()
	case "cursor":
		return cmdExists("cursor") || cursorInstalled()
	case "windsurf":
		return cmdExists("windsurf") || windsurfInstalled()
	case "copilot":
		return copilotInstalled()
	default:
		return false
	}
}

func continueInstalled() bool {
	home, _ := os.UserHomeDir()
	extDir := filepath.Join(home, ".vscode", "extensions")
	entries, _ := os.ReadDir(extDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "continue.continue") {
			return true
		}
	}
	return false
}

func cursorInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		return fileExists("/Applications/Cursor.app")
	case "windows":
		home, _ := os.UserHomeDir()
		return fileExists(filepath.Join(home, "AppData", "Local", "Programs", "cursor", "Cursor.exe"))
	default:
		return cmdExists("cursor")
	}
}

func windsurfInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		return fileExists("/Applications/Windsurf.app")
	case "windows":
		home, _ := os.UserHomeDir()
		return fileExists(filepath.Join(home, "AppData", "Local", "Programs", "windsurf", "Windsurf.exe"))
	default:
		return cmdExists("windsurf")
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
