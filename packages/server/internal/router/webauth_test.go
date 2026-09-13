package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"server/internal/access"
	"server/internal/auth"
	"server/internal/identity"
)

// ghStubUser is the profile a stub code resolves to.
type ghStubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// githubStub serves the GitHub OAuth endpoints: the access token echoes the
// code, and /user maps token tok-<code> back to the seeded profile.
type githubStub struct {
	server *httptest.Server
	users  map[string]ghStubUser
}

func newGitHubStub(t *testing.T, users map[string]ghStubUser) *githubStub {
	t.Helper()
	stub := &githubStub{users: users}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
		}{AccessToken: "tok-" + r.FormValue("code"), TokenType: "bearer"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		u, ok := stub.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok-")]
		if !ok {
			http.Error(w, "unknown code", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(u)
	})
	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

func newOAuthTestServer(t *testing.T, allowSignup bool, users map[string]ghStubUser) *testServer {
	t.Helper()
	stub := newGitHubStub(t, users)

	idCfg := identity.DefaultConfig()
	idCfg.GitHub = &identity.GitHubConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      stub.server.URL + "/login/oauth/authorize",
		TokenURL:     stub.server.URL + "/login/oauth/access_token",
		APIBase:      stub.server.URL,
	}

	return newTestServerFull(t, auth.Config{
		AllowSignup: allowSignup,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	}, access.Default(), idCfg)
}

// oauthRoundTrip drives start → callback for one code and returns the final
// response. cookies carry existing sessions (e.g. the connect flow).
func oauthRoundTrip(t *testing.T, h http.Handler, code, redirect string, cookies map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	startHeaders := map[string]string{}
	if len(cookies) > 0 {
		startHeaders["Cookie"] = cookieHeader(cookies)
	}
	start := doReq(t, h, http.MethodGet, "/-/auth/github/start?redirect="+url.QueryEscape(redirect), startHeaders, nil)
	if start.Code != http.StatusFound {
		t.Fatalf("start status = %d, want 302", start.Code)
	}

	authorize, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse start location: %v", err)
	}
	state := authorize.Query().Get("state")
	if state == "" {
		t.Fatalf("authorize url %q carries no state", authorize)
	}

	cbHeaders := map[string]string{}
	if len(cookies) > 0 {
		cbHeaders["Cookie"] = cookieHeader(cookies)
	}
	return doReq(t, h, http.MethodGet,
		"/-/auth/github/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state),
		cbHeaders, nil)
}

func cookieHeader(cookies map[string]string) string {
	parts := make([]string, 0, len(cookies))
	for k, v := range cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

// sessionCookieValue pulls the gmpm_session secret out of a Set-Cookie header.
func sessionCookieValue(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	cookies := rec.Result().Cookies()
	idx := slices.IndexFunc(cookies, func(c *http.Cookie) bool { return c.Name == identity.SessionCookie })
	if idx < 0 {
		t.Fatalf("no %s cookie in %v", identity.SessionCookie, rec.Header().Values("Set-Cookie"))
	}
	c := cookies[idx]
	if !c.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	return c.Value
}

func oauthErrorParam(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect %q: %v", rec.Header().Get("Location"), err)
	}
	return u.Query().Get("auth_error")
}

