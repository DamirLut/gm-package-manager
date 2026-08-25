package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"server/internal/access"
)

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
	ts := newTestServer(t)
	h, store := ts.handler, ts.store

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
	token := loginToken(t, ts, "alice")

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.Header.Set("Authorization", "Bearer "+token)
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
	ts := newTestServer(t)
	h, store := ts.handler, ts.store

	content := "fake tarball payload"
	if _, err := store.PutTarball(t.Context(), "@scope/foo", "foo-1.0.0.tgz", strings.NewReader(content)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	token := loginToken(t, ts, "alice")

	tests := []string{
		"/@scope/foo/-/foo-1.0.0.tgz",
		"/@scope%2Ffoo/-/foo-1.0.0.tgz",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("Authorization", "Bearer "+token)
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

func TestManifestETag(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")
	if rec := doPublish(ts, token, "/@acme/lib",
		publishPayload(t, "@acme/lib", "1.0.0", []byte("tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d", rec.Code)
	}
	headers := map[string]string{"Authorization": "Bearer " + token}

	rec := doReq(t, ts.handler, http.MethodGet, "/@acme/lib", headers, nil)
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag header")
	}

	for _, match := range []string{etag, "W/" + etag} {
		h := map[string]string{"Authorization": headers["Authorization"], "If-None-Match": match}
		rec := doReq(t, ts.handler, http.MethodGet, "/@acme/lib", h, nil)
		if rec.Code != http.StatusNotModified {
			t.Errorf("If-None-Match %q: status = %d, want 304", match, rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("If-None-Match %q: body = %q, want empty", match, rec.Body.String())
		}
	}

	// changed content must produce a different ETag
	if rec := doPublish(ts, token, "/@acme/lib",
		publishPayload(t, "@acme/lib", "1.1.0", []byte("tar2"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("second publish: status %d", rec.Code)
	}
	rec = doReq(t, ts.handler, http.MethodGet, "/@acme/lib", headers, nil)
	if again := rec.Header().Get("ETag"); again == etag {
		t.Error("ETag unchanged after publish")
	}
}

func TestAbbreviatedManifest(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	payload := publishPayload(t, publishPkg, "1.0.0", []byte("tar"), func(b map[string]any) {
		v := versionOf(t, b)
		v["dependencies"] = map[string]string{"left-pad": "^1.3.0"}
		v["readme"] = "long readme text"
	})
	if rec := doPublish(ts, token, "/"+publishPkg, payload, ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d, body %s", rec.Code, rec.Body.String())
	}

	full := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	var fullDoc struct {
		Versions map[string]map[string]json.RawMessage `json:"versions"`
	}
	json.Unmarshal(full.Body.Bytes(), &fullDoc)
	if _, ok := fullDoc.Versions["1.0.0"]["gm"]; !ok {
		t.Fatal("full manifest lost gm")
	}
	if _, ok := fullDoc.Versions["1.0.0"]["readme"]; !ok {
		t.Fatal("full manifest lost readme")
	}

	rec := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg, map[string]string{
		"Authorization": "Bearer " + token,
		"Accept":        "application/vnd.npm.install-v1+json",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("abbreviated status = %d, body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != abbrevType {
		t.Errorf("content-type = %q, want %q", ct, abbrevType)
	}

	var abbr struct {
		Name     string                     `json:"name"`
		DistTags map[string]string          `json:"dist-tags"`
		Modified string                     `json:"modified"`
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &abbr); err != nil {
		t.Fatalf("unmarshal abbreviated: %v", err)
	}
	if abbr.Name != publishPkg || abbr.DistTags["latest"] != "1.0.0" || abbr.Modified == "" {
		t.Errorf("abbreviated top level wrong: %+v", abbr)
	}

	var ver map[string]any
	if err := json.Unmarshal(abbr.Versions["1.0.0"], &ver); err != nil {
		t.Fatalf("unmarshal version entry: %v", err)
	}
	if _, ok := ver["dependencies"]; !ok {
		t.Error("abbreviated version lost dependencies")
	}
	if _, ok := ver["gm"]; !ok {
		t.Error("abbreviated version lost gm")
	}
	dist, ok := ver["dist"].(map[string]any)
	if !ok || dist["tarball"] == "" {
		t.Error("abbreviated version lost dist.tarball")
	}
	for _, gone := range []string{"readme", "description", "author"} {
		if _, ok := ver[gone]; ok {
			t.Errorf("abbreviated version should drop %s", gone)
		}
	}
}

func TestReadScopeGuard(t *testing.T) {
	ts := newTestServer(t) // default rules: reads are public
	token := loginToken(t, ts, "alice")
	if rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d", rec.Code)
	}

	targets := []string{
		"/" + publishPkg,
		"/@acme/lib/-/lib-1.0.0.tgz",
	}
	for _, target := range targets {
		rec := doReq(t, ts.handler, http.MethodGet, target, nil, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s anonymous on public registry: status = %d, want 200", target, rec.Code)
		}
		rec = doReq(t, ts.handler, http.MethodGet, target,
			map[string]string{"Authorization": "Bearer " + token}, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s with token: status = %d, want 200", target, rec.Code)
		}
	}
}

// Private patterns deny reads even when a later catch-all allows them.
func TestReadScopeGuardPrivateRules(t *testing.T) {
	authd := []string{access.GroupAuthenticated}
	ts := newTestServerWithRules(t, []access.Rule{
		{Pattern: "@secret/**", Access: authd, Publish: authd},
		{Pattern: "@johns/**", Access: []string{"alice"}, Publish: authd},
		{Pattern: "@demo/**", Access: []string{access.GroupAnonymous}, Publish: authd},
		{Pattern: "**", Access: []string{access.GroupAll}, Publish: authd},
	})

	alice := loginToken(t, ts, "alice")
	for _, pkg := range []string{"@secret/lib", "@johns/lib", "@demo/lib"} {
		rec := doPublish(ts, alice, "/"+pkg,
			publishPayload(t, pkg, "1.0.0", []byte("tar "+pkg), nil), "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("publish %s: status %d, body %s", pkg, rec.Code, rec.Body.String())
		}
	}

	bob := loginToken(t, ts, "bob")

	tests := []struct {
		target string
		token  string // "" = anonymous
		want   int
	}{
		{"/@secret/lib", "", http.StatusUnauthorized},
		{"/@secret/lib/-/lib-1.0.0.tgz", "", http.StatusUnauthorized},
		{"/@secret/lib", alice, http.StatusOK},
		{"/@secret/lib/-/lib-1.0.0.tgz", alice, http.StatusOK},
		{"/@johns/lib", alice, http.StatusOK}, // named-user match
		{"/@johns/lib", bob, http.StatusForbidden},
		{"/@demo/lib", alice, http.StatusForbidden}, // $anonymous excludes authenticated
		{"/@demo/lib", "", http.StatusOK},
	}
	for _, tt := range tests {
		name := tt.target + "/" + map[bool]string{true: "anon", false: "authed"}[tt.token == ""]
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.token != "" {
				headers["Authorization"] = "Bearer " + tt.token
			}
			rec := doReq(t, ts.handler, http.MethodGet, tt.target, headers, nil)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusUnauthorized {
				if wa := rec.Header().Get("WWW-Authenticate"); wa != "Bearer" {
					t.Errorf("www-authenticate = %q, want Bearer", wa)
				}
			}
		})
	}

	data, err := os.ReadFile(ts.auditPath)
	if err != nil || !strings.Contains(string(data), `"action":"package.access.denied"`) {
		t.Errorf("audit log missing package.access.denied: %v\n%s", err, data)
	}
}

func TestAuditStub(t *testing.T) {
	h := newTestServer(t).handler

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
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

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
			rec := doReq(t, ts.handler, http.MethodGet, target,
				map[string]string{"Authorization": "Bearer " + token}, nil)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}
