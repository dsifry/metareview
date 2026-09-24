package epicready

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Spec §6.6 at epic-ready: the freshness section follows the verdict line.
func TestCreateWritesTheMutationFreshnessSection(t *testing.T) {
	root := epicRepo(t)
	data, err := os.ReadFile(filepath.Join(root, "src/a.go"))
	if err != nil {
		t.Fatal(err)
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	report := `{"files":{"src/a.go":{"source":"x","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{}}`
	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
		"lists":{"mutate":[],"test":[],"support":[],"global":[],"ignore":["**",".beads/**",".metareview/**","docs/**"]},"exclusions":[],
		"files":{"src/a.go":{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum([]byte(report)), sum(data))
	dir := t.TempDir()
	for name, body := range map[string]string{"incremental.json": report, "attestation.json": att} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Create(root, "epic-1", Options{Base: "main", MutationReportPaths: []string{filepath.Join(dir, "incremental.json")}})
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(root, res.ReviewRel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "## Mutation Evidence Freshness\n\n- Mode: `advisory`") || !strings.Contains(string(log), "- Kills: 1 verified") {
		t.Errorf("review log lacks the freshness section:\n%s", log)
	}
}
