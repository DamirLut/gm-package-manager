package identity

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"server/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(t.Context(), database.Config{
		Path:        filepath.Join(t.TempDir(), "meta.db"),
		AutoMigrate: true,
	}, log)
	if err != nil {
		t.Fatalf("database open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := DefaultConfig()
	cfg.SessionTTL = time.Hour
	cfg.StateTTL = time.Minute
	return New(db.DB, cfg, log)
}

func ghUser(id, login string) *ExternalUser {
	return &ExternalUser{Provider: "github", ID: id, Username: login, AvatarURL: "https://avatar/" + login}
}

// seedPasswordUser creates an old-style password account directly.
func seedPasswordUser(t *testing.T, s *Service, name string) int64 {
	t.Helper()
	now := time.Now()
	res, err := s.db.Exec(
		"INSERT INTO users (username, password_hash, status, created_at, updated_at) VALUES (?, '$argon2id$v=19$m=1,t=1,p=1$c2FsdA$a2V5', 'active', ?, ?)",
		name, now, now,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSessionLifecycle(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	userID, _, err := s.SignupWithIdentity(ctx, ghUser("1", "octocat"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}

	secret, err := s.CreateSession(ctx, userID, "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess, err := s.VerifySession(ctx, secret)
	if err != nil {
		t.Fatalf("verify session: %v", err)
	}
	if sess.UserID != userID || sess.Username != "octocat" {
		t.Errorf("session = %+v, want user octocat", sess)
	}

	s.RevokeSession(ctx, secret)
	if _, err := s.VerifySession(ctx, secret); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("revoked session verify = %v, want ErrInvalidSession", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	userID, _, err := s.SignupWithIdentity(ctx, ghUser("1", "octocat"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	secret, err := s.CreateSession(ctx, userID, "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.db.Exec("UPDATE sessions SET expires_at = ?", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VerifySession(ctx, secret); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expired session verify = %v, want ErrInvalidSession", err)
	}
}

func TestStateOneTime(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)

	secret, err := s.NewState(ctx, State{Kind: StateLogin, Provider: "github", Redirect: "/account"})
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	st, err := s.ConsumeState(ctx, secret)
	if err != nil {
		t.Fatalf("consume state: %v", err)
	}
	if st.Kind != StateLogin || st.Provider != "github" || st.Redirect != "/account" {
		t.Errorf("state = %+v", st)
	}
	if _, err := s.ConsumeState(ctx, secret); !errors.Is(err, ErrInvalidState) {
		t.Errorf("replayed state = %v, want ErrInvalidState", err)
	}

	expired, err := s.NewState(ctx, State{Kind: StateLogin, Provider: "github"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE oauth_states SET expires_at = ?", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeState(ctx, expired); !errors.Is(err, ErrInvalidState) {
		t.Errorf("expired state = %v, want ErrInvalidState", err)
	}
}

func TestSignupUsernameDeduplication(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	seedPasswordUser(t, s, "octocat")

	// the taken name pushes the first signup to the -gh suffix
	id1, name1, err := s.SignupWithIdentity(ctx, ghUser("1", "octocat"))
	if err != nil {
		t.Fatalf("signup 1: %v", err)
	}
	if name1 != "octocat-gh" {
		t.Errorf("name1 = %q, want octocat-gh", name1)
	}
	id2, name2, err := s.SignupWithIdentity(ctx, ghUser("2", "octocat"))
	if err != nil {
		t.Fatalf("signup 2: %v", err)
	}
	if name2 != "octocat-gh2" || id2 == id1 {
		t.Errorf("name2 = %q id2 = %d, want octocat-gh and a fresh id", name2, id2)
	}

	// a free name is used verbatim
	if _, name3, err := s.SignupWithIdentity(ctx, ghUser("3", "octavia")); err != nil || name3 != "octavia" {
		t.Errorf("name3 = %q err = %v, want octavia", name3, err)
	}
}

func TestLinkIdentity(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	ownerID, _, err := s.SignupWithIdentity(ctx, ghUser("10", "octavia"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	otherID := seedPasswordUser(t, s, "alice")

	// the same external account must not join a second profile
	if err := s.LinkIdentity(ctx, otherID, ghUser("10", "octavia")); !errors.Is(err, ErrIdentityTaken) {
		t.Errorf("link to another user = %v, want ErrIdentityTaken", err)
	}
	// re-linking to its own profile is idempotent
	if err := s.LinkIdentity(ctx, ownerID, ghUser("10", "octavia")); err != nil {
		t.Errorf("idempotent relink = %v, want nil", err)
	}
	// one account per provider per profile
	if err := s.LinkIdentity(ctx, ownerID, ghUser("11", "octavia2")); !errors.Is(err, ErrProviderLinked) {
		t.Errorf("second github link = %v, want ErrProviderLinked", err)
	}
}

func TestUnlinkGuard(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	oauthID, _, err := s.SignupWithIdentity(ctx, ghUser("10", "octavia"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	pwID := seedPasswordUser(t, s, "alice")
	if err := s.LinkIdentity(ctx, pwID, ghUser("11", "octavia2")); err != nil {
		t.Fatalf("link: %v", err)
	}

	// a password user may lose the provider...
	if err := s.UnlinkIdentity(ctx, pwID, "github"); err != nil {
		t.Errorf("unlink from password user = %v, want nil", err)
	}
	// ...an OAuth-only one may not: it would be left with no way in
	if err := s.UnlinkIdentity(ctx, oauthID, "github"); !errors.Is(err, ErrLastLoginMethod) {
		t.Errorf("unlink last method = %v, want ErrLastLoginMethod", err)
	}
	// an unknown provider identity is a clean miss
	plain := seedPasswordUser(t, s, "bob")
	if err := s.UnlinkIdentity(ctx, plain, "gitlab"); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("unlink unlinked = %v, want ErrIdentityNotFound", err)
	}
}

func TestDeleteAccountCascades(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	userID, name, err := s.SignupWithIdentity(ctx, ghUser("10", "octavia"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	secret, err := s.CreateSession(ctx, userID, "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	s.RecordEvent(ctx, userID, EventLogin, "10.0.0.1", "test-agent", "github", "")

	if err := s.DeleteAccount(ctx, userID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if _, err := s.VerifySession(ctx, secret); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("session after delete = %v, want ErrInvalidSession", err)
	}
	if _, err := s.Profile(ctx, userID); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("profile after delete = %v, want ErrInvalidSession", err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM events WHERE user_id = ?", userID,
	).Scan(&n); err != nil || n != 0 {
		t.Errorf("events after delete = %d (err %v), want 0", n, err)
	}
	_ = name
}

func TestRecordAndListEvents(t *testing.T) {
	ctx := t.Context()
	s := newTestService(t)
	userID, _, err := s.SignupWithIdentity(ctx, ghUser("10", "octavia"))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	s.RecordEvent(ctx, userID, EventLogin, "1.1.1.1", "ua", "github", "")
	s.RecordEvent(ctx, userID, EventPackagePublish, "1.1.1.1", "ua", "", "pkg@1.0.0")
	s.RecordEvent(ctx, userID, EventIdentityLinked, "1.1.1.1", "ua", "github", "octavia")

	logins, err := s.Logins(ctx, userID, 0)
	if err != nil {
		t.Fatalf("logins: %v", err)
	}
	if len(logins) != 1 || logins[0].Provider != "github" {
		t.Errorf("logins = %+v, want one github login", logins)
	}
	activity, err := s.Activity(ctx, userID, 0)
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	if len(activity) != 3 {
		t.Errorf("activity len = %d, want 3", len(activity))
	}
	if activity[0].Type != EventIdentityLinked {
		t.Errorf("newest activity = %q, want %q", activity[0].Type, EventIdentityLinked)
	}
}

func TestFromEnvGitHubConfig(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(ctx, database.Config{Path: filepath.Join(dir, "meta.db"), AutoMigrate: true}, log)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t.Setenv("GITHUB_CLIENT_ID", "id")
	t.Setenv("GITHUB_CLIENT_SECRET", "secret")
	svc, err := FromEnv(db.DB, log)
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if _, ok := svc.Provider("github"); !ok {
		t.Error("github provider missing with credentials set")
	}

	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	svc, err = FromEnv(db.DB, log)
	if err != nil {
		t.Fatalf("FromEnv without credentials: %v", err)
	}
	if _, ok := svc.Provider("github"); ok {
		t.Error("github provider present without credentials")
	}

	t.Setenv("GITHUB_CLIENT_ID", "id-only")
	if _, err := FromEnv(db.DB, log); err == nil {
		t.Error("FromEnv with client id but no secret = nil error, want error")
	}
}
