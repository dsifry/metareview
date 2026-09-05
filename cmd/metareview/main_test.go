package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/evidence"
	fsmrun "github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/setup"
)

// runCLI drives the CLI entry point exactly as main does, but with explicit IO and working
// directory, and returns the exit code plus captured stdout/stderr.
func runCLI(t *testing.T, cwd string, stdin io.Reader, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	code := run(args, stdin, &out, &errb, cwd)
	return code, out.String(), errb.String()
}

// gitRepo builds a git repo (config isolated for determinism) with a base commit on main and a
// feature branch carrying a task file and a small change, so review commands have a base..head diff.
func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	write("seed.txt", "seed\n")
	run("add", ".")
	run("commit", "-m", "initial")
	run("checkout", "-q", "-b", "feature")
	write("docs/tasks/t.md", "# Task\n\nDo the thing.\n")
	write("src/a.go", "package src\n\nvar A = 1\n")
	run("add", "-A")
	run("commit", "-m", "feature change")
	return root
}

// --- top-level dispatch ---

func TestRunHelpAndVersion(t *testing.T) {
	cwd := t.TempDir()
	for _, args := range [][]string{{}, {"--help"}, {"-h"}} {
		code, out, _ := runCLI(t, cwd, nil, args...)
		if code != 0 || !strings.Contains(out, "Usage:") {
			t.Errorf("args %v: code=%d out=%q", args, code, out)
		}
	}
	for _, args := range [][]string{{"--version"}, {"-v"}} {
		code, out, _ := runCLI(t, cwd, nil, args...)
		if code != 0 || strings.TrimSpace(out) == "" {
			t.Errorf("args %v: code=%d out=%q", args, code, out)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, errOut := runCLI(t, t.TempDir(), nil, "bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown command") {
		t.Fatalf("code=%d errOut=%q", code, errOut)
	}
}

func TestRunStatusPlain(t *testing.T) {
	code, out, _ := runCLI(t, gitRepo(t), nil, "status")
	if code != 0 || !strings.Contains(out, "mode:") || !strings.Contains(out, "git: present") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestRunStatusJSON(t *testing.T) {
	root := gitRepo(t)
	cases := [][]string{
		{"status", "--json"},
		{"status", "--json", "--target", "docs/tasks/t.md"},
		{"status", "--json", "--scope=branch"},
		{"status", "--json", "--scope", "branch"},
		{"status", "--json", "--scope", "branch", "--base", "main"},
	}
	for _, args := range cases {
		code, out, errOut := runCLI(t, root, nil, args...)
		if code != 0 && code != 1 {
			t.Errorf("args %v: unexpected code=%d errOut=%q", args, code, errOut)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("args %v: expected JSON output", args)
		}
	}
}

func TestRunStatusJSONUsageError(t *testing.T) {
	code, _, errOut := runCLI(t, gitRepo(t), nil, "status", "--json", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Usage: metareview status --json") {
		t.Fatalf("code=%d errOut=%q", code, errOut)
	}
}

func TestRunContextBuild(t *testing.T) {
	root := gitRepo(t)
	code, out, errOut := runCLI(t, root, nil, "context", "build", "docs/tasks/t.md")
	if code != 0 || !strings.Contains(out, "docs/metareview/context/") {
		t.Fatalf("code=%d out=%q errOut=%q", code, out, errOut)
	}
}

func TestRunContextDiff(t *testing.T) {
	root := gitRepo(t)
	code, out, _ := runCLI(t, root, nil, "context", "diff", "--base", "main")
	if code != 0 || !strings.Contains(out, "\"") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, _, errOut := runCLI(t, root, nil, "context", "diff", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewArtifact(t *testing.T) {
	root := gitRepo(t)
	// Without --scaffold-only, the scaffold is created but the command exits 1 (not completed).
	code, out, errOut := runCLI(t, root, nil, "review", "artifact", "docs/tasks/t.md")
	if code != 1 || !strings.Contains(out, "docs/metareview/reviews/") || !strings.Contains(errOut, "not completed") {
		t.Fatalf("code=%d out=%q errOut=%q", code, out, errOut)
	}
	// With --scaffold-only, exit 0.
	code, _, _ = runCLI(t, root, nil, "review", "artifact", "docs/tasks/t.md", "--scaffold-only")
	if code != 0 {
		t.Fatalf("scaffold-only: code=%d", code)
	}
	// Unknown option.
	code, _, errOut = runCLI(t, root, nil, "review", "artifact", "docs/tasks/t.md", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewTaskDoneBlocking(t *testing.T) {
	root := gitRepo(t)
	// No lens marker recorded, so the require-lenses gate blocks -> exit 1 with a review path.
	code, out, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "main")
	if code != 1 || !strings.Contains(out, "docs/metareview/reviews/") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, _, errOut := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewPrReadyBlocking(t *testing.T) {
	root := gitRepo(t)
	code, out, _ := runCLI(t, root, nil, "review", "pr-ready", "--base", "main", "--include-working-tree")
	if code != 1 || !strings.Contains(out, "docs/metareview/reviews/") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, _, errOut := runCLI(t, root, nil, "review", "pr-ready", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewEpicReadyUnknownOption(t *testing.T) {
	code, _, errOut := runCLI(t, gitRepo(t), nil, "review", "epic-ready", "docs/tasks/t.md", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewPrompt(t *testing.T) {
	root := gitRepo(t)
	code, out, _ := runCLI(t, root, nil, "review", "prompt", "--base", "main")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, _, errOut := runCLI(t, root, nil, "review", "prompt", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunReviewGate(t *testing.T) {
	root := gitRepo(t)
	// A clean staged index gates green (nothing owed on the staged scope) -> exit 0.
	code, _, _ := runCLI(t, root, nil, "review", "gate")
	if code != 0 && code != 1 {
		t.Fatalf("gate: unexpected code=%d", code)
	}
	// --all and --push scopes are accepted.
	for _, args := range [][]string{{"review", "gate", "--all"}, {"review", "gate", "--push", "--base", "main"}} {
		if code, _, _ := runCLI(t, root, nil, args...); code != 0 && code != 1 {
			t.Errorf("args %v: unexpected code=%d", args, code)
		}
	}
	// --pre-push-stdin reads ref lines from stdin; empty stdin means nothing to push -> not blocked.
	code, _, _ = runCLI(t, root, strings.NewReader(""), "review", "gate", "--push", "--pre-push-stdin")
	if code != 0 {
		t.Fatalf("pre-push-stdin empty: code=%d", code)
	}
	code, _, errOut := runCLI(t, root, nil, "review", "gate", "--bogus")
	if code != 2 || !strings.Contains(errOut, "Unknown option") {
		t.Fatalf("unknown option: code=%d errOut=%q", code, errOut)
	}
}

func TestRunRecordLenses(t *testing.T) {
	root := gitRepo(t)
	// Success (in-session-emulated).
	code, out, errOut := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "main", "--lenses", "correctness")
	if code != 0 || !strings.Contains(out, "Recorded pr-ready review-evidence") {
		t.Fatalf("code=%d out=%q errOut=%q", code, out, errOut)
	}
	// Invalid scope.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--scope", "bogus", "--lenses", "x"); code != 2 || !strings.Contains(e, "--scope must be") {
		t.Errorf("scope: code=%d e=%q", code, e)
	}
	// Invalid mode.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--mode", "bogus", "--lenses", "x"); code != 2 || !strings.Contains(e, "--mode must be") {
		t.Errorf("mode: code=%d e=%q", code, e)
	}
	// No lenses.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--base", "main"); code != 2 || !strings.Contains(e, "--lenses must name") {
		t.Errorf("lenses: code=%d e=%q", code, e)
	}
	// subagent-adjudicated without --from-run.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--mode", "subagent-adjudicated", "--base", "main", "--lenses", "x"); code != 2 || !strings.Contains(e, "requires --from-run") {
		t.Errorf("from-run required: code=%d e=%q", code, e)
	}
	// subagent-adjudicated with a traversal-y from-run.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--mode", "subagent-adjudicated", "--from-run", "../evil", "--base", "main", "--lenses", "x"); code != 2 || !strings.Contains(e, "not a valid run id") {
		t.Errorf("from-run traversal: code=%d e=%q", code, e)
	}
	// Unknown option.
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Errorf("unknown option: code=%d e=%q", code, e)
	}
}

func TestRunLearn(t *testing.T) {
	root := gitRepo(t)
	// Help.
	if code, out, _ := runCLI(t, root, nil, "learn", "--help"); code != 0 || !strings.Contains(out, "learn") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
	// Missing --post-merge.
	if code, _, e := runCLI(t, root, nil, "learn", "--base", "main"); code != 2 || !strings.Contains(e, "Missing value for --post-merge") {
		t.Errorf("missing post-merge: code=%d e=%q", code, e)
	}
	// Unknown option.
	if code, _, e := runCLI(t, root, nil, "learn", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Errorf("unknown option: code=%d e=%q", code, e)
	}
}

func TestRunEvidence(t *testing.T) {
	root := gitRepo(t)
	// Help.
	if code, out, _ := runCLI(t, root, nil, "evidence"); code != 0 || !strings.Contains(out, "evidence run") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
	// run success.
	if code, out, _ := runCLI(t, root, nil, "evidence", "run", "--", "true"); code != 0 || !strings.Contains(out, "\"exitCode\"") {
		t.Errorf("run: code=%d out=%q", code, out)
	}
	// run with a failing command surfaces the command's exit code.
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "false"); code == 0 {
		t.Errorf("run false: expected nonzero code, got %d", code)
	}
	// run missing separator.
	if code, _, e := runCLI(t, root, nil, "evidence", "run"); code != 2 || !strings.Contains(e, "evidence run --") {
		t.Errorf("missing sep: code=%d e=%q", code, e)
	}
	// import missing pr.
	if code, _, e := runCLI(t, root, nil, "evidence", "import"); code != 2 || !strings.Contains(e, "Missing value for --github-checks") {
		t.Errorf("import missing pr: code=%d e=%q", code, e)
	}
	// import unknown option.
	if code, _, e := runCLI(t, root, nil, "evidence", "import", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Errorf("import unknown: code=%d e=%q", code, e)
	}
	// unknown evidence subcommand.
	if code, _, e := runCLI(t, root, nil, "evidence", "bogus"); code != 2 || !strings.Contains(e, "Unknown evidence command") {
		t.Errorf("unknown sub: code=%d e=%q", code, e)
	}
}

func TestRunOverride(t *testing.T) {
	root := gitRepo(t)
	// Usage with no subcommand.
	if code, _, e := runCLI(t, root, nil, "override"); code != 2 || !strings.Contains(e, "Usage: metareview override") {
		t.Errorf("usage: code=%d e=%q", code, e)
	}
	// list on an empty store.
	if code, out, _ := runCLI(t, root, nil, "override", "list"); code != 0 || !strings.Contains(out, "no process overrides") {
		t.Errorf("list empty: code=%d out=%q", code, out)
	}
	// list unknown option.
	if code, _, e := runCLI(t, root, nil, "override", "list", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Errorf("list unknown: code=%d e=%q", code, e)
	}
	// request missing id.
	if code, _, e := runCLI(t, root, nil, "override", "request"); code != 2 || !strings.Contains(e, "override request <finding-id>") {
		t.Errorf("request missing id: code=%d e=%q", code, e)
	}
	// unknown subcommand.
	if code, _, e := runCLI(t, root, nil, "override", "bogus"); code != 2 || !strings.Contains(e, "Usage: metareview override") {
		t.Errorf("unknown sub: code=%d e=%q", code, e)
	}
}

func TestRunSetup(t *testing.T) {
	root := gitRepo(t)
	// --check emits JSON.
	if code, out, _ := runCLI(t, root, nil, "setup", "--check"); code != 0 || !strings.Contains(out, "{") {
		t.Errorf("check: code=%d out=%q", code, out)
	}
	// No flags -> usage.
	if code, _, e := runCLI(t, root, nil, "setup"); code != 2 || !strings.Contains(e, "Usage: metareview setup") {
		t.Errorf("no flags: code=%d e=%q", code, e)
	}
	// Both install and uninstall -> error.
	if code, _, e := runCLI(t, root, nil, "setup", "--install-hooks", "--uninstall-hooks"); code != 2 || !strings.Contains(e, "not both") {
		t.Errorf("both: code=%d e=%q", code, e)
	}
	// Unknown option.
	if code, _, e := runCLI(t, root, nil, "setup", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Errorf("unknown: code=%d e=%q", code, e)
	}
	// bootstrap dry-run.
	if code, out, _ := runCLI(t, root, nil, "setup", "--bootstrap-prereqs", "--dry-run"); code != 0 || !strings.Contains(out, "bootstrap plan") {
		t.Errorf("bootstrap: code=%d out=%q", code, out)
	}
}

func TestRunSetupInstallHooks(t *testing.T) {
	root := gitRepo(t)
	// dry-run previews without changing anything.
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--dry-run"); code != 0 || !strings.Contains(out, "dry run") {
		t.Errorf("install dry-run: code=%d out=%q", code, out)
	}
	// No TTY, no --yes: shows directions, changes nothing.
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks"); code != 0 || !strings.Contains(out, "NOTHING was changed") {
		t.Errorf("install no-tty: code=%d out=%q", code, out)
	}
	// --yes installs.
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes"); code != 0 || !strings.Contains(out, "Installed") {
		t.Errorf("install yes: code=%d out=%q", code, out)
	}
	// Already installed.
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes"); code != 0 || !strings.Contains(out, "Already installed") {
		t.Errorf("install again: code=%d out=%q", code, out)
	}
	// uninstall --yes.
	if code, out, _ := runCLI(t, root, nil, "setup", "--uninstall-hooks", "--yes"); code != 0 || !strings.Contains(out, "uninstalled") {
		t.Errorf("uninstall: code=%d out=%q", code, out)
	}
	// uninstall when nothing installed.
	if code, out, _ := runCLI(t, root, nil, "setup", "--uninstall-hooks", "--dry-run"); code != 0 || !strings.Contains(out, "Nothing to uninstall") {
		t.Errorf("uninstall dry-run nothing: code=%d out=%q", code, out)
	}
}

// The interactive prompt path is driven by overriding the isTTY seam and feeding stdin.
func TestRunSetupInstallHooksInteractive(t *testing.T) {
	root := gitRepo(t)
	orig := isTTY
	t.Cleanup(func() { isTTY = orig })
	isTTY = func(*os.File) bool { return true }
	// "y" proceeds with the install.
	if code, out, _ := runCLI(t, root, strings.NewReader("y\n"), "setup", "--install-hooks"); code != 0 || !strings.Contains(out, "Installed") {
		t.Errorf("interactive yes: code=%d out=%q", code, out)
	}
	// "n" (via uninstall prompt) makes no change.
	if code, out, _ := runCLI(t, root, strings.NewReader("n\n"), "setup", "--uninstall-hooks"); code != 0 || !strings.Contains(out, "No changes made") {
		t.Errorf("interactive no: code=%d out=%q", code, out)
	}
}

// --- direct helper tests ---

func TestValidateFromRunDiff(t *testing.T) {
	root := t.TempDir()
	writeAudit := func(runID string, lines ...string) {
		p := filepath.Join(root, ".metareview", "runs", runID, "audit.jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	initEvent := func(base, head, workflow string) string {
		d, _ := json.Marshal(fsmrun.InitData{BaseSHA: base, Head: head, Workflow: workflow})
		e, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeInit, Data: d})
		return string(e)
	}
	transition := func(outcome fsmrun.Outcome) string {
		d, _ := json.Marshal(fsmrun.TransitionData{Outcome: outcome})
		e, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeTransition, Data: d})
		return string(e)
	}

	// Missing run dir.
	if err := validateFromRunDiff(root, "ghost", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "no such FSM run") {
		t.Errorf("missing run: %v", err)
	}
	// Malformed event line.
	writeAudit("bad", "{not json")
	if err := validateFromRunDiff(root, "bad", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "malformed event") {
		t.Errorf("malformed: %v", err)
	}
	// First event not init.
	writeAudit("noinit", transition(fsmrun.OutcomeReviewed))
	if err := validateFromRunDiff(root, "noinit", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "does not start with a valid init") {
		t.Errorf("no init: %v", err)
	}
	// Diff mismatch.
	writeAudit("mismatch", initEvent("other", "head2", ""), transition(fsmrun.OutcomeReviewed))
	if err := validateFromRunDiff(root, "mismatch", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "different diff") {
		t.Errorf("mismatch: %v", err)
	}
	// Wrong workflow.
	writeAudit("wf", initEvent("b", "h", "review-loop"), transition(fsmrun.OutcomeReviewed))
	if err := validateFromRunDiff(root, "wf", "b", "h", "epic-review-loop"); err == nil || !strings.Contains(err.Error(), "requires") {
		t.Errorf("workflow: %v", err)
	}
	// Non-passing final outcome.
	writeAudit("failed", initEvent("b", "h", ""), transition(fsmrun.OutcomeReviewed), transition(fsmrun.Outcome("failed")))
	if err := validateFromRunDiff(root, "failed", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "not a passing review") {
		t.Errorf("failed: %v", err)
	}
	// Passing.
	writeAudit("ok", initEvent("b", "h", ""), transition(fsmrun.OutcomeReviewed))
	if err := validateFromRunDiff(root, "ok", "b", "h", ""); err != nil {
		t.Errorf("passing: %v", err)
	}
}

