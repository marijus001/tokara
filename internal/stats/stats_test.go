package stats

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCollectorAddEvent(t *testing.T) {
	c := NewCollector(5)
	c.AddEvent(Event{Provider: "anthropic", Action: "compacted"})
	c.AddEvent(Event{Provider: "openai", Action: "pass-through"})

	events := c.RecentEvents(10)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	// Most recent first
	if events[0].Provider != "openai" {
		t.Error("newest event should be first")
	}
}

func TestCollectorMaxEvents(t *testing.T) {
	c := NewCollector(3)
	for i := 0; i < 10; i++ {
		c.AddEvent(Event{Provider: "test", Action: "compacted"})
	}

	events := c.RecentEvents(10)
	if len(events) != 3 {
		t.Errorf("expected max 3 events, got %d", len(events))
	}
}

func TestFormatUptime(t *testing.T) {
	c := NewCollector(5)
	// Override start time to test formatting
	c.startTime = time.Now().Add(-2*time.Hour - 14*time.Minute)

	uptime := c.FormatUptime()
	if uptime != "2h 14m" {
		t.Errorf("expected '2h 14m', got '%s'", uptime)
	}
}

func TestBuildSnapshot(t *testing.T) {
	c := NewCollector(5)
	c.AddEvent(Event{Provider: "anthropic", Action: "compacted", InputK: 120, OutputK: 30, SavedPct: 75})

	snap := c.BuildSnapshot(142, 8, 847203, 3)

	if snap.Requests != 142 {
		t.Errorf("requests = %d, want 142", snap.Requests)
	}
	if snap.Compactions != 8 {
		t.Errorf("compactions = %d, want 8", snap.Compactions)
	}
	if snap.Sessions != 3 {
		t.Errorf("sessions = %d, want 3", snap.Sessions)
	}
	if len(snap.RecentEvents) != 1 {
		t.Fatalf("events = %d, want 1", len(snap.RecentEvents))
	}
}

func TestSnapshotJSON(t *testing.T) {
	snap := Snapshot{
		Uptime:      "2h 14m",
		Requests:    142,
		Compactions: 8,
		TokensSaved: 847203,
	}

	data := snap.ToJSON()
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["uptime"] != "2h 14m" {
		t.Errorf("uptime = %v", parsed["uptime"])
	}
}
