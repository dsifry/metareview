package taskdone

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// freshnessReport writes, outside the repository, an attested Stryker report whose one kill is in
// file (spec §6.1), with file's current content as the attested digest.
func freshnessReport(t *testing.T, root, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		t.Fatal(err)
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	report := fmt.Sprintf(`{"files":{%q:{"source":"x","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{"t.test.ts":{"tests":[{"id":"t1"}]}}}`, file)
	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
		"lists":{"mutate":["src/**"],"test":["*.test.ts"],"support":[],"global":[],"ignore":["**"]},"exclusions":[],
		"files":{%q:{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum([]byte(report)), file, sum(data))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "incremental.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attestation.json"), []byte(att), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "incremental.json")
}

// Spec §6.4–§6.6 at task-done: the freshness section is written, and task-done stays advisory
// even under enforce.
func TestCreateJudgesMutationFreshness(t *testing.T) {
	root := smallTaskRepo(t)
	report := freshnessReport(t, root, "src/small.go")
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
	if err := os.WriteFile(filepath.Join(root, "src/small.go"), []byte("package src\n\nvar X = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Create(root, smallTarget, Options{Base: "main", MutationReportPaths: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(filepath.Join(root, res.ReviewRel))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Mutation Evidence Freshness", "- Mode: `advisory`", "- Kills: 0 verified, 1 stale", "Mutation evidence stale: src/small.go changed"} {
		if !strings.Contains(string(log), want) {
			t.Errorf("review log lacks %q:\n%s", want, log)
		}
	}
}
