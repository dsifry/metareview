package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/workflows"
)

func kinds() map[string]KindInfo {
	lenses := func(p map[string]any) error {
		if v, ok := p["lenses"]; ok {
			n, isInt := v.(int)
			if !isInt || n < 1 || n > 8 {
				return errors.New("lenses must be 1..8")
			}
		}
		return nil
	}
	return map[string]KindInfo{
		"review-lenses":         {DefaultExec: "subagent", AllowedExec: []string{"inline", "subagent"}, ValidateParams: lenses},
		"match-then-adjudicate": {DefaultExec: "fork", AllowedExec: []string{"fork"}},
		"agent-edit":            {DefaultExec: "inline", AllowedExec: []string{"inline", "subagent"}},
		"still-present":         {DefaultExec: "fork", AllowedExec: []string{"fork"}},
		"cmd":                   {DefaultExec: "fork", AllowedExec: []string{"fork"}},
		"prove": {DefaultExec: "fork", AllowedExec: []string{"fork"}, ValidateParams: func(p map[string]any) error {
			for k := range p {
				if k != "test_cmd" {
					return errors.New("unknown param " + k)
				}
			}
			return nil
		}},
	}
}

const example = `workflow: example
version: 1
vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }
states: [discover, adjudicate, fix, verify, done, failed]
cmds:
  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }
nodes:
  discover:   { kind: review-lenses, exec: subagent, model: $REVIEWER, lenses: 8 }
  adjudicate: { kind: match-then-adjudicate, exec: fork, model: $JUDGE, effort: $JUDGE_EFFORT }
  fix:        { kind: agent-edit }
  verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }
transitions:
  - { from: discover, to: done, gate: findings_empty, outcome: clean }
  - { from: discover, to: adjudicate, gate: findings_nonempty }
  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }
  - { from: adjudicate, to: fix, gate: confirmed_nonempty }
  - { from: fix, to: verify, gate: commit_exists }
  - { from: verify, to: done, gate: all_fixed, outcome: fixed }
  - { from: verify, to: discover, gate: bugs_remain, loop: true }
convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }
repo_mode: advisory
on_overflow: notify
`

func mustParse(t *testing.T, src string) *Workflow {
	t.Helper()
	w, err := Parse([]byte(src), Options{Kinds: kinds()})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return w
}

func reasonOf(t *testing.T, src string) (string, string) {
	t.Helper()
	_, err := Parse([]byte(src), Options{Kinds: kinds()})
	if !errs.Is(err, CodeWorkflowInvalid) {
		t.Fatalf("expected ERR_WORKFLOW_INVALID, got %v", err)
	}
	e := errs.As(err)
	return e.Field("reason"), e.Field("at")
}

