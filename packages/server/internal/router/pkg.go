package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/storage"
)

const abbrevType = "application/vnd.npm.install-v1+json"

// splitPkg splits a request path into a package name and the rest.
// net/http puts an already-decoded string into r.URL.Path, so all
// spelling variants ("/@scope/foo", "/@scope%2Ffoo", "/%40scope%2Ffoo")
// converge to a single form. Router patterns like /{pkg}/-/{file} don't work
// here: the slash inside a scoped name breaks segment splitting.
func splitPkg(p string) (name, rest string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}
	if strings.HasPrefix(p, "@") {
		scope, rest, _ := strings.Cut(p, "/")
		name, tail, _ := strings.Cut(rest, "/")
		if name == "" {
			return "", ""
		}
		return scope + "/" + name, tail
	}
	name, rest, _ = strings.Cut(p, "/")
	if name == "" {
		return "", ""
	}
	return name, rest
}

// GET /@scope/foo, /@scope%2Ffoo, /%40scope%2Ffoo — package document
// GET /<name>/-/<file>.tgz — tarball download
func handlePkg(store storage.Storage, auditor *audit.Logger, rules []access.Rule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, rest := splitPkg(r.URL.Path)
		if name == "" || (rest != "" && !strings.HasPrefix(rest, "-/")) {
			WriteError(w, ErrNotFound)
			return
		}
		if !guardRead(w, r, auditor, rules, name) {
			return
		}
		if rest == "" {
			serveManifest(w, r, store, name)
			return
		}
		serveTarball(w, r, store, name, strings.TrimPrefix(rest, "-/"))
	}
}

// guardRead combines both layers: the server's packages config and the
// token's read scopes; on failure it writes the response and returns false.
func guardRead(w http.ResponseWriter, r *http.Request, auditor *audit.Logger, rules []access.Rule, name string) bool {
	p, _ := auth.UserFrom(r.Context())
	if access.Allow(rules, auth.ActionRead, name, p) &&
		(p == nil || p.Can(auth.ActionRead, name)) {
		return true
	}
	if p == nil {
		unauthorized(w)
		return false
	}
	auditor.Record(audit.Event{
		Action:  audit.ActionPackageAccessDenied,
		Actor:   p.Name,
		Package: name,
		IP:      clientIP(r),
		UA:      r.UserAgent(),
	})
	WriteError(w, ErrForbidden)
	return false
}

// serveManifest answers with the full document, or the abbreviated form when
// the client asks for npm's install-v1 media type; both carry an ETag and
// honor If-None-Match with 304.
func serveManifest(w http.ResponseWriter, r *http.Request, store storage.Storage, name string) {
	data, err := store.GetManifest(r.Context(), name)
	if errors.Is(err, storage.ErrNotExist) {
		WriteError(w, ErrNotFound)
		return
	}
	if err != nil {
		WriteError(w, ErrInternal)
		return
	}

	body, contentType := data, "application/json"
	if strings.Contains(r.Header.Get("Accept"), abbrevType) {
		body, err = abbreviate(data)
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}
		contentType = abbrevType
	}

	etag := etagOf(body)
	w.Header().Set("ETag", etag)
	if ifNoneMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func serveTarball(w http.ResponseWriter, r *http.Request, store storage.Storage, name, filename string) {
	rc, size, err := store.GetTarball(r.Context(), name, filename)
	if errors.Is(err, storage.ErrNotExist) {
		WriteError(w, ErrNotFound)
		return
	}
	if err != nil {
		WriteError(w, ErrInternal)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func etagOf(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// ifNoneMatch applies RFC 7232 weak comparison to a comma-separated header.
func ifNoneMatch(header, etag string) bool {
	for _, cand := range strings.Split(header, ",") {
		cand = strings.Trim(strings.TrimPrefix(strings.TrimSpace(cand), "W/"), `"`)
		if cand == strings.Trim(etag, `"`) {
			return true
		}
	}
	return false
}

// abbrevKeys is npm's install-v1 subset plus gm: gmpm reads gm.destination
// from metadata without downloading the tarball first.
var abbrevKeys = map[string]struct{}{
	"name": {}, "version": {},
	"dependencies": {}, "optionalDependencies": {},
	"peerDependencies": {}, "peerDependenciesMeta": {},
	"bundleDependencies": {}, "bundledDependencies": {},
	"bin": {}, "directories": {}, "engines": {},
	"os": {}, "cpu": {}, "funding": {}, "deprecated": {},
	"_hasShrinkwrap": {}, "workspaces": {},
	"dist": {}, "gm": {},
}

type abbrevDoc struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Modified string                     `json:"modified,omitempty"`
	Versions map[string]json.RawMessage `json:"versions"`
}

func abbreviate(data []byte) ([]byte, error) {
	var full struct {
		Name     string                     `json:"name"`
		DistTags map[string]string          `json:"dist-tags"`
		Time     map[string]string          `json:"time"`
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(data, &full); err != nil {
		return nil, err
	}

	out := abbrevDoc{
		Name:     full.Name,
		DistTags: full.DistTags,
		Modified: full.Time["modified"],
		Versions: make(map[string]json.RawMessage, len(full.Versions)),
	}
	for v, raw := range full.Versions {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		filtered := make(map[string]json.RawMessage, len(obj))
		for k, val := range obj {
			if _, keep := abbrevKeys[k]; keep {
				filtered[k] = val
			}
		}
		enc, err := json.Marshal(filtered)
		if err != nil {
			return nil, err
		}
		out.Versions[v] = enc
	}
	return json.Marshal(out)
}
