// Package converge implements the convergence predicate tree evaluated at a
// loop boundary: atoms (all_fixed, no_fixation_progress, max_iterations,
// budget, cmd) composed with any/all/not.
//
// Deterministic workflow structure; the cmd atom's result is whatever the
// sanctioned command says — auditable, never "deterministic results".
package converge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// AllFixed reports whether every bug found so far is fixed. Nothing found is
// NOT fixed (review-loop expresses "nothing to fix" with findings_empty).
func AllFixed(s run.Snapshot) bool { return len(s.AllFound) > 0 && s.Unfixed == 0 }

// CmdResult is what a sanctioned command produced.
type CmdResult struct {
	Stdout, Stderr []byte
	ExitCode       int
	Duration       time.Duration
}

// Runner executes a declared command by name (argv is pinned at consent time
// and never supplied here). Implemented by cmdexec.Guarded, which also audits.
type Runner interface {
	Run(ctx context.Context, name string, stdin []byte) (CmdResult, error)
}

// Result is what an evaluation decided. Atom/Class name the deciding atom
// (for `any`, the first child that fired; for `all`, the joined names and the
// first child's class).
type Result struct {
	Stop   bool
	Atom   string
	Class  run.Outcome
	Reason string
}

// Caller is Runner plus the typed-decode Call the cmd kind needs; the single
// Guarded factory returns one and the machine hands it to executors.
type Caller interface {
	Runner
	Call(ctx context.Context, name string, stdin []byte, out any) error
}

// Predicate is one node of the convergence tree.
type Predicate interface {
	Name() string
	Class() run.Outcome
	Evaluate(ctx context.Context, s run.Snapshot) (Result, error)
}

// MaxDepth bounds nesting so composite names stay within run.MaxShort.
const MaxDepth = 4

// MaxAtoms bounds the number of leaves for the same reason.
const MaxAtoms = 32

// Error codes produced by this package.
const (
	CodeBadConvergence   = "ERR_BAD_CONVERGENCE"
	CodeCmdOutputInvalid = "ERR_CMD_OUTPUT_INVALID"
	CodeCmdFailed        = "ERR_CMD_FAILED"
)

// Validate checks the structure of a convergence tree without binding it.
// cmdNames lists the declared command names a cmd atom may reference (nil
// accepts any name; workflow.Parse always passes the declared list).
func Validate(node *yaml.Node, cmdNames []string) error {
	_, err := parse(node, nil, cmdNames, true, 0)
	return err
}

// Parse validates and binds a convergence tree; cmd atoms call runner.
func Parse(node *yaml.Node, runner Runner) (Predicate, error) {
	return parse(node, runner, nil, true, 0)
}

// MustParse binds a tree that already passed Validate (workflow.Parse ran
// it); the error path of parse cannot recur on validated input.
func MustParse(node *yaml.Node, runner Runner) Predicate {
	p, _ := parse(node, runner, nil, true, 0)
	return p
}

func bad(detail string) error {
	return errs.E(CodeBadConvergence, detail, "detail", detail)
}

