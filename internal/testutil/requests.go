package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Token targets for context size helpers.
// Based on 128K context window with 60%/80% thresholds.
const (
	smallTokenTarget  = 10_000  // ~10K tokens, under 60% precompute threshold
	mediumTokenTarget = 80_000  // ~80K tokens, between 60%-80% (triggers precompute)
	largeTokenTarget  = 110_000 // ~110K tokens, over 80% (triggers compaction)
)

// Default models per provider.
const (
	DefaultAnthropicModel = "claude-3-5-sonnet-20241022"
	DefaultOpenAIModel    = "gpt-4o"
	DefaultGoogleModel    = "gemini-1.5-pro"
)

// systemPrompt is a realistic system prompt used across all providers.
const systemPrompt = "You are an expert Go developer and code reviewer. " +
	"Analyze code for correctness, performance, and idiomatic style. " +
	"Suggest improvements and provide refactored examples when appropriate."

// AnthropicRequest builds an Anthropic Messages API request JSON with the given
// model, conversation turns, and approximate token target. Returns body bytes
// and headers suitable for sending to the gateway.
func AnthropicRequest(model string, turns int, tokenTarget int) ([]byte, http.Header) {
	conversation := buildConversation(turns, tokenTarget)

	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages":   conversation,
	}

	data, _ := json.Marshal(body)

	headers := http.Header{}
	headers.Set("x-api-key", "sk-ant-test-key")
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("Content-Type", "application/json")

	return data, headers
}

// AnthropicStreamingRequest builds an Anthropic Messages API request with
// streaming enabled. Same as AnthropicRequest but adds "stream": true.
func AnthropicStreamingRequest(model string, turns int, tokenTarget int) ([]byte, http.Header) {
	conversation := buildConversation(turns, tokenTarget)

	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages":   conversation,
		"stream":     true,
	}

	data, _ := json.Marshal(body)

	headers := http.Header{}
	headers.Set("x-api-key", "sk-ant-test-key")
	headers.Set("anthropic-version", "2023-06-01")
	headers.Set("Content-Type", "application/json")

	return data, headers
}

// OpenAIRequest builds an OpenAI Chat Completions API request JSON. The system
// prompt is included as the first message with role "system".
func OpenAIRequest(model string, turns int, tokenTarget int) ([]byte, http.Header) {
	conversation := buildConversation(turns, tokenTarget)

	// OpenAI puts the system prompt as the first message in the array
	messages := make([]map[string]interface{}, 0, 1+len(conversation))
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": systemPrompt,
	})
	messages = append(messages, conversation...)

	body := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	data, _ := json.Marshal(body)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer sk-test-key")
	headers.Set("Content-Type", "application/json")

	return data, headers
}

// GoogleRequest builds a Google Gemini generateContent API request JSON with
// the contents array and systemInstruction fields.
func GoogleRequest(model string, turns int, tokenTarget int) ([]byte, http.Header) {
	conversation := buildConversation(turns, tokenTarget)

	// Convert conversation turns to Gemini content format
	contents := make([]map[string]interface{}, 0, len(conversation))
	for _, turn := range conversation {
		role, _ := turn["role"].(string)
		content, _ := turn["content"].(string)
		// Gemini uses "model" instead of "assistant"
		geminiRole := role
		if geminiRole == "assistant" {
			geminiRole = "model"
		}
		contents = append(contents, map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": content},
			},
			"role": geminiRole,
		})
	}

	body := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
	}

	data, _ := json.Marshal(body)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	return data, headers
}

// SmallContext returns a request payload with ~10K tokens, well under the 60%
// precompute threshold for a 128K context window. Provider must be one of
// "anthropic", "openai", or "google".
func SmallContext(provider string) ([]byte, http.Header) {
	return contextRequest(provider, 6, smallTokenTarget, false)
}

// MediumContext returns a request payload with ~80K tokens, in the 60-80%
// range that triggers precompute but not compaction.
func MediumContext(provider string) ([]byte, http.Header) {
	return contextRequest(provider, 20, mediumTokenTarget, false)
}

