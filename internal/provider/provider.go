package provider

import (
	"net/http"
	"strings"
)

type Provider struct {
	Name         string
	UpstreamBase string
}

func Detect(r *http.Request) Provider {
	if r.Header.Get("anthropic-version") != "" ||
		strings.HasPrefix(r.Header.Get("x-api-key"), "sk-ant") {
		return Provider{Name: "anthropic", UpstreamBase: "https://api.anthropic.com"}
	}

	auth := r.Header.Get("Authorization")
	path := r.URL.Path
	if strings.HasPrefix(auth, "Bearer ") &&
		(strings.Contains(path, "/chat/completions") ||
			strings.Contains(path, "/completions") ||
			strings.Contains(path, "/embeddings")) {
		return Provider{Name: "openai", UpstreamBase: "https://api.openai.com"}
	}

	if strings.Contains(path, "/v1beta/models") ||
		(strings.Contains(path, "/v1/models") && strings.Contains(path, ":generateContent")) {
		return Provider{Name: "google", UpstreamBase: "https://generativelanguage.googleapis.com"}
	}

	if r.Header.Get("x-api-key") != "" && strings.Contains(path, "/chat/completions") {
		return Provider{Name: "openrouter", UpstreamBase: "https://openrouter.ai/api"}
	}

	return Provider{Name: "unknown", UpstreamBase: ""}
}
