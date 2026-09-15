package router

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/blob"
	"server/internal/database"
	"server/internal/identity"
	"server/internal/storage"
)

type testServer struct {
	handler   http.Handler
	auth      *auth.Service
	identity  *identity.Service
	store     storage.Storage
	auditPath string
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerFull(t, auth.Config{
		AllowSignup: true,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	}, access.Default(), identity.Config{GitHub: nil})
}

// newTestServerWithRules runs the router on custom packages-access rules.
func newTestServerWithRules(t *testing.T, rules []access.Rule) *testServer {
	t.Helper()
	return newTestServerFull(t, auth.Config{
		AllowSignup: true,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	}, rules, identity.Config{GitHub: nil})
}

// newTestServerFull wires the router with custom auth and identity configs;
// identity.Config{GitHub: nil} disables OAuth entirely.
func newTestServerFull(t *testing.T, cfg auth.Config, rules []access.Rule, idCfg identity.Config) *testServer {
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
	idSvc := identity.New(db.DB, idCfg, log)

	auditor, err := audit.New(filepath.Join(dir, "audit.jsonl"), log)
	if err != nil {
		t.Fatalf("audit init: %v", err)
	}
	t.Cleanup(func() { auditor.Close() })

	store := storage.New(blob.NewLocal(filepath.Join(dir, "packages")))

	return &testServer{
		handler:   New(log, store, svc, idSvc, auditor, rules),
		auth:      svc,
		identity:  idSvc,
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
