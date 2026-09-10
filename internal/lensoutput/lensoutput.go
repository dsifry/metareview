// Package lensoutput is the typed lens-output contract: the deterministic parse, validation,
// and anchor-verification layer for lens findings (v0.12).
//
// It ports the lab's validated 0.12 pipeline (harnesseval adapters/metareview.py, rc5 typed
// schema + rc6 anchor gate; benchmarked at median F1 0.670 across the three-run series, ship
// gate ≥ 0.639 passed — see dsifry/metareview#159). The lab measured what each half buys:
// the typed schema cut output tokens 27% at ~0.5% malformation, and the anchor-in-diff gate
// is a real fabricated-finding catcher at F1-neutral (bootstrap CI [−0.064, +0.074]).
//
// The contract is deliberately two layers, both deterministic and pre-LLM:
//
//   - Validate (single finding): the standalone shape check — tag enum, severity enum,
//     confidence range, non-empty file/issue/consequence, and a sane line range.
//   - ValidatePayload (a lens's whole output against a diff): the pipeline the lab runs —
//     malformed entries are rejected and COUNTED, never crash the run; entries that are
//     well-formed but anchored outside the diff are rejected by the anchor gate; and
//     below-bar confidence is suppressed (the rubric's anchored-confidence rule, enforced
//     deterministically instead of trusted from the model).
//
// The rejection buckets mirror the lab's stats exactly (schema / enum / anchor / suppression /
// kept) so product-side rejection telemetry and lab-side [lens-validate] lines stay
// comparable run-over-run. One deliberate divergence, documented here so it is a decision and
// not a drift: an entry with an EMPTY file passes the enum bucket and dies in the anchor
// bucket (the lab's behavior — the anchor gate is what catches file-less entries), while
// Validate() rejects it up front, because a standalone finding with no file is invalid
// regardless of who asks.
//
// Anchor semantics (normative, #159): the file must appear in the diff's changed-file set and
// the cited range must intersect a hunk for that file, allowing ±AnchorContext lines of slack
// — findings may cite context just outside hunk bodies, and the slack is what keeps the gate
// F1-neutral rather than recall-negative. Both sides of the intersection use the slack, which
// is why it is ±, not one-sided.
//
// This package is a leaf: it imports run (for the candidate conversion) and nothing else from
// the tree, so any consumer can depend on it without a cycle.
package lensoutput

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// AnchorContext is the lines of slack around a hunk an anchor may cite and still count as
// in-diff. Ten: the rc6 A/B value (F1 0.646 vs 0.639 baseline, CI spanning zero — the gate
// catches fabrications without costing recall). Changing it is a benchmark-visible contract
// change, not a tuning knob.
const AnchorContext = 10

// Tag is a finding's class in the typed contract: a bug alleges the code does the wrong
// thing; an advisory is real and staff-worthy without alleging incorrect behavior (the 0.11.1
// advisory taxonomy, carried into the typed schema).
type Tag string

const (
	TagBug      Tag = "bug"
	TagAdvisory Tag = "advisory"
)

// SuppressionFloor is the anchored-confidence rule's floor: findings below it are suppressed
// unless they are P0 (a P0 is reported regardless of confidence — the rubric's only
// override, enforced here deterministically so a model cannot argue its way past it).
const SuppressionFloor = 50

// Sentinel validation errors. They exist so callers (and the payload pipeline's bucket
// accounting) can distinguish a contract violation from a wiring bug; Validate returns the
// first failing check, in declaration order.
var (
	ErrTag         = errors.New("lensoutput: tag must be \"bug\" or \"advisory\"")
	ErrSeverity    = errors.New("lensoutput: severity must be P0, P1, P2, or P3")
	ErrConfidence  = errors.New("lensoutput: confidence must be within 0..100")
	ErrFile        = errors.New("lensoutput: file must be non-empty")
	ErrLineRange   = errors.New("lensoutput: need start_line >= 1 and end_line >= start_line")
	ErrIssue       = errors.New("lensoutput: issue must be non-empty")
	ErrConsequence = errors.New("lensoutput: consequence must be non-empty")
)

