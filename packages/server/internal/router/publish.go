package router

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/storage"
)

// base64 tarball inside JSON; 10MiB = Verdaccio default max_body_size.
const publishBodyLimit = 10 << 20

// Every published version must carry gm: gmpm installs into
// <project>/<gm.destination>, the IDE shows gm.displayName — without them
// the install silently lands in a fallback path
type gmMeta struct {
	Destination string `json:"destination"`
	DisplayName string `json:"displayName"`
}

type attachment struct {
	Data   string `json:"data"`
	Length int64  `json:"length"`
}

type publishBody struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]json.RawMessage `json:"versions"`
	Attaches map[string]attachment      `json:"_attachments"`
}

type publishResponse struct {
	Success bool   `json:"success"`
	OK      string `json:"ok"`
}

// PUT /<pkg> — npm publish contract: statuses are protocol, messages are not.
func handlePublish(store storage.Storage, auditor *audit.Logger, rules []access.Rule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, rest := splitPkg(r.URL.Path)
		if name == "" || rest != "" {
			WriteError(w, NewError(http.StatusNotFound, "unsupported registry call"))
			return
		}

		p, _ := auth.UserFrom(r.Context())
		if p == nil || !p.Can(auth.ActionPublish, name) ||
			!access.Allow(rules, auth.ActionPublish, name, p) {
			actor := ""
			if p != nil {
				actor = p.Name
			}
			auditor.Record(audit.Event{
				Action:  audit.ActionPackageAccessDenied,
				Actor:   actor,
				Package: name,
				IP:      clientIP(r),
				UA:      r.UserAgent(),
			})
			WriteError(w, ErrForbidden)
			return
		}

		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			WriteError(w, NewError(http.StatusUnsupportedMediaType, "wrong content-type, expect: application/json"))
			return
		}

		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, publishBodyLimit))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteError(w, NewError(http.StatusRequestEntityTooLarge, "request body too large"))
			return
		}
		if err != nil {
			WriteError(w, NewError(http.StatusBadRequest, "can't parse incoming json"))
			return
		}
		var body publishBody
		if err := json.Unmarshal(data, &body); err != nil {
			WriteError(w, NewError(http.StatusBadRequest, "can't parse incoming json"))
			return
		}

		if !validatePkgName(name) || body.Name != name {
			WriteError(w, NewError(http.StatusBadRequest, "invalid or mismatching package name"))
			return
		}

		if len(body.Versions) != 1 || len(body.Attaches) != 1 {
			WriteError(w, NewError(http.StatusBadRequest, "must contain exactly one version and one attachment"))
			return
		}

		version, verRaw, _ := onlyKey(body.Versions)
		filename, att, _ := onlyKey(body.Attaches)
		// npm names scoped tarballs "@scope/name-x.y.z.tgz" but storage
		// keeps one flat directory per package; store and serve by base name.
		filename = path.Base(filename)

		ver, err := semver.StrictNewVersion(version)
		if err != nil {
			WriteError(w, NewError(http.StatusBadRequest, "invalid semver version: "+version))
			return
		}

		meta := readVerMeta(verRaw)
		if inner, err := semver.StrictNewVersion(meta.Version); err != nil || !inner.Equal(ver) {
			WriteError(w, NewError(http.StatusBadRequest, "version mismatch: "+meta.Version+" != "+version))
			return
		}

		if msg := gmProblem(verRaw); msg != "" {
			WriteError(w, NewError(http.StatusUnprocessableEntity, msg))
			return
		}

		tarball, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil || len(tarball) == 0 {
			WriteError(w, NewError(http.StatusUnprocessableEntity, "refusing to accept zero-length file"))
			return
		}

		sum := sha1.Sum(tarball)
		shasum := hex.EncodeToString(sum[:])

		if meta.Dist.Shasum != shasum {
			WriteError(w, NewError(http.StatusBadRequest, "shasum error, "+meta.Dist.Shasum+" != "+shasum))
			return
		}

		// The 409 check and all writes share the per-package lock, so
		// concurrent publishes of one package cannot interleave.
		unlock := store.Lock(name)
		defer unlock()

		existing, err := store.GetManifest(r.Context(), name)
		isNew := errors.Is(err, storage.ErrNotExist)
		if err != nil && !isNew {
			WriteError(w, ErrInternal)
			return
		}

		doc := map[string]any{}
		if !isNew && json.Unmarshal(existing, &doc) != nil {
			WriteError(w, ErrInternal)
			return
		}

		versions := asMap(doc["versions"])
		if _, taken := versions[version]; taken {
			WriteError(w, NewError(http.StatusConflict, "this package is already present"))
			return
		}

		if _, err := store.PutTarball(r.Context(), name, filename, bytes.NewReader(tarball)); err != nil {
			WriteError(w, ErrInternal)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		doc["_id"], doc["name"] = name, name
		doc["_rev"] = nextRev(doc)

		verObj := verDocument(verRaw, tarballURL(r, name, filename), shasum)
		stampAuthor(verObj, p.Name)
		versions[version] = verObj
		doc["versions"] = versions

		tags := asMap(doc["dist-tags"])
		for tag, target := range body.DistTags {
			tags[tag] = target
		}
		if isNew {
			if _, ok := tags["latest"]; !ok {
				tags["latest"] = version
			}
			doc["maintainers"] = []any{map[string]string{"name": p.Name}}
		}
		doc["dist-tags"] = tags

		times := asMap(doc["time"])
		if isNew {
			times["created"] = now
		}
		times[version], times["modified"] = now, now
		doc["time"] = times

		attaches := asMap(doc["_attachments"])
		attaches[filename] = map[string]any{"shasum": shasum, "length": len(tarball)}
		doc["_attachments"] = attaches

		merged, err := json.Marshal(doc)
		if err == nil {
			err = store.PutManifest(r.Context(), name, merged)
		}
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}

		auditor.Record(audit.Event{
			Action:  audit.ActionPackagePublish,
			Actor:   p.Name,
			Package: name,
			Version: version,
			IP:      clientIP(r),
			UA:      r.UserAgent(),
			Success: true,
		})

		okMsg := "created new package"
		if !isNew {
			okMsg = "package changed"
		}
		WriteJSON(w, http.StatusCreated, publishResponse{Success: true, OK: okMsg})
	}
}

