// Package status answers one question in a form a program can branch on: may work proceed,
// and if not, what must be cleared first.
//
// metareview's Completion Rule is prose in CLAUDE.md - "before saying work is done, run the
// appropriate gate". Prose is not a gate: an agent can skip it, or run it, read NEEDS_REVISION
// and carry on. A host hook can enforce it, but only against a machine-readable contract, and
// `metareview status` printed for humans. This is that contract. A host hook is meant to be a
// thin shim over it, which is what would keep the enforcement model-agnostic rather than one
// implementation per host drifting apart. hooks/pre-finish.sh is that shim, and .claude/settings.json
// registers it — the manifest in hooks/hooks.json applies only to a plugin install, so for a long
// while the hook existed as a file nothing loaded, and the enforcement it describes never once ran.
package status

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/reviewstate"
	"github.com/dsifry/metareview/internal/version"
)

// Blocker is one target that must be cleared before work proceeds.
type Blocker struct {
	Target        string `json:"target"`
	RunID         string `json:"run_id"`
	Verdict       string `json:"verdict"`
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	BlockingCount int    `json:"blocking_count,omitempty"`
	AttemptNumber int    `json:"attempt_number,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`
}

// Report is the whole answer. Blocked is the field a hook branches on; MustClear is what it
// tells the operator.
type Report struct {
	Version   string              `json:"version"`
	Mode      string              `json:"mode"`
	Git       bool                `json:"git"`
	Beads     bool                `json:"beads"`
	Metaswarm bool                `json:"metaswarm"`
	Reviews   []reviewlog.Summary `json:"reviews"`
	MustClear []Blocker           `json:"must_clear"`
	Blocked   bool                `json:"blocked"`
	// Target is the scope this report was built for, empty when it covers everything. A reader
	// has to be able to tell a clean repository from a clean corner of a blocked one.
	Target string `json:"target,omitempty"`
	// Abandoned are FSM runs stopped somewhere that is not an ending.
	Abandoned []AbandonedRun `json:"abandoned,omitempty"`
	// Warnings say why an answer may be narrower or wider than asked for — a scope that could
	// not be resolved reports unscoped rather than empty, and has to say so.
	Warnings []string `json:"warnings,omitempty"`
}

// Build reads the repository's review logs and reports what remains unresolved.
//
// Blocking is taken from the review log's own HasUnresolvedBlockers, never re-derived here: a
// second definition of "blocker" would drift from the first, and this branch has repeatedly
// found exactly that failure - one predicate updated at three call sites out of four.
// Build reports on everything. BuildFor narrows to one target.
func Build(root string) (Report, error) { return BuildFor(root, "") }

// BuildFor reports only what stands against `target`, or everything when target is empty.
//
// Scoping is what makes a host hook usable. Unscoped, `blocked` spans the entire review history -
// 66 entries on this repository - so a Stop hook wired to it would refuse an agent because of
// work it never touched. A gate that always says no is a livelock, and an operator who has to
// disable it to get anything done has no gate at all.
func BuildFor(root, target string) (Report, error) {
	return buildFor(root, target, nil)
}

