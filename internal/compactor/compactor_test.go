package compactor

import (
	"strings"
	"testing"
	"time"

	"github.com/marijus001/tokara/internal/message"
	"github.com/marijus001/tokara/internal/session"
)

func makeMessages(count int, tokensEach int) []message.Message {
	msgs := make([]message.Message, count)
	content := strings.Repeat("a", tokensEach*4) // ~tokensEach tokens
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = message.Message{Role: role, Content: content}
	}
	return msgs
}

func TestPassThroughWhenSmall(t *testing.T) {
	store := session.NewStore()
	c := New(Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 4,
		ModelContextWindow:  128000,
	}, store)

	// Small conversation — well under 60%
	msgs := makeMessages(3, 100) // ~300 tokens total
	result := c.Process("test", msgs, "")

	if result.Action != ActionPassThrough {
		t.Errorf("expected pass-through, got %d", result.Action)
	}
	if len(result.Messages) != 3 {
		t.Errorf("expected 3 messages unchanged, got %d", len(result.Messages))
	}
}

func TestPrecomputeTriggeredAtThreshold(t *testing.T) {
	store := session.NewStore()
	c := New(Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 4,
		ModelContextWindow:  1000, // Small window for testing
	}, store)

	// 650 tokens = 65% of 1000 — above precompute, below compact
	msgs := makeMessages(6, 100) // ~600 tokens + overhead
	result := c.Process("test", msgs, "")

	if result.Action != ActionPrecompute {
		t.Errorf("expected precompute, got %d", result.Action)
	}
	// Messages unchanged (just precomputing in background)
	if len(result.Messages) != 6 {
		t.Errorf("expected 6 messages, got %d", len(result.Messages))
	}

	// Wait for background goroutine
	time.Sleep(100 * time.Millisecond)

	sess := store.Get("test")
	state, _, _ := sess.GetCompaction()

	if state != session.StateReady {
		t.Errorf("expected compaction ready after precompute, got %d", state)
	}
}

func TestCompactionAppliedOverThreshold(t *testing.T) {
	store := session.NewStore()
	c := New(Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 2,
		ModelContextWindow:  500, // Very small for testing
	}, store)

	// 8 messages * ~100 tokens = ~800 tokens = 160% of 500 — way over
	msgs := makeMessages(8, 100)
	result := c.Process("test", msgs, "")

	if result.Action != ActionApplied {
		t.Errorf("expected applied, got %d", result.Action)
	}
	// Should have fewer messages (summary + 2 preserved)
	if len(result.Messages) > 4 {
		t.Errorf("expected ≤4 messages after compaction, got %d", len(result.Messages))
	}
	// First message should be the summary
	if !strings.Contains(result.Messages[0].Content, "[Conversation summary]") {
		t.Error("first message should be conversation summary")
	}
	// Should achieve some compression
	if result.CompactedTokens >= result.OriginalTokens {
		t.Error("compaction should reduce token count")
	}
}

func TestPrecomputedCompactionUsed(t *testing.T) {
	store := session.NewStore()
	cfg := Config{
		PrecomputeThreshold: 0.60,
		CompactThreshold:    0.80,
		PreserveRecentTurns: 2,
		ModelContextWindow:  500,
	}
	c := New(cfg, store)

	// Pre-set a ready compaction
	sess := store.Get("precomp")
	sess.SetCompactionReady("This is the precomputed summary of old context.", 15)

	msgs := makeMessages(8, 100) // Over threshold
	result := c.Process("precomp", msgs, "")

	if result.Action != ActionAlreadyReady {
		t.Errorf("expected already-ready, got %d", result.Action)
	}
	if !strings.Contains(result.Messages[0].Content, "precomputed summary") {
		t.Error("should use precomputed summary")
	}
}

func TestLooksLikeCode(t *testing.T) {
	if !looksLikeCode("function validateToken(t) { return true; }") {
		t.Error("should detect function keyword")
	}
	if !looksLikeCode("import React from 'react'") {
		t.Error("should detect import")
	}
	if looksLikeCode("Hello, how are you doing today?") {
		t.Error("should not detect prose as code")
	}
}