func onlyKey[T any](m map[string]T) (string, T, bool) {
	for k, v := range m {
		return k, v, true
	}
	var zero T
	return "", zero, false
}

func asMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return m
}

type verMeta struct {
	Version string `json:"version"`
	Dist    struct {
		Shasum string `json:"shasum"`
	} `json:"dist"`
}

// broken JSON decodes to zero values, which fail validation downstream.
func readVerMeta(raw json.RawMessage) verMeta {
	var m verMeta
	json.Unmarshal(raw, &m)
	return m
}

func gmProblem(raw json.RawMessage) string {
	const prefix = "gm schema: "
	gmRaw, ok := verField(raw, "gm")
	if !ok {
		return prefix + `package.json must contain a gm object with destination and displayName`
	}
	var gm gmMeta
	if obj, ok := gmRaw.(map[string]any); ok {
		b, _ := json.Marshal(obj)
		json.Unmarshal(b, &gm)
	}
	if gm.Destination == "" {
		return prefix + `gm.destination must be a non-empty relative path`
	}
	if !validDestination(gm.Destination) {
		return prefix + `gm.destination must be a relative path without '..', '.' or '\' segments`
	}
	if gm.DisplayName == "" {
		return prefix + `gm.displayName must be a non-empty string`
	}
	return ""
}

func verField(raw json.RawMessage, field string) (any, bool) {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return nil, false
	}
	v, ok := obj[field]
	return v, ok
}

// fills in dist.tarball/shasum, keeps everything else as sent.
func verDocument(raw json.RawMessage, tarballURL, shasum string) map[string]any {
	obj := map[string]any{}
	if json.Unmarshal(raw, &obj) != nil {
		obj = map[string]any{}
	}
	dist := asMap(obj["dist"])
	dist["tarball"], dist["shasum"] = tarballURL, shasum
	obj["dist"] = dist
	return obj
}

func stampAuthor(ver map[string]any, actor string) {
	author := asMap(ver["author"])
	ver["author"] = author
	author["name"] = actor
	author["avatar"] = avatarURL(actor)
}

func avatarURL(username string) string {
	return "https://blobatar.dev/avatar/" + url.PathEscape(username) + "?gen=2"
}

// stored URLs must match how clients reach this server (reverse proxy included).
func tarballURL(r *http.Request, name, filename string) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/" + name + "/-/" + filename
}

// nextRev produces the CouchDB-style revision "N-<hash>" npm unpublish
// sends back via DELETE /<pkg>/-rev/:rev.
func nextRev(doc map[string]any) string {
	n := 0
	if cur, ok := doc["_rev"].(string); ok {
		v, err := strconv.Atoi(strings.SplitN(cur, "-", 2)[0])
		if err == nil {
			n = v
		}
	}
	return fmt.Sprintf("%d-%s", n+1, randomSuffix())
}

func randomSuffix() string {
	var b [4]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
