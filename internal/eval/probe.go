package eval

import "strings"

// Probe checks whether facts survive in the compressed output.
func Probe(compressed string, facts []Fact) []FactResult {
	lower := strings.ToLower(compressed)
	var results []FactResult
	for _, fact := range facts {
		found := false
		for _, kw := range fact.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				found = true
				break
			}
		}
		results = append(results, FactResult{Fact: fact, Found: found})
	}
	return results
}

// Assess runs probes and builds a quality report.
func Assess(fixtureName string, originalTokens, compressedTokens int, compressed string, facts []Fact) QualityReport {
	results := Probe(compressed, facts)

	total := len(facts)
	found := 0
	required := 0
	requiredFound := 0
	for _, r := range results {
		if r.Found {
			found++
		}
		if r.Fact.Required {
			required++
			if r.Found {
				requiredFound++
			}
		}
	}

	score := 0.0
	if total > 0 {
		score = float64(found) / float64(total)
	}

	return QualityReport{
		FixtureName:      fixtureName,
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		CompressionRatio: float64(compressedTokens) / float64(max(originalTokens, 1)),
		TotalFacts:       total,
		FactsFound:       found,
		RequiredFacts:    required,
		RequiredFound:    requiredFound,
		Score:            score,
		Results:          results,
		Passed:           requiredFound == required && score >= 0.7,
	}
}
