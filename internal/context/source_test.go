package context

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marijus001/tokara/internal/api"
)

func TestNilSource(t *testing.T) {
	src := &NilSource{}

	if src.Available() {
		t.Error("nil source should not be available")
	}
	if src.Name() != "none" {
		t.Errorf("expected 'none', got %s", src.Name())
	}

	chunks, err := src.Query("test", QueryOpts{})
	if err != nil {
		t.Errorf("nil source query should not error: %v", err)
	}
	if chunks != nil {
		t.Error("nil source should return nil chunks")
	}
}

func TestCloudSourceQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/query":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"llmContext":   "function auth() { ... }",
				"tokenCount":  150,
				"totalResults": 3,
			})
		}
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "tk_live_test")
	src := NewCloudSource(client)

	if !src.Available() {
		t.Error("cloud source should be available")
	}
	if src.Name() != "cloud" {
		t.Errorf("expected 'cloud', got %s", src.Name())
	}

	chunks, err := src.Query("how does auth work", QueryOpts{MaxTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Tokens != 150 {
		t.Errorf("expected 150 tokens, got %d", chunks[0].Tokens)
	}
}

func TestCloudSourceUnavailable(t *testing.T) {
	// Point at a server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "tk_live_test")
	src := NewCloudSource(client)

	if src.Available() {
		t.Error("should not be available when API returns 500")
	}
}
