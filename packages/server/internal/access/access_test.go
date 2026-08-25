package access

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"server/internal/auth"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, pkg string
		want         bool
	}{
		{"**", "@any/thing", true},
		{"**", "plain", true},
		{"@*/*", "@scope/foo", true},
		{"@*/*", "@SCOPE/FOO", true},
		{"@*/*", "@a/b/c", false},
		{"@*/*", "plain", false},
		{"@myco/**", "@myco/lib", true},
		{"@myco/**", "@other/lib", false},
		{"@myco/**", "@myco/a/b", false},
		{"@myco/lib", "@myco/lib", true},
		{"@myco/lib", "@myco/other", false},
		{"lodash", "lodash", true},
		{"lodash", "lodashx", false},
	}
	for _, tt := range tests {
		if got := match(tt.pattern, tt.pkg); got != tt.want {
			t.Errorf("match(%q, %q) = %v, want %v", tt.pattern, tt.pkg, got, tt.want)
		}
	}
}

var anon *auth.Principal
var john = &auth.Principal{Name: "john"}

func TestAllowGroups(t *testing.T) {
	rules := []Rule{
		{Pattern: "**", Access: []string{GroupAll}, Publish: []string{GroupAuthenticated}},
	}

	if !Allow(rules, "read", "@a/b", anon) || !Allow(rules, "read", "@a/b", john) {
		t.Error("$all must allow anonymous and authenticated")
	}
	if !Allow(rules, "publish", "@a/b", john) {
		t.Error("$authenticated publish must allow a principal")
	}
	if Allow(rules, "publish", "@a/b", anon) {
		t.Error("$authenticated publish must reject anonymous")
	}
}

func TestAllowFirstMatchWins(t *testing.T) {
	rules := []Rule{
		{Pattern: "@secret/**", Access: []string{GroupAuthenticated}},
		{Pattern: "**", Access: []string{GroupAll}},
	}

	if Allow(rules, "read", "@secret/x", anon) {
		t.Error("first matched rule must win over later $all")
	}
	if !Allow(rules, "read", "@secret/x", john) {
		t.Error("authenticated principal must pass the first rule")
	}
	if !Allow(rules, "read", "@open/x", anon) {
		t.Error("unmatched by the first rule falls through to **")
	}
}

func TestAllowAnonymousAndUserGroups(t *testing.T) {
	rules := []Rule{
		{Pattern: "demo", Access: []string{GroupAnonymous}},
		{Pattern: "johns", Access: []string{"john"}},
	}

	if Allow(rules, "read", "demo", john) {
		t.Error("$anonymous must not match an authenticated principal")
	}
	if !Allow(rules, "read", "demo", anon) {
		t.Error("$anonymous must match anonymous")
	}
	if !Allow(rules, "read", "johns", john) {
		t.Error("named user must match")
	}
	if Allow(rules, "read", "johns", &auth.Principal{Name: "bob"}) {
		t.Error("other user must not match")
	}
}

func TestFromEnvDefault(t *testing.T) {
	rules, err := FromEnv(newSilentLogger(t))
	if err != nil {
		t.Fatalf("default FromEnv: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("default rules = %d, want 2", len(rules))
	}
	if !Allow(rules, "read", "@a/b", anon) {
		t.Error("default must keep reads public")
	}
	if Allow(rules, "publish", "@a/b", anon) {
		t.Error("default must require authentication for publish")
	}
}

func TestFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "packages.json")
	cfg := map[string]any{
		"packages": []map[string]any{
			{"pattern": "@secret/**", "access": []string{"$authenticated"}},
			{"pattern": "**", "access": []string{"$all"}, "publish": []string{"$all"}},
		},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PACKAGES_CONFIG", path)

	rules, err := FromEnv(newSilentLogger(t))
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	if !Allow(rules, "publish", "anything", anon) {
		t.Error("config must grant publish to everyone via $all")
	}
	if Allow(rules, "read", "@secret/pkg", anon) {
		t.Error("@secret/** must deny anonymous reads")
	}
}

func TestFromEnvBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packages.json")
	os.WriteFile(path, []byte("{oops"), 0o644)
	t.Setenv("PACKAGES_CONFIG", path)

	if _, err := FromEnv(newSilentLogger(t)); err == nil {
		t.Fatal("broken JSON must fail")
	}
}

func newSilentLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
