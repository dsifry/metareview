package prready

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/reviewers"
)

// freshnessReport writes, outside the repository, an attested Stryker report whose one kill is in
// file, attesting file's committed content (spec §6.1).
func freshnessReport(t *testing.T, root, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, file))
	if err != nil {
		t.Fatal(err)
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	report := fmt.Sprintf(`{"files":{%q:{"source":"x","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{"t.test.ts":{"tests":[{"id":"t1"}]}}}`, file)
	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
		"lists":{"mutate":[],"test":[],"support":[],"global":[],"ignore":["**",".metareview/**","docs/**"]},"exclusions":[],
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

func commitAll(t *testing.T, root, message string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", message}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func staleRows(t *testing.T, root string) []string {
	t.Helper()
	records, err := findings.All(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range records {
		if strings.HasPrefix(r.Fingerprint, "mutation:stale:") {
			out = append(out, r.Status)
		}
	}
	return out
}

// Spec §6.2–§6.5 at pr-ready: HEAD is under review, enforce blocks on stale evidence, and fresh
// evidence supersedes the stale row.
func TestCreateJudgesMutationFreshnessAgainstHEAD(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
	root := smallPRReadyRepo(t)
	report := freshnessReport(t, root, "seed.txt")
	// Uncommitted: pr-ready reads HEAD, so nothing is stale yet.
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Create(root, Options{Base: "main", MutationReportPaths: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(filepath.Join(root, res.ReviewRel))
	if !strings.Contains(string(log), "- Kills: 1 verified, 0 stale") || !strings.Contains(string(log), "- Mode: `enforce`") {
		t.Fatalf("an uncommitted edit is not under review in HEAD mode:\n%s", log)
	}
	// Committed: the kill is stale, and under enforce it blocks.
	commitAll(t, root, "edit")
	res, err = Create(root, Options{Base: "main", MutationReportPaths: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	log, _ = os.ReadFile(filepath.Join(root, res.ReviewRel))
	if res.Verdict != "NEEDS_REVISION" || !strings.Contains(string(log), "Mutation evidence stale: seed.txt changed") {
		t.Fatalf("committed edit under enforce: verdict %s\n%s", res.Verdict, log)
	}
	if got := staleRows(t, root); len(got) != 1 || got[0] != "open" {
		t.Fatalf("stale rows %v", got)
	}
	// Fresh evidence for the committed content supersedes the stale row.
	fresh := freshnessReport(t, root, "seed.txt")
	if _, err := Create(root, Options{Base: "main", MutationReportPaths: []string{fresh}}); err != nil {
		t.Fatal(err)
	}
	if got := staleRows(t, root); len(got) != 1 || got[0] != findings.StatusSuperseded {
		t.Errorf("fresh evidence must supersede the stale row: %v", got)
	}
}

// Spec §6.7: a run without reports keeps its reviewer-input digest — the new fields are omitempty.
func TestNoReportsKeepTheReviewerInputDigestShape(t *testing.T) {
	encoded, err := json.Marshal(reviewers.MutationContext{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"Reports":null}` {
		t.Errorf("an empty mutation context must serialize as before, got %s", encoded)
	}
}
