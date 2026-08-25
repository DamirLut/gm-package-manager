package router

import (
	"regexp"
	"strings"
)

// Port of the Verdaccio name charset (validation-utils.ts).
var nameRe = regexp.MustCompile(`^[-a-zA-Z0-9_.!~*'()@]+$`)

func validateName(n string) bool {
	n = strings.ToLower(n)
	if !nameRe.MatchString(n) ||
		strings.HasPrefix(n, ".") ||
		n == "node_modules" || n == "__proto__" || n == "favicon.ico" {
		return false
	}
	return true
}

// GameMaker packages must be scoped (@scope/name): gmpm silently skips
// unscoped ones during install.
func validatePkgName(pkg string) bool {
	scope, name, ok := strings.Cut(pkg, "/")
	if !ok || !strings.HasPrefix(scope, "@") {
		return false
	}
	return validateName(strings.TrimPrefix(scope, "@")) && validateName(name)
}

// validDestination allows only clean relative paths like "Release/Plugin".
func validDestination(d string) bool {
	if d == "" || strings.HasPrefix(d, "/") || strings.ContainsRune(d, '\\') {
		return false
	}
	for seg := range strings.SplitSeq(d, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
