package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"server/internal/access"
	"server/internal/audit"
	"server/internal/auth"
	"server/internal/identity"
	"server/internal/storage"
)

func New(logger *slog.Logger, store storage.Storage, authenticator *auth.Service, identities *identity.Service, auditor *audit.Logger, rules []access.Rule) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))
	r.Use(bearer(authenticator))

	r.Get("/-/ping", handlePing)

	// npm calls this on every install; an empty object is enough
	r.Post("/-/npm/v1/security/advisories/bulk", handleAudit)

	// npm adduser bootstrap (Basic or JSON body, see login.go)
	r.Put("/-/user/*", handleLogin(authenticator, identities, auditor))

	// IDE package list (see packages.go)
	r.Get("/-/verdaccio/data/packages", handlePackages(store, logger))
	r.Get("/-/packages", handlePackages(store, logger))

	// website search (see search.go)
	r.Get("/-/verdaccio/data/search", handleSearch(store, logger))

	// website package page (see web.go)
	r.Get("/-/verdaccio/data/sidebar/*", handleSidebar(store))
	r.Get("/-/verdaccio/data/package/readme/*", handleReadme(store))

	// website auth & profile (see webauth.go, account.go); the session
	// middleware stays inside the group so npm traffic never pays for it
	r.Group(func(web chi.Router) {
		web.Use(sessionAuth(identities))
		web.Get("/-/auth/session", handleWebSession(identities))
		web.Get("/-/auth/{provider}/start", handleOAuthStart(identities))
		web.Get("/-/auth/{provider}/callback", handleOAuthCallback(identities, authenticator, auditor))
		web.Post("/-/auth/logout", handleLogout(identities))
		web.Get("/-/account/logins", handleAccountLogins(identities))
		web.Get("/-/account/activity", handleAccountActivity(identities))
		web.Delete("/-/account/identities/{provider}", handleIdentityUnlink(identities, auditor))
		web.Delete("/-/account", handleAccountDelete(identities, auditor))
	})

	// npm publish (see publish.go)
	r.Put("/*", handlePublish(store, identities, auditor, rules))

	// scoped names contain a slash and arrive in different encodings,
	// so the package path is parsed manually (see pkg.go)
	r.Get("/*", handlePkg(store, auditor, rules))

	// npm unpublish (see unpublish.go)
	r.Delete("/*", handleUnpublish(store, identities, auditor, rules))

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
