package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyForwardsRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Error("missing forwarded x-api-key header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_test","content":[{"text":"hello"}]}`))
	}))
	defer upstream.Close()

	p := New(Options{
		ProviderOverride: map[string]string{
			"anthropic": upstream.URL,
		},
	})

	body := strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest("POST", "/v1/messages", body)
	req.Header.Set("x-api-key", "sk-ant-test")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	respBody, _ := io.ReadAll(rec.Result().Body)
	if !strings.Contains(string(respBody), "msg_test") {
		t.Errorf("unexpected response: %s", respBody)
	}
}

func TestProxyReturns502ForUnknownProvider(t *testing.T) {
	p := New(Options{})

	req := httptest.NewRequest("GET", "/random/path", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}
