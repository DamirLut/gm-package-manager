package identity

import (
	"context"
	"fmt"
	"time"
)

// Event types stored in the events table; they feed both the login history
// and the recent-activity feed of the profile.
const (
	EventLogin            = "login"
	EventIdentityLinked   = "identity_linked"
	EventIdentityUnlinked = "identity_unlinked"
	EventPackagePublish   = "package_publish"
	EventPackageUnpublish = "package_unpublish"
)

// Event is one entry of the profile's feed. Identifiers only — never raw
// tokens or credentials, same rule as the audit log.
type Event struct {
	Type      string    `json:"type"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	defaultEventLimit = 20
	maxEventLimit     = 100
)

// RecordEvent appends to the user's feed. Failures are logged, never fatal
// for the request that caused the event.
func (s *Service) RecordEvent(ctx context.Context, userID int64, typ, ip, ua, provider, detail string) {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO events (user_id, type, ip, user_agent, provider, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, typ, nullIfEmpty(ip), nullIfEmpty(ua), nullIfEmpty(provider), nullIfEmpty(detail), time.Now(),
	); err != nil {
		s.log.Error("identity: record event", "err", err, "user_id", userID, "type", typ)
	}
}

// Logins is the login history: type=login entries, newest first.
func (s *Service) Logins(ctx context.Context, userID int64, limit int) ([]Event, error) {
	return s.listEvents(ctx, userID, EventLogin, limit)
}

// Activity is the recent-activity feed: every event type.
func (s *Service) Activity(ctx context.Context, userID int64, limit int) ([]Event, error) {
	return s.listEvents(ctx, userID, "", limit)
}

func (s *Service) listEvents(ctx context.Context, userID int64, typ string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = defaultEventLimit
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}

	query := `SELECT type, IFNULL(ip, ''), IFNULL(user_agent, ''), IFNULL(provider, ''), IFNULL(detail, ''), created_at
		FROM events WHERE user_id = ?`
	args := []any{userID}
	if typ != "" {
		query += " AND type = ?"
		args = append(args, typ)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("identity: list events: %w", err)
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Type, &e.IP, &e.UserAgent, &e.Provider, &e.Detail, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("identity: list events: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
