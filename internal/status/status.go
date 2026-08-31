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

	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/reviewlog"
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
	for _, s := range logs {
		if !s.HasUnresolvedBlockers {
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
			Kind:    "unreviewed",
		})
	}
	r.Abandoned = DiscoverAbandonedRuns(root)
	for _, a := range r.Abandoned {
		r.MustClear = append(r.MustClear, Blocker{
			Target: a.Workflow + " @ " + a.State, RunID: a.RunID, Verdict: VerdictAbandoned, Kind: "fsm-run",
		})
	}
	r.Blocked = len(r.MustClear) > 0
	return r, nil
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
	// The scope is resolved BEFORE the report is built, so the commit set the report needs for
	// currency checks is the same one the scoping uses — resolved once, with this caller's base
	// and its injected git seam rather than a second guess made with neither.
	scope, scopeErr := ResolveBranchScope(root, base, run)
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
	for _, s := range scoped {
		if !s.HasUnresolvedBlockers {
			continue
		}
		r.MustClear = append(r.MustClear, Blocker{
			Target: s.Target, RunID: s.RunID, Verdict: s.Verdict, Kind: s.Kind, Path: s.Path,
			BlockingCount: s.BlockingFindingCount, AttemptNumber: s.AttemptNumber, MaxAttempts: s.MaxAttempts,
		})
	}
	for _, f := range scope.Unreviewed(all) {
		r.MustClear = append(r.MustClear, Blocker{Target: f, Verdict: VerdictUnreviewed, Kind: "unreviewed"})
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
			Target: a.Workflow + " @ " + a.State, RunID: a.RunID, Verdict: VerdictAbandoned, Kind: "fsm-run",
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
