package learning

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/dsifry/metareview/internal/state"
)

type KnowledgeWriteResult struct {
	Path    string            `json:"path"`
	Records []KnowledgeRecord `json:"records"`
}

type KnowledgeRecord struct {
	SchemaVersion int         `json:"schemaVersion"`
	ID            string      `json:"id"`
	RunID         string      `json:"runId"`
	Fact          string      `json:"fact"`
	Scope         string      `json:"scope"`
	Provenance    string      `json:"provenance"`
	SourceRefs    []SourceRef `json:"sourceRefs"`
	Confidence    string      `json:"confidence"`
}

func WriteKnowledge(root, runID string, candidates []Candidate) (KnowledgeWriteResult, error) {
	rel := knowledgeRelPath(root)
	path := filepath.Join(root, filepath.FromSlash(rel))
	result := KnowledgeWriteResult{Path: rel}
	for _, candidate := range candidates {
		record := KnowledgeRecord{
			SchemaVersion: 1,
			ID:            deterministicID("knowledge", runID, candidate),
			RunID:         runID,
			Fact:          candidate.Text,
			Scope:         candidate.Scope,
			Provenance:    candidate.Provenance,
			SourceRefs:    candidate.SourceRefs,
			Confidence:    candidate.Confidence,
		}
		if err := state.AppendJSONL(path, record); err != nil {
			return KnowledgeWriteResult{}, err
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}

func knowledgeRelPath(root string) string {
	if info, err := os.Stat(filepath.Join(root, ".beads")); err == nil && info.IsDir() {
		return ".beads/knowledge/metareview.jsonl"
	}
	return ".metareview/knowledge/metareview.jsonl"
}

func deterministicID(prefix, runID string, candidate Candidate) string {
	hash := sha1.Sum([]byte(runID + "\n" + candidate.Kind + "\n" + candidate.Text + "\n" + candidate.Scope + "\n" + candidate.Disposition))
	return "mrv-" + prefix + "-" + hex.EncodeToString(hash[:])[:16]
}
