package token

import "testing"

func TestEstimate(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello world", 3},
		{string(make([]byte, 4000)), 1000},
	}
	for _, tt := range tests {
		got := Estimate(tt.input)
		low := int(float64(tt.expected) * 0.8)
		high := int(float64(tt.expected)*1.2) + 1
		if got < low || got > high {
			t.Errorf("Estimate(%d chars) = %d, want ~%d", len(tt.input), got, tt.expected)
		}
	}
}

func TestEstimateMessages(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: string(make([]byte, 400))},
		{Role: "user", Content: string(make([]byte, 2000))},
		{Role: "assistant", Content: string(make([]byte, 800))},
	}
	got := EstimateMessages(messages)
	if got < 700 || got > 900 {
		t.Errorf("EstimateMessages = %d, want ~812", got)
	}
}
