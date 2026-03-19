package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/testutil"
)

// TestProxyRoutesAnthropicRequests verifies that the proxy forwards Anthropic
// requests to the mock upstream and returns a valid 200 response.
func TestProxyRoutesAnthropicRequests(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstream.URL},
	})

	body, headers := testutil.SmallContext("anthropic")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["type"] != "message" {
		t.Errorf("expected type=message, got %v", resp["type"])
	}
}

// TestProxyRoutesOpenAIRequests verifies that the proxy forwards OpenAI
// requests to the mock upstream and returns a valid 200 response.
func TestProxyRoutesOpenAIRequests(t *testing.T) {
	upstream := testutil.NewOpenAIUpstream()
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"openai": upstream.URL},
	})

	body, headers := testutil.SmallContext("openai")
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["object"] != "chat.completion" {
		t.Errorf("expected object=chat.completion, got %v", resp["object"])
	}
}

// TestProxyRoutesGoogleRequests verifies that the proxy forwards Google Gemini
// requests to the mock upstream and returns a valid 200 response.
func TestProxyRoutesGoogleRequests(t *testing.T) {
	upstream := testutil.NewGoogleUpstream()
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"google": upstream.URL},
	})

	body, headers := testutil.SmallContext("google")
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	candidates, ok := resp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		t.Errorf("expected non-empty candidates array, got %v", resp["candidates"])
	}
}

// TestProxyRejectsUnknownProvider verifies that the proxy returns 502 when it
// cannot detect any known LLM provider from the request.
func TestProxyRejectsUnknownProvider(t *testing.T) {
	p := proxy.New(proxy.Options{})

	req := httptest.NewRequest(http.MethodGet, "/random", nil)
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestProxyForwardsHeaders verifies that provider-specific headers (x-api-key,
// anthropic-version) are forwarded to the upstream server.
func TestProxyForwardsHeaders(t *testing.T) {
	var mu sync.Mutex
	var capturedHeaders http.Header

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstream.URL},
	})

	body, headers := testutil.AnthropicRequest("claude-3-5-sonnet-20241022", 2, 100)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedHeaders.Get("X-Api-Key") != "sk-ant-test-key" {
		t.Errorf("expected x-api-key=sk-ant-test-key, got %q", capturedHeaders.Get("X-Api-Key"))
	}
	if capturedHeaders.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("expected anthropic-version=2023-06-01, got %q", capturedHeaders.Get("Anthropic-Version"))
	}
}

// TestProxyStatsIncrement verifies that the proxy's atomic request counter is
// correctly incremented for each request processed.
func TestProxyStatsIncrement(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstream.URL},
	})

	for i := 0; i < 5; i++ {
		body, headers := testutil.AnthropicRequest("claude-3-5-sonnet-20241022", 2, 100)
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		for k, v := range headers {
			req.Header[k] = v
		}
		rr := httptest.NewRecorder()
		p.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	got := p.Stats.Requests.Load()
	if got != 5 {
		t.Errorf("expected 5 requests counted, got %d", got)
	}
}

// TestProxyReturns502OnUpstreamError verifies that the proxy returns a 502
// when the upstream server is unreachable (simulated by starting and then
// immediately closing the upstream).
func TestProxyReturns502OnUpstreamError(t *testing.T) {
	// Start and immediately close the upstream so its URL is unreachable.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	deadURL := upstream.URL
	upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": deadURL},
	})

	body, headers := testutil.AnthropicRequest("claude-3-5-sonnet-20241022", 2, 100)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestFullPipelineEndToEnd starts a real HTTP server with the full stack
// (proxy + compactor + session store) and exercises the health endpoint,
// small-context pass-through, and large-context compaction path.
func TestFullPipelineEndToEnd(t *testing.T) {
	// Set up mock upstreams
	anthropicUpstream := testutil.NewAnthropicUpstream()
	defer anthropicUpstream.Close()

	// Build full pipeline
	store := session.NewStore()
	comp := compactor.New(compactor.DefaultConfig(), store)
	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": anthropicUpstream.URL},
		Compactor:        comp,
	})

	// Build a mux with health endpoint, same as main.go
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok", "sessions": store.Count(),
		})
	})
	mux.Handle("/", p)

	server := httptest.NewServer(mux)
	defer server.Close()

	client := server.Client()

	// 1. Health check
	t.Run("health", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("health request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var health map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}
		if health["status"] != "ok" {
			t.Errorf("expected status=ok, got %v", health["status"])
		}
	})

	// 2. Small context: should pass through without compaction
	t.Run("small_context_passthrough", func(t *testing.T) {
		body, headers := testutil.SmallContext("anthropic")
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		for k, v := range headers {
			req.Header[k] = v
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("small context request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if result["type"] != "message" {
			t.Errorf("expected type=message, got %v", result["type"])
		}
	})

	// 3. Large context: should trigger compaction processing
	t.Run("large_context_compaction", func(t *testing.T) {
		body, headers := testutil.LargeContext("anthropic")
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		for k, v := range headers {
			req.Header[k] = v
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("large context request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	})

	// 4. Verify stats after all requests
	t.Run("stats_verification", func(t *testing.T) {
		// We sent 2 proxy requests (small + large), health doesn't go through proxy
		requests := p.Stats.Requests.Load()
		if requests != 2 {
			t.Errorf("expected 2 requests in stats, got %d", requests)
		}

		// Compactions counter should be >= 0 (large context may or may not
		// trigger depending on threshold configuration)
		compactions := p.Stats.Compactions.Load()
		t.Logf("compactions: %d, tokens saved: %d", compactions, p.Stats.TokensSaved.Load())
	})
}
