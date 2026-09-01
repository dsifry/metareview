// Package workflow turns a workflow YAML into a validated, resolvable
// Workflow: states, transitions, nodes, declared commands, and the
// convergence tree. Every rule is static; nothing here runs anything.
package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Error codes.
const (
	CodeWorkflowInvalid   = "ERR_WORKFLOW_INVALID"
	CodeVarUnset          = "ERR_VAR_UNSET"
	CodeVarUnknown        = "ERR_VAR_UNKNOWN"
	CodeCalibrationPinned = "ERR_CALIBRATION_PINNED"
	CodeCmdNotFound       = "ERR_CMD_NOT_FOUND"
	CodeCmdChanged        = "ERR_CMD_CHANGED"
)

// Calibration pins (design §17: the reference judge and effort).
const (
	CalibrationJudge  = "gpt-5.2"
	CalibrationEffort = "medium"
)

// Reserved names.
const (
	FailedState    = run.State("failed")
	JudgeNodeName  = "judge" // spec 5's `fsm judge --run` appends llm_call{Node: "judge"}
	DefaultTimeout = 60 * time.Second
	MaxTimeout     = 3600 * time.Second
)

// VarSpec declares a workflow variable.
type VarSpec struct {
	Default  string
	Required bool
}

// CmdDecl is a declared command (the only place argv is written).
type CmdDecl struct {
	Name    string
	Argv    []string
	Timeout time.Duration
	Env     []string
}

// Node is a state's work: a kind plus its exec mode and parameters.
type Node struct {
	Name   string
	Kind   string
	Exec   string
	Model  string
	Effort string
	Params map[string]any
	Cmd    string
}

// Transition is one edge; Loop marks the single loop-closing edge.
type Transition struct {
	From, To run.State
	Gate     string
	Outcome  run.Outcome
	Loop     bool
}

// KindInfo is what Parse needs to know about a kind (from the registry).
type KindInfo struct {
	DefaultExec    string
	AllowedExec    []string
	ValidateParams func(map[string]any) error
	// NeedsJudge marks the kinds that call an LLM judge. exec: fork does not imply it —
	// the cmd kind forks a subprocess and carries no model — so callers that validate judge
	// configuration (machine's Preflight) must key on this rather than on Exec.
	NeedsJudge bool
	// FixScopedDiff marks a kind whose diff must be the FIX's own diff (FixEntryHead..head), not the
	// reviewed change (base..head). The prove kind sets it: its pin added-line bind and owed-pin check
	// must see what the fix changed, which in a loop differs from base..head (a restore-type fix nets
	// out against the original base). The machine falls back to base..head when FixEntryHead is unset.
	FixScopedDiff bool
}

// Options parameterizes Parse. Kinds is required.
type Options struct {
	Kinds map[string]KindInfo
}

// Workflow is the parsed, validated (and possibly resolved) workflow.
type Workflow struct {
	Name        string
	Version     int
	Vars        map[string]VarSpec
	States      []run.State
	Initial     run.State
	Transitions []Transition
	Nodes       map[run.State]*Node
	Cmds        map[string]*CmdDecl
	Convergence *yaml.Node
	RepoMode    string
	OnOverflow  string
	Hash        string
	Refs        map[run.State][]string
	CmdRefs     map[string][]string
	Warnings    []string
}

var (
	statePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	varPattern   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	cmdPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	varRef       = regexp.MustCompile(`\$(\$|[A-Z_][A-Z0-9_]*)`)
	execModes    = map[string]bool{"inline": true, "subagent": true, "fork": true}
	reservedEnv  = map[string]bool{"PATH": true, "HOME": true, "LANG": true, "TMPDIR": true, "BASH_ENV": true, "ENV": true, "PYTHONPATH": true, "PYTHONSTARTUP": true, "PYTHONHOME": true, "NODE_OPTIONS": true, "NODE_PATH": true, "PERL5OPT": true, "PERL5LIB": true, "RUBYOPT": true, "RUBYLIB": true, "JAVA_TOOL_OPTIONS": true, "SHELLOPTS": true, "PS4": true, "IFS": true, "CDPATH": true, "GLOBIGNORE": true, "PROMPT_COMMAND": true}
	reservedPfx  = []string{"MRV_", "LD_", "DYLD_", "GIT_"}
)

