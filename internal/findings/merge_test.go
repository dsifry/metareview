package findings

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mergeRepo is a git repository whose FINDINGS.md two branches both regenerate. attributes is its
// .gitattributes ("" for none).
func mergeRepo(t *testing.T, attributes string) (root string, git func(args ...string) (string, error)) {
	t.Helper()
	root = t.TempDir()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_EDITOR=true"}
	git = func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	must := func(args ...string) {
		t.Helper()
		if out, err := git(args...); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	must("init", "-q", "-b", "main")
	if attributes != "" {
		if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(attributes), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, git
}

func mergeBlocker(id string) Record {
	return Record{ID: id, Title: "Blocker " + id, Reviewer: "security-reviewer", Severity: "high", Classification: "blocking", Status: "open"}
}

func pendingOverride(id string) Record {
	return Record{ID: id, Title: "Override " + id, Reviewer: "validation-reviewer", Severity: "high", Classification: "blocking",
		Status: StatusOverridePending, OverrideRequestedBy: "claude-agent", OverrideRequestedAt: "2026-09-29T00:00:00Z", OverrideRequestReason: "why " + id}
}

// divergeAndMerge renders a base FINDINGS.md on main, then on two branches that each add a blocker and an
// override, and merges one into the other. It returns the merge's error and the merged file.
func divergeAndMerge(t *testing.T, attributes string) (error, string) {
	t.Helper()
	root, git := mergeRepo(t, attributes)
	must := func(args ...string) {
		t.Helper()
		if out, err := git(args...); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	render := func(records ...Record) {
		t.Helper()
		if err := RenderIndexWithRecords(root, records); err != nil {
			t.Fatal(err)
		}
	}
	base := []Record{mergeBlocker("mrvf-20260901-000000000000000-pr-ready-a-001"), pendingOverride("mrvf-20260901-000000000000000-pr-ready-a-002")}
	render(base...)
	must("add", "-A")
	must("commit", "-q", "-m", "base")
	must("switch", "-q", "-c", "branch-a")
	render(append(base, mergeBlocker("mrvf-20260902-000000000000000-pr-ready-b-001"), pendingOverride("mrvf-20260902-000000000000000-pr-ready-b-002"))...)
	must("commit", "-q", "-am", "a")
	must("switch", "-q", "-c", "branch-b", "main")
	render(append(base, mergeBlocker("mrvf-20260903-000000000000000-pr-ready-c-001"), pendingOverride("mrvf-20260903-000000000000000-pr-ready-c-002"))...)
	must("commit", "-q", "-am", "b")
	_, err := git("merge", "-q", "--no-edit", "branch-a")
	merged, rerr := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	return err, string(merged)
}

var mergeIDs = []string{
	"mrvf-20260901-000000000000000-pr-ready-a-001", "mrvf-20260901-000000000000000-pr-ready-a-002",
	"mrvf-20260902-000000000000000-pr-ready-b-001", "mrvf-20260902-000000000000000-pr-ready-b-002",
	"mrvf-20260903-000000000000000-pr-ready-c-001", "mrvf-20260903-000000000000000-pr-ready-c-002",
}

// AC-5.6 (#181), the observed behavior: two branches that each regenerate FINDINGS.md with a new blocker and
// a new override conflict on a plain merge — both insert at the end of the same two lists — and a human
// resolving the markers by hand can drop either side's line.
func TestFindingsIndexConflictsOnAPlainMerge(t *testing.T) {
	err, merged := divergeAndMerge(t, "")
	if err == nil || !strings.Contains(merged, "<<<<<<<") {
		t.Fatalf("expected a conflict on a plain merge (the #181 report); got err=%v\n%s", err, merged)
	}
}

// With the repository's own .gitattributes, the same merge completes with no conflict and keeps every
// blocker and every override of both branches (#181).
func TestFindingsIndexMergesWithoutConflictUnderTheRepositoryAttributes(t *testing.T) {
	attributes, err := os.ReadFile(filepath.Join("..", "..", ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	err, merged := divergeAndMerge(t, string(attributes))
	if err != nil || strings.Contains(merged, "<<<<<<<") {
		t.Fatalf("the merge must complete without conflict: err=%v\n%s", err, merged)
	}
	for _, id := range mergeIDs {
		if !strings.Contains(merged, "- "+id+" [") {
			t.Errorf("merged FINDINGS.md lost %s:\n%s", id, merged)
		}
	}
	// The next render — here from a fresh clone's empty ledger — normalizes the merged file: every line
	// carried once, in the section it belongs to.
	root := t.TempDir()
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(merged), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RenderIndexWithRecords(root, nil); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Each line as often as a canonical render of both branches' records gives it (a pending override is
	// both a blocker and an override entry).
	canonical := t.TempDir()
	var all []Record
	for i, id := range mergeIDs {
		if i%2 == 0 {
			all = append(all, mergeBlocker(id))
		} else {
			all = append(all, pendingOverride(id))
		}
	}
	if err := RenderIndexWithRecords(canonical, all); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(canonical, "docs", "metareview", "FINDINGS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range mergeIDs {
		if got, want := strings.Count(string(rendered), "- "+id+" ["), strings.Count(string(want), "- "+id+" ["); got != want {
			t.Errorf("%s rendered %d times after the merge, %d in a canonical render:\n%s", id, got, want, rendered)
		}
	}
	if n := strings.Count(string(rendered), "## Process Overrides"); n != 1 {
		t.Errorf("Process Overrides rendered %d times:\n%s", n, rendered)
	}
}
