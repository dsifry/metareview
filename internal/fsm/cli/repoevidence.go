package cli

// Issue #146: the production git seams for the repository-side covering-test search.
// The judge package never spawns git (the command-seam DI pattern), so the seams live
// here next to the escalation sandbox's showFile and are wired into kind.Deps.RepoSearch
// in machineDeps. Everything derives from the pinned head SHA, so a verdict's recorded
// context stays replayable.

import (
	"context"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/mockai"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// repoSearch wires kind.Deps.RepoSearch: one finding's repository-head search through
// real git at the snapshot's head. Mock runs keep it nil — mock verdicts are fixtures,
// and a fixture must not be attributed to evidence that was never weighed.
func (c *ctxDeps) repoSearch(root string, scenario *mockai.Scenario, mode judgeMode) kind.RepoSearchFunc {
	if scenario != nil || mode != judgeReal {
		return nil
	}
	return func(ctx context.Context, snap run.Snapshot, f run.Finding) (judge.RepoEvidence, error) {
		return judge.RepoTestEvidence(c.grepHead(ctx, root, snap.Head), c.showHead(ctx, root, snap.Head), f, judge.MaxGapEvidenceFiles)
	}
}

// grepHead returns the paths at rev whose content matches the RE2 pattern. It delegates
// to the shared judge.GrepSeam — the one implementation of the git-grep contract
// (exit 1 is no-matches, -z NUL parsing, rev: prefix strip); this wrapper only binds the
// invocation's git runner and directory.
func (c *ctxDeps) grepHead(ctx context.Context, root, rev string) judge.GrepPaths {
	raw := func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return c.gitRawCtx(ctx, dir, args...)
	}
	return judge.GrepSeam(ctx, raw, root, rev)
}

// showHead returns one path's content at rev through the shared judge.ShowSeam, with
// absence (not in the tree) distinct from failure — the same separation showFile makes
// for the escalation tree.
func (c *ctxDeps) showHead(ctx context.Context, root, rev string) judge.ShowHead {
	raw := func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		out, code, err := c.gitRawCtx(ctx, dir, args...)
		return out, code, err
	}
	return judge.ShowSeam(ctx, raw, root, rev)
}

// repoEvidencePaths runs the repository search for every gap-claim finding and returns
// the evidence paths for the escalation tree. A seam error yields no paths (escalation
// stays available; the primary arm already failed open the same way) and never a hard
// failure of the escalation itself.
//
// Deliberately not memoized against the executor's searchRepo: the escalation resolves
// lazily, before the executor may have searched, so the caches cannot meet. The cost is
// bounded — one search per gap-claim finding, per registry, and escalation is opt-in —
// and re-running keeps escalationFor a pure function of the snapshot.
func (c *ctxDeps) repoEvidencePaths(ctx context.Context, root string, snap run.Snapshot) []string {
	var out []string
	for _, f := range snap.Findings {
		if _, ok := claimcheck.Detect(f.IssueText); !ok {
			continue
		}
		ev, err := judge.RepoTestEvidence(c.grepHead(ctx, root, snap.Head), c.showHead(ctx, root, snap.Head), f, judge.MaxGapEvidenceFiles)
		if err != nil {
			continue
		}
		for _, e := range ev.Evidence {
			out = append(out, e.Path)
		}
	}
	return out
}
