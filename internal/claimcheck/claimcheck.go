// Package claimcheck classifies review findings whose truth depends on the ABSENCE of
// something, and finds the diff evidence that can confirm or contradict the claim.
//
// Why this exists (issue #140, eval evidence): unverified "missing tests" claims are the
// single largest confirmed-fabrication mode across the reviewed frameworks — 34% of a
// stratified audit of confirmed rejections (n=778) were findings asserting a missing test
// whose spec file was in the same diff, often a few lines below the change. The claim is
// mechanically checkable at review time, but nothing made the check: the lens asserted
// absence without looking, and the adjudication prompt carried only the finding's own
// file, so the judge could not see the contradicting spec either.
//
// The two halves of the fix use this package:
//   - the adjudicate executor selects covering-test hunks into the judge's diff context
//     (internal/fsm/judge.ContextForGapClaim) whenever Detect fires, so the judge
//     verifies claimed absence the same way it verifies claimed defects;
//   - the lens rubrics (rubrics/testing-quality-rubric.md, rubrics/task-done-review-rubric.md)
//     require the same search before the finding is emitted at all.
//
// Design rule: this package never DECIDES a claim. A test file whose added lines mention
// the claim's subject is evidence a judge weighs, not a contradiction — measured on the
// eval corpus, 14/21 hallucinated gap-claims had such a file, but so did 149 real gap
// findings (tests that exist yet do not cover the claimed behavior). Evidence is
// injected; the verdict stays the judge's.
//
// This package is a leaf: it imports nothing from the rest of the tree, like internal/lens.
package claimcheck

import (
	"regexp"
	"sort"
	"strings"
)

// Class is a claim class whose verification requires evidence of absence.
type Class string

// ClassTestingGap is the claim class "tests/specs/coverage for X are absent" (issue #140).
const ClassTestingGap Class = "testing-gap"

// gapClaim matches the phrasings observed in the eval corpus's fabricated and genuine
// testing-gap findings: "no tests", "zero test coverage", "lacks a spec", "untested",
// "nothing asserts", "test file is missing", ... It is deliberately recall-leaning: a
// false positive costs one extra context selection, a false negative costs the whole fix.
var gapClaim = regexp.MustCompile(`(?i)\b(?:no|zero|missing|absent|lacks?|without|lacking)\s+(?:(?:a|an|any|corresponding|relevant)\s+)?(?:unit\s+|integration\s+|e2e\s+|end[-. ]to[-. ]end\s+)?(?:tests?|specs?|spec\b|test\s+cases?|test\s+coverage|coverage|assertions?)\b` +
	`|\b(?:tests?|specs?|coverage)\s+(?:are|is)\s+(?:missing|absent|lacking)\b` +
	`|\b(?:untested|uncovered|not\s+tested|not\s+covered)\b` +
	`|\bnothing\s+(?:asserts?|verifies?|tests?|checks?)\b` +
	`|\bno\s+test\s+(?:asserts?|covers?|verifies?|validates?)\b` +
	`|\b(?:test|spec)\s+file\s+(?:is\s+)?(?:missing|absent)\b`)

// Detect reports whether a finding's text asserts the absence of tests, specs or coverage,
// and returns the claim class. The text is untrusted reviewer output; the regex only
// selects which evidence to gather, never what the judge is told to conclude.
func Detect(text string) (Class, bool) {
	if gapClaim.MatchString(text) {
		return ClassTestingGap, true
	}
	return "", false
}

// testPath matches the test-file conventions the eval corpus's repos actually use
// (Ruby, Go, JS/TS, Python): test/ tests/ __tests__/ spec/ specs/ directories, the
// *.test.* / *.spec.* / *_test.go / *_test.py / test_*.py file shapes. It is a path-shape
// test only — nothing here understands a language.
var testPath = regexp.MustCompile(`(?i)(^|/)(tests?|__tests__|spec|specs)(/|$)|[._-](test|spec)\.[A-Za-z0-9]+$|(^|/)test_[^/]+\.py$|[^/]*_test\.go$`)

// IsTestPath reports whether a repository path is test-shaped.
func IsTestPath(p string) bool {
	return testPath.MatchString(p)
}

// supportPath matches test-support directories whose files mention many subjects without
// asserting any behavior (fixtures, fabricators, factories). Their content is weak
// contradiction evidence at best, so EvidenceFor skips them to keep the injected hunks
// on files a judge can actually weigh.
var supportPath = regexp.MustCompile(`(^|/)(fixtures|fabricators|factories|mocks|mockdata|testdata)/`)

