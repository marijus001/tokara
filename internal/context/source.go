// Package context defines the pluggable context source interface.
// The proxy queries a context source to enrich LLM requests with
// relevant code from an indexed codebase.
package context

// Chunk represents a piece of retrieved code context.
type Chunk struct {
	Code     string  `json:"code"`
	File     string  `json:"file"`
	Language string  `json:"language"`
	Type     string  `json:"type"` // function, class, module, etc.
	Tokens   int     `json:"tokens"`
	Score    float64 `json:"score"`
}

// QueryOpts configures a context query.
type QueryOpts struct {
	MaxTokens int
	Limit     int
	Compress  float64 // 0 = no compression, 0.5 = balanced, etc.
	Filter    bool    // Apply relevance filtering
	ProjectID string
}

// Source is the interface for context backends.
type Source interface {
	// Query retrieves relevant code chunks for the given query string.
	Query(query string, opts QueryOpts) ([]Chunk, error)

	// Available returns true if the source is configured and reachable.
	Available() bool

	// Name returns the source type for logging.
	Name() string
}

// NilSource is a no-op context source for the free tier.
type NilSource struct{}

func (n *NilSource) Query(query string, opts QueryOpts) ([]Chunk, error) {
	return nil, nil
}

func (n *NilSource) Available() bool {
	return false
}

func (n *NilSource) Name() string {
	return "none"
}
