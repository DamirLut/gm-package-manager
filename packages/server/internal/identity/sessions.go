package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Session is the authenticated website visitor resolved from the cookie.
type Session struct {
	ID       int64
	UserID   int64
	Username string
}

type sessionRow struct {
	ID        int64
	UserID    int64
	Username  string
	Status    string
	ExpiresAt time.Time
}

// CreateSession mints the cookie secret; the caller sets it with
// SetSessionCookie after every other step of the login succeeded.
func (s *Service) CreateSession(ctx context.Context, userID int64, ip, ua string) (string, error) {
	secret, hash, err := newSecret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (user_id, token_hash, created_ip, user_agent, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		userID, hash, ip, ua, now, now.Add(s.cfg.SessionTTL),
	); err != nil {
		return "", fmt.Errorf("identity: create session: %w", err)
	}
	return secret, nil
}

func (s *Service) VerifySession(ctx context.Context, secret string) (*Session, error) {
	if len(secret) != 2*secretEntropyBytes {
		return nil, ErrInvalidSession
	}
	var row sessionRow
	err := s.db.QueryRowContext(ctx,
		`SELECT se.id, se.user_id, u.username, u.status, se.expires_at
		 FROM sessions se JOIN users u ON u.id = se.user_id
		 WHERE se.token_hash = ?`, hashSecret(secret),
	).Scan(&row.ID, &row.UserID, &row.Username, &row.Status, &row.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("identity: find session: %w", err)
	}
	if row.Status != StatusActive || !row.ExpiresAt.After(time.Now()) {
		return nil, ErrInvalidSession
	}
	return &Session{ID: row.ID, UserID: row.UserID, Username: row.Username}, nil
}

func (s *Service) RevokeSession(ctx context.Context, secret string) {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", hashSecret(secret)); err != nil {
		s.log.Error("identity: revoke session", "err", err)
	}
}

// SetSessionCookie stores the session secret; SameSite=Lax doubles as the
// CSRF defense for the account API's POST/DELETE endpoints.
func (s *Service) SetSessionCookie(w http.ResponseWriter, r *http.Request, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    secret,
		Path:     "/",
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   requestSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// ReadSessionCookie returns the raw cookie secret, "" when absent.
func ReadSessionCookie(r *http.Request) string {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// requestSecure mirrors tarballURL: behind nginx the scheme arrives in
// X-Forwarded-Proto.
func requestSecure(r *http.Request) bool {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto == "https"
	}
	return r.TLS != nil
}
