package router

import (
	"net/http"
	"strings"

	"server/internal/auth"
)

// bearer authenticates "Authorization: Bearer" requests and puts the
// resulting Principal into the context. Requests without credentials stay
// anonymous; package reads then require the read scope (see pkg.go). Other
// schemes such as Basic are bootstrap-only for the login endpoint and
// ignored elsewhere.
func bearer(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}
			scheme, token, _ := strings.Cut(header, " ")
			if !strings.EqualFold(scheme, "Bearer") {
				next.ServeHTTP(w, r)
				return
			}
			p, err := svc.Verify(r.Context(), token)
			if err != nil {
				// Bootstrap must survive a stale token attached by the client.
				if strings.HasPrefix(r.URL.Path, "/-/user/") {
					next.ServeHTTP(w, r)
					return
				}
				unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	WriteJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
}
