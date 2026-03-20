package model

import "strings"

type contextEntry struct {
	prefix string
	window int
}

// Sorted so that more specific prefixes appear before shorter ones.
// The first matching prefix wins.
var contextTable = []contextEntry{
	// OpenAI - specific variants before shorter prefixes
	{"gpt-4.1-mini", 1048576},
	{"gpt-4.1-nano", 1048576},
	{"gpt-4.1", 1048576},
	{"gpt-4o-mini", 128000},
	{"gpt-4o", 128000},
	{"gpt-4-turbo", 128000},
	{"gpt-3.5-turbo", 16385},
	{"o4-mini", 200000},
	{"o3", 200000},

	// Anthropic - specific variants before shorter prefixes
	{"claude-opus-4", 200000},
	{"claude-sonnet-4", 200000},
	{"claude-haiku-4", 200000},
	{"claude-3.5-sonnet", 200000},
	{"claude-3.5-haiku", 200000},
	{"claude-3-opus", 200000},
	{"claude-3-sonnet", 200000},
	{"claude-3-haiku", 200000},

	// Google - specific variants before shorter prefixes
	{"gemini-2.0", 1048576},
	{"gemini-1.5-pro", 2097152},
	{"gemini-1.5-flash", 1048576},
}

const defaultContextWindow = 128000

// ContextWindow returns the maximum context window for the given model name.
// Falls back to 128000 for unknown models.
func ContextWindow(modelName string) int {
	lower := strings.ToLower(modelName)
	for _, e := range contextTable {
		if strings.Contains(lower, e.prefix) {
			return e.window
		}
	}
	return defaultContextWindow
}
