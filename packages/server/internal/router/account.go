package router

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"server/internal/audit"
	"server/internal/identity"
)

const accountBodyLimit = 4 << 10

// GET /-/account/logins?limit=N — the login history.
func handleAccountLogins(svc *identity.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess == nil {
			unauthorizedWeb(w)
			return
		}
		events, err := svc.Logins(r.Context(), sess.UserID, limitParam(r))
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-store")
		WriteJSON(w, http.StatusOK, events)
	}
}

// GET /-/account/activity?limit=N — the recent-activity feed.
func handleAccountActivity(svc *identity.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess == nil {
			unauthorizedWeb(w)
			return
		}
		events, err := svc.Activity(r.Context(), sess.UserID, limitParam(r))
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-store")
		WriteJSON(w, http.StatusOK, events)
	}
}

// DELETE /-/account/identities/{provider} — forget a linked provider account.
func handleIdentityUnlink(svc *identity.Service, auditor *audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess == nil {
			unauthorizedWeb(w)
			return
		}
		provider := chi.URLParam(r, "provider")
		if _, ok := svc.Provider(provider); !ok {
			WriteError(w, ErrNotFound)
			return
		}
		if err := svc.UnlinkIdentity(r.Context(), sess.UserID, provider); err != nil {
			switch {
			case errors.Is(err, identity.ErrLastLoginMethod):
				WriteError(w, NewError(http.StatusBadRequest, "cannot unlink the last login method"))
			case errors.Is(err, identity.ErrIdentityNotFound):
				WriteError(w, ErrNotFound)
			default:
				WriteError(w, ErrInternal)
			}
			return
		}
		ip, ua := clientIP(r), r.UserAgent()
		svc.RecordEvent(r.Context(), sess.UserID, identity.EventIdentityUnlinked, ip, ua, provider, "")
		auditor.Record(audit.Event{
			Action:   audit.ActionIdentityUnlinked,
			Actor:    sess.Username,
			IP:       ip,
			UA:       ua,
			Success:  true,
			Metadata: map[string]string{"provider": provider},
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /-/account — irreversible; the body must confirm the username.
// Published packages stay (registry immutability, as in npm), but every
// session, identity, npm token and event of the user is gone.
func handleAccountDelete(svc *identity.Service, auditor *audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess == nil {
			unauthorizedWeb(w)
			return
		}
		var body struct {
			Username string `json:"username"`
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, accountBodyLimit))
		if err != nil || json.Unmarshal(data, &body) != nil || body.Username != sess.Username {
			WriteError(w, NewError(http.StatusBadRequest, "type your username to confirm account deletion"))
			return
		}
		if err := svc.DeleteAccount(r.Context(), sess.UserID); err != nil {
			WriteError(w, ErrInternal)
			return
		}
		svc.ClearSessionCookie(w, r)
		auditor.Record(audit.Event{
			Action:  audit.ActionAccountDeleted,
			Actor:   sess.Username,
			IP:      clientIP(r),
			UA:      r.UserAgent(),
			Success: true,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func unauthorizedWeb(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store")
	WriteJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
}

func limitParam(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}
