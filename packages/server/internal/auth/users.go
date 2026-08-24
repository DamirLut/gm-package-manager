package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type userRow struct {
	ID           int64
	Username     string
	PasswordHash string
	Status       string
}

func findUserByUsername(ctx context.Context, db *sql.DB, username string) (*userRow, error) {
	var u userRow
	err := db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, status FROM users WHERE username = ?", username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}
	return &u, nil
}

func createUser(ctx context.Context, db *sql.DB, username, passwordHash string) (int64, error) {
	now := time.Now()
	res, err := db.ExecContext(ctx,
		"INSERT INTO users (username, password_hash, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)",
		username, passwordHash, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("auth: create user: %w", err)
	}
	return res.LastInsertId()
}
