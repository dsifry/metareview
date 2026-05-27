package learning

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/learnsource"
	"github.com/dsifry/metareview/internal/markdown"
	"github.com/dsifry/metareview/internal/sessionhistory"
	"github.com/dsifry/metareview/internal/state"
)

type ReviewOptions struct {
	PostMergePR string
	Base        string
	GitHubPR    string
	SessionRoot string
	Now         time.Time
}

type ReviewResult struct {
	RunID       string
	AcceptedRel string
	DiscardRel  string
}

type learningRunRecord struct {
	SchemaVersion      int      `json:"schemaVersion"`
	ID                 string   `json:"id"`
	Scope              string   `json:"scope"`
	PostMergePR        string   `json:"postMergePr"`
	BaseSHA            string   `json:"baseSha"`
	HeadSHA            string   `json:"headSha"`
	AcceptedPath       string   `json:"acceptedPath"`
	DiscardPath        string   `json:"discardPath"`
	KnowledgePath      string   `json:"knowledgePath"`
	CalibrationPath    string   `json:"calibrationPath"`
	AcceptedCount      int      `json:"acceptedCount"`
	DiscardedCount     int      `json:"discardedCount"`
	CalibrationCount   int      `json:"calibrationCount"`
	SessionUnavailable string   `json:"sessionUnavailable,omitempty"`
	GitHubUnavailable  string   `json:"githubUnavailable,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	SourceRefs         []string `json:"sourceRefs"`
}

type fileSnapshot struct {
	existed bool
	isDir   bool
	content []byte
}

func RunPostMerge(root string, options ReviewOptions) (ReviewResult, error) {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	ghPR := options.GitHubPR
	if ghPR == "" {
		ghPR = options.PostMergePR
	}
	source, err := learnsource.Collect(root, learnsource.Options{Base: options.Base, GitHubPR: ghPR})
	if err != nil {
		return ReviewResult{}, err
	}
	session, err := sessionhistory.Collect(root, sessionhistory.Options{SessionRoot: options.SessionRoot})
	if err != nil {
		return ReviewResult{}, err
	}
	records, err := state.ReadJSONL[findings.Record](filepath.Join(root, ".metareview", "findings.jsonl"))
	if err != nil {
		return ReviewResult{}, err
	}
	candidateResult := ExtractCandidates(Input{Findings: records, GitHub: source.GitHub, Session: session})
	pruned := PruneCandidates(PruneInput{Candidates: candidateResult.Knowledge, Knowledge: source.Knowledge})

	runID, acceptedRel, discardRel := learningPaths(options.PostMergePR, now)
	acceptedPath := filepath.Join(root, filepath.FromSlash(acceptedRel))
	discardPath := filepath.Join(root, filepath.FromSlash(discardRel))
	runsPath := filepath.Join(root, ".metareview", "learning-runs.jsonl")
	knowledgeRel := knowledgeRelPath(root)
	knowledgePath := filepath.Join(root, filepath.FromSlash(knowledgeRel))
	calibrationRel := ".metareview/calibration.jsonl"
	calibrationPath := filepath.Join(root, filepath.FromSlash(calibrationRel))
	gitignorePath := filepath.Join(root, ".gitignore")
	snapshots := map[string]fileSnapshot{}
	for _, path := range []string{acceptedPath, discardPath, runsPath, knowledgePath, calibrationPath, gitignorePath} {
		snapshots[path] = snapshot(path)
	}

	result := ReviewResult{RunID: runID, AcceptedRel: acceptedRel, DiscardRel: discardRel}
	err = func() error {
		if err := EnsureLearningGitPolicy(root); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(acceptedPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(acceptedPath, []byte(acceptedMarkdown(runID, options, source, session, pruned.Accepted, candidateResult.Calibration)), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(discardPath, []byte(discardMarkdown(runID, pruned.Discarded)), 0o644); err != nil {
			return err
		}
		knowledgeWrite, err := WriteKnowledge(root, runID, pruned.Accepted)
		if err != nil {
			return err
		}
		calibrationWrite, err := WriteCalibration(root, runID, candidateResult.Calibration)
		if err != nil {
			return err
		}
		record := learningRunRecord{
			SchemaVersion:    1,
			ID:               runID,
			Scope:            "post-merge-learning",
			PostMergePR:      options.PostMergePR,
			BaseSHA:          source.Git.BaseSHA,
			HeadSHA:          source.Git.HeadSHA,
			AcceptedPath:     acceptedRel,
			DiscardPath:      discardRel,
			KnowledgePath:    knowledgeWrite.Path,
			CalibrationPath:  calibrationWrite.Path,
			AcceptedCount:    len(pruned.Accepted),
			DiscardedCount:   len(pruned.Discarded),
			CalibrationCount: len(candidateResult.Calibration),
			CreatedAt:        now.UTC().Format(time.RFC3339Nano),
			SourceRefs:       sourceRefs(source, session),
		}
		if !session.Available {
			record.SessionUnavailable = session.UnavailableReason
		}
		if !source.GitHub.Available {
			record.GitHubUnavailable = source.GitHub.UnavailableReason
		}
		return state.AppendJSONL(runsPath, record)
	}()
	if err != nil {
		restoreSnapshots(snapshots)
		removeEmptyLearningDirs(root)
		return ReviewResult{}, err
	}
	return result, nil
}

func learningPaths(prNumber string, at time.Time) (string, string, string) {
	runID := state.RunID("learn-post-merge", firstNonEmpty(prNumber, "unknown-pr"), at)
	base := filepath.ToSlash(filepath.Join("docs", "metareview", "learning", runID))
	return runID, base + "-accepted.md", base + "-discarded.md"
}

func acceptedMarkdown(runID string, options ReviewOptions, source learnsource.Context, session sessionhistory.Context, accepted []Candidate, calibration []Candidate) string {
	var builder strings.Builder
	builder.WriteString("# metareview Accepted Learning\n\n")
	builder.WriteString("Run ID: " + markdown.InlineCode(runID) + "\n\n")
	builder.WriteString("Post-merge PR: " + markdown.InlineCode(options.PostMergePR) + "\n\n")
	builder.WriteString("## Source Status\n\n")
	builder.WriteString("- Git base: " + markdown.InlineCode(source.Git.BaseSHA) + "\n")
	builder.WriteString("- Git head: " + markdown.InlineCode(source.Git.HeadSHA) + "\n")
	builder.WriteString("- GitHub: " + availability(source.GitHub.Available, source.GitHub.UnavailableReason) + "\n")
	builder.WriteString("- Session history: " + availability(session.Available, session.UnavailableReason) + "\n\n")
	builder.WriteString("## Git Diff Summary\n\n")
	builder.WriteString(learningChangedFilesMarkdown(source.Git.ChangedFiles))
	builder.WriteString("\n\n")
	builder.WriteString("## GitHub Context\n\n")
	builder.WriteString(source.GitHubMarkdown)
	builder.WriteString("\n## Accepted Learning\n\n")
	if len(accepted) == 0 {
		builder.WriteString("No accepted learning candidates.\n")
	} else {
		for _, candidate := range accepted {
			builder.WriteString("- " + candidate.Text + "\n")
			builder.WriteString("  - Provenance: " + candidate.Provenance + "\n")
			builder.WriteString("  - Confidence: " + candidate.Confidence + "\n")
			builder.WriteString("  - Source refs: " + sourceRefText(candidate.SourceRefs) + "\n")
		}
	}
	builder.WriteString("\n## Calibration Candidates\n\n")
	if len(calibration) == 0 {
		builder.WriteString("No reviewer calibration candidates.\n")
	} else {
		for _, candidate := range calibration {
			builder.WriteString("- " + candidate.Disposition + ": " + candidate.Text + "\n")
		}
	}
	return builder.String()
}

func discardMarkdown(runID string, discarded []DiscardedCandidate) string {
	var builder strings.Builder
	builder.WriteString("# metareview Discarded Candidates\n\n")
	builder.WriteString("Run ID: " + markdown.InlineCode(runID) + "\n\n")
	builder.WriteString("## Discarded Candidates\n\n")
	if len(discarded) == 0 {
		builder.WriteString("No discarded learning candidates.\n")
		return builder.String()
	}
	for _, discard := range discarded {
		builder.WriteString("- " + discard.Reason + ": " + discard.Candidate.Text + "\n")
	}
	return builder.String()
}

func sourceRefs(source learnsource.Context, session sessionhistory.Context) []string {
	refs := []string{}
	for _, log := range source.ReviewLogs {
		if log.Path != "" {
			refs = append(refs, log.Path)
		}
	}
	for _, signal := range session.Signals {
		if signal.Path != "" {
			refs = append(refs, signal.Path)
		}
	}
	return refs
}

func sourceRefText(refs []SourceRef) string {
	if len(refs) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, strings.TrimSpace(ref.Type+" "+firstNonEmpty(ref.ID, ref.Path, ref.URL)))
	}
	return strings.Join(parts, "; ")
}

func availability(available bool, reason string) string {
	if available {
		return "available"
	}
	return "unavailable (" + firstNonEmpty(reason, "unknown") + ")"
}

func snapshot(path string) fileSnapshot {
	info, err := os.Stat(path)
	if err != nil {
		return fileSnapshot{existed: false}
	}
	if info.IsDir() {
		return fileSnapshot{existed: true, isDir: true}
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{existed: false}
	}
	return fileSnapshot{existed: true, content: bytes}
}

func restoreSnapshots(snapshots map[string]fileSnapshot) {
	for path, snapshot := range snapshots {
		if snapshot.existed {
			if snapshot.isDir {
				_ = os.MkdirAll(path, 0o755)
				continue
			}
			_ = os.MkdirAll(filepath.Dir(path), 0o755)
			_ = os.WriteFile(path, snapshot.content, 0o644)
			continue
		}
		_ = os.Remove(path)
	}
}

func removeEmptyLearningDirs(root string) {
	for _, rel := range []string{
		filepath.Join("docs", "metareview", "learning"),
		filepath.Join("docs", "metareview"),
		filepath.Join("docs"),
		filepath.Join(".metareview", "knowledge"),
		filepath.Join(".beads", "knowledge"),
	} {
		_ = os.Remove(filepath.Join(root, rel))
	}
}

func markdownList(values []string, empty string) string {
	if len(values) == 0 {
		return empty
	}
	lines := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		lines = append(lines, "- "+value)
	}
	if len(lines) == 0 {
		return empty
	}
	return strings.Join(lines, "\n")
}
