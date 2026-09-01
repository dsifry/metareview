package kind

import (
	"context"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeExec is a gate.Exec that answers the git verbs the reproduction engine issues, driving a full
// proven path through the REAL engine without a repository — so the adapter's git closure and the
// engine→run result mapping are both exercised.
func fakeExec(t *testing.T) func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
	t.Helper()
	return func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		// Skip any leading `-c <value>` config pairs the engine prefixes onto a call.
		i := 0
		for i+1 < len(args) && args[i] == "-c" {
			i += 2
		}
		verb := ""
		if i < len(args) {
			verb = args[i]
		}
		switch verb {
		case "diff":
			return []byte("A\tpkg/x_test.go\n"), nil, 0, nil
		case "show":
			return []byte("package pkg\n"), nil, 0, nil
		default: // worktree add / remove
			return nil, nil, 0, nil
		}
	}
}

func TestReproductionProverEmpty(t *testing.T) {
	rs, err := ReproductionProver{}.ProveReproductions(context.Background(), nil, ProveSpec{})
	if err != nil || rs != nil {
		t.Fatalf("no proofs: %v %v", rs, err)
	}
}

// The adapter maps the engine's outcome onto a run.ProofResult carrying the originating proof, and
// forwards git through the injected Exec seam. A genuine assertion fail-before / pass-after is proven.
func TestReproductionProverMapsProven(t *testing.T) {
	p := reproDP("f1")
	calls := 0
	rp := ReproductionProver{
		Exec: fakeExec(t),
		Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
			calls++
			if calls == 1 {
				return 1, "=== RUN   T\n--- FAIL: T\n", nil
			}
			return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
		},
	}
	spec := ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}
	rs, err := rp.ProveReproductions(context.Background(), []run.DifferentialProof{p}, spec)
	if err != nil {
		t.Fatalf("ProveReproductions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven || rs[0].Outcome != run.PinProven || rs[0].Proof.ID != p.ID {
		t.Fatalf("a proven reproduction must map onto its proof: %+v", rs)
	}
}

// A misconfiguration (no test command) surfaces as an error the node then aborts on, not a silent
// pass — the engine's own guard, propagated through the adapter.
func TestReproductionProverPropagatesEngineError(t *testing.T) {
	rp := ReproductionProver{Exec: fakeExec(t)}
	spec := ProveSpec{Dir: t.TempDir(), PreFixSHA: "pre", PostFixSHA: "post"} // no TestCmd
	if _, err := rp.ProveReproductions(context.Background(), []run.DifferentialProof{reproDP("f1")}, spec); err == nil {
		t.Fatal("a missing test command must surface as an error")
	}
}

// Provers routes each kind to its own engine: pins to the mutation prover, reproductions to the
// reproduction prover.
func TestProversRoutesByKind(t *testing.T) {
	pinRun := func(_ context.Context, _ string, argv []string) (int, string, error) {
		return 1, "FAIL", nil // any non-zero so the pin path completes
	}
	reproCalls := 0
	p := Provers{
		Mutation: MutationProver{Run: pinRun},
		Reproduction: ReproductionProver{
			Exec: fakeExec(t),
			Run: func(_ context.Context, _ string, _ []string) (int, string, error) {
				reproCalls++
				if reproCalls == 1 {
					return 1, "=== RUN   T\n--- FAIL: T\n", nil
				}
				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
			},
		},
	}
	spec := ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}

	// A pin flows to the mutation prover: the pin's baseline is checked and the result comes back.
	pin := pinDP("f1", "addedLine")
	if _, err := p.ProvePins(context.Background(), []run.DifferentialProof{pin}, spec); err != nil {
		t.Fatalf("ProvePins: %v", err)
	}
	// A reproduction flows to the reproduction prover and is proven end to end.
	rs, err := p.ProveReproductions(context.Background(), []run.DifferentialProof{reproDP("f2")}, spec)
	if err != nil {
		t.Fatalf("ProveReproductions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven {
		t.Fatalf("the reproduction must be proven through the composite: %+v", rs)
	}
	if reproCalls == 0 {
		t.Fatal("the reproduction prover must have run")
	}
}

