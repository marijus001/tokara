package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/marijus001/tokara/internal/detect"
	"github.com/marijus001/tokara/internal/prompt"
)

var (
	rose  = lipgloss.Color("#e11d48")
	brand = lipgloss.NewStyle().Foreground(rose).Bold(true)
	dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a6070"))
	bold  = lipgloss.NewStyle().Foreground(lipgloss.Color("#fafafa")).Bold(true)
)

const defaultPort = 18741

// RunWizard runs the interactive first-run setup wizard.
// Returns true if setup completed successfully.
func RunWizard(version string) bool {
	prompt.Banner()

	fmt.Printf("  No account needed. Free local compression\n")
	fmt.Printf("  starts immediately.\n")
	prompt.Blank()

	// Step 1: Free or paid
	mode := prompt.Choose("How would you like to use Tokara?", []string{
		"Free — local compression only",
		"Connect API key (paid features)",
	})

	var apiKey string
	if mode == 1 {
		prompt.Blank()
		apiKey = prompt.Ask("Enter your API key:", "")
		if apiKey == "" || (!strings.HasPrefix(apiKey, "tk_live_") && !strings.HasPrefix(apiKey, "tk_test_")) {
			prompt.Fail("Invalid API key. Must start with tk_live_ or tk_test_")
			prompt.Info("You can add it later with `tokara upgrade`")
			apiKey = ""
		} else {
			prompt.OK("API key accepted")
		}
	}
	prompt.Blank()

	// Step 2: Save config
	if err := SaveTokaraConfig(apiKey, defaultPort); err != nil {
		prompt.Fail(fmt.Sprintf("Failed to save config: %v", err))
		return false
	}
	prompt.OK("Config saved to ~/.tokara/config.toml")
	prompt.Blank()

	// Step 3: Detect and configure tools
	gatewayURL := fmt.Sprintf("http://localhost:%d", defaultPort)
	allTools := detect.AllTools(gatewayURL)

	// Let user choose which tools to scan
	toolNames := make([]string, len(allTools))
	for i, t := range allTools {
		toolNames[i] = fmt.Sprintf("%s  %s", t.Name, dim.Render(t.Desc))
	}

	selected := prompt.SelectMultiple("Which AI tools would you like to detect?", toolNames)
	prompt.Blank()

	detected := detect.DetectSelected(allTools, selected)

	if len(detected) > 0 {
		fmt.Printf("  Found %s AI tool%s:\n", bold.Render(fmt.Sprintf("%d", len(detected))), pluralS(len(detected)))
		prompt.Blank()
		for _, tool := range detected {
			fmt.Printf("  %s %s  %s\n",
				lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("●"),
				bold.Render(tool.Name),
				dim.Render(tool.Desc),
			)
		}
		prompt.Blank()

		var configured []string
		for _, tool := range detected {
			if tool.ConfigType == detect.ConfigNote {
				prompt.Info(fmt.Sprintf("%s: %s", tool.Name, tool.Note))
				prompt.Blank()
				continue
			}

			if !prompt.Confirm(fmt.Sprintf("Configure %s to use Tokara?", bold.Render(tool.Name)), true) {
				prompt.Info(fmt.Sprintf("Skipped %s", tool.Name))
				prompt.Blank()
				continue
			}

			// Show what will change
			diff := DiffPreview(tool, gatewayURL)
			if diff != "" {
				fmt.Println(dim.Render(diff))
			}

			if !prompt.Confirm("Apply these changes?", true) {
				prompt.Info("Skipped")
				prompt.Blank()
				continue
			}

			result := ConfigureTool(tool, gatewayURL)
			if result.Success {
				prompt.OK(result.Details)
				if result.Backup != "" {
					prompt.Info(fmt.Sprintf("Backup saved to %s", result.Backup))
				}
				configured = append(configured, tool.Name)
			} else {
				prompt.Fail(result.Details)
			}
			prompt.Blank()
		}

		if len(configured) > 0 {
			prompt.OK(fmt.Sprintf("Configured %d tool%s: %s", len(configured), pluralS(len(configured)), strings.Join(configured, ", ")))
		}
	} else {
		prompt.Info("No AI tools detected.")
		prompt.Blank()
		prompt.Info("Supported tools: Claude Code, OpenAI Codex, OpenClaw, OpenCode")
		prompt.Info(fmt.Sprintf("Point any tool at %s manually.", bold.Render(gatewayURL)))
	}
	prompt.Blank()

	// Step 4: Summary
	fmt.Printf("  %s Setup complete!\n", brand.Render("✓"))
	prompt.Blank()
	modeStr := "free"
	if apiKey != "" {
		modeStr = "paid"
	}
	prompt.Info(fmt.Sprintf("Mode: %s", modeStr))
	prompt.Info(fmt.Sprintf("Proxy: localhost:%d", defaultPort))
	prompt.Info("Run `tokara` to start the proxy daemon")
	prompt.Info("Run `tokara status` for live stats")
	prompt.Blank()

	return true
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
