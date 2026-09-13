package router

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"server/internal/audit"
	"server/internal/auth"
	"server/internal/identity"
)

// Error reasons carried to the website as ?auth_error=...; the site maps
// them to banner texts (see the header error handling).
const (
	oauthErrProviderDisabled = "provider_disabled"
	oauthErrStateInvalid     = "state_invalid"
	oauthErrExchangeFailed   = "exchange_failed"
	oauthErrSignupDisabled   = "signup_disabled"
	oauthErrInternal         = "internal"
	oauthErrAccessDenied     = "access_denied"
	oauthErrIdentityTaken    = "identity_taken"
	oauthErrProviderLinked   = "provider_linked"
	oauthErrSuspended        = "account_suspended"
	oauthErrSessionExpired   = "session_expired"
)

type sessionKeyType struct{}

// sessionAuth resolves the gmpm_session cookie into the context. Requests
// without a valid session stay anonymous; each handler decides.
func sessionAuth(svc *identity.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret := identity.ReadSessionCookie(r); secret != "" {
				if sess, err := svc.VerifySession(r.Context(), secret); err == nil {
					r = r.WithContext(withSession(r.Context(), sess))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func withSession(ctx context.Context, sess *identity.Session) context.Context {
	return context.WithValue(ctx, sessionKeyType{}, sess)
}

func sessionFrom(ctx context.Context) *identity.Session {
	sess, _ := ctx.Value(sessionKeyType{}).(*identity.Session)
	return sess
}

// GET /-/auth/{provider}/start — sign in through the provider, or link it
// to the current user when a website session already exists.
func handleOAuthStart(svc *identity.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "provider")
		provider, ok := svc.Provider(name)
		if !ok {
			oauthRedirect(w, r, "/", oauthErrProviderDisabled)
			return
		}

		st := identity.State{Kind: identity.StateLogin, Provider: name, Redirect: sanitizeRedirect(r.URL.Query().Get("redirect"))}
		if sess := sessionFrom(r.Context()); sess != nil {
			st.Kind, st.UserID = identity.StateConnect, sess.UserID
		}
		secret, err := svc.NewState(r.Context(), st)
		if err != nil {
			oauthRedirect(w, r, "/", oauthErrInternal)
			return
		}
		http.Redirect(w, r, provider.AuthCodeURL(oauthCallbackURL(r, name), secret), http.StatusFound)
	}
}

// GET /-/auth/{provider}/callback — the provider returns the browser here.
func handleOAuthCallback(svc *identity.Service, authSvc *auth.Service, auditor *audit.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "provider")
		provider, ok := svc.Provider(name)
		if !ok {
			oauthRedirect(w, r, "/", oauthErrProviderDisabled)
			return
		}
		ctx := r.Context()
		ip, ua := clientIP(r), r.UserAgent()

		st, err := svc.ConsumeState(ctx, r.URL.Query().Get("state"))
		if err != nil || st.Provider != name {
			oauthRedirect(w, r, "/", oauthErrStateInvalid)
			return
		}
		if r.URL.Query().Get("error") != "" {
			// the user refused the consent screen
			oauthRedirect(w, r, st.Redirect, oauthErrAccessDenied)
			return
		}

		external, err := provider.Exchange(ctx, r.URL.Query().Get("code"), oauthCallbackURL(r, name))
		if err != nil {
			auditor.Record(audit.Event{Action: audit.ActionOAuthLoginFailed, IP: ip, UA: ua, Metadata: map[string]string{"provider": name}})
			oauthRedirect(w, r, st.Redirect, oauthErrExchangeFailed)
			return
		}

		switch st.Kind {
		case identity.StateConnect:
			oauthConnect(w, r, svc, auditor, external, st, name, ip, ua)
		case identity.StateLogin:
			oauthLogin(w, r, svc, authSvc, auditor, external, st, name, ip, ua)
		default:
			oauthRedirect(w, r, "/", oauthErrStateInvalid)
		}
	}
}

func oauthConnect(w http.ResponseWriter, r *http.Request, svc *identity.Service, auditor *audit.Logger,
	external *identity.ExternalUser, st *identity.State, name, ip, ua string,
) {
	ctx := r.Context()
	sess := sessionFrom(ctx)
	if sess == nil || sess.UserID != st.UserID {
		oauthRedirect(w, r, st.Redirect, oauthErrSessionExpired)
		return
	}
	if err := svc.LinkIdentity(ctx, sess.UserID, external); err != nil {
		auditor.Record(audit.Event{Action: audit.ActionIdentityLinked, Actor: sess.Username, IP: ip, UA: ua, Metadata: map[string]string{"provider": name}})
		oauthRedirect(w, r, st.Redirect, oauthErrReason(err))
		return
	}
	svc.RecordEvent(ctx, sess.UserID, identity.EventIdentityLinked, ip, ua, name, external.Username)
	auditor.Record(audit.Event{
		Action:   audit.ActionIdentityLinked,
		Actor:    sess.Username,
		IP:       ip,
		UA:       ua,
		Success:  true,
		Metadata: map[string]string{"provider": name, "external": external.Username},
	})
	http.Redirect(w, r, st.Redirect, http.StatusFound)
}