func TestSmallHelpers(t *testing.T) {
	if present(true) != "present" || present(false) != "missing" {
		t.Error("present")
	}
	if short("0123456789abcdef") != "0123456789ab" || short("short") != "short" {
		t.Error("short")
	}
	if !isPassingReviewOutcome(fsmrun.OutcomeClean) || isPassingReviewOutcome(fsmrun.Outcome("failed")) {
		t.Error("isPassingReviewOutcome")
	}
	if executablePath() == "" {
		t.Error("executablePath should resolve the test binary")
	}
}

// catchExit runs fn (which is expected to call exit()) with buffered IO and returns the exit code it
// panicked with, or -1 if it returned normally. Used to drive the panic-based helpers directly.
func catchExit(t *testing.T, fn func()) (code int, errOut string) {
	t.Helper()
	prevOut, prevErr := stdout, stderr
	t.Cleanup(func() { stdout, stderr = prevOut, prevErr })
	var out, errb bytes.Buffer
	stdout, stderr = &out, &errb
	code = -1
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(exitCode); ok {
					code = e.code
					return
				}
				panic(r)
			}
		}()
		fn()
	}()
	return code, errb.String()
}

func TestRealMainAndMain(t *testing.T) {
	// realMain delegates to run for a normal invocation.
	var out bytes.Buffer
	if code := realMain([]string{"--version"}, strings.NewReader(""), &out, &out); code != 0 {
		t.Fatalf("realMain --version: code=%d", code)
	}
	// getwd failure surfaces as exit code 1.
	origGetwd := getwd
	t.Cleanup(func() { getwd = origGetwd })
	getwd = func() (string, error) { return "", os.ErrPermission }
	var errb bytes.Buffer
	if code := realMain([]string{"status"}, strings.NewReader(""), &out, &errb); code != 1 || !strings.Contains(errb.String(), "permission") {
		t.Fatalf("realMain getwd err: code=%d errb=%q", code, errb.String())
	}
	getwd = origGetwd
	// main() wraps realMain in osExit; capture the code instead of exiting.
	origExit, origArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = origExit, origArgs })
	captured := -1
	osExit = func(c int) { captured = c }
	os.Args = []string{"metareview", "--version"}
	main()
	if captured != 0 {
		t.Fatalf("main captured exit=%d", captured)
	}
}

