package provider

import (
	"net/http"
	"testing"
)

func TestDetectAnthropic(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://localhost:18741/v1/messages", nil)
	req.Header.Set("x-api-key", "sk-ant-test")
	req.Header.Set("anthropic-version", "2023-06-01")
	p := Detect(req)
	if p.Name != "anthropic" {
		t.Errorf("expected anthropic, got %s", p.Name)
	}
	if p.UpstreamBase != "https://api.anthropic.com" {
		t.Errorf("unexpected upstream: %s", p.UpstreamBase)
	}
}

func TestDetectOpenAI(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://localhost:18741/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer sk-test123")
	p := Detect(req)
	if p.Name != "openai" {
		t.Errorf("expected openai, got %s", p.Name)
	}
}

func TestDetectGoogle(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://localhost:18741/v1beta/models/gemini-pro:generateContent", nil)
	p := Detect(req)
	if p.Name != "google" {
		t.Errorf("expected google, got %s", p.Name)
	}
}

func TestDetectUnknown(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://localhost:18741/random", nil)
	p := Detect(req)
	if p.Name != "unknown" {
		t.Errorf("expected unknown, got %s", p.Name)
	}
}
