package artifactreview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dsifry/metareview/internal/contextpack"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte{}, 0o644)
	}
	return nil
}

func ensureFindingsIndex(root string) error {
	path := filepath.Join(root, "docs", "metareview", "FINDINGS.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("# metareview Findings\n\nNo unresolved findings recorded yet.\n"), 0o644)
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
	ctx, err := contextpack.Build(root, target, runAt)
	if err != nil {
		return Result{}, err
	}
	if ctx.RunID != runID {
		return Result{}, fmt.Errorf("context pack run ID mismatch: expected %s, got %s", runID, ctx.RunID)
	}
	if err := os.MkdirAll(filepath.Dir(reviewPath), 0o755); err != nil {
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
		Status: "open", Verdict: "NOT_REVIEWED", ExecutionMode: "in-session-emulated",
		PreviousRunID: prev, BaseSHA: head, HeadSHA: head, ContextPath: ctx.ContextRel, ReviewPath: reviewRel,
		Reviewers:  []string{"feasibility", "completeness", "scope-alignment", "architecture", "intent-preservation"},
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
		"Execution mode: `in-session-emulated`\n\n" +
		"Previous run: " + markdown.InlineCode(prevDisplay) + "\n\n" +
		"## Verdict\n\nNOT_REVIEWED\n\n" +
		"## Completion Requirements\n\nThis scaffold is not a completed review. It blocks downstream gates until all required reviewer rows are populated and the verdict is `PASS` or `PASS_ADVISORY` with zero blocking findings.\n\n" +
		"## Reviewer Prompts\n\nUse `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses independently before aggregation:\n\n" +
		"- Feasibility\n" +
		"- Completeness\n" +
		"- Scope and alignment\n" +
		"- Architecture\n" +
		"- Intent preservation\n\n" +
		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Warnings | Notes |\n| --- | --- | ---: | ---: | --- |\n\n" +
		"## Findings\n\nNo reviewer findings recorded yet.\n"
	if err := os.WriteFile(reviewPath, []byte(content), 0o644); err != nil {
		return Result{}, err
	}
	if err := state.AppendJSONL(filepath.Join(root, ".metareview", "runs.jsonl"), record); err != nil {
		return Result{}, err
	}
	return Result{RunID: runID, ReviewRel: reviewRel, ContextRel: ctx.ContextRel, PreviousRun: previousRun}, nil
}
