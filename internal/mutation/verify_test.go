package mutation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A real module with a real test, so the proof is end to end rather than against a fake runner.
func fixtureRepo(t *testing.T, guard, test string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module fixture\n\ngo 1.22\n")
	write("calc.go", "package fixture\n\n// Allow reports whether n is within the limit.\nfunc Allow(n int) bool {\n\treturn "+guard+"\n}\n")
	write("calc_test.go", "package fixture\n\nimport \"testing\"\n\n"+test)
	return dir
}

// The property the whole fix phase rests on: a pin is proven only when breaking the line makes
// the test fail AND restoring it makes the test pass. Either half alone is worthless — a test
// that always fails "detects" everything, and a test that always passes detects nothing.
func TestVerifyProvesAPinnedLine(t *testing.T) {
	// A test that genuinely pins the boundary: it asserts Allow(10) is false, which only holds
	// for `n < 10`, not for `n <= 10`.
	dir := fixtureRepo(t, "n < 10", "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n")
	v := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}
	res, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "TestBoundary"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	if !res[0].Proven {
		t.Errorf("a pinned line must verify: %+v", res[0])
	}
}

// The case that matters most: the fix agent claims a pin, but no test actually holds the line.
// Before this node existed the FSM believed the claim.
func TestVerifyRefusesAnUnpinnedClaim(t *testing.T) {
	// A test that exercises the function but asserts nothing about the boundary.
	dir := fixtureRepo(t, "n < 10", "func TestItRuns(t *testing.T) {\n\t_ = Allow(1)\n}\n")
	v := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}
	res, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "TestItRuns"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res[0].Proven {
		t.Error("a claim no test holds must NOT verify — this is the whole point of the node")
	}
	if !strings.Contains(strings.ToLower(res[0].Detail), "survived") {
		t.Errorf("the reason must say the mutation survived, got %q", res[0].Detail)
	}
}

// A mutation that does not compile fails every test for a useless reason, so it proves nothing.
// Two of the orchestrator's own verification mutations had this flaw on 2026-08-29.
func TestVerifyRejectsAMutationThatDoesNotCompile(t *testing.T) {
	dir := fixtureRepo(t, "n < 10", "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"no\")\n\t}\n}\n")
	v := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}
	res, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "n < 10", To: "n <<< 10", Test: "TestBoundary"}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res[0].Proven {
		t.Error("a non-compiling mutation must not count as proof")
	}
	if !strings.Contains(strings.ToLower(res[0].Detail), "compile") {
		t.Errorf("the reason must name the build failure, got %q", res[0].Detail)
	}
}

// Text that appears more than once cannot be mutated unambiguously, and text that appears not at
// all means the pin does not describe this tree.
func TestVerifyRefusesAmbiguousOrAbsentAnchors(t *testing.T) {
	dir := fixtureRepo(t, "n < 10", "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"no\")\n\t}\n}\n")
	v := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}
	res, err := v.Verify(context.Background(), []Pin{
		{File: "calc.go", From: "n", To: "m", Test: "T"},
		{File: "calc.go", From: "not present anywhere", To: "x", Test: "T"},
		{File: "missing.go", From: "a", To: "b", Test: "T"},
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for i, want := range []string{"once", "once", "read"} {
		if res[i].Proven {
			t.Errorf("result %d must not be proven: %+v", i, res[i])
		}
		if !strings.Contains(strings.ToLower(res[i].Detail), want) {
			t.Errorf("result %d detail = %q, want it to mention %q", i, res[i].Detail, want)
		}
	}
}

// The live tree is never touched: the fix agent that left its mutation in run.go on 2026-08-29
// is the reason this is asserted rather than assumed.
func TestVerifyLeavesTheTreeExactlyAsItFoundIt(t *testing.T) {
	dir := fixtureRepo(t, "n < 10", "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"no\")\n\t}\n}\n")
	before, err := os.ReadFile(filepath.Join(dir, "calc.go"))
	if err != nil {
		t.Fatal(err)
	}
	v := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}
	if _, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "T"}}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "calc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("verification modified the tree it was given")
	}
}

