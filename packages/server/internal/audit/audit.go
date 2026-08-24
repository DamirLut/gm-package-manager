package audit

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	ActionLoginSuccess        = "auth.login.success"
	ActionLoginFailed         = "auth.login.failed"
	ActionTokenRevoked        = "token.revoked"
	ActionPackagePublish      = "package.publish"
	ActionPackageUnpublish    = "package.unpublish"
	ActionPackageAccessDenied = "package.access.denied"
)

// Event is one audit record. Only identifiers (username, token prefix) may
// appear here — never raw tokens, passwords or Authorization headers.
type Event struct {
	TS          time.Time         `json:"ts"`
	Action      string            `json:"action"`
	Actor       string            `json:"actor"`
	TokenPrefix string            `json:"token_prefix,omitempty"`
	IP          string            `json:"ip,omitempty"`
	UA          string            `json:"ua,omitempty"`
	Package     string            `json:"package,omitempty"`
	Version     string            `json:"version,omitempty"`
	Success     bool              `json:"success"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Logger appends one JSON object per line to <storage>/audit.jsonl and
// mirrors events into the structured log.
type Logger struct {
	mu  sync.Mutex
	f   *os.File
	log *slog.Logger
}

func New(path string, log *slog.Logger) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("audit: create directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %q: %w", path, err)
	}
	return &Logger{f: f, log: log}, nil
}

func (l *Logger) Record(e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		l.log.Error("audit marshal failed", "err", err)
		return
	}
	line = append(line, '\n')

	l.mu.Lock()
	_, werr := l.f.Write(line)
	l.mu.Unlock()
	if werr != nil {
		l.log.Error("audit write failed", "err", werr)
	}

	l.log.Info("audit",
		"action", e.Action,
		"actor", e.Actor,
		"token_prefix", e.TokenPrefix,
		"ip", e.IP,
		"success", e.Success,
	)
}

func (l *Logger) Close() error { return l.f.Close() }
