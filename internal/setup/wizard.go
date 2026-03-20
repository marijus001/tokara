package setup

import (
	"fmt"

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
		if apiKey == "" || (len(apiKey) < 8) {
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

	// Step 3: Detect installed tools
	gatewayURL := fmt.Sprintf("http://localhost:%d", defaultPort)
	allTools := detect.AllTools(gatewayURL)
	detected := detect.DetectAll(allTools)

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

		// Show launch instructions for CLI tools
		hasLaunchable := false
		for _, tool := range detected {
			if detect.CanLaunch(tool) {
				if !hasLaunchable {
					fmt.Printf("  %s\n", bold.Render("Launch through Tokara:"))
					prompt.Blank()
					hasLaunchable = true
				}
				fmt.Printf("    tokara run %s\n", tool.ID)
			}
		}
		if hasLaunchable {
			prompt.Blank()
		}

		// Show manual instructions for GUI tools
		for _, tool := range detected {
			if tool.Note != "" {
				prompt.Info(fmt.Sprintf("%s: %s", tool.Name, tool.Note))
				prompt.Blank()
			}
		}
	} else {
		prompt.Info("No AI tools detected.")
		prompt.Blank()
		prompt.Info("Supported tools: Claude Code, OpenAI Codex, Aider")
		prompt.Info(fmt.Sprintf("Launch any tool through Tokara: tokara run claude"))
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
	prompt.Info("Run `tokara` to start the proxy dashboard")
	prompt.Info("Run `tokara run claude` to launch Claude Code through the proxy")
	prompt.Blank()

	return true
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