// delExec is a gate.Exec for deletion tests: it answers the binding diff (`git diff … -- File`), the
// partition diff (`--name-status`), and the worktree/show verbs. bindingDiff is returned for the
// binding read; the partition reports the prod file deleted plus a new test file added.
func delExec(bindingDiff string) gate.Exec {
	return func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		i := 0
		for i+1 < len(args) && args[i] == "-c" {
			i += 2
		}
		a := args[i:]
		switch a[0] {
		case "diff":
			for _, x := range a {
				if x == "--name-status" {
					return []byte("A\tpkg/repro_test.go\nD\ta.go\n"), nil, 0, nil
				}
			}
			return []byte(bindingDiff), nil, 0, nil
		case "show":
			return []byte("package pkg\n"), nil, 0, nil
		default: // worktree add / remove / prune
			return nil, nil, 0, nil
		}
	}
}

// A binding diff that removes the span from a.go.
const aRemovedDiff = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,1 @@\n ctx\n-gone()\n"

// delReviewerRun drives the reproduction (-run present: fail-before then pass-after) and the two scope
// runs (no -run: both green) so a well-formed deletion is proven end to end.
func delReviewerRun() (func(context.Context, string, []string) (int, string, error), *int) {
	repro := 0
	fn := func(_ context.Context, _ string, argv []string) (int, string, error) {
		hasRun := false
		for _, x := range argv {
			if x == "-run" {
				hasRun = true
			}
		}
		if hasRun {
			repro++
			if repro == 1 {
				return 1, "=== RUN   T\n--- FAIL: T\n", nil
			}
			return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
		}
		return 0, "ok\n", nil // whole-suite scope run: green
	}
	return fn, &repro
}

func delSpec(t *testing.T) ProveSpec {
	return ProveSpec{Dir: t.TempDir(), TestCmd: []string{"go", "test", "./..."}, PreFixSHA: "pre", PostFixSHA: "post"}
}

func delProof(removed string) run.DifferentialProof {
	return run.DifferentialProof{ID: "d1", Finding: "f1", Kind: run.ProofDeletion, Test: "T", Deletes: &run.DeletionRef{File: "a.go", ParentSHA: "pre", Removed: removed}}
}

func TestProveDeletionsEmpty(t *testing.T) {
	rs, err := ReproductionProver{}.ProveDeletions(context.Background(), nil, ProveSpec{})
	if err != nil || rs != nil {
		t.Fatalf("no proofs: %v %v", rs, err)
	}
}

// A well-formed deletion — the span is removed from its file, the reproduction holds, and the whole
// suite stays green — is proven end to end.
func TestProveDeletionProven(t *testing.T) {
	runFn, _ := delReviewerRun()
	rp := ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn}
	rs, err := rp.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, delSpec(t))
	if err != nil {
		t.Fatalf("ProveDeletions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven || rs[0].Outcome != run.PinProven {
		t.Fatalf("a well-formed deletion must be proven: %+v", rs)
	}
	if !strings.Contains(rs[0].FailBefore, "--- FAIL: T") {
		t.Fatalf("a proven deletion must carry the fail-before output for the §9.2 reviewer: %q", rs[0].FailBefore)
	}
}

// The binding rejects a span the reviewed diff does not remove (malformed), and a wrong parent anchor,
// an empty span, and a diff-read failure.
func TestProveDeletionBinding(t *testing.T) {
	runFn, _ := delReviewerRun()
	base := func() ReproductionProver { return ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn} }

	// span not among the removed lines → malformed.
	rs, _ := base().ProveDeletions(context.Background(), []run.DifferentialProof{delProof("neverRemoved()")}, delSpec(t))
	if rs[0].Outcome != run.PinMalformed || !strings.Contains(rs[0].Detail, "does not remove") {
		t.Fatalf("a span the diff does not remove must be malformed: %+v", rs[0])
	}
	// empty span → malformed.
	rs, _ = base().ProveDeletions(context.Background(), []run.DifferentialProof{delProof("")}, delSpec(t))
	if rs[0].Outcome != run.PinMalformed {
		t.Fatalf("an empty span must be malformed: %+v", rs[0])
	}
	// wrong parent anchor → malformed.
	p := delProof("gone()")
	p.Deletes.ParentSHA = "not-pre"
	rs, _ = base().ProveDeletions(context.Background(), []run.DifferentialProof{p}, delSpec(t))
	if rs[0].Outcome != run.PinMalformed || !strings.Contains(rs[0].Detail, "parent") {
		t.Fatalf("a wrong parent anchor must be malformed: %+v", rs[0])
	}
	// missing anchor → unverifiable.
	spec := delSpec(t)
	spec.PreFixSHA = ""
	rs, _ = base().ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, spec)
	if rs[0].Outcome != run.PinUnverifiable {
		t.Fatalf("a missing anchor must be unverifiable: %+v", rs[0])
	}
	// diff read failure → unverifiable.
	failExec := gate.Exec(func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
		i := 0
		for i+1 < len(args) && args[i] == "-c" {
			i += 2
		}
		if args[i] == "diff" {
			for _, x := range args {
				if x == "--name-status" {
					return []byte("D\ta.go\n"), nil, 0, nil
				}
			}
			return nil, nil, 128, nil // the binding diff read fails
		}
		return nil, nil, 0, nil
	})
	rs, _ = ReproductionProver{Exec: failExec, Run: runFn}.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, delSpec(t))
	if rs[0].Outcome != run.PinUnverifiable || !strings.Contains(rs[0].Detail, "diff") {
		t.Fatalf("a failed diff read must be unverifiable: %+v", rs[0])
	}
}

