package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/marijus001/tokara/internal/stats"
)

type liveTickMsg time.Time

type panel int

const (
	panelLogs    panel = iota // default: live event feed
	panelConfig               // show config
	panelTools                // detected tools
	panelHelp                 // command reference
	panelUpgrade              // API key input
)

// Callbacks provides data to the TUI without importing other packages.
type Callbacks struct {
	GetSnapshot func() stats.Snapshot
	GetConfig   func() string
	GetTools    func() string
	SaveAPIKey  func(key string) error
}

// LiveModel is a bubbletea model for the in-process proxy dashboard.
type LiveModel struct {
	cb       Callbacks
	version  string
	addr     string
	mode     string
	snapshot stats.Snapshot

	activePanel  panel
	upgradeInput string
	upgradeMsg   string

	quitting bool
	width    int
	height   int
}

// NewLiveModel creates a live dashboard that auto-refreshes.
func NewLiveModel(cb Callbacks, version, addr, mode string) LiveModel {
	return LiveModel{
		cb:       cb,
		version:  version,
		addr:     addr,
		mode:     mode,
		snapshot: cb.GetSnapshot(),
	}
}

func (m LiveModel) Init() tea.Cmd {
	return liveTickCmd()
}

func (m LiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Upgrade input mode — capture keystrokes
		if m.activePanel == panelUpgrade {
			switch msg.Type {
			case tea.KeyEsc:
				m.activePanel = panelLogs
				m.upgradeInput = ""
				m.upgradeMsg = ""
			case tea.KeyEnter:
				if m.upgradeInput != "" {
					if err := m.cb.SaveAPIKey(m.upgradeInput); err != nil {
						m.upgradeMsg = fmt.Sprintf("  ✗ %v", err)
					} else {
						m.upgradeMsg = "  ✓ API key saved — restart proxy to enable paid features"
						m.mode = "paid"
					}
					m.upgradeInput = ""
				}
			case tea.KeyBackspace:
				if len(m.upgradeInput) > 0 {
					m.upgradeInput = m.upgradeInput[:len(m.upgradeInput)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.upgradeInput += string(msg.Runes)
				}
			}
			return m, nil
		}

		// Normal mode
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "l":
			m.activePanel = panelLogs
		case "c":
			if m.activePanel == panelConfig {
				m.activePanel = panelLogs
			} else {
				m.activePanel = panelConfig
			}
		case "t":
			if m.activePanel == panelTools {
				m.activePanel = panelLogs
			} else {
				m.activePanel = panelTools
			}
		case "h":
			if m.activePanel == panelHelp {
				m.activePanel = panelLogs
			} else {
				m.activePanel = panelHelp
			}
		case "u":
			m.activePanel = panelUpgrade
			m.upgradeInput = ""
			m.upgradeMsg = ""
		case "esc":
			m.activePanel = panelLogs
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case liveTickMsg:
		m.snapshot = m.cb.GetSnapshot()
		return m, liveTickCmd()
	}

	return m, nil
}

func (m LiveModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(white).Bold(true)

	// ── Top: Header + Stats (compact, always visible) ──

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s v%s %s proxy running\n",
		titleStyle.Render("▓"),
		nameStyle.Render("tokara"),
		m.version,
		dividerStyle.Render("—"),
	))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s    %s %s    %s %s\n",
		labelStyle.Render("Proxy:"),
		valueStyle.Render(m.addr),
		labelStyle.Render("Mode:"),
		valueStyle.Render(m.mode),
		labelStyle.Render("Uptime:"),
		valueStyle.Render(m.snapshot.Uptime),
	))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s   %s   %s %s   %s   %s %s   %s   %s %s\n",
		labelStyle.Render("Requests"),
		valueStyle.Render(formatNum(m.snapshot.Requests)),
		dividerStyle.Render("│"),
		labelStyle.Render("Compactions"),
		valueStyle.Render(formatNum(m.snapshot.Compactions)),
		dividerStyle.Render("│"),
		labelStyle.Render("Tokens saved"),
		valueStyle.Render(formatNum(m.snapshot.TokensSaved)),
		dividerStyle.Render("│"),
		labelStyle.Render("Sessions"),
		valueStyle.Render(fmt.Sprintf("%d", m.snapshot.Sessions)),
	))
	b.WriteString("\n")

	// ── Divider ──

	divWidth := 60
	if m.width > 4 {
		divWidth = m.width - 4
	}
	if divWidth > 80 {
		divWidth = 80
	}
	b.WriteString(fmt.Sprintf("  %s\n", dividerStyle.Render(strings.Repeat("─", divWidth))))

	// ── Bottom panel ──

	bottomLines := m.height - 11 // top section + divider + footer
	if bottomLines < 5 {
		bottomLines = 5
	}

	var panelContent string
	switch m.activePanel {
	case panelConfig:
		panelContent = m.renderConfig()
	case panelTools:
		panelContent = m.renderTools()
	case panelHelp:
		panelContent = m.renderHelp()
	case panelUpgrade:
		panelContent = m.renderUpgrade()
	default:
		panelContent = m.renderLogs(bottomLines)
	}

	b.WriteString(panelContent)

	// Pad to fill remaining space
	contentLines := strings.Count(panelContent, "\n")
	for i := contentLines; i < bottomLines; i++ {
		b.WriteString("\n")
	}

	// ── Footer ──

	if m.activePanel == panelUpgrade {
		b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("[enter] save  [esc] cancel")))
	} else {
		b.WriteString(fmt.Sprintf("  %s\n",
			helpStyle.Render("[h] help  [c] config  [t] tools  [l] logs  [u] upgrade  [q] quit"),
		))
	}

	return b.String()
}