// buildFor is BuildFor with the branch's commit set supplied by the caller.
//
// BuildForBranch resolves that scope itself, with the caller's base and the injected git seam.
// Having BuildFor resolve its own with a fixed empty base and a nil runner meant every
// branch-scoped status ran gitcontext collection and rev-list TWICE and discarded the inner
// result, and — worse than the waste — a test injecting a fake RunGit still shelled out to the
// real git here, and an explicit --base never reached the currency check at all. Threading it
// through keeps one answer to "what are this branch's commits" instead of two that can disagree.
func buildFor(root, target string, current map[string]bool) (Report, error) {
	rep := repo.Detect(root)
	r := Report{
		Version:   version.Version,
		Mode:      string(rep.Mode),
		Git:       rep.Capabilities.Git,
		Beads:     rep.Capabilities.Beads,
		Metaswarm: rep.Capabilities.Metaswarm,
		MustClear: []Blocker{},
		Target:    target,
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return r, err
	}
	// Scoping narrows the whole report, not just must_clear. A document that says
	// `"target": "t-1"` while listing every other target's reviews invites the reader to think
	// they are seeing everything, which is the misreading the field exists to prevent.
	r.Reviews = logs
	// The branch's own commits, so a review can be checked for currency before it is allowed to
	// answer for a path. Resolved here only when the caller had no scope of its own; where it
	// cannot be worked out the set stays nil, and covers then refuses to credit CoveredPaths at
	// all rather than crediting them blindly.
	if current == nil && target != "" {
		if scope, err := ResolveBranchScope(root, "", nil); err == nil {
			current = scope.Commits
		}
	}
	if target != "" {
		scoped := make([]reviewlog.Summary, 0, len(logs))
		for _, s := range logs {
			if covers(s, target, current) {
				scoped = append(scoped, s)
			}
		}
		r.Reviews = scoped
	}
	// A run whose findings were repaired in a later attempt no longer blocks: the repair loop records
	// the fix as a child linked by previousRunId, and a clean child means the parent's findings were
	// cleared. Without this, a first review that found anything blocks forever — the gate could only
	// ever clear a change that passed on its first look, which no real change does.
	superseded := supersededRuns(logs)
	// Same-head dedup (issue #97): re-running the SAME review over the SAME (kind, target, baseSha, headSha)
	// records a fresh log each time. supersededRuns only clears an ancestor of a CLEAN child, so three
	// NEEDS_REVISION re-runs over one diff would otherwise render the branch as three identical blockers that
	// never clear. The latest same-head/same-base run supersedes the earlier ones (a fix loop reviews a
	// DIFFERENT commit, and two runs at the same head but a DIFFERENT base — different diffs — are not
	// collapsed, issue #99). Shared with the projector so the gate and pr-ready agree.
	for id := range reviewstate.StaleSameHeadRunIDs(logs) {
		superseded[id] = true
	}
	for _, s := range logs {
		if !reviewstate.LogBlocks(s) { // unresolved blockers OR an ESCALATED verdict — one shared predicate
			continue
		}
		if superseded[s.RunID] {
			continue
		}
		if target != "" && !covers(s, target, current) {
			continue
		}
		r.MustClear = append(r.MustClear, Blocker{
			Target: s.Target, RunID: s.RunID, Verdict: s.Verdict, Kind: s.Kind, Path: s.Path,
			BlockingCount: s.BlockingFindingCount, AttemptNumber: s.AttemptNumber, MaxAttempts: s.MaxAttempts,
		})
	}
	// An UNREVIEWED target is not a cleared one. Scoping matched a target string against the
	// review log, and a target nothing had ever reviewed matched nothing, produced an empty
	// must_clear, and reported blocked:false — so the narrower the scope an agent claimed, the
	// more certainly the gate let it through, and asking about a file that had never been
	// reviewed was the reliable way to be told everything was fine.
	//
	// The Completion Rule is "before saying work is done, run the gate". A gate that answers
	// "nothing to clear" when it has never looked at the work states the opposite.
	if target != "" && len(r.Reviews) == 0 {
		r.MustClear = append(r.MustClear, Blocker{
			Target:  target,
			Verdict: VerdictUnreviewed,
			Kind:    UnreviewedKind,
		})
	}
	r.Abandoned = DiscoverAbandonedRuns(root)
	for _, a := range r.Abandoned {
		r.MustClear = append(r.MustClear, Blocker{
			Target: a.Workflow + " @ " + a.State, RunID: a.RunID, Verdict: VerdictAbandoned, Kind: AbandonedKind,
		})
	}
	r.Blocked = len(r.MustClear) > 0
	return r, nil
}

// AbandonedKind is the Blocker.Kind for an FSM run left mid-flight; UnreviewedKind is the kind for a
// changed file no passing review has read.
const (
	AbandonedKind  = "fsm-run"
	UnreviewedKind = "unreviewed"
)

// CommitScope selects which files a pending `git commit` will actually write, so the gate measures exactly
// that set and cannot be walked around by the flag that decides it.
type CommitScope int

const (
	// ScopeStaged is a plain `git commit`: it writes the index, so the gate measures `git diff --cached`.
	ScopeStaged CommitScope = iota
	// ScopeAll is `git commit -a`/`--all` (or a commit that names a pathspec, or `--include`/`--only`): the
	// commit pulls in tracked working-tree content beyond the curated index, so the gate must measure ALL
	// tracked changes vs HEAD (`git diff HEAD`), not just what happens to be staged. This is the hole a
	// staged-only gate left wide open: `git commit -am` stages-and-writes in one step, so at PreToolUse time
	// nothing is staged yet and a staged-only gate saw an empty diff and waved the whole commit through.
	ScopeAll
)

