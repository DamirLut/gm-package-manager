package audit

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestLogger(t *testing.T) (string, *Logger) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "audit.jsonl")
	l, err := New(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return path, l
}

func readLines(t *testing.T, path string) []Event {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []Event
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}

func TestRecordWritesJSONL(t *testing.T) {
	path, l := newTestLogger(t)

	l.Record(Event{Action: ActionLoginSuccess, Actor: "alice", Success: true})
	l.Record(Event{
		Action:      ActionLoginFailed,
		Actor:       "alice",
		TokenPrefix: "gmpm_abcd",
		IP:          "127.0.0.1",
		UA:          "npm/10",
		Package:     "@scope/foo",
		Version:     "1.0.0",
		Metadata:    map[string]string{"reason": "bad password"},
	})

	events := readLines(t, path)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Action != ActionLoginSuccess || !events[0].Success || events[0].TS.IsZero() {
		t.Errorf("first event wrong: %+v", events[0])
	}
	if events[1].TokenPrefix != "gmpm_abcd" || events[1].Package != "@scope/foo" ||
		events[1].Version != "1.0.0" || events[1].Metadata["reason"] != "bad password" {
		t.Errorf("second event fields lost: %+v", events[1])
	}
}

// Appends from concurrent goroutines must never interleave mid-line:
// every written line stays a single parseable JSON object.
func TestRecordConcurrent(t *testing.T) {
	path, l := newTestLogger(t)

	const workers, perWorker = 16, 25
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range perWorker {
				l.Record(Event{Action: ActionPackageAccessDenied, Actor: "u", Metadata: map[string]string{"n": string(rune('a' + j))}})
			}
		}()
	}
	wg.Wait()

	if events := readLines(t, path); len(events) != workers*perWorker {
		t.Fatalf("got %d lines, want %d — writes were interleaved or lost",
			len(events), workers*perWorker)
	}
}

// Appends survive reopen: O_APPEND continues the existing file.
func TestRecordAppendsAcrossReopens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	l1, err := New(path, log)
	if err != nil {
		t.Fatal(err)
	}
	l1.Record(Event{Action: ActionLoginSuccess, Actor: "a", Success: true})
	l1.Close()

	l2, err := New(path, log)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	l2.Record(Event{Action: ActionLoginFailed, Actor: "b"})

	if events := readLines(t, path); len(events) != 2 || events[0].Actor != "a" || events[1].Actor != "b" {
		t.Fatalf("events = %+v", events)
	}
}
