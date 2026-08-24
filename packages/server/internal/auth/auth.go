package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	TokenPrefix = "gmpm_"

	tokenEntropyBytes = 32
	lastUsedInterval  = time.Hour

	userStatusActive = "active"
	tokenTypeUser    = "user"

	signupEnv = "DISABLE_SIGNUP"
)

// Login tokens carry the owner's full set of scopes.
var userScopes = ActionRead + ":*," + ActionPublish + ":*"

var (
	ErrInvalidCredentials = errors.New("bad username/password, access denied")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type Config struct {
	AllowSignup bool          // DISABLE_SIGNUP=false (default): first login creates the account
	TokenTTL    time.Duration // default 90d
	DelayBase   time.Duration // progressive delay base, 250ms
	DelayCap    time.Duration // progressive delay cap, ~8s
}

func DefaultConfig() Config {
	return Config{
		AllowSignup: true,
		TokenTTL:    90 * 24 * time.Hour,
		DelayBase:   250 * time.Millisecond,
		DelayCap:    8 * time.Second,
	}
}

func FromEnv(db *sql.DB, log *slog.Logger) (*Service, error) {
	cfg := DefaultConfig()
	if v := os.Getenv(signupEnv); v != "" {
		disabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("auth: invalid %s %q, want true or false", signupEnv, v)
		}
		cfg.AllowSignup = !disabled
	}
	log.Info("auth selected", "allow_signup", cfg.AllowSignup)
	return New(db, cfg, log), nil
}

type Service struct {
	db     *sql.DB
	log    *slog.Logger
	cfg    Config
	delays *limiter
}

func New(db *sql.DB, cfg Config, log *slog.Logger) *Service {
	return &Service{db: db, log: log, cfg: cfg, delays: newLimiter(cfg.DelayBase, cfg.DelayCap)}
}

// IssuedToken is a freshly minted token. Secret is shown to the client once;
// Prefix is safe for logs and UI.
type IssuedToken struct {
	Secret string
	Prefix string
}

func (s *Service) Login(ctx context.Context, name, pass, ip string) (*IssuedToken, error) {
	if name == "" || pass == "" {
		return nil, ErrInvalidCredentials
	}

	k := s.delays.key(name, ip)
	if err := sleepContext(ctx, s.delays.penalty(k)); err != nil {
		return nil, err
	}

	u, err := findUserByUsername(ctx, s.db, name)
	if err != nil {
		return nil, err
	}

	switch {
	case u != nil:
		if !verifyPassword(u.PasswordHash, pass) {
			s.delays.fail(k)
			return nil, ErrInvalidCredentials
		}
	case s.cfg.AllowSignup:
		verifyPassword(dummyHash(), pass) // keep timing flat across the branches
		hash, err := hashPassword(pass)
		if err != nil {
			return nil, err
		}
		id, err := createUser(ctx, s.db, name, hash)
		if err != nil {
			return nil, err
		}
		u = &userRow{ID: id, Username: name, Status: userStatusActive}
		s.log.Info("user created", "username", name, "ip", ip)
	default:
		verifyPassword(dummyHash(), pass)
		s.delays.fail(k)
		return nil, ErrInvalidCredentials
	}

	tok, err := s.issueToken(ctx, u.ID, ip)
	if err != nil {
		return nil, err
	}
	s.delays.reset(k)
	s.log.Info("login ok", "username", name, "ip", ip, "token_prefix", tok.Prefix)
	return tok, nil
}

// Verify resolves a presented token into a Principal. Numeric ids never
// leave this package's callers: they are for internal bookkeeping only.
func (s *Service) Verify(ctx context.Context, token string) (*Principal, error) {
	if len(token) != len(TokenPrefix)+2*tokenEntropyBytes {
		return nil, ErrInvalidToken
	}
	sum := sha256.Sum256([]byte(token))
	row, err := findTokenByHash(ctx, s.db, hex.EncodeToString(sum[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if row.UserStatus != userStatusActive || row.RevokedAt.Valid || !row.ExpiresAt.After(now) {
		return nil, ErrInvalidToken
	}

	// last_used_at is refreshed at most once an hour per token.
	if !row.LastUsedAt.Valid || now.Sub(row.LastUsedAt.Time) >= lastUsedInterval {
		_ = touchToken(ctx, s.db, row.ID, now)
	}

	return &Principal{
		Name:    row.Username,
		Scopes:  ParseScopes(row.Scopes),
		TokenID: row.ID,
	}, nil
}

func (s *Service) issueToken(ctx context.Context, userID int64, ip string) (*IssuedToken, error) {
	secret, err := newTokenSecret()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(secret))

	prefix := secret[:len(TokenPrefix)+4]
	now := time.Now()
	if _, err := insertToken(ctx, s.db, userID, tokenTypeUser,
		hex.EncodeToString(sum[:]), prefix, userScopes,
		now, now.Add(s.cfg.TokenTTL), ip,
	); err != nil {
		return nil, err
	}
	return &IssuedToken{Secret: secret, Prefix: prefix}, nil
}

// gmpm_<hex(32B crypto/rand)> — alphabet [0-9a-f_] fits the gmpm charset
// constraint ^[a-zA-Z0-9=_]+$.
func newTokenSecret() (string, error) {
	b := make([]byte, tokenEntropyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: token entropy: %w", err)
	}
	return TokenPrefix + hex.EncodeToString(b), nil
}