// A non-exit panic inside the dispatch is re-raised by run rather than swallowed.
func TestRunReraisesNonExitPanic(t *testing.T) {
	root := gitRepo(t)
	orig := isTTY
	t.Cleanup(func() { isTTY = orig })
	isTTY = func(*os.File) bool { panic("boom") }
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected the non-exit panic to be re-raised")
		}
		if s, ok := r.(string); !ok || s != "boom" {
			t.Fatalf("unexpected re-raised value: %v", r)
		}
	}()
	// The install prompt path calls isTTY, which panics a plain string (not an exitCode).
	runCLI(t, root, strings.NewReader("y\n"), "setup", "--install-hooks")
}

func TestFlagAndParseHelpers(t *testing.T) {
	// flagValue: missing value (flag at end).
	if code, e := catchExit(t, func() { flagValue([]string{"--base"}, 0, "--base") }); code != 2 || !strings.Contains(e, "Missing value for --base") {
		t.Errorf("flagValue end: code=%d e=%q", code, e)
	}
	// flagValue: next token is another flag.
	if code, _ := catchExit(t, func() { flagValue([]string{"--base", "--other"}, 0, "--base") }); code != 2 {
		t.Errorf("flagValue flag-follows: code=%d", code)
	}
	// flagValue: success returns the value (no exit).
	if code, _ := catchExit(t, func() {
		if v := flagValue([]string{"--base", "main"}, 0, "--base"); v != "main" {
			t.Errorf("flagValue value=%q", v)
		}
	}); code != -1 {
		t.Errorf("flagValue success should not exit: code=%d", code)
	}
	// mustPositiveInt: invalid and non-positive.
	if code, e := catchExit(t, func() { mustPositiveInt("abc", "--max-attempts") }); code != 2 || !strings.Contains(e, "greater than 0") {
		t.Errorf("mustPositiveInt abc: code=%d e=%q", code, e)
	}
	if code, _ := catchExit(t, func() { mustPositiveInt("0", "--max-attempts") }); code != 2 {
		t.Errorf("mustPositiveInt 0: code=%d", code)
	}
	if code, _ := catchExit(t, func() {
		if n := mustPositiveInt("3", "--max-attempts"); n != 3 {
			t.Errorf("mustPositiveInt value=%d", n)
		}
	}); code != -1 {
		t.Errorf("mustPositiveInt success should not exit: code=%d", code)
	}
}

