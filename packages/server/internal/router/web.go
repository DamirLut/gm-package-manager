package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"server/internal/storage"
)

// GET /-/verdaccio/data/sidebar/<pkg>       — metadata panel
// GET /-/verdaccio/data/package/readme/<pk> — readme of the latest version
func webPkgName(r *http.Request) string {
	name := chi.URLParam(r, "*")
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	return name
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

		latestRaw, ok := versions[tags["latest"]]
		latest := map[string]any{}
		if ok && json.Unmarshal(latestRaw, &latest) != nil {
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

		var latest struct {
			Readme string `json:"readme"`
		}
		if raw, ok := doc.Versions[doc.DistTags["latest"]]; ok {
			json.Unmarshal(raw, &latest)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(latest.Readme))
	}
}
