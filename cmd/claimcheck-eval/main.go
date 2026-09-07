// claimcheck-eval measures the issue #140 fix against the harnesseval corpus.
//
// It is a read-only yardstick, not a test: it reads the eval lab's stored ground truth
// (runs/*/readjudication3.json — every unmatched finding re-judged three-way by gpt-5.2
// with the full PR diff) and the cached PR diffs (.cache/pr_diffs), runs metareview's own
// claim detector and covering-test evidence search over every record, and reports the
// confusion matrix evidence-found × ground-truth-verdict. The numbers that motivated the
// fix (14/21 hallucinated gap-claims had a covering spec in the same diff) come from this
// tool; re-run it after touching internal/claimcheck to see what moved.
//
// Usage:
//
//	go run ./cmd/claimcheck-eval -harnesseval ../harnesseval
//	go run ./cmd/claimcheck-eval -harnesseval ../harnesseval -framework metareview-realistic -verbose -limit 20
//	go run ./cmd/claimcheck-eval -harnesseval ../harnesseval -repos ../harnesseval-repos
//
// -repos enables the issue #146 repo-side structural pass: a directory of corpus clones
// (<repos>/<org>/<repo> or <repos>/<org>-<repo>, one per PR repo), each pinned to its
// PR's head via the pull ref. The matrix then separates diff-only from repo-only
// evidence — the structural half of the #146 measure; the A/B re-judge (old vs new
// prompt against the v2 ground truth) needs model spend and stays open in the issue.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// record is one readjudication3 row: the finding as adjudicated, its source lens, the
// binary (v1) verdict, and the v2 three-way verdict with rationale.
type record struct {
	RunID      string  `json:"run_id"`
	URL        string  `json:"url"`
	Framework  string  `json:"framework"`
	IssueText  string  `json:"issue_text"`
	SourceLens string  `json:"source_lens"`
	OldVerdict string  `json:"old_verdict"`
	NewVerdict string  `json:"new_verdict"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

// options is everything main parses; run is pure against them so tests drive the report
// with a fixture directory instead of the real lab.
type options struct {
	dir       string
	framework string
	verbose   bool
	limit     int
	// repos, when set, is a directory of corpus clones (one per PR repo: <repos>/<org>/<repo>
	// or <repos>/<org>-<repo>) and enables the issue #146 repo-side structural pass: the
	// matrix gains the combined diff×repo rows. Empty keeps the diff-only report.
	repos string
}

// osExit is swapped in tests so main() itself is coverable (the cmd/metareview pattern).
var osExit = os.Exit

func main() { osExit(realMain()) }

// realMain parses the flags and reports; main is only the exit wrapper around it.
func realMain() int {
	var o options
	fs := flag.NewFlagSet("claimcheck-eval", flag.ContinueOnError)
	fs.SetOutput(os.Stderr) // a flag misuse must say why, not exit 2 into silence
	fs.StringVar(&o.dir, "harnesseval", "../harnesseval", "harnesseval checkout (read-only)")
	fs.StringVar(&o.framework, "framework", "metareview-realistic", "framework filter (empty = all)")
	fs.BoolVar(&o.verbose, "verbose", false, "print every claim with its evidence")
	fs.IntVar(&o.limit, "limit", 0, "with -verbose, stop after N claims (0 = all)")
	fs.StringVar(&o.repos, "repos", "", "directory of corpus clones (<repos>/<org>/<repo> or <repos>/<org>-<repo>); enables the repo-side structural pass (issue #146)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if err := evaluate(o, os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "claimcheck-eval:", err)
		return 1
	}
	return 0
}

func evaluate(o options, stdout, stderr io.Writer) error {
	records, err := loadRecords(o.dir, o.framework)
	if err != nil {
		return err
	}
	diffs, missing, fatal := loadDiffs(o.dir, records)
	if fatal != nil {
		return fatal
	}
	if missing != nil {
		// A missing cached diff must not sink the whole report: the claims it would have
		// measured are named and skipped, so the operator knows what to re-fetch.
		_, _ = fmt.Fprintln(stderr, "claimcheck-eval:", missing)
	}
	return report(stdout, records, diffs, o)
}

func loadRecords(dir, framework string) ([]record, error) {
	var out []record
	runs := filepath.Join(dir, "runs")
	matches, err := filepath.Glob(filepath.Join(runs, "*", "readjudication3.json"))
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		var rj struct {
			RunID     string   `json:"run_id"`
			URL       string   `json:"url"`
			Framework string   `json:"framework"`
			Records   []record `json:"records"`
		}
		if err := json.Unmarshal(raw, &rj); err != nil {
			return nil, fmt.Errorf("%s: %w", m, err)
		}
		if framework != "" && rj.Framework != framework {
			continue
		}
		for i := range rj.Records {
			r := rj.Records[i]
			r.RunID, r.URL, r.Framework = rj.RunID, rj.URL, rj.Framework
			out = append(out, r)
		}
	}
	return out, nil
}

// loadDiffs reads the cached diff for every URL in records. A MISSING cache file is
// reported as the returned error and skipped (the report continues; the operator re-fetches
// later). An UNPARSABLE cache file is a hard error by construction: reporting an
// all-no-evidence confusion matrix from corrupt data with a success exit code is the exact
// silent-failure mode a measurement tool must not have.
func loadDiffs(dir string, records []record) (diffs map[string]string, missing, fatal error) {
	urls := map[string]bool{}
	for _, r := range records {
		urls[r.URL] = true
	}
	diffs = map[string]string{}
	var missingURLs []string
	for u := range urls {
		sum := sha1.Sum([]byte(u))
		p := filepath.Join(dir, ".cache", "pr_diffs", hex.EncodeToString(sum[:])[:16]+".json")
		raw, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			missingURLs = append(missingURLs, u)
			continue
		}
		if err != nil {
			// unreadable is not missing: a permission failure or a directory at the
			// cache path is corrupt data, and skipping it silently reports an
			// incomplete measurement as success (CodeRabbit #145)
			return nil, nil, fmt.Errorf("read cached diff %s: %w", p, err)
		}
		var c struct {
			Diff string `json:"diff"`
		}
		// An empty diff is corrupt data wearing a valid shape: it would produce an
		// all-no-evidence matrix with a success exit — the exact silent failure this
		// tool must not have. The error names which failure it was; the empty case has
		// no err to wrap (Bugbot #145: %!w(<nil>) told the operator nothing).
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, nil, fmt.Errorf("corrupt cached diff %s: %w", p, err)
		}
		if c.Diff == "" {
			return nil, nil, fmt.Errorf("cached diff %s is empty", p)
		}
		diffs[u] = c.Diff
	}
	// Every missing URL is named, sorted: naming one map-iteration-random victim hides the
	// rest, and the operator re-fetches only what they were told about.
	sort.Strings(missingURLs)
	if len(missingURLs) > 0 {
		missing = fmt.Errorf("no cached diff for %d PR(s): %s", len(missingURLs), strings.Join(missingURLs, ", "))
	}
	return diffs, missing, nil
}

// errWriter remembers the first write error so the report body stays linear; report
// checks it once on return. A stdout that has gone away must surface, not panic mid-table.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

func (e *errWriter) println(s string) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintln(e.w, s)
}

func report(w io.Writer, records []record, diffs map[string]string, o options) error {
	out := &errWriter{w: w}
	// -repos enables the issue #146 repo-side pass: the matrix rows become the combined
	// diff×repo classification instead of the diff-only one. Claims whose PR has no
	// clone are disclosed and measured on the diff dimension alone, so the repo
	// dimension never silently shrinks the matrix.
	var pass *repoPass
	var missingRepos error
	var noClone int
	if o.repos != "" {
		pass, missingRepos = resolveRepos(o, records, reportGit)
		if missingRepos != nil {
			_, _ = fmt.Fprintln(out.w, "claimcheck-eval:", missingRepos)
		}
	}
	var total, claims, skipped, detailed, repoErrs int
	matrix := map[[2]string]int{}
	lensClaims := map[string]int{}
	byLens := map[string]int{}
	for _, r := range records {
		total++
		byLens[r.SourceLens]++
		if _, ok := claimcheck.Detect(r.IssueText); !ok {
			continue
		}
		claims++
		lensClaims[r.SourceLens]++
		diff, ok := diffs[r.URL]
		if !ok {
			// counted as a claim made, but unmeasurable without its diff: disclosed below
			// so the matrix visibly sums to claims minus skipped.
			skipped++
			continue
		}
		// The finding's own file, when its text names one, keeps EvidenceFor from using
		// that file as covering evidence for itself (CodeRabbit #145): a gap claim
		// anchored in a test file is not contradicted by that same file.
		f := run.Finding{File: findingFileFromText(r.IssueText), IssueText: r.IssueText}
		ev := judge.GapClaimEvidence(diff, f, judge.MaxGapEvidenceFiles)
		// #146: the repo-side search at the PR's pinned head, when the pass is on.
		var repoEv judge.RepoEvidence
		if pass != nil && pass.rev(r.URL) != "" {
			var err error
			if repoEv, err = pass.search(r.URL, f); err != nil {
				repoErrs++
				repoEv = judge.RepoEvidence{Ran: false}
			}
		} else if pass != nil {
			noClone++
		}
		var found string
		switch {
		case pass == nil:
			if len(ev) > 0 {
				found = "evidence"
			} else {
				found = "no-evidence"
			}
		case len(ev) > 0 && len(repoEv.Evidence) > 0:
			found = "both"
		case len(ev) > 0:
			found = "diff-only"
		case len(repoEv.Evidence) > 0:
			found = "repo-only"
		default:
			found = "no-evidence"
		}
		matrix[[2]string{found, r.NewVerdict}]++
		// the limit counts PRINTED detail records, not claims made: a leading uncached
		// claim is skipped before this point and must not consume the limit (Bugbot #145)
		if o.verbose && (o.limit <= 0 || detailed < o.limit) {
			detailed++
			var paths []string
			for _, e := range ev {
				paths = append(paths, e.Path)
			}
			out.printf("[%s | %s] %s\n  evidence: %s\n  v2 rationale: %.180s\n\n",
				r.NewVerdict, r.SourceLens, clip(r.IssueText, 150), strings.Join(paths, ", "), r.Rationale)
		}
	}

	pct := 0.0
	if total > 0 {
		pct = 100 * float64(claims) / float64(total)
	}
	out.printf("records: %d  claims detected: %d (%.1f%%)\n", total, claims, pct)
	if skipped > 0 {
		out.printf("claims skipped (no cached diff): %d\n", skipped)
	}
	rows := []string{"evidence", "no-evidence"}
	if pass != nil {
		rows = []string{"both", "diff-only", "repo-only", "no-evidence"}
		out.printf("claims without a repo clone (measured on the diff only): %d\n", noClone)
		out.printf("repo search errors: %d\n", repoErrs)
	}
	verdicts := []string{"bug", "important_non_bug", "hallucination", "unresolved"}
	out.printf("\n%-14s", "evidence\\v2")
	for _, v := range verdicts {
		out.printf("%8s", clip(v, 8))
	}
	out.println("")
	for _, found := range rows {
		out.printf("%-14s", found)
		for _, v := range verdicts {
			out.printf("%8d", matrix[[2]string{found, v}])
		}
		out.println("")
	}
	halWith, halWo := matrix[[2]string{"evidence", "hallucination"}], matrix[[2]string{"no-evidence", "hallucination"}]
	if pass != nil {
		halWith = matrix[[2]string{"both", "hallucination"}] + matrix[[2]string{"diff-only", "hallucination"}]
		halWo = matrix[[2]string{"repo-only", "hallucination"}] + matrix[[2]string{"no-evidence", "hallucination"}]
	}
	if halWith+halWo > 0 {
		out.printf("\nhallucinated gap-claims with covering evidence in the diff: %d/%d\n", halWith, halWith+halWo)
	}
	out.println("\nclaims by lens:")
	var lenses []string
	for l := range byLens {
		lenses = append(lenses, l)
	}
	sort.Slice(lenses, func(i, j int) bool { return lensClaims[lenses[i]] > lensClaims[lenses[j]] })
	for _, l := range lenses {
		if lensClaims[l] > 0 {
			out.printf("  %-42s %4d / %4d records\n", l, lensClaims[l], byLens[l])
		}
	}
	return out.err
}

// findingFileFromText extracts the leading repo path a review finding names
// ("app/models/x.rb:36-43 — ..."), the shape every harnesseval record carries. Empty
// when the text names no path.
func findingFileFromText(text string) string {
	m := leadingPath.FindString(strings.TrimSpace(text))
	if m == "" {
		return ""
	}
	return strings.TrimSuffix(m, ":")
}

var leadingPath = regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+\.[A-Za-z0-9]{1,8}:?`)

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// rune-safe: a byte cut can split a multibyte character and print mojibake
	r := []rune(s)
	for len(string(r)) > n && len(r) > 0 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