// CommitGate is the PRE-COMMIT decision over a plain `git commit` (ScopeStaged). See CommitGateScoped for
// the scope-aware form the hook drives; this is the staged-only default kept for callers and tests.
func CommitGate(root, base string, run RunGit) (blocked bool, message string, err error) {
	return CommitGateScoped(root, base, ScopeStaged, run)
}

// CommitGateScoped is the PRE-COMMIT decision, scoped to THIS commit — the files the commit will WRITE, not
// the whole branch. `scope` says which files that is (see CommitScope): ScopeStaged is the index; ScopeAll
// is every tracked working-tree change, which is what `git commit -a` and a pathspec commit actually record.
// A commit only asks "were the files I am committing reviewed", so a normal small commit is a small check
// that never needs the whole-branch sharded review. Whole-branch integration is PushGate's job. Both the
// decision AND the message live here (tested, mutation-checkable); the CLI is a thin shim.
//
// Coverage is by FILE: a passing review that read a file on a branch commit clears it. (Content-aware
// currency — re-flag a file edited since it was reviewed — is a deliberate follow-up, not wired here.)
func CommitGateScoped(root, base string, scope CommitScope, run RunGit) (blocked bool, message string, err error) {
	if run == nil {
		run = realGit
	}
	branch, err := ResolveBranchScope(root, base, run)
	if err != nil {
		// Fail closed: not knowing what the branch is means we cannot say the committed files are reviewed.
		return true, "metareview: commit blocked — branch scope could not be resolved (" + err.Error() +
			"); failing closed. Record an override reason to commit anyway.\n", nil
	}
	files, ferr := gitCommitFiles(run, root, scope)
	if ferr != nil {
		// Fail closed, exactly as an unresolvable scope does: not knowing what this commit writes means we
		// cannot say the committed files were reviewed.
		return true, "metareview: commit blocked — could not list the files this commit writes (" + ferr.Error() +
			"); failing closed. Record an override reason to commit anyway.\n", nil
	}
	if len(files) == 0 {
		return false, "", nil // nothing this commit writes (or only generated) → nothing to gate
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return false, "", err
	}
	commitScope := BranchScope{Base: branch.Base, Head: branch.Head, Commits: branch.Commits, Files: files}
	unreviewed := commitScope.Unreviewed(logs)
	if len(unreviewed) == 0 {
		return false, "", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d file(s) this commit writes are not reviewed:\n", len(unreviewed))
	for _, f := range unreviewed {
		fmt.Fprintf(&b, "  - %s\n", f)
	}
	baseArg := ""
	if base != "" {
		baseArg = " --base " + base
	}
	fmt.Fprintf(&b, "Review the changes: `metareview review prompt%s`, run the adversarial review and record coverage.\n", baseArg)
	return true, b.String(), nil
}

// PushGate is the PRE-PUSH decision, scoped to the WHOLE branch: everything about whether the branch's code
// is reviewed — unreviewed changed files, open (non-superseded) blocking reviews, an unresolvable scope
// (fail closed) — minus abandoned runs (CommitBlockers). There is NO claim-free exemption: every changed
// file must be reviewed, whitespace-only included (see TestPushGateDoesNotExemptWhitespaceOnlyChange). This
// is the integration pass: it catches what per-commit reviews missed and anything spanning commits, and on a
// large branch it is where the single sharded review belongs. Same tested-decision-plus-message shape as CommitGate.
func PushGate(root, base string, run RunGit) (blocked bool, message string, err error) {
	// committedOnly: a push sends commits, so uncommitted local WIP must not block a reviewed branch.
	report, err := buildForBranch(root, base, run, true)
	if err != nil {
		return false, "", err
	}
	blockers := CommitBlockers(report)
	if len(blockers) == 0 {
		return false, "", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "metareview: push blocked — %d unreviewed or unresolved across this branch:\n", len(blockers))
	for _, bl := range blockers {
		fmt.Fprintf(&b, "  - [%s] %s\n", bl.Verdict, bl.Target)
	}
	baseArg := ""
	if base != "" {
		baseArg = " --base " + base
	}
	fmt.Fprintf(&b, "Run the whole-branch review (sharded if large): `metareview review pr-ready%s --evidence <file>`, clear findings, then push; or record an override reason.\n", baseArg)
	return true, b.String(), nil
}

// pushedRef is one line of git's pre-push stdin: "<local-ref> <local-sha> <remote-ref> <remote-sha>".
type pushedRef struct {
	localRef  string
	localSHA  string
	remoteRef string
}

// isZeroSHA reports whether a sha is the all-zero deletion sentinel git streams for a ref DELETION (40 hex
// zeros for sha1, 64 for sha256). A deletion carries no content, so there is nothing to review.
func isZeroSHA(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// PushGateForRefs applies the pre-push gate to the refs git streams on stdin (one line each:
// "<local-ref> <local-sha> <remote-ref> <remote-sha>"). It closes issue #82's silent bypass, where a push of
// a ref OTHER than the checked-out branch (`git push origin evil:main`, `HEAD~3:main`) was evaluated against
// the checked-out branch — not its own content — and so slipped past the gate with no --no-verify.
//
//   - A ref DELETION (all-zero local sha) is skipped — no content to review.
//   - If there is no non-deletion ref, the push is not blocked (nothing to gate).
//   - If EVERY non-deletion ref's local sha equals the checked-out HEAD, the pushed content IS the checked-out
//     branch, so it delegates to PushGate and gates exactly as before.
//   - Otherwise it BLOCKS (fail closed), naming each ref whose sha != HEAD: the gate measures the checked-out
//     branch and cannot verify a different ref's content. The remedy is to check that ref out and push it, or
//     push with --no-verify to override deliberately.
//
// This is the fail-closed MVP (issue #82, Option B): it does not attempt to review a non-checked-out ref's own
// content. A malformed line, or a HEAD that cannot be resolved, also fails closed — an unverifiable push must
// block, never wave through.
func PushGateForRefs(root, base, stdin string, run RunGit) (blocked bool, message string, err error) {
	refs, malformed := parsePushedRefs(stdin)
	if malformed != "" {
		return true, fmt.Sprintf("metareview: PUSH BLOCKED — a pre-push ref line from git was malformed (%q), so the gate cannot tell what is being pushed; failing closed. Push with --no-verify to override deliberately.\n", malformed), nil
	}
	var content []pushedRef
	for _, r := range refs {
		if isZeroSHA(r.localSHA) {
			continue // a deletion carries no content
		}
		if strings.HasPrefix(r.remoteRef, "refs/tags/") {
			// A tag is a pointer to commits that were gated when their BRANCH was pushed, not new branch
			// content — and an annotated tag's object sha never equals HEAD, so gating it would falsely block
			// a legit `git push origin v1.0` (issue #82 acceptance: tags are not falsely blocked).
			continue
		}
		content = append(content, r)
	}
	if len(content) == 0 {
		return false, "", nil // only deletions / tags (or an empty ref list) — nothing to review
	}
	if run == nil {
		run = realGit
	}
	out, herr := run(root, "rev-parse", "HEAD")
	head := strings.TrimSpace(string(out))
	if herr != nil || head == "" {
		return true, "metareview: PUSH BLOCKED — could not resolve the checked-out HEAD, so the gate cannot verify the pushed refs; failing closed. Push with --no-verify to override deliberately.\n", nil
	}
	var mismatched []pushedRef
	for _, r := range content {
		if r.localSHA != head {
			mismatched = append(mismatched, r)
		}
	}
	if len(mismatched) == 0 {
		// Every pushed ref IS the checked-out branch — gate it exactly as before.
		return PushGate(root, base, run)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "metareview: PUSH BLOCKED — a pushed ref is not the checked-out branch, so the review gate cannot verify its content (issue #82):\n")
	for _, r := range mismatched {
		fmt.Fprintf(&b, "  - %s ← %s @ %s (checked-out HEAD is %s)\n", r.remoteRef, r.localRef, shortSHA(r.localSHA), shortSHA(head))
	}
	fmt.Fprint(&b, "Check that ref out and push it so the gate reviews its own content, or push with --no-verify to override deliberately.\n")
	return true, b.String(), nil
}

// parsePushedRefs splits git's pre-push stdin into refs. A blank line is ignored. A non-blank line with fewer
// than two fields has no local sha, so the gate cannot classify it — that line is returned as `malformed` so
// the caller fails closed rather than silently dropping a ref that might carry content.
func parsePushedRefs(stdin string) (refs []pushedRef, malformed string) {
	for _, line := range strings.Split(stdin, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return nil, line
		}
		r := pushedRef{localRef: f[0], localSHA: f[1]}
		if len(f) >= 3 {
			r.remoteRef = f[2]
		} else {
			r.remoteRef = "(unknown remote ref)"
		}
		refs = append(refs, r)
	}
	return refs, ""
}

// CommitBlockers is the subset of a report's MustClear that a COMMIT gate acts on: everything about
// whether this branch's code is reviewed — unreviewed changed files, open (non-superseded) blocking
// reviews, and an unresolvable scope (fail closed) — but NOT abandoned FSM runs. An abandoned run means a
// review loop was left unfinished, which is a "before you finish the session" concern (the Stop scope),
// not "is the code in this commit reviewed". Blocking every commit on an unrelated leftover run would be
// noise an operator learns to bypass with --no-verify, which is how a gate stops being one.
func CommitBlockers(r Report) []Blocker {
	out := make([]Blocker, 0, len(r.MustClear))
	for _, b := range r.MustClear {
		if b.Kind == AbandonedKind {
			continue
		}
		out = append(out, b)
	}
	return out
}

// VerdictAbandoned is the verdict on an FSM run left mid-flight. Every loop here ends by handing
// control to an agent, and nothing brought it back: six runs on this repository sit at `fix`, and
// none has ever reached `verify`. They were invisible to this report, so the gate whose job is
// saying "work is unfinished" could not see the plainest unfinished work there was.
const VerdictAbandoned = "ABANDONED"

// VerdictUnreviewed is the verdict on a target no review log covers. It is not a review outcome —
// it is the absence of one — and it is named so a hook can tell "you have findings to fix" from
// "you have not run a review", which call for different things from the operator.
const VerdictUnreviewed = "UNREVIEWED"

// VerdictUnscoped is the verdict when the branch scope itself could not be resolved. It is not a
// statement about any review; it says the gate could not work out what the work IS, and a gate
// that cannot see the work has not cleared it. This existed as a warning first, which nothing
// branched on, so the answer was "exit 0 with a note attached".
const VerdictUnscoped = "UNSCOPED"

// Emit writes the report as JSON and returns the process exit code: 1 when something must be
// cleared, 0 otherwise. It lives here rather than in main so the contract - including the exit
// decision a hook depends on - is covered by tests.
// Emit writes the whole report. EmitFor narrows it to one target.
func Emit(root string, w io.Writer) (int, error) { return EmitFor(root, "", w) }

// BuildForBranch reports on the work in hand: blockers against this branch's own commits or the
// files it changed, plus any changed file no passing review has read.
//
// This is the scope a Stop hook wants. Unscoped, the answer spans the whole review history and
// refuses every session for work it never touched; scoped to a target string it could not match
// a source file at all, so it cleared everything.
func BuildForBranch(root, base string, run RunGit) (Report, error) {
	return buildForBranch(root, base, run, false)
}

// buildForBranch is BuildForBranch with an explicit scope mode. committedOnly true measures only the
// committed range (PushGate — a push sends commits, so uncommitted WIP must not block it); false folds in
// staged/working-tree/untracked changes too (the Stop-hook / `status --scope branch` view).
func buildForBranch(root, base string, run RunGit, committedOnly bool) (Report, error) {
	// The scope is resolved BEFORE the report is built, so the commit set the report needs for
	// currency checks is the same one the scoping uses — resolved once, with this caller's base
	// and its injected git seam rather than a second guess made with neither.
	scope, scopeErr := resolveBranchScope(root, base, run, committedOnly)
	commits := map[string]bool(nil)
	if scopeErr == nil {
		commits = scope.Commits
	}
	r, err := buildFor(root, "", commits)
	if err != nil {
		return r, err
	}
	if err := scopeErr; err != nil {
		// The scope could not be worked out, so this cannot be narrowed. The unscoped answer is
		// returned rather than an empty one: a gate that cannot tell what the work is must fail
		// toward blocking, never toward "nothing to do".
		//
		// And it BLOCKS, which this used to only claim. A warning nothing branches on is not a
		// gate: a repository with no resolvable base — a branch with no main/master, a shallow
		// clone, a detached head — returned blocked:false and exit 0 with committed, unreviewed
		// work in the tree, while emitting a warning into JSON that the Stop hook does not read.
		// Not knowing what the work is is precisely the state in which finishing must not be
		// allowed.
		r.Warnings = append(r.Warnings, "branch scope unavailable, reporting unscoped: "+err.Error())
		r.MustClear = append(r.MustClear, Blocker{
			Target:  "branch scope could not be resolved: " + err.Error(),
			Verdict: VerdictUnscoped,
			Kind:    "scope",
		})
		r.Blocked = true
		return r, nil
	}
	all := r.Reviews
	scoped := make([]reviewlog.Summary, 0, len(all))
	for _, s := range all {
		if scope.InScope(s) {
			scoped = append(scoped, s)
		}
	}
	r.Reviews, r.Target = scoped, "branch "+shortSHA(scope.Base)+".."+shortSHA(scope.Head)
	r.MustClear = []Blocker{}
	// Supersede over the FULL log set (all), not scoped, so lineage is complete. BuildForBranch rebuilds
	// must_clear from scoped reviews and DISCARDS buildFor's — the same trap that once dropped the
	// abandoned runs (see below). Without applying the supersede here too, the branch scope — the one the
	// Stop/commit hook actually uses — blocks a successfully-repaired branch forever, which is exactly the
	// "reach clean after a repair" this change exists to enable.
	superseded := supersededRuns(all)
	// Same-head dedup (issue #97): the LATEST re-run of a review over a given (kind, target, head) supersedes
	// the earlier ones, so re-running `review pr-ready` over one commit renders the branch as a single blocker
	// rather than one per run. Computed over the FULL log set (all), like supersededRuns, so lineage is
	// complete. Shared with Build and the projector.
	for id := range reviewstate.StaleSameHeadRunIDs(all) {
		superseded[id] = true
	}
	for _, s := range scoped {
		if !reviewstate.LogBlocks(s) || superseded[s.RunID] { // unresolved blockers OR ESCALATED — shared predicate
			continue
		}
		r.MustClear = append(r.MustClear, Blocker{
			Target: s.Target, RunID: s.RunID, Verdict: s.Verdict, Kind: s.Kind, Path: s.Path,
			BlockingCount: s.BlockingFindingCount, AttemptNumber: s.AttemptNumber, MaxAttempts: s.MaxAttempts,
		})
	}
	for _, f := range scope.Unreviewed(all) {
		r.MustClear = append(r.MustClear, Blocker{Target: f, Verdict: VerdictUnreviewed, Kind: UnreviewedKind})
	}
	// An abandoned run blocks in EVERY scope, and re-adding it here is not a detail.
	//
	// Build populated MustClear with the abandoned runs; the line above that empties MustClear
	// to rebuild it from scoped reviews dropped them, and only this scope dropped them. The Stop
	// hook defaults to --scope branch, so the one scope enforcement actually runs in was the one
	// that could not see an unfinished run — the gate returned exit 0 with runs abandoned
	// mid-loop, which is the precise failure this layer exists to prevent. Unscoped and --target
	// reports kept them, so the two scopes disagreed about the same unfinished work.
	//
	// It is not filtered by scope because it is not a claim about files. A run abandoned at `fix`
	// has produced no review to scope, and asking a narrower question must never be the way to be
	// told the loop finished.
	for _, a := range r.Abandoned {
		r.MustClear = append(r.MustClear, Blocker{
			Target: a.Workflow + " @ " + a.State, RunID: a.RunID, Verdict: VerdictAbandoned, Kind: AbandonedKind,
		})
	}
	r.Blocked = len(r.MustClear) > 0
	return r, nil
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// EmitForBranch writes the branch-scoped report and returns the process exit code.
func EmitForBranch(root, base string, run RunGit, w io.Writer) (int, error) {
	r, err := BuildForBranch(root, base, run)
	if err != nil {
		return 0, err
	}
	return emit(r, w)
}

// EmitFor writes the report for one target and returns the process exit code.
func EmitFor(root, target string, w io.Writer) (int, error) {
	r, err := BuildFor(root, target)
	if err != nil {
		return 0, err
	}
	return emit(r, w)
}

// emit is the one place a Report becomes bytes and an exit code, so every scope answers in the
// same shape and with the same exit convention.
//
// Report holds strings, bools, ints, slices of those, and []reviewlog.Summary, whose own fields
// bottom out in the same kinds. What makes encoding/json fail is a channel, a func, a cyclic
// pointer graph or a failing custom Marshaler, and the type graph contains none, so an error
// branch here would be unreachable and untestable.
func emit(r Report, w io.Writer) (int, error) {
	out, _ := json.MarshalIndent(r, "", "  ")
	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return 0, err
	}
	if r.Blocked {
		return 1, nil
	}
	return 0, nil
}

// covers reports whether a review can answer for target.
//
// Target-string equality was the only test, and it never worked for a source file: a review's
// target is a task id, a document path, or the literal `current branch`, so asking about
// internal/foo.go matched no review, produced an empty must_clear and reported the file clear.
// The narrower the scope claimed, the more certainly the gate let work through.
//
// Two ways a review can answer now. It is still the named target — that is how a task id or a
// spec path is asked about. Or it examined the file, which is what CoveredPaths records. A
// review with no CoveredPaths is one written before the field existed: it cannot answer for a
// path, and saying so is what keeps an old log from silently clearing a file it never read.
// supersededRuns collects every run retired by a later CLEAN attempt in its OWN previousRunId lineage.
// A clean child (no unresolved blockers) means the parent's findings were cleared, so the parent — and
// any earlier attempt it chains back to — no longer blocks. Only the same lineage supersedes: an
// unrelated PASS never clears an open review, which would be the stale-review failure covers() guards
// against, one level up. The visited guard also makes a malformed cyclic chain terminate.
func supersededRuns(logs []reviewlog.Summary) map[string]bool {
	prevOf := make(map[string]string, len(logs))
	targetOf := make(map[string]string, len(logs))
	kindOf := make(map[string]string, len(logs))
	for _, s := range logs {
		prevOf[s.RunID] = s.PreviousRunID
		targetOf[s.RunID] = s.Target
		kindOf[s.RunID] = s.Kind
	}
	superseded := map[string]bool{}
	for _, s := range logs {
		if s.HasUnresolvedBlockers {
			continue
		}
		// Walk the ancestors of this clean attempt; each was repaired by it — but only within the SAME
		// target AND the SAME kind. A previousRunId that points across targets or kinds (a mis-linked
		// --previous-run) must not clear an unrelated open review: dropping a blocker for work that was
		// never fixed is a false-CLEAR, the worst failure a gate can have. Both guards bound the walk, and
		// the visited guard makes a malformed cyclic chain terminate.
		//
		// The KIND guard matters because the target guard is VACUOUS for pr-ready (every pr-ready run
		// records the target `current branch`), and because a task-done and an epic-ready review CAN share
		// the same target text (a path/id) — so without it, a clean child of one kind could suppress an
		// open blocker of another. pr-ready-to-pr-ready supersede still rests on the explicit previousRunId
		// link a real repair creates, which is never forged spontaneously.
		for prev := prevOf[s.RunID]; prev != "" && !superseded[prev] && targetOf[prev] == s.Target && kindOf[prev] == s.Kind; prev = prevOf[prev] {
			superseded[prev] = true
		}
	}
	return superseded
}

func covers(s reviewlog.Summary, target string, current map[string]bool) bool {
	if s.Target == target {
		return true
	}
	// Answering for a PATH requires having examined a version of it this branch still has.
	//
	// CoveredPaths crediting had no commit test at all, so a PASS recorded on an unrelated
	// commit answered for a file that branch had since rewritten: `status --json --target
	// internal/foo.go` returned blocked:false and exit 0 for work nothing current had read. The
	// hook takes this path whenever METAREVIEW_TARGET is set, so it was reachable in enforcement,
	// and it is the same defect as the branch-scope one — a review vouching for code it never saw.
	//
	// A nil `current` means the commit set could not be established. That is not permission to
	// credit: an unknown answer must fail toward blocking, exactly as an unresolvable scope does.
	if s.HeadSHA == "" || !current[s.HeadSHA] {
		return false
	}
	for _, p := range s.CoveredPaths {
		if p == target {
			return true
		}
	}
	return false
}
