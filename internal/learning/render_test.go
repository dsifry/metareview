package learning

import (
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/learnsource"
	"github.com/dsifry/metareview/internal/sessionhistory"
)

func TestLearningChangedFilesMarkdownFiltersGeneratedArtifactsAndCaps(t *testing.T) {
	files := []string{
		"docs/metareview/context/mrv-20260527-context.md",
		"internal/learning/review.go",
		"docs/metareview/reviews/mrv-20260527.md",
		"cmd/metareview/main.go",
		"docs/metareview/learning/mrv-accepted.md",
		"skills/learn-post-merge/SKILL.md",
		".metareview/learning-runs.jsonl",
		"tests/go/test-learn-post-merge.sh",
		"README.md",
		"docs/SERVICE_INVENTORY.md",
		"internal/learning/review.go",
		"docs/superpowers/plans/slice-5.md",
		"internal/learning/render.go",
		"internal/learning/render_test.go",
		"tests/run-all.sh",
		"go.mod",
		"docs/QUICKSTART.md",
		"docs/INSTALL.md",
	}

	rendered := learningChangedFilesMarkdown(files)

	assertContains(t, rendered, "- `internal/learning/review.go`")
	assertContains(t, rendered, "- `cmd/metareview/main.go`")
	assertContains(t, rendered, "- `docs/QUICKSTART.md`")
	assertContains(t, rendered, "- ... 1 more changed files omitted")
	assertNotContains(t, rendered, "docs/metareview/context/mrv-20260527-context.md")
	assertNotContains(t, rendered, "docs/metareview/reviews/mrv-20260527.md")
	assertNotContains(t, rendered, "docs/metareview/learning/mrv-accepted.md")
	assertNotContains(t, rendered, ".metareview/learning-runs.jsonl")
	if strings.Count(rendered, "internal/learning/review.go") != 1 {
		t.Fatalf("expected duplicate changed files to render once, got:\n%s", rendered)
	}
}

func TestLearningChangedFilesMarkdownReportsOnlyGeneratedArtifacts(t *testing.T) {
	rendered := learningChangedFilesMarkdown([]string{
		"docs/metareview/context/mrv-20260527-context.md",
		"docs/metareview/reviews/mrv-20260527.md",
		"docs/metareview/learning/mrv-accepted.md",
		".metareview/learning-runs.jsonl",
	})

	assertContains(t, rendered, "No non-generated changed files discovered.")
}

func TestAcceptedMarkdownUsesCompactLearningSummary(t *testing.T) {
	source := learnsource.Context{
		Git: gitcontext.Context{
			BaseSHA: "base",
			HeadSHA: "head",
			ChangedFiles: []string{
				"docs/metareview/context/mrv-20260527-context.md",
				"internal/learning/review.go",
				"docs/metareview/reviews/mrv-20260527.md",
				"cmd/metareview/main.go",
				"docs/metareview/learning/mrv-accepted.md",
				"skills/learn-post-merge/SKILL.md",
				"tests/go/test-learn-post-merge.sh",
				"README.md",
				"docs/SERVICE_INVENTORY.md",
				"docs/superpowers/plans/slice-5.md",
				"internal/learning/render.go",
				"internal/learning/render_test.go",
				"tests/run-all.sh",
				"go.mod",
				"docs/QUICKSTART.md",
				"docs/INSTALL.md",
			},
		},
		GitHub: githubcontext.Context{
			Available:         false,
			UnavailableReason: "remote-unavailable",
		},
		GitHubMarkdown: "GitHub context unavailable: remote-unavailable\n",
	}
	session := sessionhistory.Context{
		Available:         false,
		UnavailableReason: "session-root-missing",
	}
	options := ReviewOptions{
		PostMergePR: "slice-4",
		Now:         time.Date(2026, 5, 27, 3, 20, 34, 0, time.UTC),
	}

	rendered := acceptedMarkdown("mrv-run", options, source, session, nil, nil)

	assertContains(t, rendered, "- GitHub: unavailable (remote-unavailable)")
	assertContains(t, rendered, "- Session history: unavailable (session-root-missing)")
	assertContains(t, rendered, "GitHub context unavailable: remote-unavailable")
	assertContains(t, rendered, "No accepted learning candidates.")
	assertContains(t, rendered, "No reviewer calibration candidates.")
	assertContains(t, rendered, "- ... 1 more changed files omitted")
	assertNotContains(t, rendered, "docs/metareview/context/mrv-20260527-context.md")
	assertNotContains(t, rendered, "docs/metareview/reviews/mrv-20260527.md")
	assertNotContains(t, rendered, "docs/metareview/learning/mrv-accepted.md")
}

func assertContains(t *testing.T, text string, expected string) {
	t.Helper()
	if !strings.Contains(text, expected) {
		t.Fatalf("expected output to contain %q, got:\n%s", expected, text)
	}
}

func assertNotContains(t *testing.T, text string, unexpected string) {
	t.Helper()
	if strings.Contains(text, unexpected) {
		t.Fatalf("expected output not to contain %q, got:\n%s", unexpected, text)
	}
}
