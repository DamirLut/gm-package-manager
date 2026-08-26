package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"server/internal/storage"
)

// GET /-/verdaccio/data/sidebar/<pkg>?v=<ver>       — metadata panel
// GET /-/verdaccio/data/package/readme/<pkg>?v=<ver> — readme; the optional
// v query parameter selects a specific version instead of dist-tags.latest.
func webPkgName(r *http.Request) string {
	name := chi.URLParam(r, "*")
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	return name
}

// resolveVersion returns the requested version from the manifest, or
// dist-tags.latest when no version is requested; ok is false when the
// selection does not exist in the manifest.
func resolveVersion(versions map[string]json.RawMessage, distTags map[string]string, requested string) (json.RawMessage, bool) {
	if requested != "" {
		raw, ok := versions[requested]
		return raw, ok
	}
	raw, ok := versions[distTags["latest"]]
	return raw, ok
}

func handleSidebar(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := webPkgName(r)
		data, err := store.GetManifest(r.Context(), name)
		if errors.Is(err, storage.ErrNotExist) {
			WriteError(w, ErrNotFound)
			return
		}
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}

		var doc map[string]json.RawMessage
		if json.Unmarshal(data, &doc) != nil {
			WriteError(w, ErrInternal)
			return
		}

		var tags map[string]string
		json.Unmarshal(doc["dist-tags"], &tags)
		var versions map[string]json.RawMessage
		json.Unmarshal(doc["versions"], &versions)

		raw, ok := resolveVersion(versions, tags, r.URL.Query().Get("v"))
		if !ok {
			WriteError(w, ErrNotFound)
			return
		}
		latest := map[string]any{}
		if json.Unmarshal(raw, &latest) != nil {
			WriteError(w, ErrInternal)
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"_id":       name,
			"latest":    latest,
			"versions":  versions,
			"dist-tags": tags,
			"time":      doc["time"],
		})
	}
}

func handleReadme(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := webPkgName(r)
		data, err := store.GetManifest(r.Context(), name)
		if errors.Is(err, storage.ErrNotExist) {
			WriteError(w, ErrNotFound)
			return
		}
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}

		var doc struct {
			DistTags map[string]string          `json:"dist-tags"`
			Versions map[string]json.RawMessage `json:"versions"`
		}
		if json.Unmarshal(data, &doc) != nil {
			WriteError(w, ErrInternal)
			return
		}

		raw, ok := resolveVersion(doc.Versions, doc.DistTags, r.URL.Query().Get("v"))
		if !ok {
			WriteError(w, ErrNotFound)
			return
		}
		var version struct {
			Readme string `json:"readme"`
		}
		json.Unmarshal(raw, &version)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(version.Readme))
	}
}
