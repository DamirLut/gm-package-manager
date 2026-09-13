package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"
)

// Provider is one external login method. Adding gitlab or google later is a
// new file implementing this interface plus a line in identity.New.
type Provider interface {
	Name() string
	// AuthCodeURL builds the consent screen URL; state is the one-time
	// CSRF secret minted by the service.
	AuthCodeURL(redirectURI, state string) string
	// Exchange trades the callback code for the external profile.
	Exchange(ctx context.Context, code, redirectURI string) (*ExternalUser, error)
}

// ExternalUser is the provider-side account as returned by Exchange.
type ExternalUser struct {
	Provider  string
	ID        string
	Username  string
	Email     string
	AvatarURL string
}

type githubProvider struct {
	cfg *GitHubConfig
}

func newGitHubProvider(cfg *GitHubConfig) *githubProvider { return &githubProvider{cfg: cfg} }

func (p *githubProvider) Name() string { return "github" }

// No scopes requested: the registry needs only the public profile.
func (p *githubProvider) oauth2Config(redirectURI string) oauth2.Config {
	return oauth2.Config{
		ClientID:     p.cfg.ClientID,
		ClientSecret: p.cfg.ClientSecret,
		RedirectURL:  redirectURI,
		Endpoint: oauth2.Endpoint{
			AuthURL:  p.cfg.AuthURL,
			TokenURL: p.cfg.TokenURL,
		},
	}
}

func (p *githubProvider) AuthCodeURL(redirectURI, state string) string {
	cfg := p.oauth2Config(redirectURI)
	return cfg.AuthCodeURL(state)
}

func (p *githubProvider) Exchange(ctx context.Context, code, redirectURI string) (*ExternalUser, error) {
	cfg := p.oauth2Config(redirectURI)
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("identity: github code exchange: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.APIBase+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("identity: github user request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(tok))
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("identity: github user fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity: github user fetch: status %d", resp.StatusCode)
	}

	var body struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("identity: github user decode: %w", err)
	}
	if body.ID == 0 || body.Login == "" {
		return nil, fmt.Errorf("identity: github user fetch: empty profile")
	}
	return &ExternalUser{
		Provider:  p.Name(),
		ID:        strconv.FormatInt(body.ID, 10),
		Username:  body.Login,
		Email:     body.Email,
		AvatarURL: body.AvatarURL,
	}, nil
}
