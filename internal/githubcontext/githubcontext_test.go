package githubcontext

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectUnavailableWithoutPRNumber(t *testing.T) {
	ctx, err := Collect(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context")
	}
	if ctx.UnavailableReason != "pr-number-unavailable" {
		t.Fatalf("unexpected reason: %s", ctx.UnavailableReason)
	}
}

func TestCollectUnavailableWithoutGh(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx, err := Collect(t.TempDir(), "12")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context")
	}
	if ctx.UnavailableReason != "gh-unavailable" {
		t.Fatalf("unexpected reason: %s", ctx.UnavailableReason)
	}
}

func TestCollectUnavailableWithoutRemote(t *testing.T) {
	bin := t.TempDir()
	writeMockGh(t, bin, `{"number":12}`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()

	ctx, err := Collect(root, "12")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context")
	}
	if ctx.UnavailableReason != "remote-unavailable" {
		t.Fatalf("unexpected reason: %s", ctx.UnavailableReason)
	}
}

func TestCollectUnavailableWhenGhAuthFails(t *testing.T) {
	bin := t.TempDir()
	writeMockGhAuthFailure(t, bin)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "remote", "add", "origin", "https://github.com/acme/repo.git")

	ctx, err := Collect(root, "12")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context")
	}
	if ctx.UnavailableReason != "gh-auth-unavailable" {
		t.Fatalf("unexpected reason: %s", ctx.UnavailableReason)
	}
}

func TestCollectSummarizesAndRedactsGitHubText(t *testing.T) {
	bin := t.TempDir()
	longComment := strings.Repeat("x", maxExcerptRunes+25)
	credentialValue := "redaction-test-value"
	bearerValue := "redaction-bearer-value"
	payload := map[string]any{
		"number": 12,
		"url":    "https://github.com/acme/repo/pull/12",
		"title":  "Improve parser",
		"body":   "PR body contains token=" + credentialValue,
		"comments": []map[string]any{{
			"author": map[string]any{"login": "alice"},
			"url":    "https://github.com/acme/repo/pull/12#issuecomment-1",
			"body":   longComment + " secret=" + credentialValue,
		}},
		"reviews": []map[string]any{{
			"author": map[string]any{"login": "bob"},
			"url":    "https://github.com/acme/repo/pull/12#pullrequestreview-1",
			"body":   "LGTM with Authorization: Bearer " + bearerValue,
			"state":  "APPROVED",
		}},
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	writeMockGh(t, bin, string(bytes))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "remote", "add", "origin", "https://github.com/acme/repo.git")

	ctx, err := Collect(root, "12")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if !ctx.Available {
		t.Fatalf("expected available context: %s", ctx.UnavailableReason)
	}
	rendered := RenderMarkdown(ctx)
	for _, forbidden := range []string{credentialValue, "Bearer " + bearerValue} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered markdown leaked secret-like text: %s", forbidden)
		}
	}
	if !strings.Contains(rendered, "https://github.com/acme/repo/pull/12#issuecomment-1") {
		t.Fatalf("rendered markdown missing comment provenance:\n%s", rendered)
	}
	if len([]rune(ctx.Comments[0].Body)) > maxExcerptRunes+len(redactionMarker)+8 {
		t.Fatalf("comment excerpt was not bounded: %d", len([]rune(ctx.Comments[0].Body)))
	}
}

func TestRedactCommonBareCredentialPatterns(t *testing.T) {
	githubOAuthToken := "gho_" + strings.Repeat("1", 20)
	githubServerToken := "ghs_" + strings.Repeat("2", 20)
	openAIToken := "sk-proj-" + strings.Repeat("a", 24)
	input := "tokens " + githubOAuthToken + " " + githubServerToken + " " + openAIToken
	redacted := Redact(input)
	for _, forbidden := range []string{githubOAuthToken, githubServerToken, openAIToken} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, redactionMarker) {
		t.Fatalf("missing redaction marker: %q", redacted)
	}
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
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeMockGhAuthFailure(t *testing.T, dir string) {
	t.Helper()
	script := `#!/usr/bin/env bash
set -euo pipefail
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  exit 1
fi
exit 0
`
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
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
