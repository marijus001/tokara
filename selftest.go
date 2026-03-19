package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/compress"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/eval"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/testutil"
	"github.com/marijus001/tokara/internal/token"
)

// ANSI escape codes matching the tokara brand.
const (
	colorRose  = "\033[1;38;2;225;29;72m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorBold  = "\033[1m"
	colorReset = "\033[0m"
	colorDim   = "\033[2m"
)

// selfTest defines a single diagnostic check.
type selfTest struct {
	group string
	name  string
	fn    func() error
}

// runSelfTest executes the tokara self-diagnostics suite.
func runSelfTest() {
	fmt.Println()
	fmt.Printf("  %s▓%s %stokara self-test%s\n", colorRose, colorReset, colorBold, colorReset)
	fmt.Println()

	// --- Start mock upstreams ---
	anthropicUp := testutil.NewAnthropicUpstream()
	defer anthropicUp.Close()
	openaiUp := testutil.NewOpenAIUpstream()
	defer openaiUp.Close()
	googleUp := testutil.NewGoogleUpstream()
	defer googleUp.Close()

	overrides := map[string]string{
		"anthropic": anthropicUp.URL,
		"openai":    openaiUp.URL,
		"google":    googleUp.URL,
	}

	// --- Build tests ---
	tests := []selfTest{
		// Routing tests
		{group: "Routing", name: "Anthropic", fn: routingTest("anthropic", overrides)},
		{group: "Routing", name: "OpenAI", fn: routingTest("openai", overrides)},
		{group: "Routing", name: "Google", fn: routingTest("google", overrides)},
		// Compaction tests
		{group: "Compaction", name: "Small context (10K) — pass-through", fn: compactionPassThroughTest(overrides)},
		{group: "Compaction", name: "Large context (110K) — compacted", fn: compactionAppliedTest(overrides)},
	}

	// Quality tests — compress fixtures and check fact survival
	for _, fixture := range eval.DefaultFixtures() {
		f := fixture // capture
		tests = append(tests, selfTest{
			group: "Quality",
			name:  fmt.Sprintf("%s — fact survival", f.Name),
			fn:    qualityTest(f),
		})
	}

	// --- Run tests ---
	passed := 0
	failed := 0
	currentGroup := ""

	for _, t := range tests {
		if t.group != currentGroup {
			if currentGroup != "" {
				fmt.Println()
			}
			fmt.Printf("  %s%s%s\n", colorBold, t.group, colorReset)
			currentGroup = t.group
		}

		start := time.Now()
		err := t.fn()
		elapsed := time.Since(start)

		if err != nil {
			failed++
			fmt.Printf("  %s✗%s %-40s %s%v%s\n", colorRed, colorReset, t.name, colorRed, err, colorReset)
		} else {
			passed++
			fmt.Printf("  %s✓%s %-40s %s%dms%s\n", colorGreen, colorReset, t.name, colorDim, elapsed.Milliseconds(), colorReset)
		}
	}

	// --- Summary ---
	total := passed + failed
	fmt.Println()
	if failed == 0 {
		fmt.Printf("  %s%d/%d checks passed ✓%s\n", colorGreen, passed, total, colorReset)
	} else {
		fmt.Printf("  %s%d/%d checks passed (%d failed) ✗%s\n", colorRed, passed, total, failed, colorReset)
	}
	fmt.Println()
}

// routingTest returns a test function that sends a single request for the given
// provider through the proxy and verifies a 200 response.
func routingTest(provider string, overrides map[string]string) func() error {
	return func() error {
		store := session.NewStore()
		p := proxy.New(proxy.Options{
			ProviderOverride: overrides,
			ContextSource:    &tkctx.NilSource{},
		})

		body, headers := testutil.SmallContext(provider)

		path := "/v1/messages"
		switch provider {
		case "openai":
			path = "/v1/chat/completions"
		case "google":
			path = "/v1beta/models/gemini-1.5-pro:generateContent"
		}

		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}

		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			return fmt.Errorf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}

		_ = store // keep store alive for the duration of the test
		return nil
	}
}

// compactionPassThroughTest sends a small context request and verifies that
// the compactor passes it through without modification (tokens stay the same).
func compactionPassThroughTest(overrides map[string]string) func() error {
	return func() error {
		store := session.NewStore()
		comp := compactor.New(compactor.DefaultConfig(), store)

		p := proxy.New(proxy.Options{
			ProviderOverride: overrides,
			Compactor:        comp,
			ContextSource:    &tkctx.NilSource{},
		})

		body, headers := testutil.SmallContext("anthropic")
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}

		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			return fmt.Errorf("expected 200, got %d", rec.Code)
		}
		// Small context should not trigger compaction.
		if p.Stats.Compactions.Load() != 0 {
			return fmt.Errorf("expected 0 compactions, got %d", p.Stats.Compactions.Load())
		}
		return nil
	}
}

// qualityTest compresses a fixture at ratio 0.5 and checks that key facts survive.
func qualityTest(fixture eval.Fixture) func() error {
	return func() error {
		var allContent strings.Builder
		for _, m := range fixture.Messages {
			allContent.WriteString(m.Content)
			allContent.WriteString("\n")
		}
		text := allContent.String()

		result := compress.Compact(text, 0.5)
		report := eval.Assess(
			fixture.Name,
			token.Estimate(text),
			token.Estimate(result.Compressed),
			result.Compressed,
			fixture.Facts,
		)

		if !report.Passed {
			return fmt.Errorf("score %.0f%% (%d/%d facts, %d/%d required)",
				report.Score*100, report.FactsFound, report.TotalFacts,
				report.RequiredFound, report.RequiredFacts)
		}
		return nil
	}
}

// compactionAppliedTest sends a large context request and verifies that the
// compactor activates and saves tokens.
func compactionAppliedTest(overrides map[string]string) func() error {
	return func() error {
		store := session.NewStore()
		comp := compactor.New(compactor.DefaultConfig(), store)

		p := proxy.New(proxy.Options{
			ProviderOverride: overrides,
			Compactor:        comp,
			ContextSource:    &tkctx.NilSource{},
		})

		body, headers := testutil.LargeContext("anthropic")
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}

		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			return fmt.Errorf("expected 200, got %d", rec.Code)
		}
		if p.Stats.Compactions.Load() == 0 {
			return fmt.Errorf("expected compaction to trigger, but 0 compactions recorded")
		}
		saved := p.Stats.TokensSaved.Load()
		if saved <= 0 {
			return fmt.Errorf("expected tokens saved > 0, got %d", saved)
		}
		return nil
	}
}