// End to end: a fix agent's claim, decoded and bounded as the FSM decodes it, carried as the FSM
// carries it, verified against a real repository with a real build and real test runs, and turned
// into the findings a gate blocks on.
//
// This is the property the fix phase exists for. Before it, `agent-edit` returned
// {commit, summary} and the machine believed it: nothing recorded what a fix pinned and nothing
// proved it held. Here the honest claim is proven and the empty one is refused, by running the
// tests rather than by asking anyone.
func TestEndToEndAClaimIsProvenOrRefused(t *testing.T) {
	for _, tc := range []struct {
		name        string
		test        string
		wantProven  bool
		wantFinding bool
	}{
		{
			name:        "a fix whose test really holds the line",
			test:        "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n",
			wantProven:  true,
			wantFinding: false,
		},
		{
			name:        "a fix whose test only executes the line",
			test:        "func TestItRuns(t *testing.T) {\n\t_ = Allow(1)\n}\n",
			wantProven:  false,
			wantFinding: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureRepo(t, "n < 10", tc.test)
			pins := []Pin{{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "TestBoundary"}}

			results, err := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}.Verify(context.Background(), pins)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if results[0].Proven != tc.wantProven {
				t.Fatalf("Proven = %v, want %v (%s)", results[0].Proven, tc.wantProven, results[0].Detail)
			}

			// What the gate sees.
			found := FindingsForPins(results)
			if got := len(found) > 0; got != tc.wantFinding {
				t.Fatalf("findings = %d, want any = %v", len(found), tc.wantFinding)
			}
			if tc.wantFinding {
				f := found[0]
				if f.Classification != "blocking" {
					t.Errorf("an unproven fix must block, got %q", f.Classification)
				}
				if f.Evidence == nil || f.Evidence[0].Path != "calc.go" {
					t.Errorf("the finding must name the file: %+v", f.Evidence)
				}
				if !strings.Contains(f.Found, "survived") {
					t.Errorf("the finding must say what happened, got %q", f.Found)
				}
			}
		})
	}
}

// tail() trims a command's output for a finding. Its boundary and its truncation branch were both
// unpinned — mutating `len(s) > 400` to `>=` survived, and the truncating path was never executed
// at all. Found by running our own mutation standard against this package.
func TestTailTrimsAtItsBoundary(t *testing.T) {
	exactly400 := strings.Repeat("a", 400)
	if got := tail(exactly400); got != exactly400 {
		t.Errorf("output of exactly 400 bytes must be kept whole, got %d bytes", len(got))
	}
	over := strings.Repeat("b", 401)
	got := tail(over)
	if !strings.HasPrefix(got, "...") {
		t.Errorf("output past the boundary must be marked as truncated: %q", got[:10])
	}
	if len(got) != 403 {
		t.Errorf("truncated length = %d, want 403 (the marker plus the last 400)", len(got))
	}
	// Newlines become separators so a finding stays one line.
	if strings.Contains(tail("a\nb"), "\n") {
		t.Error("newlines must not survive into a finding")
	}
}

