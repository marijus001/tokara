package compress

import "regexp"

// Line categories, ordered by removal priority (lowest value first).
const (
	CatBlank      = "blank"
	CatDebug      = "debug"
	CatComment    = "comment"
	CatBody       = "body"
	CatKeyBody    = "key_body"
	CatSignature  = "signature"
	CatImport     = "import"
	CatType       = "type"
	CatStructural = "structural"
	CatFence      = "fence"
)

// Precompiled regexes for line classification.
var (
	reDebug       = regexp.MustCompile(`^(console\.\w+\(|debugger|print\(|logging\.\w+\(|logger\.\w+\()`)
	reComment     = regexp.MustCompile(`^(//|/\*|\*/?\s|\*\s)`)
	reHashComment = regexp.MustCompile(`^#[^!{]`)
	reFence       = regexp.MustCompile("^```")
	reMarkdown    = regexp.MustCompile(`^#{1,6}\s`)
	reCSSAtRule   = regexp.MustCompile(`^(@media|@keyframes|@supports|@layer|@import|@font-face|:root\s*\{|:host\s*\{)`)
	reCSSSelector = regexp.MustCompile(`^[.#&*:%\[\w][^\n]*\{\s*$`)
	reYAMLKey     = regexp.MustCompile(`^[a-zA-Z_][\w-]*\s*:`)
	reTOMLSection = regexp.MustCompile(`^\[\w`)
	reImport      = regexp.MustCompile(`^(import\s|from\s|export\s|require\(|module\.exports|using\s)`)
	reExportSig   = regexp.MustCompile(`^export\s+(default\s+)?(function|class|const|let|var|async|interface|type|enum)\s`)
	reTypeDef     = regexp.MustCompile(`^(type\s+\w|interface\s+\w|typedef\s|@dataclass|@interface|enum\s+\w)`)
	reFuncClass   = regexp.MustCompile(`^(export\s+)?(async\s+)?(function\*?\s+\w|class\s+\w)`)
	reLangFunc    = regexp.MustCompile(`^(def\s+\w|fn\s+\w|func\s+\w|pub\s+(fn|async\s+fn|struct|enum|trait|impl)\s)`)
	reArrowFunc   = regexp.MustCompile(`^(const|let|var)\s+\w+\s*=\s*(async\s+)?(\([^)]*\)|[a-zA-Z_$]\w*)\s*=>`)
	reMethod      = regexp.MustCompile(`^(static\s+)?(async\s+)?(get\s+|set\s+)?[a-zA-Z_$]\w*\s*\([^)]*\)\s*[:{]\s*$`)
	reReturn      = regexp.MustCompile(`^(return\s|yield\s|throw\s|raise\s|exports\.\w|module\.exports)`)
	reAPICall     = regexp.MustCompile(`\.(get|post|put|delete|patch|fetch|request|send|emit|dispatch|subscribe|listen|on|use)\s*\(`)
	reThisAssign  = regexp.MustCompile(`^(this|self)\.\w+\s*=`)
	reCloseBrace  = regexp.MustCompile(`^[}\])];?\s*$`)
	reVarDecl     = regexp.MustCompile(`^(const|let|var)\s+`)
	reControlFlow = regexp.MustCompile(`^(if|else|for|while|switch|case|try|catch|finally|do)\b`)
	reAssignment  = regexp.MustCompile(`\s*=\s`)
	reDoubleEq    = regexp.MustCompile(`==`)
)

// ClassifyLine returns the semantic category of a line of code.
func ClassifyLine(line string) string {
	trimmed := trimLeft(line)

	if trimmed == "" {
		return CatBlank
	}

	if reDebug.MatchString(trimmed) {
		return CatDebug
	}
	// Comments — but NOT markdown headers (## ...) or shebangs (#!)
	if reComment.MatchString(trimmed) {
		return CatComment
	}

	// Code fences
	if reFence.MatchString(trimmed) {
		return CatFence
	}

	// Markdown headers: ## and above (2+ hashes). Single # is ambiguous — treat as comment.
	if len(trimmed) > 2 && trimmed[0] == '#' && trimmed[1] == '#' && reMarkdown.MatchString(trimmed) {
		return CatStructural
	}
	if trimmed == "---" {
		return CatStructural
	}

	// Hash comments (Python, Ruby, etc.) — # followed by space or text, but not #! or #{
	if trimmed[0] == '#' && reHashComment.MatchString(trimmed) {
		return CatComment
	}

	// Export with signature — check BEFORE generic import
	if reExportSig.MatchString(trimmed) {
		return CatSignature
	}

	// Import/export/require
	if reImport.MatchString(trimmed) {
		return CatImport
	}

	// Type definitions
	if reTypeDef.MatchString(trimmed) {
		return CatType
	}

	// Function/class/method signatures — check BEFORE CSS selector
	if reFuncClass.MatchString(trimmed) {
		return CatSignature
	}
	if reLangFunc.MatchString(trimmed) {
		return CatSignature
	}
	if reArrowFunc.MatchString(trimmed) {
		return CatSignature
	}
	if reMethod.MatchString(trimmed) {
		return CatSignature
	}

	// CSS/structural — now safe to check after signatures are handled
	if reCSSAtRule.MatchString(trimmed) {
		return CatStructural
	}
	if reCSSSelector.MatchString(trimmed) {
		return CatStructural
	}
	if reTOMLSection.MatchString(trimmed) {
		return CatStructural
	}

	// YAML key: value
	if reYAMLKey.MatchString(trimmed) && !containsChar(trimmed, '(') {
		return CatKeyBody
	}

	// Key body lines
	if reReturn.MatchString(trimmed) {
		return CatKeyBody
	}
	if reAPICall.MatchString(trimmed) {
		return CatKeyBody
	}
	if reThisAssign.MatchString(trimmed) {
		return CatKeyBody
	}

	return CatBody
}

// IsAnchor returns true if the category is never removed during compression.
func IsAnchor(cat string) bool {
	switch cat {
	case CatSignature, CatImport, CatType, CatStructural, CatFence:
		return true
	}
	return false
}

func trimLeft(s string) string {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return s[i:]
		}
	}
	return ""
}

func containsChar(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}