func oauthLogin(w http.ResponseWriter, r *http.Request, svc *identity.Service, authSvc *auth.Service,
	auditor *audit.Logger, external *identity.ExternalUser, st *identity.State, name, ip, ua string,
) {
	ctx := r.Context()

	user, err := svc.LinkedUser(ctx, name, external.ID)
	switch {
	case err == nil:
		if user.Status != identity.StatusActive {
			oauthRedirect(w, r, st.Redirect, oauthErrSuspended)
			return
		}
	case errors.Is(err, identity.ErrIdentityNotFound) && authSvc.AllowSignup():
		user = &identity.LinkedUser{}
		if user.ID, user.Username, err = svc.SignupWithIdentity(ctx, external); err != nil {
			oauthRedirect(w, r, st.Redirect, oauthErrInternal)
			return
		}
	case errors.Is(err, identity.ErrIdentityNotFound):
		auditor.Record(audit.Event{Action: audit.ActionOAuthLoginFailed, Actor: external.Username, IP: ip, UA: ua, Metadata: map[string]string{"provider": name, "reason": "signup_disabled"}})
		oauthRedirect(w, r, st.Redirect, oauthErrSignupDisabled)
		return
	default:
		oauthRedirect(w, r, st.Redirect, oauthErrInternal)
		return
	}

	secret, err := svc.CreateSession(ctx, user.ID, ip, ua)
	if err != nil {
		oauthRedirect(w, r, st.Redirect, oauthErrInternal)
		return
	}
	svc.SetSessionCookie(w, r, secret)
	svc.RecordEvent(ctx, user.ID, identity.EventLogin, ip, ua, name, "")
	auditor.Record(audit.Event{
		Action:   audit.ActionOAuthLoginSuccess,
		Actor:    user.Username,
		IP:       ip,
		UA:       ua,
		Success:  true,
		Metadata: map[string]string{"provider": name},
	})
	http.Redirect(w, r, st.Redirect, http.StatusFound)
}

// GET /-/auth/session — the current website visitor or null.
func handleWebSession(svc *identity.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store")
		sess := sessionFrom(r.Context())
		if sess == nil {
			WriteJSON(w, http.StatusOK, sessionResponse{})
			return
		}
		profile, err := svc.Profile(r.Context(), sess.UserID)
		if err != nil {
			WriteJSON(w, http.StatusOK, sessionResponse{})
			return
		}
		WriteJSON(w, http.StatusOK, sessionResponse{User: profile})
	}
}

type sessionResponse struct {
	User *identity.Profile `json:"user"`
}

// POST /-/auth/logout
func handleLogout(svc *identity.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if secret := identity.ReadSessionCookie(r); secret != "" {
			svc.RevokeSession(r.Context(), secret)
		}
		svc.ClearSessionCookie(w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

// sanitizeRedirect allows only local site paths — no open redirect through
// the login, and nothing on the registry side of the proxy (/-...).
func sanitizeRedirect(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") ||
		strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/-") || strings.Contains(p, "://") {
		return "/"
	}
	return p
}

// oauthCallbackURL mirrors tarballURL: the registered OAuth app callback is
// scheme+host exact, so both must match how the browser reaches the server.
func oauthCallbackURL(r *http.Request, provider string) string {
	return requestScheme(r) + "://" + r.Host + "/-/auth/" + provider + "/callback"
}

// oauthRedirect sends the browser back into the site, with an ?auth_error=
// banner code when the round trip failed.
func oauthRedirect(w http.ResponseWriter, r *http.Request, path, reason string) {
	if reason != "" {
		u, err := url.Parse(path)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		q := u.Query()
		q.Set("auth_error", reason)
		u.RawQuery = q.Encode()
		path = u.String()
	}
	http.Redirect(w, r, path, http.StatusFound)
}

func oauthErrReason(err error) string {
	switch {
	case errors.Is(err, identity.ErrIdentityTaken):
		return oauthErrIdentityTaken
	case errors.Is(err, identity.ErrProviderLinked):
		return oauthErrProviderLinked
	default:
		return oauthErrInternal
	}
}
