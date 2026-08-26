package router

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestWebSidebarAndReadme(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	const readme = "# DebuggerDump\n\nExport profiling data."
	rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("tar"), func(body map[string]any) {
			versionOf(t, body)["readme"] = readme
		}), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish: status %d, body %s", rec.Code, rec.Body.String())
	}

	// readme of the latest version as plain text; the website client
	// percent-encodes the id (encodeURIComponent)
	rd := doReq(t, ts.handler, http.MethodGet,
		"/-/verdaccio/data/package/readme/"+url.PathEscape(publishPkg), nil, nil)
	if rd.Code != http.StatusOK {
		t.Fatalf("readme status = %d", rd.Code)
	}
	if got := rd.Body.String(); got != readme {
		t.Errorf("readme = %q, want %q", got, readme)
	}
	if ct := rd.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("readme content-type = %q", ct)
	}

	// sidebar carries the latest version and the manifest's collections
	sb := doReq(t, ts.handler, http.MethodGet,
		"/-/verdaccio/data/sidebar/"+url.PathEscape(publishPkg), nil, nil)
	if sb.Code != http.StatusOK {
		t.Fatalf("sidebar status = %d", sb.Code)
	}
	var side struct {
		ID       string            `json:"_id"`
		Latest   map[string]any    `json:"latest"`
		Versions map[string]any    `json:"versions"`
		DistTags map[string]string `json:"dist-tags"`
		Time     map[string]any    `json:"time"`
	}
	if err := json.Unmarshal(sb.Body.Bytes(), &side); err != nil {
		t.Fatalf("unmarshal sidebar: %v", err)
	}
	if side.ID != publishPkg {
		t.Errorf("_id = %q, want %q", side.ID, publishPkg)
	}
	if _, ok := side.Versions["1.0.0"]; !ok {
		t.Errorf("versions missing 1.0.0: %v", side.Versions)
	}
	if side.Latest["version"] != "1.0.0" || side.Latest["gm"] == nil {
		t.Errorf("latest wrong: %v", side.Latest)
	}
	if side.DistTags["latest"] != "1.0.0" {
		t.Errorf("dist-tags = %v", side.DistTags)
	}
	if side.Time["created"] == nil {
		t.Errorf("time incomplete: %v", side.Time)
	}

	// unknown package is a clean 404 for both calls, in either spelling
	for _, path := range []string{
		"/-/verdaccio/data/package/readme/@acme/missing",
		"/-/verdaccio/data/sidebar/@acme/missing",
		"/-/verdaccio/data/package/readme/%40acme%2fmissing",
		"/-/verdaccio/data/sidebar/%40acme%2fmissing",
	} {
		if got := doReq(t, ts.handler, http.MethodGet, path, nil, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, got.Code)
		}
	}
}
