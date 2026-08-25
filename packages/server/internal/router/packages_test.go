package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestPackagesList(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "alice")

	if rec := doPublish(ts, token, "/@acme/lib",
		publishPayload(t, "@acme/lib", "1.0.0", []byte("lib v1 tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish lib 1.0.0: status %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := doPublish(ts, token, "/@acme/lib",
		publishPayload(t, "@acme/lib", "1.1.0", []byte("lib v2 tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish lib 1.1.0: status %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := doPublish(ts, token, "/@acme/other",
		publishPayload(t, "@acme/other", "2.0.0", []byte("other tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish other 2.0.0: status %d, body %s", rec.Code, rec.Body.String())
	}

	type entry struct {
		ID      string            `json:"_id"`
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Time    string            `json:"time"`
		GM      map[string]string `json:"gm"`
		Dist    struct {
			Tarball string `json:"tarball"`
			Shasum  string `json:"shasum"`
		} `json:"dist"`
	}

	var short, verdaccio []entry
	for _, target := range []string{"/-/packages", "/-/verdaccio/data/packages"} {
		rec := doReq(t, ts.handler, http.MethodGet, target, nil, nil) // anonymous read stays open
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d, body %s", target, rec.Code, rec.Body.String())
		}
		var list []entry
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("GET %s: unmarshal %q: %v", target, rec.Body.String(), err)
		}
		if len(list) != 2 {
			t.Fatalf("GET %s: entries = %d, want 2 (%q)", target, len(list), rec.Body.String())
		}
		if target == "/-/verdaccio/data/packages" {
			verdaccio = list
		} else {
			short = list
		}
	}
	if fmt.Sprint(short) != fmt.Sprint(verdaccio) {
		t.Errorf("paths disagree:\nshort     %v\nverdaccio %v", short, verdaccio)
	}

	lib, other := short[0], short[1] // sorted by name
	if lib.Name != "@acme/lib" || other.Name != "@acme/other" {
		t.Fatalf("order wrong: %+v", short)
	}

	if lib.Version != "1.1.0" {
		t.Errorf("latest version = %q, want 1.1.0", lib.Version)
	}
	if lib.ID != "@acme/lib@1.1.0" {
		t.Errorf("_id = %q, want @acme/lib@1.1.0", lib.ID)
	}
	if lib.GM["displayName"] != "Lib 1.1.0" || lib.GM["destination"] != "scripts/1.1.0" {
		t.Errorf("gm not from latest version: %+v", lib.GM)
	}
	if lib.Time == "" {
		t.Error("time is empty")
	}
	if want := "http://example.com/@acme/lib/-/lib-1.1.0.tgz"; lib.Dist.Tarball != want {
		t.Errorf("dist.tarball = %q, want %q", lib.Dist.Tarball, want)
	}
}

func TestPackagesListEmpty(t *testing.T) {
	h := newTestServer(t).handler

	rec := doReq(t, h, http.MethodGet, "/-/packages", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if body := bytes.TrimSpace(rec.Body.Bytes()); !bytes.Equal(body, []byte("[]")) {
		t.Errorf("body = %s, want [] (empty array, not null)", body)
	}
}

// Broken entries are skipped, good ones still listed.
func TestPackagesListSkipsBrokenEntries(t *testing.T) {
	ts := newTestServer(t)

	broken := map[string]string{
		"@bad/corrupt":  `{oops`,
		"@bad/nolatest": `{"name":"@bad/nolatest","versions":{}}`,
		"@bad/noversion": `{
			"name":"@bad/noversion",
			"dist-tags":{"latest":"9.9.9"},
			"versions":{"1.0.0":{"version":"1.0.0"}},
			"time":{"1.0.0":"2026-01-01T00:00:00Z"}
		}`,
	}
	for name, man := range broken {
		if err := ts.store.PutManifest(t.Context(), name, []byte(man)); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}
	token := loginToken(t, ts, "alice")
	if rec := doPublish(ts, token, "/@good/pkg",
		publishPayload(t, "@good/pkg", "1.0.0", []byte("tar"), nil), ""); rec.Code != http.StatusCreated {
		t.Fatalf("publish good: status %d, body %s", rec.Code, rec.Body.String())
	}

	rec := doReq(t, ts.handler, http.MethodGet, "/-/verdaccio/data/packages", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 1 || list[0].Name != "@good/pkg" {
		t.Errorf("entries = %+v, want only @good/pkg", list)
	}
}