// parse walks the tree. When cmdNames is nil every cmd name is accepted
// (Parse trusts workflow.Parse's earlier Validate); otherwise names must be
// declared. top is true at the root and for direct children of a root-level
// `any` (depth tracks nesting): `all_fixed` is legal only there, so a give-up
// can never carry the class `fixed` through `all`/`not` (plan C3).
func parse(node *yaml.Node, runner Runner, cmdNames []string, top bool, depth int) (Predicate, error) {
	if node == nil || node.Kind == 0 {
		return nil, bad("empty convergence")
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return nil, bad("empty convergence")
		}
		return parse(node.Content[0], runner, cmdNames, true, 0)
	}
	if depth > MaxDepth {
		return nil, bad(fmt.Sprintf("line %d: convergence tree deeper than %d", node.Line, MaxDepth))
	}
	if node.Kind == yaml.ScalarNode {
		// Bare form: `no_fixation_progress` / `all_fixed` (the shipped YAMLs use it).
		switch node.Value {
		case "all_fixed":
			if !top {
				return nil, bad(fmt.Sprintf("line %d: all_fixed may only appear at the top level or directly under a top-level any", node.Line))
			}
			return allFixed{}, nil
		case "no_fixation_progress":
			return noProgress{}, nil
		}
		return nil, bad(fmt.Sprintf("line %d: unknown atom %q", node.Line, node.Value))
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return nil, bad(fmt.Sprintf("line %d: an atom is a mapping with exactly one key", node.Line))
	}
	key, val := node.Content[0].Value, node.Content[1]
	switch key {
	case "all_fixed", "no_fixation_progress":
		var on bool
		if err := val.Decode(&on); err != nil || !on {
			return nil, bad(fmt.Sprintf("line %d: %s must be true", node.Line, key))
		}
		if key == "all_fixed" {
			if !top {
				return nil, bad(fmt.Sprintf("line %d: all_fixed may only appear at the top level or directly under a top-level any", node.Line))
			}
			return allFixed{}, nil
		}
		return noProgress{}, nil
	case "max_iterations":
		var n int
		if err := val.Decode(&n); err != nil || n <= 0 {
			return nil, bad(fmt.Sprintf("line %d: max_iterations must be a positive integer", node.Line))
		}
		return maxIter{n: n}, nil
	case "budget":
		var tokens int64
		if val.Kind != yaml.MappingNode || len(val.Content) != 2 || val.Content[0].Value != "tokens" || val.Content[1].Decode(&tokens) != nil || tokens <= 0 {
			return nil, bad(fmt.Sprintf("line %d: budget must be {tokens: positive integer}", node.Line))
		}
		return budget{tokens: tokens}, nil
	case "cmd":
		var name string
		if err := val.Decode(&name); err != nil || name == "" {
			return nil, bad(fmt.Sprintf("line %d: cmd must name a declared command", node.Line))
		}
		if cmdNames != nil && !contains(cmdNames, name) {
			return nil, bad(fmt.Sprintf("line %d: unknown cmd %q", node.Line, name))
		}
		return &cmdAtom{name: name, runner: runner}, nil
	case "any", "all":
		if val.Kind != yaml.SequenceNode || len(val.Content) == 0 || len(val.Content) > MaxAtoms {
			return nil, bad(fmt.Sprintf("line %d: %s must be a list of 1..%d predicates", node.Line, key, MaxAtoms))
		}
		kids := make([]Predicate, 0, len(val.Content))
		for _, c := range val.Content {
			p, err := parse(c, runner, cmdNames, key == "any" && depth == 0, depth+1)
			if err != nil {
				return nil, err
			}
			kids = append(kids, p)
		}
		return &compound{op: key, kids: kids}, nil
	case "not":
		inner, err := parse(val, runner, cmdNames, false, depth+1)
		if err != nil {
			return nil, err
		}
		return &not{inner: inner}, nil
	}
	return nil, bad(fmt.Sprintf("line %d: unknown atom %q", node.Line, key))
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

type allFixed struct{}

func (allFixed) Name() string       { return "all_fixed" }
func (allFixed) Class() run.Outcome { return run.OutcomeFixed }
func (a allFixed) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
	return decide(a, AllFixed(s), "all bugs fixed"), nil
}

// decide builds a Result for a leaf atom.
func decide(p Predicate, stop bool, reason string) Result {
	r := Result{Stop: stop, Atom: p.Name(), Class: p.Class()}
	if stop {
		r.Reason = reason
	}
	return r
}

type noProgress struct{}

func (noProgress) Name() string       { return "no_fixation_progress" }
func (noProgress) Class() run.Outcome { return run.OutcomeStalled }
func (n noProgress) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
	stop := s.PrevUnfixed != nil && s.Unfixed >= *s.PrevUnfixed
	reason := ""
	if stop {
		reason = fmt.Sprintf("unfixed %d >= previous %d", s.Unfixed, *s.PrevUnfixed)
	}
	return decide(n, stop, reason), nil
}

type maxIter struct{ n int }

func (maxIter) Name() string       { return "max_iterations" }
func (maxIter) Class() run.Outcome { return run.OutcomeOverflow }
func (m maxIter) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
	return decide(m, s.Iteration+1 >= m.n, fmt.Sprintf("iteration %d reached max_iterations %d", s.Iteration, m.n)), nil
}

type budget struct{ tokens int64 }

func (budget) Name() string       { return "budget" }
func (budget) Class() run.Outcome { return run.OutcomeOverflow }
func (b budget) Evaluate(_ context.Context, s run.Snapshot) (Result, error) {
	t := s.Tokens.Total()
	return decide(b, t >= b.tokens, fmt.Sprintf("tokens %d >= budget %d", t, b.tokens)), nil
}

type cmdAtom struct {
	name   string
	runner Runner
}