func TestMustResultFileAndMutationReport(t *testing.T) {
	dir := t.TempDir()
	// mustResultFile: missing.
	if code, e := catchExit(t, func() { mustResultFile(filepath.Join(dir, "nope.json")) }); code != 2 || !strings.Contains(e, "does not exist") {
		t.Errorf("result missing: code=%d e=%q", code, e)
	}
	// mustResultFile: a directory.
	if code, e := catchExit(t, func() { mustResultFile(dir) }); code != 2 || !strings.Contains(e, "not a regular file") {
		t.Errorf("result dir: code=%d e=%q", code, e)
	}
	// mustResultFile: not a metareview result (invalid JSON).
	bad := filepath.Join(dir, "bad.json")
	must(t, os.WriteFile(bad, []byte("{not json"), 0o644))
	if code, e := catchExit(t, func() { mustResultFile(bad) }); code != 2 || !strings.Contains(e, "not a metareview review result") {
		t.Errorf("result bad: code=%d e=%q", code, e)
	}
	// mustResultFile: valid.
	good := filepath.Join(dir, "good.json")
	must(t, os.WriteFile(good, []byte(`{"schemaVersion":1,"id":"r","kind":"shard"}`), 0o644))
	if code, _ := catchExit(t, func() {
		if p := mustResultFile(good); p != good {
			t.Errorf("result path=%q", p)
		}
	}); code != -1 {
		t.Errorf("result valid should not exit: code=%d", code)
	}
	// appendCrossShardResult: first ok, repeat errors.
	if code, _ := catchExit(t, func() {
		if got := appendCrossShardResult(nil, good); len(got) != 1 {
			t.Errorf("appendCrossShardResult first: %v", got)
		}
	}); code != -1 {
		t.Errorf("appendCrossShardResult first should not exit: code=%d", code)
	}
	if code, e := catchExit(t, func() { appendCrossShardResult([]string{good}, good) }); code != 2 || !strings.Contains(e, "Repeated --cross-shard-result") {
		t.Errorf("appendCrossShardResult repeat: code=%d e=%q", code, e)
	}
	// mustMutationReport: missing, dir, invalid, valid.
	if code, e := catchExit(t, func() { mustMutationReport(filepath.Join(dir, "nope.json")) }); code != 2 || !strings.Contains(e, "does not exist") {
		t.Errorf("mutation missing: code=%d e=%q", code, e)
	}
	if code, _ := catchExit(t, func() { mustMutationReport(dir) }); code != 2 {
		t.Errorf("mutation dir: code=%d", code)
	}
	if code, _ := catchExit(t, func() { mustMutationReport(bad) }); code != 2 {
		t.Errorf("mutation invalid: code=%d", code)
	}
	report := filepath.Join(dir, "report.json")
	must(t, os.WriteFile(report, []byte(`{"files":{"a.go":{"mutants":[{"status":"killed","mutator":"X","line":1}]}}}`), 0o644))
	if code, _ := catchExit(t, func() {
		if p := mustMutationReport(report); p != report {
			t.Errorf("mutation path=%q", p)
		}
	}); code != -1 {
		t.Errorf("mutation valid should not exit: code=%d", code)
	}
}

func TestExitHelpersAndBundle(t *testing.T) {
	// exitOnErr / exitGateBroken: nil is a no-op; an error exits.
	if code, _ := catchExit(t, func() { exitOnErr(nil) }); code != -1 {
		t.Errorf("exitOnErr nil should not exit: code=%d", code)
	}
	if code, e := catchExit(t, func() { exitOnErr(os.ErrPermission) }); code != 1 || !strings.Contains(e, "permission") {
		t.Errorf("exitOnErr err: code=%d e=%q", code, e)
	}
	if code, _ := catchExit(t, func() { exitGateBroken(nil) }); code != -1 {
		t.Errorf("exitGateBroken nil should not exit: code=%d", code)
	}
	if code, _ := catchExit(t, func() { exitGateBroken(os.ErrPermission) }); code != 2 {
		t.Errorf("exitGateBroken err: code=%d", code)
	}
}

