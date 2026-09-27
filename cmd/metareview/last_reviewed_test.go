package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitIn(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-q", "-m", name)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

func recordMarker(t *testing.T, dir, scope, verdict string) {
	t.Helper()
	if code, _, errOut := runCLI(t, dir, nil, "review", "record-lenses", "--scope", scope, "--base", "main",
		"--verdict", verdict, "--mode", "in-session-emulated", "--lenses", "security"); code != 0 {
		t.Fatalf("record-lenses %s %s: %d %s", scope, verdict, code, errOut)
	}
}

// AC-3.5 (#176): with passing task-done markers at C1 and C3, on C5 `--base last-reviewed` is C3. A NEEDS_REVISION
// marker and a marker on an unrelated branch are ignored; with no marker the command exits 2 naming the problem.
func TestLastReviewedBase(t *testing.T) {
	root := gitRepo(t) // on feature, one commit ahead of main
	c1 := gitIn(t, root, "rev-parse", "HEAD")
	recordMarker(t, root, "task-done", "PASS")
	commitIn(t, root, "c2.txt")
	c3 := commitIn(t, root, "c3.txt")
	recordMarker(t, root, "task-done", "PASS")
	commitIn(t, root, "c4.txt")
	recordMarker(t, root, "task-done", "NEEDS_REVISION")
	gitIn(t, root, "checkout", "-q", "-b", "unrelated", c1)
	commitIn(t, root, "x.txt")
	recordMarker(t, root, "task-done", "PASS")
	gitIn(t, root, "checkout", "-q", "feature")
	commitIn(t, root, "c5.txt")

	code, out, errOut := runCLI(t, root, nil, "review", "checkpoint", "--scope", "task-done")
	if code != 0 || strings.TrimSpace(out) != c3 {
		t.Fatalf("checkpoint = %d %q %q, want %s", code, out, errOut, c3)
	}
	// A gate run with --base last-reviewed reviews C3..HEAD.
	runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "last-reviewed")
	if base := lastRunBase(t, root, "task-done"); base != c3 {
		t.Fatalf("task-done --base last-reviewed reviewed from %s, want %s", base, c3)
	}
	if runs, _ := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl")); !strings.Contains(string(runs), `"scope":"task-done"`) ||
		!strings.Contains(string(runs), `"requestedBase":"last-reviewed"`) {
		t.Fatalf("the incremental task-done run must record the token:\n%s", runs)
	}
	// No passing pr-ready marker at all: exit 2, naming the problem, and nothing is recorded.
	for _, args := range [][]string{{"review", "checkpoint", "--scope", "pr-ready"}, {"review", "pr-ready", "--base", "last-reviewed"}} {
		code, _, errOut := runCLI(t, root, nil, args...)
		if code != 2 || !strings.Contains(errOut, "no passing pr-ready review") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, errOut)
		}
	}
	if code, _, errOut := runCLI(t, root, nil, "review", "checkpoint", "--scope", "bogus"); code != 2 || !strings.Contains(errOut, "--scope") {
		t.Fatalf("bad scope: %d %q", code, errOut)
	}
	if code, _, errOut := runCLI(t, root, nil, "review", "checkpoint", "--bogus"); code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("bad option: %d %q", code, errOut)
	}

	// A linked worktree uses the markers recorded there.
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, root, "worktree", "add", "-q", "-b", "wt-branch", wt)
	recordMarker(t, wt, "pr-ready", "PASS")
	wtHead := gitIn(t, wt, "rev-parse", "HEAD")
	commitIn(t, wt, "w.txt")
	if code, out, errOut := runCLI(t, wt, nil, "review", "checkpoint", "--scope", "pr-ready"); code != 0 || strings.TrimSpace(out) != wtHead {
		t.Fatalf("worktree checkpoint = %d %q %q, want %s", code, out, errOut, wtHead)
	}
}

