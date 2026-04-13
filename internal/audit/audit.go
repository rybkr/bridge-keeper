package audit

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Severity is a stable audit severity label.
type Severity int

const (
	Debug Severity = iota
	Info
	Warning
	Error
)

func (s Severity) String() string {
	return [...]string{"DEBUG", "INFO", "WARN", "ERROR"}[s]
}

// Event is a structured audit record written as JSONL.
type Event struct {
	Time     string         `json:"time"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// Logger writes structured audit events to an injected writer.
type Logger struct {
	mu       sync.Mutex
	out      io.Writer
	minLevel Severity
}

// NewLogger creates a structured logger. A nil writer produces a no-op logger.
func NewLogger(out io.Writer, minLevel Severity) *Logger {
	return &Logger{
		out:      out,
		minLevel: minLevel,
	}
}

// Log emits a JSONL audit event. Errors are intentionally ignored because audit
// failures must not crash the runtime.
func (l *Logger) Log(severity Severity, message string, fields map[string]any) {
	if l == nil || l.out == nil || severity < l.minLevel {
		return
	}

	event := Event{
		Time:     time.Now().UTC().Format(time.RFC3339Nano),
		Severity: severity.String(),
		Message:  message,
		Fields:   fields,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(data, '\n'))
}
