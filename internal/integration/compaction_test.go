package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/testutil"
)

func newTestProxy(upstreamURL string) (*proxy.Proxy, *session.Store) {
	store := session.NewStore()
	comp := compactor.New(compactor.Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 4,
		ModelContextWindow:  128000,
	}, store)
	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstreamURL, "openai": upstreamURL},
		Compactor:        comp,
	})
	return p, store
}

// sendRequest builds an HTTP request from the body and headers produced by
// testutil context helpers and sends it through the proxy, returning the
// recorded response.
func sendRequest(t *testing.T, p *proxy.Proxy, body []byte, headers http.Header, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	for key, vals := range headers {
		for _, val := range vals {
			req.Header.Add(key, val)
		}
	}
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)
	return rr
}

func TestSmallContextPassesThrough(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, _ := newTestProxy(upstream.URL)

	body, headers := testutil.SmallContext("anthropic")
	t.Log(testutil.DebugRequestSize("SmallContext", body))

	rr := sendRequest(t, p, body, headers, "/v1/messages")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := p.Stats.Compactions.Load(); got != 0 {
		t.Fatalf("expected 0 compactions for small context, got %d", got)
	}
	t.Logf("pass-through: requests=%d compactions=%d tokens_saved=%d",
		p.Stats.Requests.Load(), p.Stats.Compactions.Load(), p.Stats.TokensSaved.Load())
}

func TestLargeContextTriggersCompaction(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, _ := newTestProxy(upstream.URL)

	body, headers := testutil.LargeContext("anthropic")
	t.Log(testutil.DebugRequestSize("LargeContext", body))

	rr := sendRequest(t, p, body, headers, "/v1/messages")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := p.Stats.Compactions.Load(); got == 0 {
		t.Fatal("expected compactions > 0 for large context, got 0")
	}
	if got := p.Stats.TokensSaved.Load(); got == 0 {
		t.Fatal("expected tokens saved > 0 for large context, got 0")
	}
	t.Logf("compaction: requests=%d compactions=%d tokens_saved=%d",
		p.Stats.Requests.Load(), p.Stats.Compactions.Load(), p.Stats.TokensSaved.Load())
}

func TestMediumContextTriggersPrecompute(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, store := newTestProxy(upstream.URL)

	body, headers := testutil.MediumContext("anthropic")
	t.Log(testutil.DebugRequestSize("MediumContext", body))

	rr := sendRequest(t, p, body, headers, "/v1/messages")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// No compaction should have been applied synchronously.
	if got := p.Stats.Compactions.Load(); got != 0 {
		t.Fatalf("expected 0 compactions (precompute only), got %d", got)
	}

	// Wait for the background precompute goroutine to complete.
	time.Sleep(500 * time.Millisecond)

	// Use the deterministic session ID to look up the session that was created.
	// The session ID is generated from provider + model + system prompt.
	// Parse the body to extract model and system prompt for the lookup.
	var reqBody map[string]interface{}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	model, _ := reqBody["model"].(string)
	systemPrompt, _ := reqBody["system"].(string)
	sessID := session.SessionID("anthropic", model, systemPrompt)

	sess := store.Get(sessID)
	state, summary, tokens := sess.GetCompaction()

	t.Logf("precompute: session=%s state=%d summary_len=%d summary_tokens=%d store_count=%d",
		sessID, state, len(summary), tokens, store.Count())

	// After the background goroutine completes, the session should be in
	// StateReady with a non-empty compacted summary.
	if state != session.StateReady {
		t.Fatalf("expected session compaction state=StateReady (%d), got %d", session.StateReady, state)
	}
	if summary == "" {
		t.Fatal("expected non-empty compacted summary after precompute")
	}
	if tokens == 0 {
		t.Fatal("expected compacted tokens > 0 after precompute")
	}
}

func TestOpenAICompaction(t *testing.T) {
	upstream := testutil.NewOpenAIUpstream()
	defer upstream.Close()

	p, _ := newTestProxy(upstream.URL)

	body, headers := testutil.LargeContext("openai")
	t.Log(testutil.DebugRequestSize("LargeContext (openai)", body))

	// OpenAI detection requires /chat/completions in the path.
	rr := sendRequest(t, p, body, headers, "/v1/chat/completions")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := p.Stats.Compactions.Load(); got == 0 {
		t.Fatal("expected compactions > 0 for large OpenAI context, got 0")
	}
	if got := p.Stats.TokensSaved.Load(); got == 0 {
		t.Fatal("expected tokens saved > 0 for large OpenAI context, got 0")
	}
	t.Logf("openai compaction: requests=%d compactions=%d tokens_saved=%d",
		p.Stats.Requests.Load(), p.Stats.Compactions.Load(), p.Stats.TokensSaved.Load())
}

func TestCompactionResponseIntegrity(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, _ := newTestProxy(upstream.URL)

	body, headers := testutil.LargeContext("anthropic")
	t.Log(testutil.DebugRequestSize("LargeContext (integrity)", body))

	rr := sendRequest(t, p, body, headers, "/v1/messages")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	respBody, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if len(respBody) == 0 {
		t.Fatal("response body is empty after compaction")
	}

	var respJSON map[string]interface{}
	if err := json.Unmarshal(respBody, &respJSON); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nbody: %s", err, string(respBody))
	}
	if len(respJSON) == 0 {
		t.Fatal("response JSON is empty object")
	}

	t.Logf("response integrity: valid JSON, %d bytes, %d top-level keys",
		len(respBody), len(respJSON))
}