func TestTaskDoneAndPrReadyAllFlags(t *testing.T) {
	root := gitRepo(t)
	dir := t.TempDir()
	evidence := filepath.Join(dir, "ev.md")
	must(t, os.WriteFile(evidence, []byte("- go test ./... — pass\n"), 0o644))
	report := filepath.Join(dir, "report.json")
	must(t, os.WriteFile(report, []byte(`{"files":{"a.go":{"mutants":[{"status":"killed","mutator":"X","line":1}]}}}`), 0o644))
	// task-done with every flag exercises each flag case (result blocks on the require-lenses gate).
	code, out, _ := runCLI(t, root, nil,
		"review", "task-done", "docs/tasks/t.md",
		"--base", "main", "--previous-run", "none", "--max-attempts", "2",
		"--evidence", evidence, "--mutation-report", report)
	if code != 0 && code != 1 {
		t.Fatalf("task-done all flags: code=%d out=%q", code, out)
	}
	// pr-ready with the flag set it accepts.
	code, _, _ = runCLI(t, root, nil,
		"review", "pr-ready", "--base", "main", "--previous-run", "none", "--max-attempts", "2",
		"--evidence", evidence, "--mutation-report", report, "--github-pr", "7")
	if code != 0 && code != 1 {
		t.Fatalf("pr-ready all flags: code=%d", code)
	}
	// epic-ready flags.
	code, _, _ = runCLI(t, root, nil,
		"review", "epic-ready", "docs/tasks/t.md", "--base", "main", "--previous-run", "none",
		"--max-attempts", "2", "--evidence", evidence, "--mutation-report", report)
	if code != 0 && code != 1 {
		t.Fatalf("epic-ready all flags: code=%d", code)
	}
	// A bad flag value routes through mustPositiveInt -> exit 2.
	if code, _, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--max-attempts", "abc"); code != 2 {
		t.Errorf("task-done bad max-attempts: code=%d", code)
	}
}

func TestContextBuildError(t *testing.T) {
	// A target outside the repo surfaces the build error via exitOnErr -> exit 1.
	if code, _, _ := runCLI(t, gitRepo(t), nil, "context", "build", "does-not-exist.md"); code != 1 {
		t.Fatalf("context build error: code=%d", code)
	}
}

func TestEvidenceRunErrors(t *testing.T) {
	root := gitRepo(t)
	// A command that cannot start (no receipt) surfaces the run error via exitOnErr.
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "/nonexistent/binary-xyz"); code == 0 {
		t.Errorf("evidence run nonexistent: expected nonzero, got 0")
	}
}

func TestOverrideRequestGrantList(t *testing.T) {
	root := gitRepo(t) // has a local git user.email, so defaultActor resolves
	// An override attaches to a real finding, so run a review to record one, then read its id.
	if code, _, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "main"); code != 1 {
		t.Fatalf("seed review: code=%d", code)
	}
	id := firstFindingID(t, root)
	if code, out, e := runCLI(t, root, nil, "override", "request", id, "--reason", "stepped outside the workflow deliberately"); code != 0 || !strings.Contains(out, "override requested") {
		t.Fatalf("request: code=%d out=%q e=%q", code, out, e)
	}
	// list now shows the pending override, and --pending exits 1 while unacknowledged.
	if code, out, _ := runCLI(t, root, nil, "override", "list"); code != 0 || !strings.Contains(out, "pending") {
		t.Fatalf("list pending: code=%d out=%q", code, out)
	}
	if code, _, e := runCLI(t, root, nil, "override", "list", "--pending"); code != 1 || !strings.Contains(e, "awaiting acknowledgement") {
		t.Fatalf("list --pending: code=%d e=%q", code, e)
	}
	// grant from a different actor acknowledges it; list then shows granted.
	if code, out, _ := runCLI(t, root, nil, "override", "grant", id, "--by", "someone-else", "--reason", "exception accepted by maintainer"); code != 0 || !strings.Contains(out, "override granted") {
		t.Fatalf("grant: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, root, nil, "override", "list"); code != 0 || !strings.Contains(out, "granted") {
		t.Fatalf("list granted: code=%d out=%q", code, out)
	}
}

func TestHookInstallConflictAndForce(t *testing.T) {
	root := gitRepo(t)
	// Point core.hooksPath somewhere else so install sees a conflict.
	cmd := exec.Command("git", "config", "core.hooksPath", ".git/other-hooks")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	// Without --force the conflict blocks (exit 1).
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes"); code != 1 || !strings.Contains(out, "CONFLICT") {
		t.Fatalf("conflict: code=%d out=%q", code, out)
	}
	// With --force it installs over the conflict.
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes", "--force"); code != 0 || !strings.Contains(out, "Installed") {
		t.Fatalf("force: code=%d out=%q", code, out)
	}
}

// firstFindingID returns the id of the first finding recorded under the repo, for override tests.
func firstFindingID(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".metareview", "findings.jsonl"))
	if err != nil {
		t.Fatalf("read findings: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.ID != "" {
			return rec.ID
		}
	}
	t.Fatal("no finding id found")
	return ""
}

func TestRunFSM(t *testing.T) {
	// The fsm branch forwards to fsmcli.Run and exits with its code; --agent-prompt prints the
	// driver contract and returns 0.
	code, out, _ := runCLI(t, gitRepo(t), nil, "fsm", "--agent-prompt")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("fsm --agent-prompt: code=%d out=%q", code, out)
	}
}

// After a review records a blocking finding, status --json reports blocked (exit 1) in both the
// default and branch scopes, covering the code!=0 exit paths.
func TestStatusJSONBlocked(t *testing.T) {
	root := gitRepo(t)
	if code, _, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "main"); code != 1 {
		t.Fatalf("seed: code=%d", code)
	}
	if code, out, _ := runCLI(t, root, nil, "status", "--json"); code != 1 || strings.TrimSpace(out) == "" {
		t.Fatalf("status --json blocked: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, root, nil, "status", "--json", "--scope", "branch", "--base", "main"); code != 1 || strings.TrimSpace(out) == "" {
		t.Fatalf("status --json branch blocked: code=%d out=%q", code, out)
	}
}

