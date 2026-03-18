package session

import (
	"testing"
	"time"
)

func TestStoreGetCreatesSession(t *testing.T) {
	store := NewStore()
	sess := store.Get("test-id")
	if sess.ID != "test-id" {
		t.Errorf("expected id test-id, got %s", sess.ID)
	}
	if store.Count() != 1 {
		t.Errorf("expected 1 session, got %d", store.Count())
	}
}

func TestStoreGetReturnsSame(t *testing.T) {
	store := NewStore()
	s1 := store.Get("id1")
	s2 := store.Get("id1")
	if s1 != s2 {
		t.Error("expected same session pointer")
	}
}

func TestSessionAddTurn(t *testing.T) {
	sess := &Session{ID: "test"}
	sess.AddTurn("user", "hello world", 3)
	sess.AddTurn("assistant", "hi there", 3)

	if sess.TurnCount() != 2 {
		t.Errorf("expected 2 turns, got %d", sess.TurnCount())
	}
	if sess.TotalTokens != 6 {
		t.Errorf("expected 6 tokens, got %d", sess.TotalTokens)
	}
}

func TestSessionCompactionLifecycle(t *testing.T) {
	sess := &Session{ID: "test"}

	if sess.CompactionState != StateIdle {
		t.Error("should start idle")
	}

	sess.SetCompactionPending()
	if sess.CompactionState != StatePending {
		t.Error("should be pending")
	}

	sess.SetCompactionReady("compacted summary", 50)
	if sess.CompactionState != StateReady {
		t.Error("should be ready")
	}
	if sess.CompactedSummary != "compacted summary" {
		t.Error("summary not stored")
	}

	sess.ResetCompaction()
	if sess.CompactionState != StateIdle {
		t.Error("should be idle after reset")
	}
	if sess.CompactedSummary != "" {
		t.Error("summary should be cleared")
	}
}

func TestSessionID(t *testing.T) {
	id1 := SessionID("anthropic", "claude-sonnet", "system prompt")
	id2 := SessionID("anthropic", "claude-sonnet", "system prompt")
	id3 := SessionID("openai", "gpt-4o", "system prompt")

	if id1 != id2 {
		t.Error("same inputs should produce same ID")
	}
	if id1 == id3 {
		t.Error("different inputs should produce different IDs")
	}
	if len(id1) != 16 {
		t.Errorf("expected 16 char ID, got %d", len(id1))
	}
}

func TestStoreCleanup(t *testing.T) {
	store := NewStore()
	s1 := store.Get("old")
	s1.LastActivity = time.Now().Add(-2 * time.Hour)
	store.Get("new") // fresh session

	removed := store.Cleanup(1 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if store.Count() != 1 {
		t.Errorf("expected 1 remaining, got %d", store.Count())
	}
}
