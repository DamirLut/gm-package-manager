package auth

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testConfig() Config {
	return Config{
		AllowSignup: true,
		TokenTTL:    time.Hour,
		DelayBase:   time.Nanosecond,
		DelayCap:    time.Microsecond,
	}
}

func newTestService(t *testing.T, cfg Config) (*Service, *sql.DB) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)&_time_format=sqlite")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
	CREATE TABLE tokens (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id), type TEXT NOT NULL DEFAULT 'user',
		token_hash TEXT NOT NULL UNIQUE, prefix TEXT NOT NULL, scopes TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL, last_used_at TIMESTAMP,
		revoked_at TIMESTAMP, created_ip TEXT NOT NULL)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return New(db, cfg, log), db
}

func login(t *testing.T, s *Service, name, pass string) *IssuedToken {
	t.Helper()
	tok, err := s.Login(t.Context(), name, pass, "10.0.0.1")
	if err != nil {
		t.Fatalf("Login(%q): %v", name, err)
	}
	return tok
}

const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789_"

// checkTokenShape asserts the gmpm token contract: gmpm_<hex(64)> and the
// whole string staying inside the gmpm charset ^[a-zA-Z0-9=_]+$.
func checkTokenShape(t *testing.T, tok *IssuedToken) {
	t.Helper()
	if !strings.HasPrefix(tok.Secret, TokenPrefix) {
		t.Fatalf("token missing %q prefix: %q", TokenPrefix, tok.Secret)
	}
	hexPart := strings.TrimPrefix(tok.Secret, TokenPrefix)
	if len(hexPart) != 64 {
		t.Fatalf("token entropy = %d chars, want 64", len(hexPart))
	}
	for _, r := range tok.Secret {
		if !strings.ContainsRune(tokenAlphabet, r) {
			t.Fatalf("token alphabet violation at %q in %q", r, tok.Secret)
		}
	}
	if strings.TrimPrefix(tok.Prefix, TokenPrefix) != hexPart[:4] {
		t.Errorf("prefix = %q, want first 4 chars of secret", tok.Prefix)
	}
}