// LargeContext returns a request payload with ~110K tokens, over the 80%
// compaction threshold.
func LargeContext(provider string) ([]byte, http.Header) {
	return contextRequest(provider, 30, largeTokenTarget, false)
}

// LargeStreamingContext returns a large context (~110K tokens) with streaming
// enabled. Only meaningful for Anthropic; other providers return the same as
// LargeContext.
func LargeStreamingContext(provider string) ([]byte, http.Header) {
	return contextRequest(provider, 30, largeTokenTarget, true)
}

// contextRequest dispatches to the appropriate provider request builder.
func contextRequest(provider string, turns int, tokenTarget int, stream bool) ([]byte, http.Header) {
	switch provider {
	case "anthropic":
		if stream {
			return AnthropicStreamingRequest(DefaultAnthropicModel, turns, tokenTarget)
		}
		return AnthropicRequest(DefaultAnthropicModel, turns, tokenTarget)
	case "openai":
		return OpenAIRequest(DefaultOpenAIModel, turns, tokenTarget)
	case "google":
		return GoogleRequest(DefaultGoogleModel, turns, tokenTarget)
	default:
		return AnthropicRequest(DefaultAnthropicModel, turns, tokenTarget)
	}
}

// buildConversation generates a multi-turn conversation alternating user and
// assistant messages. User messages contain realistic code review requests with
// Go code snippets. Assistant messages contain refactored code with
// explanations. Content is padded to hit the approximate token target
// (~4 chars per token).
func buildConversation(turns int, tokenTarget int) []map[string]interface{} {
	if turns <= 0 {
		turns = 1
	}

	// Reserve some budget for system prompt overhead (~4 chars/token)
	systemTokens := (len(systemPrompt) + 3) / 4
	// Each message has ~4 tokens of framing overhead
	framingOverhead := turns * 4
	remainingTokens := tokenTarget - systemTokens - framingOverhead
	if remainingTokens < 0 {
		remainingTokens = tokenTarget
	}

	// Chars needed total, distributed across all turns
	totalChars := remainingTokens * 4
	charsPerTurn := totalChars / turns
	if charsPerTurn < 100 {
		charsPerTurn = 100
	}

	messages := make([]map[string]interface{}, 0, turns)

	for i := 0; i < turns; i++ {
		if i%2 == 0 {
			// User message: code review request with Go snippet
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": userMessage(i/2, charsPerTurn),
			})
		} else {
			// Assistant message: refactored code with explanation
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": assistantMessage(i/2, charsPerTurn),
			})
		}
	}

	// Ensure the last message is from the user (required by most APIs)
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if role, _ := last["role"].(string); role == "assistant" {
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": userMessage(turns, charsPerTurn),
			})
		}
	}

	return messages
}

