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
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(token|secret|password|api[_-]?key)\s*[:=]\s*("[^"]+"|'[^']+'|[^\s` + "`" + `,;]+)`),
}

// keyPrefixPatterns match provider keys by their prefix. Unfiltered, "sk-" matched the "sk-done-…" in every
// "task-done-…" review path and "ghs_" matched "laughs_…", redacting ordinary text (#184). So each match is
// kept only if it is word-interior text — see keyIsWordInterior; everything else is redacted.
var keyPrefixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{8,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
}

// keyIsWordInterior reports whether the key-prefix match text[start:end] is ordinary lowercase text rather than
// a key: it has no uppercase letter AND it continues a lowercase word — the byte before it is a lowercase letter
// that does not end a two-character backslash escape (\n, \t, \b …) or a %XX percent-escape. Real provider
// keys are random base62 and carry uppercase letters, so they are redacted in any context — after ANSI colour
// codes (ESC[32m), \u003c or \x3d escapes, or plain letters — without enumerating contexts; a lowercase-only
// match is redacted unless it is word-interior.
func keyIsWordInterior(text string, start, end int) bool {
	if strings.IndexFunc(text[start:end], func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0 {
		return false
	}
	if start == 0 || !isLowerASCII(text[start-1]) {
		return false
	}
	if start >= 2 && text[start-2] == '\\' { // a \n, \t, \b … escape
		return false
	}
	if start >= 3 && text[start-3] == '%' && isHexASCII(text[start-2]) && isHexASCII(text[start-1]) { // a %2f, %3d … escape
		return false
	}
	return true
}

func isLowerASCII(c byte) bool { return c >= 'a' && c <= 'z' }

func isHexASCII(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// redactKeyPrefixes replaces every key-prefix match that is not word-interior with the marker, leaving the text
// around it untouched.
func redactKeyPrefixes(text string) string {
	for _, pattern := range keyPrefixPatterns {
		var b strings.Builder
		last := 0
		for _, m := range pattern.FindAllStringIndex(text, -1) {
			if keyIsWordInterior(text, m[0], m[1]) {
				continue
			}
			b.WriteString(text[last:m[0]])
			b.WriteString(redactionMarker)
			last = m[1]
		}
		b.WriteString(text[last:])
		text = b.String()
	}
	return text
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
	// The whole-value patterns run first: an Authorization: Bearer value or a token=… value is redacted entire,
	// tail and all, before the key-prefix pass could replace only the key and leave the rest behind (#184).
	for _, pattern := range secretPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, redactMatch)
	}
	return redactKeyPrefixes(redacted)
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