func (c *cmdAtom) Name() string       { return "cmd:" + c.name }
func (c *cmdAtom) Class() run.Outcome { return run.OutcomeCustom }
func (c *cmdAtom) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
	res, err := c.runner.Run(ctx, c.name, Payload(s))
	if err != nil {
		return Result{}, err
	}
	if res.ExitCode != 0 {
		return Result{}, errs.E(CodeCmdFailed, fmt.Sprintf("cmd %s exited %d", c.name, res.ExitCode), "name", c.name, "exit", fmt.Sprint(res.ExitCode))
	}
	var out struct {
		Stop   bool   `json:"stop"`
		Reason string `json:"reason"`
	}
	dec := json.NewDecoder(bytes.NewReader(res.Stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return Result{}, errs.E(CodeCmdOutputInvalid, fmt.Sprintf("cmd %s stdout is not {stop, reason}: %v", c.name, err), "name", c.name)
	}
	return decide(c, out.Stop, out.Reason), nil
}

// Payload is the JSON handed to sanctioned commands: the snapshot with var
// values replaced by their sha256 (commands are consented to run, not to
// receive credentials) and node outputs omitted (they are not the
// convergence question and can be megabytes).
func Payload(s run.Snapshot) []byte {
	c := s.Clone()
	c.NodeOutputs = nil
	for k, v := range c.Vars {
		sum := sha256.Sum256([]byte(v))
		c.Vars[k] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return run.MarshalCanonical(c)
}

type compound struct {
	op   string
	kids []Predicate
}

func (c *compound) Name() string {
	names := make([]string, len(c.kids))
	for i, k := range c.kids {
		names[i] = k.Name()
	}
	return c.op + "(" + strings.Join(names, "+") + ")"
}

func (c *compound) Class() run.Outcome { return c.kids[0].Class() }

// Evaluate: any stops with the first firing child's Result; all stops only
// when every child fires (Atom = names joined by "+", Class = the first's).
// Errors abort evaluation at the first failing child.
func (c *compound) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
	var names, reasons []string
	for _, k := range c.kids {
		r, err := k.Evaluate(ctx, s)
		if err != nil {
			return Result{}, err
		}
		if c.op == "any" {
			if r.Stop {
				return r, nil
			}
			continue
		}
		if !r.Stop {
			return Result{Atom: c.Name(), Class: c.Class()}, nil
		}
		names = append(names, r.Atom)
		reasons = append(reasons, r.Reason)
	}
	if c.op == "any" {
		return Result{Atom: c.Name(), Class: c.Class()}, nil
	}
	return Result{Stop: true, Atom: strings.Join(names, "+"), Class: c.Class(), Reason: strings.Join(reasons, "; ")}, nil
}

type not struct{ inner Predicate }

func (n *not) Name() string { return "not(" + n.inner.Name() + ")" }

// Class of a negation is always custom: inverting an atom must never mint a
// `fixed` or `stalled` classification (plan C3).
func (n *not) Class() run.Outcome { return run.OutcomeCustom }
func (n *not) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
	r, err := n.inner.Evaluate(ctx, s)
	if err != nil {
		return Result{}, err
	}
	out := Result{Stop: !r.Stop, Atom: n.Name(), Class: n.Class()}
	if out.Stop {
		out.Reason = "inner predicate did not fire"
	}
	return out, nil
}

// Stats describes a validated convergence tree (spec 5 §2: `fsm converge --check`).
type Stats struct {
	Atoms int
	Depth int
	Cmds  []string
}

// Describe validates node like Validate and reports its atom count, nesting depth, and the cmd names it references.
func Describe(node *yaml.Node, cmdNames []string) (Stats, error) {
	if err := Validate(node, cmdNames); err != nil {
		return Stats{}, err
	}
	st := Stats{Cmds: []string{}}
	describe(node, 0, &st)
	return st, nil
}

// describe walks a tree Validate already accepted: a scalar is an atom, a `cmd` mapping is a cmd atom, the
// `max_iterations`/`budget` mappings are atoms, `any`/`all`/`not` recurse.
func describe(node *yaml.Node, depth int, st *Stats) {
	if node.Kind == yaml.DocumentNode {
		describe(node.Content[0], depth, st)
		return
	}
	if depth > st.Depth {
		st.Depth = depth
	}
	if node.Kind == yaml.ScalarNode {
		st.Atoms++
		return
	}
	key, val := node.Content[0].Value, node.Content[1]
	switch key {
	case "any", "all":
		for _, c := range val.Content {
			describe(c, depth+1, st)
		}
	case "not":
		describe(val, depth+1, st)
	case "cmd":
		st.Atoms++
		st.Cmds = append(st.Cmds, val.Value)
	default: // max_iterations, budget
		st.Atoms++
	}
}
