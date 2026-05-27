package learning

import (
	"path/filepath"

	"github.com/dsifry/metareview/internal/state"
)

type CalibrationWriteResult struct {
	Path    string              `json:"path"`
	Records []CalibrationRecord `json:"records"`
}

type CalibrationRecord struct {
	SchemaVersion int         `json:"schemaVersion"`
	ID            string      `json:"id"`
	RunID         string      `json:"runId"`
	ReviewerScope string      `json:"reviewerScope"`
	Disposition   string      `json:"disposition"`
	Lesson        string      `json:"lesson"`
	Provenance    string      `json:"provenance"`
	SourceRefs    []SourceRef `json:"sourceRefs"`
	Confidence    string      `json:"confidence"`
}

func WriteCalibration(root, runID string, candidates []Candidate) (CalibrationWriteResult, error) {
	rel := ".metareview/calibration.jsonl"
	path := filepath.Join(root, filepath.FromSlash(rel))
	result := CalibrationWriteResult{Path: rel}
	for _, candidate := range candidates {
		record := CalibrationRecord{
			SchemaVersion: 1,
			ID:            deterministicID("calibration", runID, candidate),
			RunID:         runID,
			ReviewerScope: candidate.Scope,
			Disposition:   candidate.Disposition,
			Lesson:        candidate.Text,
			Provenance:    candidate.Provenance,
			SourceRefs:    candidate.SourceRefs,
			Confidence:    candidate.Confidence,
		}
		if err := state.AppendJSONL(path, record); err != nil {
			return CalibrationWriteResult{}, err
		}
		result.Records = append(result.Records, record)
	}
	return result, nil
}
