package router

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"server/internal/storage"
)

func newTestServer(t *testing.T) (http.Handler, storage.Storage) {
	t.Helper()
	store := storage.NewLocal(t.TempDir())
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), store), store
}

func TestSplitPkg(t *testing.T) {
	tests := []struct {
		path     string
		wantName string
		wantRest string
	}{
		{"/@scope/foo", "@scope/foo", ""},
		{"/@scope/foo/-/foo-1.0.0.tgz", "@scope/foo", "-/foo-1.0.0.tgz"},
		{"/foo", "foo", ""},
		{"/foo/-/foo-1.0.0.tgz", "foo", "-/foo-1.0.0.tgz"},
		{"/", "", ""},
		{"", "", ""},
		{"/@scope", "", ""},
		{"/@scope/", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name, rest := splitPkg(tt.path)
			if name != tt.wantName || rest != tt.wantRest {
				t.Fatalf("splitPkg(%q) = (%q, %q), want (%q, %q)",
					tt.path, name, rest, tt.wantName, tt.wantRest)
			}
		})
	}
}

// Verifies that all name spelling variants reach the handler
// and are parsed identically through the real router.
func TestPkgDocSpellings(t *testing.T) {
	h, store := newTestServer(t)

	for _, name := range []string{"@scope/foo", "foo"} {
		err := store.PutManifest(t.Context(), name, []byte(`{"name":"`+name+`"}`))
		if err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	tests := []struct {
		target   string
		wantName string
	}{
		{"/@scope/foo", "@scope/foo"},
		{"/@scope%2Ffoo", "@scope/foo"},
		{"/%40scope%2Ffoo", "@scope/foo"},
		{"/foo", "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type = %q, want application/json", ct)
			}
			var got struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal body %q: %v", rec.Body.String(), err)
			}
			if got.Name != tt.wantName {
				t.Fatalf("name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestTarballDownload(t *testing.T) {
	h, store := newTestServer(t)

	content := "fake tarball payload"
	if _, err := store.PutTarball(t.Context(), "@scope/foo", "foo-1.0.0.tgz", strings.NewReader(content)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tests := []string{
		"/@scope/foo/-/foo-1.0.0.tgz",
		"/@scope%2Ffoo/-/foo-1.0.0.tgz",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
				t.Fatalf("content-type = %q, want application/octet-stream", ct)
			}
			if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(content)) {
				t.Fatalf("content-length = %q, want %d", cl, len(content))
			}
			if rec.Body.String() != content {
				t.Fatalf("body = %q, want %q", rec.Body.String(), content)
			}
		})
	}
}

func TestAuditStub(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/-/npm/v1/security/advisories/bulk", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "{}" {
		t.Fatalf("body = %q, want {}", body)
	}
}

func TestPkgNotFound(t *testing.T) {
	h, _ := newTestServer(t)

	tests := []string{
		"/",
		"/@scope",
		"/@scope/foo/extra/deep",
		"/missing",                 // no manifest
		"/missing/-/m-1.0.0.tgz",   // no tarball
		"/@scope/foo/-/absent.tgz", // pkg absent at all
		"/../escape",               // traversal never matches a package
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}
