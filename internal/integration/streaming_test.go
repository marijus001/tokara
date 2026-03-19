package integration

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/testutil"
)

func TestStreamingPassesThrough(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstream.URL},
	})

	body, headers := testutil.AnthropicStreamingRequest(
		testutil.DefaultAnthropicModel, 3, 1000,
	)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	respBody, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	respStr := string(respBody)

	if !strings.Contains(respStr, "event: message_start") {
		t.Error("response missing 'event: message_start' SSE event")
	}
	if !strings.Contains(respStr, "event: message_stop") {
		t.Error("response missing 'event: message_stop' SSE event")
	}
}

func TestStreamingWithCompaction(t *testing.T) {
	upstream := testutil.NewAnthropicUpstream()
	defer upstream.Close()

	store := session.NewStore()
	comp := compactor.New(compactor.DefaultConfig(), store)
	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{"anthropic": upstream.URL},
		Compactor:        comp,
	})

	body, headers := testutil.LargeStreamingContext("anthropic")

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	for k, v := range headers {
		req.Header[k] = v
	}

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	respBody, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if len(respBody) == 0 {
		t.Error("response body is empty; expected non-empty streaming response after compaction")
	}
}