func invalid(reason, at, detail string) error {
	return errs.E(CodeWorkflowInvalid, detail, "reason", reason, "at", at)
}

// ---- raw YAML shapes ----

type rawWorkflow struct {
	Workflow    string                    `yaml:"workflow"`
	Version     int                       `yaml:"version"`
	Vars        map[string]rawVar         `yaml:"vars"`
	States      []string                  `yaml:"states"`
	Cmds        yaml.Node                 `yaml:"cmds"`
	Nodes       map[string]map[string]any `yaml:"nodes"`
	Transitions yaml.Node                 `yaml:"transitions"`
	Convergence yaml.Node                 `yaml:"convergence"`
	RepoMode    string                    `yaml:"repo_mode"`
	OnOverflow  string                    `yaml:"on_overflow"`
}

type rawVar struct {
	Default  *string `yaml:"default"`
	Required bool    `yaml:"required"`
}

type rawCmd struct {
	Argv    []any    `yaml:"argv"`
	Timeout *int     `yaml:"timeout"`
	Env     []string `yaml:"env"`
}

type rawTransition struct {
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Gate    string `yaml:"gate"`
	Outcome string `yaml:"outcome"`
	Loop    bool   `yaml:"loop"`
	On      string `yaml:"on"`
}

// Parse decodes and statically validates raw. $VAR tokens stay unresolved.
func Parse(raw []byte, opts Options) (*Workflow, error) {
	if len(opts.Kinds) == 0 {
		return nil, invalid("missing_kinds", "options", "Options.Kinds is required")
	}
	var rw rawWorkflow
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&rw); err != nil {
		if strings.Contains(err.Error(), "not found in type") {
			return nil, invalid("unknown_key", "document", err.Error())
		}
		return nil, invalid("bad_yaml", "document", err.Error())
	}
	sum := sha256.Sum256(raw)
	w := &Workflow{
		Name: rw.Workflow, Version: rw.Version, Vars: map[string]VarSpec{}, Nodes: map[run.State]*Node{},
		Cmds: map[string]*CmdDecl{}, RepoMode: rw.RepoMode, OnOverflow: rw.OnOverflow,
		Hash: hex.EncodeToString(sum[:]), Refs: map[run.State][]string{}, CmdRefs: map[string][]string{},
	}
	if w.Name == "" {
		return nil, invalid("missing_name", "workflow", "workflow name is required")
	}
	if w.Version != 1 {
		return nil, invalid("bad_version", "version", fmt.Sprintf("version must be 1, got %d", w.Version))
	}
	if len(rw.States) == 0 {
		return nil, invalid("no_initial", "states", "states must be non-empty")
	}
	seen := map[string]bool{}
	for _, s := range rw.States {
		if !statePattern.MatchString(s) || s == JudgeNodeName || seen[s] {
			return nil, invalid("bad_state", "states."+s, "state names must match ^[a-z][a-z0-9_-]{0,31}$, be unique, and not be `judge`")
		}
		seen[s] = true
		w.States = append(w.States, run.State(s))
	}
	w.Initial = w.States[0]
	if err := w.parseVars(rw.Vars); err != nil {
		return nil, err
	}
	if err := w.parseCmds(&rw.Cmds); err != nil {
		return nil, err
	}
	if err := w.parseNodes(rw.Nodes, opts.Kinds); err != nil {
		return nil, err
	}
	if err := w.parseTransitions(&rw.Transitions); err != nil {
		return nil, err
	}
	if err := w.validateGraph(); err != nil {
		return nil, err
	}
	if rw.Convergence.Kind != 0 {
		w.Convergence = &rw.Convergence
		if err := converge.ValidateBounded(w.Convergence, w.cmdNames(), w.LoopTransition() != nil); err != nil {
			return nil, invalid("bad_convergence", "convergence", errs.As(err).Field("detail"))
		}
	} else if w.LoopTransition() != nil {
		return nil, invalid("missing_convergence", "convergence", "a loop requires a convergence predicate")
	}
	if w.RepoMode == "" {
		w.RepoMode = "advisory"
	}
	if w.RepoMode != "advisory" && w.RepoMode != "enforcing" {
		return nil, invalid("bad_repo_mode", "repo_mode", "repo_mode must be advisory or enforcing")
	}
	if w.OnOverflow != "" && w.Cmds[w.OnOverflow] == nil {
		return nil, invalid("unknown_cmd", "on_overflow", "on_overflow names an undeclared cmd "+w.OnOverflow)
	}
	if err := w.collectRefs(); err != nil {
		return nil, err
	}
	w.warn()
	return w, nil
}

