package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/marijus001/tokara/internal/detect"
)

// Launch opens a new terminal window running the given command
// with SDK env vars pointing at the gateway.
func Launch(toolCmd string, gatewayURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return launchDarwin(toolCmd, gatewayURL)
	case "linux":
		return launchLinux(toolCmd, gatewayURL)
	case "windows":
		return launchWindows(toolCmd, gatewayURL)
	default:
		return fmt.Errorf("unsupported platform %s; run manually:\n  %s %s",
			runtime.GOOS, buildEnvExports(gatewayURL, "bash"), toolCmd)
	}
}

// buildEnvExports generates export commands for the given shell type.
func buildEnvExports(gatewayURL string, shell string) string {
	vars := detect.SDKEnvVars(gatewayURL)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		switch shell {
		case "cmd":
			parts = append(parts, fmt.Sprintf("set %s=%s", k, vars[k]))
		default: // bash, zsh, sh
			parts = append(parts, fmt.Sprintf("export %s='%s'", k, vars[k]))
		}
	}

	switch shell {
	case "cmd":
		return strings.Join(parts, " && ")
	default:
		return strings.Join(parts, "; ")
	}
}

// launchDarwin opens Terminal.app via osascript.
func launchDarwin(toolCmd string, gatewayURL string) error {
	cwd, _ := os.Getwd()
	exports := buildEnvExports(gatewayURL, "bash")
	script := fmt.Sprintf(
		`tell application "Terminal"
			activate
			do script "cd '%s'; %s; clear; %s"
		end tell`,
		cwd, exports, toolCmd)

	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

// launchLinux tries common terminal emulators in order.
func launchLinux(toolCmd string, gatewayURL string) error {
	cwd, _ := os.Getwd()
	exports := buildEnvExports(gatewayURL, "bash")
	inner := fmt.Sprintf("cd '%s'; %s; %s; exec bash", cwd, exports, toolCmd)

	// Try gnome-terminal first.
	if path, err := exec.LookPath("gnome-terminal"); err == nil {
		cmd := exec.Command(path, "--", "bash", "-c", inner)
		return cmd.Run()
	}

	// Try xterm.
	if path, err := exec.LookPath("xterm"); err == nil {
		cmd := exec.Command(path, "-e", fmt.Sprintf("bash -c '%s'", inner))
		return cmd.Run()
	}

	// Try x-terminal-emulator (Debian/Ubuntu alternative).
	if path, err := exec.LookPath("x-terminal-emulator"); err == nil {
		cmd := exec.Command(path, "-e", "bash", "-c", inner)
		return cmd.Run()
	}

	return fmt.Errorf("no terminal emulator found; run manually:\n  %s %s", exports, toolCmd)
}

// launchWindows opens a new cmd.exe window.
func launchWindows(toolCmd string, gatewayURL string) error {
	exports := buildEnvExports(gatewayURL, "cmd")
	script := fmt.Sprintf("%s && %s", exports, toolCmd)

	cmd := exec.Command("cmd", "/c", "start", "cmd", "/k", script)
	return cmd.Run()
}
