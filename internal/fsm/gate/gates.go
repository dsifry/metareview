package gate

import (
	"context"
	"fmt"
	"sort"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Gate codes.
const (
	CodeNoFindings       = "ERR_NO_FINDINGS"
	CodeFindingsPresent  = "ERR_FINDINGS_PRESENT"
	CodeNoConfirmed      = "ERR_NO_CONFIRMED"
	CodeConfirmedPresent = "ERR_CONFIRMED_PRESENT"
	CodeNoCommit         = "ERR_NO_COMMIT"
	CodeGateInapplicable = "ERR_GATE_INAPPLICABLE"
	CodeBugsRemain       = "ERR_BUGS_REMAIN"
	CodeAllFixed         = "ERR_ALL_FIXED"
	CodeBugsKnown        = "ERR_BUGS_KNOWN"
)

// Gate evaluates a snapshot; nil means pass.
type Gate func(ctx context.Context, s run.Snapshot, g Git) *run.GateError

func fail(name, code, detail string) *run.GateError {
	d, truncated := run.CapDetail(detail)
	return &run.GateError{Code: code, Gate: name, Detail: d, DetailTruncated: truncated}
}

var builtin = map[string]Gate{
	"findings_nonempty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.Findings) > 0 {
			return nil
		}
		return fail("findings_nonempty", CodeNoFindings, "no findings this iteration")
	},
	"findings_empty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.Findings) == 0 {
			return nil
		}
		return fail("findings_empty", CodeFindingsPresent, fmt.Sprintf("%d findings present", len(s.Findings)))
	},
	"confirmed_nonempty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.Confirmed) > 0 {
			return nil
		}
		return fail("confirmed_nonempty", CodeNoConfirmed, "no confirmed bugs this iteration")
	},
	"confirmed_empty": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.Confirmed) == 0 {
			return nil
		}
		return fail("confirmed_empty", CodeConfirmedPresent, fmt.Sprintf("%d confirmed bugs present", len(s.Confirmed)))
	},
	// nothing_found / nothing_confirmed are the iteration-0 clean exits: they
	// refuse once any bug is known, so a later discovery miss cannot end a
	// loop as clean while bugs remain.
	"nothing_found": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.AllFound) > 0 {
			return fail("nothing_found", CodeBugsKnown, fmt.Sprintf("%d bugs already known (%d unfixed)", len(s.AllFound), s.Unfixed))
		}
		if len(s.Findings) > 0 {
			return fail("nothing_found", CodeFindingsPresent, fmt.Sprintf("%d findings present", len(s.Findings)))
		}
		return nil
	},
	"nothing_confirmed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if len(s.AllFound) > 0 {
			return fail("nothing_confirmed", CodeBugsKnown, fmt.Sprintf("%d bugs already known (%d unfixed)", len(s.AllFound), s.Unfixed))
		}
		if len(s.Confirmed) > 0 {
			return fail("nothing_confirmed", CodeConfirmedPresent, fmt.Sprintf("%d confirmed bugs present", len(s.Confirmed)))
		}
		return nil
	},
	"commit_exists": commitExists,
	"all_fixed": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if converge.AllFixed(s) {
			return nil
		}
		return fail("all_fixed", CodeBugsRemain, fmt.Sprintf("%d of %d bugs unfixed", s.Unfixed, len(s.AllFound)))
	},
	"bugs_remain": func(_ context.Context, s run.Snapshot, _ Git) *run.GateError {
		if !converge.AllFixed(s) {
			return nil
		}
		return fail("bugs_remain", CodeAllFixed, fmt.Sprintf("all %d bugs fixed", len(s.AllFound)))
	},
}

// commitExists passes when at least one commit landed since the fix entry
// and the tree is clean. Git failures are reported as ERR_GIT gate errors so
// the audit shows them.
func commitExists(ctx context.Context, s run.Snapshot, g Git) *run.GateError {
	const name = "commit_exists"
	if s.FixEntryHead == "" {
		return fail(name, CodeGateInapplicable, "no fix entry recorded for this iteration")
	}
	head, err := g.Head(ctx)
	if err != nil {
		return fail(name, CodeGit, err.Error())
	}
	n, err := g.CommitCount(ctx, s.FixEntryHead, head)
	if err != nil {
		return fail(name, CodeGit, err.Error())
	}
	clean, porcelain, err := g.Status(ctx)
	if err != nil {
		return fail(name, CodeGit, err.Error())
	}
	if n > 0 && clean {
		return nil
	}
	wd, _, err := g.WorkingDiff(ctx, run.MaxDetail)
	if err != nil {
		return fail(name, CodeGit, err.Error())
	}
	return fail(name, CodeNoCommit, fmt.Sprintf("%d commits since %s; clean=%v\n--- status ---\n%s--- working diff ---\n%s", n, s.FixEntryHead, clean, porcelain, wd))
}

// Names lists the built-in gate names, sorted.
func Names() []string {
	names := make([]string, 0, len(builtin))
	for n := range builtin {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Builtin returns the gate by name.
func Builtin(name string) (Gate, bool) {
	g, ok := builtin[name]
	return g, ok
}