func (w *Workflow) parseVars(vars map[string]rawVar) error {
	if len(vars) > run.MaxVars {
		return invalid("bad_var", "vars", fmt.Sprintf("more than %d vars", run.MaxVars))
	}
	for name, v := range vars {
		if !varPattern.MatchString(name) {
			return invalid("bad_var", "vars."+name, "var names must match ^[A-Z_][A-Z0-9_]*$")
		}
		if v.Required && v.Default != nil {
			return invalid("bad_var", "vars."+name, "a required var cannot carry a default")
		}
		spec := VarSpec{Required: v.Required}
		if v.Default != nil {
			spec.Default = *v.Default
		}
		w.Vars[name] = spec
	}
	return nil
}

func (w *Workflow) parseCmds(n *yaml.Node) error {
	if n.Kind == 0 {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return invalid("bad_cmd", "cmds", "cmds must be a mapping")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		name := n.Content[i].Value
		at := "cmds." + name
		if !cmdPattern.MatchString(name) {
			return invalid("bad_cmd", at, "cmd names must match ^[a-z][a-z0-9_-]{0,31}$")
		}
		if w.Cmds[name] != nil {
			return invalid("duplicate_cmd", at, "cmd declared twice")
		}
		var rc rawCmd
		if err := strictNode(n.Content[i+1], &rc, "argv", "timeout", "env"); err != nil {
			return invalid("bad_cmd", at, err.Error())
		}
		if len(rc.Argv) == 0 || len(rc.Argv) > run.MaxArgv {
			return invalid("bad_cmd", at, fmt.Sprintf("argv must have 1..%d elements", run.MaxArgv))
		}
		d := &CmdDecl{Name: name, Timeout: DefaultTimeout}
		for _, a := range rc.Argv {
			s, ok := a.(string)
			if !ok || s == "" {
				return invalid("bad_cmd", at, "argv elements must be non-empty strings")
			}
			d.Argv = append(d.Argv, s)
		}
		if rc.Timeout != nil {
			if *rc.Timeout < 1 || *rc.Timeout > 3600 {
				return invalid("bad_cmd", at, "timeout must be 1..3600 seconds")
			}
			d.Timeout = time.Duration(*rc.Timeout) * time.Second
		}
		if len(rc.Env) > run.MaxEnv {
			return invalid("bad_env", at, fmt.Sprintf("more than %d env names", run.MaxEnv))
		}
		envSeen := map[string]bool{}
		for _, e := range rc.Env {
			if !varPattern.MatchString(e) || envSeen[e] || reservedEnvName(e) {
				return invalid("bad_env", at, "env name "+e+" is invalid, duplicate, or reserved")
			}
			envSeen[e] = true
			d.Env = append(d.Env, e)
		}
		w.Cmds[name] = d
	}
	if len(w.Cmds) > run.MaxAllowedCmds {
		return invalid("bad_cmd", "cmds", fmt.Sprintf("more than %d cmds", run.MaxAllowedCmds))
	}
	return nil
}

func reservedEnvName(e string) bool {
	if reservedEnv[e] {
		return true
	}
	for _, p := range reservedPfx {
		if strings.HasPrefix(e, p) {
			return true
		}
	}
	return false
}

