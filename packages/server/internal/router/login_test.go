package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"server/internal/auth"
)

const loginPath = "/-/user/org.couchdb.user:alice"

func loginBody(name, password string) string {
	return `{"name":"` + name + `","password":"` + password + `"}`
}

func TestLoginContract(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest(http.MethodPut, loginPath, nil)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache, no-store" {
		t.Errorf("cache-control = %q, want %q", cc, "no-cache, no-store")
	}

	var got struct {
		OK    string `json:"ok"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
	}
	if want := "you are authenticated as 'alice'"; got.OK != want {
		t.Errorf("ok = %q, want %q", got.OK, want)
	}
	secret, ok := strings.CutPrefix(got.Token, "gmpm_")
	if !ok || len(secret) != 64 {
		t.Fatalf("token shape wrong: %q", got.Token)
	}
	for _, r := range secret {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("token entropy not hex: %q", got.Token)
		}
	}
	// full token stays inside the gmpm charset ^[a-zA-Z0-9=_]+$
	for _, r := range got.Token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '=', r == '_':
		default:
			t.Errorf("token contains char outside [a-zA-Z0-9=_]: %q", got.Token)
		}
	}

	p, err := ts.auth.Verify(t.Context(), got.Token)
	if err != nil {
		t.Fatalf("issued token does not verify: %v", err)
	}
	if p.Name != "alice" {
		t.Errorf("principal name = %q, want alice", p.Name)
	}
	if !p.Can(auth.ActionRead, "@any/pkg") || !p.Can(auth.ActionPublish, "@any/pkg") {
		t.Error("login token must carry full read/publish scopes")
	}
}

func TestLoginFromBodyPassword(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest(http.MethodPut, "/-/user/org.couchdb.user:bob",
		strings.NewReader(loginBody("bob", "pw")))
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestLoginBadTail(t *testing.T) {
	ts := newTestServer(t)

	for _, target := range []string{
		"/-/user/wrong:alice",
		"/-/user/org.couchdb.user:",
		"/-/user/alice",
	} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, target, nil)
			req.SetBasicAuth("alice", "secret")
			rec := httptest.NewRecorder()
			ts.handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestLoginMissingPassword(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest(http.MethodPut, loginPath, strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	ts := newTestServer(t)

	req := httptest.NewRequest(http.MethodPut, loginPath, nil)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: status = %d, body %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, loginPath, nil)
	req.SetBasicAuth("alice", "wrong")
	rec = httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if wa := rec.Header().Get("WWW-Authenticate"); wa != "Bearer" {
		t.Errorf("www-authenticate = %q, want Bearer", wa)
	}
	want := `{"error":"bad username/password, access denied"}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestLoginUnknownUserSignupDisabled(t *testing.T) {
	ts := newTestServerWithAuth(t, auth.Config{
		AllowSignup: false,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	})

	req := httptest.NewRequest(http.MethodPut, loginPath, nil)
	req.SetBasicAuth("stranger", "pw")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if want := "bad username/password, access denied"; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %q, want contains %q", rec.Body.String(), want)
	}
}

func TestLoginWithStaleBearerAttached(t *testing.T) {
	ts := newTestServer(t)

	stale := strings.Repeat("a", 69) // right length, unknown token
	req := httptest.NewRequest(http.MethodPut, loginPath, nil)
	req.Header.Set("Authorization", "Bearer "+stale)
	req.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (stale token must not block bootstrap), body %s",
			rec.Code, rec.Body.String())
	}
}

func TestAuditEventsWritten(t *testing.T) {
	ts := newTestServer(t)

	ok := httptest.NewRequest(http.MethodPut, loginPath, nil)
	ok.SetBasicAuth("alice", "secret")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, ok)

	bad := httptest.NewRequest(http.MethodPut, loginPath, nil)
	bad.SetBasicAuth("alice", "wrong")
	rec = httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, bad)

	data, err := os.ReadFile(ts.auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	joined := string(data)
	for _, action := range []string{"auth.login.success", "auth.login.failed"} {
		if !strings.Contains(joined, `"action":"`+action+`"`) {
			t.Errorf("audit log missing event %q:\n%s", action, joined)
		}
	}
	if strings.Contains(joined, "secret") {
		t.Error("audit log leaked a password")
	}
}

func TestBearerOnPkgRoutes(t *testing.T) {
	ts := newTestServer(t)
	if err := ts.store.PutManifest(t.Context(), "@scope/foo", []byte(`{"name":"@scope/foo"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	loginReq := httptest.NewRequest(http.MethodPut, "/-/user/org.couchdb.user:carol", nil)
	loginReq.SetBasicAuth("carol", "pw")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, loginReq)
	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("no token from login")
	}

	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{
			name:    "anonymous read stays open",
			headers: nil,
			want:    http.StatusOK,
		},
		{
			name:    "valid bearer",
			headers: map[string]string{"Authorization": "Bearer " + resp.Token},
			want:    http.StatusOK,
		},
		{
			name:    "unknown token",
			headers: map[string]string{"Authorization": "Bearer gmpm_" + strings.Repeat("e", 64)},
			want:    http.StatusUnauthorized,
		},
		{
			name:    "malformed bearer",
			headers: map[string]string{"Authorization": "Bearer"},
			want:    http.StatusUnauthorized,
		},
		{
			name:    "basic outside login is ignored",
			headers: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			want:    http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doReq(t, ts.handler, http.MethodGet, "/@scope/foo", tt.headers, nil)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusUnauthorized {
				if wa := rec.Header().Get("WWW-Authenticate"); wa != "Bearer" {
					t.Errorf("www-authenticate = %q, want Bearer", wa)
				}
			}
		})
	}
}
