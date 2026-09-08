package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/runchain"
)

func TestDbgUnreviewed(t *testing.T) {
	root, _, headSHA := gitRepo(t)
	digest := "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pushGateFixtureWithBlockingLog(t, root, headSHA, []findings.Record{{
		ID: "mrvf-20260907-x-001", Status: findings.StatusOverridden, Classification: "blocking",
		Severity: "high", OverrideGrantedBy: "boss", OverrideGrantReason: "accepted for release",
	}})
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "now.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-x`\nTarget: `current branch`\nReviewer input digest: `"+digest+"`\n\n## Verdict\n\nNEEDS_REVISION\n\n## Blocking Findings\n\n### mrvf-20260907-x-001: a blocker\n")
	mustWriteFile(t, filepath.Join(root, "docs", "metareview", "reviews", "later.md"),
		"# metareview: pr-ready review\n\nRun ID: `mrv-later`\nTarget: `current branch`\nReviewer input digest: `"+digest+"`\n\n## Verdict\n\nPASS_ADVISORY\n")
	rows := []gateLogRow{{"mrv-x", "now.md", "NEEDS_REVISION"}, {"mrv-later", "later.md", "PASS_ADVISORY"}}
	out := ""
	for _, r := range rows {
		b, _ := json.Marshal(map[string]any{
			"id": r.id, "scope": "pr-ready", "verdict": r.verdict,
			"baseSha": "base0000", "headSha": headSHA,
			"target":            map[string]string{"type": "branch", "id": "fix/146"},
			"reviewLogPath":     "docs/metareview/reviews/" + r.path,
			"reviewers":         []string{"pr-readiness-reviewer", "validation-reviewer"},
			"reviewInputDigest": digest,
			"coveredPaths":      []string{"a.go", "b.go"},
		})
		out += string(b) + "\n"
	}
	mustWriteFile(t, filepath.Join(root, ".metareview", "runs.jsonl"), out)
	if raw, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl")); err == nil {
		t.Logf("DBG runs.jsonl raw: %s", string(raw))
	}
	if runs, err := runchain.ReadRuns(root); err != nil {
		t.Logf("DBG ReadRuns ERR: %v", err)
	} else {
		for _, r := range runs {
			t.Logf("DBG run %s verdict=%s covered=%v head=%q scope=%s", r.ID, r.Verdict, r.CoveredPaths, r.HeadSHA, r.Scope)
		}
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range logs {
		t.Logf("log %s path=%s verdict=%s head=%q coveredKnown=%v covered=%v hasUnresolved=%v ids=%v", s.RunID, s.Path, s.Verdict, s.HeadSHA, s.CoveredPathsKnown, s.CoveredPaths, s.HasUnresolvedBlockers, s.FindingIDs)
	}
	scope, scopeErr := resolveBranchScope(root, "", nil, true)
	if scopeErr != nil {
		t.Logf("scopeErr: %v", scopeErr)
		return
	}
	t.Logf("unreviewed=%v commitsHasHead=%v", scope.Unreviewed(logs), scope.Commits[headSHA])
	resolved := reconcileLogsAgainstLedger(root, logs, new([]string))
	t.Logf("DBG resolved map: %v", resolved)
	rep, err := BuildForBranch(root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range rep.MustClear {
		t.Logf("MUSTCLEAR verdict=%s target=%s kind=%s", b.Verdict, b.Target, b.Kind)
	}
}
