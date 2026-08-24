package auth

import "context"

type Principal struct {
	Name    string
	Scopes  []Scope
	TokenID int64 // internal only: never exposed in API responses or logs
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func UserFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*Principal)
	return p, ok
}
