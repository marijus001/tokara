package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/testutil"
)

// sendAnthropicRequest sends an Anthropic-format request through the proxy and
// returns the recorded response.
func sendAnthropicRequest(t *testing.T, p *proxy.Proxy, turns, tokenTarget int) *httptest.ResponseRecorder {
	t.Helper()
	body, headers := testutil.AnthropicRequest(testutil.DefaultAnthropicModel, turns, tokenTarget)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

// TestMultiTurnSessionTracking sends 5 requests with increasing context sizes
// through the same proxy and verifies all are counted.
func TestMultiTurnSessionTracking(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, store := newTestProxy(upstream.URL)

	tokenTargets := []int{5_000, 10_000, 15_000, 20_000, 25_000}
	for i, target := range tokenTargets {
		rec := sendAnthropicRequest(t, p, 4+i*2, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i+1, rec.Code)
		}
		t.Logf("request %d: %s", i+1, testutil.DebugRequestSize(
			"sent", mustMarshalAnthropicRequest(t, 4+i*2, target),
		))
	}

	gotRequests := p.Stats.Requests.Load()
	if gotRequests != 5 {
		t.Fatalf("expected 5 requests tracked, got %d", gotRequests)
	}
	t.Logf("stats: requests=%d compactions=%d tokens_saved=%d sessions=%d",
		p.Stats.Requests.Load(),
		p.Stats.Compactions.Load(),
		p.Stats.TokensSaved.Load(),
		store.Count(),
	)
}

// TestGrowingConversationTriggersCompaction simulates a conversation growing past
// the 80% compaction threshold and verifies compaction fires.
func TestGrowingConversationTriggersCompaction(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p, store := newTestProxy(upstream.URL)

	// Gradually increase context: 10K, 30K, 60K, 90K, 120K
	// 128K * 0.80 = 102.4K threshold — the 120K request should trigger compaction
	tokenTargets := []int{10_000, 30_000, 60_000, 90_000, 120_000}
	for i, target := range tokenTargets {
		turns := 4 + i*4
		rec := sendAnthropicRequest(t, p, turns, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d (%dK tokens): expected status 200, got %d",
				i+1, target/1000, rec.Code)
		}
		body := mustMarshalAnthropicRequest(t, turns, target)
		t.Logf("request %d: %s", i+1, testutil.DebugRequestSize("sent", body))
	}

	// Give background goroutines time to complete precomputation
	time.Sleep(500 * time.Millisecond)

	compactions := p.Stats.Compactions.Load()
	if compactions <= 0 {
		t.Fatalf("expected at least 1 compaction, got %d (the 120K request should exceed 80%% of 128K window)",
			compactions)
	}

	t.Logf("stats: requests=%d compactions=%d tokens_saved=%d sessions=%d",
		p.Stats.Requests.Load(),
		p.Stats.Compactions.Load(),
		p.Stats.TokensSaved.Load(),
		store.Count(),
	)
}

// TestMultiProviderSameProxy verifies that a single proxy can handle requests
// from multiple providers (Anthropic and OpenAI) simultaneously.
func TestMultiProviderSameProxy(t *testing.T) {
	anthropicUpstream := testutil.NewAnthropicUpstream()
	defer anthropicUpstream.Close()

	openaiUpstream := testutil.NewOpenAIUpstream()
	defer openaiUpstream.Close()

	store := session.NewStore()
	comp := compactor.New(compactor.DefaultConfig(), store)
	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{
			"anthropic": anthropicUpstream.URL,
			"openai":    openaiUpstream.URL,
		},
		Compactor: comp,
	})

	// Send Anthropic request
	anthropicBody, anthropicHeaders := testutil.AnthropicRequest(
		testutil.DefaultAnthropicModel, 4, 5_000,
	)
	anthropicReq := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(anthropicBody))
	for k, vals := range anthropicHeaders {
		for _, v := range vals {
			anthropicReq.Header.Add(k, v)
		}
	}
	anthropicRec := httptest.NewRecorder()
	p.ServeHTTP(anthropicRec, anthropicReq)

	if anthropicRec.Code != http.StatusOK {
		t.Fatalf("anthropic request: expected status 200, got %d", anthropicRec.Code)
	}
	t.Logf("anthropic response: status=%d", anthropicRec.Code)

	// Send OpenAI request — needs /chat/completions path for detection
	openaiBody, openaiHeaders := testutil.OpenAIRequest(
		testutil.DefaultOpenAIModel, 4, 5_000,
	)
	openaiReq := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(openaiBody))
	for k, vals := range openaiHeaders {
		for _, v := range vals {
			openaiReq.Header.Add(k, v)
		}
	}
	openaiRec := httptest.NewRecorder()
	p.ServeHTTP(openaiRec, openaiReq)

	if openaiRec.Code != http.StatusOK {
		t.Fatalf("openai request: expected status 200, got %d", openaiRec.Code)
	}
	t.Logf("openai response: status=%d", openaiRec.Code)

	gotRequests := p.Stats.Requests.Load()
	if gotRequests != 2 {
		t.Fatalf("expected 2 requests tracked, got %d", gotRequests)
	}
	t.Logf("stats: requests=%d compactions=%d tokens_saved=%d sessions=%d",
		p.Stats.Requests.Load(),
		p.Stats.Compactions.Load(),
		p.Stats.TokensSaved.Load(),
		store.Count(),
	)
}

// mustMarshalAnthropicRequest is a helper that returns the body bytes for
// logging/debug purposes.
func mustMarshalAnthropicRequest(t *testing.T, turns, tokenTarget int) []byte {
	t.Helper()
	body, _ := testutil.AnthropicRequest(testutil.DefaultAnthropicModel, turns, tokenTarget)
	return body
}
