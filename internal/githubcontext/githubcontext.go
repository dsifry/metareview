package githubcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
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

func Collect(root, prNumber string) (Context, error) {
	prNumber = strings.TrimSpace(prNumber)
	if prNumber == "" {
		return unavailable("pr-number-unavailable", prNumber), nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return unavailable("gh-unavailable", prNumber), nil
	}
	remote, err := command(root, "git", "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(remote) == "" {
		return unavailable("remote-unavailable", prNumber), nil
	}
	if _, err := command(root, "gh", "auth", "status"); err != nil {
		return unavailable("gh-auth-unavailable", prNumber), nil
	}
	out, err := command(root, "gh", "pr", "view", prNumber, "--json", "number,url,title,body,comments,reviews")
	if err != nil {
		return unavailable("github-pr-unavailable", prNumber), nil
	}
	var parsed prView
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return Context{}, err
	}
	ctx := Context{
		Available: true,
		PRNumber:  prNumber,
		Remote:    strings.TrimSpace(remote),
		URL:       Redact(parsed.URL),
		Title:     excerpt(Redact(parsed.Title)),
		Body:      excerpt(Redact(parsed.Body)),
		Comments:  make([]Entry, 0, len(parsed.Comments)),
		Reviews:   make([]Entry, 0, len(parsed.Reviews)),
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

func command(root, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
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

func redactMatch(value string) string {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "authorization:") {
		return "Authorization: Bearer " + redactionMarker
	}
	for _, sep := range []string{":", "="} {
		if index := strings.Index(value, sep); index >= 0 {
			key := strings.TrimSpace(value[:index])
			return key + sep + redactionMarker
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
	truncated := string(runes[:maxExcerptRunes])
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
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
