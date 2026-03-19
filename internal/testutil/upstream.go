package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

// NewAnthropicUpstream returns an httptest.Server that mimics the Anthropic
// Messages API. It handles POST to any path, reads the request body to extract
// the "model" and "stream" fields, and returns either a single JSON message or
// a series of SSE events when streaming is requested.
func NewAnthropicUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		model, _ := req["model"].(string)
		if model == "" {
			model = "claude-3-5-sonnet-20241022"
		}

		stream, _ := req["stream"].(bool)

		if stream {
			writeAnthropicStream(w, model)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := fmt.Sprintf(`{"id":"msg_test_001","type":"message","role":"assistant","model":"%s","content":[{"type":"text","text":"Mock Anthropic response."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":12}}`, model)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
}

func writeAnthropicStream(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	events := []struct {
		event string
		data  string
	}{
		{
			event: "message_start",
			data:  fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_test_001","type":"message","role":"assistant","model":"%s","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}`, model),
		},
		{
			event: "content_block_start",
			data:  `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		},
		{
			event: "content_block_delta",
			data:  `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Mock streamed response."}}`,
		},
		{
			event: "content_block_stop",
			data:  `{"type":"content_block_stop","index":0}`,
		},
		{
			event: "message_delta",
			data:  `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`,
		},
		{
			event: "message_stop",
			data:  `{"type":"message_stop"}`,
		},
	}

	for _, ev := range events {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.event, ev.data)
		flusher.Flush()
	}
}

// NewOpenAIUpstream returns an httptest.Server that mimics the OpenAI Chat
// Completions API. It handles POST to any path, reads the "model" field from
// the request body, and returns a chat completion JSON response.
func NewOpenAIUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		model, _ := req["model"].(string)
		if model == "" {
			model = "gpt-4o"
		}

		w.Header().Set("Content-Type", "application/json")
		resp := fmt.Sprintf(`{"id":"chatcmpl-test001","object":"chat.completion","model":"%s","choices":[{"index":0,"message":{"role":"assistant","content":"Mock OpenAI response."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":9,"total_tokens":19}}`, model)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
}

// NewGoogleUpstream returns an httptest.Server that mimics the Google Gemini
// API. It handles POST to any path and returns a generateContent-style JSON
// response with a single text candidate.
func NewGoogleUpstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := `{"candidates":[{"content":{"parts":[{"text":"Mock Gemini response."}],"role":"model"},"finishReason":"STOP"}]}`
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
}