// userMessage generates a realistic user code review request with Go code.
func userMessage(turnIndex int, targetChars int) string {
	snippets := []string{
		`Please review this Go HTTP handler for issues:

func HandleUpload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "failed to read file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read data", http.StatusInternalServerError)
		return
	}

	filename := header.Filename
	outPath := filepath.Join("/uploads", filename)
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		http.Error(w, "failed to save", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "uploaded %s (%d bytes)", filename, len(data))
}`,
		`Can you review this database connection pool implementation?

type Pool struct {
	mu      sync.Mutex
	conns   []*sql.DB
	maxSize int
}

func NewPool(dsn string, maxSize int) (*Pool, error) {
	pool := &Pool{maxSize: maxSize}
	for i := 0; i < maxSize; i++ {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open connection %d: %w", i, err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		pool.conns = append(pool.conns, db)
	}
	return pool, nil
}

func (p *Pool) Get() *sql.DB {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.conns) == 0 {
		return nil
	}
	conn := p.conns[len(p.conns)-1]
	p.conns = p.conns[:len(p.conns)-1]
	return conn
}

func (p *Pool) Put(conn *sql.DB) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conns = append(p.conns, conn)
}`,
		`I need a review of this caching middleware:

type Cache struct {
	store    map[string]cacheEntry
	mu       sync.RWMutex
	ttl      time.Duration
}

type cacheEntry struct {
	value     []byte
	headers   http.Header
	status    int
	expiresAt time.Time
}

func (c *Cache) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		key := r.URL.String()
		c.mu.RLock()
		entry, found := c.store[key]
		c.mu.RUnlock()

		if found && time.Now().Before(entry.expiresAt) {
			for k, v := range entry.headers {
				w.Header()[k] = v
			}
			w.WriteHeader(entry.status)
			w.Write(entry.value)
			return
		}

		rec := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}}
		next.ServeHTTP(rec, r)

		c.mu.Lock()
		c.store[key] = cacheEntry{
			value:     rec.body.Bytes(),
			headers:   rec.Header().Clone(),
			status:    rec.statusCode,
			expiresAt: time.Now().Add(c.ttl),
		}
		c.mu.Unlock()
	})
}`,
		`Review this worker pool pattern for correctness:

type Job struct {
	ID      int
	Payload string
}

type Result struct {
	JobID int
	Data  string
	Err   error
}

func RunWorkerPool(jobs []Job, workers int) []Result {
	jobCh := make(chan Job, len(jobs))
	resultCh := make(chan Result, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for job := range jobCh {
				result := processJob(job)
				resultCh <- result
			}
		}(i)
	}

	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var results []Result
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}

func processJob(j Job) Result {
	time.Sleep(100 * time.Millisecond)
	return Result{JobID: j.ID, Data: strings.ToUpper(j.Payload)}
}`,
		`Check this rate limiter implementation:

type RateLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rps,
		lastRefill: time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens < 1.0 {
		return false
	}
	rl.tokens -= 1.0
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}`,
	}

	base := snippets[turnIndex%len(snippets)]
	return padContent(base, targetChars)
}

