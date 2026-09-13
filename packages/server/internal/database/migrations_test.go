package database

import (
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// testMigrationProvider builds the same goose provider migrate() uses, so a
// test can stop after an older migration and seed data into that schema.
func testMigrationProvider(t *testing.T, db *DB) *goose.Provider {
	t.Helper()
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migration filesystem: %v", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrations)
	if err != nil {
		t.Fatalf("migration provider: %v", err)
	}
	return p
}

func TestMigratePreservesUsersAcrossRebuild(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(ctx, Config{Path: path, AutoMigrate: false}, newTestLogger())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	provider := testMigrationProvider(t, db)
	if _, err := provider.UpTo(ctx, 1); err != nil {
		t.Fatalf("apply 00001: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		"INSERT INTO users (username, password_hash, status, created_at, updated_at) VALUES ('alice', 'hash', 'active', ?, ?)", now, now,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO tokens (user_id, token_hash, prefix, scopes, created_at, expires_at, created_ip) VALUES (1, 'h', 'gmpm_x', 'read:*', ?, ?, '127.0.0.1')", now, now,
	); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var hash string
	if err := db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE username = 'alice'").Scan(&hash); err != nil {
		t.Fatalf("alice lost in the users rebuild: %v", err)
	}
	if hash != "hash" {
		t.Errorf("password_hash = %q, want %q", hash, "hash")
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tokens WHERE user_id = 1").Scan(&n); err != nil || n != 1 {
		t.Errorf("tokens survived = %d (err %v), want 1", n, err)
	}

	// the rebuilt users table allows NULL passwords; npm tokens cascade
	// with the account, so account deletion never wedges on foreign keys
	if _, err := db.Exec("INSERT INTO users (username, password_hash, status, created_at, updated_at) VALUES ('gh-only', NULL, 'active', ?, ?)", now, now); err != nil {
		t.Fatalf("NULL password_hash rejected: %v", err)
	}
	if _, err := db.Exec("DELETE FROM users WHERE username = 'alice'"); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tokens WHERE user_id = 1").Scan(&n); err != nil || n != 0 {
		t.Errorf("tokens after cascade = %d (err %v), want 0", n, err)
	}

	for _, table := range []string{"identities", "sessions", "oauth_states", "events"} {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}
}
