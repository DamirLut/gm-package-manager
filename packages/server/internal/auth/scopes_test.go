package auth

import "testing"

func principal(scopes string) *Principal {
	return &Principal{Name: "tester", Scopes: ParseScopes(scopes)}
}

func TestCan(t *testing.T) {
	tests := []struct {
		name   string
		scopes string
		action string
		pkg    string
		want   bool
	}{
		{"star covers everything", "read:*", ActionRead, "@any/pkg", true},
		{"wrong action", "read:*", ActionPublish, "@any/pkg", false},
		{"org wildcard", "publish:@myco/**", ActionPublish, "@myco/lib", true},
		{"org wildcard nested", "read:@myco/**", ActionRead, "@MYCO/a/b/c", true},
		{"org wildcard case-insensitive pattern side", "read:@MyCo/**", ActionRead, "@myco/lib", true},
		{"other org denied", "read:@myco/**", ActionRead, "@other/lib", false},
		{"bare org name not matched by wildcard scope", "read:@myco/**", ActionRead, "@myco", false},
		{"exact package", "publish:@myco/lib", ActionPublish, "@myco/lib", true},
		{"exact package mismatch", "publish:@myco/lib", ActionPublish, "@myco/other", false},
		{"exact is case-sensitive", "publish:@myco/lib", ActionPublish, "@MyCo/Lib", false},
		{"multiple scopes any match wins", "read:@a/**,publish:b", ActionPublish, "b", true},
		{"empty scopes deny all", "", ActionRead, "pkg", false},
		{"malformed entries ignored", "::,,junk,publish:", ActionRead, "pkg", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := principal(tt.scopes).Can(tt.action, tt.pkg); got != tt.want {
				t.Fatalf("Can(%q, %q) with scopes %q = %v, want %v",
					tt.action, tt.pkg, tt.scopes, got, tt.want)
			}
		})
	}
}

func TestParseScopes(t *testing.T) {
	got := ParseScopes("read:*, publish:@myco/**")
	if len(got) != 2 ||
		got[0] != (Scope{Action: "read", Pattern: "*"}) ||
		got[1] != (Scope{Action: "publish", Pattern: "@myco/**"}) {
		t.Fatalf("ParseScopes = %+v", got)
	}
}
