package artifactreview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/contextpack"
	"github.com/dsifry/metareview/internal/lens"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/state"
)

type Result struct {
	RunID       string
	ReviewRel   string
	ContextRel  string
	PreviousRun string
}

type runRecord struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ID            string              `json:"id"`
	Scope         string              `json:"scope"`
	Target        map[string]string   `json:"target"`
	Status        string              `json:"status"`
	Verdict       string              `json:"verdict"`
	ExecutionMode string              `json:"executionMode"`
	PreviousRunID *string             `json:"previousRunId"`
	BaseSHA       string              `json:"baseSha"`
	HeadSHA       string              `json:"headSha"`
	ContextPath   string              `json:"contextPackPath"`
	ReviewPath    string              `json:"reviewLogPath"`
	Reviewers     []string            `json:"reviewers"`
	FindingIDs    []string            `json:"findingIds"`
	SourceRefs    []map[string]string `json:"sourceRefs"`
	CreatedAt     string              `json:"createdAt"`
	UpdatedAt     string              `json:"updatedAt"`
	RepoRoot      string              `json:"repoRoot"`
	GitHead       string              `json:"gitHead"`
}

// Seams over the stdlib and cross-package calls whose error branches below are otherwise reachable
// only through a filesystem or permission failure a normal (non-root) test runner cannot force.
// Each defaults to the real function and is overridden — then restored via t.Cleanup — in tests, so
// the behavior in production is identical to calling the wrapped function directly.
var (
	buildContext = contextpack.Build
	mkdirAll     = os.MkdirAll
	writeFile    = os.WriteFile
)

func gitHead(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	return string(out[:len(out)-1])
}

func ensureEmpty(path string) error {
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return writeFile(path, []byte{}, 0o644)
	}
	return nil
}

func ensureFindingsIndex(root string) error {
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFile(path, []byte("# metareview Findings\n\nNo unresolved findings recorded yet.\n"), 0o644)
}

// rubricLinks maps a lens Slug to its dedicated rubric, for the lenses that have one. The rest
// share rubrics/artifact-review-rubric.md and carry no suffix. Only this per-slug metadata is kept
// alongside the canonical set; the names and count come from lens.All so the scaffold's prompt list
// cannot declare a reviewer the marker does not, or drift from the reviewer slugs it is generated
// beside (both derive from the same source).
var rubricLinks = map[string]string{
	"security":             "rubrics/security-review-rubric.md",
	"testing-quality":      "rubrics/testing-quality-rubric.md",
	"data-migration":       "rubrics/data-migration-rubric.md",
	"mechanical-precision": "rubrics/mechanical-precision-rubric.md",
}

// reviewerPrompts renders one bullet per canonical lens, in order, with a dedicated-rubric suffix
// where one exists — derived from lens.All, so it stays in lockstep with record.Reviewers
// (lens.Slugs()) and the "Required lenses:" marker.
func reviewerPrompts() string {
	var b strings.Builder
	for _, l := range lens.All {
		b.WriteString("- " + l.Display)
		if r, ok := rubricLinks[l.Slug]; ok {
			b.WriteString(" (see " + markdown.InlineCode(r) + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func Create(root, target, previousRun string, at time.Time) (Result, error) {
	runAt := at
	var runID string
	var reviewRel string
	var reviewPath string
	for {
		runID = state.RunID("artifact", target, runAt)
		reviewRel = filepath.ToSlash(filepath.Join("docs", "metareview", "reviews", runID+".md"))
		reviewPath = filepath.Join(root, filepath.FromSlash(reviewRel))
		if _, err := os.Stat(reviewPath); os.IsNotExist(err) {
			break
		}
		runAt = runAt.Add(time.Nanosecond)
	}
	ctx, err := buildContext(root, target, runAt)
	if err != nil {
		return Result{}, err
	}
	if ctx.RunID != runID {
		return Result{}, fmt.Errorf("context pack run ID mismatch: expected %s, got %s", runID, ctx.RunID)
	}
	if err := mkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
		return Result{}, err
	}
	head := gitHead(root)
	now := runAt.UTC().Format(time.RFC3339Nano)
	var prev *string
	if previousRun != "" {
		prev = &previousRun
	}
	record := runRecord{
		SchemaVersion: 1,
		ID:            runID, Scope: "artifact",
		Target: map[string]string{"type": "path", "path": target},
		Status: "open", Verdict: "NOT_REVIEWED", ExecutionMode: "pending-parallel-subagents",
		PreviousRunID: prev, BaseSHA: head, HeadSHA: head, ContextPath: ctx.ContextRel, ReviewPath: reviewRel,
		Reviewers:  lens.Slugs(),
		FindingIDs: []string{}, SourceRefs: []map[string]string{{"type": "path", "path": target}},
		CreatedAt: now, UpdatedAt: now, RepoRoot: root, GitHead: head,
	}
	if err := ensureEmpty(filepath.Join(root, ".metareview", "findings.jsonl")); err != nil {
		return Result{}, err
	}
	if err := ensureFindingsIndex(root); err != nil {
		return Result{}, err
	}
	prevDisplay := "none"
	if previousRun != "" {
		prevDisplay = previousRun
	}
	content := "# metareview: artifact review\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"Target: " + markdown.InlineCode(target) + "\n\n" +
		"Context pack: " + markdown.InlineCode(ctx.ContextRel) + "\n\n" +
		"Execution mode: `pending-parallel-subagents`\n\n" +
		"Previous run: " + markdown.InlineCode(prevDisplay) + "\n\n" +
		// The lens set is recorded in the log itself so a completed review stays judged against the
		// rubric that was required when it ran. Without it, adding a lens retroactively marks every
		// earlier review incomplete, and reviewlog.requiredLenses has nothing to fall back on but
		// the era default. See internal/reviewlog.
		"Required lenses: " + markdown.InlineCode(strings.Join(record.Reviewers, ", ")) + "\n\n" +
		"## Verdict\n\nNOT_REVIEWED\n\n" +
		"## Completion Requirements\n\nThis scaffold is not a completed review. Artifact review defaults to parallel subagents for the required lenses. The artifact-review workflow is explicit authorization to delegate those lenses. Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation; if used, state that the review is not independently adversarial and treat it as weaker evidence. Completion requires every required reviewer row to be populated, each reviewer to have a verdict, blocking findings to be fixed and re-reviewed or explicitly human-accepted, and the aggregate verdict to be the actual artifact-review verdict returned by the reviewer set rather than a fixed example result.\n\n" +
		"## Reviewer Prompts\n\nUse `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses as parallel subagents by default before aggregation:\n\n" +
		reviewerPrompts() + "\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Warnings | Notes |\n| --- | --- | ---: | ---: | --- |\n\n" +
		"## Orchestrator Notes (not findings)\n\n" +
		"Orchestrator context and synthesis go here (e.g. checkout sparse, filtered file-not-found artifacts, consolidation narrative). This section is audit trail only — it is NOT a finding stream. Do not extract sentences from here as review findings; only the `## Findings` section and its classified `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections contain review findings.\n\n" +
		"## Findings\n\nNo reviewer findings recorded yet.\n"
	if err := writeFile(reviewPath, []byte(content), 0o644); err != nil {
		return Result{}, err
	}
	if err := state.AppendJSONL(filepath.Join(root, ".metareview", "runs.jsonl"), record); err != nil {
		return Result{}, err
	}
	return Result{RunID: runID, ReviewRel: reviewRel, ContextRel: ctx.ContextRel, PreviousRun: previousRun}, nil
}