// The reproduction half must hold: a deletion whose test does not fail-before/pass-after carries the
// reproduction's own (non-proven) outcome.
func TestProveDeletionReproductionNotProven(t *testing.T) {
	// -run runs return pass-before (code 0) → survived; scope never reached.
	runFn := func(_ context.Context, _ string, argv []string) (int, string, error) {
		for _, x := range argv {
			if x == "-run" {
				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil // passes pre-fix → survived
			}
		}
		return 0, "ok\n", nil
	}
	rs, _ := ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn}.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, delSpec(t))
	if rs[0].Proven || rs[0].Outcome != run.PinSurvived {
		t.Fatalf("a deletion whose reproduction does not hold must not be proven: %+v", rs[0])
	}
}

// The whole-suite scope check must be green→green: a green→red suite blocks the deletion (over-deletion).
func TestProveDeletionScopeRegresses(t *testing.T) {
	var repro, scope int
	runFn := func(_ context.Context, _ string, argv []string) (int, string, error) {
		for _, x := range argv {
			if x == "-run" { // reproduction: fail-before then pass-after
				repro++
				if repro == 1 {
					return 1, "=== RUN   T\n--- FAIL: T\n", nil
				}
				return 0, "=== RUN   T\n--- PASS: T\nok\n", nil
			}
		}
		scope++ // whole suite: baseline green, post-fix RED → over-deletion regression
		if scope == 1 {
			return 0, "ok\n", nil
		}
		return 1, "--- FAIL: TestExisting\n", nil
	}
	rs, _ := ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn}.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, delSpec(t))
	if rs[0].Proven || rs[0].Outcome != run.PinSurvived || !strings.Contains(rs[0].Detail, "green→red") {
		t.Fatalf("a green→red scope must block the deletion: %+v", rs[0])
	}
}

// Provers routes a deletion to the reproduction prover's deletion path.
func TestProversRoutesDeletion(t *testing.T) {
	runFn, _ := delReviewerRun()
	p := Provers{Reproduction: ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn}}
	rs, err := p.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, delSpec(t))
	if err != nil || len(rs) != 1 || !rs[0].Proven {
		t.Fatalf("the composite must route + prove the deletion: %+v %v", rs, err)
	}
}

// A deletion whose reproduction cannot even run (no consent-hashed test command) is unverifiable.
func TestProveDeletionReproductionUnrunnable(t *testing.T) {
	runFn, _ := delReviewerRun()
	spec := delSpec(t)
	spec.TestCmd = nil // the engine's Reproduce then errors (misconfiguration)
	rs, _ := ReproductionProver{Exec: delExec(aRemovedDiff), Run: runFn}.ProveDeletions(context.Background(), []run.DifferentialProof{delProof("gone()")}, spec)
	if rs[0].Proven || rs[0].Outcome != run.PinUnverifiable || !strings.Contains(rs[0].Detail, "reproduction check could not run") {
		t.Fatalf("an unrunnable reproduction must be unverifiable: %+v", rs[0])
	}
}

func TestSpanIsRemoved(t *testing.T) {
	removed := []string{"gone()", "also gone"}
	if !spanIsRemoved(removed, "gone()") {
		t.Fatal("a single removed line must match")
	}
	if !spanIsRemoved(removed, "gone()\nalso gone") {
		t.Fatal("a multi-line span all removed must match")
	}
	if spanIsRemoved(removed, "gone()\nnot removed") {
		t.Fatal("a span with a line the diff did not remove must not match")
	}
	if spanIsRemoved(removed, "\n\n") {
		t.Fatal("a whitespace-only span matches nothing")
	}
	if spanIsRemoved(nil, "gone()") {
		t.Fatal("nothing removed matches nothing")
	}
}
