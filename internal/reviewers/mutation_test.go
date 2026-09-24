package reviewers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

// The ordinary repository runs no mutation engine, and must review exactly as it did before.
// This gate is opt-in, and it never raises "you should be running mutation testing" — a gate that
// scolds you for not opting in is a gate people opt out of.
func TestNoMutationReportsChangeNothing(t *testing.T) {
	if got := (MutationContext{}).Findings(); len(got) != 0 {
		t.Errorf("an absent engine raises nothing: %+v", got)
	}
	plain := RunTaskDone(Context{Git: GitContext{ChangedFiles: []string{"a.go"}}})
	withEmpty := RunTaskDone(Context{Git: GitContext{ChangedFiles: []string{"a.go"}}, Mutation: MutationContext{}})
	if len(plain) != len(withEmpty) {
		t.Errorf("an empty mutation context changed the review: %d vs %d", len(plain), len(withEmpty))
	}
}

// Two engines over the same package is a thing the roadmap wants — gremlins and ooze disagreed by
// 137 mutants on one package, so their union finds more than either alone — and it must not turn
// the site they AGREE on into two findings.
func TestTwoEnginesDoNotDoubleReportTheSameSite(t *testing.T) {
	agreed := mutation.Mutant{Status: mutation.Survived, File: "bind.go", Line: 53, Operator: "INVERT_NEGATIVES"}
	onlyOoze := mutation.Mutant{Status: mutation.Survived, File: "bind.go", Line: 77, Operator: "SWAP_BRANCH"}
	ctx := MutationContext{Reports: []mutation.Report{
		{Engine: "ooze", Target: "./internal/findings", Mutants: []mutation.Mutant{agreed, onlyOoze}},
		{Engine: "gremlins", Target: "./internal/findings", Mutants: []mutation.Mutant{agreed}},
	}}
	got := ctx.Findings()
	if len(got) != 2 {
		t.Fatalf("the agreed site is one finding and the extra one is another, got %d: %+v", len(got), got)
	}
	// Reports are sorted before translation, so the same inputs always produce the same list.
	// A review log that reshuffles between runs cannot be read as a diff.
	if got[0].Reviewer != "mutation-gremlins" {
		t.Errorf("engines must be ordered deterministically, got %q first", got[0].Reviewer)
	}
	if got[0].Fingerprint == got[1].Fingerprint {
		t.Error("two distinct sites collapsed into one fingerprint")
	}
}

// Mutation findings are ordinary findings: they join the same ledger the deterministic lints use,
// so one run chain, one set of fingerprints and one override mechanism cover both.
func TestMutationFindingsJoinEverySurface(t *testing.T) {
	ctx := MutationContext{Reports: []mutation.Report{{
		Engine:  "gremlins",
		Mutants: []mutation.Mutant{{Status: mutation.Survived, File: "a.go", Line: 3, Operator: "INVERT_NEGATIVES"}},
	}}}
	surfaces := map[string][]Finding{
		"task-done":  RunTaskDone(Context{Mutation: ctx}),
		"pr-ready":   RunPRReady(PRReadyContext{Mutation: ctx}),
		"epic-ready": RunEpicReady(EpicReadyContext{Mutation: ctx}),
	}
	for name, got := range surfaces {
		var found bool
		for _, f := range got {
			if f.Reviewer == "mutation-gremlins" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s did not surface the surviving mutant: %+v", name, got)
		}
	}
}

// Two engines disagreeing about one file must not silently become one engine's answer.
//
// The uncovered fingerprint was once "mutation:uncovered:<file>" with no engine in it, so the
// dedupe above treated gremlins' and stryker's reports of the same file as the same claim and
// discarded the second — losing its sites and leaving the kept finding's count wrong. Engines
// genuinely disagree here (gremlins and ooze differed by 137 mutants on one package during this
// work), so dropping one is data loss, not deduplication.
func TestTwoEnginesUncoveredInOneFileAreTwoClaims(t *testing.T) {
	site := func(line int) mutation.Mutant {
		// The typed constant, never a literal: this test first used "NoCoverage", which is not a
		// status this package knows, so every mutant fell through to `unresolved` and the test
		// passed without ever reaching the uncovered path it exists to guard.
		return mutation.Mutant{File: "lib/parser.js", Line: line, Status: mutation.Uncovered, Operator: "Cond"}
	}
	got := MutationContext{Reports: []mutation.Report{
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
		{Engine: "stryker", Mutants: []mutation.Mutant{site(99)}},
	}}.Findings()
	if len(got) != 2 {
		t.Fatalf("two engines, two claims; got %d: %+v", len(got), got)
	}
	all := got[0].Finding + "\n" + got[1].Finding
	for _, want := range []string{"10", "99"} {
		if !strings.Contains(all, want) {
			t.Errorf("line %s was dropped:\n%s", want, all)
		}
	}
	if got[0].Fingerprint == got[1].Fingerprint {
		t.Errorf("both engines share fingerprint %q, so one will be dropped", got[0].Fingerprint)
	}
	// The same engine reporting the same file twice IS one claim, and still collapses.
	same := MutationContext{Reports: []mutation.Report{
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
		{Engine: "gremlins", Mutants: []mutation.Mutant{site(10)}},
	}}.Findings()
	if len(same) != 1 {
		t.Errorf("one engine repeating itself is one finding, got %d", len(same))
	}
}

