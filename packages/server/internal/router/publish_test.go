package router

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

const publishPkg = "@acme/lib"

func loginToken(t *testing.T, ts *testServer, user string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/-/user/org.couchdb.user:"+user, nil)
	req.SetBasicAuth(user, "pw")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("login %s: status %d, body %s", user, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login %s: no token in response (%v)", user, err)
	}
	return out.Token
}

// publishPayload builds a valid document for name@version; tweak breaks
// specific fields for negative tests.
func publishPayload(t *testing.T, name, version string, tarball []byte, tweak func(body map[string]any)) io.Reader {
	t.Helper()
	sum := sha1.Sum(tarball)
	base := strings.TrimPrefix(name, strings.SplitN(name, "/", 2)[0]+"/")
	body := map[string]any{
		"_id":       name,
		"name":      name,
		"dist-tags": map[string]string{"latest": version},
		"versions": map[string]any{
			version: map[string]any{
				"name":    name,
				"version": version,
				"gm": map[string]any{
					"destination": "scripts/" + version,
					"displayName": "Lib " + version,
				},
				"dist": map[string]any{"shasum": hex.EncodeToString(sum[:])},
			},
		},
		"_attachments": map[string]any{
			fmt.Sprintf("%s-%s.tgz", base, version): map[string]any{
				"data":         base64.StdEncoding.EncodeToString(tarball),
				"length":       len(tarball),
				"content_type": "application/octet-stream",
			},
		},
	}
	if tweak != nil {
		tweak(body)
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return bytes.NewReader(data)
}

func versionOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	versions, ok := body["versions"].(map[string]any)
	if !ok || len(versions) != 1 {
		t.Fatal("payload has no single versions entry")
	}
	for _, v := range versions {
		obj, ok := v.(map[string]any)
		if !ok {
			t.Fatal("version is not an object")
		}
		return obj
	}
	return nil
}

func doPublish(ts *testServer, token, target string, payload io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, target, payload)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec
}

func TestPublishHappyPath(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	tar := []byte("fake tarball bytes for 1.0.0")
	rec := doPublish(ts, token, "/"+publishPkg, publishPayload(t, publishPkg, "1.0.0", tar, nil), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	want := `{"success":true,"ok":"created new package"}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}

	var doc struct {
		Versions map[string]struct {
			GM map[string]string `json:"gm"`
			Di struct {
				Tarball string `json:"tarball"`
				Shasum  string `json:"shasum"`
			} `json:"dist"`
		} `json:"versions"`
		Tags     map[string]string `json:"dist-tags"`
		Time     map[string]string `json:"time"`
		Attaches map[string]struct {
			Shasum string `json:"shasum"`
			Length int    `json:"length"`
		} `json:"_attachments"`
		Maintainers []map[string]string `json:"maintainers"`
	}
	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	if man.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, body %s", man.Code, man.Body.String())
	}
	if err := json.Unmarshal(man.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	v1 := doc.Versions["1.0.0"]
	if want := "http://example.com/@acme/lib/-/lib-1.0.0.tgz"; v1.Di.Tarball != want {
		t.Errorf("dist.tarball = %q, want %q", v1.Di.Tarball, want)
	}
	if v1.GM["destination"] != "scripts/1.0.0" || v1.GM["displayName"] != "Lib 1.0.0" {
		t.Errorf("gm not preserved: %+v", v1.GM)
	}
	if doc.Tags["latest"] != "1.0.0" {
		t.Errorf("dist-tags.latest = %q, want 1.0.0", doc.Tags["latest"])
	}
	if doc.Time["created"] == "" || doc.Time["modified"] == "" || doc.Time["1.0.0"] == "" {
		t.Errorf("time incomplete: %+v", doc.Time)
	}
	sum := sha1.Sum(tar)
	at := doc.Attaches["lib-1.0.0.tgz"]
	if at.Shasum != hex.EncodeToString(sum[:]) || at.Length != len(tar) {
		t.Errorf("_attachments wrong: %+v", at)
	}
	if len(doc.Maintainers) != 1 || doc.Maintainers[0]["name"] != "alice" {
		t.Errorf("maintainers = %+v", doc.Maintainers)
	}

	// the tarball is downloadable and byte-identical
	dl := doReq(t, ts.handler, http.MethodGet, "/@acme/lib/-/lib-1.0.0.tgz",
		map[string]string{"Authorization": "Bearer " + token}, nil)
	if dl.Code != http.StatusOK || !bytes.Equal(dl.Body.Bytes(), tar) {
		t.Fatalf("tarball download: status %d, %d bytes", dl.Code, dl.Body.Len())
	}
}

// npm sends the scoped package name inside the attachment key
// ("@scope/name-x.y.z.tgz"); it must be stored under the base name.
func TestPublishNpmScopedAttachment(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	tar := []byte("scoped tarball bytes")
	rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", tar, func(body map[string]any) {
			body["_attachments"] = map[string]any{
				"@acme/lib-1.0.0.tgz": map[string]any{
					"data":         base64.StdEncoding.EncodeToString(tar),
					"length":       len(tar),
					"content_type": "application/octet-stream",
				},
			}
		}), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var doc struct {
		Versions map[string]struct {
			Dist struct {
				Tarball string `json:"tarball"`
			} `json:"dist"`
		} `json:"versions"`
		Attaches map[string]any `json:"_attachments"`
	}
	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg, nil, nil)
	if man.Code != http.StatusOK {
		t.Fatalf("manifest status = %d", man.Code)
	}
	if err := json.Unmarshal(man.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	wantURL := "http://example.com/@acme/lib/-/lib-1.0.0.tgz"
	if got := doc.Versions["1.0.0"].Dist.Tarball; got != wantURL {
		t.Errorf("dist.tarball = %q, want %q", got, wantURL)
	}
	if _, ok := doc.Attaches["lib-1.0.0.tgz"]; !ok {
		t.Errorf("_attachments keys = %v, want lib-1.0.0.tgz", doc.Attaches)
	}

	dl := doReq(t, ts.handler, http.MethodGet, "/@acme/lib/-/lib-1.0.0.tgz", nil, nil)
	if dl.Code != http.StatusOK || !bytes.Equal(dl.Body.Bytes(), tar) {
		t.Fatalf("tarball download: status %d, %d bytes", dl.Code, dl.Body.Len())
	}
}

func TestPublishSecondVersion(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	if rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("v1 tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("first publish: status %d, body %s", rec.Code, rec.Body.String())
	}
	rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.1.0", []byte("v2 tar"), nil), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("second publish: status %d, body %s", rec.Code, rec.Body.String())
	}
	if want := `"ok":"package changed"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %q, want contains %q", rec.Body.String(), want)
	}

	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	var doc struct {
		Versions map[string]json.RawMessage `json:"versions"`
		Tags     map[string]string          `json:"dist-tags"`
	}
	json.Unmarshal(man.Body.Bytes(), &doc)
	if len(doc.Versions) != 2 {
		t.Errorf("versions = %d, want 2", len(doc.Versions))
	}
	if doc.Tags["latest"] != "1.1.0" {
		t.Errorf("dist-tags.latest = %q, want 1.1.0", doc.Tags["latest"])
	}
}

