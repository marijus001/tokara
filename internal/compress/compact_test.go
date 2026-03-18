package compress

import (
	"strings"
	"testing"
)

func TestCompactEmpty(t *testing.T) {
	r := Compact("", 0.5)
	if r.Compressed != "" || r.OriginalTokens != 0 {
		t.Error("expected empty result for empty input")
	}
}

func TestCompactPreservesSignatures(t *testing.T) {
	code := `import express from 'express'

function validateToken(token) {
  // Check if token is valid
  console.log('validating', token)
  const decoded = jwt.verify(token, SECRET)
  if (!decoded) {
    throw new Error('Invalid token')
  }
  return decoded
}

export default function createServer() {
  const app = express()
  app.use(middleware)
  return app
}`

	r := Compact(code, 0.9) // Aggressive compression

	// Must keep signatures
	if !strings.Contains(r.Compressed, "function validateToken") {
		t.Error("lost function signature: validateToken")
	}
	if !strings.Contains(r.Compressed, "export default function createServer") {
		t.Error("lost function signature: createServer")
	}
	// Must keep imports
	if !strings.Contains(r.Compressed, "import express") {
		t.Error("lost import statement")
	}
	// Should remove debug
	if strings.Contains(r.Compressed, "console.log") {
		t.Error("should have removed console.log")
	}
	// Should achieve significant compression
	if r.Ratio > 0.6 {
		t.Errorf("expected ratio < 0.6 at aggressive compression, got %f", r.Ratio)
	}
}

func TestCompactLightPreservesMore(t *testing.T) {
	code := `function process(data) {
  // Validate input
  console.log('processing', data)
  const result = transform(data)
  return result
}`

	light := Compact(code, 0.2)
	aggressive := Compact(code, 0.9)

	if light.CompressedTokens < aggressive.CompressedTokens {
		t.Error("light compression should keep more tokens than aggressive")
	}
}

func TestCompactRejectsWhenNotWorthwhile(t *testing.T) {
	// Very short code — compression won't achieve much
	code := `function hello() {
  return "hi"
}`

	r := Compact(code, 0.1) // Very light
	if !r.Rejected && r.Ratio >= refusalThreshold {
		t.Error("should reject compression when savings are minimal")
	}
}

func TestCompactRemovesCommentsBeforeBody(t *testing.T) {
	code := `import fs from 'fs'

// This is a comment
// Another comment
function readFile(path) {
  const data = fs.readFileSync(path)
  const parsed = JSON.parse(data)
  return parsed
}`

	r := Compact(code, 0.5)

	// At 0.5 ratio, comments should be gone
	if strings.Contains(r.Compressed, "This is a comment") {
		t.Error("comments should be removed at 0.5 ratio")
	}
	// But return should still be there (key_body has higher priority)
	if !strings.Contains(r.Compressed, "return parsed") {
		t.Error("return statement should be preserved at 0.5 ratio")
	}
}

func TestNormalizeRatio(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{0.5, 0.5},
		{0.0, 0.0},
		{1.5, 0.333}, // > 1: calculated as 1 - 1/1.5 ≈ 0.333
		{2.0, 0.5},
		{10.0, 0.9},
	}
	for _, tt := range tests {
		got := NormalizeRatio(tt.input)
		if got < tt.expected-0.05 || got > tt.expected+0.05 {
			t.Errorf("NormalizeRatio(%f) = %f, want ~%f", tt.input, got, tt.expected)
		}
	}
}
