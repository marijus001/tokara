package proxy

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/marijus001/tokara/internal/compactor"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/message"
	"github.com/marijus001/tokara/internal/provider"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/stats"
)

// Options configures the proxy behavior.
type Options struct {
	// ProviderOverride maps provider names to custom upstream URLs (for testing).
	ProviderOverride map[string]string
	// Compactor handles smart hybrid compaction. If nil, no compaction occurs.
	Compactor *compactor.Compactor
	// ContextSource provides RAG context enrichment. If nil, no enrichment.
	ContextSource tkctx.Source
	// StatsCollector records detailed events for the TUI. If nil, no events emitted.
	StatsCollector *stats.Collector
}

// Stats tracks cumulative proxy statistics.
type Stats struct {
	Requests    atomic.Int64
	Compactions atomic.Int64
	TokensSaved atomic.Int64
}

// Proxy is the core reverse proxy handler.
type Proxy struct {
	opts   Options
	client *http.Client
	Stats  Stats
}

// New creates a Proxy with the given options.
func New(opts Options) *Proxy {
	return &Proxy{
		opts: opts,
		client: &http.Client{
			Timeout: 5 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ServeHTTP handles incoming requests by detecting the provider and forwarding.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.Stats.Requests.Add(1)

	prov := provider.Detect(r)

	if prov.Name == "unknown" {
		http.Error(w, `{"error":"unknown provider — could not detect LLM API from request"}`, http.StatusBadGateway)
		return
	}

	upstream := prov.UpstreamBase
	if override, ok := p.opts.ProviderOverride[prov.Name]; ok {
		upstream = override
	}

	// Read body for compaction processing
	var bodyBytes []byte
	if r.Body != nil {
		r.Body = http.MaxBytesReader(nil, r.Body, 100*1024*1024) // 100MB limit
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			http.Error(w, `{"error":"request body too large or unreadable"}`, http.StatusRequestEntityTooLarge)
			return
		}
	}

	// Try to compact if compactor is configured
	finalBody := bodyBytes
	if p.opts.Compactor != nil && len(bodyBytes) > 0 && r.Method == "POST" {
		finalBody = p.maybeCompact(bodyBytes, prov)
	}

	p.forward(w, r, upstream, prov, finalBody)
}

func (p *Proxy) maybeCompact(body []byte, prov provider.Provider) []byte {
	parsed, err := message.ParseRequestBody(bytes.NewReader(body), prov.Name)
	if err != nil || len(parsed.Messages) == 0 {
		return body
	}

	// RAG context enrichment (paid tier)
	if p.opts.ContextSource != nil && p.opts.ContextSource.Available() {
		lastMsg := parsed.Messages[len(parsed.Messages)-1]
		if lastMsg.Role == "user" && lastMsg.Content != "" {
			chunks, err := p.opts.ContextSource.Query(lastMsg.Content, tkctx.QueryOpts{
				MaxTokens: 4000,
				Filter:    true,
			})
			if err == nil && len(chunks) > 0 {
				// Inject RAG context at position 0 (optimal per research)
				ragContent := "[Relevant codebase context]\n"
				for _, c := range chunks {
					ragContent += c.Code + "\n"
				}
				ragMsg := message.Message{Role: "user", Content: ragContent}
				enriched := make([]message.Message, 0, len(parsed.Messages)+1)
				enriched = append(enriched, ragMsg)
				enriched = append(enriched, parsed.Messages...)
				parsed.Messages = enriched
				log.Printf("[rag] injected %d chunks at position 0", len(chunks))
			}
		}
	}

	sessID := session.SessionID(prov.Name, parsed.Model, parsed.SystemPrompt)
	result := p.opts.Compactor.Process(sessID, parsed.Messages, parsed.SystemPrompt)

	switch result.Action {
	case compactor.ActionApplied, compactor.ActionAlreadyReady:
		saved := result.OriginalTokens - result.CompactedTokens
		p.Stats.Compactions.Add(1)
		p.Stats.TokensSaved.Add(int64(saved))

		action := "compacted"
		if result.Action == compactor.ActionAlreadyReady {
			action = "precomputed"
		}
		savedPct := saved * 100 / max(1, result.OriginalTokens)

		log.Printf("[%s] %s %s %dK → %dK (%d%% saved, %d turns)",
			prov.Name, parsed.Model, action,
			result.OriginalTokens/1000, result.CompactedTokens/1000,
			savedPct, result.TurnsCompacted)

		if p.opts.StatsCollector != nil {
			p.opts.StatsCollector.AddEvent(stats.Event{
				Provider: prov.Name,
				Model:    parsed.Model,
				Action:   action,
				InputK:   result.OriginalTokens / 1000,
				OutputK:  result.CompactedTokens / 1000,
				SavedPct: int(savedPct),
			})
		}

		rewritten, err := message.RewriteMessages(parsed, result.Messages)
		if err != nil {
			log.Printf("[compactor] rewrite error: %v", err)
			return body
		}
		return rewritten

	case compactor.ActionPrecompute:
		log.Printf("[%s] %s precomputing (%dK tokens, %d%% of window)",
			prov.Name, parsed.Model,
			result.OriginalTokens/1000,
			result.OriginalTokens*100/max(1, result.OriginalTokens))

		if p.opts.StatsCollector != nil {
			p.opts.StatsCollector.AddEvent(stats.Event{
				Provider: prov.Name,
				Model:    parsed.Model,
				Action:   "precomputing",
				InputK:   result.OriginalTokens / 1000,
			})
		}

	case compactor.ActionPassThrough:
		if p.opts.StatsCollector != nil {
			p.opts.StatsCollector.AddEvent(stats.Event{
				Provider: prov.Name,
				Model:    parsed.Model,
				Action:   "pass-through",
				InputK:   result.OriginalTokens / 1000,
			})
		}
	}

	return body
}

func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, upstream string, prov provider.Provider, body []byte) {
	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, `{"error":"invalid upstream URL"}`, http.StatusBadGateway)
		return
	}

	outURL := *r.URL
	outURL.Scheme = upstreamURL.Scheme
	outURL.Host = upstreamURL.Host

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error":"failed to create upstream request"}`, http.StatusBadGateway)
		return
	}

	// Hop-by-hop headers that must not be forwarded (RFC 7230)
	hopByHop := map[string]bool{
		"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
		"Proxy-Authorization": true, "Te": true, "Trailers": true,
		"Transfer-Encoding": true, "Upgrade": true,
	}
	for key, vals := range r.Header {
		if hopByHop[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, val := range vals {
			outReq.Header.Add(key, val)
		}
	}
	outReq.Host = upstreamURL.Host
	outReq.ContentLength = int64(len(body))
	outReq.Header.Set("Content-Length", strconv.Itoa(len(body)))

	start := time.Now()
	resp, err := p.client.Do(outReq)
	if err != nil {
		log.Printf("[%s] upstream error: %v", prov.Name, err)
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	latency := time.Since(start)
	log.Printf("[%s] %s %s → %d (%dms)", prov.Name, r.Method, r.URL.Path, resp.StatusCode, latency.Milliseconds())

	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response body back (supports SSE)
	if f, ok := w.(http.Flusher); ok {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				f.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}
}
