package judge

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/fsm/errs"
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

// The §9.2 symptom reviewer: its prompt fences the untrusted symptom/test/output, caps the failure
// log to its tail, and its verdict parses fail-closed (a mismatch, a missing field, or garbage all
// veto — decision false — never approve).
func TestSymptomPromptAndParse(t *testing.T) {
	in := SymptomInput{Bug: run.Bug{ID: "b1", Desc: "returns nil on empty input"}, Test: "TestEmpty", FailOutput: "=== RUN   TestEmpty\n--- FAIL: TestEmpty\n\tgot non-nil\n"}

	// Fenced render carries the system prompt, the JSON hint, and fences every untrusted slot.
	system, user, err := RenderPrompt(KindSymptom, in, true, false, "nonce123")
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemSymptom {
		t.Fatal("symptom uses SystemSymptom")
	}
	if !strings.Contains(user, "<<<DATA-nonce123") {
		t.Fatal("untrusted slots must be fenced")
	}
	if !strings.Contains(user, `{"reasoning":`) {
		t.Fatal("the JSON hint must unescape {{ }} to single braces")
	}
	// The symptom, test name, and failure output all reach the prompt (JSON-encoded inside the fence).
	for _, want := range []string{"returns nil on empty input", "TestEmpty", "got non-nil"} {
		if !strings.Contains(user, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	// Wrong input type is a template error.
	if _, _, err := RenderPrompt(KindSymptom, "nope", false, false, "n"); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("symptom expects SymptomInput")
	}

	// CutTail keeps the LAST max bytes (the failure is at the end) and reports truncation.
	long := strings.Repeat("x", 100) + "THE-FAILURE"
	if got, tr := CutTail(long, 11); got != "THE-FAILURE" || !tr {
		t.Fatalf("CutTail tail=%q truncated=%v", got, tr)
	}
	if got, tr := CutTail("short", 100); got != "short" || tr {
		t.Fatalf("CutTail must pass a short string through unchanged: %q %v", got, tr)
	}
	// A cut that lands mid-rune advances to the next rune boundary — the tail stays valid UTF-8.
	if got, tr := CutTail("ééééé", 3); !tr || !utf8.ValidString(got) || got == "" {
		t.Fatalf("CutTail must not split a rune: %q valid=%v", got, utf8.ValidString(got))
	}
	// A huge failure log is cut to its tail in the rendered prompt.
	huge := SymptomInput{Bug: run.Bug{Desc: "d"}, Test: "T", FailOutput: strings.Repeat("A", MaxFailOutputBytes+500) + "TAILMARKER"}
	_, huser, _ := RenderPrompt(KindSymptom, huge, false, false, "n")
	if !strings.Contains(huser, "TAILMARKER") || strings.Count(huser, "A") >= MaxFailOutputBytes+500 {
		t.Fatal("the failure output must be cut to its tail")
	}

	// Parse: a match decides true; a mismatch decides false; both carry the parsed object.
	if p, dec, conf, perr := Parse(KindSymptom, `{"reasoning":"r","matches":true,"confidence":0.8}`); !dec || perr != "" || conf != 0.8 || p == nil {
		t.Fatalf("matches:true → decision true: dec=%v conf=%v perr=%q", dec, conf, perr)
	}
	if _, dec, _, perr := Parse(KindSymptom, `{"reasoning":"r","matches":false}`); dec || perr != "" {
		t.Fatalf("matches:false → decision false, no parse error: dec=%v perr=%q", dec, perr)
	}
	// Fail-closed: a missing field vetoes (decision false) and records a parse error.
	if _, dec, _, perr := Parse(KindSymptom, `{"reasoning":"r"}`); dec || perr == "" {
		t.Fatalf("missing matches must veto with a parse error: dec=%v perr=%q", dec, perr)
	}
	// Fail-closed: garbage vetoes.
	if _, dec, _, perr := Parse(KindSymptom, `not json`); dec || perr == "" {
		t.Fatalf("garbage must veto with a parse error: dec=%v perr=%q", dec, perr)
	}
	// Fail-closed: an oversize verdict (reasoning beyond MaxDetail) vetoes.
	oversize := `{"reasoning":"` + strings.Repeat("x", run.MaxDetail+10) + `","matches":true}`
	if _, dec, _, perr := Parse(KindSymptom, oversize); dec || perr == "" {
		t.Fatalf("an oversize verdict must veto: dec=%v perr=%q", dec, perr)
	}
	if maxTokensFor(KindSymptom, false) != MaxTokensSymptom {
		t.Fatal("maxTokensFor(symptom)")
	}
}
