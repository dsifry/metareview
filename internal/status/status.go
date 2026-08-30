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
	if target != "" {
		scoped := make([]reviewlog.Summary, 0, len(logs))
		for _, s := range logs {
			if covers(s, target) {
				scoped = append(scoped, s)
			}
		}
		r.Reviews = scoped
	}
	for _, s := range logs {
		if !s.HasUnresolvedBlockers {
			continue
		}
		if target != "" && !covers(s, target) {
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
	r.Blocked = len(r.MustClear) > 0
	return r, nil
}

// VerdictUnreviewed is the verdict on a target no review log covers. It is not a review outcome —
// it is the absence of one — and it is named so a hook can tell "you have findings to fix" from
// "you have not run a review", which call for different things from the operator.
const VerdictUnreviewed = "UNREVIEWED"

// Emit writes the report as JSON and returns the process exit code: 1 when something must be
// cleared, 0 otherwise. It lives here rather than in main so the contract - including the exit
// decision a hook depends on - is covered by tests.
// Emit writes the whole report. EmitFor narrows it to one target.
func Emit(root string, w io.Writer) (int, error) { return EmitFor(root, "", w) }

// EmitFor writes the report for one target and returns the process exit code.
func EmitFor(root, target string, w io.Writer) (int, error) {
	r, err := BuildFor(root, target)
	if err != nil {
		return 0, err
	}
	// Report holds strings, bools, ints, slices of those, and []reviewlog.Summary, whose own
	// fields bottom out in the same kinds - not "only strings, bools, ints and slices of the
	// same", as this said before. What makes encoding/json fail is a channel, a func, a cyclic
	// pointer graph or a failing custom Marshaler, and the type graph contains none, so a branch
	// here would be unreachable and untestable.
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
func covers(s reviewlog.Summary, target string) bool {
	if s.Target == target {
		return true
	}
	for _, p := range s.CoveredPaths {
		if p == target {
			return true
		}
	}
	return false
}