func TestLoginRoundtrip(t *testing.T) {
	s, _ := newTestService(t, testConfig())

	tok := login(t, s, "alice", "secret")

	checkTokenShape(t, tok)

	p, err := s.Verify(t.Context(), tok.Secret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p.Name != "alice" {
		t.Errorf("name = %q, want alice", p.Name)
	}
	for _, pkg := range []string{"@scope/foo", "plain"} {
		if !p.Can(ActionRead, pkg) || !p.Can(ActionPublish, pkg) {
			t.Errorf("login token lacks full scopes for %q", pkg)
		}
	}
}

func TestLoginTwiceWithSameCredentials(t *testing.T) {
	s, _ := newTestService(t, testConfig())

	first := login(t, s, "alice", "secret")
	second := login(t, s, "alice", "secret")
	if first.Secret == second.Secret {
		t.Error("two logins produced the same token")
	}
}

func TestWrongPasswordAndUnknownUserSameError(t *testing.T) {
	// Create alice through a signup-enabled service...
	s, db := newTestService(t, testConfig())
	login(t, s, "alice", "secret")

	// ...then exercise failure branches with signup disabled.
	closed := testConfig()
	closed.AllowSignup = false
	noSignup := New(db, closed, slog.New(slog.NewTextHandler(io.Discard, nil)))

	wrongPass, err := noSignup.Login(t.Context(), "alice", "nope", "10.0.0.1")
	if wrongPass != nil || err != ErrInvalidCredentials {
		t.Fatalf("wrong password: (%v, %v), want ErrInvalidCredentials", wrongPass, err)
	}
	unknown, err := noSignup.Login(t.Context(), "ghost", "whatever", "10.0.0.1")
	if unknown != nil || err != ErrInvalidCredentials {
		t.Fatalf("unknown user: (%v, %v), want the same ErrInvalidCredentials", unknown, err)
	}
}

func TestDisableSignupLeavesNoUserRow(t *testing.T) {
	cfg := testConfig()
	cfg.AllowSignup = false
	s, db := newTestService(t, cfg)

	if _, err := s.Login(t.Context(), "ghost", "pw", "10.0.0.1"); err != ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("users table has %d rows, want 0", n)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	s, db := newTestService(t, testConfig())
	tok := login(t, s, "alice", "secret")

	past := time.Now().Add(-time.Minute).UTC()
	if _, err := db.Exec("UPDATE tokens SET expires_at = ? WHERE prefix = ?", past, tok.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(t.Context(), tok.Secret); err != ErrInvalidToken {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestRevokedTokenRejected(t *testing.T) {
	s, db := newTestService(t, testConfig())
	tok := login(t, s, "alice", "secret")

	now := time.Now().UTC()
	if _, err := db.Exec("UPDATE tokens SET revoked_at = ? WHERE prefix = ?", now, tok.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(t.Context(), tok.Secret); err != ErrInvalidToken {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestUnknownTokenRejected(t *testing.T) {
	s, _ := newTestService(t, testConfig())

	for _, tok := range []string{
		TokenPrefix + strings.Repeat("e", 64),
		TokenPrefix + strings.Repeat("f", 63), // wrong length
		TokenPrefix,
		"bearer whatever",
	} {
		if _, err := s.Verify(t.Context(), tok); err != ErrInvalidToken {
			t.Errorf("Verify(%q...) err = %v, want ErrInvalidToken", tok[:12], err)
		}
	}
}

func TestLastUsedThrottled(t *testing.T) {
	s, db := newTestService(t, testConfig())
	tok := login(t, s, "alice", "secret")
	lastUsed := func() time.Time {
		t.Helper()
		var ts time.Time
		if err := db.QueryRow("SELECT last_used_at FROM tokens WHERE prefix = ?", tok.Prefix).Scan(&ts); err != nil {
			t.Fatal(err)
		}
		return ts
	}

	recent := time.Now().Add(-5 * time.Minute).UTC()
	if _, err := db.Exec("UPDATE tokens SET last_used_at = ? WHERE prefix = ?", recent, tok.Prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(t.Context(), tok.Secret); err != nil {
		t.Fatal(err)
	}
	if got := lastUsed(); !got.Equal(recent) {
		t.Errorf("last_used_at bumped within throttle window: %v", got)
	}

	stale := time.Now().Add(-2 * lastUsedInterval).UTC()
	if _, err := db.Exec("UPDATE tokens SET last_used_at = ? WHERE prefix = ?", stale, tok.Prefix); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC()
	if _, err := s.Verify(t.Context(), tok.Secret); err != nil {
		t.Fatal(err)
	}
	if got := lastUsed(); got.Before(before.Add(-time.Second)) {
		t.Errorf("last_used_at not refreshed after window: %v (want >= %v)", got, before)
	}
}

func TestProgressiveDelaySchedule(t *testing.T) {
	cfg := testConfig()
	cfg.DelayBase = 250 * time.Millisecond
	cfg.DelayCap = 8 * time.Second
	s, _ := newTestService(t, cfg)

	k := s.delays.key("alice", "10.0.0.1")
	want := []time.Duration{0, 250 * time.Millisecond, 500 * time.Millisecond,
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second}
	for i, d := range want {
		if got := s.delays.penalty(k); got != d {
			t.Fatalf("penalty after %d failures = %v, want %v", i, got, d)
		}
		s.delays.fail(k)
	}

	s.delays.reset(k)
	if got := s.delays.penalty(k); got != 0 {
		t.Errorf("penalty after reset = %v, want 0", got)
	}
}

func TestDelaySleepRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sleepContext(ctx, time.Hour); err == nil {
		t.Fatal("sleep on cancelled context returned nil")
	}
}

func BenchmarkHashPassword(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := hashPassword("benchmark-password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	h, err := hashPassword("benchmark-password")
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		if !verifyPassword(h, "benchmark-password") {
			b.Fatal("verify failed")
		}
	}
}
