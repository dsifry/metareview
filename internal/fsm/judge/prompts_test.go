package judge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// The escalated call is the only one that carries a sandbox, and the addendum is what tells the
// judge the tree exists. Asserted here directly as well as end to end in kind, so the branch is
// covered where it lives.
func TestAdjudicatePromptCarriesTheSandboxAddendumOnlyWhenGivenATree(t *testing.T) {
	in := AdjudicateInput{Diff: "diff --git a/x.go b/x.go\n", Candidate: run.Finding{IssueText: "c"}}
	system, _, err := RenderPrompt(KindAdjudicate, in, true, false, "n0")
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if strings.Contains(system, "head/") {
		t.Error("a call with no sandbox must not describe one")
	}

	in.Sandbox = true
	withTree, _, err := RenderPrompt(KindAdjudicate, in, true, false, "n0")
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	for _, want := range []string{"head/", "base/"} {
		if !strings.Contains(withTree, want) {
			t.Errorf("an escalated call must name %q so the judge knows to read the tree", want)
		}
	}
	if len(withTree) <= len(system) {
		t.Error("the addendum did not lengthen the system message")
	}
}

// The input hash is what makes an escalated call replayable, so it must not move when nothing
// about the question did. Carrying the sandbox PATH put a per-invocation temp directory inside
// the hashed input, giving every escalated call a fresh input_hash even with identical evidence
// and an identical prompt.
func TestSandboxFlagDoesNotDestabiliseTheInputHash(t *testing.T) {
	in := AdjudicateInput{Diff: "diff --git a/x.go b/x.go\n", Candidate: run.Finding{IssueText: "c"}, Sandbox: true}
	first, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	// The same question, escalated again in a different run: a new tree, a new temp directory.
	second, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("the hashed input is not stable across runs:\n%s\n%s", first, second)
	}
	if strings.Contains(string(first), "/tmp") || strings.Contains(string(first), "mrv-evidence") {
		t.Errorf("a per-invocation path leaked into the hashed input: %s", first)
	}
}
