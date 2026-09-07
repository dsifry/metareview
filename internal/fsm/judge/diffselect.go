package judge

import (
	"crypto/sha1"
	"unicode/utf8"
	"encoding/hex"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/run"
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
	want := NormalizePath(path)
	for _, b := range parseUnifiedDiff(diff) {
		if NormalizePath(b.path) == want {
			return true
		}
	}
	return false
}

// NormalizePath is the one definition of "the same file" shared by every path predicate here and
// in kind. Exact string equality was the previous rule, so a discover node that emitted
// "./internal/x.go" or "a/internal/x.go" - both ordinary ways to write a path, and both produced
// by real tools - was judged to have no evidence for a file the diff plainly carried, and its
// finding was kept as unverified_no_evidence for a human who would have found it right there.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	for _, prefix := range []string{"a/", "b/"} {
		p = strings.TrimPrefix(p, prefix)
	}
	return path.Clean(p)
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
	want := NormalizePath(path) // the same definition of "same file" DiffHasFile uses
	var block *fileBlock
	all := parseUnifiedDiff(diff)
	for i := range all {
		if NormalizePath(all[i].path) == want {
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
// AddedLinesInFile returns the contents of the lines the diff ADDED to a specific file. It keys file
// sections on the "diff --git" header via the shared parser, so an added line whose own content begins
// with "+++" (or "---") is a hunk line, never mistaken for a section header. Each returned string is
// the added line without its leading "+". A file the diff does not touch yields none.
func AddedLinesInFile(diff, path string) []string {
	want := NormalizePath(path)
	var out []string
	for _, b := range parseUnifiedDiff(diff) {
		if NormalizePath(b.path) != want {
			continue
		}
		for _, h := range b.hunks {
			for _, line := range h.lines {
				// Hunk lines are "@@…", " context", "-removed", or "+added"; only the last starts
				// with "+" (the "@@" header does not), so an added "+++foo" content line is included.
				if strings.HasPrefix(line, "+") {
					out = append(out, line[1:])
				}
			}
		}
	}
	return out
}

// RemovedLinesInFile returns the contents of the lines the diff REMOVED from a specific file — the
// mirror of AddedLinesInFile, for the §9.4 deletion binding (a deletion's Removed span must appear as
// "-" lines in the fix's own diff for the file). It keys file sections on the "diff --git" header via
// the shared parser, so a removed line whose own content begins with "---" is a hunk line, never
// mistaken for the "--- a/file" section header (that header lives in block.header, not the hunks).
// Each returned string is the removed line without its leading "-". A file the diff does not touch
// yields none.
func RemovedLinesInFile(diff, path string) []string {
	want := NormalizePath(path)
	var out []string
	for _, b := range parseUnifiedDiff(diff) {
		if NormalizePath(b.path) != want {
			continue
		}
		for _, h := range b.hunks {
			for _, line := range h.lines {
				// Hunk lines are "@@…", " context", "-removed", or "+added"; only "-" marks a removed
				// line (the "@@" header does not start with "-"), so a removed "---foo" content line is
				// included.
				if strings.HasPrefix(line, "-") {
					out = append(out, line[1:])
				}
			}
		}
	}
	return out
}

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
	// h.lines[0] is the @@ header and the window is prepended with it, so a hunk that is nothing
	// but a header has no body to window: returning here avoids emitting the header twice, which
	// produced a hunk with two @@ lines and no content.
	if len(h.lines) == 1 {
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
	// Keyed on the normalized path, like DiffHasFile: prose that quotes the candidate's own file
	// from a diff header ("b/internal/x.go") names the same file, not an extra one. Keying on the
	// raw string spent a maxReferencedFiles slot and a share of the budget on an alias of the
	// primary - showing it twice and dropping a genuine second file to make room.
	seen := map[string]bool{NormalizePath(own): true}
	for _, m := range namedPaths(text) {
		key := NormalizePath(m)
		if seen[key] || !DiffHasFile(diff, m) {
			continue
		}
		seen[key] = true
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
	seen := map[string]bool{NormalizePath(own): true}
	for _, m := range namedPaths(text) {
		key := NormalizePath(m)
		if seen[key] {
			continue
		}
		seen[key] = true
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

// ChangedBlocks returns one claimcheck.Block per changed file: its post-image path and
// the lines the diff added to it. It is the bridge from this package's unified-diff
// parser to claimcheck's evidence search, so the parser stays single and cmd tooling can
// reach the same view (AddedLinesInFile is the one-file form of the same idea).
func ChangedBlocks(diff string) []claimcheck.Block {
	all := parseUnifiedDiff(diff)
	out := make([]claimcheck.Block, 0, len(all))
	for _, b := range all {
		blk := claimcheck.Block{Path: b.path}
		for _, h := range b.hunks {
			for _, line := range h.lines {
				if strings.HasPrefix(line, "+") {
					blk.Added = append(blk.Added, line[1:])
				}
			}
		}
		out = append(out, blk)
	}
	return out
}

// MaxGapEvidenceFiles bounds how many covering-test files one testing-gap claim can pull
// into the judge's context. Like maxReferencedFiles it bounds prompt size, not judgment:
// the evidence is ranked by how strongly its added lines reference the claim's subject.
// Exported because every measurement call site (the corpus golden, the eval tool) must cap
// identically or its numbers describe a different search than the product runs.
const MaxGapEvidenceFiles = 3

// GapClaimEvidence returns the covering-test evidence for a testing-gap finding: the
// diff's test files whose added lines reference the claim's subject, strongest first.
// It is the "verify the claimed absence" half of issue #140 - the same search the lens
// rubrics now require before the finding is emitted at all.
func GapClaimEvidence(diff string, f run.Finding, max int) []claimcheck.Evidence {
	return claimcheck.EvidenceFor(ChangedBlocks(diff), claimcheck.Finding{File: f.File, Line: f.Line, Text: f.IssueText}, max)
}

// gapClaimDisclosure is prefixed to a gap-claim context so the judge knows what the extra
// test-file hunks are and why they are there. It lives in the diff VALUE, not the prompt
// template, like every other metareview disclosure: the templates are byte-pinned to
// harnesseval, the evidence is ours to shape.
const gapClaimDisclosure = "[metareview: the finding claims tests, specs or coverage are ABSENT for a subject this diff changes. " +
	"This diff also changes the following test-shaped files, whose added lines reference the claimed subject; their hunks are appended below. " +
	"Before accepting the claim, verify the claimed absence against them - a test that exists yet does not cover the claimed behavior still makes the claim true, but a test that does cover it makes the claim false.]\n"

// gapClaimRepoDisclosure follows the diff-side one when the repository-head search found
// covering-test candidates (issue #146): those files are NOT in the diff, so without the
// disclosure the judge cannot tell what the appended full files are or where they came from.
const gapClaimRepoDisclosure = "[metareview: the repository head was also searched for test-shaped files whose content references the claimed subject; " +
	"the full (line-capped) content of each match is appended below and was NOT changed by this diff. " +
	"Weigh it the same way: a repository test that covers the claimed behavior makes the claim false, one that does not leaves the claim real.]\n"

// gapClaimRepoNone is appended when the repository search COMPLETED but found no
// candidate. Only a completed search may say this - an error or a skipped search stays
// silent, because silence is what the pre-#146 context looked like and a failed search
// must never read as evidence of absence.
const gapClaimRepoNone = "[metareview: the repository head was also searched for test-shaped files whose content references the claimed subject; none matched. " +
	"Treat this as the search record the claim's confirmation rests on.]\n"

// ContextForGapClaim is ContextForClaim for a finding whose claim class is testing-gap
// (claimcheck.Detect): the primary file, the files the text names, the test files whose
// added lines reference the claim's subject, AND - when repo is non-nil - the repository-
// head candidates from the issue #146 search, with disclosures naming every source. The
// cal.com failure (#140) was exactly a claim whose contradicting spec sat in the same
// diff, eight lines below the change, and the judge was never shown it; the #146 blind
// spot was the covering test that lives OUTSIDE the diff entirely.
func ContextForGapClaim(diff string, alreadyTruncated bool, f run.Finding, budget int, repo *RepoEvidence) (out string, truncated bool, hash string, evidence []claimcheck.Evidence) {
	evidence = GapClaimEvidence(diff, f, MaxGapEvidenceFiles)
	// Referenced paths and evidence paths are deduped by NORMALIZED value: a finding
	// that names "b/spec/x.rb" while the evidence returns "spec/x.rb" names the same
	// file, and the raw-key lookup would append its hunks twice (CodeRabbit #145).
	var paths []string
	inPaths := map[string]bool{}
	addPath := func(p string) bool {
		key := NormalizePath(p)
		if inPaths[key] {
			return false
		}
		inPaths[key] = true
		paths = append(paths, p)
		return true
	}
	for _, p := range ReferencedPaths(diff, f.File, f.IssueText) {
		addPath(p)
	}
	for _, e := range evidence {
		addPath(e.Path)
	}
	// Repository candidates join the same normalized dedup: a path the diff's hunks
	// already carry is NOT appended again from its full head content — the hunks win,
	// the full body behind them is redundant context spend. "Already carries" means
	// CONTENT: the evidence's tokens must be visible in the path's rendered hunks. A
	// repo match on unchanged lines of a touched file renders no hunk line, and dropping
	// its body would leave the judge with neither the covering assertion nor a search
	// record while the audit claims the evidence was offered.
	var repoFiles []repoFile
	coveredByHunks := func(p string, tokens []string) bool {
		// SelectDiff cannot render only when the path is not in the diff at all — both dedup
		// sources guarantee it is (ReferencedPaths filters on DiffHasFile; diff evidence is
		// built from the diff's own blocks). An empty sel then matches no token, which is
		// the desired "not covered" answer, so no separate !ok branch exists to dead-spot.
		sel, _, _ := SelectDiff(diff, p, 0, budget)
		low := strings.ToLower(sel)
		concat := strings.ReplaceAll(low, "_", "")
		for _, t := range tokens {
			if strings.Contains(low, t) || strings.Contains(concat, strings.ReplaceAll(t, "_", "")) {
				return true
			}
		}
		return false
	}
	if repo != nil {
		for _, e := range repo.Evidence {
			if body, ok := repo.Content[e.Path]; ok {
				if addPath(e.Path) {
					repoFiles = append(repoFiles, repoFile{path: e.Path, body: body, inDiff: DiffHasFile(diff, e.Path)})
				} else if !coveredByHunks(e.Path, e.Tokens) {
					// the diff touches the path but its hunks do not render the matched
					// content: the body carries the only view of the covering lines
					repoFiles = append(repoFiles, repoFile{path: e.Path, body: body, inDiff: DiffHasFile(diff, e.Path)})
				}
			}
		}
		// every repo path is evidence for the audit trail, even one whose full content
		// was skipped because the diff already shows its hunks
		for _, e := range repo.Evidence {
			addEvidencePath(&evidence, e.Path)
		}
	}
	if len(paths) == 0 {
		out, truncated, _ := ContextFor(diff, alreadyTruncated, f.File, f.Line, budget)
		if repo != nil && repo.Ran && len(repo.Evidence) == 0 {
			// Hash the context the judge actually receives — the disclosure is part of it.
			out = gapClaimRepoNone + out
			sum := sha1.Sum([]byte(out))
			return out, truncated, hex.EncodeToString(sum[:]), evidence
		}
		_, _, hash := ContextFor(diff, alreadyTruncated, f.File, f.Line, budget)
		return out, truncated, hash, evidence
	}
	// Same split as ContextForClaim: the declared file keeps two shares of the budget,
	// every corroborating path one. Repository candidates' paths are already IN paths
	// (they joined the normalized dedup above), so they are not counted again here —
	// and each repo body is CLIPPED to its share: a full head file is not bounded by the
	// diff the way SelectDiff hunks are, and an unclipped body could push the context
	// far past the caller's budget while the truncated flag said complete.
	share := budget / (len(paths) + 2)
	// The primary's truncation flag is preserved, not inferred from byte length: a small
	// elided block is easily outweighed by the disclosure and repeated file headers, which
	// would send partial context to the adjudicator marked complete (CodeRabbit #145).
	primary, primaryTruncated, _ := ContextFor(diff, alreadyTruncated, f.File, f.Line, budget-share*len(paths))
	bodyClipped := false
	for i := range repoFiles {
		clipped := clipBody(repoFiles[i].body, share)
		if len(clipped) != len(repoFiles[i].body) {
			bodyClipped = true
		}
		repoFiles[i].body = clipped
	}
	var b strings.Builder
	if len(evidence) > 0 {
		b.WriteString(gapClaimDisclosure)
	}
	if len(repoFiles) > 0 {
		b.WriteString(gapClaimRepoDisclosure)
	} else if repo != nil && repo.Ran && len(repo.Evidence) == 0 {
		// The main path owes the same honesty as the early return: a completed search that
		// found nothing is the search record the criterion asks the judge to cite.
		b.WriteString(gapClaimRepoNone)
	}
	b.WriteString(primary)
	for _, p := range paths {
		if sel, ok, _ := SelectDiff(diff, p, 0, share); ok {
			b.WriteString("\n" + sel)
		}
	}
	for _, rf := range repoFiles {
		b.WriteString("\n" + rf.header() + rf.body)
	}
	out = b.String()
	sum := sha1.Sum([]byte(out))
	return out, alreadyTruncated || primaryTruncated || bodyClipped, hex.EncodeToString(sum[:]), evidence
}

// clipBody cuts a repository file's bounded content to at most n bytes, on a line
// boundary where one fits, with an elision marker so the judge can tell the body was
// cut rather than the file simply ending there. The cut never splits a multi-byte UTF-8
// rune: an invalid rune in the prompt or the hashed context corrupts what the judge
// reads and what the audit records.
func clipBody(body string, n int) string {
	if len(body) <= n {
		return body
	}
	cut := body[:n]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i+1]
	}
	return cut + "[metareview: repository file truncated at the context budget]\n"
}

// repoFile is one repository-head candidate the judge needs beyond the diff hunks: its
// bounded full content, under a header whose provenance is CHECKED against the diff, not
// asserted — a diff-touched file's hunks show only the changed lines, and the label must
// say the full head content is what carries the rest.
type repoFile struct {
	path   string
	body   string
	inDiff bool
}

func (rf repoFile) header() string {
	if rf.inDiff {
		return "--- " + rf.path + " (repository head; also touched by this diff — the hunks above show only the changed lines, this body is the full head content) ---\n"
	}
	return "--- " + rf.path + " (repository head, unchanged by this diff) ---\n"
}

// addEvidencePath appends a path to the evidence list unless it is already there
// (normalized), keeping the audit list one-entry-per-file.
func addEvidencePath(evidence *[]claimcheck.Evidence, path string) {
	for _, e := range *evidence {
		if NormalizePath(e.Path) == NormalizePath(path) {
			return
		}
	}
	*evidence = append(*evidence, claimcheck.Evidence{Path: path})
}
