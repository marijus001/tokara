// Package cli implements the tokara command-line interface.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/marijus001/tokara/internal/api"
	"github.com/marijus001/tokara/internal/config"
	"github.com/marijus001/tokara/internal/daemon"
	"github.com/marijus001/tokara/internal/prompt"
	"github.com/marijus001/tokara/internal/setup"
	"github.com/marijus001/tokara/internal/tui"
)

var (
	rose  = lipgloss.Color("#e11d48")
	brand = lipgloss.NewStyle().Foreground(rose).Bold(true)
	info  = lipgloss.NewStyle().Foreground(lipgloss.Color("#fafafa"))
	dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a6070"))
	ok    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	fail  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
)

// Run is the main CLI entry point. It routes to the appropriate subcommand.
func Run(version string, args []string) {
	if len(args) == 0 {
		runStart(version, 18741)
		return
	}

	switch args[0] {
	case "setup":
		setup.RunWizard(version)
	case "status":
		runStatus()
	case "stop":
		runStop()
	case "upgrade":
		runUpgrade()
	case "index":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "  Usage: tokara index <directory>\n")
			os.Exit(1)
		}
		runIndex(args[1])
	case "version", "--version", "-v":
		fmt.Printf("tokara %s\n", version)
	case "help", "--help", "-h":
		printHelp(version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printHelp(version)
		os.Exit(1)
	}
}

func runStart(version string, defaultPort int) {
	// First run: if no config exists, run setup wizard
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil {
		// No config file — first run, trigger setup
		setup.RunWizard(version)
		// Reload config after setup
		cfg, _ = config.LoadFile(config.DefaultPath())
	}
	_ = cfg

	pid := daemon.IsRunning()
	if pid != 0 {
		fmt.Println()
		fmt.Printf("  %s %s proxy already running (pid %d)\n", brand.Render("▓"), info.Render("tokara"), pid)
		fmt.Printf("  %s to see live stats\n", dim.Render("Run `tokara status`"))
		fmt.Println()
		return
	}

	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s cannot find executable: %v\n", fail.Render("✗"), err)
		os.Exit(1)
	}

	port := defaultPort
	if envPort := os.Getenv("TOKARA_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			port = p
		}
	}

	fmt.Println()
	fmt.Printf("  %s %s — context compression for LLMs\n", brand.Render("▓"), info.Render("tokara"))
	fmt.Println()

	newPid, err := daemon.Start(executable, port)
	if err != nil {
		fmt.Printf("  %s %v\n", fail.Render("✗"), err)
		os.Exit(1)
	}

	fmt.Printf("  %s Proxy started on localhost:%d (pid %d)\n", ok.Render("✓"), port, newPid)
	fmt.Printf("  %s Run %s to see live stats\n", dim.Render(" "), info.Render("`tokara status`"))
	fmt.Println()
}

func runUpgrade() {
	fmt.Println()
	apiKey := prompt.Ask("Enter your Tokara API key:", "")
	if apiKey == "" {
		prompt.Fail("No key provided")
		fmt.Println()
		return
	}
	if !strings.HasPrefix(apiKey, "tk_live_") && !strings.HasPrefix(apiKey, "tk_test_") {
		prompt.Fail("Invalid key. Must start with tk_live_ or tk_test_")
		fmt.Println()
		return
	}

	if err := setup.SaveTokaraConfig(apiKey, 18741); err != nil {
		prompt.Fail(fmt.Sprintf("Failed to save: %v", err))
		fmt.Println()
		return
	}

	prompt.OK("API key saved. Restart proxy to apply: `tokara stop && tokara`")
	fmt.Println()
}

func runStatus() {
	pid := daemon.IsRunning()
	if pid == 0 {
		fmt.Println()
		fmt.Printf("  %s No proxy running. Run %s to start.\n", fail.Render("✗"), info.Render("`tokara`"))
		fmt.Println()
		os.Exit(1)
	}

	// Launch TUI connected to the proxy's stats endpoint
	statsURL := fmt.Sprintf("http://127.0.0.1:%d/stats", 18741)
	if envPort := os.Getenv("TOKARA_PORT"); envPort != "" {
		statsURL = fmt.Sprintf("http://127.0.0.1:%s/stats", envPort)
	}

	model := tui.NewModel(statsURL)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}

func runStop() {
	fmt.Println()
	if err := daemon.Stop(); err != nil {
		fmt.Printf("  %s %v\n", fail.Render("✗"), err)
		fmt.Println()
		os.Exit(1)
	}
	fmt.Printf("  %s Proxy stopped\n", ok.Render("✓"))
	fmt.Println()
}

func runIndex(dirPath string) {
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil || !cfg.HasAPIKey() {
		fmt.Println()
		prompt.Fail("API key required for indexing. Run `tokara upgrade` first.")
		fmt.Println()
		os.Exit(1)
	}

	client := api.NewClient(cfg.APIBase, cfg.APIKey)
	if err := setup.RunIndex(client, dirPath, ""); err != nil {
		prompt.Fail(err.Error())
		os.Exit(1)
	}
}

func printHelp(version string) {
	fmt.Println()
	fmt.Printf("  %s %s v%s — context compression for LLMs\n", brand.Render("▓"), info.Render("tokara"), version)
	fmt.Println()
	fmt.Printf("  %s\n", info.Render("Commands:"))
	fmt.Printf("    %s          %s\n", info.Render("tokara"), dim.Render("Start proxy (runs setup on first use)"))
	fmt.Printf("    %s    %s\n", info.Render("tokara setup"), dim.Render("Run setup wizard again"))
	fmt.Printf("    %s   %s\n", info.Render("tokara status"), dim.Render("Show live stats dashboard"))
	fmt.Printf("    %s     %s\n", info.Render("tokara stop"), dim.Render("Stop the proxy daemon"))
	fmt.Printf("    %s  %s\n", info.Render("tokara upgrade"), dim.Render("Add API key for paid features"))
	fmt.Printf("    %s %s\n", info.Render("tokara index ."), dim.Render("Index codebase for RAG (paid)"))
	fmt.Printf("    %s  %s\n", info.Render("tokara version"), dim.Render("Print version"))
	fmt.Printf("    %s     %s\n", info.Render("tokara help"), dim.Render("Show this help"))
	fmt.Println()
}
