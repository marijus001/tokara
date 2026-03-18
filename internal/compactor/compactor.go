// Package compactor implements smart hybrid context compaction.
//
// It monitors context size on each request and applies compression
// using a three-phase approach:
//   - < 60% of model window: pass-through (no action)
//   - 60-80%: start background precomputation
//   - > 80%: apply compaction (precomputed if ready, synchronous otherwise)
//
// Compaction priority: old prose → code signatures → verbose tool output.
// Recent turns, tool outputs, and system prompts are preserved.
package compactor

import (
	"strings"
	"sync"

	"github.com/marijus001/tokara/internal/compress"
	"github.com/marijus001/tokara/internal/message"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/token"
)

// Config holds compaction parameters.
type Config struct {
	PrecomputeThreshold float64 // Start background compaction (default 0.60)
	CompactThreshold    float64 // Apply compaction (default 0.80)
	PreserveRecentTurns int     // Never compress last N turns (default 4)
	ModelContextWindow  int     // Model's max context tokens (default 128000)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 4,
		ModelContextWindow:  128000,
	}
}

// Action describes what the compactor decided to do.
type Action int

const (
	ActionPassThrough   Action = iota // No compression needed
	ActionPrecompute                  // Started background precomputation
	ActionApplied                     // Applied compaction to messages
	ActionAlreadyReady                // Applied precomputed compaction
)

// Result holds the outcome of a compaction decision.
type Result struct {
	Action           Action
	Messages         []message.Message // Possibly compacted messages
	OriginalTokens   int
	CompactedTokens  int
	TurnsCompacted   int
}

// Compactor manages smart hybrid context compaction.
type Compactor struct {
	cfg   Config
	store *session.Store
	mu    sync.Mutex
}

// New creates a Compactor with the given config and session store.
func New(cfg Config, store *session.Store) *Compactor {
	return &Compactor{
		cfg:   cfg,
		store: store,
	}
}

// Process evaluates messages and decides whether/how to compact.
// Returns the (possibly modified) messages and the action taken.
func (c *Compactor) Process(sessID string, msgs []message.Message, systemPrompt string) Result {
	totalTokens := token.Estimate(systemPrompt)
	for _, m := range msgs {
		totalTokens += token.Estimate(m.Content) + 4
	}

	sess := c.store.Get(sessID)
	threshold := int(float64(c.cfg.ModelContextWindow) * c.cfg.CompactThreshold)
	precomputeAt := int(float64(c.cfg.ModelContextWindow) * c.cfg.PrecomputeThreshold)

	// Under precompute threshold — pass through
	if totalTokens < precomputeAt {
		return Result{
			Action:          ActionPassThrough,
			Messages:        msgs,
			OriginalTokens:  totalTokens,
			CompactedTokens: totalTokens,
		}
	}

	// Between precompute and compact threshold — start background work if not already running
	if totalTokens < threshold {
		state, _, _ := sess.GetCompaction()

		if state == session.StateIdle {
			go c.precompute(sess, msgs)
		}
		return Result{
			Action:          ActionPrecompute,
			Messages:        msgs,
			OriginalTokens:  totalTokens,
			CompactedTokens: totalTokens,
		}
	}

	// Over compact threshold — apply compaction
	state, summary, summaryTokens := sess.GetCompaction()

	if state == session.StateReady && summary != "" {
		// Use precomputed compaction
		compacted := applyPrecomputed(summary, msgs, c.cfg.PreserveRecentTurns)
		compactedTokens := token.Estimate(systemPrompt)
		for _, m := range compacted {
			compactedTokens += token.Estimate(m.Content) + 4
		}
		sess.ResetCompaction()
		return Result{
			Action:           ActionAlreadyReady,
			Messages:         compacted,
			OriginalTokens:   totalTokens,
			CompactedTokens:  compactedTokens,
			TurnsCompacted:   len(msgs) - len(compacted) + 1, // +1 for summary message
		}
	}

	// No precomputed result — compact synchronously
	compacted, turnsCompacted := c.compactMessages(msgs)
	compactedTokens := token.Estimate(systemPrompt)
	for _, m := range compacted {
		compactedTokens += token.Estimate(m.Content) + 4
	}
	_ = summaryTokens
	sess.ResetCompaction()

	return Result{
		Action:           ActionApplied,
		Messages:         compacted,
		OriginalTokens:   totalTokens,
		CompactedTokens:  compactedTokens,
		TurnsCompacted:   turnsCompacted,
	}
}

