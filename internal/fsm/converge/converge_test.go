package converge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

func node(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	return &n
}

type fakeRunner struct {
	calls  []string
	stdins [][]byte
	res    CmdResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (CmdResult, error) {
	f.calls = append(f.calls, name)
	f.stdins = append(f.stdins, stdin)
	return f.res, f.err
}

// snap builds a snapshot with `found` bugs (ids b00, b01, ...), of which the first `unfixed` are
// still present and the rest are fixed. entry is the ENTERING SET — the ids that were unfixed
// when the iteration began — stated explicitly, because which of them are now fixed is the whole
// measurement and no positional rule expresses both cases. nil means no boundary has been crossed.
//
// The shape is a set, not a count, because both counts were tried and both were wrong: unfixed
// totals stall a productive loop, and fixed totals never stall a stuck one.
func snap(iter int, unfixed int, entry []string, tokens int64, found int) run.Snapshot {
	// tokens are spread over the non-Input counters so a Total() that only reads Input fails.
	s := run.Snapshot{Iteration: iter, Unfixed: unfixed, UnfixedAtEntry: entry,
		Tokens: run.TokenTotals{Output: tokens / 2, CacheRead: tokens - tokens/2 - tokens/4, Reasoning: tokens / 4}}
	for i := 0; i < found; i++ {
		id := bug(i)
		s.AllFound = append(s.AllFound, run.Bug{ID: id, Desc: "d"})
		s.Status = append(s.Status, run.BugStatus{ID: id, StillPresent: i < unfixed})
	}
	return s
}

func bug(i int) string { return fmt.Sprintf("b%02d", i) }

// bugs names ids by index, so a fixture can say exactly which bugs entered the iteration.
func bugs(idx ...int) []string {
	out := []string{}
	for _, i := range idx {
		out = append(out, bug(i))
	}
	return out
}

func TestC1AllFixed(t *testing.T) {
	if AllFixed(snap(0, 0, nil, 0, 0)) {
		t.Fatal("empty AllFound must not be fixed")
	}
	if !AllFixed(snap(0, 0, nil, 0, 1)) {
		t.Fatal("found + unfixed 0 → fixed")
	}
	if AllFixed(snap(0, 1, nil, 0, 1)) {
		t.Fatal("unfixed 1 → not fixed")
	}
}

func TestC2Atoms(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		yaml string
		s    run.Snapshot
		stop bool
		atom string
		cls  run.Outcome
	}{
		{"all_fixed-bare-true", "all_fixed", snap(0, 0, nil, 0, 2), true, "all_fixed", run.OutcomeFixed},
		{"all_fixed-map-false", "{all_fixed: true}", snap(0, 1, nil, 0, 2), false, "all_fixed", run.OutcomeFixed},
		// (iteration, unfixed, ENTERING SET, tokens, bugs known). Ids b00.. ; the first `unfixed`
		// are still present, the rest are fixed.
		{"nfp-no-boundary-yet", "no_fixation_progress", snap(1, 5, nil, 0, 5), false, "no_fixation_progress", run.OutcomeStalled},
		// Entered with b00-b02, all still unfixed. Nothing handed to this round was fixed.
		{"nfp-entering-set-untouched", "{no_fixation_progress: true}", snap(1, 5, bugs(0, 1, 2), 0, 5), true, "no_fixation_progress", run.OutcomeStalled},
		// Entered with b02-b04; b03 and b04 are now fixed. Progress.
		{"nfp-entering-set-partly-cleared", "no_fixation_progress", snap(1, 3, bugs(2, 3, 4), 0, 5), false, "no_fixation_progress", run.OutcomeStalled},
		// THE CASE THE UNFIXED-TOTAL RULE STALLED: unfixed rose 4 -> 23 because discovery found 21
		// new bugs, while entering bugs b23-b30 were cleared. Progress, and the old rule stopped it.
		{"nfp-discovery-outpaces-fixing", "no_fixation_progress", snap(1, 23, bugs(0, 23, 24), 0, 31), false, "no_fixation_progress", run.OutcomeStalled},
		// THE CASE THE FIXED-TOTAL RULE MISSED: the round cleared only bugs it discovered itself
		// (b23-b30), and every bug it entered with (b00-b02) is untouched. Stalled.
		{"nfp-only-new-bugs-fixed", "no_fixation_progress", snap(1, 23, bugs(0, 1, 2), 0, 31), true, "no_fixation_progress", run.OutcomeStalled},
		// Entering with nothing unfixed is not a stall — there was nothing to make progress on.
		{"nfp-entered-clean", "no_fixation_progress", snap(1, 0, []string{}, 0, 5), false, "no_fixation_progress", run.OutcomeStalled},
		{"max-iter-3", "{max_iterations: 5}", snap(3, 1, nil, 0, 1), false, "max_iterations", run.OutcomeOverflow},
		{"max-iter-4", "{max_iterations: 5}", snap(4, 1, nil, 0, 1), true, "max_iterations", run.OutcomeOverflow},
		{"budget-under", "{budget: {tokens: 100}}", snap(0, 1, nil, 99, 1), false, "budget", run.OutcomeOverflow},
		{"budget-at", "{budget: {tokens: 100}}", snap(0, 1, nil, 100, 1), true, "budget", run.OutcomeOverflow},
	}
	for _, c := range cases {
		p, err := Parse(node(t, c.yaml), nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		r, err := p.Evaluate(ctx, c.s)
		if err != nil || r.Stop != c.stop || r.Atom != c.atom || r.Class != c.cls || (r.Stop && r.Reason == "") || (!r.Stop && r.Reason != "") {
			t.Errorf("%s: got %+v err=%v", c.name, r, err)
		}
		if p.Name() != c.atom || p.Class() != c.cls {
			t.Errorf("%s: Name/Class", c.name)
		}
	}
}

func TestC2CmdAtom(t *testing.T) {
	ctx := context.Background()
	s := snap(2, 1, nil, 7, 1)
	s.Vars = map[string]string{"JUDGE": "secret-model"}
	s.NodeOutputs = map[string]json.RawMessage{"n@0": json.RawMessage(`{"big":true}`)}
	fr := &fakeRunner{res: CmdResult{Stdout: []byte(`{"stop": true, "reason": "plateau"}`)}}
	p, err := Parse(node(t, "{cmd: notify}"), fr)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "cmd:notify" || p.Class() != run.OutcomeCustom {
		t.Fatal("cmd atom name/class")
	}
	r, err := p.Evaluate(ctx, s)
	if err != nil || !r.Stop || r.Reason != "plateau" || r.Atom != "cmd:notify" || r.Class != run.OutcomeCustom {
		t.Fatalf("got %+v %v", r, err)
	}
	if fr.calls[0] != "notify" {
		t.Fatal("name passed")
	}
	// stdin is the redacted payload: vars hashed, never the raw value.
	var got run.Snapshot
	if err := json.Unmarshal(fr.stdins[0], &got); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("secret-model"))
	if got.Vars["JUDGE"] != "sha256:"+hex.EncodeToString(sum[:]) || strings.Contains(string(fr.stdins[0]), "secret-model") {
		t.Fatalf("payload not redacted: %s", fr.stdins[0])
	}
	if got.Iteration != 2 || got.Tokens.Total() != 7 {
		t.Fatal("payload carries the snapshot")
	}
	if strings.Contains(string(fr.stdins[0]), `"big"`) {
		t.Fatal("payload must omit node outputs")
	}
	if s.Vars["JUDGE"] != "secret-model" {
		t.Fatal("Payload must not mutate the snapshot")
	}

	// stop:false path, and a missing reason is legal
	fr.res = CmdResult{Stdout: []byte(`{"stop": false, "reason": ""}`)}
	if r, err := p.Evaluate(ctx, s); err != nil || r.Stop || r.Reason != "" {
		t.Fatalf("stop false: %+v %v", r, err)
	}
	fr.res = CmdResult{Stdout: []byte(`{"stop": true}`)}
	if r, err := p.Evaluate(ctx, s); err != nil || !r.Stop || r.Reason != "" {
		t.Fatalf("reason optional: %+v %v", r, err)
	}
	// invalid outputs
	for _, out := range []string{`{"stop": "yes"}`, `not json`, `{"stop": true, "extra": 1}`, ``, `{"stop": true, "reason": 5}`} {
		fr.res = CmdResult{Stdout: []byte(out)}
		_, err := p.Evaluate(ctx, s)
		if !errs.Is(err, CodeCmdOutputInvalid) {
			t.Errorf("%q: %v", out, err)
		}
	}
	fr.res = CmdResult{Stdout: []byte(`{"stop": true, "reason": "x"}`), ExitCode: 3}
	if _, err := p.Evaluate(ctx, s); !errs.Is(err, CodeCmdFailed) || errs.As(err).Field("exit") != "3" {
		t.Fatalf("exit: %v", err)
	}
	boom := errors.New("boom")
	fr.err = boom
	if _, err := p.Evaluate(ctx, s); !errors.Is(err, boom) {
		t.Fatalf("runner error must propagate: %v", err)
	}
}

