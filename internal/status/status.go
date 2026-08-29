// Package status answers one question in a form a program can branch on: may work proceed,
// and if not, what must be cleared first.
//
// metareview's Completion Rule is prose in CLAUDE.md - "before saying work is done, run the
// appropriate gate". Prose is not a gate: an agent can skip it, or run it, read NEEDS_REVISION
// and carry on. A host hook can enforce it, but only against a machine-readable contract, and
// `metareview status` printed for humans. This is that contract. A host hook is meant to be a
// thin shim over it, which is what would keep the enforcement model-agnostic rather than one
// implementation per host drifting apart - but no hook ships in this repository yet, so this is
// the surface they will sit on rather than a description of something already wired.
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
}

// Build reads the repository's review logs and reports what remains unresolved.
//
// Blocking is taken from the review log's own HasUnresolvedBlockers, never re-derived here: a
// second definition of "blocker" would drift from the first, and this branch has repeatedly
// found exactly that failure - one predicate updated at three call sites out of four.
func Build(root string) (Report, error) {
	rep := repo.Detect(root)
	r := Report{
		Version:   version.Version,
		Mode:      string(rep.Mode),
		Git:       rep.Capabilities.Git,
		Beads:     rep.Capabilities.Beads,
		Metaswarm: rep.Capabilities.Metaswarm,
		MustClear: []Blocker{},
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return r, err
	}
	r.Reviews = logs
	for _, s := range logs {
		if !s.HasUnresolvedBlockers {
			continue
		}
		r.MustClear = append(r.MustClear, Blocker{
			Target: s.Target, RunID: s.RunID, Verdict: s.Verdict, Kind: s.Kind, Path: s.Path,
			BlockingCount: s.BlockingFindingCount, AttemptNumber: s.AttemptNumber, MaxAttempts: s.MaxAttempts,
		})
	}
	r.Blocked = len(r.MustClear) > 0
	return r, nil
}

// Emit writes the report as JSON and returns the process exit code: 1 when something must be
// cleared, 0 otherwise. It lives here rather than in main so the contract - including the exit
// decision a hook depends on - is covered by tests.
func Emit(root string, w io.Writer) (int, error) {
	r, err := Build(root)
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
