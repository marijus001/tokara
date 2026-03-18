package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with the Tokara hosted API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Tokara API client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CompressRequest is the payload for POST /compress.
type CompressRequest struct {
	Text     string  `json:"text"`
	Strategy string  `json:"strategy"` // "compact", "distill"
	Ratio    float64 `json:"ratio"`
	Query    string  `json:"query,omitempty"` // Required for distill
}

// CompressResponse is returned from POST /compress.
type CompressResponse struct {
	Compressed       string  `json:"compressed"`
	OriginalTokens   int     `json:"originalTokens"`
	CompressedTokens int     `json:"compressedTokens"`
	Ratio            float64 `json:"ratio"`
	Strategy         string  `json:"strategy"`
	Rejected         bool    `json:"rejected"`
}

// Compress calls the /compress endpoint.
func (c *Client) Compress(req CompressRequest) (*CompressResponse, error) {
	var resp CompressResponse
	if err := c.post("/compress", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IngestFile represents a single file to ingest.
type IngestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// IngestRequest is the payload for POST /ingest.
type IngestRequest struct {
	Files     []IngestFile `json:"files"`
	Clear     bool         `json:"clear,omitempty"`
	ProjectID string       `json:"projectId,omitempty"`
}

// IngestResponse is returned from POST /ingest.
type IngestResponse struct {
	Message string `json:"message"`
	Chunks  int    `json:"chunks"`
	Store   string `json:"store"`
	Model   string `json:"model"`
}

// Ingest calls the /ingest endpoint to index files.
func (c *Client) Ingest(req IngestRequest) (*IngestResponse, error) {
	var resp IngestResponse
	if err := c.post("/ingest", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// QueryRequest is the payload for POST /query.
type QueryRequest struct {
	Query     string  `json:"query"`
	Limit     int     `json:"limit,omitempty"`
	MaxTokens int     `json:"maxTokens,omitempty"`
	Compress  float64 `json:"compress,omitempty"`
	Filter    bool    `json:"filter,omitempty"`
	ProjectID string  `json:"projectId,omitempty"`
}

// QueryResponse is returned from POST /query.
type QueryResponse struct {
	LLMContext   string `json:"llmContext"`
	TokenCount   int    `json:"tokenCount"`
	TotalResults int    `json:"totalResults"`
}

// Query calls the /query endpoint for RAG retrieval.
func (c *Client) Query(req QueryRequest) (*QueryResponse, error) {
	var resp QueryResponse
	if err := c.post("/query", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// HealthResponse from GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health checks API connectivity.
func (c *Client) Health() (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.get("/health", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) post(path string, body interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode error: %w", err)
		}
	}
	return nil
}

func (c *Client) get(path string, result interface{}) error {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode error: %w", err)
		}
	}
	return nil
}