// The unresolved fingerprint must not depend on where the report file was written.
//
// Load fills Target with the report's file path, so keying on it gave a CI job writing
// /tmp/build-1234/mut.json and a developer's local copy different fingerprints for the same run.
// Fingerprint identity is what makes an `overridden` record suppress rediscovery, so a granted
// override on this — the highest-severity finding the package raises — silently failed to
// rematch, and nothing surfaced the failure.
func TestTheUnresolvedFingerprintSurvivesADifferentPath(t *testing.T) {
	// Anything the package does not recognise is undecided; "timeout" is what gremlins emits.
	undecided := []mutation.Mutant{{File: "a.go", Line: 4, Status: "timeout", Operator: "Cond"}}
	fp := func(target string) string {
		f := (mutation.Report{Engine: "gremlins", Target: target, Mutants: undecided}).Findings()
		if len(f) != 1 {
			t.Fatalf("want one undecided finding, got %d", len(f))
		}
		return f[0].Fingerprint
	}
	ci, local := fp("/tmp/build-1234/mut.json"), fp("/Users/dev/project/mut.json")
	if ci != local {
		t.Errorf("the same run keys differently by path:\n  ci    %s\n  local %s", ci, local)
	}
	// It still distinguishes genuinely different claims: another engine, and other mutants.
	other := (mutation.Report{Engine: "stryker", Mutants: undecided}).Findings()[0].Fingerprint
	if other == ci {
		t.Error("two engines' undecided sets must not share a fingerprint")
	}
	elsewhere := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "b.go", Line: 9, Status: "timeout"}}}).Findings()[0].Fingerprint
	if elsewhere == ci {
		t.Error("different undecided mutants are different claims")
	}

	// A large failure must not produce an unbounded key. The measured gremlins run left 97
	// mutants undecided from one bad timeout, so this is the ordinary case, not the edge: the
	// key names the first few sites and then the count, and stays a fingerprint rather than
	// becoming a transcript.
	many := func(n int, file string) string {
		ms := make([]mutation.Mutant, n)
		for i := range ms {
			ms[i] = mutation.Mutant{File: file, Line: i + 1, Status: "timeout"}
		}
		return (mutation.Report{Engine: "gremlins", Mutants: ms}).Findings()[0].Fingerprint
	}
	big := many(97, "a.go")
	if len(big) > 200 {
		t.Errorf("the key grows with the failure (%d chars): %s", len(big), big)
	}
	// The WHOLE set is part of the identity, not a prefix of it. A capped list collided: two
	// reports from one engine sharing their first eight sites and their total count keyed
	// identically despite differing later, so the second was dropped by the dedupe and an
	// override granted for one rematched the other. Both bots on #24 caught this; the first
	// version of this test did not, because it only ever compared 97 sites against 96.
	shared := func(differAt int) string {
		ms := make([]mutation.Mutant, 20)
		for i := range ms {
			ms[i] = mutation.Mutant{File: "a.go", Line: i + 1, Status: "timeout"}
		}
		ms[differAt].Line = 900 + differAt
		return (mutation.Report{Engine: "gremlins", Mutants: ms}).Findings()[0].Fingerprint
	}
	if shared(19) == shared(18) {
		t.Error("two sets identical in their first eight sites and count must not share a fingerprint")
	}
	// Concatenating "file:line" strings without a length prefix lets the boundary move: one
	// mutant in a file named "a:1b" at line 2 concatenates to exactly what two mutants at a.go:1
	// and b.go:2 do. A colon in a path is unusual, not impossible, and a fingerprint that can be
	// forged by a filename is not an identity.
	keyOf := func(ms ...mutation.Mutant) string {
		return (mutation.Report{Engine: "gremlins", Mutants: ms}).Findings()[0].Fingerprint
	}
	two := keyOf(mutation.Mutant{File: "a", Line: 1, Status: "timeout"},
		mutation.Mutant{File: "b", Line: 2, Status: "timeout"})
	one := keyOf(mutation.Mutant{File: "a:1b", Line: 2, Status: "timeout"})
	if two == one {
		t.Errorf("a colon in a path collapsed two claims into one: %s", two)
	}
	// Enough entropy to be an identity. A short digest passes every collision test above (four
	// fixtures rarely collide in one byte) and then collides constantly against a real repo's
	// worth of reports, which is the failure mode that matters: an override rematching a finding
	// it was never granted for. 128 bits is the floor, and the key stays bounded regardless.
	digest := strings.TrimPrefix(big, "mutation:unresolved:gremlins:")
	if len(digest) < 32 {
		t.Errorf("the identity carries only %d hex chars: %s", len(digest), big)
	}
	// Truncation must not erase the difference between two different large failures.
	if big == many(96, "a.go") {
		t.Error("97 undecided and 96 undecided must not share a fingerprint")
	}
	if big == many(97, "b.go") {
		t.Error("the same count in a different file must not share a fingerprint")
	}
	// Order of arrival is not part of the claim.
	shuffled := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "z.go", Line: 2, Status: "timeout"}, {File: "a.go", Line: 1, Status: "timeout"}}}).Findings()[0].Fingerprint
	reversed := (mutation.Report{Engine: "gremlins", Mutants: []mutation.Mutant{
		{File: "a.go", Line: 1, Status: "timeout"}, {File: "z.go", Line: 2, Status: "timeout"}}}).Findings()[0].Fingerprint
	if shuffled != reversed {
		t.Errorf("emission order changed the fingerprint:\n  %s\n  %s", shuffled, reversed)
	}
}