func TestC3Compose(t *testing.T) {
	ctx := context.Background()
	// 5 known, 5 unfixed -> 0 fixed, no more than the previous 0: nfp fires. max_iterations 5
	// fires at iteration 4. budget does not.
	fired := snap(4, 5, bugs(0, 1, 2), 0, 5)
	quiet := snap(0, 1, nil, 0, 1)

	anyP, err := Parse(node(t, "any: [{budget: {tokens: 1000}}, no_fixation_progress, {max_iterations: 5}]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if anyP.Name() != "any(budget+no_fixation_progress+max_iterations)" || anyP.Class() != run.OutcomeOverflow {
		t.Fatalf("any name/class: %s %s", anyP.Name(), anyP.Class())
	}
	r, _ := anyP.Evaluate(ctx, fired)
	if !r.Stop || r.Atom != "no_fixation_progress" || r.Class != run.OutcomeStalled {
		t.Fatalf("any must report the first firing child: %+v", r)
	}
	r, _ = anyP.Evaluate(ctx, quiet)
	if r.Stop || r.Atom != anyP.Name() || r.Class != run.OutcomeOverflow || r.Reason != "" {
		t.Fatalf("any quiet: %+v", r)
	}

	allP, err := Parse(node(t, "all: [no_fixation_progress, {max_iterations: 5}]"), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _ = allP.Evaluate(ctx, fired)
	if !r.Stop || r.Atom != "no_fixation_progress+max_iterations" || r.Class != run.OutcomeStalled || !strings.Contains(r.Reason, "; ") {
		t.Fatalf("all fired: %+v", r)
	}
	// 5 known, 1 unfixed -> 4 fixed, up from 2: nfp quiet. max fires, so `all` is only partial.
	r, _ = allP.Evaluate(ctx, snap(4, 1, bugs(1, 2, 3), 0, 5))
	if r.Stop || r.Atom != allP.Name() || r.Reason != "" {
		t.Fatalf("all partial: %+v", r)
	}

	notP, err := Parse(node(t, "not: {budget: {tokens: 10}}"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if notP.Name() != "not(budget)" || notP.Class() != run.OutcomeCustom {
		t.Fatal("not name/class: negation is always custom")
	}
	r, _ = notP.Evaluate(ctx, quiet)
	if !r.Stop || r.Atom != "not(budget)" || r.Class != run.OutcomeCustom || r.Reason == "" {
		t.Fatalf("not inverts: %+v", r)
	}
	r, _ = notP.Evaluate(ctx, snap(0, 1, nil, 10, 1))
	if r.Stop || r.Reason != "" {
		t.Fatalf("not inverts (2): %+v", r)
	}

	// Error abort: the failing cmd atom stops evaluation; later atoms are not visited.
	boom := errors.New("boom")
	fr := &fakeRunner{err: boom}
	for _, src := range []string{"any: [{cmd: c}, {cmd: d}]", "all: [{cmd: c}, {cmd: d}]", "not: {cmd: c}"} {
		fr.calls = nil
		p, err := Parse(node(t, src), fr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Evaluate(ctx, quiet); !errors.Is(err, boom) {
			t.Fatalf("%s: %v", src, err)
		}
		if len(fr.calls) != 1 {
			t.Fatalf("%s: evaluated %d atoms after the error", src, len(fr.calls))
		}
	}
}

func TestC4ValidateAndParseErrors(t *testing.T) {
	bad := []struct{ name, yaml string }{
		{"empty", ""},
		{"unknown-atom", "{frobnicate: 1}"},
		{"unknown-scalar", "frobnicate"},
		{"two-keys", "{all_fixed: true, max_iterations: 2}"},
		{"list-not-atom", "[1, 2]"},
		{"all_fixed-false", "{all_fixed: false}"},
		{"nfp-string", "{no_fixation_progress: yes please}"},
		{"max-iter-zero", "{max_iterations: 0}"},
		{"max-iter-string", "{max_iterations: five}"},
		{"budget-no-tokens", "{budget: {}}"},
		{"budget-extra", "{budget: {tokens: 5, dollars: 1}}"},
		{"budget-scalar", "{budget: 5}"},
		{"cmd-empty", "{cmd: ''}"},
		{"cmd-list", "{cmd: [a, b]}"},
		{"cmd-unknown", "{cmd: nope}"},
		{"any-empty", "any: []"},
		{"any-scalar", "any: x"},
		{"all-bad-child", "all: [{max_iterations: -1}]"},
		{"not-bad-child", "not: {budget: 0}"},
		{"all_fixed-under-not", "not: all_fixed"},
		{"all_fixed-under-all", "all: [{not: all_fixed}, {max_iterations: 5}]"},
		{"all_fixed-map-under-all", "all: [{all_fixed: true}, {max_iterations: 5}]"},
		{"all_fixed-nested-any", "any: [{any: [all_fixed]}]"},
		{"too-deep", "not: {not: {not: {not: {not: {max_iterations: 1}}}}}"},
		{"too-wide", "any: [" + strings.Repeat("{max_iterations: 1}, ", MaxAtoms) + "{max_iterations: 1}]"},
	}
	if err := Validate(node(t, "any: ["+strings.Repeat("{max_iterations: 1}, ", MaxAtoms-1)+"{max_iterations: 1}]"), nil); err != nil {
		t.Fatalf("at MaxAtoms: %v", err)
	}
	if err := Validate(node(t, "not: {not: {not: {not: {max_iterations: 1}}}}"), nil); err != nil {
		t.Fatalf("at MaxDepth: %v", err)
	}
	// all_fixed is legal at the top level and directly under a top-level any
	for _, ok := range []string{"all_fixed", "{all_fixed: true}", "any: [all_fixed, {max_iterations: 5}]"} {
		if err := Validate(node(t, ok), nil); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, c := range bad {
		err := Validate(node(t, c.yaml), []string{"notify"})
		if !errs.Is(err, CodeBadConvergence) || errs.As(err).Field("detail") == "" {
			t.Errorf("%s: want ERR_BAD_CONVERGENCE with detail, got %v", c.name, err)
		}
	}
	if err := Validate(node(t, "{cmd: notify}"), []string{"notify"}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(nil, nil); !errs.Is(err, CodeBadConvergence) {
		t.Fatal("nil node")
	}
	// A multi-document node is rejected too.
	multi := &yaml.Node{Kind: yaml.DocumentNode}
	if err := Validate(multi, nil); !errs.Is(err, CodeBadConvergence) {
		t.Fatal("empty document")
	}
	if p := MustParse(node(t, "{cmd: anything}"), &fakeRunner{}); p == nil || p.Name() != "cmd:anything" {
		t.Fatal("MustParse binds validated input")
	}
	// Parse (cmdNames nil) accepts any cmd name — workflow.Parse validated it earlier.
	if _, err := Parse(node(t, "{cmd: anything}"), &fakeRunner{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(node(t, "{cmd: [x]}"), nil); !errs.Is(err, CodeBadConvergence) {
		t.Fatal("non-string cmd via Parse")
	}
}

func TestDescribe(t *testing.T) {
	st, err := Describe(node(t, "any: [no_fixation_progress, {cmd: notify}, {max_iterations: 5}, {budget: {tokens: 4000000}}, {all: [no_fixation_progress, {not: {cmd: chk}}]}]"), []string{"notify", "chk"})
	if err != nil || st.Atoms != 6 || st.Depth != 3 || len(st.Cmds) != 2 || st.Cmds[0] != "notify" || st.Cmds[1] != "chk" {
		t.Fatalf("describe: %+v %v", st, err)
	}
	if st, err := Describe(node(t, "no_fixation_progress"), nil); err != nil || st.Atoms != 1 || st.Depth != 0 || len(st.Cmds) != 0 {
		t.Fatalf("scalar: %+v %v", st, err)
	}
	if _, err := Describe(node(t, "any: [{cmd: ghost}]"), []string{"notify"}); err == nil {
		t.Fatal("undeclared cmd must fail")
	}
	if _, err := Describe(node(t, "bogus: 1"), nil); err == nil {
		t.Fatal("invalid tree")
	}
}

// A looping workflow must carry something that terminates. no_fixation_progress is a STALL
// detector, not a bound: an agent may fix one bug it entered with and discover five, forever, and
// that is progress every round. The old predicate hid this by being too strict — it required the
// unfixed total to strictly decrease, and that total is bounded below by zero, so it terminated
// as a side effect. Replacing it with a correct stall detector removed a guarantee nobody had
// written down, so it is written down here.
func TestValidateBoundedRequiresATerminatingPredicate(t *testing.T) {
	for name, src := range map[string]string{
		"only nfp":            "no_fixation_progress",
		"only all_fixed":      "all_fixed",
		"nfp and all_fixed":   "any: [no_fixation_progress, all_fixed]",
		"nfp under a nesting": "any: [{not: {no_fixation_progress: true}}, no_fixation_progress]",
		// `all` stops only when EVERY child fires, so two bounds under it guarantee nothing: the
		// run continues until they fire together, which nothing arranges. This tree names two
		// bounds and is unbounded. It was accepted before, which made the whole check a
		// formality — an author could satisfy it with a bound that could never stop the run.
		"two bounds under all": "all: [{max_iterations: 3}, {budget: {tokens: 100}}]",
		"a bound under all":    "any: [no_fixation_progress, {all: [{max_iterations: 9}, {cmd: chk}]}]",
		"a negated bound":      "any: [no_fixation_progress, {not: {max_iterations: 3}}]",
	} {
		if err := ValidateBounded(node(t, src), []string{"chk"}, true); err == nil {
			t.Errorf("%s: a looping workflow with no bound must be refused", name)
		}
		// ...and the same tree is fine for a workflow that does not loop.
		if err := ValidateBounded(node(t, src), []string{"chk"}, false); err != nil {
			t.Errorf("%s: a non-looping workflow needs no bound: %v", name, err)
		}
	}
	for name, src := range map[string]string{
		"max_iterations": "any: [no_fixation_progress, {max_iterations: 5}]",
		"budget":         "any: [no_fixation_progress, {budget: {tokens: 100}}]",
		"cmd":            "any: [no_fixation_progress, {cmd: chk}]",
		"bound alone":    "{max_iterations: 3}",
		// Nested `any` preserves the guarantee — the bound alone still stops the run.
		"nested under any": "any: [no_fixation_progress, {any: [{cmd: chk}, {max_iterations: 9}]}]",
		// A bound beside an `all` that buries one: the reachable bound is what counts.
		"one reachable": "any: [{max_iterations: 4}, {all: [{cmd: chk}, no_fixation_progress]}]",
	} {
		if err := ValidateBounded(node(t, src), []string{"chk"}, true); err != nil {
			t.Errorf("%s: must be accepted as bounded: %v", name, err)
		}
	}
	// An invalid tree still fails for its own reason, not for missing a bound.
	if err := ValidateBounded(node(t, "frobnicate"), nil, true); err == nil ||
		strings.Contains(err.Error(), "terminating predicate") {
		t.Errorf("a malformed tree must fail on its own error: %v", err)
	}
}
