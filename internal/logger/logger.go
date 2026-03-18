package logger

import (
	"encoding/json"
	"io"
	"time"
)

type Entry struct {
	Timestamp    string `json:"timestamp"`
	Provider     string `json:"provider"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Action       string `json:"action"`
	LatencyMs    int64  `json:"latency_ms,omitempty"`
	Error        string `json:"error,omitempty"`
}

type Logger struct {
	w io.Writer
}

func New(w io.Writer) *Logger {
	return &Logger{w: w}
}

func (l *Logger) LogRequest(e Entry) {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, _ := json.Marshal(e)
	data = append(data, '\n')
	l.w.Write(data)
}
