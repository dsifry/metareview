package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/sandbox"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// escalationFor builds the second opinion a rejected cross-file candidate gets: the same judge
// the run declared, confined to a materialized tree of the changed files at base and head.
//
// It is off unless --escalate. Unattended, a false reject is the dangerous direction - it drops
// a real finding and nothing says so - while a false confirm only costs a human a look, which is
// why it exists. It is opt-in rather than default because the implementation has known defects
// that make it drop findings; see escalation in wiring.go.
//
// It returns (nil, nil), meaning "escalation unavailable, the rejection stands", when the run's
// judge is not agentic. An HTTP judge cannot read a tree, so escalating to it would re-ask the
// same question with the same evidence at twice the cost.
func (c *ctxDeps) escalationFor(root string) kind.EscalateFunc {
	return func(ctx context.Context, snap run.Snapshot, node *workflow.Node) (*kind.Escalation, error) {
		if node == nil || !strings.HasPrefix(strings.ToLower(node.Model), judge.CodexPrefix) {
			return nil, nil
		}
		paths, err := c.changedPaths(root, snap.BaseSHA, snap.Head)
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		// Findings routinely turn on a file the branch never touched. A tree of only the
		// changed files cannot settle those, so every path any finding names is carried too -
		// at head, since an unchanged file has no separate base side.
		paths = append(paths, c.referencedByFindings(snap, paths)...)
		dir, err := c.deps.TempDir("mrv-evidence-")
		if err != nil {
			return nil, err
		}
		c.sandboxRoots = append(c.sandboxRoots, dir)
		tree, err := sandbox.Materialize(dir, snap.BaseSHA, snap.Head, paths, c.showFile(root))
		if err != nil {
			return nil, err
		}
		j, err := c.newJudge()
		if err != nil {
			return nil, err
		}
		return &kind.Escalation{
			Judge:    judge.WithCodexWorkDir(j, tree.Root),
			Model:    node.Model,
			Effort:   node.Effort,
			Evidence: run.EvidenceSandbox,
			TreeHash: tree.TreeHash,
			BaseSHA:  tree.BaseSHA,
			HeadSHA:  tree.HeadSHA,
		}, nil
	}
}

// referencedByFindings collects the paths this run's findings name that are not already in
// the changed set. Bounded by the per-finding cap in AllReferencedPaths and by dedup here.
func (c *ctxDeps) referencedByFindings(snap run.Snapshot, changed []string) []string {
	have := map[string]bool{}
	for _, p := range changed {
		have[p] = true
	}
	var extra []string
	for _, f := range snap.Findings {
		for _, p := range judge.AllReferencedPaths(f.File, f.IssueText) {
			if have[p] {
				continue
			}
			have[p] = true
			extra = append(extra, p)
		}
	}
	return extra
}

// changedPaths lists what the branch touched, NUL-delimited so a path containing a space or a
// newline survives (git quotes those in its line-oriented output, and the quoting is lossy to
// parse; -z avoids the question).
func (c *ctxDeps) changedPaths(root, base, head string) ([]string, error) {
	out, code, err := c.git(root, "diff", "--no-ext-diff", "--name-only", "-z", "--no-renames", base+".."+head)
	if err != nil {
		return nil, err
	}
	// gate.RealExec reports a failed git as (code, nil): a nonzero exit arrives with err == nil.
	// Returning (nil, nil) here would read to the caller as "escalation deliberately unavailable,
	// the rejection stands", and the finding would be recorded as a hallucination - a real finding
	// deleted by an outage, which is the failure escalation exists to prevent. A resumed run whose
	// recorded BaseSHA no longer resolves after a rebase or gc exits 128 on exactly this call.
	if code != 0 {
		return nil, fmt.Errorf("git diff %s..%s failed with exit code %d", base, head, code)
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// showFile reads one path at one revision. A path absent at a revision is not an error: a file
// added on the branch has no base side, and the judge is told so by its absence from base/.
func (c *ctxDeps) showFile(root string) sandbox.ShowFunc {
	return func(rev, path string) ([]byte, bool, error) {
		// Absence and failure must be distinguishable. Mapping every nonzero exit to "absent"
		// means an unreadable object produces a partial or empty tree that still carries a
		// well-formed TreeHash, and the judge is asked to settle a claim inside a directory that
		// was never materialized while the audit records evidence=sandbox.
		//
		// `cat-file -e` cannot answer this: it exits 128 both for a path missing from the tree and
		// for a revision that does not resolve. `ls-tree` separates them - it exits 0 whether or
		// not the path is there and says which by printing it, so a nonzero exit is a real failure.
		listed, code, err := c.git(root, "ls-tree", "--name-only", "-z", rev, "--", path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git ls-tree %s -- %s failed with exit code %d", rev, path, code)
		}
		if strings.Trim(listed, "\x00") == "" {
			return nil, false, nil
		}
		// Read the blob raw. c.git trims, and a file body must reach the tree byte for byte or
		// its line numbers no longer match the finding that points into it.
		out, code, err := c.gitRaw(root, "cat-file", "blob", rev+":"+path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git cat-file blob %s:%s failed with exit code %d", rev, path, code)
		}
		return out, true, nil
	}
}
