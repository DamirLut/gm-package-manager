package auth

import "strings"

const (
	ActionRead    = "read"
	ActionPublish = "publish"
)

// Scope is "<action>:<pattern>" where pattern is "*" (every package),
// "@org/**" (the whole organization) or "@org/name" (a single package).
type Scope struct {
	Action  string
	Pattern string
}

func ParseScopes(raw string) []Scope {
	var out []Scope
	for part := range strings.SplitSeq(raw, ",") {
		action, pattern, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || action == "" || pattern == "" {
			continue
		}
		out = append(out, Scope{Action: action, Pattern: pattern})
	}
	return out
}

// Can reports whether the principal may perform action on pkg.
func (p *Principal) Can(action, pkg string) bool {
	for _, s := range p.Scopes {
		if s.Action == action && scopeMatches(s.Pattern, pkg) {
			return true
		}
	}
	return false
}

// scopeMatches: "*" matches every package, "@org/**" matches any package
// under @org case-insensitively, everything else compares exactly.
func scopeMatches(pattern, pkg string) bool {
	if pattern == "*" {
		return true
	}
	if org, ok := strings.CutSuffix(pattern, "/**"); ok && strings.HasPrefix(org, "@") && len(org) > 1 {
		return strings.HasPrefix(strings.ToLower(pkg), strings.ToLower(org)+"/")
	}
	return pattern == pkg
}
