package cli

import (
	"context"
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
// It is on by default. Unattended, a false reject is the dangerous direction - it drops a real
// finding and nothing says so - while a false confirm only costs a human a look. --no-escalate
// turns it off.
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
		dir, err := c.deps.TempDir("mrv-evidence-")
		if err != nil {
			return nil, err
		}
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

// changedPaths lists what the branch touched, NUL-delimited so a path containing a space or a
// newline survives (git quotes those in its line-oriented output, and the quoting is lossy to
// parse; -z avoids the question).
func (c *ctxDeps) changedPaths(root, base, head string) ([]string, error) {
	out, code, err := c.git(root, "diff", "--no-ext-diff", "--name-only", "-z", "--no-renames", base+".."+head)
	if err != nil || code != 0 {
		return nil, err
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
		out, code, err := c.git(root, "show", "--no-ext-diff", rev+":"+path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, nil
		}
		return []byte(out), true, nil
	}
}
