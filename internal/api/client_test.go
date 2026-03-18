package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompressEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compress" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tk_live_test" {
			t.Error("missing or wrong auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type")
		}

		var req CompressRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Strategy != "distill" {
			t.Errorf("expected distill, got %s", req.Strategy)
		}

		json.NewEncoder(w).Encode(CompressResponse{
			Compressed:       "compressed text",
			OriginalTokens:   1000,
			CompressedTokens: 300,
			Ratio:            0.3,
			Strategy:         "distill",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "tk_live_test")
	resp, err := client.Compress(CompressRequest{
		Text:     "original text",
		Strategy: "distill",
		Ratio:    0.5,
		Query:    "how does auth work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Compressed != "compressed text" {
		t.Errorf("unexpected compressed: %s", resp.Compressed)
	}
	if resp.CompressedTokens != 300 {
		t.Errorf("expected 300 tokens, got %d", resp.CompressedTokens)
	}
}

func TestQueryEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(QueryResponse{
			LLMContext:   "relevant code context",
			TokenCount:   500,
			TotalResults: 3,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "tk_live_test")
	resp, err := client.Query(QueryRequest{
		Query:     "how does auth work",
		MaxTokens: 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.LLMContext != "relevant code context" {
		t.Error("unexpected context")
	}
	if resp.TotalResults != 3 {
		t.Errorf("expected 3 results, got %d", resp.TotalResults)
	}
}

func TestHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "ok", Version: "0.3.0"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "tk_live_test")
	resp, err := client.Health()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected ok, got %s", resp.Status)
	}
}

func TestAPIErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad_key")
	_, err := client.Health()
	if err == nil {
		t.Error("expected error for 401 response")
	}
}
