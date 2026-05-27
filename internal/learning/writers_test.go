package learning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteKnowledgeUsesBeadsWhenPresentAndPreservesRecords(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(root, ".beads", "knowledge", "metareview.jsonl")
	mustWrite(t, existingPath, `{"id":"existing","fact":"keep me"}`+"\n")

	result, err := WriteKnowledge(root, "run-1", []Candidate{writerCandidate("Before adding service paths, reviewers should require inventory reuse.")})
	if err != nil {
		t.Fatalf("WriteKnowledge returned error: %v", err)
	}
	if result.Path != ".beads/knowledge/metareview.jsonl" {
		t.Fatalf("unexpected knowledge path: %s", result.Path)
	}
	lines := readLines(t, existingPath)
	if len(lines) != 2 || !strings.Contains(lines[0], "keep me") {
		t.Fatalf("existing record not preserved: %+v", lines)
	}
	var record KnowledgeRecord
	if err := json.Unmarshal([]byte(lines[1]), &record); err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.RunID != "run-1" || record.Fact == "" || len(record.SourceRefs) != 1 {
		t.Fatalf("knowledge record missing deterministic metadata: %+v", record)
	}

	second, err := WriteKnowledge(root, "run-1", []Candidate{writerCandidate("Before adding service paths, reviewers should require inventory reuse.")})
	if err != nil {
		t.Fatalf("second WriteKnowledge returned error: %v", err)
	}
	if second.Records[0].ID != record.ID {
		t.Fatalf("expected deterministic ID %s, got %s", record.ID, second.Records[0].ID)
	}
}

func TestWriteKnowledgeFallsBackWhenBeadsAbsent(t *testing.T) {
	root := t.TempDir()
	result, err := WriteKnowledge(root, "run-2", []Candidate{writerCandidate("Reviewers should preserve original intent after plan iteration.")})
	if err != nil {
		t.Fatalf("WriteKnowledge returned error: %v", err)
	}
	if result.Path != ".metareview/knowledge/metareview.jsonl" {
		t.Fatalf("unexpected fallback path: %s", result.Path)
	}
	if _, err := os.Stat(filepath.Join(root, ".metareview", "knowledge", "metareview.jsonl")); err != nil {
		t.Fatalf("fallback knowledge file missing: %v", err)
	}
}

func TestWriteCalibrationAppendsRecords(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".metareview", "calibration.jsonl")
	mustWrite(t, path, `{"id":"existing","disposition":"false-positive"}`+"\n")

	result, err := WriteCalibration(root, "run-3", []Candidate{{
		Kind:           "reviewer-calibration",
		Text:           "Existing golden tests covered this path.",
		Scope:          "reviewer-calibration",
		Provenance:     "finding disposition",
		SourceRefs:     []SourceRef{{Type: "finding", ID: "mrvf-1"}},
		Confidence:     "medium",
		ProposedTarget: "reviewer-calibration",
		Disposition:    "rebutted",
	}})
	if err != nil {
		t.Fatalf("WriteCalibration returned error: %v", err)
	}
	if result.Path != ".metareview/calibration.jsonl" {
		t.Fatalf("unexpected calibration path: %s", result.Path)
	}
	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("expected append, got %+v", lines)
	}
	var record CalibrationRecord
	if err := json.Unmarshal([]byte(lines[1]), &record); err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.RunID != "run-3" || record.Disposition != "rebutted" || record.ReviewerScope != "reviewer-calibration" {
		t.Fatalf("calibration record missing metadata: %+v", record)
	}
}

func writerCandidate(text string) Candidate {
	return Candidate{
		Kind:           "knowledge-candidate",
		Text:           text,
		Scope:          "repository-review",
		Provenance:     "test",
		SourceRefs:     []SourceRef{{Type: "finding", ID: "mrvf-1"}},
		Confidence:     "high",
		ProposedTarget: "repository-knowledge",
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(bytes))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
