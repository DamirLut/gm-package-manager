package database

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOpenAppliesMigrations(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(ctx, Config{Path: path, AutoMigrate: true}, newTestLogger())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var version int64
	err = db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&version)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	if want := int64(1); version != want {
		t.Errorf("max(version_id) = %d, want %d", version, want)
	}
}

func TestOpenIdempotent(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")
	cfg := Config{Path: path, AutoMigrate: true}

	db, err := Open(ctx, cfg, newTestLogger())
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	db.Close()

	db, err = Open(ctx, cfg, newTestLogger())
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id = 1").Scan(&count)
	if err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	if count != 1 {
		t.Errorf("version 1 recorded %d times, want 1", count)
	}
}

func TestOpenAppliesPragmas(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(ctx, Config{Path: path}, newTestLogger())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("pragma journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}

	var foreignKeys int64
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestOpenCreatesNestedDir(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "nested", "dirs", "test.db")

	db, err := Open(ctx, Config{Path: path}, newTestLogger())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
}

func TestOpenWithoutMigrations(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(ctx, Config{Path: path, AutoMigrate: false}, newTestLogger())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var name string
	err = db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'",
	).Scan(&name)
	if err == nil {
		t.Error("goose_db_version exists, want absent when AutoMigrate is false")
	}
}

func TestFromEnv(t *testing.T) {
	newDiscardLog := func() *slog.Logger { return newTestLogger() }

	tests := []struct {
		name    string
		env     map[string]string
		chdir   bool
		wantErr string
		check   func(t *testing.T, db *DB)
	}{
		{
			name:  "defaults",
			chdir: true,
			check: func(t *testing.T, db *DB) {
				var path string
				err := db.QueryRowContext(t.Context(), "SELECT file FROM pragma_database_list() LIMIT 1").Scan(&path)
				if err != nil {
					t.Fatalf("pragma_database_list: %v", err)
				}
				if !strings.HasSuffix(path, filepath.Join("storage", "metadata.db")) {
					t.Errorf("db path = %q, want suffix %q", path, filepath.Join("storage", "metadata.db"))
				}
			},
		},
		{
			name: "custom path",
			env:  map[string]string{"DATABASE_PATH": filepath.Join(t.TempDir(), "custom.db")},
			check: func(t *testing.T, db *DB) {
				var version int64
				err := db.QueryRowContext(t.Context(), "SELECT max(version_id) FROM goose_db_version").Scan(&version)
				if err != nil {
					t.Fatalf("migrations not applied: %v", err)
				}
			},
		},
		{
			name: "auto migrate disabled",
			env: map[string]string{
				"DATABASE_PATH":         filepath.Join(t.TempDir(), "test.db"),
				"DATABASE_AUTO_MIGRATE": "false",
			},
			check: func(t *testing.T, db *DB) {
				var name string
				err := db.QueryRowContext(t.Context(),
					"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'",
				).Scan(&name)
				if err == nil {
					t.Error("goose_db_version exists, want absent")
				}
			},
		},
		{
			name: "invalid auto migrate value",
			env: map[string]string{
				"DATABASE_PATH":         filepath.Join(t.TempDir(), "test.db"),
				"DATABASE_AUTO_MIGRATE": "banana",
			},
			wantErr: `invalid DATABASE_AUTO_MIGRATE "banana"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if tt.chdir {
				t.Chdir(t.TempDir())
			}

			db, err := FromEnv(newDiscardLog())
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("FromEnv() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("FromEnv() error = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("FromEnv() error = %v", err)
			}
			defer db.Close()
			tt.check(t, db)
		})
	}
}
