package router

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"server/internal/audit"
	"server/internal/auth"
	"server/internal/database"
	"server/internal/storage"
)

type testServer struct {
	handler   http.Handler
	auth      *auth.Service
	store     storage.Storage
	auditPath string
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerWithAuth(t, auth.Config{
		AllowSignup: true,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	})
}

func newTestServerWithAuth(t *testing.T, cfg auth.Config) *testServer {
	t.Helper()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := database.Open(t.Context(), database.Config{
		Path:        filepath.Join(dir, "meta.db"),
		AutoMigrate: true,
	}, log)
	if err != nil {
		t.Fatalf("database open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	svc := auth.New(db.DB, cfg, log)

	auditor, err := audit.New(filepath.Join(dir, "audit.jsonl"), log)
	if err != nil {
		t.Fatalf("audit init: %v", err)
	}
	t.Cleanup(func() { auditor.Close() })

	store := storage.NewLocal(filepath.Join(dir, "packages"))

	return &testServer{
		handler:   New(log, store, svc, auditor),
		auth:      svc,
		store:     store,
		auditPath: filepath.Join(dir, "audit.jsonl"),
	}
}

func doReq(t *testing.T, h http.Handler, method, target string, headers map[string]string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
