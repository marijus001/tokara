package session

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// CompactionState tracks the background compaction status.
type CompactionState int

const (
	StateIdle    CompactionState = iota
	StatePending                 // Background compaction in progress
	StateReady                   // Precomputed compaction available
)

// Turn represents a single conversation turn.
type Turn struct {
	Role      string
	Content   string
	Tokens    int
	Timestamp time.Time
}

// Session tracks the state of a single conversation.
type Session struct {
	ID               string
	Turns            []Turn
	TotalTokens      int
	CompactionState  CompactionState
	CompactedSummary string // Precomputed compacted context
	CompactedTokens  int
	LastActivity     time.Time
	mu               sync.Mutex
}

// AddTurn appends a turn and updates token count.
func (s *Session) AddTurn(role, content string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Turns = append(s.Turns, Turn{
		Role:      role,
		Content:   content,
		Tokens:    tokens,
		Timestamp: time.Now(),
	})
	s.TotalTokens += tokens
	s.LastActivity = time.Now()
}

// TurnCount returns the number of turns.
func (s *Session) TurnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Turns)
}

// SetCompactionReady stores a precomputed compaction result.
func (s *Session) SetCompactionReady(summary string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CompactedSummary = summary
	s.CompactedTokens = tokens
	s.CompactionState = StateReady
}

// SetCompactionPending marks compaction as in progress.
func (s *Session) SetCompactionPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CompactionState = StatePending
}

// GetCompaction returns the current compaction state, summary, and token count.
func (s *Session) GetCompaction() (CompactionState, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CompactionState, s.CompactedSummary, s.CompactedTokens
}

// ResetCompaction clears compaction state after it's applied.
func (s *Session) ResetCompaction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CompactionState = StateIdle
	s.CompactedSummary = ""
	s.CompactedTokens = 0
}

// Store manages all active sessions.
type Store struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewStore creates an empty session store.
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
	}
}

// Get returns a session by ID, creating one if it doesn't exist.
func (s *Store) Get(id string) *Session {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if ok {
		return sess
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock
	if sess, ok := s.sessions[id]; ok {
		return sess
	}
	sess = &Session{
		ID:           id,
		LastActivity: time.Now(),
	}
	s.sessions[id] = sess
	return sess
}

// SessionID generates a deterministic session ID from provider + model + system prompt.
func SessionID(provider, model, systemPrompt string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:%s", provider, model, systemPrompt)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// Count returns the number of active sessions.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Cleanup removes sessions that haven't been active for the given duration.
func (s *Store) Cleanup(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, sess := range s.sessions {
		sess.mu.Lock()
		if sess.LastActivity.Before(cutoff) {
			delete(s.sessions, id)
			removed++
		}
		sess.mu.Unlock()
	}
	return removed
}