// A repository with src/a.ts committed and an attested report whose one kill is in src/a.ts.
func freshnessRepo(t *testing.T) (root, report string) {
	t.Helper()
	root = t.TempDir()
	gitIn := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/a.ts"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn("init", "-q")
	gitIn("config", "user.email", "t@example.com")
	gitIn("config", "user.name", "T")
	gitIn("add", "-A")
	gitIn("commit", "-qm", "init")
	state := filepath.Join(root, ".state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	reportText := `{"files":{"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{"t.test.ts":{"tests":[{"id":"t1"}]}}}`
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
		"lists":{"mutate":["src/**"],"test":["*.test.ts"],"support":[],"global":[],"ignore":[".state/**"]},"exclusions":[".state/**"],
		"files":{"src/a.ts":{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum(reportText), sum("a"))
	report = filepath.Join(state, "incremental.json")
	if err := os.WriteFile(report, []byte(reportText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "attestation.json"), []byte(att), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, report
}

func TestLoadMutationContext(t *testing.T) {
	if ctx, err := LoadMutationContext(t.TempDir(), nil, nil, "pr-ready", true); err != nil || ctx.Mode != "" || len(ctx.Freshness) != 0 {
		t.Errorf("no reports: nothing loaded, nothing serialized: %+v %v", ctx, err)
	}
	root, report := freshnessRepo(t)
	ctx, err := LoadMutationContext(root, []string{report}, nil, "task-done", false)
	if err != nil || ctx.Mode != "advisory" || ctx.Freshness[0].Verified != 1 || !strings.Contains(ctx.FreshnessSection, "1 verified") {
		t.Fatalf("worktree: %+v %v", ctx, err)
	}
	if got := ctx.Engines(); len(got) != 1 || got[0] != "stryker" {
		t.Errorf("engines %v", got)
	}
	// Uncommitted: stale in the working tree, not in HEAD mode.
	if err := os.WriteFile(filepath.Join(root, "src/a.ts"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
	wt, _ := LoadMutationContext(root, []string{report}, nil, "pr-ready", false)
	head, _ := LoadMutationContext(root, []string{report}, nil, "pr-ready", true)
	if wt.Freshness[0].Stale != 1 || head.Freshness[0].Stale != 0 || wt.Mode != "enforce" {
		t.Errorf("working tree %+v, HEAD %+v", wt.Freshness[0], head.Freshness[0])
	}
	found := false
	for _, f := range wt.Findings() {
		found = found || (strings.HasPrefix(f.Fingerprint, "mutation:stale:enforce:stryker:src/a.ts:") && f.Classification == "blocking")
	}
	if !found {
		t.Errorf("the enforced stale finding joins the context's findings: %+v", wt.Findings())
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "bogus")
	if _, err := LoadMutationContext(root, []string{report}, nil, "pr-ready", false); err == nil {
		t.Error("an invalid mode is an error")
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "")
	if _, err := LoadMutationContext(root, []string{filepath.Join(root, "missing.json")}, nil, "pr-ready", false); err == nil {
		t.Error("an unreadable report is an error")
	}
	if _, err := LoadMutationContext(t.TempDir(), []string{report}, nil, "pr-ready", false); err == nil {
		t.Error("a content error (not a repository) is an error")
	}
}
