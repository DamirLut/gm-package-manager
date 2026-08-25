package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func publishAndRev(t *testing.T, ts *testServer, token, version string) string {
	t.Helper()
	rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, version, []byte("tar "+version), nil), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish %s: status %d, body %s", version, rec.Code, rec.Body.String())
	}
	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	var doc struct {
		Rev string `json:"_rev"`
	}
	json.Unmarshal(man.Body.Bytes(), &doc)
	if doc.Rev == "" {
		t.Fatal("manifest has no _rev")
	}
	return doc.Rev
}

func TestUnpublish(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	rev := publishAndRev(t, ts, token, "1.0.0")
	if !strings.HasPrefix(rev, "1-") {
		t.Errorf("first revision = %q, want prefix 1-", rev)
	}
	// a second publish bumps the revision counter
	rev = publishAndRev(t, ts, token, "1.1.0")
	if !strings.HasPrefix(rev, "2-") {
		t.Errorf("second revision = %q, want prefix 2-", rev)
	}

	req := httptest.NewRequest(http.MethodDelete, "/"+publishPkg+"/-rev/"+rev, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unpublish: status %d, body %s", rec.Code, rec.Body.String())
	}

	headers := map[string]string{"Authorization": "Bearer " + token}
	for _, target := range []string{"/" + publishPkg, "/@acme/lib/-/lib-1.1.0.tgz"} {
		if got := doReq(t, ts.handler, http.MethodGet, target, headers, nil); got.Code != http.StatusNotFound {
			t.Errorf("GET %s after unpublish: status %d, want 404", target, got.Code)
		}
	}

	data, err := os.ReadFile(ts.auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), `"action":"package.unpublish"`) {
		t.Errorf("audit log missing package.unpublish:\n%s", data)
	}
}

func TestUnpublishStaleRevision(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	publishAndRev(t, ts, token, "1.0.0")
	rev := publishAndRev(t, ts, token, "1.1.0")

	req := httptest.NewRequest(http.MethodDelete, "/"+publishPkg+"/-rev/1-stale", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("stale rev: status = %d, want 409", rec.Code)
	}

	_ = rev // the document is still there under its current revision
	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	if man.Code != http.StatusOK {
		t.Errorf("manifest gone after rejected unpublish: status %d", man.Code)
	}
}

func TestUnpublishDenied(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/"+publishPkg+"/-rev/1-x", nil)
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: status = %d, want 401", rec.Code)
	}

	// missing package is 404 for an authorized user
	token := loginToken(t, ts, "alice")
	req = httptest.NewRequest(http.MethodDelete, "/@acme/gone/-rev/"+strings.Repeat("a", 8), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing package: status = %d, want 404", rec.Code)
	}
}
