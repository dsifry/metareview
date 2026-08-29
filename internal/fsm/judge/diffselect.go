package judge

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

// Selecting the diff a judge sees is deliberately language-agnostic. A unified diff's
// structure - "diff --git" file blocks containing "@@ -a,b +c,d @@" hunks - is a property
// of the format, identical for Go, Python, YAML, Markdown or anything else metareview is
// pointed at. Nothing here parses the file's contents.
//
// In particular this does NOT use git's --function-context (-W) or the section heading git
// appends after the @@ marker: both come from xfuncname regexes that exist only for the
// languages git ships a driver for, so they would silently degrade on everything else.

// fileBlock is one file's slice of a unified diff.
type fileBlock struct {
	path   string   // the post-image path ("b/..." without the prefix)
	header []string // "diff --git", index, ---/+++ lines: everything before the first @@
	hunks  []hunk
}

type hunk struct {
	start, count int // the post-image line range this hunk covers
	lines        []string
}

// parseUnifiedDiff splits a unified diff into per-file blocks. Anything it cannot parse is
// kept as header text rather than dropped, so an unrecognised section is never silently lost.
func parseUnifiedDiff(diff string) []fileBlock {
	var blocks []fileBlock
	var cur *fileBlock
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if cur != nil {
				blocks = append(blocks, *cur)
			}
			cur = &fileBlock{path: postImagePathFromHeader(line)}
			cur.header = append(cur.header, line)
		case cur == nil:
			continue // preamble before the first file block
		case strings.HasPrefix(line, "@@"):
			start, count := parseHunkRange(line)
			cur.hunks = append(cur.hunks, hunk{start: start, count: count, lines: []string{line}})
		case len(cur.hunks) > 0:
			h := &cur.hunks[len(cur.hunks)-1]
			h.lines = append(h.lines, line)
		default:
			cur.header = append(cur.header, line)
		}
	}
	if cur != nil {
		blocks = append(blocks, *cur)
	}
	return blocks
}

