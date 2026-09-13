package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const signupNameLimit = 60

// Identity is one linked provider account as exposed by the API.
type Identity struct {
	Provider  string    `json:"provider"`
	Username  string    `json:"username"`
	Email     string    `json:"email,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Profile is the unified account behind the website session.
type Profile struct {
	Username   string     `json:"username"`
	CreatedAt  time.Time  `json:"created_at"`
	AvatarURL  string     `json:"avatar_url,omitempty"`
	Identities []Identity `json:"identities"`
}

// LinkedUser is the profile owning a provider account; Status lets the
// caller decide what a suspended owner means.
type LinkedUser struct {
	ID       int64
	Username string
	Status   string
}

// Profile assembles the account page payload: profile fields plus every
// linked provider account.
func (s *Service) Profile(ctx context.Context, userID int64) (*Profile, error) {
	var p Profile
	err := s.db.QueryRowContext(ctx,
		"SELECT username, created_at FROM users WHERE id = ?", userID,
	).Scan(&p.Username, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("identity: profile: %w", err)
	}
	if p.Identities, err = s.Identities(ctx, userID); err != nil {
		return nil, err
	}
	for _, id := range p.Identities {
		if id.AvatarURL != "" {
			p.AvatarURL = id.AvatarURL
			break
		}
	}
	return &p, nil
}

func (s *Service) Identities(ctx context.Context, userID int64) ([]Identity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, username, IFNULL(email, ''), IFNULL(avatar_url, ''), created_at
		 FROM identities WHERE user_id = ? ORDER BY created_at, id`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("identity: list identities: %w", err)
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var id Identity
		if err := rows.Scan(&id.Provider, &id.Username, &id.Email, &id.AvatarURL, &id.CreatedAt); err != nil {
			return nil, fmt.Errorf("identity: list identities: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LinkedUser resolves the provider account into its owner.
func (s *Service) LinkedUser(ctx context.Context, provider, providerAccountID string) (*LinkedUser, error) {
	var u LinkedUser
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.status
		 FROM identities i JOIN users u ON u.id = i.user_id
		 WHERE i.provider = ? AND i.provider_account_id = ?`,
		provider, providerAccountID,
	).Scan(&u.ID, &u.Username, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("identity: linked user: %w", err)
	}
	return &u, nil
}

// LinkIdentity attaches an external account to the user. Already-linked
// accounts stay untouched (idempotent) unless owned by someone else.
func (s *Service) LinkIdentity(ctx context.Context, userID int64, u *ExternalUser) error {
	owner, err := s.LinkedUser(ctx, u.Provider, u.ID)
	switch {
	case errors.Is(err, ErrIdentityNotFound):
		// not linked yet: fall through to the checks below
	case err != nil:
		return err
	default:
		if owner.ID == userID {
			return nil
		}
		return ErrIdentityTaken
	}

	has, err := s.hasProvider(ctx, userID, u.Provider)
	if err != nil {
		return err
	}
	if has {
		return ErrProviderLinked
	}
	return s.insertIdentity(ctx, userID, u)
}

// UnlinkIdentity detaches the provider. It refuses to remove the last login
// method: an account with neither a password nor a provider cannot be
// entered anymore.
func (s *Service) UnlinkIdentity(ctx context.Context, userID int64, provider string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("identity: unlink: %w", err)
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM identities WHERE user_id = ?", userID,
	).Scan(&n); err != nil {
		return fmt.Errorf("identity: unlink: %w", err)
	}
	if n == 1 {
		var hash sql.NullString
		if err := tx.QueryRowContext(ctx,
			"SELECT password_hash FROM users WHERE id = ?", userID,
		).Scan(&hash); err != nil {
			return fmt.Errorf("identity: unlink: %w", err)
		}
		if !hash.Valid {
			return ErrLastLoginMethod
		}
	}
	res, err := tx.ExecContext(ctx,
		"DELETE FROM identities WHERE user_id = ? AND provider = ?", userID, provider,
	)
	if err != nil {
		return fmt.Errorf("identity: unlink: %w", err)
	}
	if affected, err := res.RowsAffected(); err != nil || affected == 0 {
		return ErrIdentityNotFound
	}
	return tx.Commit()
}

// SignupWithIdentity creates the profile for a first-time provider account.
// The provider login seeds the username; collisions resolve with a -gh
// suffix, so external accounts never merge into existing ones.
func (s *Service) SignupWithIdentity(ctx context.Context, u *ExternalUser) (userID int64, username string, err error) {
	base := u.Username
	if len(base) > signupNameLimit {
		base = base[:signupNameLimit]
	}
	if base == "" {
		base = "user"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", fmt.Errorf("identity: signup: %w", err)
	}
	defer tx.Rollback()

	for attempt := 0; attempt < 25; attempt++ {
		name := base
		switch {
		case attempt == 1:
			name += "-gh"
		case attempt > 1:
			name = fmt.Sprintf("%s-gh%d", base, attempt)
		}
		var one int
		err = tx.QueryRowContext(ctx, "SELECT 1 FROM users WHERE username = ?", name).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			now := time.Now()
			res, err := tx.ExecContext(ctx,
				"INSERT INTO users (username, password_hash, status, created_at, updated_at) VALUES (?, NULL, ?, ?, ?)",
				name, StatusActive, now, now,
			)
			if err != nil {
				return 0, "", fmt.Errorf("identity: signup: %w", err)
			}
			userID, err = res.LastInsertId()
			if err != nil {
				return 0, "", fmt.Errorf("identity: signup: %w", err)
			}
			username = name
			break
		}
		if err != nil {
			return 0, "", fmt.Errorf("identity: signup: %w", err)
		}
	}
	if userID == 0 {
		return 0, "", ErrSignupFailed
	}

	if err := insertIdentityTx(ctx, tx, userID, u); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("identity: signup: %w", err)
	}
	return userID, username, nil
}

// DeleteAccount removes the profile; identities, sessions, npm tokens and
// events go with it through ON DELETE CASCADE. Published packages stay, as
// in npm.
func (s *Service) DeleteAccount(ctx context.Context, userID int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return fmt.Errorf("identity: delete account: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return ErrInvalidSession
	}
	return nil
}

func (s *Service) hasProvider(ctx context.Context, userID int64, provider string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM identities WHERE user_id = ? AND provider = ?", userID, provider,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("identity: find provider: %w", err)
	}
	return true, nil
}

func (s *Service) insertIdentity(ctx context.Context, userID int64, u *ExternalUser) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("identity: link identity: %w", err)
	}
	defer tx.Rollback()
	if err := insertIdentityTx(ctx, tx, userID, u); err != nil {
		return err
	}
	return tx.Commit()
}

func insertIdentityTx(ctx context.Context, tx *sql.Tx, userID int64, u *ExternalUser) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx,
		`INSERT INTO identities (user_id, provider, provider_account_id, username, email, avatar_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, u.Provider, u.ID, u.Username, nullIfEmpty(u.Email), nullIfEmpty(u.AvatarURL), now, now,
	)
	if err != nil {
		return fmt.Errorf("identity: link identity: %w", err)
	}
	return nil
}
