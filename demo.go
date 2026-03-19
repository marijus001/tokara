package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/marijus001/tokara/internal/compactor"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/stats"
	"github.com/marijus001/tokara/internal/testutil"
	"github.com/marijus001/tokara/internal/tui"
)

// runDemo starts the proxy with mock upstreams and sends simulated traffic
// so the TUI dashboard can be observed without real API keys.
func runDemo() {
	// Start mock upstreams
	anthropicUp := testutil.NewAnthropicUpstream()
	defer anthropicUp.Close()
	openaiUp := testutil.NewOpenAIUpstream()
	defer openaiUp.Close()
	googleUp := testutil.NewGoogleUpstream()
	defer googleUp.Close()

	store := session.NewStore()
	comp := compactor.New(compactor.DefaultConfig(), store)
	collector := stats.NewCollector(50)

	p := proxy.New(proxy.Options{
		ProviderOverride: map[string]string{
			"anthropic": anthropicUp.URL,
			"openai":    openaiUp.URL,
			"google":    googleUp.URL,
		},
		Compactor:     comp,
		ContextSource: &tkctx.NilSource{},
	})

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":"%s","mode":"demo"}`, version)
	})
	mux.Handle("/", p)

	addr := "127.0.0.1:18741"
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Silence log output so it doesn't corrupt the TUI
	log.SetOutput(io.Discard)

	// Start fake traffic generator
	go generateTraffic(addr)

	// TUI callbacks
	cb := tui.Callbacks{
		GetSnapshot: func() stats.Snapshot {
			return collector.BuildSnapshot(
				p.Stats.Requests.Load(),
				p.Stats.Compactions.Load(),
				p.Stats.TokensSaved.Load(),
				store.Count(),
			)
		},
		GetConfig: func() []tui.ConfigItem {
			return []tui.ConfigItem{
				{Key: "Mode", Value: "demo", Field: "mode"},
				{Key: "Port", Value: "18741", Field: "port"},
				{Key: "Compact at", Value: "80%", Field: "compact"},
				{Key: "Precomp at", Value: "60%", Field: "precompute"},
				{Key: "Keep turns", Value: "4", Field: "turns"},
			}
		},
		SaveConfig: func(field, value string) error {
			return fmt.Errorf("demo mode — no config changes")
		},
		GetTools: func() []tui.ToolItem { return nil },
		SaveAPIKey: func(key string) error {
			return fmt.Errorf("demo mode — no config changes")
		},
	}

	model := tui.NewLiveModel(cb, version+" (demo)", addr, "demo")
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}

	server.Close()
}

// generateTraffic sends simulated requests through the proxy at random intervals.
func generateTraffic(addr string) {
	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	providers := []string{"anthropic", "openai"}
	models := map[string][]string{
		"anthropic": {"claude-sonnet-4-6", "claude-haiku-4-5-20251001"},
		"openai":    {"gpt-4o", "gpt-4o-mini"},
	}
	paths := map[string]string{
		"anthropic": "/v1/messages",
		"openai":    "/v1/chat/completions",
	}

	// Simulate growing conversation
	tokenSize := 5000

	for {
		provider := providers[rand.Intn(len(providers))]
		model := models[provider][rand.Intn(len(models[provider]))]
		path := paths[provider]

		// Vary context size — occasionally send large ones
		turns := 4 + rand.Intn(8)
		currentSize := tokenSize + rand.Intn(10000)

		// Every ~5th request, send a bigger context to trigger compaction
		if rand.Intn(5) == 0 {
			currentSize = 90000 + rand.Intn(30000)
			turns = 20 + rand.Intn(20)
		}

		var body []byte
		var headers http.Header

		switch provider {
		case "anthropic":
			body, headers = testutil.AnthropicRequest(model, turns, currentSize)
		case "openai":
			body, headers = testutil.OpenAIRequest(model, turns, currentSize)
		}

		req, _ := http.NewRequest("POST", "http://"+addr+path, bytes.NewReader(body))
		for k, v := range headers {
			req.Header[k] = v
		}

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}

		// Grow context over time
		tokenSize += 2000 + rand.Intn(3000)
		if tokenSize > 120000 {
			tokenSize = 5000 // reset
		}

		// Random interval 2-5 seconds
		time.Sleep(time.Duration(2000+rand.Intn(3000)) * time.Millisecond)
	}
}
