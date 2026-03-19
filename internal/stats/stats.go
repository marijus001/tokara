package stats

import (
	"encoding/json"
	"sync"
	"time"
)

// Snapshot holds a point-in-time view of proxy statistics.
type Snapshot struct {
	Uptime       string  `json:"uptime"`
	UptimeSecs   int64   `json:"uptime_secs"`
	Requests     int64   `json:"requests"`
	Compactions  int64   `json:"compactions"`
	TokensSaved  int64   `json:"tokens_saved"`
	Sessions     int     `json:"sessions"`
	RecentEvents []Event `json:"recent_events"`
}

// Event represents a single proxy event for the TUI feed.
type Event struct {
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Action    string `json:"action"` // "pass-through", "compacted", "precomputing"
	InputK    int    `json:"input_k"`
	OutputK   int    `json:"output_k"`
	SavedPct  int    `json:"saved_pct"`
}

// Collector accumulates stats and events from the proxy.
type Collector struct {
	startTime time.Time
	mu        sync.Mutex
	events    []Event
	maxEvents int
}

// NewCollector creates a stats collector.
func NewCollector(maxEvents int) *Collector {
	return &Collector{
		startTime: time.Now(),
		maxEvents: maxEvents,
	}
}

// AddEvent records a proxy event.
func (c *Collector) AddEvent(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.Timestamp == "" {
		e.Timestamp = time.Now().Format("15:04")
	}
	c.events = append(c.events, e)
	if len(c.events) > c.maxEvents {
		c.events = c.events[len(c.events)-c.maxEvents:]
	}
}

// RecentEvents returns the latest events.
func (c *Collector) RecentEvents(n int) []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n > len(c.events) {
		n = len(c.events)
	}
	// Return most recent n, newest first
	result := make([]Event, n)
	for i := 0; i < n; i++ {
		result[i] = c.events[len(c.events)-1-i]
	}
	return result
}

// Uptime returns the duration since the collector was created.
func (c *Collector) Uptime() time.Duration {
	return time.Since(c.startTime)
}

// FormatUptime returns a human-readable uptime string.
func (c *Collector) FormatUptime() string {
	d := c.Uptime()
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours > 0 {
		return formatDuration(hours, "h", mins, "m")
	}
	secs := int(d.Seconds()) % 60
	if mins > 0 {
		return formatDuration(mins, "m", secs, "s")
	}
	return formatDuration(secs, "s", 0, "")
}

func formatDuration(a int, aUnit string, b int, bUnit string) string {
	if bUnit == "" || b == 0 {
		return intToStr(a) + aUnit
	}
	return intToStr(a) + aUnit + " " + intToStr(b) + bUnit
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	result := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		result = append(result, byte('0'+n%10))
		n /= 10
	}
	if neg {
		result = append(result, '-')
	}
	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// BuildSnapshot creates a stats snapshot for the TUI.
func (c *Collector) BuildSnapshot(requests, compactions, tokensSaved int64, sessions int) Snapshot {
	return Snapshot{
		Uptime:       c.FormatUptime(),
		UptimeSecs:   int64(c.Uptime().Seconds()),
		Requests:     requests,
		Compactions:  compactions,
		TokensSaved:  tokensSaved,
		Sessions:     sessions,
		RecentEvents: c.RecentEvents(c.maxEvents),
	}
}

// ToJSON serializes a snapshot to JSON bytes.
func (s Snapshot) ToJSON() []byte {
	data, _ := json.Marshal(s)
	return data
}
