package githubcontext

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// withSeams installs fake external-process seams for the test: lookGh returns lookErr, and runCommand
// dispatches to run. Both are restored on cleanup. No subprocess is spawned, so these tests are fast and
// gremlins can mutation-test Collect without per-mutant process spawns timing out.
func withSeams(t *testing.T, lookErr error, run func(root, name string, args ...string) (string, error)) {
	t.Helper()
	origRun, origLook := runCommand, lookGh
	t.Cleanup(func() { runCommand, lookGh = origRun, origLook })
	lookGh = func() error { return lookErr }
	runCommand = run
}

// ghScript returns a runCommand that answers the git-remote / gh-auth / gh-pr-view calls Collect makes.
func ghScript(remote string, remoteErr, authErr, prErr error, prOut string) func(root, name string, args ...string) (string, error) {
	return func(root, name string, args ...string) (string, error) {
		switch {
		case name == "git":
			return remote, remoteErr
		case name == "gh" && len(args) > 0 && args[0] == "auth":
			return "", authErr
		case name == "gh" && len(args) > 0 && args[0] == "pr":
			return prOut, prErr
		}
		return "", errors.New("unexpected command")
	}
}

func TestCollectUnavailableWithoutPRNumber(t *testing.T) {
	ctx, err := Collect(t.TempDir(), "")
	if err != nil || ctx.Available || ctx.UnavailableReason != "pr-number-unavailable" {
		t.Fatalf("want pr-number-unavailable, got err=%v ctx=%+v", err, ctx)
	}
}

func TestCollectUnavailableWithoutGh(t *testing.T) {
	// Exercises the real lookGh seam (a fast filesystem check): gh absent from PATH -> gh-unavailable.
	t.Setenv("PATH", t.TempDir())
	ctx, err := Collect(t.TempDir(), "12")
	if err != nil || ctx.Available || ctx.UnavailableReason != "gh-unavailable" {
		t.Fatalf("want gh-unavailable, got err=%v ctx=%+v", err, ctx)
	}
}

func TestCollectUnavailableWithoutRemote(t *testing.T) {
	withSeams(t, nil, ghScript("", nil, nil, nil, "")) // empty remote
	ctx, _ := Collect(t.TempDir(), "12")
	if ctx.Available || ctx.UnavailableReason != "remote-unavailable" {
		t.Fatalf("want remote-unavailable, got %+v", ctx)
	}
}

func TestCollectUnavailableWhenGhAuthFails(t *testing.T) {
	withSeams(t, nil, ghScript("https://github.com/acme/repo.git", nil, errors.New("auth"), nil, ""))
	ctx, _ := Collect(t.TempDir(), "12")
	if ctx.Available || ctx.UnavailableReason != "gh-auth-unavailable" {
		t.Fatalf("want gh-auth-unavailable, got %+v", ctx)
	}
}

func TestCollectUnavailableWhenPRViewFails(t *testing.T) {
	withSeams(t, nil, ghScript("https://github.com/acme/repo.git", nil, nil, errors.New("pr view"), ""))
	ctx, _ := Collect(t.TempDir(), "12")
	if ctx.Available || ctx.UnavailableReason != "github-pr-unavailable" {
		t.Fatalf("want github-pr-unavailable, got %+v", ctx)
	}
}

func TestCollectErrorsOnMalformedJSON(t *testing.T) {
	withSeams(t, nil, ghScript("https://github.com/acme/repo.git", nil, nil, nil, "this is not json"))
	if _, err := Collect(t.TempDir(), "12"); err == nil {
		t.Fatal("expected an error when gh returns malformed JSON")
	}
}

