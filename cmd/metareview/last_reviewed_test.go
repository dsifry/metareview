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
