# metareview task-done context

Run ID: `mrv-20260902-034259118558000-task-done-githubcontext-37fa1398`

## Task

package githubcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const maxExcerptRunes = 500
const redactionMarker = "[REDACTED]"

type Context struct {
	Available         bool
	UnavailableReason string
	PRNumber          string
	Remote            string
	URL               string
	Title             string
	Body              string
	ReviewDecision    string
	Comments          []Entry
	Reviews           []Entry
}

type Entry struct {
	Author string
	URL    string
	State  string
	Body   string
}

type prView struct {
	Number   int       `json:"number"`
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	Decision string    `json:"reviewDecision"`
	Comments []comment `json:"comments"`
	Reviews  []review  `json:"reviews"`
}

type comment struct {
	Author author `json:"author"`
	URL    string `json:"url"`
	Body   string `json:"body"`
}

type review struct {
	Author author `json:"author"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Body   string `json:"body"`
}

type author struct {
	Login string `json:"login"`
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization:\s*bearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{8,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(token|secret|password|api[_-]?key)\s*[:=]\s*("[^"]+"|'[^']+'|[^\s` + "`" + `,;]+)`),
}

// runCommand and lookGh are the external-process seams. Production shells out (realCommand, exec.LookPath);
// tests inject fast in-process fakes so Collect needs no git/gh/bash subprocess — which keeps the unit
// tests quick and, crucially, makes gremlins mutation testing fast and reliable (per-mutant test reruns no
// longer spawn processes, so parallel workers stop timing out and masking survivors).
var (
	runCommand = realCommand
	lookGh     = func() error { _, err := exec.LookPath("gh"); return err }
)

func Collect(root, prNumber string) (Context, error) {
	prNumber = strings.TrimSpace(prNumber)
	if prNumber == "" {
		return unavailable("pr-number-unavailable", prNumber), nil
	}
	if err := lookGh(); err != nil {
		return unavailable("gh-unavailable", prNumber), nil
	}
	remote, err := runCommand(root, "git", "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(remote) == "" {
		return unavailable("remote-unavailable", prNumber), nil
	}
	if _, err := runCommand(root, "gh", "auth", "status"); err != nil {
		return unavailable("gh-auth-unavailable", prNumber), nil
	}
	out, err := runCommand(root, "gh", "pr", "view", prNumber, "--json", "number,url,title,body,reviewDecision,comments,reviews")
	if err != nil {
		return unavailable("github-pr-unavailable", prNumber), nil
	}
	var parsed prView
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return Context{}, err
	}
	ctx := Context{
		Available:      true,
		PRNumber:       prNumber,
		Remote:         strings.TrimSpace(remote),
		URL:            Redact(parsed.URL),
		Title:          excerpt(Redact(parsed.Title)),
		Body:           excerpt(Redact(parsed.Body)),
		ReviewDecision: excerpt(Redact(parsed.Decision)),
		Comments:       make([]Entry, 0, len(parsed.Comments)),
		Reviews:        make([]Entry, 0, len(parsed.Reviews)),
	}
	for _, item := range parsed.Comments {
		ctx.Comments = append(ctx.Comments, Entry{
			Author: excerpt(Redact(item.Author.Login)),
			URL:    Redact(item.URL),
			Body:   excerpt(Redact(item.Body)),
		})
	}
	for _, item := range parsed.Reviews {
		ctx.Reviews = append(ctx.Reviews, Entry{
			Author: excerpt(Redact(item.Author.Login)),
			URL:    Redact(item.URL),
			State:  excerpt(Redact(item.State)),
			Body:   excerpt(Redact(item.Body)),
		})
	}
	return ctx, nil
}

func unavailable(reason, prNumber string) Context {
	return Context{Available: false, UnavailableReason: reason, PRNumber: prNumber}
}

func realCommand(root, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- fixed git/gh subcommands with caller-provided args
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	return strings.TrimSpace(string(out)), nil
}

func Redact(text string) string {
	redacted := text
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, redactMatch)
	}
	return redacted
}

// credKeyName matches exactly the key names of the token/secret/password/api_key pattern, so only a
// genuine `key<sep>value` match keeps its key as provenance. Any other match (a bare token, a PEM
// block) has no key/value shape and is redacted whole — preserving a "prefix" there leaks the secret.
var credKeyName = regexp.MustCompile(`(?i)^(token|secret|password|api[_-]?key)$`)

