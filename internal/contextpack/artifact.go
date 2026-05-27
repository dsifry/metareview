package contextpack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/repo"
	"github.com/dsifry/metareview/internal/state"
)

type Result struct {
	RunID      string
	ContextRel string
}

func gitValue(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(out))
}

func assertInsideFile(root, target string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, target))
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	targetReal, err := filepath.EvalSymlinks(targetAbs)
	if err != nil {
		return "", err
	}
	if targetReal != rootReal && !strings.HasPrefix(targetReal, rootReal+string(filepath.Separator)) {
		return "", fmt.Errorf("target artifact is outside repository root: %s", target)
	}
	info, err := os.Stat(targetReal)
	if err != nil {
		return "", fmt.Errorf("target artifact not found: %s", target)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("target artifact is not a regular file: %s", target)
	}
	return targetReal, nil
}

func readLimited(path string, limit int) string {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(bytes)
	if len(text) > limit {
		return text[:limit]
	}
	return text
}

func knowledgeFacts(root string) []string {
	dir := filepath.Join(root, ".beads", "knowledge")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var facts []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		lines := strings.Split(readLimited(filepath.Join(dir, entry.Name()), 20000), "\n")
		for i, line := range lines {
			if i >= 5 {
				break
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			facts = append(facts, "- "+entry.Name()+": "+markdown.PlainText(line))
		}
	}
	return facts
}

func Build(root, target string, at time.Time) (Result, error) {
	targetPath, err := assertInsideFile(root, target)
	if err != nil {
		return Result{}, err
	}
	runID := state.RunID("artifact", target, at)
	report := repo.Detect(root)
	contextRel := filepath.ToSlash(filepath.Join("docs", "metareview", "context", runID+"-context.md"))
	outputPath := filepath.Join(root, filepath.FromSlash(contextRel))
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return Result{}, err
	}
	serviceInventory := "No service inventory found."
	if report.Files.ServiceInventory != nil {
		serviceInventory = "Source: " + markdown.InlineCode(*report.Files.ServiceInventory) + "\n\n" +
			markdown.FencedCodeBlock("markdown", readLimited(filepath.Join(root, *report.Files.ServiceInventory), 2000))
	}
	facts := knowledgeFacts(root)
	factText := "No Beads knowledge facts found."
	if len(facts) > 0 {
		factText = strings.Join(facts, "\n")
	}
	content := "# metareview context: " + markdown.PlainText(target) + "\n\n" +
		"Run ID: " + markdown.InlineCode(runID) + "\n\n" +
		"## Target\n\n" +
		"- Path: " + markdown.InlineCode(target) + "\n" +
		"- Repository mode: " + markdown.InlineCode(report.Mode) + "\n" +
		"- Git branch: " + markdown.InlineCode(gitValue(root, "branch", "--show-current")) + "\n" +
		"- Git head: " + markdown.InlineCode(gitValue(root, "rev-parse", "--short", "HEAD")) + "\n\n" +
		"## Artifact Excerpt\n\n" + markdown.FencedCodeBlock("markdown", readLimited(targetPath, 4000)) + "\n\n" +
		"## Service Inventory\n\n" + serviceInventory + "\n\n" +
		"## Knowledge Facts\n\n" + factText + "\n\n" +
		"## Suggested Reviewers\n\n- feasibility\n- completeness\n- scope/alignment\n- architecture\n- intent preservation\n"
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return Result{}, err
	}
	return Result{RunID: runID, ContextRel: contextRel}, nil
}