// postImagePathFromHeader reads the b/ side of a "diff --git a/x b/x" line. git quotes a
// path containing non-ASCII or control bytes; the b/ side is taken from the last " b/" so a
// path that itself contains " b/" does not mis-split.
func postImagePathFromHeader(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	// git C-quotes a path containing non-ASCII or control bytes, so the b/ side appears as
	// ` "b/...` rather than ` b/...`. Checking only the bare form drops those files entirely.
	if i := strings.LastIndex(rest, " \"b/"); i >= 0 {
		if unquoted, err := strconv.Unquote(rest[i+1:]); err == nil {
			return strings.TrimPrefix(unquoted, "b/")
		}
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return ""
}

// parseHunkRange reads the post-image range from "@@ -a,b +c,d @@". A missing count means 1.
func parseHunkRange(line string) (start, count int) {
	i := strings.Index(line, "+")
	if i < 0 {
		return 0, 0
	}
	field := line[i+1:]
	if j := strings.IndexAny(field, " \t"); j >= 0 {
		field = field[:j]
	}
	s, c, found := strings.Cut(field, ",")
	start, _ = strconv.Atoi(s)
	if !found {
		return start, 1
	}
	count, _ = strconv.Atoi(c)
	return start, count
}

// DiffHasFile reports whether a unified diff carries a block for path. It is the
// precondition for adjudicating a finding: a judge cannot evaluate a candidate whose file
// is absent from the evidence, and that is knowable without spending the call.
func DiffHasFile(diff, path string) bool {
	for _, b := range parseUnifiedDiff(diff) {
		if b.path == path {
			return true
		}
	}
	return false
}

// SelectDiff returns the slice of a unified diff that bears on one candidate: the hunks of
// the candidate's own file, nearest to line first, plus a header naming what was left out.
// It reports whether anything was elided and the hash of what it returned.
//
// A candidate whose file is absent yields ok == false: the caller must not spend a judge
// call on evidence that cannot contain the answer. Compare CutDiff, which takes the first
// budget bytes of the whole branch diff - on a 538-file branch that is the alphabetically
// first dozen files and nothing the candidate refers to.
func SelectDiff(diff, path string, line, budget int) (out string, ok bool, hash string) {
	var block *fileBlock
	all := parseUnifiedDiff(diff)
	for i := range all {
		if all[i].path == path {
			block = &all[i]
			break
		}
	}
	if block == nil {
		return "", false, ""
	}
	// nearest-first: the hunk covering line, then outward by distance
	order := make([]int, len(block.hunks))
	for i := range order {
		order[i] = i
	}
	dist := func(h hunk) int {
		switch {
		case line == 0:
			return h.start // no line: prefer the top of the file
		case line < h.start:
			return h.start - line
		case line > h.start+h.count:
			return line - (h.start + h.count)
		default:
			return 0
		}
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && dist(block.hunks[order[j]]) < dist(block.hunks[order[j-1]]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	head := strings.Join(block.header, "\n")
	kept, kbytes := map[int]bool{}, len(head)
	cutInside := false
	for n, i := range order {
		size := len(strings.Join(block.hunks[i].lines, "\n")) + 1
		if kbytes+size > budget {
			// The nearest hunk alone can exceed the budget - this branch has hunks over
			// 300 KB. Rather than blow the budget or send nothing, keep a line window
			// centred on the candidate's line. Positional, so it stays language-agnostic;
			// the result is no longer a well-formed hunk, so it says so.
			if n == 0 {
				block.hunks[i].lines = windowLines(block.hunks[i], line, budget-kbytes-disclosureReserve)
				cutInside = true
				kept[i] = true
			}
			continue
		}
		kept[i], kbytes = true, kbytes+size
	}
	var b strings.Builder
	if cutInside {
		b.WriteString("[metareview: the hunk at line " + strconv.Itoa(line) + " of " + path +
			" exceeded the budget; showing a line window around it, not a complete hunk]\n")
	}
	if len(kept) < len(block.hunks) {
		// Disclosure lives in the diff VALUE, not the prompt template: the templates are
		// byte-pinned to harnesseval by TestJ1Provenance and must not be edited.
		b.WriteString("[metareview: showing " + strconv.Itoa(len(kept)) + " of " +
			strconv.Itoa(len(block.hunks)) + " hunks for " + path + ", nearest to line " +
			strconv.Itoa(line) + " first]\n")
	}
	b.WriteString(head)
	for i := range block.hunks {
		if kept[i] {
			b.WriteString("\n" + strings.Join(block.hunks[i].lines, "\n"))
		}
	}
	out = b.String()
	sum := sha1.Sum([]byte(out))
	return out, true, hex.EncodeToString(sum[:])
}

// ChangedPaths lists every post-image path in a unified diff, for telling the judge what
// exists outside the window it was given.
func ChangedPaths(diff string) []string {
	blocks := parseUnifiedDiff(diff)
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.path != "" {
			out = append(out, b.path)
		}
	}
	return out
}

// disclosureReserve is the room kept for the two "[metareview: ...]" notices so the
// selection still fits its budget once they are prepended.
const disclosureReserve = 256

// windowLines keeps the @@ header plus the lines around target, within budget. Selection is
// by position in the hunk only: no language, no syntax, no notion of a statement or block.
func windowLines(h hunk, target, budget int) []string {
	if len(h.lines) == 0 || budget <= 0 {
		return h.lines
	}
	centre := 1
	if target > h.start {
		centre = target - h.start + 1
	}
	if centre >= len(h.lines) {
		centre = len(h.lines) - 1
	}
	lo, hi, used := centre, centre, len(h.lines[0])+1+len(h.lines[centre])+1
	for lo > 1 || hi < len(h.lines)-1 {
		grew := false
		if lo > 1 && used+len(h.lines[lo-1])+1 <= budget {
			lo--
			used += len(h.lines[lo]) + 1
			grew = true
		}
		if hi < len(h.lines)-1 && used+len(h.lines[hi+1])+1 <= budget {
			hi++
			used += len(h.lines[hi]) + 1
			grew = true
		}
		if !grew {
			break
		}
	}
	out := []string{h.lines[0]}
	return append(out, h.lines[lo:hi+1]...)
}

// maxPathList caps the changed-path listing so it cannot itself consume the budget.
const maxPathList = 4000

// ContextFor builds the diff value for one candidate: the hunks of its own file when the
// diff carries them, and otherwise an explicit statement that no diff is available plus the
// paths that did change. The second form matters more than it looks. Sending the first N
// bytes of an unrelated file lets the judge answer confidently from evidence that cannot
// contain the answer - measured on this branch, 0.94-0.99 confidence on 100 candidates whose
// files were all absent from the window. Saying so lets it abstain instead of confabulating.
//
// truncated is true whenever the judge is not seeing the whole story, so a caller can
// distinguish a complete answer from a partial one.
func ContextFor(diff string, alreadyTruncated bool, file string, line, budget int) (out string, truncated bool, hash string) {
	if file == "" {
		// A finding with no file attribution is unlocalised: there is nothing to select on,
		// so the head of the diff is the best available evidence. CutDiff reports whether it
		// had to cut, so the judge is still told when it is seeing part of the story.
		return CutDiff(diff, alreadyTruncated)
	}
	if sel, ok, h := SelectDiff(diff, file, line, budget); ok {
		elided := len(sel) < len(diff)
		return sel, alreadyTruncated || elided, h
	}
	var b strings.Builder
	b.WriteString("[metareview: no diff is available for " + file +
		"; it is not among the changed files. The branch changed these paths:]\n")
	for _, p := range ChangedPaths(diff) {
		if b.Len()+len(p)+1 > maxPathList {
			b.WriteString("...\n")
			break
		}
		b.WriteString(p + "\n")
	}
	out = b.String()
	sum := sha1.Sum([]byte(out))
	return out, true, hex.EncodeToString(sum[:])
}

// maxReferencedFiles caps how many extra files one finding can pull in. The finding text is
// untrusted reviewer output, and for prompt construction it selects which of OUR OWN hunks to
// show and nothing more - an uncapped list would still let one finding crowd out the budget.
//
// It does NOT stay inside the diff everywhere: cli.referencedByFindings feeds these same paths to
// the evidence sandbox, which materializes repository files at both revisions. That path filters
// anything escaping the tree before it gets there, and this cap bounds how many files one finding
// can cause to be copied - but the claim "our own hunks and nothing more" is true of the prompt,
// not of the sandbox.
const maxReferencedFiles = 4

// referencedPath matches a path-shaped token inside a finding's prose: two or more slash
// separated segments ending in an extension. It deliberately does NOT enumerate this repo's
// top-level directories or a set of extensions - metareview runs against many repositories
// and languages, and a hardcoded list silently matches nothing in the next one. Precision
// comes from DiffHasFile instead: a match is only used when the diff actually carries it.
var referencedPath = regexp.MustCompile(`[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+\.[A-Za-z0-9]+`)

// rootFile matches a bare filename with no directory - README.md, CLAUDE.md, package.json.
// Requiring a slash misses exactly the documents that sit at the repository root, which is
// where the user-facing ones live. The stem must be at least two characters so prose
// abbreviations ("e.g.", "i.e.") do not match; anything else that slips through is filtered by
// existence when the evidence tree is built, and at worst costs one escalation.
var rootFile = regexp.MustCompile(`\b[A-Za-z0-9_-]{2,}\.[A-Za-z0-9]{1,5}\b`)

// namedPaths is every path-shaped token in a finding's prose, directories or bare filenames.
func namedPaths(text string) []string {
	out := referencedPath.FindAllString(text, -1)
	for _, m := range rootFile.FindAllString(text, -1) {
		// rootFile's character class contains no slash, so every match here is a bare name.
		// A bare name that is the tail of a path already matched is that same file written
		// twice, not a second one: "internal/x/y.go" must not also yield "y.go".
		tail := false
		for _, p := range out {
			if strings.HasSuffix(p, "/"+m) {
				tail = true
				break
			}
		}
		if !tail {
			out = append(out, m)
		}
	}
	return out
}

// ReferencedPaths returns the paths a finding's text names, excluding its own file and
// anything the diff does not carry, capped at maxReferencedFiles in first-mentioned order.
//
// Findings are routinely cross-file: "reviewlog.go blocks on eight lenses but README says
// five", "this test pins the wrong constant in kind.go". Selecting only the declared file
// hands the judge one side of the comparison and it correctly declines - measured here, two
// of four sampled rejections were exactly that.
func ReferencedPaths(diff, own, text string) []string {
	var out []string
	seen := map[string]bool{own: true}
	for _, m := range namedPaths(text) {
		if seen[m] || !DiffHasFile(diff, m) {
			continue
		}
		seen[m] = true
		if out = append(out, m); len(out) == maxReferencedFiles {
			break
		}
	}
	return out
}

// MentionsOtherFiles reports whether a finding's text names a path other than its own file,
// whether or not the branch changed that file. It is the escalation trigger: a claim like "the
// code now requires eight lenses but these four documents still say five" is cross-file even
// though the documents are unchanged, and filtering on the diff would silently never escalate
// exactly the findings a second opinion is best at.
func MentionsOtherFiles(own, text string) bool {
	for _, m := range namedPaths(text) {
		if m != own {
			return true
		}
	}
	return false
}

// AllReferencedPaths is ReferencedPaths without the diff filter: every path the text names
// except the finding's own, capped. A sandbox built from these can carry an unchanged file the
// claim depends on, which a tree of only the changed files cannot.
func AllReferencedPaths(own, text string) []string {
	var out []string
	seen := map[string]bool{own: true}
	for _, m := range namedPaths(text) {
		if seen[m] {
			continue
		}
		seen[m] = true
		if out = append(out, m); len(out) == maxReferencedFiles {
			break
		}
	}
	return out
}

// ContextForClaim is ContextFor plus the files the finding's text names. The declared file
// keeps the larger share of the budget: it is the primary locus, the others are corroboration.
func ContextForClaim(diff string, alreadyTruncated bool, file string, line int, text string, budget int) (out string, truncated bool, hash string) {
	refs := ReferencedPaths(diff, file, text)
	if len(refs) == 0 {
		return ContextFor(diff, alreadyTruncated, file, line, budget)
	}
	share := budget / (len(refs) + 2) // the declared file gets two shares
	// The second return of ContextFor is named `truncated`, not `ok`. It was bound here to a
	// variable called `ok` and OR'd in as `!ok`, so the term claimed truncation exactly when the
	// primary was NOT truncated - an inversion that never changed an answer, because on this path
	// ContextFor's flag is true by construction (it carries one file out of several) and the
	// length test below decided every reachable case. Dropped rather than corrected.
	//
	// The length test is an approximation and known to be imperfect in both directions: each
	// selection re-emits its own file header, so a claim carried in FULL can produce more bytes
	// than the diff it came from (measured: 40193 from 40074) and read as complete. Measuring it
	// properly needs SelectDiff to report whether it cut, which it does not - and its budget is
	// not a hard cap, since a smaller budget can return MORE bytes (measured: 40102 at budget 133
	// against 39983 unbounded). That contract is worth settling before this signal is rebuilt on
	// top of it; see docs/0.10.0-candidates.md.
	primary, _, _ := ContextFor(diff, alreadyTruncated, file, line, budget-share*len(refs))
	var b strings.Builder
	b.WriteString(primary)
	for _, r := range refs {
		if sel, got, _ := SelectDiff(diff, r, 0, share); got {
			b.WriteString("\n" + sel)
		}
	}
	out = b.String()
	sum := sha1.Sum([]byte(out))
	return out, alreadyTruncated || len(out) < len(diff), hex.EncodeToString(sum[:])
}
