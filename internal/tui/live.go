package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/marijus001/tokara/internal/stats"
)

const dashboardURL = "https://tokara.dev/dashboard"

type liveTickMsg time.Time
type configSavedMsg struct{}

type panel int

const (
	panelLogs    panel = iota // default: live event feed
	panelConfig               // show config
	panelTools                // detected tools
	panelHelp                 // command reference
	panelUpgrade              // API key input
)

// ConfigItem represents a single editable configuration field.
type ConfigItem struct {
	Key   string // display name (e.g. "Port")
	Value string // current display value
	Field string // internal field name for saving (e.g. "port")
}

// ToolItem represents a tool's state for the interactive tools panel.
type ToolItem struct {
	ID        string
	Name      string
	Desc      string
	Detected  bool
	Enabled   bool
	CanToggle bool // false for ConfigNote types or not-found tools
}

// Callbacks provides data to the TUI without importing other packages.
type Callbacks struct {
	GetSnapshot func() stats.Snapshot
	GetConfig   func() []ConfigItem // returns structured config items
	SaveConfig  func(field, value string) error
	GetTools    func() []ToolItem
	ToggleTool  func(id string, enable bool) error
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

	// Config panel state
	configCursor  int
	configEditing bool
	configInput   string
	configItems   []ConfigItem
	configMsg     string // success/error after save

	// Tools panel state
	toolsCursor int
	toolsList   []ToolItem
	toolsLoaded bool
	toolsMsg    string

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
		height:   40, // sensible default until WindowSizeMsg arrives
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
						m.upgradeMsg = fmt.Sprintf("  \u2717 %v", err)
					} else {
						m.upgradeMsg = "  \u2713 API key saved — restart proxy to enable paid features"
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
					// Press u with empty input → open browser
					if m.upgradeInput == "" && string(msg.Runes) == "u" {
						openBrowser(dashboardURL)
						m.upgradeMsg = "  Opening tokara.dev/dashboard..."
						return m, nil
					}
					m.upgradeInput += string(msg.Runes)
				}
			}
			return m, nil
		}

		// Config panel input mode
		if m.activePanel == panelConfig {
			if m.configEditing {
				switch msg.Type {
				case tea.KeyEsc:
					m.configEditing = false
					m.configInput = ""
				case tea.KeyEnter:
					if m.configCursor >= 0 && m.configCursor < len(m.configItems) {
						field := m.configItems[m.configCursor].Field
						if err := m.cb.SaveConfig(field, m.configInput); err != nil {
							m.configMsg = fmt.Sprintf("  \u2717 %v", err)
						} else {
							m.configMsg = "  \u2713 saved"
							m.configItems = m.cb.GetConfig()
						}
						m.configEditing = false
						m.configInput = ""
						return m, clearConfigMsgAfter()
					}
				case tea.KeyBackspace:
					if len(m.configInput) > 0 {
						m.configInput = m.configInput[:len(m.configInput)-1]
					}
				default:
					if msg.Type == tea.KeyRunes {
						m.configInput += string(msg.Runes)
					}
				}
				return m, nil
			}

			// Config panel navigation (not editing)
			switch msg.Type {
			case tea.KeyUp:
				if m.configCursor > 0 {
					m.configCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.configCursor < len(m.configItems)-1 {
					m.configCursor++
				}
				return m, nil
			case tea.KeyEnter:
				if m.configCursor >= 0 && m.configCursor < len(m.configItems) {
					m.configEditing = true
					// Pre-fill with the raw value (strip trailing % for display)
					m.configInput = m.configItems[m.configCursor].Value
					m.configMsg = ""
				}
				return m, nil
			case tea.KeyEsc:
				m.activePanel = panelLogs
				m.configEditing = false
				m.configMsg = ""
				return m, nil
			}
		}

		// Tools panel interactive mode
		if m.activePanel == panelTools {
			switch msg.String() {
			case "up", "k":
				if m.toolsCursor > 0 {
					m.toolsCursor--
				}
				return m, nil
			case "down", "j":
				if m.toolsCursor < len(m.toolsList)-1 {
					m.toolsCursor++
				}
				return m, nil
			case " ":
				if m.toolsCursor >= 0 && m.toolsCursor < len(m.toolsList) {
					item := m.toolsList[m.toolsCursor]
					if item.CanToggle {
						newEnabled := !item.Enabled
						if m.cb.ToggleTool != nil {
							if err := m.cb.ToggleTool(item.ID, newEnabled); err != nil {
								m.toolsMsg = fmt.Sprintf("  \u2717 %v", err)
							} else {
								m.toolsMsg = ""
								m.refreshTools()
							}
						}
					}
				}
				return m, nil
			case "esc":
				m.activePanel = panelLogs
				m.toolsMsg = ""
				return m, nil
			case "q", "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "l":
				m.activePanel = panelLogs
				m.toolsMsg = ""
			case "c":
				m.activePanel = panelConfig
				m.configItems = m.cb.GetConfig()
				m.configCursor = 0
				m.configEditing = false
				m.configMsg = ""
				m.toolsMsg = ""
			case "h":
				m.activePanel = panelHelp
				m.toolsMsg = ""
			case "u":
				m.activePanel = panelUpgrade
				m.upgradeInput = ""
				m.upgradeMsg = ""
				m.toolsMsg = ""
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
				m.configItems = m.cb.GetConfig()
				m.configCursor = 0
				m.configEditing = false
				m.configMsg = ""
			}
		case "t":
			if m.activePanel == panelTools {
				m.activePanel = panelLogs
			} else {
				m.activePanel = panelTools
				m.refreshTools()
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

	case configSavedMsg:
		m.configMsg = ""

	case liveTickMsg:
		m.snapshot = m.cb.GetSnapshot()
		// Refresh tools list periodically if panel is active
		if m.activePanel == panelTools {
			m.refreshTools()
		}
		return m, liveTickCmd()
	}

	return m, nil
}

func (m *LiveModel) refreshTools() {
	if m.cb.GetTools != nil {
		m.toolsList = m.cb.GetTools()
		m.toolsLoaded = true
		if m.toolsCursor >= len(m.toolsList) {
			m.toolsCursor = len(m.toolsList) - 1
		}
		if m.toolsCursor < 0 {
			m.toolsCursor = 0
		}
	}
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

	// Pad to fill remaining space (leave 1 line gap before footer)
	contentLines := strings.Count(panelContent, "\n")
	for i := contentLines; i < bottomLines-1; i++ {
		b.WriteString("\n")
	}
	b.WriteString("\n") // blank line before footer

	// ── Footer ──

	switch {
	case m.activePanel == panelUpgrade:
		b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("[enter] save  [u] open dashboard  [esc] cancel")))
	case m.activePanel == panelConfig && m.configEditing:
		b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("[enter] save  [esc] cancel")))
	case m.activePanel == panelConfig:
		b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("[\u2191/\u2193] move  [enter] edit  [esc] back")))
	case m.activePanel == panelTools:
		b.WriteString(fmt.Sprintf("  %s\n",
			helpStyle.Render("[\u2191/\u2193] move  [space] toggle  [esc] back  [q] quit"),
		))
	default:
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
	available := maxLines - 3 // header line + "Activity:" + blank
	if available < 3 {
		available = 3
	}
	if show > available {
		show = available
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

	cursorStyle := lipgloss.NewStyle().Foreground(rose).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(muted)
	valStyle := lipgloss.NewStyle().Foreground(white).Bold(true)
	inputCursor := lipgloss.NewStyle().Foreground(rose).Render("\u2588")

	for i, item := range m.configItems {
		prefix := "   "
		if i == m.configCursor {
			prefix = cursorStyle.Render(" > ")
		}

		// Pad key to align values
		paddedKey := fmt.Sprintf("%-24s", item.Key)

		if m.configEditing && i == m.configCursor {
			b.WriteString(fmt.Sprintf("%s%s%s%s\n",
				prefix,
				keyStyle.Render(paddedKey),
				valStyle.Render(m.configInput),
				inputCursor,
			))
		} else {
			b.WriteString(fmt.Sprintf("%s%s%s\n",
				prefix,
				keyStyle.Render(paddedKey),
				valStyle.Render(item.Value),
			))
		}
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("~/.tokara/config.toml")))

	if m.configMsg != "" {
		b.WriteString(fmt.Sprintf("%s\n", m.configMsg))
	}

	return b.String()
}