func TestOAuthLoginSignupAndSession(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-octavia": {ID: 101, Login: "octavia"},
	})

	rec := oauthRoundTrip(t, ts.handler, "code-octavia", "/account", nil)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/account" {
		t.Fatalf("callback = %d %q, want 302 /account", rec.Code, rec.Header().Get("Location"))
	}
	secret := sessionCookieValue(t, rec)

	sess := doReq(t, ts.handler, http.MethodGet, "/-/auth/session",
		map[string]string{"Cookie": cookieHeader(map[string]string{identity.SessionCookie: secret})}, nil)
	if sess.Code != http.StatusOK {
		t.Fatalf("session status = %d, want 200", sess.Code)
	}
	var body struct {
		User *struct {
			Username   string `json:"username"`
			Identities []struct {
				Provider string `json:"provider"`
				Username string `json:"username"`
			} `json:"identities"`
		} `json:"user"`
	}
	if err := json.Unmarshal(sess.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if body.User == nil || body.User.Username != "octavia" {
		t.Fatalf("session user = %+v, want octavia", body.User)
	}
	if len(body.User.Identities) != 1 || body.User.Identities[0].Provider != "github" {
		t.Errorf("identities = %+v, want one github", body.User.Identities)
	}

	logins := doReq(t, ts.handler, http.MethodGet, "/-/account/logins",
		map[string]string{"Cookie": cookieHeader(map[string]string{identity.SessionCookie: secret})}, nil)
	var events []struct {
		Type     string `json:"type"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(logins.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode logins: %v", err)
	}
	if len(events) != 1 || events[0].Type != "login" || events[0].Provider != "github" {
		t.Errorf("logins = %+v, want one github login", events)
	}
}

func TestOAuthLoginExistingIdentity(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-octavia": {ID: 101, Login: "octavia"},
	})

	first := oauthRoundTrip(t, ts.handler, "code-octavia", "/", nil)
	secret := sessionCookieValue(t, first)

	// the same github account logging in again re-enters the same profile
	second := oauthRoundTrip(t, ts.handler, "code-octavia", "/", nil)
	if second.Code != http.StatusFound || oauthErrorParam(t, second) != "" {
		t.Fatalf("second login = %d %q, want a clean redirect", second.Code, second.Header().Get("Location"))
	}
	if got := sessionCookieValue(t, second); got == "" {
		t.Error("second login minted no session cookie")
	} else if got == secret {
		t.Error("second login reused the first session secret")
	}
}

func TestOAuthSignupDisabled(t *testing.T) {
	ts := newOAuthTestServer(t, false, map[string]ghStubUser{
		"code-stranger": {ID: 202, Login: "stranger"},
	})

	rec := oauthRoundTrip(t, ts.handler, "code-stranger", "/", nil)
	if got := oauthErrorParam(t, rec); got != "signup_disabled" {
		t.Errorf("auth_error = %q, want signup_disabled", got)
	}
}

func TestOAuthStateReplayRejected(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-octavia": {ID: 101, Login: "octavia"},
	})

	start := doReq(t, ts.handler, http.MethodGet, "/-/auth/github/start?redirect=/", nil, nil)
	if start.Code != http.StatusFound {
		t.Fatalf("start = %d, want 302", start.Code)
	}
	state := mustAuthorizeState(t, start)
	callback := "/-/auth/github/callback?code=code-octavia&state=" + url.QueryEscape(state)

	first := doReq(t, ts.handler, http.MethodGet, callback, nil, nil)
	if first.Code != http.StatusFound {
		t.Fatalf("first callback = %d, want 302", first.Code)
	}

	// replaying the same state finds nothing to consume
	replay := doReq(t, ts.handler, http.MethodGet, callback, nil, nil)
	if got := oauthErrorParam(t, replay); got != "state_invalid" {
		t.Errorf("replayed callback auth_error = %q, want state_invalid", got)
	}
}

func mustAuthorizeState(t *testing.T, start *httptest.ResponseRecorder) string {
	t.Helper()
	u, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse start location: %v", err)
	}
	return u.Query().Get("state")
}

func TestOAuthConnectFlow(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-a": {ID: 101, Login: "octavia"},
		"code-b": {ID: 202, Login: "henry"},
	})

	secretA := sessionCookieValue(t, oauthRoundTrip(t, ts.handler, "code-a", "/", nil))
	secretB := sessionCookieValue(t, oauthRoundTrip(t, ts.handler, "code-b", "/", nil))
	cookiesB := map[string]string{identity.SessionCookie: secretB}

	// henry tries to claim octavia's github account: refused
	rec := oauthRoundTrip(t, ts.handler, "code-a", "/account", cookiesB)
	if got := oauthErrorParam(t, rec); got != "identity_taken" {
		t.Errorf("claiming a linked account auth_error = %q, want identity_taken", got)
	}
	// linking his own account again is idempotent
	rec = oauthRoundTrip(t, ts.handler, "code-b", "/account", cookiesB)
	if got := oauthErrorParam(t, rec); got != "" {
		t.Errorf("own relink auth_error = %q, want none", got)
	}
	// and octavia's session survives the failed claim on her identity
	sess := doReq(t, ts.handler, http.MethodGet, "/-/auth/session",
		map[string]string{"Cookie": cookieHeader(map[string]string{identity.SessionCookie: secretA})}, nil)
	if !strings.Contains(sess.Body.String(), `"username":"octavia"`) {
		t.Errorf("octavia session = %q, want her profile", sess.Body.String())
	}
}

func TestUnlinkLastLoginMethod(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-a": {ID: 101, Login: "octavia"},
	})
	secret := sessionCookieValue(t, oauthRoundTrip(t, ts.handler, "code-a", "/", nil))
	cookies := map[string]string{identity.SessionCookie: secret}

	// octavia has no password: dropping github would lock her out
	rec := doReq(t, ts.handler, http.MethodDelete, "/-/account/identities/github",
		map[string]string{"Cookie": cookieHeader(cookies)}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unlink last method = %d, want 400", rec.Code)
	}

	sess := doReq(t, ts.handler, http.MethodGet, "/-/auth/session",
		map[string]string{"Cookie": cookieHeader(cookies)}, nil)
	if !strings.Contains(sess.Body.String(), `"username":"octavia"`) {
		t.Errorf("session after refused unlink = %q, want octavia still present", sess.Body.String())
	}
}

func TestAccountDeleteFlow(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-a": {ID: 101, Login: "octavia"},
	})
	secret := sessionCookieValue(t, oauthRoundTrip(t, ts.handler, "code-a", "/", nil))
	cookies := map[string]string{identity.SessionCookie: secret}

	// wrong confirmation is refused
	rec := doReq(t, ts.handler, http.MethodDelete, "/-/account",
		map[string]string{"Cookie": cookieHeader(cookies)},
		strings.NewReader(`{"username":"someone-else"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("delete with wrong username = %d, want 400", rec.Code)
	}

	rec = doReq(t, ts.handler, http.MethodDelete, "/-/account",
		map[string]string{"Cookie": cookieHeader(cookies)},
		strings.NewReader(`{"username":"octavia"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}

	// the session died with the account, and the name is free again
	sess := doReq(t, ts.handler, http.MethodGet, "/-/auth/session",
		map[string]string{"Cookie": cookieHeader(cookies)}, nil)
	if sess.Body.String() != `{"user":null}`+"\n" {
		t.Errorf("session after delete = %q, want null", sess.Body.String())
	}
	rec = oauthRoundTrip(t, ts.handler, "code-a", "/", nil)
	if got := oauthErrorParam(t, rec); got != "" {
		t.Errorf("re-signup auth_error = %q, want none", got)
	}
}

func TestWebSessionAnonymous(t *testing.T) {
	ts := newOAuthTestServer(t, true, nil)

	sess := doReq(t, ts.handler, http.MethodGet, "/-/auth/session", nil, nil)
	if sess.Body.String() != `{"user":null}`+"\n" {
		t.Errorf("anonymous session = %q, want null", sess.Body.String())
	}
	if rec := doReq(t, ts.handler, http.MethodGet, "/-/account/logins", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous logins = %d, want 401", rec.Code)
	}
	if rec := doReq(t, ts.handler, http.MethodDelete, "/-/account", nil, strings.NewReader(`{}`)); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous delete = %d, want 401", rec.Code)
	}
	// disabled provider names never reach a consent screen
	rec := doReq(t, ts.handler, http.MethodGet, "/-/auth/gitlab/start", nil, nil)
	if got := oauthErrorParam(t, rec); got != "provider_disabled" {
		t.Errorf("unknown provider auth_error = %q, want provider_disabled", got)
	}
}

func TestOAuthExchangeFailure(t *testing.T) {
	ts := newOAuthTestServer(t, true, map[string]ghStubUser{
		"code-a": {ID: 101, Login: "octavia"},
	})

	rec := oauthRoundTrip(t, ts.handler, "code-unknown", "/", nil)
	if got := oauthErrorParam(t, rec); got != "exchange_failed" {
		t.Errorf("bad code auth_error = %q, want exchange_failed", got)
	}
}

func TestSanitizeRedirect(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "/"},
		{"/account", "/account"},
		{"https://evil.example", "/"},
		{"//evil.example", "/"},
		{"/-/ping", "/"}, // registry side of the proxy is off limits
		{"/account?tab=login", "/account?tab=login"},
	}
	for _, tt := range tests {
		if got := sanitizeRedirect(tt.in); got != tt.want {
			t.Errorf("sanitizeRedirect(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
