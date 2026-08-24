package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"server/internal/storage"
)

func New(logger *slog.Logger, store storage.Storage) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/-/ping", handlePing)

	// npm calls this on every install; an empty object is enough
	r.Post("/-/npm/v1/security/advisories/bulk", handleAudit)

	// scoped names contain a slash and arrive in different encodings,
	// so the package path is parsed manually (see pkg.go)
	r.Get("/*", handlePkg(store))

	return r
}

func handlePing(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleAudit(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, struct{}{})
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(rec, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.Status(),
				"duration", time.Since(start),
			)
		})
	}
}