func TestCollectSummarizesAndRedactsGitHubText(t *testing.T) {
	longComment := strings.Repeat("x", maxExcerptRunes+25)
	credentialValue := "redaction-test-value"
	bearerValue := "redaction-bearer-value"
	payload := map[string]any{
		"number":         12,
		"url":            "https://github.com/acme/repo/pull/12",
		"title":          "Improve parser",
		"body":           "PR body contains token=" + credentialValue,
		"reviewDecision": "APPROVED",
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
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	withSeams(t, nil, ghScript("https://github.com/acme/repo.git", nil, nil, nil, string(raw)))

	ctx, err := Collect(t.TempDir(), "12")
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
	if !strings.Contains(rendered, "Review decision: APPROVED") {
		t.Fatalf("rendered markdown missing review decision:\n%s", rendered)
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

// realCommand is the production seam; these direct tests cover its success and both failure branches
// (stderr present, and stderr empty -> fall back to err.Error()), the latter killing the line-137 mutant.
func TestRealCommandSuccess(t *testing.T) {
	out, err := realCommand(t.TempDir(), "echo", "seam-ok")
	if err != nil || out != "seam-ok" {
		t.Fatalf("realCommand echo = %q, %v; want \"seam-ok\", nil", out, err)
	}
}

func TestRealCommandErrorUsesStderr(t *testing.T) {
	// A command that fails while writing to stderr: the returned message is that stderr text.
	_, err := realCommand(t.TempDir(), "ls", "/metareview-no-such-path-xyz123")
	if err == nil || err.Error() == "" {
		t.Fatalf("ls of a missing path should error with stderr text, got %v", err)
	}
}

func TestRealCommandErrorFallsBackWhenStderrEmpty(t *testing.T) {
	// A nonexistent binary fails before writing stderr, so command falls back to err.Error() (non-empty).
	_, err := realCommand(t.TempDir(), "metareview-no-such-binary-xyz123")
	if err == nil || err.Error() == "" {
		t.Fatalf("nonexistent binary should error with a non-empty message, got %v", err)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "\t"); got != "" {
		t.Fatalf("all-empty inputs should yield %q, got %q", "", got)
	}
	if got := firstNonEmpty("", "second"); got != "second" {
		t.Fatalf("should skip the empty value and return the first non-empty, got %q", got)
	}
}

func TestRenderMarkdownUnavailable(t *testing.T) {
	if out := RenderMarkdown(Context{Available: false, UnavailableReason: "gh-unavailable"}); !strings.Contains(out, "GitHub context unavailable: gh-unavailable") {
		t.Fatalf("unavailable render wrong: %q", out)
	}
	if out := RenderMarkdown(Context{Available: false}); !strings.Contains(out, "unavailable: unknown") {
		t.Fatalf("empty reason should render 'unknown': %q", out)
	}
}

func TestRenderMarkdownFull(t *testing.T) {
	ctx := Context{
		Available: true, PRNumber: "12", URL: "https://github.com/acme/repo/pull/12",
		Title: "Improve parser", Body: "body text", ReviewDecision: "APPROVED",
		Comments: []Entry{{Author: "alice", URL: "u1", Body: "nice"}},
		Reviews:  []Entry{{Author: "bob", URL: "u2", State: "CHANGES_REQUESTED", Body: "lgtm"}},
	}
	out := RenderMarkdown(ctx)
	for _, want := range []string{
		"- PR: https://github.com/acme/repo/pull/12", "- Title: Improve parser",
		"- Review decision: APPROVED", "- Body excerpt: body text",
		"Comments:", "- alice u1: nice", "Reviews:", "- CHANGES_REQUESTED by bob u2: lgtm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderMarkdownOmitsEmptyOptionalFields(t *testing.T) {
	out := RenderMarkdown(Context{Available: true, URL: "u", Title: "t"})
	for _, absent := range []string{"Review decision:", "Body excerpt:", "Comments:", "Reviews:"} {
		if strings.Contains(out, absent) {
			t.Fatalf("empty optional %q should be omitted: %q", absent, out)
		}
	}
	// an entry with an empty author renders "unknown"; empty state/url/body branches are skipped.
	out2 := RenderMarkdown(Context{Available: true, URL: "u", Title: "t", Comments: []Entry{{}}})
	if !strings.Contains(out2, "- unknown\n") {
		t.Fatalf("empty author should render 'unknown' with no trailing fields: %q", out2)
	}
}

func TestRedactMatchSeparatorAtStart(t *testing.T) {
	// A value whose separator is at index 0 has an empty prefix, so it is not a recognized
	// credential key and the whole match is redacted. (The prefix-preservation path is only for
	// genuine token/secret/password/api_key keys; see TestRedactWholeSecretsLeakNoPrefix.)
	if got := redactMatch(":=value"); got != redactionMarker {
		t.Fatalf("redactMatch(\":=value\") = %q, want %q", got, redactionMarker)
	}
}

// A whole-secret match (a PEM private key block, or a key=value whose value itself contains ':')
// must not leak any of the secret material through redactMatch's key-prefix preservation. The old
// logic preserved value[:index] up to the first ':' anywhere, then the first '=', which leaked the
// PEM body (base64 '=' padding sits at the end) and the value prefix before an in-value ':'.
func TestRedactWholeSecretsLeakNoPrefix(t *testing.T) {
	// ghctx-1: an unencrypted PEM key whose base64 body ends in '=' padding.
	pemBody := "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwZZTOPSECRETKEYMATERIALabcd=="
	pem := "-----BEGIN PRIVATE KEY-----\n" + pemBody + "\n-----END PRIVATE KEY-----"
	if red := Redact(pem); strings.Contains(red, "MIIEvQ") || strings.Contains(red, "TOPSECRETKEYMATERIAL") {
		t.Fatalf("PEM key body leaked through redaction: %q", red)
	}
	// ghctx-2: a token whose value contains a ':' — the leftmost separator is the '=' key boundary,
	// so nothing of the value may survive.
	if red := Redact("token=abc:def:ghi"); strings.Contains(red, "abc") || strings.Contains(red, "def") {
		t.Fatalf("token value leaked through redaction: %q", red)
	}
	if red := Redact(`secret="a:b:c"`); strings.Contains(red, "a:b") {
		t.Fatalf("quoted secret value leaked through redaction: %q", red)
	}
	// A genuine key=value still keeps its key name (provenance) while dropping the value.
	if red := Redact("token=plainsecret"); !strings.Contains(red, "token=[REDACTED]") {
		t.Fatalf("legitimate key prefix should be preserved: %q", red)
	}
}

func TestExcerptExactlyAtBoundary(t *testing.T) {
	// Exactly maxExcerptRunes runes must NOT be truncated (no "..." suffix). The <= -> < boundary
	// mutant (line 222) would truncate and append "...".
	s := strings.Repeat("x", maxExcerptRunes)
	if got := excerpt(s); got != s {
		t.Fatalf("excerpt at exactly maxExcerptRunes should be unchanged, got %d runes ending %q", len([]rune(got)), got[max(0, len(got)-4):])
	}
}
