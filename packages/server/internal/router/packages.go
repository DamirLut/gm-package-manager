package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"server/internal/storage"
)

// GET /-/verdaccio/data/packages and GET /-/packages — package list for the
// latest version of every local package, Verdaccio UI payload shape.
func handlePackages(store storage.Storage, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		names, err := store.ListPackages(r.Context())
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}
		slices.Sort(names)

		list := make([]map[string]any, 0, len(names))
		for _, name := range names {
			info, err := latestVersion(r.Context(), store, name)
			if err != nil {
				log.Warn("package list: skipping broken entry", "package", name, "error", err)
				continue
			}
			if info != nil {
				list = append(list, info)
			}
		}

		WriteJSON(w, http.StatusOK, list)
	}
}

// latestVersion returns the manifest's versions[dist-tags.latest] stamped
// with time[latest]; nil means no servable latest version.
func latestVersion(ctx context.Context, store storage.Storage, name string) (map[string]any, error) {
	data, err := store.GetManifest(ctx, name)
	if errors.Is(err, storage.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var doc struct {
		DistTags map[string]string          `json:"dist-tags"`
		Versions map[string]json.RawMessage `json:"versions"`
		Time     map[string]string          `json:"time"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil, errors.New("broken manifest")
	}

	latest := doc.DistTags["latest"]
	raw, ok := doc.Versions[latest]
	if !ok {
		return nil, nil
	}

	info := map[string]any{}
	if json.Unmarshal(raw, &info) != nil {
		return nil, fmt.Errorf("broken version %s", latest)
	}
	info["_id"] = name + "@" + latest
	info["time"] = doc.Time[latest]
	return info, nil
}