// strictNode decodes a mapping node into out, refusing keys outside allowed.
func strictNode(n *yaml.Node, out any, allowed ...string) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if !containsStr(allowed, n.Content[i].Value) {
			return fmt.Errorf("line %d: unknown field %q", n.Line, n.Content[i].Value)
		}
	}
	return n.Decode(out)
}

func (w *Workflow) parseNodes(nodes map[string]map[string]any, kinds map[string]KindInfo) error {
	names := make([]string, 0, len(nodes))
	for n := range nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := nodes[name]
		at := "nodes." + name
		if !w.hasState(run.State(name)) {
			return invalid("unknown_state", at, "node for undeclared state "+name)
		}
		node := &Node{Name: name, Params: map[string]any{}}
		for k, v := range raw {
			s, isStr := v.(string)
			switch k {
			case "kind", "exec", "model", "effort", "cmd":
				if !isStr {
					return invalid("unknown_key", at+"."+k, k+" must be a string")
				}
				switch k {
				case "kind":
					node.Kind = s
				case "exec":
					node.Exec = s
				case "model":
					node.Model = s
				case "effort":
					node.Effort = s
				case "cmd":
					node.Cmd = s
				}
			default:
				node.Params[k] = v
			}
		}
		if node.Kind == "" {
			return invalid("node_without_kind", at, "node needs a kind")
		}
		info, ok := kinds[node.Kind]
		if !ok {
			return invalid("unknown_kind", at, "unknown kind "+node.Kind)
		}
		if node.Exec == "" {
			node.Exec = info.DefaultExec
		}
		if !execModes[node.Exec] {
			return invalid("unknown_exec", at, "exec must be inline, subagent, or fork")
		}
		if !containsStr(info.AllowedExec, node.Exec) {
			return invalid("exec_kind_mismatch", at, fmt.Sprintf("kind %s does not allow exec %s", node.Kind, node.Exec))
		}
		if info.ValidateParams != nil {
			if err := info.ValidateParams(node.Params); err != nil {
				return invalid("bad_params", at, err.Error())
			}
		}
		if (node.Kind == "cmd") != (node.Cmd != "") {
			return invalid("cmd_without_kind", at, "cmd: is required on cmd kinds and forbidden elsewhere")
		}
		if node.Cmd != "" && w.Cmds[node.Cmd] == nil {
			return invalid("unknown_cmd", at, "node references undeclared cmd "+node.Cmd)
		}
		w.Nodes[run.State(name)] = node
	}
	return nil
}

func (w *Workflow) parseTransitions(n *yaml.Node) error {
	switch n.Kind {
	case yaml.SequenceNode:
		for i, c := range n.Content {
			var rt rawTransition
			if err := strictNode(c, &rt, "from", "to", "gate", "outcome", "loop"); err != nil {
				return invalid("unknown_key", fmt.Sprintf("transitions[%d]", i), err.Error())
			}
			if err := w.addTransition(rt, fmt.Sprintf("transitions[%d]", i)); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			from, to, ok := splitArrow(key)
			if !ok {
				return invalid("unknown_key", "transitions."+key, "mapping-form keys look like from→to")
			}
			if from == "*" {
				continue // the implicit *→failed rule
			}
			var rt rawTransition
			if err := strictNode(n.Content[i+1], &rt, "gate", "outcome", "loop", "on"); err != nil {
				return invalid("unknown_key", "transitions."+key, err.Error())
			}
			rt.From, rt.To = from, to
			if err := w.addTransition(rt, "transitions."+key); err != nil {
				return err
			}
		}
	default:
		return invalid("unknown_key", "transitions", "transitions must be a list or a mapping")
	}
	return nil
}

func splitArrow(key string) (string, string, bool) {
	for _, sep := range []string{"→", "->"} {
		if i := strings.Index(key, sep); i > 0 {
			return strings.TrimSpace(key[:i]), strings.TrimSpace(key[i+len(sep):]), true
		}
	}
	return "", "", false
}

