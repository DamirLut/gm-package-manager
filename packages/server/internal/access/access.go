package access

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"server/internal/auth"
)

const (
	GroupAll           = "$all"
	GroupAnonymous     = "$anonymous"
	GroupAuthenticated = "$authenticated"
)

// Rule grants action groups on every package matching Pattern.
type Rule struct {
	Pattern string   `json:"pattern"`
	Access  []string `json:"access"`
	Publish []string `json:"publish"`
}

// Default keeps reads public and requires authentication for publishing.
func Default() []Rule {
	publish := []string{GroupAuthenticated}
	return []Rule{
		{Pattern: "@*/*", Access: []string{GroupAll}, Publish: publish},
		{Pattern: "**", Access: []string{GroupAll}, Publish: publish},
	}
}

// FromEnv loads the rules from the JSON file named by PACKAGES_CONFIG,
// falling back to Default().
func FromEnv(log *slog.Logger) ([]Rule, error) {
	path := os.Getenv("PACKAGES_CONFIG")
	if path == "" {
		log.Info("packages access", "source", "built-in default", "read", "public")
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("access: read %q: %w", path, err)
	}
	var file struct {
		Packages []Rule `json:"packages"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("access: parse %q: %w", path, err)
	}
	if len(file.Packages) == 0 {
		return nil, fmt.Errorf("access: %q defines no package rules", path)
	}
	log.Info("packages access", "source", path, "rules", len(file.Packages))
	return file.Packages, nil
}

// Allow reports whether p may perform action on pkg. The first matching
// rule wins; no match means deny.
func Allow(rules []Rule, action, pkg string, p *auth.Principal) bool {
	for _, r := range rules {
		if !match(r.Pattern, pkg) {
			continue
		}
		groups := r.Access
		if action == auth.ActionPublish {
			groups = r.Publish
		}
		return inGroups(groups, p)
	}
	return false
}

func inGroups(groups []string, p *auth.Principal) bool {
	for _, g := range groups {
		switch g {
		case GroupAll:
			return true
		case GroupAnonymous:
			if p == nil {
				return true
			}
		case GroupAuthenticated:
			if p != nil {
				return true
			}
		default:
			if p != nil && p.Name == g {
				return true
			}
		}
	}
	return false
}

// match supports the glob subset used by the packages section: "**" matches
// everything, "*" spans any characters within one segment. Names compare
// case-insensitively, like scope matching in the auth package.
func match(pattern, pkg string) bool {
	pattern, pkg = strings.ToLower(pattern), strings.ToLower(pkg)
	if pattern == "**" {
		return true
	}
	pp, np := strings.Split(pattern, "/"), strings.Split(pkg, "/")
	if len(pp) != len(np) {
		return false
	}
	for i := range pp {
		if !segmentMatch(pp[i], np[i]) {
			return false
		}
	}
	return true
}

func segmentMatch(pattern, s string) bool {
	if pattern == "" {
		return s == ""
	}
	if pattern[0] == '*' {
		for i := range len(s) + 1 {
			if segmentMatch(pattern[1:], s[i:]) {
				return true
			}
		}
		return false
	}
	return s != "" && pattern[0] == s[0] && segmentMatch(pattern[1:], s[1:])
}