func TestPublishAnonymousForbidden(t *testing.T) {
	ts := newTestServer(t)

	rec := doPublish(ts, "", "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("tar"), nil), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}

	data, err := os.ReadFile(ts.auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), `"action":"package.access.denied"`) {
		t.Errorf("audit log missing package.access.denied:\n%s", data)
	}
}

func TestPublishValidationChain(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	tests := []struct {
		name        string
		target      string
		contentType string
		payload     func() io.Reader
		want        int
	}{
		{
			name:   "sub-path is not publish",
			target: "/" + publishPkg + "/-rev/12",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), nil)
			},
			want: http.StatusNotFound,
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), nil)
			},
			want: http.StatusUnsupportedMediaType,
		},
		{
			name:    "broken json",
			payload: func() io.Reader { return strings.NewReader("{oops") },
			want:    http.StatusBadRequest,
		},
		{
			name:   "unscoped name",
			target: "/plain",
			payload: func() io.Reader {
				return publishPayload(t, "plain", "1.0.0", []byte("t"), nil)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "name mismatch with url",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					b["name"] = "@acme/other"
				})
			},
			want: http.StatusBadRequest,
		},
		{
			name: "two versions",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					extra, _ := b["versions"].(map[string]any)["1.0.0"].(map[string]any)
					b["versions"].(map[string]any)["2.0.0"] = extra
				})
			},
			want: http.StatusBadRequest,
		},
		{
			name: "no attachments",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					delete(b, "_attachments")
				})
			},
			want: http.StatusBadRequest,
		},
		{
			name: "bad semver",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "not.semver", []byte("t"), nil)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "inner version differs from key",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["version"] = "9.9.9"
				})
			},
			want: http.StatusBadRequest,
		},
		{
			name: "missing gm block",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					delete(versionOf(t, b), "gm")
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "flat gm key instead of object",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["gm"] = "scripts"
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "empty destination",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["gm"].(map[string]any)["destination"] = ""
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "destination escapes upward",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["gm"].(map[string]any)["destination"] = "../escape"
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "absolute destination",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["gm"].(map[string]any)["destination"] = "/etc"
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "empty display name",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["gm"].(map[string]any)["displayName"] = ""
				})
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "zero-length tarball",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", nil, nil)
			},
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "shasum mismatch",
			payload: func() io.Reader {
				return publishPayload(t, publishPkg, "1.0.0", []byte("t"), func(b map[string]any) {
					versionOf(t, b)["dist"].(map[string]any)["shasum"] = strings.Repeat("0", 40)
				})
			},
			want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target
			if target == "" {
				target = "/" + publishPkg
			}
			ct := tt.contentType
			if ct == "" {
				ct = "application/json"
			}
			rec := doPublish(ts, token, target, tt.payload(), ct)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestPublishDuplicateVersionConflict(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	first := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("tar"), nil), "")
	if first.Code != http.StatusCreated {
		t.Fatalf("setup: status %d, body %s", first.Code, first.Body.String())
	}

	rec := doPublish(ts, token, "/"+publishPkg,
		publishPayload(t, publishPkg, "1.0.0", []byte("other tar"), nil), "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if want := "this package is already present"; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %q, want contains %q", rec.Body.String(), want)
	}
}

func TestPublishConcurrentVersions(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := fmt.Sprintf("1.0.%d", i)
			rec := doPublish(ts, token, "/"+publishPkg,
				publishPayload(t, publishPkg, v, []byte("tar-"+v), nil), "")
			codes[i] = rec.Code
		}()
	}
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusCreated {
			t.Fatalf("publish %d: status %d, want 201", i, code)
		}
	}

	man := doReq(t, ts.handler, http.MethodGet, "/"+publishPkg,
		map[string]string{"Authorization": "Bearer " + token}, nil)
	var doc struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	json.Unmarshal(man.Body.Bytes(), &doc)
	if len(doc.Versions) != n {
		t.Errorf("stored versions = %d, want %d", len(doc.Versions), n)
	}
}
