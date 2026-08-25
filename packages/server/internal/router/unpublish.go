package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/storage"
)

// DELETE /<pkg>/-rev/:rev — npm unpublish: :rev must match the packument's
// _rev, a stale one is a 409; shares the publish right.
func handleUnpublish(store storage.Storage, auditor *audit.Logger, rules []access.Rule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, rest := splitPkg(r.URL.Path)
		if name == "" || !strings.HasPrefix(rest, "-rev/") {
			WriteError(w, ErrNotFound)
			return
		}

		p, _ := auth.UserFrom(r.Context())
		if p == nil {
			unauthorized(w)
			return
		}
		if !p.Can(auth.ActionPublish, name) || !access.Allow(rules, auth.ActionPublish, name, p) {
			auditor.Record(audit.Event{
				Action:  audit.ActionPackageAccessDenied,
				Actor:   p.Name,
				Package: name,
				IP:      clientIP(r),
				UA:      r.UserAgent(),
			})
			WriteError(w, ErrForbidden)
			return
		}

		unlock := store.Lock(name)
		defer unlock()

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
			Rev string `json:"_rev"`
		}
		json.Unmarshal(data, &doc)
		if got := strings.TrimPrefix(rest, "-rev/"); got == "" || got != doc.Rev {
			WriteError(w, NewError(http.StatusConflict, "revision conflict"))
			return
		}

		if err := store.DeletePackage(r.Context(), name); err != nil {
			WriteError(w, ErrInternal)
			return
		}

		auditor.Record(audit.Event{
			Action:  audit.ActionPackageUnpublish,
			Actor:   p.Name,
			Package: name,
			Version: "*",
			IP:      clientIP(r),
			UA:      r.UserAgent(),
			Success: true,
		})
		WriteJSON(w, http.StatusOK, map[string]string{"ok": "package removed"})
	}
}