func TestArtifactPreviousRun(t *testing.T) {
	root := gitRepo(t)
	if code, _, _ := runCLI(t, root, nil, "review", "artifact", "docs/tasks/t.md", "--previous-run", "prev-x", "--scaffold-only"); code != 0 {
		t.Fatalf("artifact --previous-run: code=%d", code)
	}
}

// Non-blocking review returns (exit 0) once a lens marker satisfies the require-lenses gate and
// validation evidence satisfies the test-reviewer.
func TestReviewNonBlockingWithMarker(t *testing.T) {
	for _, scope := range []string{"task-done", "pr-ready", "epic-ready"} {
		root := gitRepo(t)
		ev := filepath.Join(t.TempDir(), "ev.md")
		must(t, os.WriteFile(ev, []byte(`{"schemaVersion":1,"kind":"validation","exitCode":0,"summary":"go test ./... pass"}`+"\n"), 0o644))
		if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--scope", scope, "--base", "main", "--lenses", "correctness"); code != 0 {
			t.Fatalf("%s record-lenses: code=%d e=%q", scope, code, e)
		}
		var args []string
		switch scope {
		case "task-done":
			args = []string{"review", "task-done", "docs/tasks/t.md", "--base", "main", "--evidence", ev}
		case "epic-ready":
			args = []string{"review", "epic-ready", "docs/tasks/t.md", "--base", "main", "--evidence", ev}
		default:
			args = []string{"review", "pr-ready", "--base", "main", "--evidence", ev}
		}
		code, out, errOut := runCLI(t, root, nil, args...)
		if code != 0 {
			t.Fatalf("%s non-blocking: code=%d out=%q err=%q", scope, code, out, errOut)
		}
	}
}

// review prompt with no --base labels from the resolved base SHA, truncated to 12 chars.
func TestReviewPromptNoBase(t *testing.T) {
	code, out, _ := runCLI(t, gitRepo(t), nil, "review", "prompt")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("review prompt no base: code=%d out=%q", code, out)
	}
}

func TestReviewShardResultFlags(t *testing.T) {
	root := gitRepo(t)
	dir := t.TempDir()
	res := filepath.Join(dir, "r.json")
	must(t, os.WriteFile(res, []byte(`{"schemaVersion":1,"id":"r","kind":"shard"}`), 0o644))
	cross := filepath.Join(dir, "c.json")
	must(t, os.WriteFile(cross, []byte(`{"schemaVersion":1,"id":"c","kind":"cross-shard"}`), 0o644))
	// task-done with shard result flags exercises those flag cases.
	if code, _, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "main", "--shard-result", res, "--cross-shard-result", cross); code != 0 && code != 1 {
		t.Errorf("task-done shard flags: code=%d", code)
	}
	// pr-ready with shard result flags.
	if code, _, _ := runCLI(t, root, nil, "review", "pr-ready", "--base", "main", "--shard-result", res, "--cross-shard-result", cross); code != 0 && code != 1 {
		t.Errorf("pr-ready shard flags: code=%d", code)
	}
}

func TestRecordLensesVerdictAndSubagentSuccess(t *testing.T) {
	root := gitRepo(t)
	// --verdict flag.
	if code, _, _ := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "main", "--verdict", "PASS", "--lenses", "security"); code != 0 {
		t.Fatalf("record-lenses --verdict: code=%d", code)
	}
	// subagent-adjudicated success needs a real FSM run whose init base/head match the current diff.
	base, head := diffEndpoints(t, root)
	writeFSMRun(t, root, "fsmrun1", base, head, "")
	if code, out, e := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "main", "--mode", "subagent-adjudicated", "--from-run", "fsmrun1", "--lenses", "security"); code != 0 || !strings.Contains(out, "Recorded") {
		t.Fatalf("record-lenses subagent success: code=%d out=%q e=%q", code, out, e)
	}
}

// diffEndpoints returns the base..head SHAs record-lenses computes for --base main, so a fake FSM
// run's init event can be written to match.
func diffEndpoints(t *testing.T, root string) (string, string) {
	t.Helper()
	code, out, _ := runCLI(t, root, nil, "context", "diff", "--base", "main")
	if code != 0 {
		t.Fatalf("context diff: code=%d", code)
	}
	var ctx struct {
		BaseSHA string `json:"baseSha"`
		HeadSHA string `json:"headSha"`
	}
	if err := json.Unmarshal([]byte(out), &ctx); err != nil {
		t.Fatalf("parse context diff: %v", err)
	}
	if ctx.BaseSHA == "" || ctx.HeadSHA == "" {
		t.Fatalf("empty endpoints: %s", out)
	}
	return ctx.BaseSHA, ctx.HeadSHA
}

func writeFSMRun(t *testing.T, root, runID, base, head, workflow string) {
	t.Helper()
	initD, _ := json.Marshal(fsmrun.InitData{BaseSHA: base, Head: head, Workflow: workflow})
	initE, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeInit, Data: initD})
	trD, _ := json.Marshal(fsmrun.TransitionData{Outcome: fsmrun.OutcomeReviewed})
	trE, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeTransition, Data: trD})
	p := filepath.Join(root, ".metareview", "runs", runID, "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(string(initE)+"\n"+string(trE)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleEvidenceViaSeams(t *testing.T) {
	root := gitRepo(t)
	orig := evidenceRun
	t.Cleanup(func() { evidenceRun = orig })
	// A run that failed to start (no receipt) surfaces the run error.
	evidenceRun = func(context.Context, []string, evidence.RunOptions) (evidence.Receipt, error) {
		return evidence.Receipt{}, os.ErrNotExist
	}
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "x"); code != 1 {
		t.Errorf("run no-receipt: code=%d", code)
	}
	// A completed run that exited nonzero, with a run error, exits with the receipt's code.
	evidenceRun = func(context.Context, []string, evidence.RunOptions) (evidence.Receipt, error) {
		return evidence.Receipt{SchemaVersion: 1, ExitCode: 7}, os.ErrClosed
	}
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "x"); code != 7 {
		t.Errorf("run exitcode w/ err: code=%d", code)
	}
	// A completed run with a run error but exit code 0 exits 1.
	evidenceRun = func(context.Context, []string, evidence.RunOptions) (evidence.Receipt, error) {
		return evidence.Receipt{SchemaVersion: 1, ExitCode: 0}, os.ErrClosed
	}
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "x"); code != 1 {
		t.Errorf("run err exit0: code=%d", code)
	}
	// A clean run that exited nonzero (no run error) exits with that code.
	evidenceRun = func(context.Context, []string, evidence.RunOptions) (evidence.Receipt, error) {
		return evidence.Receipt{SchemaVersion: 1, ExitCode: 3}, nil
	}
	if code, _, _ := runCLI(t, root, nil, "evidence", "run", "--", "x"); code != 3 {
		t.Errorf("run clean nonzero: code=%d", code)
	}

	// import success via the seam (avoids gh + network).
	origImport := importGitHubChecks
	t.Cleanup(func() { importGitHubChecks = origImport })
	importGitHubChecks = func(context.Context, string, evidence.GitHubCheckOptions) (evidence.Bundle, error) {
		return evidence.Bundle{Receipts: []evidence.Receipt{{SchemaVersion: 1, ExitCode: 0}}}, nil
	}
	if code, out, _ := runCLI(t, root, nil, "evidence", "import", "--github-checks", "7", "--repo", "o/r"); code != 0 || strings.TrimSpace(out) == "" {
		t.Errorf("import success: code=%d out=%q", code, out)
	}
	// import whose bundle contains a failing check exits 1.
	importGitHubChecks = func(context.Context, string, evidence.GitHubCheckOptions) (evidence.Bundle, error) {
		return evidence.Bundle{Receipts: []evidence.Receipt{{SchemaVersion: 1, ExitCode: 1}}}, nil
	}
	if code, _, _ := runCLI(t, root, nil, "evidence", "import", "--github-checks", "7"); code != 1 {
		t.Errorf("import failing: code=%d", code)
	}
}