// precompute runs compaction in the background and stores the result.
func (c *Compactor) precompute(sess *session.Session, msgs []message.Message) {
	sess.SetCompactionPending()

	compactable := getCompactableMessages(msgs, c.cfg.PreserveRecentTurns)
	if len(compactable) == 0 {
		sess.ResetCompaction()
		return
	}

	summary := compactMessages(compactable)
	summaryTokens := token.Estimate(summary)

	sess.SetCompactionReady(summary, summaryTokens)
}

// compactMessages compacts old messages synchronously.
func (c *Compactor) compactMessages(msgs []message.Message) ([]message.Message, int) {
	preserve := c.cfg.PreserveRecentTurns
	if preserve >= len(msgs) {
		return msgs, 0
	}

	compactable := msgs[:len(msgs)-preserve]
	recent := msgs[len(msgs)-preserve:]

	summary := compactMessages(compactable)
	summaryMsg := message.Message{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary,
	}

	result := make([]message.Message, 0, 1+len(recent))
	result = append(result, summaryMsg) // Position 0 — per research
	result = append(result, recent...)

	return result, len(compactable)
}

// getCompactableMessages returns messages eligible for compaction
// (everything except the last N preserved turns).
func getCompactableMessages(msgs []message.Message, preserve int) []message.Message {
	if preserve >= len(msgs) {
		return nil
	}
	return msgs[:len(msgs)-preserve]
}

// compactMessages takes a slice of messages and produces a compressed summary.
func compactMessages(msgs []message.Message) string {
	var parts []string

	for _, m := range msgs {
		// Determine if this is code or prose
		content := m.Content

		// Check if this looks like tool output (preserve more carefully)
		isToolOutput := m.Role == "tool" ||
			strings.HasPrefix(content, "```") ||
			strings.Contains(content, "$ ") // command output

		if isToolOutput {
			// Tool outputs: apply light compression only (preserve near-verbatim)
			result := compress.Compact(content, 0.2)
			parts = append(parts, "["+m.Role+"] "+result.Compressed)
		} else if looksLikeCode(content) {
			// Code: compress to signatures
			result := compress.Compact(content, 0.7)
			parts = append(parts, "["+m.Role+"] "+result.Compressed)
		} else {
			// Prose: compress aggressively
			result := compress.Compact(content, 0.5)
			parts = append(parts, "["+m.Role+"] "+result.Compressed)
		}
	}

	return strings.Join(parts, "\n\n")
}

// applyPrecomputed builds the final message list with the precomputed summary.
func applyPrecomputed(summary string, msgs []message.Message, preserve int) []message.Message {
	if preserve >= len(msgs) {
		return msgs
	}

	recent := msgs[len(msgs)-preserve:]
	summaryMsg := message.Message{
		Role:    "user",
		Content: "[Conversation summary]\n" + summary,
	}

	result := make([]message.Message, 0, 1+len(recent))
	result = append(result, summaryMsg) // Position 0
	result = append(result, recent...)
	return result
}

// looksLikeCode returns true if content appears to contain code.
func looksLikeCode(content string) bool {
	codeSignals := []string{
		"function ", "class ", "import ", "export ", "const ", "let ", "var ",
		"def ", "fn ", "func ", "pub ", "async ", "return ",
		"if (", "for (", "while (", "switch (",
	}
	for _, sig := range codeSignals {
		if strings.Contains(content, sig) {
			return true
		}
	}
	return false
}