// IsSupportPath reports whether a path is test-support rather than a test itself.
func IsSupportPath(p string) bool {
	return supportPath.MatchString(p)
}

// Block is one changed file as claimcheck needs it: the post-image path and the lines the
// diff ADDED. It is deliberately minimal so internal/fsm/judge can produce it from its own
// unified-diff parser (the single parser; claimcheck does not re-parse diffs) and tests
// can construct them directly.
type Block struct {
	Path  string
	Added []string
}

// Finding is the minimal finding shape claimcheck needs: where the reviewer filed it and
// what it says. File/Line feed subject extraction; Text is the claim.
type Finding struct {
	File string
	Line int
	Text string
}

// Evidence is one test-shaped file in the diff whose added lines reference the claim's
// subject, with the tokens that matched.
type Evidence struct {
	Path   string   `json:"path"`
	Tokens []string `json:"tokens"`
	Score  int      `json:"score"`
}

// ident matches an identifier-shaped token: ≥2 chars, letters/digits/underscore, starting
// with a letter or underscore. Subject tokens must survive the stoplists below.
var ident = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)

// bareFile matches a filename-shaped token (README.md, topic_embed.rb) so a claim that
// names a file yields that file's stem as a strong subject token — the claim "no
// spec/models/embeddable_host_spec.rb exists" is contradicted by that very file's hunks.
var bareFile = regexp.MustCompile(`\b[A-Za-z0-9_.-]+\.[A-Za-z0-9]{1,8}\b`)

// stopWords are tokens too generic to identify a subject: review prose, path components,
// and the claim vocabulary itself. Split into one map for one lookup; the categories are
// documentation, not separate behavior.
var stopWords = func() map[string]bool {
	const w = `the a an this that these those and or but not none missing absent without lacks lacking ` +
		`tests test testing specs spec coverage covered assertions assertion file files function functions ` +
		`method methods class classes change changes changed diff branch repo repository code line lines ` +
		`case cases scenario scenarios behavior behaviour logic value values return returns call calls ` +
		`use uses using make makes made add adds added remove removes removed something anything everything ` +
		`required require requires needed need present existing create creates created update updates ` +
		`updated delete deletes deleted handle handles handled check checks checked valid validate ` +
		`validates invalid properly correctly explicitly specifically clearly actually verify ` +
		`verifies verified assert asserts asserted exercising exercises exercised zero ` +
		`src source app lib test tests spec specs controllers models views helpers services components ` +
		`utils internal pkg cmd java main resources javascripts styles assets migrations fixtures ` +
		`factories fabricators integration unit browser server client core common shared api web admin ` +
		`config settings schema db database sql types interfaces domain factory builder builders mock ` +
		`mocks stub stubs`
	m := make(map[string]bool)
	for _, s := range strings.Fields(w) {
		m[s] = true
	}
	return m
}()

// subjectTokens returns the lowercase tokens that identify what the claim says is
// untested. Strong tokens are named files' stems and distinctive identifiers
// (CamelCase with an inner capital or an underscore — "topic_embed", "host_allowed",
// "EmbeddableHost"); weak tokens are any other surviving identifier. Both are matched
// case-insensitively; the strength distinction only sets the evidence bar (one strong hit
// admits a file, two weak hits are needed without one).
func subjectTokens(f Finding) (strong, weak map[string]bool) {
	strong, weak = map[string]bool{}, map[string]bool{}
	for _, m := range bareFile.FindAllString(f.Text, -1) {
		stem := strings.TrimSuffix(m, "."+lastExt(m))
		stem = strings.ToLower(strings.TrimLeft(stem, "._-"))
		if len(stem) >= 4 && !stopWords[stem] {
			strong[stem] = true
		}
	}
	if f.File != "" {
		base := f.File
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		stem := strings.ToLower(strings.TrimSuffix(base, "."+lastExt(base)))
		if len(stem) >= 4 && !stopWords[stem] {
			strong[stem] = true
		}
	}
	for _, m := range ident.FindAllString(f.Text, -1) {
		t := strings.Trim(m, "_")
		tl := strings.ToLower(t)
		if len(t) < 5 || stopWords[tl] {
			continue
		}
		camel := false
		for _, c := range t[1:] {
			if c >= 'A' && c <= 'Z' {
				camel = true
				break
			}
		}
		if camel || strings.ContainsRune(t, '_') {
			strong[tl] = true
		} else {
			weak[tl] = true
		}
	}
	return strong, weak
}

func lastExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

// camelJoin splits a CamelCase identifier into its underscore-joined lowercase form so a
// strong token "topic_embed" matches the code's "TopicEmbed", and an acronym boundary
// still splits ("HTTPServer" → "http_server"). Runes, not a regexp: RE2 has no lookahead,
// and the acronym rule needs to see the next rune to know whether an uppercase run ended.
func camelJoin(s string) (string, bool) {
	var parts []string
	var cur []rune
	lastUpper := false // the case of the ORIGINAL rune cur grew from, not its stored form
	isUpper := func(r rune) bool { return r >= 'A' && r <= 'Z' }
	isLower := func(r rune) bool { return (r >= 'a' && r <= 'z') || r >= 0x80 }
	flush := func() {
		if len(cur) > 0 {
			parts = append(parts, string(cur))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case r == '_' || r == '-' || r == '.':
			flush()
			lastUpper = false
		case isUpper(r):
			// A new part starts at lower→upper (camel hump) or when an uppercase run is
			// about to end (acronym: the last letter of "HTTP" before "Server"). cur
			// stores lowercase, so the previous case is tracked in lastUpper.
			if len(cur) > 0 && (!lastUpper || (i+1 < len(rs) && isLower(rs[i+1]))) {
				flush()
			}
			cur = append(cur, r+('a'-'A'))
			lastUpper = true
		default:
			cur = append(cur, r)
			lastUpper = false
		}
	}
	flush()
	return strings.Join(parts, "_"), len(parts) > 1
}

// lineTokens tokenizes one added line into the sets a subject token can match: the
// lowercase identifiers, the camel-split lowercase joins, and the underscore-stripped
// identifier concatenation (so "embeddablehost" matches the line's "EmbeddableHost"
// written as prose "embeddable host").
func lineTokens(line string) (words, joins map[string]bool, concat string) {
	words, joins = map[string]bool{}, map[string]bool{}
	var sb strings.Builder
	for _, m := range ident.FindAllString(line, -1) {
		t := strings.Trim(m, "_")
		tl := strings.ToLower(t)
		if len(tl) >= 2 {
			words[tl] = true
		}
		sb.WriteString(tl)
		if j, split := camelJoin(t); split {
			joins[j] = true
		}
	}
	return words, joins, sb.String()
}

// EvidenceFor returns the test-shaped blocks whose ADDED lines reference the claim's
// subject, strongest first, capped at max (<= 0 means a small default). The finding's own
// file is skipped: it is already the primary context, and a claim filed against a test
// file ("this spec asserts nothing") is not contradicted by that spec's own existence.
//
// The bar for admission is one strong-token hit or two weak-token hits. Measured on the
// eval corpus this admits the contradicting spec for 14 of 21 hallucinated gap-claims
// while also admitting 149 genuine gap findings whose nearby tests do not cover the
// claimed behavior — which is exactly why the caller injects this as evidence for a
// judge rather than acting on it directly.
func EvidenceFor(blocks []Block, f Finding, max int) []Evidence {
	strong, weak := subjectTokens(f)
	if len(strong)+len(weak) == 0 {
		return nil
	}
	// "b/" joins "./" and "a/" — reviewers quoting diff headers emit all three spellings
	// (judge.NormalizePath documents them), and each must recognize the finding's own file.
	own := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(f.File, "./"), "a/"), "b/"))
	var out []Evidence
	for _, b := range blocks {
		if !IsTestPath(b.Path) || IsSupportPath(b.Path) {
			continue
		}
		if strings.EqualFold(strings.TrimPrefix(b.Path, "./"), own) && own != "" {
			continue
		}
		hits := map[string]bool{}
		strongHit, score := false, 0
		for _, line := range b.Added {
			words, joins, concat := lineTokens(line)
			for t := range strong {
				// joins matches a compound identifier ("TopicEmbed" when the token is
				// topic_embed); concat matches the same with underscores stripped, for
				// prose like "embeddable host" a single token cannot see.
				if words[t] || joins[t] || strings.Contains(concat, strings.ReplaceAll(t, "_", "")) {
					if !hits[t] {
						hits[t] = true
						score += 3
						strongHit = true
					}
				}
			}
			for t := range weak {
				if words[t] && !hits[t] {
					hits[t] = true
					score++
				}
			}
		}
		if strongHit || len(hits) >= 2 {
			out = append(out, Evidence{Path: b.Path, Tokens: sortedKeys(hits), Score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	if max <= 0 {
		max = 4
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
