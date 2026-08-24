package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

const (
	DefaultPath = "./storage/metadata.db"

	pathEnv        = "DATABASE_PATH"
	autoMigrateEnv = "DATABASE_AUTO_MIGRATE"
)

type Config struct {
	Path        string
	AutoMigrate bool
}

type DB struct {
	*sql.DB
	log *slog.Logger
}

func FromEnv(log *slog.Logger) (*DB, error) {
	cfg := Config{
		Path:        os.Getenv(pathEnv),
		AutoMigrate: true,
	}
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	if v := os.Getenv(autoMigrateEnv); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("database: invalid %s %q, want true or false", autoMigrateEnv, v)
		}
		cfg.AutoMigrate = b
	}

	log.Info("database selected", "path", cfg.Path, "auto_migrate", cfg.AutoMigrate)
	return Open(context.Background(), cfg, log)
}

func Open(ctx context.Context, cfg Config, log *slog.Logger) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("database: path is required")
	}
	if dir := filepath.Dir(cfg.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("database: create directory %q: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_time_format=sqlite", cfg.Path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open %q: %w", cfg.Path, err)
	}
	db := &DB{DB: sqlDB, log: log}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database: ping %q: %w", cfg.Path, err)
	}
	if cfg.AutoMigrate {
		if err := db.migrate(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}