// copyTree must not carry .git into the working copy: it would double the cost of every
// verification and put a repository inside a repository. Non-regular files are skipped rather
// than followed.
func TestCopyTreeSkipsGitAndIrregularFiles(t *testing.T) {
	src := t.TempDir()
	for _, rel := range []string{"keep.go", ".git/config", "node_modules/pkg/index.js", "sub/keep2.go"} {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(src, "keep.go"), filepath.Join(src, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	for _, want := range []string{"keep.go", "sub/keep2.go"} {
		if _, err := os.Stat(filepath.Join(dst, want)); err != nil {
			t.Errorf("%s was not copied: %v", want, err)
		}
	}
	for _, skip := range []string{".git", "node_modules", "link.go"} {
		if _, err := os.Lstat(filepath.Join(dst, skip)); err == nil {
			t.Errorf("%s should not have been copied", skip)
		}
	}
}

// The build check is configurable because "does this compile" is not the same question in every
// language, and a Go-shaped default should not be a Go-shaped assumption.
func TestBuildCommandIsConfigurable(t *testing.T) {
	dir := fixtureRepo(t, "n < 10", "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"no\")\n\t}\n}\n")
	var ran [][]string
	v := Verifier{
		Dir: dir, TestCmd: []string{"go", "test", "./..."},
		BuildCmd: []string{"true"},
		Run: func(_ context.Context, _ string, argv []string) (int, string, error) {
			ran = append(ran, argv)
			return 0, "", nil
		},
	}
	// With everything stubbed to succeed, the mutation "survives" — what matters here is which
	// commands were chosen.
	if _, err := v.Verify(context.Background(), []Pin{{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "T"}}); err != nil {
		t.Fatal(err)
	}
	var joined []string
	for _, a := range ran {
		joined = append(joined, strings.Join(a, " "))
	}
	if len(joined) < 2 || joined[1] != "true" {
		t.Errorf("the configured build command must be used, ran: %v", joined)
	}
}

// A harness with no test command cannot check anything, and must say so once rather than report
// every pin as survived — that would blame the fixes for the harness's own misconfiguration.
func TestVerifyRefusesWithNoTestCommand(t *testing.T) {
	v := Verifier{Dir: t.TempDir()}
	if _, err := v.Verify(context.Background(), []Pin{{File: "a.go", From: "x", To: "y", Test: "T"}}); err == nil {
		t.Error("a missing test command must be an error, not a wall of false blockers")
	}
	// With nothing to check there is nothing to misconfigure.
	if got, err := v.Verify(context.Background(), nil); err != nil || len(got) != 0 {
		t.Errorf("no pins: %v %+v", err, got)
	}
}

// A malformed pin is not a verdict on the fix. "I could not evaluate this claim" and "this claim
// is false" call for opposite responses — rewrite the pin, versus write a test — and collapsing
// them makes an agent with clumsy syntax indistinguishable from one shipping untested code.
func TestOutcomesDistinguishAMalformedPinFromAnUnheldFix(t *testing.T) {
	held := "func TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"no\")\n\t}\n}\n"
	for _, tc := range []struct {
		name string
		pin  Pin
		want Outcome
	}{
		{"a mutation the test catches", Pin{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "T"}, PinProven},
		{"a mutation nothing catches", Pin{File: "calc.go", From: "// Allow reports", To: "// allow reports", Test: "T"}, PinSurvived},
		{"a mutation that does not compile", Pin{File: "calc.go", From: "n < 10", To: "n <<< 10", Test: "T"}, PinMalformed},
		{"an anchor that appears twice", Pin{File: "calc.go", From: "n", To: "m", Test: "T"}, PinMalformed},
		{"an anchor that appears nowhere", Pin{File: "calc.go", From: "not here", To: "x", Test: "T"}, PinMalformed},
		{"a file that is not there", Pin{File: "gone.go", From: "a", To: "b", Test: "T"}, PinMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureRepo(t, "n < 10", held)
			got, err := Verifier{Dir: dir, TestCmd: []string{"go", "test", "./..."}}.Verify(context.Background(), []Pin{tc.pin})
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if got[0].Outcome != tc.want {
				t.Errorf("Outcome = %q, want %q (%s)", got[0].Outcome, tc.want, got[0].Detail)
			}
			if (got[0].Outcome == PinProven) != got[0].Proven {
				t.Errorf("Proven must agree with Outcome: %+v", got[0])
			}
		})
	}
}

// Severity follows the distinction: an unheld fix blocks, a malformed pin is worth fixing but is
// not evidence that the code is wrong.
func TestFindingSeverityFollowsTheOutcome(t *testing.T) {
	got := FindingsForPins([]PinResult{
		{Pin: Pin{File: "a.go"}, Outcome: PinProven, Proven: true},
		{Pin: Pin{File: "b.go"}, Outcome: PinSurvived, Detail: "the mutation survived"},
		{Pin: Pin{File: "c.go"}, Outcome: PinMalformed, Detail: "does not compile"},
	})
	if len(got) != 2 {
		t.Fatalf("a proven pin produces nothing, got %d findings", len(got))
	}
	bySeverity := map[string]string{}
	for _, f := range got {
		bySeverity[f.Evidence[0].Path] = f.Severity + "/" + f.Classification
	}
	if bySeverity["b.go"] != "high/blocking" {
		t.Errorf("an unheld fix must block: %s", bySeverity["b.go"])
	}
	if bySeverity["c.go"] != "medium/advisory" {
		t.Errorf("a malformed pin is advisory, not a verdict on the code: %s", bySeverity["c.go"])
	}
}
