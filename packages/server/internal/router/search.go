package router

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"server/internal/storage"
)

// GET /-/verdaccio/data/search?q=... — relevant package search for the
// website autocomplete. The latest version of every package is matched
// against name, gm.displayName, gm.shortDescription, description, keywords
// and author; identifiers weigh more than prose and exact hits beat
// prefixes which beat plain containment. Top hits are returned in the same
// shape as the packages list.
func handleSearch(store storage.Storage, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens := tokenize(r.URL.Query().Get("q"))
		if len(tokens) == 0 {
			WriteJSON(w, http.StatusOK, []any{})
			return
		}

		names, err := store.ListPackages(r.Context())
		if err != nil {
			WriteError(w, ErrInternal)
			return
		}

		type hit struct {
			info  map[string]any
			score int
		}
		hits := make([]hit, 0, len(names))
		for _, name := range names {
			info, err := latestVersion(r.Context(), store, name)
			if err != nil {
				log.Warn("search: skipping broken entry", "package", name, "error", err)
				continue
			}
			if info == nil {
				continue
			}
			if score := scorePackage(name, info, tokens); score > 0 {
				hits = append(hits, hit{info: info, score: score})
			}
		}

		slices.SortFunc(hits, func(a, b hit) int {
			if a.score != b.score {
				return b.score - a.score
			}
			return strings.Compare(pkgName(a.info), pkgName(b.info))
		})

		const limit = 10
		if len(hits) > limit {
			hits = hits[:limit]
		}
		out := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			out = append(out, h.info)
		}
		WriteJSON(w, http.StatusOK, out)
	}
}

// normalize folds a searchable value the same way queries are folded:
// lowercase, punctuation (hyphens, underscores, dots) becomes spaces, so
// "debugger-dump" and "Debugger Dump" match each other.
func normalize(s string) string {
	repl := strings.NewReplacer("-", " ", "_", " ", ".", " ")
	return repl.Replace(strings.ToLower(s))
}

// tokenize splits a query into normalized, deduplicated tokens.
func tokenize(query string) []string {
	seen := make(map[string]bool)
	var tokens []string
	for _, tok := range strings.Fields(normalize(query)) {
		if !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, tok)
		}
	}
	return tokens
}

// matchScore ranks how well a token matches normalized text:
// exact > prefix > contained; 0 means no match.
func matchScore(text, token string) int {
	if text == token {
		return 100
	}
	if strings.HasPrefix(text, token) {
		return 60
	}
	if strings.Contains(text, token) {
		return 30
	}
	return 0
}

type searchFields struct {
	name      string
	display   string
	shortDesc string
	desc      string
	author    string
	keywords  []string
}

func collectSearchFields(name string, info map[string]any) searchFields {
	f := searchFields{name: name}
	if gm, ok := info["gm"].(map[string]any); ok {
		f.display, _ = gm["displayName"].(string)
		f.shortDesc, _ = gm["shortDescription"].(string)
	}
	f.desc, _ = info["description"].(string)
	if author, ok := info["author"].(map[string]any); ok {
		f.author, _ = author["name"].(string)
	}
	if kws, ok := info["keywords"].([]any); ok {
		for _, kw := range kws {
			if s, ok := kw.(string); ok {
				f.keywords = append(f.keywords, s)
			}
		}
	}
	return f
}

// scorePackage sums token hits across all fields. displayName and the
// package name are identifiers and weigh most, keywords and prose weigh
// progressively less.
func scorePackage(name string, info map[string]any, tokens []string) int {
	f := collectSearchFields(name, info)
	nameText := normalize(f.name)
	display := normalize(f.display)
	shortDesc := normalize(f.shortDesc)
	desc := normalize(f.desc)
	author := normalize(f.author)
	keywords := make([]string, len(f.keywords))
	for i, kw := range f.keywords {
		keywords[i] = normalize(kw)
	}

	score := 0
	for _, token := range tokens {
		score += 3 * matchScore(display, token)
		score += 2 * matchScore(nameText, token)
		for _, kw := range keywords {
			score += matchScore(kw, token)
		}
		score += matchScore(shortDesc, token)
		score += matchScore(desc, token)
		score += matchScore(author, token)
	}
	return score
}

func pkgName(info map[string]any) string {
	name, _ := info["name"].(string)
	return name
}
