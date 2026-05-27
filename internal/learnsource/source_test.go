package learnsource

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectSourcesForPostMergeLearning(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "config", "user.email", "test-user")
	run(t, root, "git", "config", "user.name", "Test User")
	mustWrite(t, filepath.Join(root, "lib", "service.go"), "package lib\nfunc Old() string { return \"old\" }\n")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-qm", "initial")
	base := strings.TrimSpace(command(t, root, "git", "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(root, "lib", "service.go"), "package lib\nfunc New() string { return \"new\" }\n")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-qm", "change service")

	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "review.md"), reviewMarkdown("mrv-task", "task-1", "NEEDS_REVISION", "mrvf-task-001"))
	mustWrite(t, filepath.Join(root, ".metareview", "findings.jsonl"), `{"id":"mrvf-task-001","runId":"mrv-task","status":"open","classification":"blocking","severity":"high","title":"Blocked"}`+"\n")
	mustWrite(t, filepath.Join(root, ".beads", "knowledge", "metareview.jsonl"), `{"fact":"Use existing service inventory before adding new service paths."}`+"\n")

	ctx, err := Collect(root, Options{Base: base, GitHubPR: ""})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Git.BaseSHA != base {
		t.Fatalf("base mismatch: %s", ctx.Git.BaseSHA)
	}
	if !contains(ctx.Git.ChangedFiles, "lib/service.go") || !strings.Contains(ctx.Git.DiffStat, "service.go") {
		t.Fatalf("git summary missing changed file: %+v", ctx.Git)
	}
	if len(ctx.ReviewLogs) != 1 || !ctx.ReviewLogs[0].HasUnresolvedBlockers {
		t.Fatalf("review log summary missing unresolved blocker: %+v", ctx.ReviewLogs)
	}
	if len(ctx.UnresolvedFindings) != 1 || ctx.UnresolvedFindings[0] != "mrvf-task-001" {
		t.Fatalf("unresolved finding IDs not collected: %+v", ctx.UnresolvedFindings)
	}
	if len(ctx.Knowledge.Facts) != 1 || !strings.Contains(ctx.Knowledge.Facts[0].Text, "service inventory") {
		t.Fatalf("knowledge inventory missing fact: %+v", ctx.Knowledge)
	}
	if ctx.GitHub.Available || ctx.GitHub.UnavailableReason != "pr-number-unavailable" {
		t.Fatalf("GitHub unavailable state not recorded: %+v", ctx.GitHub)
	}
}

func TestCollectRedactsGitHubContext(t *testing.T) {
	bin := t.TempDir()
	credentialValue := "redaction-test-value"
	payload := `{"number":42,"url":"https://github.com/acme/repo/pull/42","title":"Fix leak","body":"token=` + credentialValue + `","comments":[{"author":{"login":"alice"},"url":"https://github.com/acme/repo/pull/42#issuecomment-1","body":"secret=` + credentialValue + `"}],"reviews":[]}`
	writeMockGh(t, bin, payload)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "config", "user.email", "test-user")
	run(t, root, "git", "config", "user.name", "Test User")
	run(t, root, "git", "remote", "add", "origin", "https://github.com/acme/repo.git")
	mustWrite(t, filepath.Join(root, "README.md"), "old\n")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-qm", "initial")
	base := strings.TrimSpace(command(t, root, "git", "rev-parse", "HEAD"))
	mustWrite(t, filepath.Join(root, "README.md"), "new\n")
	run(t, root, "git", "add", ".")
	run(t, root, "git", "commit", "-qm", "change")

	ctx, err := Collect(root, Options{Base: base, GitHubPR: "42"})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if !ctx.GitHub.Available {
		t.Fatalf("expected available GitHub context: %+v", ctx.GitHub)
	}
	rendered := ctx.GitHubMarkdown
	for _, forbidden := range []string{credentialValue} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("GitHub markdown leaked %s:\n%s", forbidden, rendered)
		}
	}
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("GitHub markdown missing redaction marker:\n%s", rendered)
	}
}

func reviewMarkdown(runID, target, verdict, findingID string) string {
	return "# metareview: task-done review\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"Target: `" + target + "`\n\n" +
		"## Verdict\n\n" + verdict + "\n\n" +
		"## Findings\n\n### " + findingID + ": Blocked\n"
}

func writeMockGh(t *testing.T, dir, payload string) {
	t.Helper()
	script := `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  cat <<'JSON'
` + payload + `
JSON
  exit 0
fi
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v failed: %v", name, args, err)
	}
}

func command(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v failed: %v", name, args, err)
	}
	return string(out)
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
