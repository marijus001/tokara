package testutil

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnthropicUpstream(t *testing.T) {
	srv := NewAnthropicUpstream()
	defer srv.Close()

	body := `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"Hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "sk-test-key")
	req.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["type"] != "message" {
		t.Errorf("expected type=message, got %v", result["type"])
	}
	if result["model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected model=claude-3-5-sonnet-20241022, got %v", result["model"])
	}
	if result["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", result["role"])
	}
}

func TestOpenAIUpstream(t *testing.T) {
	srv := NewOpenAIUpstream()
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["object"] != "chat.completion" {
		t.Errorf("expected object=chat.completion, got %v", result["object"])
	}
	if result["model"] != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %v", result["model"])
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatal("expected non-empty choices array")
	}
}

func TestGoogleUpstream(t *testing.T) {
	srv := NewGoogleUpstream()
	defer srv.Close()

	body := `{"contents":[{"parts":[{"text":"Hello"}]}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/models/gemini-pro:generateContent", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		t.Fatal("expected non-empty candidates array")
	}

	candidate := candidates[0].(map[string]interface{})
	content := candidate["content"].(map[string]interface{})
	if content["role"] != "model" {
		t.Errorf("expected role=model, got %v", content["role"])
	}
}

func TestAnthropicStreamingUpstream(t *testing.T) {
	srv := NewAnthropicUpstream()
	defer srv.Close()

	body := `{"model":"claude-3-5-sonnet-20241022","stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "sk-test-key")
	req.Header.Set("Anthropic-Version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Collect all SSE events from the stream.
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedEvents := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}

	var foundEvents []string
	scanner := bufio.NewScanner(strings.NewReader(string(rawBody)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			foundEvents = append(foundEvents, strings.TrimPrefix(line, "event: "))
		}
	}

	if len(foundEvents) != len(expectedEvents) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedEvents), len(foundEvents), foundEvents)
	}

	for i, expected := range expectedEvents {
		if foundEvents[i] != expected {
			t.Errorf("event %d: expected %q, got %q", i, expected, foundEvents[i])
		}
	}

	// Verify the content_block_delta contains the expected text.
	bodyStr := string(rawBody)
	if !strings.Contains(bodyStr, "Mock streamed response.") {
		t.Error("streaming response does not contain expected text 'Mock streamed response.'")
	}
}
