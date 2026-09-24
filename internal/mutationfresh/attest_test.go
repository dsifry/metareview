package mutationfresh

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func writeAttestation(t *testing.T, dir string, att map[string]any) {
	t.Helper()
	data, err := json.Marshal(att)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "attestation.json", string(data))
}

func validAttestation(reportSHA string) map[string]any {
	return map[string]any{
		"schemaVersion": 1, "tool": "metareview-mutation-incremental", "engine": "stryker",
		"report": "incremental.json", "reportSha256": reportSHA, "files": map[string]any{},
	}
}

// Spec §6.1: attested iff it parses, schemaVersion is 1, the tool and engine match, `report`
// resolves to the report path and reportSha256 equals the report's bytes; otherwise the first
// failing reason.
func TestReadAttestationReasons(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "incremental.json", "{}")
	report := filepath.Join(dir, "incremental.json")
	sha := sha256Hex([]byte("{}"))
	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "missing" || attSHA != "" {
		t.Errorf("no attestation: %q %q", reason, attSHA)
	}
	write(t, dir, "attestation.json", "not json")
	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "unparseable" || attSHA == "" {
		t.Errorf("not JSON: %q", reason)
	}
	write(t, dir, "attestation.json", `{"schemaVersion":1}`)
	if _, reason, _ := readAttestation(report, "stryker", sha); reason != "unparseable" {
		t.Errorf("no files: %q", reason)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"valid", func(map[string]any) {}, ""},
		{"version", func(a map[string]any) { a["schemaVersion"] = 2 }, "version"},
		{"tool", func(a map[string]any) { a["tool"] = "other" }, "tool"},
		{"engine", func(a map[string]any) { a["engine"] = "gremlins" }, "engine"},
		{"report", func(a map[string]any) { a["report"] = "other.json" }, "report mismatch"},
		{"hash", func(a map[string]any) { a["reportSha256"] = "00" }, "hash mismatch"},
	}
	for _, c := range cases {
		att := validAttestation(sha)
		c.mutate(att)
		writeAttestation(t, dir, att)
		got, reason, _ := readAttestation(report, "stryker", sha)
		if reason != c.want {
			t.Errorf("%s: reason %q, want %q", c.name, reason, c.want)
		}
		if c.want == "" && got.Report != "incremental.json" {
			t.Errorf("valid attestation not returned: %+v", got)
		}
	}
}
