// Package tui implements the tokara status TUI dashboard.
package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/marijus001/tokara/internal/stats"
)

// Brand colors
var (
	rose     = lipgloss.Color("#e11d48")
	white    = lipgloss.Color("#fafafa")
	muted    = lipgloss.Color("#8a6070")
	dimmed   = lipgloss.Color("#4a2030")
	bgDark   = lipgloss.Color("#0c0a0e")
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(rose).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(muted)

	valueStyle = lipgloss.NewStyle().
			Foreground(white).
			Bold(true)

	eventStyle = lipgloss.NewStyle().
			Foreground(muted)

	eventActionCompacted = lipgloss.NewStyle().
				Foreground(rose)

	eventActionPassthrough = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4a5568"))

	statusRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22c55e")).
			Bold(true)

	dividerStyle = lipgloss.NewStyle().
			Foreground(dimmed)

	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			Italic(true)
)

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	statsURL string
	snapshot stats.Snapshot
	err      error
	quitting bool
	width    int
}

type tickMsg time.Time
type snapshotMsg stats.Snapshot
type errMsg error

// NewModel creates a dashboard model that polls the given stats URL.
func NewModel(statsURL string) Model {
	return Model{
		statsURL: statsURL,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchSnapshot(m.statsURL), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tickMsg:
		return m, tea.Batch(fetchSnapshot(m.statsURL), tickCmd())

	case snapshotMsg:
		m.snapshot = stats.Snapshot(msg)
		m.err = nil

	case errMsg:
		m.err = msg
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	s := "\n"

	// Header
	nameStyle := lipgloss.NewStyle().Foreground(white).Bold(true)
	s += fmt.Sprintf("  %s %s %s %s        %s\n\n",
		titleStyle.Render("▓"),
		nameStyle.Render("tokara"),
		dividerStyle.Render("·"),
		statusRunning.Render("running"),
		labelStyle.Render("↑ "+m.snapshot.Uptime),
	)

	if m.err != nil {
		s += fmt.Sprintf("  %s %v\n\n", labelStyle.Render("error:"), m.err)
		s += helpStyle.Render("  [q] quit")
		return s
	}

	// Stats row
	s += fmt.Sprintf("  %s %s    %s %s    %s %s    %s %s\n\n",
		labelStyle.Render("Requests"),
		valueStyle.Render(formatNum(m.snapshot.Requests)),
		dividerStyle.Render("│"),
		labelStyle.Render("Compactions"),
		valueStyle.Render(formatNum(m.snapshot.Compactions)),
		dividerStyle.Render("│"),
		labelStyle.Render("Tokens saved"),
		valueStyle.Render(formatNum(m.snapshot.TokensSaved)),
	)

	// Recent events
	if len(m.snapshot.RecentEvents) > 0 {
		s += fmt.Sprintf("  %s\n", labelStyle.Render("Recent:"))
		for _, e := range m.snapshot.RecentEvents {
			actionStr := eventActionPassthrough.Render(e.Action)
			if e.Action == "compacted" || e.Action == "compacted (precomputed)" {
				detail := ""
				if e.InputK > 0 {
					detail = fmt.Sprintf(" %dK → %dK (%d%%)", e.InputK, e.OutputK, e.SavedPct)
				}
				actionStr = eventActionCompacted.Render(e.Action + detail)
			}
			s += fmt.Sprintf("  %s  %s  %s\n",
				eventStyle.Render(e.Timestamp),
				eventStyle.Render(e.Model),
				actionStr,
			)
		}
	} else {
		s += fmt.Sprintf("  %s\n", labelStyle.Render("Waiting for requests..."))
	}

	s += "\n"
	s += helpStyle.Render("  [q] quit status view")
	s += "\n"

	return s
}

func fetchSnapshot(url string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return errMsg(fmt.Errorf("cannot connect to proxy: %w", err))
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg(err)
		}

		var snap stats.Snapshot
		if err := json.Unmarshal(body, &snap); err != nil {
			return errMsg(err)
		}

		return snapshotMsg(snap)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func formatNum(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
