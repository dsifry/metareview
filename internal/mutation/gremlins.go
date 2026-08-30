package mutation

import (
	"encoding/json"
	"fmt"
	"strings"
)

// gremlinsReport is the shape gremlins writes with `--output`. Only the fields we read are
// declared: notably NOT its summary counters or test_efficacy, which are unsafe (see the package
// doc). The per-mutation list is the source of truth.
type gremlinsReport struct {
	GoModule string `json:"go_module"`
	Files    []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"mutations"`
	} `json:"files"`
}

// ParseGremlins normalises a gremlins JSON report.
//
// The status mapping is where the engine's vocabulary meets ours, and the important row is
// TIMED OUT: gremlins omits it from its summary entirely, so a caller trusting that summary sees
// a run in which those mutants never existed. Here it becomes Unresolved, which counts against
// the score and makes the run incomplete.
func ParseGremlins(data []byte, target string) (Report, error) {
	var raw gremlinsReport
	if err := json.Unmarshal(data, &raw); err != nil {
		return Report{}, fmt.Errorf("mutation: parsing gremlins report: %w", err)
	}
	r := Report{Engine: "gremlins", Target: target}
	for _, f := range raw.Files {
		for _, m := range f.Mutations {
			r.Mutants = append(r.Mutants, Mutant{
				Status:   gremlinsStatus(m.Status),
				File:     f.FileName,
				Line:     m.Line,
				Column:   m.Column,
				Operator: m.Type,
			})
		}
	}
	return r, nil
}

func gremlinsStatus(s string) Status {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "KILLED":
		return Killed
	case "LIVED":
		return Survived
	case "NOT COVERED", "NOT_COVERED":
		return Uncovered
	case "NOT VIABLE", "NOT_VIABLE", "SKIPPED":
		// The mutant could not be built or was deliberately skipped: nothing was measured, and
		// nothing is claimed. Not a kill.
		return Unresolved
	default:
		// TIMED OUT, RUNERROR, and anything a future version adds. Defaulting to Unresolved is
		// deliberate: an outcome this code does not recognise must never be scored as a success.
		return Unresolved
	}
}