func TestBundleExitCode(t *testing.T) {
	if bundleExitCode(evidence.Bundle{Receipts: []evidence.Receipt{{ExitCode: 0}}}) != 0 {
		t.Error("clean bundle should be 0")
	}
	if bundleExitCode(evidence.Bundle{Receipts: []evidence.Receipt{{ExitCode: 0}, {ExitCode: 2}}}) != 1 {
		t.Error("bundle with a failing receipt should be 1")
	}
}

func TestValidateFromRunDiffUnreadableTransition(t *testing.T) {
	root := t.TempDir()
	initD, _ := json.Marshal(fsmrun.InitData{BaseSHA: "b", Head: "h"})
	initE, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeInit, Data: initD})
	// A transition event whose Data is a JSON string cannot unmarshal into TransitionData.
	badTr, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeTransition, Data: json.RawMessage(`"notanobject"`)})
	p := filepath.Join(root, ".metareview", "runs", "badtr", "audit.jsonl")
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte(string(initE)+"\n"+string(badTr)+"\n"), 0o644))
	if err := validateFromRunDiff(root, "badtr", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "unreadable transition") {
		t.Fatalf("expected unreadable-transition error, got %v", err)
	}
}

func TestSetupBootstrapConfirmationRequired(t *testing.T) {
	// --bootstrap-prereqs without --dry-run and without --confirm needs confirmation -> exit 2.
	code, _, e := runCLI(t, gitRepo(t), nil, "setup", "--bootstrap-prereqs")
	if code != 2 || !strings.Contains(e, "requires --confirm-bootstrap-prereqs") {
		t.Fatalf("bootstrap confirmation: code=%d e=%q", code, e)
	}
}

func TestHookUninstallFlows(t *testing.T) {
	root := gitRepo(t)
	// Install first so uninstall has something to change.
	if code, _, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes"); code != 0 {
		t.Fatalf("install: code=%d", code)
	}
	// uninstall --dry-run previews the would-change.
	if code, out, _ := runCLI(t, root, nil, "setup", "--uninstall-hooks", "--dry-run"); code != 0 || !strings.Contains(out, "dry run") {
		t.Errorf("uninstall dry-run: code=%d out=%q", code, out)
	}
	// uninstall with no TTY and no --yes shows directions, changes nothing.
	if code, out, _ := runCLI(t, root, nil, "setup", "--uninstall-hooks"); code != 0 || !strings.Contains(out, "NOTHING was changed") {
		t.Errorf("uninstall no-tty: code=%d out=%q", code, out)
	}
	// Install again, then --force re-materializes (already-installed --force branch).
	if code, out, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes", "--force"); code != 0 || !strings.Contains(out, "Already installed") {
		t.Errorf("install force: code=%d out=%q", code, out)
	}
}

// The interactive "n" answer on install makes no change.
func TestHookInstallInteractiveDecline(t *testing.T) {
	root := gitRepo(t)
	orig := isTTY
	t.Cleanup(func() { isTTY = orig })
	isTTY = func(*os.File) bool { return true }
	if code, out, _ := runCLI(t, root, strings.NewReader("n\n"), "setup", "--install-hooks"); code != 0 || !strings.Contains(out, "No changes made") {
		t.Fatalf("install decline: code=%d out=%q", code, out)
	}
}

func TestOverrideEscalationAndPendingFilter(t *testing.T) {
	root := gitRepo(t)
	if code, _, _ := runCLI(t, root, nil, "review", "task-done", "docs/tasks/t.md", "--base", "main"); code != 1 {
		t.Fatalf("seed: code=%d", code)
	}
	id := firstFindingID(t, root)
	// request with --escalation.
	if code, _, _ := runCLI(t, root, nil, "override", "request", id, "--reason", "deliberately outside workflow", "--escalation", "context here"); code != 0 {
		t.Fatalf("request escalation: code=%d", code)
	}
	// grant, then list --pending skips the now-granted record (the pending-filter continue).
	if code, _, _ := runCLI(t, root, nil, "override", "grant", id, "--by", "maintainer", "--reason", "accepted the exception"); code != 0 {
		t.Fatalf("grant: code=%d", code)
	}
	if code, out, _ := runCLI(t, root, nil, "override", "list", "--pending"); code != 0 || strings.Contains(out, "pending") {
		t.Fatalf("list --pending after grant: code=%d out=%q", code, out)
	}
}

func TestDefaultActorNoGitConfig(t *testing.T) {
	// With git config fully isolated (no global/system/local user.email) and a non-git working dir,
	// `git config user.email` fails and defaultActor returns "".
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("HOME", t.TempDir())
	orig := workdir
	t.Cleanup(func() { workdir = orig })
	workdir = t.TempDir()
	if got := defaultActor(); got != "" {
		t.Fatalf("defaultActor with no git identity should be empty, got %q", got)
	}
}

