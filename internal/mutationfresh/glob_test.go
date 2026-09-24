package mutationfresh

import (
	"encoding/json"
	"os"
	"testing"
)

// The glob dialect is shared with the harness (spec §5.3). Both implementations run the same vectors,
// so a change to either must change both.
func TestGlobVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mutation-incremental/glob-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Match []struct {
			Pattern, Path string
			Match         bool
		} `json:"match"`
		List []struct {
			List  []string `json:"list"`
			Path  string   `json:"path"`
			Match bool     `json:"match"`
		} `json:"list"`
		Categorize struct {
			Lists Lists                             `json:"lists"`
			Cases []struct{ Path, Category string } `json:"cases"`
		} `json:"categorize"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Match {
		if got := MatchGlob(c.Pattern, c.Path); got != c.Match {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.Pattern, c.Path, got, c.Match)
		}
	}
	for _, c := range v.List {
		if got := MatchList(c.Path, c.List); got != c.Match {
			t.Errorf("MatchList(%q, %v) = %v, want %v", c.Path, c.List, got, c.Match)
		}
	}
	for _, c := range v.Categorize.Cases {
		if got := Categorize(c.Path, v.Categorize.Lists); got != c.Category {
			t.Errorf("Categorize(%q) = %q, want %q", c.Path, got, c.Category)
		}
	}
}

func TestGlobDotRuleAndAlternation(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"?.ts", ".ts", false},       // ? at the start of a segment does not match a dot
		{"a?.ts", "a..ts", true},     // ? elsewhere does
		{"?", "", false},             // ? needs a character
		{"x*", "x.ts", true},         // * after a literal matches a dot
		{"{a,b}.ts", "b.ts", true},   // alternation
		{"{a,*}.ts", ".x.ts", false}, // an alternative at the start keeps the dot rule
		{"x{a,b", "x{a,b", true},     // an unclosed brace is literal
		{"src/**", "src/.git/x", false},
		{"src/.git/*", "src/.git/x", true},
		{"a**b", "axxb", true}, // consecutive stars inside a segment are one star
	}
	for _, c := range cases {
		if got := MatchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
	if !MatchList("src/a.ts", []string{"lib/**", "src/**"}) || MatchList("src/a.d.ts", []string{"src/**", "!src/*.d.ts"}) {
		t.Error("list rule: some positive entry matches and no negated entry matches")
	}
}
