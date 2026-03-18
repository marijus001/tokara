package message

import (
	"strings"
	"testing"
)

func TestParseAnthropic(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-6",
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"}
		],
		"stream": true
	}`

	parsed, err := ParseRequestBody(strings.NewReader(body), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %s, want claude-sonnet-4-6", parsed.Model)
	}
	if parsed.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("system = %q", parsed.SystemPrompt)
	}
	if len(parsed.Messages) != 3 {
		t.Fatalf("messages count = %d, want 3", len(parsed.Messages))
	}
	if parsed.Messages[0].Content != "Hello" {
		t.Errorf("first message = %q", parsed.Messages[0].Content)
	}
	if !parsed.Stream {
		t.Error("stream should be true")
	}
}

func TestParseOpenAI(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hi"},
			{"role": "assistant", "content": "Hello!"}
		]
	}`

	parsed, err := ParseRequestBody(strings.NewReader(body), "openai")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SystemPrompt != "You are helpful." {
		t.Errorf("system = %q", parsed.SystemPrompt)
	}
	if len(parsed.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2 (system excluded)", len(parsed.Messages))
	}
}

func TestParseAnthropicContentBlocks(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-6",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "Part one."},
				{"type": "text", "text": "Part two."}
			]}
		]
	}`

	parsed, err := ParseRequestBody(strings.NewReader(body), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(parsed.Messages))
	}
	if !strings.Contains(parsed.Messages[0].Content, "Part one") {
		t.Error("missing part one")
	}
	if !strings.Contains(parsed.Messages[0].Content, "Part two") {
		t.Error("missing part two")
	}
}

func TestRewriteMessages(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-6",
		"system": "Be helpful.",
		"messages": [
			{"role": "user", "content": "Original message"}
		]
	}`

	parsed, _ := ParseRequestBody(strings.NewReader(body), "anthropic")
	newMsgs := []Message{{Role: "user", Content: "Compacted message"}}

	rewritten, err := RewriteMessages(parsed, newMsgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rewritten), "Compacted message") {
		t.Error("rewritten body should contain new message")
	}
	if strings.Contains(string(rewritten), "Original message") {
		t.Error("rewritten body should not contain original message")
	}
	if !strings.Contains(string(rewritten), "Be helpful") {
		t.Error("system prompt should be preserved")
	}
}
