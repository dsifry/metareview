package judge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// The vendored eval corpus is a stratified sample of the harnesseval readjudication3
// ground truth (issue #140): every hallucinated testing-gap claim the v2 re-adjudication
// found on metareview runs, plus bug/important/unresolved claims and non-claim findings
// for detector precision, with the (minimized) vendored diffs they were judged against —
// three Discourse test-section diffs; the cal.com records carry diff_file "" because those
// PRs change no test-shaped files at all. The detect and
// evidence expectations are generated (CLAIMCORPUS_UPDATE=1) so a matcher change that
// moves evidence on REAL review data fails here instead of silently drifting — the same
// discipline the prompt goldens enforce for prompts.
//
// Regenerate only after reviewing what moved:
//
//	CLAIMCORPUS_UPDATE=1 go test ./internal/fsm/judge/ -run TestGapClaimEvalCorpus
type corpusRecord struct {
	DiffFile  string   `json:"diff_file"`
	IssueText string   `json:"issue_text"`
	Lens      string   `json:"lens"`
	V2Verdict string   `json:"v2_verdict"`
	Detect    bool     `json:"detect"`
	Evidence  []string `json:"evidence"`
}

func TestGapClaimEvalCorpus(t *testing.T) {
	update := os.Getenv("CLAIMCORPUS_UPDATE") == "1"
	raw, err := os.ReadFile(filepath.Join("testdata", "evalcorpus", "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Source  string         `json:"source"`
		Note    string         `json:"note"`
		Records []corpusRecord `json:"records"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Records) == 0 {
		t.Fatal("empty corpus")
	}

	// Aggregate pins: the numbers that motivated issue #140. A matcher change that moves
	// these needs a conscious decision, not just a golden regen.
	var hallucinated, hallucinatedWithEvidence int
	for i := range corpus.Records {
		rec := corpus.Records[i]
		_, detected := claimcheck.Detect(rec.IssueText)
		if detected != rec.Detect {
			t.Errorf("Detect drifted on %q: got %v, want %v (lens %s, verdict %s)",
				clipCorpus(rec.IssueText), detected, rec.Detect, rec.Lens, rec.V2Verdict)
		}
		// The aggregate pins count every hallucinated gap-claim, diff or no diff: the
		// population is the point (#140), not just the measurable half of it.
		if rec.V2Verdict == "hallucination" && rec.Detect {
			hallucinated++
		}
		// A record with no diff_file comes from a PR whose diff carries no test-shaped
		// files (the minimized corpus keeps only test sections): there is nothing to
		// search, so its expectation is empty by construction and Detect is the pin.
		// The update write happens BEFORE the continue so regeneration refreshes these
		// records too — the shard review reproduced a stale detect flag surviving a
		// "rewrote 131 records" run that had skipped them.
		if rec.DiffFile == "" {
			if update {
				rec.Detect, rec.Evidence = detected, nil
				corpus.Records[i] = rec
			}
			if len(rec.Evidence) != 0 {
				t.Errorf("a record with no diff must expect no evidence: %q", clipCorpus(rec.IssueText))
			}
			continue
		}
		diff, err := os.ReadFile(filepath.Join("testdata", "evalcorpus", rec.DiffFile))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if detected {
			for _, e := range GapClaimEvidence(string(diff), run.Finding{IssueText: rec.IssueText}, MaxGapEvidenceFiles) {
				got = append(got, e.Path)
			}
		}
		if update {
			rec.Detect, rec.Evidence = detected, got
			corpus.Records[i] = rec
			continue
		}
		if !equalStrings(got, rec.Evidence) {
			t.Errorf("evidence drifted on %q: got %v, want %v (lens %s, verdict %s)",
				clipCorpus(rec.IssueText), got, rec.Evidence, rec.Lens, rec.V2Verdict)
		}
		if rec.V2Verdict == "hallucination" && rec.Detect && len(got) > 0 {
			hallucinatedWithEvidence++
		}
	}
	if update {
		out, err := json.MarshalIndent(corpus, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "evalcorpus", "records.json"), append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %d records; review the diff before committing", len(corpus.Records))
		return
	}
	if hallucinated < 15 {
		t.Errorf("hallucinated gap-claims in corpus = %d; the vendored sample must keep the population that motivated #140 (want ≥ 15)", hallucinated)
	}
	if hallucinatedWithEvidence < 10 {
		t.Errorf("hallucinated gap-claims with covering evidence = %d, want ≥ 10: the evidence search is what makes the judge able to catch them", hallucinatedWithEvidence)
	}
	t.Logf("corpus: %d records, %d hallucinated gap-claims, %d with covering evidence",
		len(corpus.Records), hallucinated, hallucinatedWithEvidence)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func clipCorpus(s string) string {
	if len(s) <= 110 {
		return s
	}
	return s[:110] + "…"
}

// The vendored file carries a note stating the regeneration contract; CLAIMCORPUS_UPDATE
// re-marshals the decoded struct, so the note must survive a round trip or the first
// regen silently strips it.
func TestGapClaimEvalCorpusPreservesVendorNote(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "evalcorpus", "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	var withNote struct {
		Source  string     `json:"source"`
		Note    string     `json:"note"`
		Records []struct{} `json:"records"`
	}
	if err := json.Unmarshal(raw, &withNote); err != nil {
		t.Fatal(err)
	}
	if withNote.Note == "" {
		t.Fatal("the vendored corpus must carry its regeneration-contract note")
	}
	// the update path re-marshals the corpus struct — the note must round-trip through it
	var corpus struct {
		Source  string         `json:"source"`
		Note    string         `json:"note"`
		Records []corpusRecord `json:"records"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	corpus.Records = corpus.Records[:0]
	out, err := json.MarshalIndent(corpus, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Note string `json:"note"`
	}
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Note != withNote.Note {
		t.Fatalf("note did not round-trip: %q != %q", back.Note, withNote.Note)
	}
}