// assistantMessage generates a realistic assistant code review response.
func assistantMessage(turnIndex int, targetChars int) string {
	responses := []string{
		`Here's my analysis of the HTTP handler with several improvements:

**Security Issues:**
1. Path traversal vulnerability - the filename from the upload is used directly
2. No file size limit - could exhaust server memory
3. World-readable file permissions (0644)

**Refactored version:**

` + "```go" + `
func HandleUpload(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "invalid file upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Sanitize filename to prevent path traversal
	sanitized := filepath.Base(header.Filename)
	if sanitized == "." || sanitized == "/" {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	// Generate unique filename
	ext := filepath.Ext(sanitized)
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	outPath := filepath.Join("/uploads", name)

	dst, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		http.Error(w, "failed to create file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": name,
		"size":     written,
	})
}
` + "```" + `

Key changes: MaxBytesReader for size limits, filepath.Base for sanitization, unique filenames, and restrictive permissions.`,
		`Good catch on requesting a review. Here's my analysis of the connection pool:

**Issues Found:**
1. sql.DB already manages its own connection pool internally - wrapping it adds unnecessary complexity
2. No context support for cancellation
3. Get() returns nil when pool is empty instead of blocking or returning an error
4. No health checking on returned connections

**Recommended approach - use sql.DB directly:**

` + "```go" + `
type DBPool struct {
	db *sql.DB
}

func NewDBPool(dsn string, maxOpen, maxIdle int) (*DBPool, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(30 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DBPool{db: db}, nil
}

func (p *DBPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

func (p *DBPool) Close() error {
	return p.db.Close()
}
` + "```" + `

The standard library's sql.DB handles connection pooling, health checks, and reconnection automatically. Your custom pool was reimplementing this less robustly.`,
		`Here's my review of the caching middleware:

**Issues:**
1. Unbounded cache growth - no eviction policy
2. Cache key doesn't account for headers (Accept, Authorization)
3. responseRecorder captures entire response in memory
4. Stale entries are never cleaned up

**Improved version with LRU eviction:**

` + "```go" + `
type LRUCache struct {
	maxEntries int
	ttl        time.Duration
	mu         sync.RWMutex
	entries    map[string]*list.Element
	order      *list.List
}

type lruEntry struct {
	key       string
	value     []byte
	headers   http.Header
	status    int
	expiresAt time.Time
}

func NewLRUCache(maxEntries int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (c *LRUCache) Get(key string) (*lruEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*lruEntry)
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return entry, true
}
` + "```" + `

This adds proper eviction, prevents unbounded memory growth, and still provides O(1) lookups.`,
		`Analysis of the worker pool pattern:

**The implementation is mostly correct, but has subtle issues:**

1. Results may not be in order (channel receives are non-deterministic)
2. No error handling for panics in worker goroutines
3. No context support for cancellation
4. Worker ID parameter is captured but unused

**Improved version:**

` + "```go" + `
func RunWorkerPool(ctx context.Context, jobs []Job, workers int) ([]Result, error) {
	jobCh := make(chan Job, len(jobs))
	resultCh := make(chan Result, len(jobs))
	errCh := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("worker panic: %v", r)
				}
			}()
			for job := range jobCh {
				select {
				case <-ctx.Done():
					return
				default:
					resultCh <- processJob(job)
				}
			}
		}()
	}

	for _, job := range jobs {
		select {
		case jobCh <- job:
		case <-ctx.Done():
			close(jobCh)
			return nil, ctx.Err()
		}
	}
	close(jobCh)

	go func() {
		wg.Wait()
		close(resultCh)
		close(errCh)
	}()

	results := make([]Result, 0, len(jobs))
	for r := range resultCh {
		results = append(results, r)
	}

	// Sort by JobID to ensure deterministic output
	sort.Slice(results, func(i, j int) bool {
		return results[i].JobID < results[j].JobID
	})

	return results, nil
}
` + "```" + `

Key improvements: context cancellation, panic recovery, and sorted results for determinism.`,
		`Rate limiter review:

**The token bucket implementation is correct. Minor improvements:**

1. Consider using atomic operations instead of mutex for better performance
2. The refill calculation can lose precision with very small time intervals
3. No per-client rate limiting (global limiter only)

**Enhanced with per-IP limiting:**

` + "```go" + `
type PerIPRateLimiter struct {
	limiters sync.Map
	rps      float64
	burst    int
	cleanup  time.Duration
}

func NewPerIPRateLimiter(rps float64, burst int) *PerIPRateLimiter {
	rl := &PerIPRateLimiter{
		rps:     rps,
		burst:   burst,
		cleanup: 10 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *PerIPRateLimiter) getLimiter(ip string) *RateLimiter {
	if v, ok := rl.limiters.Load(ip); ok {
		return v.(*RateLimiter)
	}
	limiter := NewRateLimiter(rl.rps, rl.burst)
	actual, _ := rl.limiters.LoadOrStore(ip, limiter)
	return actual.(*RateLimiter)
}

func (rl *PerIPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := rl.getLimiter(ip)
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
` + "```" + `

The per-IP version prevents a single client from consuming the entire rate limit budget.`,
	}

	base := responses[turnIndex%len(responses)]
	return padContent(base, targetChars)
}

// padContent pads or truncates content to approximate the target character count.
// Padding uses realistic-looking commentary text to avoid obviously fake filler.
func padContent(base string, targetChars int) string {
	if len(base) >= targetChars {
		return base[:targetChars]
	}

	padding := generatePadding(targetChars - len(base))
	return base + padding
}

