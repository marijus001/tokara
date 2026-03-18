package compress

import (
	"strings"

	"github.com/marijus001/tokara/internal/token"
)

const refusalThreshold = 0.9

// Result holds the output of a compression operation.
type Result struct {
	Compressed      string
	OriginalTokens  int
	CompressedTokens int
	Ratio           float64
	Rejected        bool
}

type lineEntry struct {
	line     string
	category string
	index    int
	tokens   int
	priority int
}

// NormalizeRatio converts a compression ratio to the fraction of tokens to remove.
// Supports both 0-1 fraction and >1 direct factor (e.g. 2 = 2x compression).
func NormalizeRatio(ratio float64) float64 {
	if ratio > 1 {
		r := 1 - (1 / ratio)
		if r > 0.99 {
			return 0.99
		}
		return r
	}
	if ratio < 0 {
		return 0
	}
	if ratio > 0.99 {
		return 0.99
	}
	return ratio
}

// Compact performs structure-preserving line-level compression.
// It progressively removes lines by semantic category until the token budget is met.
// Removal order: blank → debug → comment → body → key_body.
// Signatures, types, imports, and structural markers are never removed.
func Compact(text string, targetRatio float64) Result {
	if text == "" {
		return Result{}
	}

	originalTokens := token.Estimate(text)
	removeRatio := NormalizeRatio(targetRatio)
	targetTokens := max(1, int(float64(originalTokens)*(1-removeRatio)))

	if targetTokens >= originalTokens {
		return Result{
			Compressed:       text,
			OriginalTokens:   originalTokens,
			CompressedTokens: originalTokens,
			Ratio:            1,
		}
	}

	lines := strings.Split(text, "\n")
	kept := make([]lineEntry, len(lines))
	for i, line := range lines {
		kept[i] = lineEntry{
			line:     line,
			category: ClassifyLine(line),
			index:    i,
			tokens:   max(1, (len(line)+3)/4),
		}
	}

	currentTokens := sumTokens(kept)

	// Phase 1: Collapse consecutive blanks
	if currentTokens > targetTokens {
		kept = collapseBlankLines(kept)
		currentTokens = sumTokens(kept)
	}

	// Phase 2: Remove debug statements
	if currentTokens > targetTokens {
		kept = filterCategory(kept, CatDebug)
		currentTokens = sumTokens(kept)
	}

	// Phase 3: Remove comments
	if currentTokens > targetTokens {
		kept = filterCategory(kept, CatComment)
		currentTokens = sumTokens(kept)
	}

	// Phase 4: Scored removal of body + key_body lines
	if currentTokens > targetTokens {
		kept = removeBodyLines(kept, targetTokens)
		currentTokens = sumTokens(kept)
	}

	// Phase 5: Trim imports if still over budget
	if currentTokens > targetTokens {
		kept = trimImports(kept)
		currentTokens = sumTokens(kept)
	}

	// Post-processing: remove orphaned closing delimiters
	kept = removeOrphanedClosers(kept)

	compressed := joinLines(kept)
	compressedTokens := token.Estimate(compressed)
	ratio := float64(compressedTokens) / float64(originalTokens)

	if ratio >= refusalThreshold {
		return Result{
			Compressed:       text,
			OriginalTokens:   originalTokens,
			CompressedTokens: originalTokens,
			Ratio:            1,
			Rejected:         true,
		}
	}

	return Result{
		Compressed:       compressed,
		OriginalTokens:   originalTokens,
		CompressedTokens: compressedTokens,
		Ratio:            ratio,
	}
}

func collapseBlankLines(entries []lineEntry) []lineEntry {
	result := make([]lineEntry, 0, len(entries))
	prevWasBlank := false
	for _, e := range entries {
		if e.category == CatBlank {
			if !prevWasBlank && len(result) > 0 {
				prev := result[len(result)-1]
				if prev.category == CatSignature || prev.category == CatStructural || prev.category == CatFence {
					result = append(result, e)
				}
			}
			prevWasBlank = true
		} else {
			result = append(result, e)
			prevWasBlank = false
		}
	}
	return result
}

func filterCategory(entries []lineEntry, cat string) []lineEntry {
	result := make([]lineEntry, 0, len(entries))
	for _, e := range entries {
		if e.category != cat {
			result = append(result, e)
		}
	}
	return result
}

func removeBodyLines(entries []lineEntry, targetTokens int) []lineEntry {
	// Separate anchors from removable code
	var codeEntries []lineEntry
	var anchors []lineEntry

	for _, e := range entries {
		if e.category == CatBody || e.category == CatKeyBody {
			e.priority = assignPriority(e)
			codeEntries = append(codeEntries, e)
		} else {
			anchors = append(anchors, e)
		}
	}

	anchorTokens := sumTokens(anchors)

	// Sort by priority ascending (lowest value removed first)
	sortByPriority(codeEntries)

	codeTokens := sumTokens(codeEntries)
	removeIdx := 0
	for anchorTokens+codeTokens > targetTokens && removeIdx < len(codeEntries) {
		codeTokens -= codeEntries[removeIdx].tokens
		removeIdx++
	}

	removed := make(map[int]bool)
	for i := 0; i < removeIdx; i++ {
		removed[codeEntries[i].index] = true
	}

	// Filter out removed entries, preserving original order
	result := make([]lineEntry, 0, len(entries))
	for _, e := range entries {
		if (e.category != CatBody && e.category != CatKeyBody) || !removed[e.index] {
			result = append(result, e)
		}
	}
	return result
}

func assignPriority(e lineEntry) int {
	trimmed := strings.TrimSpace(e.line)
	if e.category == CatKeyBody {
		if strings.HasPrefix(trimmed, "return ") {
			return 8
		}
		return 7
	}
	if reCloseBrace.MatchString(trimmed) {
		return 6
	}
	if reVarDecl.MatchString(trimmed) {
		return 5
	}
	if reControlFlow.MatchString(trimmed) {
		return 4
	}
	if reAssignment.MatchString(trimmed) && !reDoubleEq.MatchString(trimmed) {
		return 3
	}
	if len(trimmed) < 20 {
		return 1
	}
	return 2
}

func trimImports(entries []lineEntry) []lineEntry {
	var imports []lineEntry
	var nonImports []lineEntry
	for _, e := range entries {
		if e.category == CatImport {
			imports = append(imports, e)
		} else {
			nonImports = append(nonImports, e)
		}
	}
	keepN := max(3, int(float64(len(imports))*0.3+0.5))
	if keepN > len(imports) {
		keepN = len(imports)
	}
	result := append(nonImports, imports[:keepN]...)
	sortByIndex(result)
	return result
}

func removeOrphanedClosers(entries []lineEntry) []lineEntry {
	depth := 0
	result := make([]lineEntry, 0, len(entries))
	for _, e := range entries {
		trimmed := strings.TrimSpace(e.line)
		if reCloseBrace.MatchString(trimmed) {
			if depth <= 0 {
				continue
			}
			depth--
		} else {
			opens := strings.Count(e.line, "{")
			closes := strings.Count(e.line, "}")
			depth = max(0, depth+opens-closes)
		}
		result = append(result, e)
	}
	return result
}

func sumTokens(entries []lineEntry) int {
	total := 0
	for _, e := range entries {
		total += e.tokens
	}
	return total
}

func joinLines(entries []lineEntry) string {
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.line
	}
	return strings.Join(lines, "\n")
}

func sortByPriority(entries []lineEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].priority < entries[j-1].priority; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func sortByIndex(entries []lineEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].index < entries[j-1].index; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}