// lastRunBase returns the baseSha of the last run record of scope in root's runs.jsonl.
func lastRunBase(t *testing.T, root, scope string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	base := ""
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var rec struct {
			Scope   string `json:"scope"`
			BaseSHA string `json:"baseSha"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Scope == scope {
			base = rec.BaseSHA
		}
	}
	return base
}

// An incremental pr-ready reviews only checkpoint..HEAD, but its BLOCKERS stay scoped to the whole branch: an open
// task-done finding on a file changed before the checkpoint must still block (#176 security review: it was
// projected as "unrelated" and pr-ready passed).
func TestIncrementalPRReadyKeepsWholeBranchBlockers(t *testing.T) {
	root := gitRepo(t)                               // on feature; src/a.go changed
	marker := strings.Join([]string{"TO", "DO"}, "") // the unresolved-work marker the task-done review flags
	if err := os.WriteFile(filepath.Join(root, "src", "a.go"), []byte("package src\n\n// "+marker+": finish\nfunc A() { panic(\""+marker+"\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "a with an unresolved work marker")
	if code, out, errOut := runCLI(t, root, nil, "review", "task-done", "src/a.go", "--base", "main"); code != 1 {
		t.Fatalf("the task-done review must block on the unresolved work marker: %d %s %s", code, out, errOut)
	}
	commitIn(t, root, "b.txt")
	recordMarker(t, root, "pr-ready", "PASS") // the checkpoint: a passing lens review of main..HEAD
	commitIn(t, root, "c.txt")
	if code, _, errOut := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "last-reviewed",
		"--verdict", "PASS", "--mode", "in-session-emulated", "--lenses", "security"); code != 0 {
		t.Fatalf("record-lenses --base last-reviewed: %d %s", code, errOut)
	}
	if runs, _ := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl")); !strings.Contains(string(runs), `"requestedBase":"last-reviewed"`) {
		t.Fatalf("the marker must record the last-reviewed token:\n%s", runs)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.json")
	_, receipt, _ := runCLI(t, root, nil, "evidence", "run", "--", "true")
	if err := os.WriteFile(evidence, []byte(receipt), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, root, nil, "review", "pr-ready", "--base", "last-reviewed", "--evidence", evidence)
	if code != 1 {
		t.Fatalf("an incremental pr-ready must still block on the open task-done finding: %d %s %s", code, out, errOut)
	}
	log, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimSpace(out))))
	if err != nil || !strings.Contains(string(log), "Blocked targets: src/a.go") {
		t.Fatalf("the pr-ready log must name the blocked target (%v):\n%s", err, log)
	}
}

// A marker whose head no longer exists here (a rebased head pruned by gc, or runs.jsonl copied from another clone)
// is skipped, not fatal; and the epic-ready gate resolves the token with its own scope.
func TestLastReviewedSkipsStaleMarkersAndServesEpicReady(t *testing.T) {
	root := gitRepo(t)
	fork := gitIn(t, root, "merge-base", "HEAD", "main")
	recordMarker(t, root, "epic-ready", "PASS")
	checkpoint := gitIn(t, root, "rev-parse", "HEAD")
	stale := `{"schemaVersion":1,"kind":"review-evidence","scope":"review-evidence","reviewedScope":"epic-ready","headSha":"` +
		strings.Repeat("e", 40) + `","baseSha":"` + fork + `","lensSet":["x"],"adjudicatedVerdict":"PASS","executionMode":"in-session-emulated","createdAt":"2026-09-27T00:00:00Z"}` + "\n"
	f, err := os.OpenFile(filepath.Join(root, ".metareview", "runs.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(stale); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	commitIn(t, root, "e2.txt")
	if code, out, errOut := runCLI(t, root, nil, "review", "checkpoint", "--scope", "epic-ready"); code != 0 || strings.TrimSpace(out) != checkpoint {
		t.Fatalf("checkpoint with a stale marker = %d %q %q, want %s", code, out, errOut, checkpoint)
	}
	runCLI(t, root, nil, "review", "epic-ready", "docs/tasks/t.md", "--base", "last-reviewed")
	if base := lastRunBase(t, root, "epic-ready"); base != checkpoint {
		t.Fatalf("epic-ready --base last-reviewed reviewed from %s, want %s", base, checkpoint)
	}
	if runs, _ := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl")); !strings.Contains(string(runs), `"scope":"epic-ready"`) ||
		strings.Count(string(runs), `"requestedBase":"last-reviewed"`) < 1 {
		t.Fatalf("the incremental epic-ready run must record the token:\n%s", runs)
	}
}

// A checkpoint must vouch back to the fork point: a passing review recorded over a narrow base (C2..C3) leaves
// fork..C2 unreviewed, so it is not a checkpoint.
func TestLastReviewedRefusesANarrowBaseReview(t *testing.T) {
	root := gitRepo(t)
	c2 := commitIn(t, root, "c2.txt")
	commitIn(t, root, "c3.txt")
	if code, _, errOut := runCLI(t, root, nil, "review", "record-lenses", "--scope", "task-done", "--base", c2,
		"--verdict", "PASS", "--mode", "in-session-emulated", "--lenses", "security"); code != 0 {
		t.Fatalf("record-lenses: %d %s", code, errOut)
	}
	commitIn(t, root, "c4.txt")
	if code, out, errOut := runCLI(t, root, nil, "review", "checkpoint", "--scope", "task-done"); code != 2 || !strings.Contains(errOut, "fork point") {
		t.Fatalf("a narrow-base review must not be a checkpoint: %d %q %q", code, out, errOut)
	}
}

// Without a local main or master there is no fork point to vouch back to, so last-reviewed is refused rather than
// silently measured from HEAD~1.
func TestLastReviewedNeedsAForkPoint(t *testing.T) {
	root := gitRepo(t)
	recordMarker(t, root, "pr-ready", "PASS")
	commitIn(t, root, "c2.txt")
	gitIn(t, root, "branch", "-q", "-m", "main", "develop")
	for _, args := range [][]string{{"review", "checkpoint", "--scope", "pr-ready"}, {"review", "pr-ready", "--base", "last-reviewed"}} {
		if code, _, errOut := runCLI(t, root, nil, args...); code != 2 || !strings.Contains(errOut, "no local main or master") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, errOut)
		}
	}
}

// With main present but HEAD reachable from it (a detached main tip), the refusal must name that cause — not claim
// main is missing (mr-dvf).
func TestLastReviewedNoForkPointMessageNamesTheCause(t *testing.T) {
	root := gitRepo(t)
	gitIn(t, root, "checkout", "-q", "--detach", "main")
	code, _, errOut := runCLI(t, root, nil, "review", "checkpoint", "--scope", "pr-ready")
	if code != 2 || !strings.Contains(errOut, "no commits of its own") {
		t.Fatalf("code=%d stderr=%q, want the no-commits-of-its-own cause", code, errOut)
	}
}
