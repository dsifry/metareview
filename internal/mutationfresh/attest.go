package mutationfresh

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const harnessTool = "metareview-mutation-incremental"

// Attestation is the part of <stateDir>/attestation.json the gate reads (spec §5.5, §11.2).
type Attestation struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Tool          string               `json:"tool"`
	Engine        string               `json:"engine"`
	Report        string               `json:"report"`
	ReportSha256  string               `json:"reportSha256"`
	Config        string               `json:"config"`
	Lists         Lists                `json:"lists"`
	Exclusions    []string             `json:"exclusions"`
	Files         map[string]FileEntry `json:"files"`
	Deferrals     []Deferral           `json:"deferrals"`
}

// FileEntry is one attested path: its start-of-run digest, category and whether git tracked it.
type FileEntry struct {
	Digest   string `json:"digest"`
	Category string `json:"category"`
	Tracked  bool   `json:"tracked"`
}

// Deferral is recorded pending work: kills in files named by paths (or "*") are pending.
type Deferral struct {
	Reason    string   `json:"reason"`
	Paths     []string `json:"paths"`
	Inherited bool     `json:"inherited"`
}

// readAttestation reads attestation.json beside the report and validates it against the report
// (spec §6.1). It returns the attestation, the first failing reason ("" when attested) and the
// attestation file's sha256 ("" when it cannot be read).
func readAttestation(reportPath, engine, reportSHA string) (Attestation, string, string) {
	path := filepath.Join(filepath.Dir(reportPath), "attestation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Attestation{}, "missing", ""
	}
	var att Attestation
	if err := json.Unmarshal(data, &att); err != nil || att.Files == nil {
		return Attestation{}, "unparseable", sha256Hex(data)
	}
	return att, attestationReason(att, path, reportPath, engine, reportSHA), sha256Hex(data)
}

func attestationReason(att Attestation, attPath, reportPath, engine, reportSHA string) string {
	switch {
	case att.SchemaVersion != 1:
		return "version"
	case att.Tool != harnessTool:
		return "tool"
	case att.Engine != engine:
		return "engine"
	case !sameFile(filepath.Join(filepath.Dir(attPath), filepath.FromSlash(att.Report)), reportPath):
		return "report mismatch"
	case att.ReportSha256 != reportSHA:
		return "hash mismatch"
	}
	return ""
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
