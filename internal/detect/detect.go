package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Tool represents a detectable AI coding tool.
type Tool struct {
	ID   string // unique identifier
	Name string // display name
	Desc string // short description
	Cmd  string // binary name to spawn (empty for GUI-only tools)
	Note string // manual config instructions (for tools that can't be auto-launched)
}

// SDKEnvVars returns the env vars that intercept all LLM SDK traffic.
// Setting these routes requests from any tool using these SDKs through the proxy.
func SDKEnvVars(gatewayURL string) map[string]string {
	return map[string]string{
		"ANTHROPIC_BASE_URL": gatewayURL,  // Anthropic SDK
		"OPENAI_BASE_URL":    gatewayURL,  // OpenAI SDK
		"OPENAI_API_BASE":    gatewayURL,  // Aider / older OpenAI SDK
		"ANTHROPIC_API_BASE": gatewayURL,  // Aider / LiteLLM
	}
}

// AllTools returns all known AI tools.
func AllTools(gatewayURL string) []Tool {
	return []Tool{
		{
			ID:   "claude",
			Name: "Claude Code",
			Desc: "Anthropic's official coding CLI",
			Cmd:  "claude",
		},
		{
			ID:   "codex",
			Name: "OpenAI Codex",
			Desc: "OpenAI's coding agent CLI",
			Cmd:  "codex",
		},
		{
			ID:   "aider",
			Name: "Aider",
			Desc: "Terminal AI pair programming",
			Cmd:  "aider",
		},
		{
			ID:   "continue",
			Name: "Continue.dev",
			Desc: "VS Code AI extension",
			Cmd:  "continue",
		},
		{
			ID:   "cursor",
			Name: "Cursor",
			Desc: "AI code editor",
			Note: "Set proxy in Cursor > Settings > Models > Override OpenAI Base URL:\n  " + gatewayURL + "/v1",
		},
		{
			ID:   "windsurf",
			Name: "Windsurf",
			Desc: "Codeium's AI IDE",
			Note: "Set proxy in Windsurf Settings > Custom Model Provider:\n  Base URL: " + gatewayURL + "/v1",
		},
	}
}

// CanLaunch returns true if the tool can be launched via `tokara run`.
func CanLaunch(tool Tool) bool {
	return tool.Cmd != ""
}

// Detect checks if a tool is installed on this machine.
func Detect(tool Tool) bool {
	switch tool.ID {
	case "claude":
		return cmdExists("claude") || cmdExists("claude.cmd") || winBinExists("claude")
	case "codex":
		return cmdExists("codex") || cmdExists("openai") || winBinExists("codex")
	case "aider":
		return cmdExists("aider") || winBinExists("aider")
	case "continue":
		home, _ := os.UserHomeDir()
		return fileExists(filepath.Join(home, ".continue", "config.yaml")) || continueInstalled()
	case "cursor":
		return cmdExists("cursor") || cursorInstalled()
	case "windsurf":
		return cmdExists("windsurf") || windsurfInstalled()
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

func cmdExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// winBinExists checks common Windows install locations for a binary.
// Covers npm, pnpm, yarn, volta, scoop, pipx, and standalone installers.
func winBinExists(name string) bool {
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
		// npm global
		filepath.Join(appData, "npm", name+".cmd"),
		filepath.Join(appData, "npm", name),
		// pnpm global
		filepath.Join(localAppData, "pnpm", name+".cmd"),
		filepath.Join(localAppData, "pnpm", name),
		// yarn global
		filepath.Join(localAppData, "yarn", "bin", name+".cmd"),
		// volta
		filepath.Join(localAppData, "volta", "bin", name+".exe"),
		// scoop
		filepath.Join(home, "scoop", "shims", name+".exe"),
		filepath.Join(home, "scoop", "shims", name+".cmd"),
		// pipx / pip (Python tools like aider)
		filepath.Join(home, ".local", "bin", name+".exe"),
		// Standalone installer paths
		filepath.Join(localAppData, "Programs", name, name+".exe"),
	}

	// Claude-specific standalone paths
	if name == "claude" {
		paths = append(paths,
			filepath.Join(localAppData, "AnthropicClaude", "claude.exe"),
		)
	}

	for _, p := range paths {
		if fileExists(p) {
			return true
		}
	}

	// Also check Python Scripts dirs (for pip-installed tools like aider)
	scriptsPattern := filepath.Join(localAppData, "Programs", "Python", "Python*", "Scripts", name+".exe")
	if matches, _ := filepath.Glob(scriptsPattern); len(matches) > 0 {
		return true
	}
	scriptsPattern2 := filepath.Join(appData, "Python", "Python*", "Scripts", name+".exe")
	if matches, _ := filepath.Glob(scriptsPattern2); len(matches) > 0 {
		return true
	}

	return false
}
