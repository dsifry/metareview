package judge

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

func adjInput() AdjudicateInput {
	return AdjudicateInput{Diff: "d", Candidate: run.Finding{IssueText: "c", File: "f.go"}}
}

// The vendored verifier asks "is this a real bug in the diff", which is the right question for
// the benchmark it came from and the wrong one for a merge gate. Measured on this repository's
// own branch: it rejected a whole class - a guard, exit-code row or keep-hash that could be
// deleted with the entire suite still green - as "a test-coverage gap, not incorrect behaviour".
// Every one of those was real, and unpinned invariants are exactly what a gate exists to catch.
// The addendum says so. It is metareview's, not harnesseval's, and is kept separate from the
// vendored system string so the lineage stays byte-identical.
func TestAdjudicateCarriesTheMetareviewRubric(t *testing.T) {
	system, _, err := RenderPrompt(KindAdjudicate, adjInput(), true, false, "n0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(system, SystemAdjudicate) {
		t.Fatal("the vendored system prompt must remain the prefix, unmodified")
	}
	if !strings.Contains(system, RubricAddendum) {
		t.Error("a judged (non-calibration) adjudicate call must carry the addendum")
	}
	for _, want := range []string{"delet", "test", "assert"} {
		if !strings.Contains(strings.ToLower(RubricAddendum), want) {
			t.Errorf("the addendum must speak about unpinned invariants; missing %q", want)
		}
	}
}

// --calibration is the apples-to-apples path against harnesseval, so it must send exactly what
// the reference sends: the vendored system string and nothing else.
func TestCalibrationSendsThePureReferencePrompt(t *testing.T) {
	system, _, err := RenderPrompt(KindAdjudicate, adjInput(), true, true, "n0")
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemAdjudicate {
		t.Errorf("calibration system prompt diverged from the reference:\n%q", system)
	}
}

// The addendum is about adjudicating a finding. The other kinds ask different questions and
// keep the reference prompt unchanged.
func TestOtherKindsAreUnchanged(t *testing.T) {
	m, _, err := RenderPrompt(KindMatch, MatchInput{Golden: run.Golden{Comment: "g"}, Candidate: run.Finding{IssueText: "c"}}, true, false, "n0")
	if err != nil {
		t.Fatal(err)
	}
	if m != SystemMatch {
		t.Errorf("match system prompt changed: %q", m)
	}
	s, _, err := RenderPrompt(KindStillPresent, StillPresentInput{Bug: run.Bug{Desc: "b"}, Diff: "d"}, true, false, "n0")
	if err != nil {
		t.Fatal(err)
	}
	if s != SystemStillPresent {
		t.Errorf("still-present system prompt changed: %q", s)
	}
}