func TestW1Shipped(t *testing.T) {
	for _, name := range workflows.Names() {
		raw, err := workflows.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		w := mustParse(t, string(raw))
		if w.Name != name || len(w.Warnings) != 0 {
			t.Fatalf("%s: name %q warnings %v", name, w.Name, w.Warnings)
		}
		sum := sha256.Sum256(raw)
		if w.Hash != hex.EncodeToString(sum[:]) {
			t.Fatal("hash is sha256 of the raw bytes")
		}
	}
	sdlc := mustParse(t, string(must(workflows.Read("sdlc-loop"))))
	if len(sdlc.Transitions) != 7 || sdlc.LoopTransition() == nil || sdlc.LoopTransition().From != "verify" {
		t.Fatalf("sdlc transitions: %+v", sdlc.Transitions)
	}
	if tt := sdlc.TerminalFor("verify"); tt == nil || tt.To != "done" || tt.Gate != "all_fixed" || tt.Outcome != run.OutcomeFixed {
		t.Fatalf("TerminalFor: %+v", tt)
	}
	if sdlc.TerminalFor("discover") != nil {
		t.Fatal("TerminalFor is nil off the loop state")
	}
	if strings.Join(sdlc.Refs["adjudicate"], ",") != "JUDGE,JUDGE_EFFORT" || strings.Join(sdlc.Refs["discover"], ",") != "REVIEWER,REV_EFFORT" {
		t.Fatalf("refs: %v", sdlc.Refs)
	}
	if sdlc.Nodes["fix"].Exec != "inline" || sdlc.Nodes["fix"].Params["x"] != nil {
		t.Fatal("fix node")
	}
	if !sdlc.IsTerminal("done") || !sdlc.IsTerminal("failed") || sdlc.IsTerminal("verify") {
		t.Fatal("IsTerminal")
	}
	// sdlc-loop-proved inserts a prove node gated by pins_proven between fix and verify.
	proved := mustParse(t, string(must(workflows.Read("sdlc-loop-proved"))))
	if len(proved.Transitions) != 8 || proved.Nodes["prove"].Kind != "prove" || proved.Nodes["prove"].Params["test_cmd"] != "test" {
		t.Fatalf("sdlc-loop-proved shape: %+v", proved.Nodes["prove"])
	}
	var fixToProve, proveToVerify bool
	for _, tr := range proved.Transitions {
		if tr.From == "fix" && tr.To == "prove" && tr.Gate == "commit_exists" {
			fixToProve = true
		}
		if tr.From == "prove" && tr.To == "verify" && tr.Gate == "pins_proven" {
			proveToVerify = true
		}
	}
	if !fixToProve || !proveToVerify {
		t.Fatalf("sdlc-loop-proved must route fix→prove[commit_exists]→verify[pins_proven]: %+v", proved.Transitions)
	}
	rl := mustParse(t, string(must(workflows.Read("review-loop"))))
	if rl.LoopTransition() != nil || rl.Convergence != nil || len(rl.Outgoing("adjudicate")) != 2 {
		t.Fatal("review-loop shape")
	}
	// The built-in loops must NOT hard-code a lens count: the discover node relies on the
	// review-lenses default (len(kind.Lenses)) so the full lens set — and every lens added later —
	// runs without editing the YAML. A stray "lenses: 8" here silently drops the newest lens, which
	// is exactly the drift the lens-set machinery exists to prevent.
	if _, capped := sdlc.Nodes["discover"].Params["lenses"]; capped {
		t.Fatal("sdlc-loop discover hard-codes a lens count; it must default to the full set")
	}
	if _, capped := rl.Nodes["discover"].Params["lenses"]; capped {
		t.Fatal("review-loop discover hard-codes a lens count; it must default to the full set")
	}

	ex := mustParse(t, example)
	if ex.Nodes["fix"].Exec != "inline" || ex.Nodes["verify"].Exec != "fork" {
		t.Fatal("exec defaulted from the kind")
	}
	c := ex.Cmds["notify"]
	if c == nil || c.Timeout != 30*time.Second || strings.Join(c.Env, ",") != "SLACK_WEBHOOK" || strings.Join(c.Argv, " ") != "bash ./scripts/notify.sh --model $JUDGE" {
		t.Fatalf("cmd decl: %+v", c)
	}
	if strings.Join(ex.CmdRefs["notify"], ",") != "JUDGE" || ex.OnOverflow != "notify" || ex.RepoMode != "advisory" {
		t.Fatal("cmd refs / on_overflow")
	}
	if ex.Nodes["discover"].Params["lenses"] != 8 {
		t.Fatal("params kept")
	}
	// default timeout and default repo_mode
	w := mustParse(t, strings.Replace(strings.Replace(example, ", timeout: 30", "", 1), "repo_mode: advisory\n", "", 1))
	if w.Cmds["notify"].Timeout != DefaultTimeout || w.RepoMode != "advisory" {
		t.Fatal("defaults")
	}
	// mapping form, both arrows, *→failed ignored, order preserved
	m := mustParse(t, mappingDoc)
	if len(m.Transitions) != 4 || m.Transitions[1].To != "adjudicate" || m.Transitions[3].Outcome != run.OutcomeReviewed {
		t.Fatalf("mapping form: %+v", m.Transitions)
	}
	if strings.Join(m.VarsReferencedBy("adjudicate"), ",") != "JUDGE" || len(m.VarsReferencedBy("discover")) != 0 {
		t.Fatal("VarsReferencedBy")
	}
}

