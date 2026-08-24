package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func (db *DB) migrate(ctx context.Context) error {
	start := time.Now()

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("database: migration filesystem: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, migrations)
	if err != nil {
		return fmt.Errorf("database: migration provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("database: apply migrations: %w", err)
	}
	for _, res := range results {
		db.log.Info("migration applied", "path", res.Source.Path, "duration", res.Duration)
	}
	db.log.Info("migrations up to date", "applied", len(results), "took", time.Since(start))
	return nil
}
