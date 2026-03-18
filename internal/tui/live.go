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

// LiveModel is a bubbletea model for the in-process proxy dashboard.
type LiveModel struct {
	getSnapshot func() stats.Snapshot
	version     string
	addr        string
	mode        string
	snapshot    stats.Snapshot
	quitting    bool
	width       int
	height      int
}

// NewLiveModel creates a live dashboard that auto-refreshes.
func NewLiveModel(getSnapshot func() stats.Snapshot, version, addr, mode string) LiveModel {
	return LiveModel{
		getSnapshot: getSnapshot,
		version:     version,
		addr:        addr,
		mode:        mode,
		snapshot:    getSnapshot(),
	}
}

func (m LiveModel) Init() tea.Cmd {
	return liveTickCmd()
}

func (m LiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case liveTickMsg:
		m.snapshot = m.getSnapshot()
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
	accentStyle := lipgloss.NewStyle().Foreground(rose)

	// Header
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s %s v%s %s proxy running\n",
		titleStyle.Render("▓"),
		nameStyle.Render("tokara"),
		m.version,
		dividerStyle.Render("—"),
	))
	b.WriteString("\n")

	// Info row
	b.WriteString(fmt.Sprintf("  %s %s    %s %s    %s %s\n",
		labelStyle.Render("Proxy:"),
		valueStyle.Render(m.addr),
		labelStyle.Render("Mode:"),
		valueStyle.Render(m.mode),
		labelStyle.Render("Uptime:"),
		valueStyle.Render(m.snapshot.Uptime),
	))
	b.WriteString("\n")

	// Divider
	divWidth := 60
	if m.width > 4 {
		divWidth = m.width - 4
	}
	if divWidth > 80 {
		divWidth = 80
	}
	b.WriteString(fmt.Sprintf("  %s\n", dividerStyle.Render(strings.Repeat("─", divWidth))))
	b.WriteString("\n")

	// Stats row
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

	// Recent events
	b.WriteString("\n")
	if len(m.snapshot.RecentEvents) > 0 {
		b.WriteString(fmt.Sprintf("  %s\n", labelStyle.Render("Recent:")))

		// Limit to available height
		maxEvents := len(m.snapshot.RecentEvents)
		if m.height > 0 {
			available := m.height - 14 // header + stats + footer
			if available < 3 {
				available = 3
			}
			if maxEvents > available {
				maxEvents = available
			}
		}
		if maxEvents > 15 {
			maxEvents = 15
		}

		for i := 0; i < maxEvents; i++ {
			e := m.snapshot.RecentEvents[i]

			// Truncate model name
			model := e.Model
			if len(model) > 22 {
				model = model[:22]
			}

			actionStr := eventActionPassthrough.Render(fmt.Sprintf("%-14s", e.Action))
			detail := ""
			if e.Action == "compacted" || e.Action == "compacted (precomputed)" {
				short := "compacted"
				if strings.Contains(e.Action, "precomputed") {
					short = "precomputed"
				}
				actionStr = accentStyle.Render(fmt.Sprintf("%-14s", short))
				if e.InputK > 0 {
					detail = fmt.Sprintf(" %dK → %dK", e.InputK, e.OutputK)
					if e.SavedPct > 0 {
						detail += fmt.Sprintf(" (%d%%)", e.SavedPct)
					}
				}
			}

			tokInfo := ""
			if detail == "" && e.InputK > 0 {
				tokInfo = fmt.Sprintf(" %dK", e.InputK)
			}

			b.WriteString(fmt.Sprintf("  %s  %-22s  %s%s%s\n",
				eventStyle.Render(e.Timestamp),
				eventStyle.Render(model),
				actionStr,
				valueStyle.Render(detail),
				eventStyle.Render(tokInfo),
			))
		}
	} else {
		b.WriteString(fmt.Sprintf("  %s\n", labelStyle.Render("Waiting for requests...")))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render("[q] quit")))
	b.WriteString("\n")

	return b.String()
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return liveTickMsg(t)
	})
}