// generatePadding creates realistic-looking filler text to reach the target size.
func generatePadding(chars int) string {
	paragraphs := []string{
		"\n\nAdditionally, consider the error handling patterns throughout this code. " +
			"Go's explicit error handling is a strength when used properly, but the current " +
			"implementation could benefit from wrapping errors with additional context using " +
			"fmt.Errorf and the %w verb. This makes debugging production issues significantly " +
			"easier because you can trace the full call path that led to the error.",
		"\n\nRegarding performance characteristics, the current approach allocates on each " +
			"invocation which puts pressure on the garbage collector. Consider using sync.Pool " +
			"for frequently allocated objects, or pre-allocating buffers where the maximum size " +
			"is known. Profiling with pprof would help identify the exact allocation hotspots " +
			"and guide optimization efforts.",
		"\n\nFrom a testing perspective, this code would benefit from table-driven tests that " +
			"cover edge cases like empty inputs, maximum size boundaries, concurrent access " +
			"patterns, and graceful degradation under load. The testing package's t.Parallel() " +
			"can help verify that concurrent usage is safe, and t.Cleanup() ensures resources " +
			"are properly released even when tests fail.",
		"\n\nThe logging strategy should also be reviewed. Structured logging with fields like " +
			"request ID, user ID, and operation duration makes it much easier to correlate events " +
			"across a distributed system. Consider using slog from the standard library (Go 1.21+) " +
			"which provides structured logging with configurable handlers and minimal overhead " +
			"compared to third-party solutions.",
		"\n\nFor deployment considerations, ensure that graceful shutdown is properly handled. " +
			"The server should listen for SIGTERM and SIGINT signals, stop accepting new connections, " +
			"and allow in-flight requests to complete within a reasonable timeout. The http.Server " +
			"Shutdown method handles this correctly when combined with a context with deadline.",
		"\n\nThe configuration management could be improved by using environment variables with " +
			"sensible defaults, validated at startup. This follows the twelve-factor app methodology " +
			"and makes deployment across different environments straightforward. Consider using " +
			"a configuration struct with validation tags to catch misconfiguration early.",
		"\n\nMemory management is another area worth examining. Large allocations should be " +
			"carefully profiled to ensure they are not causing excessive GC pauses. The runtime " +
			"package provides SetGCPercent and SetMemoryLimit for fine-tuning garbage collection " +
			"behavior based on your application's specific memory usage patterns and latency " +
			"requirements for the service.",
		"\n\nConcurrency safety is critical for production Go services. Beyond using sync.Mutex, " +
			"consider whether the data structure could benefit from lock-free approaches using " +
			"atomic operations, or whether channels would provide a cleaner ownership model. " +
			"The race detector (go test -race) should be part of your CI pipeline to catch " +
			"data races that might not manifest in simple test scenarios.",
	}

	var sb strings.Builder
	sb.Grow(chars)

	for sb.Len() < chars {
		for _, p := range paragraphs {
			if sb.Len() >= chars {
				break
			}
			remaining := chars - sb.Len()
			if remaining < len(p) {
				sb.WriteString(p[:remaining])
			} else {
				sb.WriteString(p)
			}
		}
	}

	result := sb.String()
	if len(result) > chars {
		return result[:chars]
	}
	return result
}

// estimateTokens returns the approximate token count for a string using the
// same formula as the token.Estimate function (~4 chars per token).
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

// RequestTokenCount estimates the total token count of a serialized request
// body, useful for verifying that test payloads hit the intended thresholds.
func RequestTokenCount(body []byte) int {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return estimateTokens(string(body))
	}

	total := 0

	// Count system prompt tokens
	if sys, ok := req["system"].(string); ok {
		total += estimateTokens(sys)
	}
	// Google format system instruction
	if si, ok := req["systemInstruction"].(map[string]interface{}); ok {
		if parts, ok := si["parts"].([]interface{}); ok {
			for _, p := range parts {
				if part, ok := p.(map[string]interface{}); ok {
					if text, ok := part["text"].(string); ok {
						total += estimateTokens(text)
					}
				}
			}
		}
	}

	// Count message tokens
	if msgs, ok := req["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if msg, ok := m.(map[string]interface{}); ok {
				if content, ok := msg["content"].(string); ok {
					total += estimateTokens(content) + 4
				}
			}
		}
	}
	// Google format contents
	if contents, ok := req["contents"].([]interface{}); ok {
		for _, c := range contents {
			if content, ok := c.(map[string]interface{}); ok {
				if parts, ok := content["parts"].([]interface{}); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]interface{}); ok {
							if text, ok := part["text"].(string); ok {
								total += estimateTokens(text) + 4
							}
						}
					}
				}
			}
		}
	}

	return total
}

// DebugRequestSize prints token count information for a request body.
// Useful during test development to verify payloads hit the right thresholds.
func DebugRequestSize(label string, body []byte) string {
	tokens := RequestTokenCount(body)
	pct := float64(tokens) / 128000.0 * 100
	return fmt.Sprintf("%s: ~%d tokens (%.1f%% of 128K window)", label, tokens, pct)
}
