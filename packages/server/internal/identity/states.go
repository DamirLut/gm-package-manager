package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StateKind distinguishes the two OAuth round trips: signing in through the
// provider versus linking it to the already signed-in user.
type StateKind string

const (
	StateLogin   StateKind = "login"
	StateConnect StateKind = "connect"
)

// State is the payload bound into the one-time CSRF secret.
type State struct {
	Kind     StateKind
	Provider string
	UserID   int64  // StateConnect only: the linking user
	Redirect string // site path to return the browser to
}

func (s *Service) NewState(ctx context.Context, st State) (string, error) {
	secret, hash, err := newSecret()
	if err != nil {
		return "", err
	}
	now := time.Now()
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO oauth_states (token_hash, kind, provider, user_id, redirect, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		hash, string(st.Kind), st.Provider, st.UserID, st.Redirect, now, now.Add(s.cfg.StateTTL),
	); err != nil {
		return "", fmt.Errorf("identity: create state: %w", err)
	}
	return secret, nil
}

// ConsumeState validates and burns a one-time state in a single statement,
// so a replayed callback finds nothing.
func (s *Service) ConsumeState(ctx context.Context, secret string) (*State, error) {
	var st State
	var kind string
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM oauth_states WHERE token_hash = ? AND expires_at > ?
		 RETURNING kind, provider, user_id, redirect`,
		hashSecret(secret), time.Now(),
	).Scan(&kind, &st.Provider, &st.UserID, &st.Redirect)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidState
	}
	if err != nil {
		return nil, fmt.Errorf("identity: consume state: %w", err)
	}
	st.Kind = StateKind(kind)
	return &st, nil
}
