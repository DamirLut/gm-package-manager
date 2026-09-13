// Package identity owns the unified profile: external provider accounts,
// website sessions and the per-user event feed. npm bearer tokens stay in
// package auth — this package is the website's way in.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// SessionCookie carries the website session secret.
const SessionCookie = "gmpm_session"

// StatusActive is the only user status allowed to hold sessions.
const StatusActive = "active"

const secretEntropyBytes = 32

const (
	githubClientIDEnv     = "GITHUB_CLIENT_ID"
	githubClientSecretEnv = "GITHUB_CLIENT_SECRET"
)

var (
	ErrInvalidSession   = errors.New("invalid or expired session")
	ErrInvalidState     = errors.New("invalid or expired oauth state")
	ErrProviderDisabled = errors.New("provider is not configured")
	ErrIdentityTaken    = errors.New("provider account is linked to another user")
	ErrProviderLinked   = errors.New("another account of this provider is already linked")
	ErrIdentityNotFound = errors.New("provider account is not linked")
	ErrLastLoginMethod  = errors.New("cannot unlink the last login method")
	ErrSignupFailed     = errors.New("could not pick a free username")
)

// GitHubConfig is non-nil when GitHub login is enabled. The URL fields exist
// so tests can point the provider at a stub server.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string // default https://github.com/login/oauth/authorize
	TokenURL     string // default https://github.com/login/oauth/access_token
	APIBase      string // default https://api.github.com
}

type Config struct {
	SessionTTL time.Duration
	StateTTL   time.Duration
	GitHub     *GitHubConfig // nil disables GitHub login
}

func DefaultConfig() Config {
	return Config{
		SessionTTL: 30 * 24 * time.Hour,
		StateTTL:   10 * time.Minute,
		GitHub: &GitHubConfig{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
			APIBase:  "https://api.github.com",
		},
	}
}

func FromEnv(db *sql.DB, log *slog.Logger) (*Service, error) {
	cfg := DefaultConfig()
	if id := os.Getenv(githubClientIDEnv); id != "" {
		secret := os.Getenv(githubClientSecretEnv)
		if secret == "" {
			return nil, fmt.Errorf("identity: %s is set but %s is empty", githubClientIDEnv, githubClientSecretEnv)
		}
		cfg.GitHub.ClientID, cfg.GitHub.ClientSecret = id, secret
	} else {
		cfg.GitHub = nil
	}
	log.Info("identity selected", "github_login", cfg.GitHub != nil)
	return New(db, cfg, log), nil
}

type Service struct {
	db        *sql.DB
	log       *slog.Logger
	cfg       Config
	providers map[string]Provider
}

func New(db *sql.DB, cfg Config, log *slog.Logger) *Service {
	providers := map[string]Provider{}
	if cfg.GitHub != nil {
		providers["github"] = newGitHubProvider(cfg.GitHub)
	}
	return &Service{db: db, log: log, cfg: cfg, providers: providers}
}

// Provider looks up an OAuth provider; missing means the deployment runs
// without credentials for it and its login must be refused.
func (s *Service) Provider(name string) (Provider, bool) {
	p, ok := s.providers[name]
	return p, ok
}

func (s *Service) SessionTTL() time.Duration { return s.cfg.SessionTTL }

// newSecret mints a URL-safe secret and returns it with its SHA-256 for
// storage, mirroring the npm token scheme in package auth.
func newSecret() (secret, hash string, err error) {
	b := make([]byte, secretEntropyBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("identity: entropy: %w", err)
	}
	secret = hex.EncodeToString(b)
	return secret, hashSecret(secret), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
