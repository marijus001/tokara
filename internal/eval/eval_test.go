package eval

import (
	"fmt"
	"testing"

	"github.com/marijus001/tokara/internal/compress"
	"github.com/marijus001/tokara/internal/token"
)

func TestProbeFindsKeywords(t *testing.T) {
	text := `The file internal/auth/handler.go contains the HandleLogin function.
It uses bcrypt with cost 12 and JWT expiry of 24h.
The server listens on port 8080.`

	facts := []Fact{
		{ID: "path", Category: "file_path", Keywords: []string{"internal/auth/handler.go"}, Required: true},
		{ID: "func", Category: "function", Keywords: []string{"HandleLogin"}, Required: true},
		{ID: "port", Category: "config", Keywords: []string{"8080"}, Required: false},
		{ID: "bcrypt", Category: "config", Keywords: []string{"bcrypt", "cost", "12"}, Required: false},
	}

	results := Probe(text, facts)

	for _, r := range results {
		if !r.Found {
			t.Errorf("expected fact %q to be found, but it was not", r.Fact.ID)
		}
	}
}

func TestProbeDetectsMissing(t *testing.T) {
	text := `This is a short text about weather and cooking recipes.
Nothing related to code or servers here.`

	facts := []Fact{
		{ID: "missing-path", Category: "file_path", Keywords: []string{"internal/auth/handler.go"}, Required: true},
		{ID: "missing-func", Category: "function", Keywords: []string{"HandleLogin"}, Required: true},
		{ID: "missing-port", Category: "config", Keywords: []string{"8080"}, Required: false},
	}

	results := Probe(text, facts)

	for _, r := range results {
		if r.Found {
			t.Errorf("expected fact %q to NOT be found, but it was", r.Fact.ID)
		}
	}
}

func TestAssessScoring(t *testing.T) {
	// Text that contains 3 out of 4 facts
	text := `File internal/auth/handler.go has HandleLogin.
Port is 8080.`

	facts := []Fact{
		{ID: "path", Category: "file_path", Keywords: []string{"internal/auth/handler.go"}, Required: true},
		{ID: "func", Category: "function", Keywords: []string{"HandleLogin"}, Required: true},
		{ID: "port", Category: "config", Keywords: []string{"8080"}, Required: false},
		{ID: "missing", Category: "config", Keywords: []string{"argon2"}, Required: false},
	}

	report := Assess("test", 1000, 500, text, facts)

	if report.TotalFacts != 4 {
		t.Errorf("expected TotalFacts=4, got %d", report.TotalFacts)
	}
	if report.FactsFound != 3 {
		t.Errorf("expected FactsFound=3, got %d", report.FactsFound)
	}
	if report.RequiredFacts != 2 {
		t.Errorf("expected RequiredFacts=2, got %d", report.RequiredFacts)
	}
	if report.RequiredFound != 2 {
		t.Errorf("expected RequiredFound=2, got %d", report.RequiredFound)
	}

	expectedScore := 3.0 / 4.0
	if report.Score != expectedScore {
		t.Errorf("expected Score=%f, got %f", expectedScore, report.Score)
	}
	if report.CompressionRatio != 0.5 {
		t.Errorf("expected CompressionRatio=0.5, got %f", report.CompressionRatio)
	}
	if !report.Passed {
		t.Error("expected Passed=true (all required found, score 0.75 >= 0.7)")
	}
}

func TestAssessFailsOnMissingRequired(t *testing.T) {
	text := "nothing useful here"

	facts := []Fact{
		{ID: "required-missing", Category: "function", Keywords: []string{"HandleLogin"}, Required: true},
		{ID: "optional-missing", Category: "config", Keywords: []string{"8080"}, Required: false},
	}

	report := Assess("test", 1000, 500, text, facts)

	if report.Passed {
		t.Error("expected Passed=false when required fact is missing")
	}
	if report.RequiredFound != 0 {
		t.Errorf("expected RequiredFound=0, got %d", report.RequiredFound)
	}
}

func TestFixtureCompression(t *testing.T) {
	fixtures := DefaultFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			var allContent string
			for _, m := range fixture.Messages {
				allContent += m.Content + "\n"
			}

			originalTokens := token.Estimate(allContent)
			result := compress.Compact(allContent, 0.5)
			compressedTokens := token.Estimate(result.Compressed)

			report := Assess(fixture.Name, originalTokens, compressedTokens, result.Compressed, fixture.Facts)

			t.Logf("--- Quality Report: %s ---", report.FixtureName)
			t.Logf("Original tokens:   %d", report.OriginalTokens)
			t.Logf("Compressed tokens: %d", report.CompressedTokens)
			t.Logf("Compression ratio: %.2f (fraction kept)", report.CompressionRatio)
			t.Logf("Facts found:       %d / %d", report.FactsFound, report.TotalFacts)
			t.Logf("Required found:    %d / %d", report.RequiredFound, report.RequiredFacts)
			t.Logf("Score:             %.2f", report.Score)
			t.Logf("Passed:            %v", report.Passed)

			for _, r := range report.Results {
				status := "FOUND"
				if !r.Found {
					status = "MISSING"
				}
				req := ""
				if r.Fact.Required {
					req = " [REQUIRED]"
				}
				t.Logf("  %-8s %s (%s)%s", status, r.Fact.ID, r.Fact.Category, req)
			}

			if report.Score < 0.5 {
				t.Errorf("quality score %.2f is below minimum threshold 0.5", report.Score)
			}
		})
	}
}

func TestFixtureAggressiveCompression(t *testing.T) {
	fixtures := DefaultFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			var allContent string
			for _, m := range fixture.Messages {
				allContent += m.Content + "\n"
			}

			originalTokens := token.Estimate(allContent)
			result := compress.Compact(allContent, 0.7)
			compressedTokens := token.Estimate(result.Compressed)

			report := Assess(fixture.Name, originalTokens, compressedTokens, result.Compressed, fixture.Facts)

			t.Logf("--- Aggressive Compression Report: %s ---", report.FixtureName)
			t.Logf("Original tokens:   %d", report.OriginalTokens)
			t.Logf("Compressed tokens: %d", report.CompressedTokens)
			t.Logf("Compression ratio: %.2f (fraction kept)", report.CompressionRatio)
			t.Logf("Facts found:       %d / %d", report.FactsFound, report.TotalFacts)
			t.Logf("Required found:    %d / %d", report.RequiredFound, report.RequiredFacts)
			t.Logf("Score:             %.2f", report.Score)
			t.Logf("Passed:            %v", report.Passed)

			var lost []string
			for _, r := range report.Results {
				if !r.Found {
					req := ""
					if r.Fact.Required {
						req = " [REQUIRED]"
					}
					lost = append(lost, fmt.Sprintf("%s (%s)%s", r.Fact.ID, r.Fact.Category, req))
				}
			}

			if len(lost) > 0 {
				t.Logf("Facts lost at 0.7 compression:")
				for _, l := range lost {
					t.Logf("  - %s", l)
				}
			} else {
				t.Log("No facts lost even at aggressive compression!")
			}

			// At aggressive compression we still expect some quality
			// but the bar is lower than the standard test
			if report.Score < 0.3 {
				t.Errorf("quality score %.2f is below minimum threshold 0.3 even for aggressive compression", report.Score)
			}
		})
	}
}
