package context

import (
	"fmt"
	"log"

	"github.com/marijus001/tokara/internal/api"
)

// CloudSource queries the Tokara hosted API for context.
type CloudSource struct {
	client *api.Client
}

// NewCloudSource creates a cloud-backed context source.
func NewCloudSource(client *api.Client) *CloudSource {
	return &CloudSource{client: client}
}

// Query retrieves relevant code chunks from the Tokara API.
func (c *CloudSource) Query(query string, opts QueryOpts) ([]Chunk, error) {
	resp, err := c.client.Query(api.QueryRequest{
		Query:     query,
		Limit:     opts.Limit,
		MaxTokens: opts.MaxTokens,
		Compress:  opts.Compress,
		Filter:    opts.Filter,
		ProjectID: opts.ProjectID,
	})
	if err != nil {
		return nil, fmt.Errorf("cloud query failed: %w", err)
	}

	log.Printf("[cloud] query=%q results=%d tokens=%d", query, resp.TotalResults, resp.TokenCount)

	// The API returns pre-formatted LLM context as a single string.
	// Wrap it as a single chunk for the proxy to inject.
	if resp.LLMContext == "" {
		return nil, nil
	}

	return []Chunk{{
		Code:   resp.LLMContext,
		Tokens: resp.TokenCount,
		Type:   "rag-context",
	}}, nil
}

// Available checks if the API is reachable.
func (c *CloudSource) Available() bool {
	resp, err := c.client.Health()
	if err != nil {
		return false
	}
	return resp.Status == "ok"
}

// Name returns "cloud".
func (c *CloudSource) Name() string {
	return "cloud"
}
