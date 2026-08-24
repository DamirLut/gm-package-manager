package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type tokenRow struct {
	ID         int64
	UserID     int64
	Scopes     string
	ExpiresAt  time.Time
	RevokedAt  sql.NullTime
	LastUsedAt sql.NullTime
	Username   string
	UserStatus string
}

const tokenSelect = `
	SELECT t.id, t.user_id, t.scopes, t.expires_at, t.revoked_at, t.last_used_at, u.username, u.status
	FROM tokens t
	JOIN users u ON u.id = t.user_id
	WHERE t.token_hash = ?`

func findTokenByHash(ctx context.Context, db *sql.DB, hash string) (*tokenRow, error) {
	var t tokenRow
	err := db.QueryRowContext(ctx, tokenSelect, hash).
		Scan(&t.ID, &t.UserID, &t.Scopes, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.Username, &t.UserStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find token: %w", err)
	}
	return &t, nil
}

func insertToken(ctx context.Context, db *sql.DB, userID int64, typ, hash, prefix, scopes string, createdAt, expiresAt time.Time, ip string) (int64, error) {
	res, err := db.ExecContext(ctx,
		"INSERT INTO tokens (user_id, type, token_hash, prefix, scopes, created_at, expires_at, created_ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		userID, typ, hash, prefix, scopes, createdAt, expiresAt, ip,
	)
	if err != nil {
		return 0, fmt.Errorf("auth: insert token: %w", err)
	}
	return res.LastInsertId()
}

func touchToken(ctx context.Context, db *sql.DB, id int64, at time.Time) error {
	_, err := db.ExecContext(ctx, "UPDATE tokens SET last_used_at = ? WHERE id = ?", at, id)
	if err != nil {
		return fmt.Errorf("auth: touch token: %w", err)
	}
	return nil
}