// scalarTransitions replaces the whole transitions block with a scalar.
func scalarTransitions() string {
	i := strings.Index(example, "transitions:")
	j := strings.Index(example, "convergence:")
	return example[:i] + "transitions: nope\n" + example[j:]
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// edit applies one textual edit to the example.
func edit(old, new string) string {
	if !strings.Contains(example, old) {
		panic("edit target missing: " + old)
	}
	return strings.Replace(example, old, new, 1)
}

func TestW2Reasons(t *testing.T) {
	cases := []struct {
		name, reason, at string
		src              string
	}{
		{"unknown-top-key", "unknown_key", "document", example + "extra: 1\n"},
		{"bad-yaml-malformed", "bad_yaml", "document", "workflow: [\n"},
		{"bad-yaml-dup-key", "bad_yaml", "document", example + "workflow: again\n"},
		{"bad-yaml-scalar-root", "bad_yaml", "document", "just a string\n"},
		{"bad-version-string", "bad_yaml", "document", edit("version: 1", "version: one")},
		{"transitions-scalar", "unknown_key", "transitions", scalarTransitions()},
		{"node-kind-not-string", "unknown_key", "nodes.fix.kind", edit("fix:        { kind: agent-edit }", "fix:        { kind: [agent-edit] }")},
		{"transition-unknown-field", "unknown_key", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: clean, extra: 1 }")},
		{"missing-name", "missing_name", "workflow", edit("workflow: example", "workflow: ''")},
		{"bad-version", "bad_version", "version", edit("version: 1", "version: 2")},
		{"no-initial", "no_initial", "states", edit("states: [discover, adjudicate, fix, verify, done, failed]", "states: []")},
		{"bad-state-charset", "bad_state", "states.Discover", edit("states: [discover,", "states: [Discover, discover,")},
		{"bad-state-judge", "bad_state", "states.judge", edit("states: [discover,", "states: [judge, discover,")},
		{"bad-state-dup", "bad_state", "states.discover", edit("states: [discover,", "states: [discover, discover,")},
		{"bad-var-name", "bad_var", "vars.judge", edit("REVIEWER: {default: claude-opus-5}", "judge: {default: x}")},
		{"bad-var-required-default", "bad_var", "vars.JUDGE", edit("JUDGE: {required: true}", "JUDGE: {required: true, default: x}")},
		{"bad-cmd-argv-empty", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: []")},
		{"bad-cmd-argv-nonstring", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: [bash, 3]")},
		{"bad-cmd-argv-emptyelem", "bad_cmd", "cmds.notify", edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: [bash, '']")},
		{"bad-cmd-name", "bad_cmd", "cmds.Notify", edit("  notify: { argv:", "  Notify: { argv:")},
		{"bad-cmd-timeout-high", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 3601")},
		{"bad-cmd-timeout-zero", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 0")},
		{"bad-cmd-timeout-string", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: soon")},
		{"bad-cmd-unknown-field", "bad_cmd", "cmds.notify", edit("timeout: 30", "timeout: 30, shell: true")},
		{"bad-cmd-not-mapping", "bad_cmd", "cmds", edit("cmds:\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "cmds: [notify]")},
		{"bad-env-charset", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [slack]")},
		{"bad-env-dup", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [SLACK_WEBHOOK, SLACK_WEBHOOK]")},
		{"bad-env-path", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [PATH]")},
		{"bad-env-mrv", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [MRV_X]")},
		{"bad-env-ld", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [LD_PRELOAD]")},
		{"bad-env-count", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15, A16, A17]")},
		{"duplicate-cmd", "duplicate_cmd", "cmds.notify", edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [a] }\n  notify: { argv: [b] }")},
		{"unknown-state-node", "unknown_state", "nodes.zzz", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  zzz: { kind: agent-edit }")},
		{"unknown-state-transition", "unknown_state", "transitions[0]", edit("{ from: discover, to: done, gate: findings_empty, outcome: clean }", "{ from: discover, to: nowhere, gate: findings_empty, outcome: clean }")},
		{"failed-in-transition", "failed_reserved", "transitions[0]", edit("{ from: discover, to: done, gate: findings_empty, outcome: clean }", "{ from: discover, to: failed, gate: findings_empty, outcome: failed }")},
		{"failed-undeclared", "failed_reserved", "states", edit("done, failed]", "done]")},
		{"failed-with-node", "failed_reserved", "states", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  failed: { kind: agent-edit }")},
		{"node-without-kind", "node_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { exec: inline }")},
		{"unknown-kind", "unknown_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: wizard }")},
		{"unknown-exec", "unknown_exec", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: agent-edit, exec: remote }")},
		{"exec-kind-mismatch", "exec_kind_mismatch", "nodes.verify", edit("verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }", "verify:     { kind: still-present, exec: inline, model: $JUDGE, effort: $JUDGE_EFFORT }")},
		{"bad-params", "bad_params", "nodes.discover", edit("lenses: 8 }", "lenses: 9 }")},
		{"cmd-on-non-cmd-kind", "cmd_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: agent-edit, cmd: notify }")},
		{"cmd-kind-without-cmd", "cmd_without_kind", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: cmd }")},
		{"unknown-cmd-node", "unknown_cmd", "nodes.fix", edit("fix:        { kind: agent-edit }", "fix:        { kind: cmd, cmd: nope }")},
		{"unknown-cmd-on-overflow", "unknown_cmd", "on_overflow", edit("on_overflow: notify", "on_overflow: nope")},
		{"unknown-cmd-atom", "bad_convergence", "convergence", edit("{cmd: notify}", "{cmd: nope}")},
		{"terminal-with-node", "terminal_with_node", "nodes.done", edit("  fix:        { kind: agent-edit }", "  fix:        { kind: agent-edit }\n  done: { kind: agent-edit }")},
		{"unknown-gate", "unknown_gate", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: vibes, outcome: clean }")},
		{"duplicate-transition", "duplicate_transition", "transitions[3]", edit("{ from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }", "{ from: adjudicate, to: fix, gate: findings_nonempty }\n  - { from: discover, to: fix, gate: findings_nonempty }")},
		{"terminal-without-outcome", "terminal_without_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty }")},
		{"outcome-on-nonterminal", "outcome_on_nonterminal", "transitions[1]", edit("{ from: discover, to: adjudicate, gate: findings_nonempty }", "{ from: discover, to: adjudicate, gate: findings_nonempty, outcome: clean }")},
		{"bad-outcome", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: great }")},
		{"bad-outcome-failed", "bad_outcome", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: failed }")},
		{"initial-terminal", "initial_terminal", "states.discover", strings.Replace(strings.Replace(example, "  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "", 1), "  - { from: discover, to: adjudicate, gate: findings_nonempty }\n", "  - { from: fix, to: adjudicate, gate: findings_nonempty }\n", 1)},
		{"bad-env-shellopts", "bad_env", "cmds.notify", edit("env: [SLACK_WEBHOOK]", "env: [SHELLOPTS]")},
		{"unreachable-state", "unreachable_state", "states.fix", edit("  - { from: adjudicate, to: fix, gate: confirmed_nonempty }\n", "")},
		{"loop-count", "loop_count", "transitions", edit("{ from: adjudicate, to: fix, gate: confirmed_nonempty }", "{ from: adjudicate, to: fix, gate: confirmed_nonempty, loop: true }")},
		{"loop-not-cycle", "loop_not_cycle", "transitions", strings.Replace(edit("{ from: verify, to: discover, gate: bugs_remain, loop: true }", "{ from: verify, to: side, gate: bugs_remain, loop: true }\n  - { from: side, to: done, gate: findings_empty, outcome: clean }"), "verify, done, failed]", "verify, side, done, failed]", 1)},
		{"loop-terminal-zero", "loop_terminal", "transitions", edit("{ from: verify, to: done, gate: all_fixed, outcome: fixed }", "{ from: verify, to: fix, gate: all_fixed }")},
		{"loop-terminal-two", "loop_terminal", "transitions", edit("{ from: verify, to: done, gate: all_fixed, outcome: fixed }", "{ from: verify, to: done, gate: all_fixed, outcome: fixed }\n  - { from: verify, to: done, gate: findings_empty, outcome: clean }")},
		{"missing-convergence", "missing_convergence", "convergence", edit("convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }\n", "")},
		{"cycle-without-loop", "cycle_without_loop", "transitions", edit("{ from: fix, to: verify, gate: commit_exists }", "{ from: fix, to: verify, gate: commit_exists }\n  - { from: fix, to: adjudicate, gate: findings_nonempty }")},
		{"bad-convergence", "bad_convergence", "convergence", edit("{max_iterations: 5}", "{max_iterations: -5}")},
		// A LOOPING workflow whose only predicates are no_fixation_progress and all_fixed has
		// nothing that terminates. nfp is a stall detector — an agent may fix one bug it entered
		// with and discover five, forever, which is progress every round — and all_fixed stops
		// only if everything gets fixed. The old nfp hid this by requiring the unfixed total to
		// strictly decrease, terminating as a side effect of being too strict. This case pins
		// that Parse calls ValidateBounded, which testing the validator alone does not.
		{"unbounded-loop", "bad_convergence", "convergence",
			edit("convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }",
				"convergence: { any: [ no_fixation_progress, all_fixed ] }")},
		{"bad-repo-mode", "bad_repo_mode", "repo_mode", edit("repo_mode: advisory", "repo_mode: strict")},
		{"unknown-var-node", "unknown_var", "nodes.discover", edit("model: $REVIEWER, lenses: 8", "model: $REVIEWERX, lenses: 8")},
		{"unknown-var-cmd", "unknown_var", "cmds.notify", edit("--model, $JUDGE]", "--model, $NOPE]")},
		{"unknown-var-param", "unknown_var", "nodes.discover", edit("lenses: 8 }", "lenses: 8, tags: [$NOPE] }")},
	}
	for _, c := range cases {
		reason, at := reasonOf(t, c.src)
		if reason != c.reason || at != c.at {
			t.Errorf("%s: got %s@%s want %s@%s", c.name, reason, at, c.reason, c.at)
		}
	}
	// missing kinds / over-cap vars, cmds, argv
	if _, err := Parse([]byte(example), Options{}); errs.As(err).Field("reason") != "missing_kinds" {
		t.Fatal("missing_kinds")
	}
	var vars []string
	for i := 0; i <= run.MaxVars; i++ {
		vars = append(vars, "V"+strings.Repeat("A", i+1)+": {default: x}")
	}
	if r, _ := reasonOf(t, edit("vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }", "vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5}, "+strings.Join(vars, ", ")+" }")); r != "bad_var" {
		t.Errorf("MaxVars: %s", r)
	}
	var cmds []string
	for i := 0; i <= run.MaxAllowedCmds; i++ {
		cmds = append(cmds, "  c"+strings.Repeat("x", i)+": { argv: [a] }")
	}
	if r, _ := reasonOf(t, edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [bash] }\n"+strings.Join(cmds, "\n"))); r != "bad_cmd" {
		t.Errorf("MaxAllowedCmds: %s", r)
	}
	if r, _ := reasonOf(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: ["+strings.Repeat("a, ", run.MaxArgv)+"a]")); r != "bad_cmd" {
		t.Errorf("MaxArgv: %s", r)
	}
	// mapping-form errors: key without an arrow, unknown field, and a transition error inside the mapping form
	for name, rep := range map[string][2]string{
		"no-arrow":      {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover": { gate: findings_empty, outcome: clean }`},
		"unknown-field": {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": { gate: findings_empty, outcome: clean, extra: 1 }`},
		"scalar-value":  {`"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": clean`},
	} {
		if r, at := reasonOf(t, strings.Replace(mappingDoc, rep[0], rep[1], 1)); r != "unknown_key" || !strings.HasPrefix(at, "transitions.") {
			t.Errorf("%s: %s@%s", name, r, at)
		}
	}
	if r, at := reasonOf(t, strings.Replace(mappingDoc, `"discover→done": { gate: findings_empty, outcome: clean }`, `"discover→done": { gate: vibes, outcome: clean }`, 1)); r != "unknown_gate" || at != "transitions.discover→done" {
		t.Errorf("mapping transition error: %s@%s", r, at)
	}
	if r, _ := reasonOf(t, edit("timeout: 30", "timeout: 30, shell: true")); r != "bad_cmd" {
		t.Errorf("cmd unknown field: %s", r)
	}
}

const mappingDoc = `workflow: rl
version: 1
vars: { JUDGE: {required: true} }
states: [discover, adjudicate, done, failed]
nodes:
  discover:   { kind: review-lenses }
  adjudicate: { kind: match-then-adjudicate, model: $JUDGE }
transitions:
  "discover→done": { gate: findings_empty, outcome: clean }
  "discover -> adjudicate": { gate: findings_nonempty }
  "adjudicate→done": { gate: confirmed_empty, outcome: clean }
  "adjudicate->done ": { gate: confirmed_nonempty, outcome: reviewed }
  "*→failed": { on: gate_error }
`

func TestW2ReservedEnvAndBoundaries(t *testing.T) {
	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "BASH_ENV", "ENV", "PYTHONPATH", "PYTHONSTARTUP", "PYTHONHOME", "NODE_OPTIONS", "NODE_PATH", "PERL5OPT", "PERL5LIB", "RUBYOPT", "RUBYLIB", "JAVA_TOOL_OPTIONS", "SHELLOPTS", "PS4", "IFS", "CDPATH", "GLOBIGNORE", "PROMPT_COMMAND", "MRV_RUN_ID", "MRV_X", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "GIT_DIR", "GIT_CONFIG_COUNT"} {
		if r, _ := reasonOf(t, edit("env: [SLACK_WEBHOOK]", "env: ["+name+"]")); r != "bad_env" {
			t.Errorf("reserved %s: %s", name, r)
		}
	}
	// acceptance boundaries
	for _, ok := range []string{edit("timeout: 30", "timeout: 1"), edit("timeout: 30", "timeout: 3600"), renameState(strings.Repeat("s", 32)), edit("env: [SLACK_WEBHOOK]", "env: [A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15, A16]")} {
		w := mustParse(t, ok)
		_ = w
	}
	if w := mustParse(t, edit("timeout: 30", "timeout: 3600")); w.Cmds["notify"].Timeout != 3600*time.Second {
		t.Fatal("3600 accepted")
	}
	if r, _ := reasonOf(t, renameState(strings.Repeat("s", 33))); r != "bad_state" {
		t.Fatal("33-char state refused")
	}
	var vars []string
	for i := 0; i < run.MaxVars-3; i++ {
		vars = append(vars, "V"+strings.Repeat("A", i+1)+": {default: x}")
	}
	mustParse(t, edit("vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }", "vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5}, "+strings.Join(vars, ", ")+" }"))
	var cmds []string
	for i := 0; i < run.MaxAllowedCmds-1; i++ {
		cmds = append(cmds, "  c"+strings.Repeat("x", i)+": { argv: [a] }")
	}
	mustParse(t, edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [bash] }\n"+strings.Join(cmds, "\n")))
	mustParse(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: ["+strings.Repeat("a, ", run.MaxArgv-1)+"a]"))
	// caller beats Default; $1 / ${X} left literal; nested maps not walked
	w := mustParse(t, edit("lenses: 8 }", "lenses: 8, note: '$1 ${JUDGE} $', deep: {model: $JUDGE} }"))
	r, eff, err := w.Resolve(map[string]string{"JUDGE": "j", "JUDGE_EFFORT": "e", "REVIEWER": "override"}, false)
	if err != nil || eff["REVIEWER"] != "override" || r.Nodes["discover"].Model != "override" {
		t.Fatalf("caller beats default: %v %v", err, eff)
	}
	if r.Nodes["discover"].Params["note"] != "$1 ${JUDGE} $" || r.Nodes["discover"].Params["deep"].(map[string]any)["model"] != "$JUDGE" {
		t.Fatalf("literal/nested: %v", r.Nodes["discover"].Params)
	}
}

// renameState renames the adjudicate state (not the kind) in the example.
func renameState(name string) string {
	s := strings.ReplaceAll(example, "adjudicate", name)
	return strings.ReplaceAll(s, "match-then-"+name, "match-then-adjudicate")
}

func TestW2Warnings(t *testing.T) {
	src := edit("  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "")
	src = strings.Replace(src, "  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }\n", "", 1)
	w := mustParse(t, src)
	if len(w.Warnings) != 1 || !strings.HasPrefix(w.Warnings[0], "loop_without_clean_exit") {
		t.Fatalf("warnings: %v", w.Warnings)
	}
}

func TestW3Resolve(t *testing.T) {
	w := mustParse(t, example)
	r, eff, err := w.Resolve(map[string]string{"JUDGE": "a", "JUDGE_EFFORT": "b"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Nodes["adjudicate"].Model != "a" || r.Nodes["adjudicate"].Effort != "b" || r.Nodes["discover"].Model != "claude-opus-5" {
		t.Fatalf("prefix pair resolution: %+v", r.Nodes["adjudicate"])
	}
	if strings.Join(r.Cmds["notify"].Argv, " ") != "bash ./scripts/notify.sh --model a" || w.Cmds["notify"].Argv[3] != "$JUDGE" {
		t.Fatal("argv substitution must not mutate the original")
	}
	if eff["REVIEWER"] != "claude-opus-5" || eff["JUDGE"] != "a" || len(eff) != 3 {
		t.Fatalf("effective: %v", eff)
	}
	if strings.Join(r.VarsReferencedBy("adjudicate"), ",") != "JUDGE,JUDGE_EFFORT" || r.Hash != w.Hash {
		t.Fatal("resolved copy keeps Refs and Hash")
	}
	// $$ literal and list params
	src := edit("lenses: 8 }", "lenses: 8, tags: [$JUDGE, 3, '$$x'], note: '$$' }")
	w2 := mustParse(t, src)
	r2, _, _ := w2.Resolve(map[string]string{"JUDGE": "j", "JUDGE_EFFORT": "e"}, false)
	tags := r2.Nodes["discover"].Params["tags"].([]any)
	if tags[0] != "j" || tags[1] != 3 || tags[2] != "$x" || r2.Nodes["discover"].Params["note"] != "$" || r2.Nodes["discover"].Params["lenses"] != 8 {
		t.Fatalf("params: %v", r2.Nodes["discover"].Params)
	}
	// errors
	if _, _, err := w.Resolve(map[string]string{"JUDGE": strings.Repeat("m", run.MaxShort+1), "JUDGE_EFFORT": "b"}, false); !errs.Is(err, CodeWorkflowInvalid) || errs.As(err).Field("reason") != "bad_var" {
		t.Fatalf("over-long model: %v", err)
	}
	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a"}, false); !errs.Is(err, CodeVarUnset) || errs.As(err).Field("name") != "JUDGE_EFFORT" {
		t.Fatalf("unset: %v", err)
	}
	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a", "JUDGE_EFFORT": "b", "FOO": "x"}, false); !errs.Is(err, CodeVarUnknown) || errs.As(err).Field("name") != "FOO" {
		t.Fatalf("unknown caller var: %v", err)
	}
	// calibration
	r3, eff3, err := w.Resolve(map[string]string{"REVIEWER": "r"}, true)
	if err != nil || eff3["JUDGE"] != CalibrationJudge || eff3["JUDGE_EFFORT"] != CalibrationEffort || r3.Nodes["adjudicate"].Model != CalibrationJudge {
		t.Fatalf("calibration pin: %v %v", eff3, err)
	}
	for _, name := range []string{"JUDGE", "JUDGE_EFFORT"} {
		if _, _, err := w.Resolve(map[string]string{name: "x"}, true); !errs.Is(err, CodeCalibrationPinned) || errs.As(err).Field("name") != name {
			t.Fatalf("pinned %s: %v", name, err)
		}
	}
	// re-resolve of stored (pinned) vars with calibration=false succeeds
	if _, _, err := w.Resolve(eff3, false); err != nil {
		t.Fatal("re-resolve stored vars")
	}
	// calibration on a workflow without JUDGE is a no-op
	noJudge := mustParse(t, strings.Replace(strings.ReplaceAll(strings.Replace(example, "JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, ", "", 1), "model: $JUDGE, effort: $JUDGE_EFFORT", "model: x"), "--model, $JUDGE", "--model, x", 1))
	if _, eff4, err := noJudge.Resolve(nil, true); err != nil || len(eff4) != 1 {
		t.Fatalf("no-judge calibration: %v %v", eff4, err)
	}
}

func TestW4ResolveCmds(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(work, "scripts", "notify.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(name string) (string, error) {
		switch name {
		case "bash":
			return "/bin/bash", nil
		case "rel":
			return "bin/rel", nil
		}
		if filepath.IsAbs(name) {
			if _, err := os.Stat(name); err == nil {
				return name, nil
			}
		}
		return "", errors.New("not found")
	}
	hashes := map[string]string{"/bin/bash": "hb", script: "hs"}
	hash := func(p string) (string, error) {
		if h, ok := hashes[p]; ok {
			return h, nil
		}
		return "", errors.New("no such file")
	}
	src := edit("cmds:\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }",
		"cmds:\n  zeta: { argv: [./scripts/notify.sh, x] }\n  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }")
	src = strings.Replace(src, "timeout: 30", "timeout: 2", 1) // 1500 ms cannot be expressed; use 2 s → 2000 ms
	w := mustParse(t, src)
	r, _, err := w.Resolve(map[string]string{"JUDGE": "gpt", "JUDGE_EFFORT": "low"}, false)
	if err != nil {
		t.Fatal(err)
	}
	allowed, sha, err := ResolveCmds(r, work, lookPath, hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 2 || allowed[0].Name != "notify" || allowed[1].Name != "zeta" {
		t.Fatalf("sorted by name: %+v", allowed)
	}
	n := allowed[0]
	if n.Argv[0] != "/bin/bash" || n.Argv[3] != "gpt" || n.TimeoutMS != 2000 || strings.Join(n.Env, ",") != "SLACK_WEBHOOK" {
		t.Fatalf("notify: %+v", n)
	}
	if n.FileHashes["/bin/bash"] != "hb" || n.FileHashes[script] != "hs" || len(n.FileHashes) != 2 {
		t.Fatalf("file hashes: %v", n.FileHashes)
	}
	z := allowed[1]
	if z.Argv[0] != script || z.FileHashes[script] != "hs" || len(z.FileHashes) != 1 || z.Env != nil || z.TimeoutMS != 60000 {
		t.Fatalf("zeta: %+v", z)
	}
	// hand-authored preimage: the fixture is the canonical JSON of this list; its sha is computed by shasum (see testdata/cmds-preimage.sha256)
	fixture, err := os.ReadFile("testdata/cmds-preimage.json")
	if err != nil {
		t.Fatal(err)
	}
	wantSha := strings.TrimSpace(string(must(os.ReadFile("testdata/cmds-preimage.sha256"))))
	got, _ := json.Marshal(allowed)
	canon, _ := run.Canonical(got)
	if string(canon) != strings.TrimSpace(strings.ReplaceAll(string(fixture), "WORK", work)) {
		t.Fatalf("preimage drift:\n%s\n%s", canon, fixture)
	}
	// the pinned .sha256 (WORK=/w) must equal CmdsSHA256 of the list resolved in /w through the fakes
	hashesW := map[string]string{"/bin/bash": "hb", "/w/scripts/notify.sh": "hs"}
	lookW := func(name string) (string, error) {
		switch name {
		case "bash":
			return "/bin/bash", nil
		case "/w/scripts/notify.sh":
			return name, nil
		}
		return "", errors.New("nf")
	}
	allowedW, shaW, err := ResolveCmds(r, "/w", lookW, hashFor2(hashesW))
	if err != nil || shaW != wantSha || CmdsSHA256(allowedW) != wantSha {
		t.Fatalf("pinned preimage sha: %s vs %s (%v)", shaW, wantSha, err)
	}
	sum := sha256.Sum256([]byte(strings.ReplaceAll(strings.TrimSpace(string(fixture)), "WORK", "/w")))
	if hex.EncodeToString(sum[:]) != wantSha {
		t.Fatalf("fixture sha256 %s != pinned %s", hex.EncodeToString(sum[:]), wantSha)
	}
	// declaration order independent
	rev := append([]run.AllowedCmd{}, allowed[1], allowed[0])
	if CmdsSHA256(rev) != sha {
		t.Fatal("order independent")
	}
	// one-byte edit moves the sha
	mod := append([]run.AllowedCmd{}, allowed...)
	mod[0].Env = []string{"SLACK_WEBHOOK", "X"}
	if CmdsSHA256(mod) == sha {
		t.Fatal("env is part of the preimage")
	}
	mod = append([]run.AllowedCmd{}, allowed...)
	mod[0].TimeoutMS = 2001
	if CmdsSHA256(mod) == sha {
		t.Fatal("timeout is part of the preimage")
	}
	// no cmds → empty
	if a, s, err := ResolveCmds(mustParse(t, string(must(workflows.Read("review-loop")))), work, lookPath, hash); a != nil || s != "" || err != nil {
		t.Fatal("no cmds")
	}
	// not found / relative lookPath result / missing relative script
	for _, bad := range []string{"argv: [nope]", "argv: [rel]", "argv: [./missing.sh]"} {
		wb := mustParse(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", bad))
		rb, _, _ := wb.Resolve(map[string]string{"JUDGE": "g", "JUDGE_EFFORT": "l"}, false)
		if _, _, err := ResolveCmds(rb, work, lookPath, hash); !errs.Is(err, CodeCmdNotFound) || errs.As(err).Field("name") != "notify" {
			t.Fatalf("%s: %v", bad, err)
		}
	}
	// VerifyCmds: ok, mismatch, missing, appeared
	if err := VerifyCmds(allowed, work, hash); err != nil {
		t.Fatal(err)
	}
	hashes[script] = "changed"
	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "mismatch" {
		t.Fatalf("mismatch: %v", err)
	}
	delete(hashes, script)
	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "missing" {
		t.Fatalf("missing: %v", err)
	}
	hashes[script] = "hs"
	hashes[filepath.Join(work, "--model")] = "hm" // an unpinned element now names a file
	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "appeared" || errs.As(err).Field("path") != filepath.Join(work, "--model") {
		t.Fatalf("appeared: %v", err)
	}
	// symlinked script: hashed through the link (target contents), keyed by the argv path
	link := filepath.Join(work, "scripts", "link.sh")
	if err := os.Symlink(script, link); err != nil {
		t.Fatal(err)
	}
	if h, err := FileSHA256(link); err != nil || h != hexSum("#!/bin/sh\n") {
		t.Fatalf("symlink hashed through: %s %v", h, err)
	}
	dirLink := filepath.Join(work, "dirlink")
	_ = os.Symlink(work, dirLink)
	if _, err := FileSHA256(dirLink); !errs.Is(err, CodeCmdChanged) {
		t.Fatal("symlink to a directory is irregular")
	}
	// FileSHA256: regular, directory, missing
	h, err := FileSHA256(script)
	if err != nil || h != hexSum("#!/bin/sh\n") {
		t.Fatalf("FileSHA256: %s %v", h, err)
	}
	if _, err := FileSHA256(work); !errs.Is(err, CodeCmdChanged) {
		t.Fatal("dir is irregular")
	}
	if _, err := FileSHA256(filepath.Join(work, "nope")); err == nil {
		t.Fatal("missing")
	}
	unreadable := filepath.Join(work, "unreadable")
	if err := os.WriteFile(unreadable, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() != 0 {
		if _, err := FileSHA256(unreadable); err == nil {
			t.Fatal("unreadable")
		}
	}
}

func hashFor2(m map[string]string) func(string) (string, error) {
	return func(p string) (string, error) {
		if h, ok := m[p]; ok {
			return h, nil
		}
		return "", errors.New("no such file")
	}
}

func hexSum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestW5Accessors(t *testing.T) {
	w := mustParse(t, example)
	out := w.Outgoing("discover")
	if len(out) != 2 || out[0].Gate != "findings_empty" || out[1].Gate != "findings_nonempty" {
		t.Fatalf("Outgoing order: %+v", out)
	}
	if w.NodeFor("done") != nil || w.NodeFor("fix") == nil {
		t.Fatal("NodeFor")
	}
	// VarsReferencedBy on a cmd node unions the cmd's refs
	src := edit("  fix:        { kind: agent-edit }", "  fix:        { kind: cmd, cmd: notify, tag: $REVIEWER }")
	wc := mustParse(t, src)
	if strings.Join(wc.VarsReferencedBy("fix"), ",") != "JUDGE,REVIEWER" {
		t.Fatalf("VarsReferencedBy cmd node: %v", wc.VarsReferencedBy("fix"))
	}
	names := w.cmdNames()
	sort.Strings(names)
	if strings.Join(names, ",") != "notify" {
		t.Fatal("cmdNames")
	}
}

// sdlc-loop-clean is the "review the code you just wrote" workflow: after every fix it RE-REVIEWS the
// change at `recheck` and only reaches done when a fresh review is clean, so a bug the fix itself
// introduces is caught. This pins that topology (distinct from sdlc-loop, which exits on all_fixed
// without re-reviewing the fix).
func TestSDLCLoopCleanReReviewsAfterFix(t *testing.T) {
	raw, err := workflows.Read("sdlc-loop-clean")
	if err != nil {
		t.Fatal(err)
	}
	w, err := Parse(raw, Options{Kinds: kinds()})
	if err != nil {
		t.Fatalf("sdlc-loop-clean must be a valid workflow: %v", err)
	}
	has := func(from, to run.State) *Transition {
		for i := range w.Transitions {
			if w.Transitions[i].From == from && w.Transitions[i].To == to {
				return &w.Transitions[i]
			}
		}
		return nil
	}
	// After a fix, control goes to `recheck` (a re-review), not straight to done.
	if has("fix", "recheck") == nil {
		t.Fatal("fix must transition to recheck (re-review the fix), not done")
	}
	if has("fix", "done") != nil {
		t.Fatal("fix must NOT exit directly to done without re-review")
	}
	// recheck exits clean only when a FRESH review finds nothing, and otherwise LOOPS back to adjudicate.
	clean := has("recheck", "done")
	if clean == nil || clean.Gate != "findings_empty" || clean.Outcome == "" {
		t.Fatalf("recheck must exit to done on findings_empty with an outcome, got %+v", clean)
	}
	loop := has("recheck", "adjudicate")
	if loop == nil || !loop.Loop || loop.Gate != "findings_nonempty" {
		t.Fatalf("recheck must LOOP back to adjudicate on findings_nonempty, got %+v", loop)
	}
	// It never exits on all_fixed (the sdlc-loop behavior this workflow deliberately replaces).
	for _, tr := range w.Transitions {
		if tr.Gate == "all_fixed" {
			t.Fatal("sdlc-loop-clean must not exit on all_fixed; it re-reviews until a fresh review is clean")
		}
	}
}