func TestExecutablePathError(t *testing.T) {
	orig := osExecutable
	t.Cleanup(func() { osExecutable = orig })
	osExecutable = func() (string, error) { return "", os.ErrNotExist }
	if got := executablePath(); got != "" {
		t.Fatalf("executablePath on error should be empty, got %q", got)
	}
}

func TestMustResultFileUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("unreadable-file permissions do not hold for root")
	}
	p := filepath.Join(t.TempDir(), "r.json")
	must(t, os.WriteFile(p, []byte(`{}`), 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if code, e := catchExit(t, func() { mustResultFile(p) }); code != 2 || !strings.Contains(e, "cannot be read") {
		t.Fatalf("mustResultFile unreadable: code=%d e=%q", code, e)
	}
}

func TestStatusJSONCleanReturns(t *testing.T) {
	// A fresh repo with no review history has nothing to clear: status --json exits 0.
	if code, out, _ := runCLI(t, gitRepo(t), nil, "status", "--json"); code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("clean status --json: code=%d out=%q", code, out)
	}
}

func TestEpicReadyBlocking(t *testing.T) {
	// No marker: the require-lenses gate blocks epic-ready -> exit 1.
	if code, out, _ := runCLI(t, gitRepo(t), nil, "review", "epic-ready", "docs/tasks/t.md", "--base", "main"); code != 1 || !strings.Contains(out, "docs/metareview/reviews/") {
		t.Fatalf("epic-ready blocking: code=%d out=%q", code, out)
	}
}

func TestRecordLensesSubagentEpicAndValidationFailure(t *testing.T) {
	root := gitRepo(t)
	base, head := diffEndpoints(t, root)
	// epic-ready subagent evidence must come from the epic-review-loop workflow.
	writeFSMRun(t, root, "epicrun", base, head, "epic-review-loop")
	if code, out, e := runCLI(t, root, nil, "review", "record-lenses", "--scope", "epic-ready", "--base", "main", "--mode", "subagent-adjudicated", "--from-run", "epicrun", "--lenses", "security"); code != 0 || !strings.Contains(out, "Recorded epic-ready") {
		t.Fatalf("epic subagent: code=%d out=%q e=%q", code, out, e)
	}
	// A from-run that reviewed a different diff fails validation -> exit 2 with the from-run error.
	writeFSMRun(t, root, "wrongdiff", "otherbase", "otherhead", "")
	if code, _, e := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "main", "--mode", "subagent-adjudicated", "--from-run", "wrongdiff", "--lenses", "security"); code != 2 || !strings.Contains(e, "record-lenses: --from-run") {
		t.Fatalf("wrong-diff from-run: code=%d e=%q", code, e)
	}
}

func TestLearnPostMergeFlags(t *testing.T) {
	// Every learn flag is parsed; RunPostMerge then runs (and errors without a real PR/session),
	// exercising the flag cases and the RunPostMerge call.
	root := gitRepo(t)
	code, _, _ := runCLI(t, root, nil, "learn", "--post-merge", "7", "--base", "main", "--github-pr", "7", "--session-root", t.TempDir())
	if code == 2 {
		t.Fatalf("learn flags should parse (RunPostMerge may fail with 1, not a usage error): code=%d", code)
	}
}

func TestValidateFromRunDiffUnreadableInit(t *testing.T) {
	root := t.TempDir()
	// An init event whose Data is a JSON string cannot unmarshal into InitData.
	badInit, _ := json.Marshal(fsmrun.Event{Type: fsmrun.TypeInit, Data: json.RawMessage(`"notanobject"`)})
	p := filepath.Join(root, ".metareview", "runs", "badinit", "audit.jsonl")
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte(string(badInit)+"\n"), 0o644))
	if err := validateFromRunDiff(root, "badinit", "b", "h", ""); err == nil || !strings.Contains(err.Error(), "init event is unreadable") {
		t.Fatalf("expected unreadable-init error, got %v", err)
	}
}

func TestUninstallHookInstallDefensiveBranches(t *testing.T) {
	root := gitRepo(t)
	// Install so the uninstall preview reports WouldChange and reaches the apply call.
	if code, _, _ := runCLI(t, root, nil, "setup", "--install-hooks", "--yes"); code != 0 {
		t.Fatalf("install: code=%d", code)
	}
	orig := uninstallHookInstall
	t.Cleanup(func() { uninstallHookInstall = orig })
	// changed==false: the apply reported nothing changed.
	uninstallHookInstall = func(string, setup.GitRunner) (bool, error) { return false, nil }
	if code, out, _ := runCLI(t, root, nil, "setup", "--uninstall-hooks", "--yes"); code != 0 || !strings.Contains(out, "nothing to uninstall") {
		t.Fatalf("uninstall changed=false: code=%d out=%q", code, out)
	}
	// An apply error exits 1.
	uninstallHookInstall = func(string, setup.GitRunner) (bool, error) { return false, os.ErrPermission }
	if code, _, e := runCLI(t, root, nil, "setup", "--uninstall-hooks", "--yes"); code != 1 || !strings.Contains(e, "metareview:") {
		t.Fatalf("uninstall error: code=%d e=%q", code, e)
	}
}

func TestOverrideRequestUnknownOption(t *testing.T) {
	// The flag loop rejects an unknown option before touching the finding store.
	if code, _, e := runCLI(t, gitRepo(t), nil, "override", "request", "someid", "--bogus"); code != 2 || !strings.Contains(e, "Unknown option") {
		t.Fatalf("override request unknown option: code=%d e=%q", code, e)
	}
}

// After a passing pr-ready review over the branch, branch-scoped status --json has nothing to clear
// and exits 0, covering the clean-return in the branch-scope path.
func TestStatusJSONBranchClean(t *testing.T) {
	root := gitRepo(t)
	ev := filepath.Join(t.TempDir(), "ev.md")
	must(t, os.WriteFile(ev, []byte(`{"schemaVersion":1,"kind":"validation","exitCode":0,"summary":"go test pass"}`+"\n"), 0o644))
	if code, _, _ := runCLI(t, root, nil, "review", "record-lenses", "--scope", "pr-ready", "--base", "main", "--lenses", "correctness"); code != 0 {
		t.Fatalf("record-lenses: code=%d", code)
	}
	if code, _, e := runCLI(t, root, nil, "review", "pr-ready", "--base", "main", "--evidence", ev); code != 0 {
		t.Fatalf("pr-ready non-blocking: code=%d e=%q", code, e)
	}
	if code, out, _ := runCLI(t, root, nil, "status", "--json", "--scope", "branch", "--base", "main"); code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("branch-scope clean status: code=%d out=%q", code, out)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
