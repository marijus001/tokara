package logger

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLogEntry(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf)

	l.LogRequest(Entry{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-6",
		InputTokens: 12000,
		Action:      "pass-through",
	})

	var entry map[string]interface{}
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if entry["provider"] != "anthropic" {
		t.Errorf("expected provider=anthropic, got %v", entry["provider"])
	}
	if entry["action"] != "pass-through" {
		t.Errorf("expected action=pass-through, got %v", entry["action"])
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("missing timestamp field")
	}
}
