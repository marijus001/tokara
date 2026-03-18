package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	rose  = lipgloss.Color("#e11d48")
	white = lipgloss.Color("#fafafa")
	muted = lipgloss.Color("#8a6070")
	green = lipgloss.Color("#22c55e")
	red   = lipgloss.Color("#ef4444")

	promptStyle = lipgloss.NewStyle().Foreground(rose).Bold(true)
	inputStyle  = lipgloss.NewStyle().Foreground(white)
	dimStyle    = lipgloss.NewStyle().Foreground(muted)
	okStyle     = lipgloss.NewStyle().Foreground(green)
	failStyle   = lipgloss.NewStyle().Foreground(red)
)

var reader = bufio.NewReader(os.Stdin)

// Ask prompts the user for text input with an optional default.
func Ask(question string, defaultVal string) string {
	hint := ""
	if defaultVal != "" {
		hint = dimStyle.Render(fmt.Sprintf(" [%s]", defaultVal))
	}
	fmt.Printf("  %s%s ", promptStyle.Render("?"), inputStyle.Render(" "+question+hint))

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

// Confirm prompts for yes/no with a default.
func Confirm(question string, defaultYes bool) bool {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Printf("  %s %s %s ",
		promptStyle.Render("?"),
		inputStyle.Render(question),
		dimStyle.Render("["+hint+"]"),
	)

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return strings.HasPrefix(line, "y")
}

// Choose presents a numbered list and returns the selected index.
// Returns -1 if the user cancels.
func Choose(question string, options []string) int {
	fmt.Printf("  %s %s\n", promptStyle.Render("?"), inputStyle.Render(question))
	for i, opt := range options {
		fmt.Printf("    %s %s\n", dimStyle.Render(fmt.Sprintf("%d.", i+1)), inputStyle.Render(opt))
	}
	fmt.Printf("  %s ", promptStyle.Render(">"))

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(options) {
		return -1
	}
	return idx - 1
}

// SelectMultiple lets the user pick multiple items by number (e.g. "1,3,5" or "a" for all).
func SelectMultiple(question string, options []string) []int {
	fmt.Printf("  %s %s\n", promptStyle.Render("?"), inputStyle.Render(question))
	for i, opt := range options {
		fmt.Printf("    %s %s\n", dimStyle.Render(fmt.Sprintf("%d.", i+1)), inputStyle.Render(opt))
	}
	fmt.Printf("\n  %s %s ", dimStyle.Render("Enter numbers (e.g. 1,2,3) or"), inputStyle.Render("a"))
	fmt.Printf("%s ", dimStyle.Render("for all:"))

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))

	if line == "a" || line == "" {
		result := make([]int, len(options))
		for i := range options {
			result[i] = i
		}
		return result
	}

	parts := strings.Split(line, ",")
	var result []int
	for _, p := range parts {
		idx, err := strconv.Atoi(strings.TrimSpace(p))
		if err == nil && idx >= 1 && idx <= len(options) {
			result = append(result, idx-1)
		}
	}
	return result
}

// OK prints a success message.
func OK(msg string) {
	fmt.Printf("  %s %s\n", okStyle.Render("\u2713"), inputStyle.Render(msg))
}

// Fail prints an error message.
func Fail(msg string) {
	fmt.Printf("  %s %s\n", failStyle.Render("\u2717"), inputStyle.Render(msg))
}

// Info prints an info message.
func Info(msg string) {
	fmt.Printf("  %s\n", dimStyle.Render(msg))
}

// Blank prints an empty line.
func Blank() {
	fmt.Println()
}

// Banner prints the tokara branding header.
func Banner() {
	fmt.Println()
	fmt.Printf("  %s %s — context compression for LLMs\n", promptStyle.Render("▓"), inputStyle.Render("tokara"))
	fmt.Println()
}