func (m LiveModel) renderLogs(maxLines int) string {
	var b strings.Builder
	b.WriteString("\n")

	if len(m.snapshot.RecentEvents) == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", labelStyle.Render("Waiting for requests...")))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("Activity:")))

	accentStyle := lipgloss.NewStyle().Foreground(rose)
	show := len(m.snapshot.RecentEvents)
	if show > maxLines-3 {
		show = maxLines - 3
	}
	if show > 20 {
		show = 20
	}

	for i := 0; i < show; i++ {
		e := m.snapshot.RecentEvents[i]
		model := e.Model
		if len(model) > 24 {
			model = model[:24]
		}

		action := eventActionPassthrough.Render(fmt.Sprintf("%-14s", e.Action))
		detail := ""
		if e.Action == "compacted" || strings.Contains(e.Action, "precomputed") {
			short := "compacted"
			if strings.Contains(e.Action, "precomputed") {
				short = "precomputed"
			}
			action = accentStyle.Render(fmt.Sprintf("%-14s", short))
			if e.InputK > 0 {
				detail = fmt.Sprintf("  %dK → %dK", e.InputK, e.OutputK)
				if e.SavedPct > 0 {
					detail += fmt.Sprintf(" (%d%%)", e.SavedPct)
				}
			}
		} else if e.InputK > 0 {
			detail = fmt.Sprintf("  %dK", e.InputK)
		}

		b.WriteString(fmt.Sprintf("  %s  %-24s  %s%s\n",
			eventStyle.Render(e.Timestamp),
			eventStyle.Render(model),
			action,
			valueStyle.Render(detail),
		))
	}

	return b.String()
}

func (m LiveModel) renderConfig() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("Configuration:")))
	if m.cb.GetConfig != nil {
		b.WriteString(m.cb.GetConfig())
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("Edit: ~/.tokara/config.toml  ·  Press [esc] to go back")))
	return b.String()
}

func (m LiveModel) renderTools() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("Detected AI Tools:")))
	if m.cb.GetTools != nil {
		b.WriteString(m.cb.GetTools())
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("Run 'tokara setup' to configure  ·  Press [esc] to go back")))
	return b.String()
}

func (m LiveModel) renderHelp() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("Commands:")))

	cmdStyle := lipgloss.NewStyle().Foreground(white).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(muted)

	cmds := []struct{ key, desc string }{
		{"h", "Show this help"},
		{"c", "Show running config"},
		{"t", "Detect installed AI tools"},
		{"l", "Live activity log (default view)"},
		{"u", "Add/update API key"},
		{"q", "Stop proxy and exit"},
	}
	for _, c := range cmds {
		b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render(fmt.Sprintf("%-4s", c.key)), descStyle.Render(c.desc)))
	}

	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("  %s\n", labelStyle.Render("Subcommands (run in another terminal):")))
	b.WriteString("\n")

	subs := []struct{ cmd, desc string }{
		{"tokara setup", "Run setup wizard"},
		{"tokara config", "Show full configuration"},
		{"tokara upgrade", "Add API key (interactive)"},
		{"tokara index .", "Index codebase for RAG"},
	}
	for _, s := range subs {
		b.WriteString(fmt.Sprintf("  %s  %s\n", cmdStyle.Render(fmt.Sprintf("%-16s", s.cmd)), descStyle.Render(s.desc)))
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("Press [esc] to go back")))
	return b.String()
}

func (m LiveModel) renderUpgrade() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("Upgrade — Add API Key:")))

	inputStyle := lipgloss.NewStyle().Foreground(white)
	cursor := lipgloss.NewStyle().Foreground(rose).Render("█")

	b.WriteString(fmt.Sprintf("  %s %s%s\n",
		labelStyle.Render("API key:"),
		inputStyle.Render(m.upgradeInput),
		cursor,
	))

	if m.upgradeMsg != "" {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", m.upgradeMsg))
	} else {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("Paste your tk_live_... or tk_test_... key")))
	}

	return b.String()
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return liveTickMsg(t)
	})
}