func (w *Workflow) addTransition(rt rawTransition, at string) error {
	t := Transition{From: run.State(rt.From), To: run.State(rt.To), Gate: rt.Gate, Outcome: run.Outcome(rt.Outcome), Loop: rt.Loop}
	if !w.hasState(t.From) || !w.hasState(t.To) {
		return invalid("unknown_state", at, "transition references an undeclared state")
	}
	if t.From == FailedState || t.To == FailedState {
		return invalid("failed_reserved", at, "failed is the implicit *→failed target and appears in no transition")
	}
	if _, ok := gate.Builtin(t.Gate); !ok {
		return invalid("unknown_gate", at, "unknown gate "+t.Gate)
	}
	for _, o := range w.Transitions {
		if o.From == t.From && o.Gate == t.Gate {
			return invalid("duplicate_transition", at, fmt.Sprintf("second transition from %s with gate %s", t.From, t.Gate))
		}
	}
	w.Transitions = append(w.Transitions, t)
	return nil
}

func (w *Workflow) validateGraph() error {
	if !w.hasState(FailedState) || w.Nodes[FailedState] != nil {
		return invalid("failed_reserved", "states", "failed must be declared and carry no node")
	}
	if len(w.Outgoing(w.Initial)) == 0 {
		return invalid("initial_terminal", "states."+string(w.Initial), "the initial state needs an outgoing transition")
	}
	for s, n := range w.Nodes {
		if w.IsTerminal(s) {
			return invalid("terminal_with_node", "nodes."+n.Name, "terminal states carry no node")
		}
	}
	for i, t := range w.Transitions {
		at := fmt.Sprintf("transitions[%d]", i)
		if w.IsTerminal(t.To) {
			if t.Outcome == "" {
				return invalid("terminal_without_outcome", at, "a transition into a terminal state needs an outcome")
			}
			if !validOutcome(t.Outcome) {
				return invalid("bad_outcome", at, "unknown outcome "+string(t.Outcome))
			}
		} else if t.Outcome != "" {
			return invalid("outcome_on_nonterminal", at, "only transitions into terminal states carry an outcome")
		}
	}
	incoming := map[run.State]int{}
	for _, t := range w.Transitions {
		incoming[t.To]++
	}
	for _, s := range w.States[1:] {
		if s != FailedState && incoming[s] == 0 {
			return invalid("unreachable_state", "states."+string(s), "state has no incoming transition")
		}
	}
	loops := 0
	var loop *Transition
	for i := range w.Transitions {
		if w.Transitions[i].Loop {
			loops++
			loop = &w.Transitions[i]
		}
	}
	if loops > 1 {
		return invalid("loop_count", "transitions", "more than one loop transition")
	}
	if loop != nil {
		if !w.reaches(loop.To, loop.From) {
			return invalid("loop_not_cycle", "transitions", "the loop's target must reach its source")
		}
		terminals := 0
		for _, t := range w.Outgoing(loop.From) {
			if !t.Loop && w.IsTerminal(t.To) && t.Outcome != "" {
				terminals++
			}
		}
		if terminals != 1 {
			return invalid("loop_terminal", "transitions", "the loop-carrying state needs exactly one outcome-bearing terminal transition")
		}
	}
	if w.hasCycleWithoutLoop() {
		return invalid("cycle_without_loop", "transitions", "non-loop transitions form a cycle")
	}
	return nil
}

func validOutcome(o run.Outcome) bool {
	for _, x := range run.Outcomes {
		if x == o && o != run.OutcomeFailed {
			return true
		}
	}
	return false
}

// reaches reports whether from reaches to via non-loop transitions.
func (w *Workflow) reaches(from, to run.State) bool {
	seen := map[run.State]bool{}
	stack := []run.State{from}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s == to {
			return true
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		for _, t := range w.Outgoing(s) {
			if !t.Loop {
				stack = append(stack, t.To)
			}
		}
	}
	return false
}

func (w *Workflow) hasCycleWithoutLoop() bool {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[run.State]int{}
	var visit func(run.State) bool
	visit = func(s run.State) bool {
		color[s] = gray
		for _, t := range w.Outgoing(s) {
			if t.Loop {
				continue
			}
			switch color[t.To] {
			case gray:
				return true
			case white:
				if visit(t.To) {
					return true
				}
			}
		}
		color[s] = black
		return false
	}
	for _, s := range w.States {
		if color[s] == white && visit(s) {
			return true
		}
	}
	return false
}