func redactMatch(value string) string {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "authorization:") {
		return "Authorization: Bearer " + redactionMarker
	}
	// Use the LEFTMOST ':' or '=' as the key/value boundary (not the first ':' anywhere, then '='):
	// a value that itself contains ':' — e.g. token=a:b — must still split at its real '=' boundary.
	// Preserve the key only when the prefix is one of the recognized credential key names; otherwise
	// the entire match is the secret and is fully redacted.
	if i := strings.IndexAny(value, ":="); i >= 0 {
		if key := strings.TrimSpace(value[:i]); credKeyName.MatchString(key) {
			return key + string(value[i]) + redactionMarker
		}
	}
	return redactionMarker
}

func RenderMarkdown(ctx Context) string {
	if !ctx.Available {
		return "GitHub context unavailable: " + firstNonEmpty(ctx.UnavailableReason, "unknown") + "\n"
	}
	var builder strings.Builder
	builder.WriteString("- PR: ")
	builder.WriteString(firstNonEmpty(Redact(ctx.URL), "unavailable"))
	builder.WriteString("\n")
	builder.WriteString("- Title: ")
	builder.WriteString(excerpt(Redact(ctx.Title)))
	builder.WriteString("\n")
	if strings.TrimSpace(ctx.ReviewDecision) != "" {
		builder.WriteString("- Review decision: ")
		builder.WriteString(excerpt(Redact(ctx.ReviewDecision)))
		builder.WriteString("\n")
	}
	if strings.TrimSpace(ctx.Body) != "" {
		builder.WriteString("- Body excerpt: ")
		builder.WriteString(excerpt(Redact(ctx.Body)))
		builder.WriteString("\n")
	}
	writeEntries(&builder, "Comments", ctx.Comments)
	writeEntries(&builder, "Reviews", ctx.Reviews)
	return builder.String()
}

func writeEntries(builder *strings.Builder, title string, entries []Entry) {
	if len(entries) == 0 {
		return
	}
	builder.WriteString("\n")
	builder.WriteString(title)
	builder.WriteString(":\n")
	for _, entry := range entries {
		builder.WriteString("- ")
		if entry.State != "" {
			builder.WriteString(excerpt(Redact(entry.State)))
			builder.WriteString(" by ")
		}
		builder.WriteString(firstNonEmpty(excerpt(Redact(entry.Author)), "unknown"))
		if entry.URL != "" {
			builder.WriteString(" ")
			builder.WriteString(Redact(entry.URL))
		}
		if strings.TrimSpace(entry.Body) != "" {
			builder.WriteString(": ")
			builder.WriteString(excerpt(Redact(entry.Body)))
		}
		builder.WriteString("\n")
	}
}

func excerpt(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxExcerptRunes {
		return text
	}
	// runes is []rune(text), so string(runes[:n]) re-encodes whole runes and is always valid UTF-8 —
	// no trailing-partial-rune trimming is needed (the former utf8.ValidString loop here was unreachable).
	truncated := string(runes[:maxExcerptRunes])
	return strings.TrimSpace(truncated) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}


## Git

- Base: `cbaa849f15dd585a3b88b00d2f5167d59f990680`
- Head: `87c1fd0352fd51b32bea4c43633d6c527216439e`
- Branch: `redact-secret-leaks`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `4309`
- Filtered diff bytes: `4309`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/githubcontext/githubcontext.go
- internal/githubcontext/githubcontext_test.go

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/githubcontext/githubcontext.go
- internal/githubcontext/githubcontext_test.go

## Diff

