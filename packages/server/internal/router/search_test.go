package router

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// seedSearchPkg publishes name@1.0.0 with search-relevant metadata.
func seedSearchPkg(t *testing.T, ts *testServer, token, name, display, short, desc string, keywords []string) {
	t.Helper()
	rec := doPublish(ts, token, "/"+name, publishPayload(t, name, "1.0.0", []byte("tar-"+name), func(body map[string]any) {
		ver := versionOf(t, body)
		ver["description"] = desc
		ver["keywords"] = keywords
		ver["gm"] = map[string]any{
			"destination":      "scripts/1.0.0",
			"displayName":      display,
			"shortDescription": short,
		}
	}), "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("publish %s: status %d, body %s", name, rec.Code, rec.Body.String())
	}
}

// searchNames runs a query and returns matched package names in result order.
func searchNames(t *testing.T, ts *testServer, q string) []string {
	t.Helper()
	rec := doReq(t, ts.handler, http.MethodGet, "/-/verdaccio/data/search?q="+url.QueryEscape(q), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET search %q: status %d, body %s", q, rec.Code, rec.Body.String())
	}
	var out []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET search %q: unmarshal %q: %v", q, rec.Body.String(), err)
	}
	names := make([]string, 0, len(out))
	for _, r := range out {
		names = append(names, r.Name)
	}
	return names
}

func TestSearchMatchesFields(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "damirlut")

	seedSearchPkg(t, ts, token, "@damirlut/sprite-atlas", "Sprite Atlas",
		"Sprite atlas packer", "Packs sprites into GameMaker texture pages",
		[]string{"sprite", "atlas", "texture"})
	seedSearchPkg(t, ts, token, "@damirlut/debugger-dump", "Debugger Dump",
		"Dumps IDE debug state", "", []string{"debug", "tool"})
	seedSearchPkg(t, ts, token, "@damirlut/audio-mixer", "Audio Mixer",
		"", "Mixes audio tracks for the game", nil)
	seedSearchPkg(t, ts, token, "@acme/zzz-anvil", "Anvil", "", "", []string{"blacksmith"})

	tests := []struct {
		query string
		want  []string
	}{
		// displayName and description
		{"sprite", []string{"@damirlut/sprite-atlas"}},
		// name fragment
		{"dump", []string{"@damirlut/debugger-dump"}},
		// keywords
		{"tool", []string{"@damirlut/debugger-dump"}},
		// scope fragment
		{"@damirlut", []string{"@damirlut/audio-mixer", "@damirlut/debugger-dump", "@damirlut/sprite-atlas"}},
		// multi-token: "debugger dump" splits across displayName and name
		{"debugger dump", []string{"@damirlut/debugger-dump"}},
		// case-insensitive
		{"SPRITE", []string{"@damirlut/sprite-atlas"}},
		// punctuation-normalized, "audio-mixer" == "audio mixer"
		{"audio mixer", []string{"@damirlut/audio-mixer"}},
		// no hits
		{"zzzznope", nil},
	}
	for _, tt := range tests {
		got := searchNames(t, ts, tt.query)
		if len(got) != len(tt.want) {
			t.Errorf("search %q = %v, want %v", tt.query, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("search %q = %v, want %v", tt.query, got, tt.want)
				break
			}
		}
	}
}

// Identifier hits must outrank keyword hits, which must outrank prose hits.
func TestSearchRanking(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "damirlut")

	seedSearchPkg(t, ts, token, "@acme/sprite-atlas", "Sprite Atlas",
		"", "Packs sprites into pages", []string{"atlas"})
	seedSearchPkg(t, ts, token, "@acme/texture-tools", "Texture Tools",
		"", "Sprite sheet utilities", []string{"sprite", "sheet"})
	seedSearchPkg(t, ts, token, "@acme/bulk-pack", "Bulk Pack",
		"", "Sprites are packed in bulk here", []string{"pack"})

	got := searchNames(t, ts, "sprite")
	want := []string{"@acme/sprite-atlas", "@acme/texture-tools", "@acme/bulk-pack"}
	if len(got) != len(want) {
		t.Fatalf("search = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("search = %v, want %v", got, want)
			break
		}
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "damirlut")
	seedSearchPkg(t, ts, token, "@acme/a", "A", "", "", nil)

	for _, q := range []string{"", "   ", "---", "..."} {
		got := searchNames(t, ts, q)
		if len(got) != 0 {
			t.Errorf("search %q = %v, want no results", q, got)
		}
	}
}

func TestSearchResultLimit(t *testing.T) {
	ts := newTestServer(t)
	token := loginToken(t, ts, "damirlut")

	for i := 0; i < 12; i++ {
		seedSearchPkg(t, ts, token, "@acme/bulk-"+string(rune('a'+i)), "Bulk Item",
			"", "", []string{"bulk"})
	}

	got := searchNames(t, ts, "bulk")
	if len(got) != 10 {
		t.Fatalf("search = %d results, want 10", len(got))
	}
	// equal scores break ties by name
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("results not sorted by name: %v", got)
			break
		}
	}
}

// Broken entries must not leak into results.
func TestSearchSkipsBrokenEntries(t *testing.T) {
	ts := newTestServer(t)
	if err := ts.store.PutManifest(t.Context(), "@bad/corrupt", []byte(`{oops`)); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	token := loginToken(t, ts, "damirlut")
	seedSearchPkg(t, ts, token, "@good/thing", "Good Thing", "about good things", "", nil)

	got := searchNames(t, ts, "thing")
	if len(got) != 1 || got[0] != "@good/thing" {
		t.Errorf("search = %v, want only @good/thing", got)
	}
}