func (m LiveModel) renderTools() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n", labelStyle.Render("AI Tools:")))

	greenDot := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	enabledTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	readyTag := lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308"))
	notFoundTag := lipgloss.NewStyle().Foreground(dimmed)
	infoTag := lipgloss.NewStyle().Foreground(dimmed).Italic(true)
	nameActive := lipgloss.NewStyle().Foreground(white).Bold(true)
	nameDim := lipgloss.NewStyle().Foreground(muted)
	descStyle := lipgloss.NewStyle().Foreground(muted)
	cursorStyle := lipgloss.NewStyle().Foreground(rose).Bold(true)

	if !m.toolsLoaded || len(m.toolsList) == 0 {
		b.WriteString(fmt.Sprintf("  %s\n", labelStyle.Render("Loading...")))
		return b.String()
	}

	for i, item := range m.toolsList {
		// Cursor
		cursor := "  "
		if i == m.toolsCursor {
			cursor = cursorStyle.Render("> ")
		}

		// Dot indicator
		dot := "  "
		if item.Detected {
			dot = greenDot.Render("\u25cf") + " "
		}

		// Tool name (bright if detected, dim if not)
		name := nameDim.Render(fmt.Sprintf("%-22s", item.Name))
		if item.Detected {
			name = nameActive.Render(fmt.Sprintf("%-22s", item.Name))
		}

		// Description
		desc := descStyle.Render(fmt.Sprintf("%-36s", item.Desc))

		// Status tag
		var tag string
		if item.Enabled {
			tag = enabledTag.Render("[enabled]")
		} else if item.Detected && item.CanToggle {
			tag = readyTag.Render("[ready]")
		} else if item.Detected && !item.CanToggle {
			tag = infoTag.Render("(info only)")
		} else {
			tag = notFoundTag.Render("(not found)")
		}

		b.WriteString(fmt.Sprintf("  %s%s%s  %s  %s\n", cursor, dot, name, desc, tag))
	}

	if m.toolsMsg != "" {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s\n", m.toolsMsg))
	}

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
		{"t", "Manage AI tool integrations"},
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

	linkStyle := lipgloss.NewStyle().Foreground(rose).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(white)
	cursor := lipgloss.NewStyle().Foreground(rose).Render("█")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rose).
		Padding(0, 2)
	b.WriteString(fmt.Sprintf("  %s\n\n",
		boxStyle.Render(fmt.Sprintf("Get your key at %s  —  press [u] to open", linkStyle.Render("tokara.dev/dashboard"))),
	))

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

func clearConfigMsgAfter() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return configSavedMsg{}
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