```diff
diff --git a/internal/githubcontext/githubcontext.go b/internal/githubcontext/githubcontext.go
index 779f91e..722e48d 100644
--- a/internal/githubcontext/githubcontext.go
+++ b/internal/githubcontext/githubcontext.go
@@ -159,15 +159,23 @@ func Redact(text string) string {
 	return redacted
 }
 
+// credKeyName matches exactly the key names of the token/secret/password/api_key pattern, so only a
+// genuine `key<sep>value` match keeps its key as provenance. Any other match (a bare token, a PEM
+// block) has no key/value shape and is redacted whole — preserving a "prefix" there leaks the secret.
+var credKeyName = regexp.MustCompile(`(?i)^(token|secret|password|api[_-]?key)$`)
+
 func redactMatch(value string) string {
 	lower := strings.ToLower(value)
 	if strings.HasPrefix(lower, "authorization:") {
 		return "Authorization: Bearer " + redactionMarker
 	}
-	for _, sep := range []string{":", "="} {
-		if index := strings.Index(value, sep); index >= 0 {
-			key := strings.TrimSpace(value[:index])
-			return key + sep + redactionMarker
+	// Use the LEFTMOST ':' or '=' as the key/value boundary (not the first ':' anywhere, then '='):
+	// a value that itself contains ':' — e.g. token=a:b — must still split at its real '=' boundary.
+	// Preserve the key only when the prefix is one of the recognized credential key names; otherwise
+	// the entire match is the secret and is fully redacted.
+	if i := strings.IndexAny(value, ":="); i >= 0 {
+		if key := strings.TrimSpace(value[:i]); credKeyName.MatchString(key) {
+			return key + string(value[i]) + redactionMarker
 		}
 	}
 	return redactionMarker
diff --git a/internal/githubcontext/githubcontext_test.go b/internal/githubcontext/githubcontext_test.go
index 9e00161..66befd3 100644
--- a/internal/githubcontext/githubcontext_test.go
+++ b/internal/githubcontext/githubcontext_test.go
@@ -225,10 +225,36 @@ func TestRenderMarkdownOmitsEmptyOptionalFields(t *testing.T) {
 }
 
 func TestRedactMatchSeparatorAtStart(t *testing.T) {
-	// A value whose separator is at index 0 (key becomes ""): index>=0 must accept it, which the
-	// index>0 boundary mutant (line 159) would skip, redacting differently.
-	if got := redactMatch(":=value"); got != ":"+redactionMarker {
-		t.Fatalf("redactMatch(\":=value\") = %q, want %q", got, ":"+redactionMarker)
+	// A value whose separator is at index 0 has an empty prefix, so it is not a recognized
+	// credential key and the whole match is redacted. (The prefix-preservation path is only for
+	// genuine token/secret/password/api_key keys; see TestRedactWholeSecretsLeakNoPrefix.)
+	if got := redactMatch(":=value"); got != redactionMarker {
+		t.Fatalf("redactMatch(\":=value\") = %q, want %q", got, redactionMarker)
+	}
+}
+
+// A whole-secret match (a PEM private key block, or a key=value whose value itself contains ':')
+// must not leak any of the secret material through redactMatch's key-prefix preservation. The old
+// logic preserved value[:index] up to the first ':' anywhere, then the first '=', which leaked the
+// PEM body (base64 '=' padding sits at the end) and the value prefix before an in-value ':'.
+func TestRedactWholeSecretsLeakNoPrefix(t *testing.T) {
+	// ghctx-1: an unencrypted PEM key whose base64 body ends in '=' padding.
+	pemBody := "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwZZTOPSECRETKEYMATERIALabcd=="
+	pem := "-----BEGIN PRIVATE KEY-----\n" + pemBody + "\n-----END PRIVATE KEY-----"
+	if red := Redact(pem); strings.Contains(red, "MIIEvQ") || strings.Contains(red, "TOPSECRETKEYMATERIAL") {
+		t.Fatalf("PEM key body leaked through redaction: %q", red)
+	}
+	// ghctx-2: a token whose value contains a ':' — the leftmost separator is the '=' key boundary,
+	// so nothing of the value may survive.
+	if red := Redact("token=abc:def:ghi"); strings.Contains(red, "abc") || strings.Contains(red, "def") {
+		t.Fatalf("token value leaked through redaction: %q", red)
+	}
+	if red := Redact(`secret="a:b:c"`); strings.Contains(red, "a:b") {
+		t.Fatalf("quoted secret value leaked through redaction: %q", red)
+	}
+	// A genuine key=value still keeps its key name (provenance) while dropping the value.
+	if red := Redact("token=plainsecret"); !strings.Contains(red, "token=[REDACTED]") {
+		t.Fatalf("legitimate key prefix should be preserved: %q", red)
 	}
 }



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/githubcontext/"],"cwd":"/Users/dsifry/Developer/metareview","exitCode":0,"startedAt":"2026-09-02T03:42:54.361Z","finishedAt":"2026-09-02T03:42:54.43195Z","stdoutSha256":"c92b672669ee445199a6dbace5c431c428b6bbd93230c997b55f426d18ada7d3","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/githubcontext/ exited 0"}

