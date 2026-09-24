// Package mutationfresh judges whether the kills in a mutation report still describe the code under
// review (spec §6 of docs/superpowers/specs/2026-09-23-mutation-incremental-design.md). The harness
// (templates/mutation-incremental/) attests what it verified; this package re-derives, from content
// digests only, which of those kills a later edit has made stale.
package mutationfresh

import "strings"

// The glob dialect is shared with the harness (spec §5.3; templates/mutation-incremental/lib/glob.mjs).
// Both implementations run testdata/mutation-incremental/glob-vectors.json. Paths are repo-relative
// POSIX paths, case-sensitive. `*` matches a run of characters except `/`; `?` matches one; `**` as a
// whole segment matches zero or more segments; `{a,b}` is alternation (no nesting). `*`, `?` and `**`
// never match a segment that starts with `.` unless the pattern segment does (as in Stryker). RE2 has
// no lookahead, so this is a small matcher rather than a translated regular expression.

type globToken struct {
	kind  byte // 'l' literal, '*', '?', '{'
	lit   byte
	start bool // at the start of a path segment: the dot rule applies
	alts  [][]globToken
}

func compileSegment(seg string, atStart bool) []globToken {
	var out []globToken
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		start := atStart && i == 0
		switch {
		case c == '*':
			out = append(out, globToken{kind: '*', start: start})
			for i+1 < len(seg) && seg[i+1] == '*' {
				i++
			}
		case c == '?':
			out = append(out, globToken{kind: '?', start: start})
		case c == '{' && strings.IndexByte(seg[i:], '}') > 0:
			end := i + strings.IndexByte(seg[i:], '}')
			var alts [][]globToken
			for _, alt := range strings.Split(seg[i+1:end], ",") {
				alts = append(alts, compileSegment(alt, start))
			}
			out = append(out, globToken{kind: '{', alts: alts})
			i = end
		default:
			out = append(out, globToken{kind: 'l', lit: c})
		}
	}
	return out
}

func matchTokens(tokens []globToken, s string) bool {
	if len(tokens) == 0 {
		return s == ""
	}
	t, rest := tokens[0], tokens[1:]
	switch t.kind {
	case '*':
		if t.start && strings.HasPrefix(s, ".") {
			return false
		}
		for k := 0; k <= len(s); k++ {
			if matchTokens(rest, s[k:]) {
				return true
			}
		}
		return false
	case '?':
		if s == "" || (t.start && s[0] == '.') {
			return false
		}
		return matchTokens(rest, s[1:])
	case '{':
		for _, alt := range t.alts {
			if matchTokens(append(append([]globToken(nil), alt...), rest...), s) {
				return true
			}
		}
		return false
	default:
		return s != "" && s[0] == t.lit && matchTokens(rest, s[1:])
	}
}

func matchSegments(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		for k := 0; k <= len(path); k++ {
			if matchSegments(pattern[1:], path[k:]) {
				return true
			}
			if k < len(path) && strings.HasPrefix(path[k], ".") {
				return false
			}
		}
		return false
	}
	return len(path) > 0 && matchTokens(compileSegment(pattern[0], true), path[0]) && matchSegments(pattern[1:], path[1:])
}

// MatchGlob reports whether path matches one pattern. A leading "./" in the pattern is removed.
func MatchGlob(pattern, path string) bool {
	return matchSegments(strings.Split(strings.TrimPrefix(pattern, "./"), "/"), strings.Split(path, "/"))
}

// MatchList: some non-`!` entry matches and no `!` entry matches.
func MatchList(path string, list []string) bool {
	hit := false
	for _, entry := range list {
		if strings.HasPrefix(entry, "!") {
			if MatchGlob(entry[1:], path) {
				return false
			}
		} else if !hit {
			hit = MatchGlob(entry, path)
		}
	}
	return hit
}

// Lists are the attested category lists (the implicit global entries included).
type Lists struct {
	Mutate  []string `json:"mutate"`
	Test    []string `json:"test"`
	Support []string `json:"support"`
	Global  []string `json:"global"`
	Ignore  []string `json:"ignore"`
}

// Categorize returns the first list that matches in the order global, support, test, mutate; else
// "ignore" for an ignored path, else "unclassified".
func Categorize(path string, lists Lists) string {
	ordered := []struct {
		name string
		list []string
	}{{"global", lists.Global}, {"support", lists.Support}, {"test", lists.Test}, {"mutate", lists.Mutate}}
	for _, l := range ordered {
		if MatchList(path, l.list) {
			return l.name
		}
	}
	if MatchList(path, lists.Ignore) {
		return "ignore"
	}
	return "unclassified"
}