func (w *Workflow) collectRefs() error {
	for s, n := range w.Nodes {
		refs := refsIn(n.Model, n.Effort)
		refs = append(refs, paramRefs(n.Params)...)
		for _, r := range refs {
			if _, ok := w.Vars[r]; !ok {
				return invalid("unknown_var", "nodes."+n.Name, "$"+r+" is not a declared var")
			}
		}
		w.Refs[s] = uniqSorted(refs)
	}
	for name, c := range w.Cmds {
		refs := refsIn(c.Argv...)
		for _, r := range refs {
			if _, ok := w.Vars[r]; !ok {
				return invalid("unknown_var", "cmds."+name, "$"+r+" is not a declared var")
			}
		}
		w.CmdRefs[name] = uniqSorted(refs)
	}
	return nil
}

func refsIn(ss ...string) []string {
	var out []string
	for _, s := range ss {
		for _, m := range varRef.FindAllStringSubmatch(s, -1) {
			if m[1] != "$" {
				out = append(out, m[1])
			}
		}
	}
	return out
}

func paramRefs(params map[string]any) []string {
	var out []string
	for _, v := range params {
		switch x := v.(type) {
		case string:
			out = append(out, refsIn(x)...)
		case []any:
			for _, e := range x {
				if s, ok := e.(string); ok {
					out = append(out, refsIn(s)...)
				}
			}
		}
	}
	return out
}

func uniqSorted(in []string) []string {
	m := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func (w *Workflow) warn() {
	if w.LoopTransition() == nil {
		return
	}
	for _, t := range w.Transitions {
		if t.Outcome == run.OutcomeClean {
			return
		}
	}
	w.Warnings = append(w.Warnings, "loop_without_clean_exit: no transition carries outcome clean; a loop that finds nothing ends overflow (all_fixed needs a non-empty AllFound)")
}

// ---- accessors ----

func (w *Workflow) hasState(s run.State) bool {
	for _, x := range w.States {
		if x == s {
			return true
		}
	}
	return false
}

func (w *Workflow) cmdNames() []string {
	names := make([]string, 0, len(w.Cmds))
	for n := range w.Cmds {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// NodeFor returns the node of s, or nil.
func (w *Workflow) NodeFor(s run.State) *Node { return w.Nodes[s] }

// Outgoing returns s's transitions in declaration order.
func (w *Workflow) Outgoing(s run.State) []Transition {
	var out []Transition
	for _, t := range w.Transitions {
		if t.From == s {
			out = append(out, t)
		}
	}
	return out
}

// IsTerminal reports whether s has no outgoing transition.
func (w *Workflow) IsTerminal(s run.State) bool { return len(w.Outgoing(s)) == 0 }

// LoopTransition returns the loop edge, or nil.
func (w *Workflow) LoopTransition() *Transition {
	for i := range w.Transitions {
		if w.Transitions[i].Loop {
			return &w.Transitions[i]
		}
	}
	return nil
}

// TerminalFor returns the loop-carrying state's outcome-bearing terminal
// transition (validated unique), or nil for any other state.
func (w *Workflow) TerminalFor(s run.State) *Transition {
	loop := w.LoopTransition()
	if loop == nil || loop.From != s {
		return nil
	}
	var found *Transition
	for i, t := range w.Transitions {
		if t.From == s && !t.Loop && w.IsTerminal(t.To) && t.Outcome != "" {
			found = &w.Transitions[i]
		}
	}
	return found
}

// VarsReferencedBy lists the $VARs a state's node (and its cmd) reference.
func (w *Workflow) VarsReferencedBy(s run.State) []string {
	refs := append([]string{}, w.Refs[s]...)
	if n := w.Nodes[s]; n != nil && n.Cmd != "" {
		refs = append(refs, w.CmdRefs[n.Cmd]...)
	}
	return uniqSorted(refs)
}