// TypedFinding is one lens finding in the typed contract. Field vocabulary mirrors the lab's
// schema exactly (tag/file/start_line/end_line/issue/consequence/confidence/severity) so a
// lens prompt emitted for either side parses on both.
type TypedFinding struct {
	Tag         Tag    `json:"tag"`
	File        string `json:"file"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Issue       string `json:"issue"`
	Consequence string `json:"consequence"`
	Confidence  int    `json:"confidence"`
	Severity    string `json:"severity"`
}

// severityOK reports whether s is one of the contract's severities.
func severityOK(s string) bool {
	switch s {
	case "P0", "P1", "P2", "P3":
		return true
	}
	return false
}

// enumOK is the lab's enum bucket as one predicate: every check that bucket rejects
// (tag, severity, confidence range, non-empty issue/consequence, line range). Validate()
// wraps it with the file check; ValidatePayload buckets its failure as enum.
func (f TypedFinding) enumOK() bool {
	switch {
	case f.Tag != TagBug && f.Tag != TagAdvisory:
		return false
	case !severityOK(f.Severity):
		return false
	case f.Confidence < 0 || f.Confidence > 100:
		return false
	case strings.TrimSpace(f.Issue) == "" || strings.TrimSpace(f.Consequence) == "":
		return false
	case f.StartLine < 1 || f.EndLine < f.StartLine:
		return false
	}
	return true
}

// Validate is the standalone contract check: enumOK plus the non-empty file the pipeline
// leaves to the anchor gate (a file-less entry cannot be anchored, but a standalone finding
// with no file is invalid on its face).
func (f TypedFinding) Validate() error {
	switch {
	case f.Tag != TagBug && f.Tag != TagAdvisory:
		return fmt.Errorf("%w: %q", ErrTag, f.Tag)
	case !severityOK(f.Severity):
		return fmt.Errorf("%w: %q", ErrSeverity, f.Severity)
	case f.Confidence < 0 || f.Confidence > 100:
		return fmt.Errorf("%w: %d", ErrConfidence, f.Confidence)
	case strings.TrimSpace(f.File) == "":
		return ErrFile
	case f.StartLine < 1 || f.EndLine < f.StartLine:
		return fmt.Errorf("%w: %d..%d", ErrLineRange, f.StartLine, f.EndLine)
	case strings.TrimSpace(f.Issue) == "":
		return ErrIssue
	case strings.TrimSpace(f.Consequence) == "":
		return ErrConsequence
	}
	return nil
}

// CanonicalText is the finding's emitted review text: "[TAG] issue consequence" (the rc3+
// format — tags ride the text through every downstream consumer that matches on prose, and
// the tag survives verbatim through extraction the lab measured at ~98% adoption).
func (f TypedFinding) CanonicalText() string {
	return fmt.Sprintf("[%s] %s %s", strings.ToUpper(string(f.Tag)),
		strings.TrimSpace(f.Issue), strings.TrimSpace(f.Consequence))
}

// Anchor is the finding's raw/extraction form, "file:start-end": the shape the lab's
// anchor-corroborated matcher (tools/anchor_matcher.py) and every diff-line consumer cite.
func (f TypedFinding) Anchor() string {
	return fmt.Sprintf("%s:%d-%d", f.File, f.StartLine, f.EndLine)
}

// ToCandidate converts to the FSM candidate shape. Severity lowercases (the payload speaks
// P0-P3, the ledger speaks p0-p3 — the lab's mapping); Category is the tag; the anchor rides
// File/Line (start) because run.Finding has no raw slot.
func (f TypedFinding) ToCandidate(source string) run.Finding {
	return run.Finding{
		IssueText: f.CanonicalText(),
		File:      f.File,
		Line:      f.StartLine,
		Severity:  strings.ToLower(f.Severity),
		Category:  string(f.Tag),
		Source:    source,
	}
}

// LineRange is a half-open... no — an inclusive [Start, End] of new-side diff lines covered
// by one hunk.
type LineRange struct{ Start, End int }

// diffFileLine matches the new-side file header of a unified diff. The path is trimmed the
// way the lab's .strip() trims: a CRLF diff leaves a trailing \r that would otherwise become
// part of the file name and defeat every anchor lookup against it.
// diffFileLine matches a to-header: `+++ b/path` or, when git C-quotes a path with special
// bytes (core.quotePath default), `+++ "b/pa th.go"` — the quote wraps the prefix too.
var diffFileLine = regexp.MustCompile(`^\+\+\+ (?:b/|"b/)(.+?)"?$`)

// hunkLine matches a hunk header; the trailing @@ is a prefix (git appends a function-context
// section after it), so there is deliberately no end anchor.
var hunkLine = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// AnchorMap parses a unified diff into file -> new-side changed ranges, mechanically. A
// hunk with zero new lines (a pure deletion, "+n,0") contributes no range: there is no
// new-side line to anchor a finding against. A file's hunks accumulate under its new-side
// path — a rename carries its hunks under the path the `+++ b/` header names, so anchors
// cite the post-rename path.
//
// Header pairing: a `+++ b/` line only switches the current file when it follows a `--- `
// from-header — a real header pair. Content lines inside a hunk can forge the to-header
// shape (an added line whose text is `++ b/fake.go` renders as `+++ b/fake.go`), and the
// pair check is what stops the single-line forgery: a forged to-header's preceding line
// is hunk body, not a from-header. The bar is two ADJACENT crafted lines (a deleted line
// whose text is `-- a/…` renders as `--- a/…`, then an added line rendering as `+++ b/…`) —
// a residual accepted deliberately and pinned by test: the threat model is accidental
// corruption and degenerate patch-emitting code, not adversarial construction. Without
// the pair check, one crafted content line switches the current file to a phantom path
// and every later hunk of the real file is mis-attributed — two-sided corruption of the
// gate.
func AnchorMap(diff string) map[string][]LineRange {
	files := make(map[string][]LineRange)
	cur := ""
	awaitingTo := false // the previous line was a `---` from-header: the next `+++ b/` is genuine
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "--- ") {
			awaitingTo = true
			continue
		}
		if m := diffFileLine.FindStringSubmatch(line); m != nil {
			if !awaitingTo {
				continue // a content line that merely looks like a to-header
			}
			awaitingTo = false
			cur = strings.TrimSpace(m[1])
			if _, ok := files[cur]; !ok {
				files[cur] = nil
			}
			continue
		}
		awaitingTo = false
		if cur == "" {
			continue
		}
		if m := hunkLine.FindStringSubmatch(line); m != nil {
			start, n := atoi(m[1]), 1
			if m[2] != "" {
				n = atoi(m[2])
			}
			if n > 0 {
				files[cur] = append(files[cur], LineRange{Start: start, End: start + n - 1})
			}
		}
	}
	return files
}

// atoi is strconv.Atoi for a regexp-verified digit string, saturating instead of wrapping:
// an absurd hunk header (a 20-digit start) is nonsense either way, but a wrap to a negative
// would fabricate an inverted range, and the error branch of real strconv would be an
// uncoverable statement in a 100%-gated package.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if n > (1<<31-11)/10 {
			return 1<<31 - 1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// AnchorInDiff reports whether the finding's citation maps into the anchor map: the file is
// in the diff and the cited range intersects some hunk of it with AnchorContext lines of
// slack on both sides. A finding anchored outside the diff is a fabricated finding by the
// contract's definition — the file or line it cites as evidence does not exist in the
// change under review.
func AnchorInDiff(f TypedFinding, files map[string][]LineRange) bool {
	// Path spellings must not decide fabrication: a lens citing "./pkg/a.go" or "b/pkg/a.go"
	// names the same changed file as one citing "pkg/a.go". Normalize the same way the
	// judge's diff-selection does (judge.NormalizePath — mirrored here to keep this package
	// a leaf importing only run; PR #162 review finding). Git may also C-quote a path with
	// special bytes (core.quotePath, the default): "pkg/spa ce.go" — strip the quotes so a
	// finding citing the plain spelling matches its own header.
	norm := func(p string) string {
		p = strings.TrimPrefix(strings.TrimSpace(p), "./")
		if len(p) >= 2 && strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
			p = p[1 : len(p)-1]
		}
		for _, prefix := range []string{"a/", "b/"} {
			p = strings.TrimPrefix(p, prefix)
		}
		return path.Clean(p)
	}
	ranges, ok := files[norm(f.File)]
	if !ok {
		// fall back to a normalized-map lookup in case the diff keys themselves carry prefixes
		for k, v := range files {
			if norm(k) == norm(f.File) {
				ranges, ok = v, true
				break
			}
		}
	}
	if !ok {
		return false
	}
	for _, r := range ranges {
		if f.StartLine-AnchorContext <= r.End && f.EndLine+AnchorContext >= r.Start {
			return true
		}
	}
	return false
}

// Stats are the rejection buckets, mirroring the lab's per-lens [lens-validate] accounting
// (schema/enum/anchor/suppression/kept) so the numbers stay comparable across the product
// and the lab.
type Stats struct {
	Schema      int // entry missing fields or carrying unparseable ones
	Enum        int // well-formed entry, value outside the contract
	Anchor      int // entry whose citation does not map into the diff
	Suppression int // below SuppressionFloor and not P0
	Kept        int
}

// wireEntry is the payload-side shape. Typed JSON with POINTER fields, not TypedFinding:
// a missing field is a schema rejection (the lab's KeyError bucket) while a present-but-
// empty one is an enum rejection, and unmarshaling straight into the typed struct would
// collapse the two. A type error (a string where a number belongs) fails the unmarshal
// itself — also schema, exactly the lab's int() coercion failures.
type wireEntry struct {
	Tag         *string `json:"tag"`
	File        *string `json:"file"`
	StartLine   *int    `json:"start_line"`
	EndLine     *int    `json:"end_line"`
	Issue       *string `json:"issue"`
	Consequence *string `json:"consequence"`
	Confidence  *int    `json:"confidence"`
	Severity    *string `json:"severity"`
}

// ValidatePayload validates one lens's typed output against the diff it reviewed. It never
// returns an error and never panics: a malformed payload is DATA, not a crash — the lab's
// ~0.5% malformation rate is the operating assumption, and the buckets are the telemetry.
func ValidatePayload(payload []byte, diff string) ([]TypedFinding, Stats) {
	var stats Stats
	var top struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal(payload, &top); err != nil {
		// Top-level shape invalid (not an object, or findings not a list): the whole
		// payload is one schema rejection, exactly the lab's accounting.
		stats.Schema = 1
		return nil, stats
	}
	if top.Findings == nil {
		// JSON null (a top-level null, or {"findings":null}) unmarshals without error
		// and leaves Findings nil — the silent-poison shape: zero kept, zero counted, the
		// run reads as "lenses found nothing" when the output was actually garbage.
		// Bucket it as a schema rejection so the LENS_VALIDATE warn fires (second-round
		// review finding; the contract's promise is rejected-AND-counted, never silent).
		stats.Schema = 1
		return nil, stats
	}
	anchors := AnchorMap(diff)
	var kept []TypedFinding
	for _, raw := range top.Findings {
		var w wireEntry
		if err := json.Unmarshal(raw, &w); err != nil {
			stats.Schema++
			continue
		}
		if w.Tag == nil || w.File == nil || w.StartLine == nil || w.EndLine == nil ||
			w.Issue == nil || w.Consequence == nil || w.Confidence == nil || w.Severity == nil {
			stats.Schema++
			continue
		}
		f := TypedFinding{
			Tag: Tag(*w.Tag), File: strings.TrimSpace(*w.File),
			StartLine: *w.StartLine, EndLine: *w.EndLine,
			Issue: strings.TrimSpace(*w.Issue), Consequence: strings.TrimSpace(*w.Consequence),
			Confidence: *w.Confidence, Severity: *w.Severity,
		}
		if !f.enumOK() {
			stats.Enum++
			continue
		}
		if !AnchorInDiff(f, anchors) {
			stats.Anchor++
			continue
		}
		if f.Confidence < SuppressionFloor && f.Severity != "P0" {
			stats.Suppression++
			continue
		}
		kept = append(kept, f)
		stats.Kept++
	}
	return kept, stats
}
