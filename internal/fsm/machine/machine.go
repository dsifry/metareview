package machine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Machine drives one run. It caches nothing across calls except the last
// View: every Advance/Record re-reads and re-verifies the log under the lock.
type Machine struct {
	deps  Deps
	runID string
	view  View
	torn  bool
}

// session is the per-call working state (spec 2 §5.3b).
type session struct {
	m        *Machine
	ctx      context.Context
	st       run.FoldState
	log      run.Log
	w        *workflow.Workflow // resolved
	pred     converge.Predicate
	runner   converge.Runner
	git      gate.Git
	warns    []string
	auditErr error // the first store error seen through the audit closure
	unlock   func()
}

var recordName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// RunID returns the run this machine drives.
func (m *Machine) RunID() string { return m.runID }

// View returns the read model captured at the end of the last call.
func (m *Machine) View() View { return m.view }

// ---------------------------------------------------------------- Init

// Init creates a run (spec 2 §5.3).
func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
	// 1. load + parse + resolve
	var raw []byte
	var err error
	if strings.Contains(o.Workflow, "/") || strings.HasSuffix(o.Workflow, ".yaml") {
		raw, err = deps.ReadFile(o.Workflow)
		if err != nil {
			return nil, errs.E(CodeWorkflowNotFound, err.Error(), "workflow", o.Workflow)
		}
	} else {
		raw, err = deps.Workflows(o.Workflow)
		if err != nil {
			return nil, errs.E(CodeWorkflowNotFound, err.Error(), "workflow", o.Workflow)
		}
	}
	if len(raw) > MaxWorkflowBytes {
		return nil, errs.E(CodeWorkflowTooLarge, fmt.Sprintf("workflow is %d bytes (max %d)", len(raw), MaxWorkflowBytes), "workflow", o.Workflow)
	}
	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
	if err != nil {
		return nil, err
	}
	switch o.RepoMode {
	case "":
	case "enforcing":
		parsed.RepoMode = "enforcing"
	default:
		return nil, errs.E(CodeBadRepoMode, "repo mode override must be empty or enforcing (tighten-only)", "got", o.RepoMode)
	}
	w, vars, err := parsed.Resolve(o.Vars, o.Calibration)
	if err != nil {
		return nil, err
	}
	// 2. commands + consent
	allowed, sha, err := workflow.ResolveCmds(w, o.WorkDir, deps.LookPath, deps.FileHash)
	if err != nil {
		return nil, err
	}
	if len(allowed) > 0 && o.AllowCustomCmds != sha {
		return nil, errs.E(CodeCmdsNotAllowed, consentList(allowed, o.WorkDir), "sha", sha)
	}
	// 3. git
	g := deps.Git(o.WorkDir)
	common, err := g.CommonDir(ctx)
	if err != nil {
		return nil, err
	}
	rootCommon, err := deps.Git(o.RepoRoot).CommonDir(ctx)
	if err != nil {
		return nil, err
	}
	if common != rootCommon {
		return nil, errs.E(CodeWorkdirForeign, "work dir is not a worktree of the repository", "work_dir", o.WorkDir, "repo_root", o.RepoRoot)
	}
	head, err := g.Head(ctx)
	if err != nil {
		return nil, err
	}
	base := o.Base
	if base == "" {
		base = "HEAD"
	}
	baseSHA, err := g.RevParse(ctx, base)
	if err != nil {
		return nil, err
	}
	_, porcelain, err := g.Status(ctx)
	if err != nil {
		return nil, err
	}
	wt, err := g.WorkTree(ctx)
	if err != nil {
		return nil, err
	}
	// 4. goldens
	goldens := []run.Golden{}
	if o.GoldensPath != "" {
		goldens, err = readGoldens(deps, o.GoldensPath)
		if err != nil {
			return nil, err
		}
	}
	// 5. mock
	mock := ""
	if o.MockDir != "" {
		dir := o.MockDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(o.RepoRoot, dir)
		}
		h, err := deps.MockLoad(dir)
		if err != nil {
			return nil, errs.E(CodeMockInvalid, err.Error(), "dir", o.MockDir)
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(filepath.Clean(dir), filepath.Clean(o.RepoRoot)), string(filepath.Separator))
		mock = rel + "#" + h[:16]
	}
	if deps.Kinds.Mock() != (mock != "") {
		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match --mock-ai")
	}
	// 6. create, sidecar, first events
	now := deps.Clock()
	runID := o.RunID
	if runID == "" {
		runID = run.RunID(w.Name, now.Time)
	}
	var initialKind run.Kind
	if n := w.NodeFor(w.Initial); n != nil {
		initialKind = run.Kind(n.Kind)
	}
	if allowed == nil {
		allowed = []run.AllowedCmd{}
	}
	initData := run.InitData{
		RunID: runID, CreatedAt: now, Workflow: w.Name, WorkflowHash: w.Hash, Vars: vars, Calibration: o.Calibration,
		Mock: mock, RepoMode: w.RepoMode, AllowedCmds: allowed, CmdsSHA256: sha, RepoRoot: o.RepoRoot, WorkDir: o.WorkDir,
		BaseSHA: baseSHA, Head: head, InitialState: w.Initial, InitialKind: initialKind, Goldens: goldens, Lineage: []string{},
	}
	m := &Machine{deps: deps, runID: runID}
	first := run.Event{SchemaVersion: run.SchemaVersion, At: now, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
	st, err := deps.Store.Create(runID, first)
	if err != nil {
		return nil, err
	}
	if err := deps.Sidecar.Write(runID, SidecarWorkflow, raw); err != nil {
		return nil, err
	}
	unlock, err := deps.Store.Lock(runID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	sess := &session{m: m, ctx: ctx, st: st, w: w, git: g}
	status, truncated := run.CapDetail(porcelain)
	if err := sess.append(run.TypeTree, run.TreeData{Head: head, TreeHash: gate.TreeHash(head, wt), Status: status, StatusTruncated: truncated}, ""); err != nil {
		return nil, err
	}
	for _, warning := range w.Warnings {
		if err := sess.warn(WarnWorkflow, warning); err != nil {
			return nil, err
		}
	}
	m.view = sess.viewOf()
	return m, nil
}

// consentList renders the human-readable command list for ERR_CMDS_NOT_ALLOWED.
func consentList(allowed []run.AllowedCmd, workDir string) string {
	var b strings.Builder
	b.WriteString("commands this workflow will run (consent with --allow-custom-cmds <sha256>):\n")
	for _, c := range allowed {
		var pinned, unpinned []string
		for _, a := range c.Argv {
			p := a
			if !filepath.IsAbs(p) {
				p = filepath.Join(workDir, a)
			}
			if h, ok := c.FileHashes[p]; ok {
				pinned = append(pinned, fmt.Sprintf("%s=%s", p, h[:min(12, len(h))]))
			} else {
				unpinned = append(unpinned, a)
			}
		}
		fmt.Fprintf(&b, "  %s: argv=%q timeout=%dms env=%v\n    pinned: %s\n    unpinned: %s\n", c.Name, c.Argv, c.TimeoutMS, c.Env, strings.Join(pinned, ", "), strings.Join(unpinned, ", "))
	}
	return b.String()
}

func readGoldens(deps Deps, path string) ([]run.Golden, error) {
	raw, err := deps.ReadFile(path)
	if err != nil {
		return nil, errs.E(CodeGoldensInvalid, err.Error(), "path", path)
	}
	if len(raw) > MaxGoldensBytes {
		return nil, errs.E(CodeGoldensInvalid, "goldens file too large", "path", path)
	}
	var goldens []run.Golden
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&goldens); err != nil {
		return nil, errs.E(CodeGoldensInvalid, err.Error(), "path", path)
	}
	if len(goldens) > run.MaxGoldens {
		return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("more than %d goldens", run.MaxGoldens), "path", path)
	}
	seen := map[string]bool{}
	for i, g := range goldens {
		if g.Comment == "" {
			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d has an empty comment", i), "path", path)
		}
		if _, truncated := run.CapText(g.Comment, run.MaxDesc); truncated {
			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d exceeds %d bytes", i, run.MaxDesc), "path", path)
		}
		if seen[g.Comment] {
			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d duplicates an earlier comment", i), "path", path)
		}
		seen[g.Comment] = true
	}
	if goldens == nil {
		goldens = []run.Golden{}
	}
	return goldens, nil
}

// ---------------------------------------------------------------- Open / load

// Open loads an existing run (spec 2 §5.3b).
func Open(ctx context.Context, deps Deps, runID string, o OpenOptions) (*Machine, error) {
	m := &Machine{deps: deps, runID: runID}
	sess, err := m.load(ctx, o.Repair)
	if err != nil {
		return nil, err
	}
	sess.unlock()
	m.view = sess.viewOf()
	return m, nil
}

// load performs §5.3b steps 1–2 and returns a locked session.
func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
	deps := m.deps
	unlock, err := deps.Store.Lock(m.runID)
	if err != nil {
		return nil, err
	}
	sess := &session{m: m, ctx: ctx, unlock: unlock}
	ok := false
	defer func() {
		if !ok {
			unlock()
		}
	}()
	log, _, err := deps.Store.EventsWithLines(m.runID)
	if err != nil {
		return nil, err
	}
	if repair {
		if err := deps.Store.RepairTail(m.runID); err != nil {
			return nil, err
		}
		dropped := len(log.Torn.Bytes)
		log, _, err = deps.Store.EventsWithLines(m.runID)
		if err != nil {
			if isStoreCode(err, run.CodeRunNotFound) {
				return nil, errs.E(run.CodeRunNotFound, "run removed; torn bytes in runs/.torn/", "run", m.runID)
			}
			return nil, err
		}
		st, err := run.FoldFull(log.Events)
		if err != nil {
			return nil, err
		}
		st.ChainHead = log.Head
		sess.st, sess.log = st, log
		if err := sess.warn(WarnAuditTornLineDropped, fmt.Sprintf("%d bytes dropped after seq %d from audit.jsonl", dropped, st.Seq)); err != nil {
			return nil, err
		}
		log, _, err = deps.Store.EventsWithLines(m.runID)
		if err != nil {
			return nil, err
		}
	}
	m.torn = log.Torn != nil
	st, err := run.FoldFull(log.Events)
	if err != nil {
		return nil, err
	}
	st.ChainHead = log.Head
	sess.st, sess.log = st, log
	snap := st.Snapshot
	raw, err := deps.Sidecar.Read(m.runID, SidecarWorkflow)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != snap.WorkflowHash {
		return nil, errs.E(CodeWorkflowChanged, "workflow sidecar does not match the run", "expected", snap.WorkflowHash, "got", got)
	}
	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
	if err != nil {
		return nil, err
	}
	parsed.RepoMode = snap.RepoMode
	w, _, err := parsed.Resolve(snap.Vars, false)
	if err != nil {
		return nil, err
	}
	sess.w = w
	if err := workflow.VerifyCmds(snap.AllowedCmds, snap.WorkDir, deps.FileHash); err != nil {
		return nil, err
	}
	if snap.Mock != "" {
		i := strings.LastIndex(snap.Mock, "#")
		rel, want := snap.Mock[:i], snap.Mock[i+1:]
		h, err := deps.MockLoad(filepath.Join(snap.RepoRoot, rel))
		if err != nil || h[:16] != want {
			return nil, errs.E(CodeMockMismatch, "mock scenario changed or missing", "mock", snap.Mock)
		}
	}
	if deps.Kinds.Mock() != (snap.Mock != "") {
		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match the run")
	}
	sess.git = deps.Git(snap.WorkDir)
	sess.runner = deps.Runner(snap.AllowedCmds, snap.WorkDir, m.runID, sess.audit)
	if w.Convergence != nil {
		sess.pred = converge.MustParse(w.Convergence, sess.runner) // validated by workflow.Parse
	}
	ok = true
	return sess, nil
}

func isStoreCode(err error, code string) bool {
	var se *run.StoreError
	return errors.As(err, &se) && se.Code == code
}

// ---------------------------------------------------------------- session helpers

// stamp builds an event with the machine's stamps (spec 2 §5.4 tail).
func (s *session) stamp(typ string, data any, node string) run.Event {
	snap := s.st.Snapshot
	return run.Event{
		SchemaVersion: run.SchemaVersion, At: s.m.deps.Clock(), Type: typ, State: snap.State, Iter: snap.Iteration,
		Node: node, Mock: snap.Mock != "", Data: run.MarshalCanonical(data),
	}
}

// append stores an event and rebinds the session state.
func (s *session) append(typ string, data any, node string) error {
	return s.appendEvent(s.stamp(typ, data, node))
}

func (s *session) appendEvent(ev run.Event) error {
	st, err := s.m.deps.Store.Append(s.m.runID, s.st, ev)
	if err != nil {
		return err
	}
	s.st = st
	return nil
}

// audit is the closure handed to executors and runners: it stamps and appends.
func (s *session) audit(ev run.Event) error {
	snap := s.st.Snapshot
	ev.SchemaVersion = run.SchemaVersion
	ev.At = s.m.deps.Clock()
	ev.State = snap.State
	ev.Iter = snap.Iteration
	ev.Mock = snap.Mock != ""
	if ev.Type == run.TypeLLMCall {
		if n := s.w.NodeFor(snap.State); n != nil {
			ev.Node = n.Name
		}
	} else {
		ev.Node = ""
	}
	if err := s.appendEvent(ev); err != nil {
		if s.auditErr == nil {
			s.auditErr = err
		}
		return err
	}
	return nil
}

func (s *session) warn(code, detail string) error {
	d, _ := run.CapText(detail, run.MaxText)
	if err := s.append(run.TypeWarn, run.WarnData{Code: code, Detail: d}, ""); err != nil {
		return err
	}
	s.warns = append(s.warns, code+": "+d)
	return nil
}

func (s *session) gateEvent(name string, gerr *run.GateError) error {
	return s.append(run.TypeGate, run.GateData{Name: name, Passed: gerr == nil, Error: gerr}, "")
}

func pseudoGate(name, code, detail string) *run.GateError {
	d, truncated := run.CapDetail(detail)
	return &run.GateError{Code: code, Gate: name, Detail: d, DetailTruncated: truncated}
}

// viewOf computes the read model.
func (s *session) viewOf() View {
	snap := s.st.Snapshot
	v := View{RunID: s.m.runID, Workflow: snap.Workflow, Snapshot: snap, NextAction: NextAdvance, Torn: s.m.torn}
	if snap.Outcome != "" {
		v.NextAction = NextNone
	}
	events := s.log.Events
	if log, err := s.m.deps.Store.Events(s.m.runID); err == nil {
		events = log.Events
	}
	if n := s.w.NodeFor(snap.State); n != nil {
		k := run.Key(n.Name, snap.Iteration)
		_, has := snap.NodeOutputs[k]
		v.Node = &NodeView{Name: n.Name, Kind: n.Kind, Exec: n.Exec, HasOutput: has, Applied: snap.Applied[k]}
		if snap.Outcome == "" && n.Exec != "fork" && !has && hasNeedsInput(events, n.Name, snap.Iteration) {
			v.NextAction = NextRecord
		}
	}
	if snap.Outcome == run.OutcomeFailed {
		v.FailedGate = lastFailedGate(events)
	}
	return v
}

// lastFailedGate finds the last gate{passed:false} before the failed transition.
func lastFailedGate(events []run.Event) *run.GateData {
	var last *run.GateData
	for _, ev := range events {
		if ev.Type != run.TypeGate {
			continue
		}
		var gd run.GateData
		if json.Unmarshal(ev.Data, &gd) == nil && !gd.Passed {
			g := gd
			last = &g
		}
	}
	return last
}

// ---------------------------------------------------------------- Advance

// Advance runs one step (spec 2 §5.4).
func (m *Machine) Advance(ctx context.Context) (AdvanceResult, error) {
	sess, err := m.load(ctx, false)
	if err != nil {
		return AdvanceResult{}, err
	}
	defer sess.unlock()
	defer func() { m.view = sess.viewOf() }()
	if m.torn {
		return AdvanceResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
	}
	return sess.advance()
}

func (s *session) advance() (AdvanceResult, error) {
	snap := s.st.Snapshot
	w := s.w
	// 2. terminal
	if snap.Outcome != "" {
		if snap.Outcome == run.OutcomeOverflow && w.OnOverflow != "" && !snap.OverflowHandled {
			return s.finish(run.TransitionData{To: snap.State, Outcome: snap.Outcome})
		}
		if err := s.terminal(); err != nil {
			return AdvanceResult{}, err
		}
		return AdvanceResult{}, errs.E(CodeRunTerminal, "run is terminal", "outcome", string(snap.Outcome))
	}
	// 4. tree
	head, err := s.git.Head(s.ctx)
	if err != nil {
		return AdvanceResult{}, err
	}
	_, porcelain, err := s.git.Status(s.ctx)
	if err != nil {
		return AdvanceResult{}, err
	}
	wt, err := s.git.WorkTree(s.ctx)
	if err != nil {
		return AdvanceResult{}, err
	}
	h := gate.TreeHash(head, wt)
	node := w.NodeFor(snap.State)
	status, truncated := run.CapDetail(porcelain)
	tree := run.TreeData{Head: head, TreeHash: h, Status: status, StatusTruncated: truncated}
	switch {
	case snap.TreeHash == "":
		if err := s.append(run.TypeTree, tree, ""); err != nil {
			return AdvanceResult{}, err
		}
	case h != snap.TreeHash && (node == nil || node.Kind != "agent-edit"):
		if snap.RepoMode == "enforcing" {
			ge := pseudoGate(GateRepoMode, CodeUnsanctionedEdit, "working tree changed outside an agent-edit node:\n"+porcelain)
			if err := s.gateEvent(GateRepoMode, ge); err != nil {
				return AdvanceResult{}, err
			}
			return s.fail(ge, head)
		}
		if err := s.warn(WarnUnsanctionedEdit, porcelain); err != nil {
			return AdvanceResult{}, err
		}
		if err := s.append(run.TypeTree, tree, ""); err != nil {
			return AdvanceResult{}, err
		}
	case h != snap.TreeHash:
		if err := s.append(run.TypeTree, tree, ""); err != nil {
			return AdvanceResult{}, err
		}
	}
	// 5. node
	if node != nil {
		res, done, err := s.runNode(node, head)
		if done || err != nil {
			return res, err
		}
	}
	// 6. transitions
	return s.transitions(head)
}

// runNode executes or requests the current node's work and applies its delta.
// done reports that advance must return res.
func (s *session) runNode(node *workflow.Node, head string) (AdvanceResult, bool, error) {
	snap := s.st.Snapshot
	kind, _ := s.m.deps.Kinds.Kind(node.Kind)
	k := run.Key(node.Name, snap.Iteration)
	if _, has := snap.NodeOutputs[k]; !has {
		text, truncated, err := s.git.Diff(s.ctx, snap.BaseSHA, head, MaxDiffBytes)
		if err != nil {
			return AdvanceResult{}, true, err
		}
		diff := Diff{Text: text, Truncated: truncated}
		if node.Exec == "fork" {
			ex, _ := s.m.deps.Kinds.Executor(node.Kind)
			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit})
			if err != nil {
				if s.ctx.Err() != nil {
					return AdvanceResult{}, true, err
				}
				if s.auditErr != nil {
					return AdvanceResult{}, true, s.auditErr
				}
				ge := pseudoGate(GateExecutor, CodeExecutorFailed, err.Error())
				if gerr := s.gateEvent(GateExecutor, ge); gerr != nil {
					return AdvanceResult{}, true, gerr
				}
				res, ferr := s.fail(ge, head)
				return res, true, ferr
			}
			if err := s.append(run.TypeNodeOutput, run.NodeOutputData{Output: out}, node.Name); err != nil {
				return AdvanceResult{}, true, err
			}
			snap = s.st.Snapshot
		} else {
			ins, err := kind.Instructions(snap, node, diff, s.m.deps.Nonce())
			if err != nil {
				return AdvanceResult{}, true, errs.Wrap(errs.E(CodeInstructionsFailed, err.Error(), "node", node.Name), err)
			}
			if !s.hasNeedsInput(node.Name, snap.Iteration) {
				if err := s.append(run.TypeNeedsInput, run.EmptyData{}, node.Name); err != nil {
					return AdvanceResult{}, true, err
				}
			}
			ni := &NeedsInput{Node: node.Name, Kind: node.Kind, Exec: node.Exec, Model: node.Model, Effort: node.Effort, Instructions: ins,
				Record: fmt.Sprintf("metareview fsm record node-output --run %s --node %s --data <file>", s.m.runID, node.Name)}
			return AdvanceResult{Status: StatusNeedsInput, From: snap.State, NeedsInput: ni, Warnings: s.warns, Untrusted: untrusted(nil, s.warns, ""), ExitCode: 3, RunID: s.m.runID}, true, nil
		}
	}
	if !snap.Applied[k] {
		out, err := kind.Decode(snap.NodeOutputs[k])
		var delta run.Delta
		if err == nil {
			delta, err = kind.Reduce(snap, out)
		}
		if err == nil {
			err = s.append(run.TypeDeltaApplied, run.DeltaAppliedData{Delta: delta, OutputHash: run.OutputHash(snap.NodeOutputs[k])}, node.Name)
			if err != nil && !isStoreCode(err, run.CodeAppendRejected) {
				return AdvanceResult{}, true, err
			}
		}
		if err != nil {
			ge := pseudoGate(GateNodeOutput, CodeNodeOutputInvalid, err.Error())
			if gerr := s.gateEvent(GateNodeOutput, ge); gerr != nil {
				return AdvanceResult{}, true, gerr
			}
			res, ferr := s.fail(ge, head)
			return res, true, ferr
		}
	}
	return AdvanceResult{}, false, nil
}

func (s *session) hasNeedsInput(node string, iter int) bool {
	return hasNeedsInput(s.log.Events, node, iter)
}

func hasNeedsInput(events []run.Event, node string, iter int) bool {
	for _, ev := range events {
		if ev.Type == run.TypeNeedsInput && ev.Node == node && ev.Iter == iter {
			return true
		}
	}
	return false
}

// transitions evaluates gates (spec 2 §5.4 step 6) and finishes.
func (s *session) transitions(head string) (AdvanceResult, error) {
	snap := s.st.Snapshot
	w := s.w
	var chosen *workflow.Transition
	var failures []*run.GateError
	eval := func(t workflow.Transition) (bool, error) {
		g, _ := gate.Builtin(t.Gate)
		gerr := g(s.ctx, s.st.Snapshot, s.git)
		if err := s.gateEvent(t.Gate, gerr); err != nil {
			return false, err
		}
		if gerr == nil {
			tt := t
			chosen = &tt
			return true, nil
		}
		failures = append(failures, gerr)
		return false, nil
	}
	tt := w.TerminalFor(snap.State)
	if tt != nil {
		if _, err := eval(*tt); err != nil {
			return AdvanceResult{}, err
		}
		if chosen == nil {
			r, err := s.pred.Evaluate(s.ctx, s.st.Snapshot)
			if err != nil && s.auditErr != nil {
				return AdvanceResult{}, s.auditErr // the atom's cmd_call could not be stored: abort, not a gate failure
			}
			if err != nil || (r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
				detail := "convergence evaluation failed"
				reason := "error"
				if err != nil {
					detail = err.Error()
				} else {
					reason = "fixed_class"
					detail = "a convergence atom classed a stop as fixed"
				}
				ge := pseudoGate(GateConverge, CodeConvergeFailed, detail)
				ge.Detail = reason + ": " + ge.Detail
				if gerr := s.gateEvent(GateConverge, ge); gerr != nil {
					return AdvanceResult{}, gerr
				}
				return s.fail(ge, head)
			}
			reason, _ := run.CapText(r.Reason, run.MaxText)
			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
				return AdvanceResult{}, err
			}
			if r.Stop {
				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: r.Atom, Outcome: r.Class}
				return s.finish(s.transitionData(*chosen, head), r.Atom+": "+reason)
			}
		}
		if chosen == nil {
			for _, t := range w.Outgoing(snap.State) {
				if t == *tt {
					continue
				}
				if ok, err := eval(t); err != nil || ok {
					if err != nil {
						return AdvanceResult{}, err
					}
					break
				}
			}
		}
	} else {
		for _, t := range w.Outgoing(snap.State) {
			if ok, err := eval(t); err != nil || ok {
				if err != nil {
					return AdvanceResult{}, err
				}
				break
			}
		}
	}
	if chosen == nil {
		return s.fail(failures[0], head)
	}
	return s.finish(s.transitionData(*chosen, head))
}

func (s *session) transitionData(t workflow.Transition, head string) run.TransitionData {
	td := run.TransitionData{From: t.From, To: t.To, Gate: t.Gate, Outcome: t.Outcome, Loop: t.Loop, Head: head}
	if n := s.w.NodeFor(t.To); n != nil {
		td.ToKind = run.Kind(n.Kind)
	}
	return td
}

// fail appends the failed transition and finishes with GATE_FAILED.
func (s *session) fail(first *run.GateError, head string) (AdvanceResult, error) {
	snap := s.st.Snapshot
	td := run.TransitionData{From: snap.State, To: workflow.FailedState, Gate: first.Gate, Outcome: run.OutcomeFailed, Head: head}
	if err := s.append(run.TypeTransition, td, ""); err != nil {
		return AdvanceResult{}, err
	}
	if err := s.terminal(); err != nil {
		return AdvanceResult{}, err
	}
	gd := &run.GateData{Name: first.Gate, Passed: false, Error: first}
	return AdvanceResult{Status: StatusGateFailed, From: snap.State, To: workflow.FailedState, Gate: gd, Outcome: run.OutcomeFailed,
		Warnings: s.warns, Untrusted: untrusted(gd, s.warns, ""), ExitCode: 1, RunID: s.m.runID}, nil
}

// finish appends the transition (unless resuming), runs the overflow handler,
// calls Terminal, and maps the outcome (spec 2 §5.7).
func (s *session) finish(td run.TransitionData, stopReason ...string) (AdvanceResult, error) {
	snap := s.st.Snapshot
	resuming := snap.Outcome != ""
	if !resuming {
		ev := s.stamp(run.TypeTransition, td, "")
		if td.Loop {
			ev.Iter = snap.Iteration + 1
		}
		if err := s.appendEvent(ev); err != nil {
			return AdvanceResult{}, err
		}
	}
	if td.Outcome == run.OutcomeOverflow && s.w.OnOverflow != "" && !s.st.Snapshot.OverflowHandled {
		if err := s.overflowHandler(); err != nil {
			return AdvanceResult{}, err
		}
	}
	res := AdvanceResult{Status: StatusAdvanced, From: td.From, To: td.To, Outcome: td.Outcome, Warnings: s.warns, RunID: s.m.runID}
	if len(stopReason) > 0 {
		res.StopReason = stopReason[0]
	}
	if resuming {
		res.From, res.StopReason = snap.State, snap.StopReason
	}
	switch td.Outcome {
	case "":
	case run.OutcomeFixed, run.OutcomeClean:
		res.Status = StatusDone
	case run.OutcomeReviewed:
		res.Status, res.ExitCode = StatusDone, 1
	default:
		res.Status, res.ExitCode = StatusStopped, 1
	}
	if td.Outcome != "" {
		if err := s.terminal(); err != nil {
			return AdvanceResult{}, err
		}
	}
	res.Untrusted = untrusted(nil, s.warns, res.StopReason)
	return res, nil
}

func (s *session) overflowHandler() error {
	snap := s.st.Snapshot
	name := s.w.OnOverflow
	payload := converge.Payload(snap)
	sum := sha256.Sum256(payload)
	res, err := s.runner.Run(s.ctx, name, payload)
	if s.auditErr != nil {
		return s.auditErr // the runner's own cmd_call could not be stored: abort, the handler is retried on resume
	}
	var argv []string
	for _, c := range snap.AllowedCmds {
		if c.Name == name {
			argv = c.Argv
		}
	}
	stdout, so := run.CapText(string(res.Stdout), run.MaxDetail)
	stderr, se := run.CapText(string(res.Stderr), run.MaxStderr)
	data := run.OverflowHandlerData{Name: name, Argv: argv, InputHash: hex.EncodeToString(sum[:]), Stdout: stdout, Stderr: stderr,
		StdoutTruncated: so, StderrTruncated: se, ExitCode: res.ExitCode, DurationMS: res.Duration.Milliseconds()}
	if err != nil {
		data.ExitCode = -1
		data.Error = errs.Code(err)
		if data.Error == "" {
			data.Error = "ERR_CMD_FAILED"
		}
	}
	if aerr := s.append(run.TypeOverflowHandler, data, ""); aerr != nil {
		return aerr
	}
	if err != nil || res.ExitCode != 0 {
		detail := fmt.Sprintf("on_overflow %s exited %d", name, res.ExitCode)
		if err != nil {
			detail = "on_overflow " + name + ": " + err.Error()
		}
		return s.warn(WarnOverflowHandlerFailed, detail)
	}
	return nil
}

func (s *session) terminal() error {
	if s.m.deps.Terminal == nil {
		return nil
	}
	return s.m.deps.Terminal(s.ctx, s.viewOf())
}

func untrusted(gd *run.GateData, warns []string, stop string) []string {
	var out []string
	if gd != nil && gd.Error != nil && gd.Error.Detail != "" {
		out = append(out, "gate.detail")
	}
	if len(warns) > 0 {
		out = append(out, "warnings")
	}
	if stop != "" {
		out = append(out, "stop_reason")
	}
	return out
}

// ---------------------------------------------------------------- Record

// Record appends host-supplied events (spec 2 §5.5).
func (m *Machine) Record(ctx context.Context, o RecordOptions) (RecordResult, error) {
	sess, err := m.load(ctx, false)
	if err != nil {
		return RecordResult{}, err
	}
	defer sess.unlock()
	defer func() { m.view = sess.viewOf() }()
	if m.torn {
		return RecordResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
	}
	snap := sess.st.Snapshot
	switch o.Kind {
	case RecordNodeOutput:
		if snap.Outcome != "" {
			return RecordResult{}, errs.E(CodeRunTerminal, "run is terminal", "outcome", string(snap.Outcome))
		}
		node := sess.w.NodeFor(snap.State)
		if node == nil || node.Name != o.Node {
			return RecordResult{}, errs.E(CodeNodeMismatch, "the current state's node is not "+o.Node, "state", string(snap.State), "node", o.Node)
		}
		if node.Exec == "fork" {
			return RecordResult{}, errs.E(CodeNodeNotHost, "node "+node.Name+" is executed by the binary, not the host", "node", node.Name)
		}
		k := run.Key(node.Name, snap.Iteration)
		if snap.Applied[k] {
			return RecordResult{}, errs.E(CodeNodeOutputApplied, "output for "+k+" is already applied", "key", k)
		}
		if _, has := snap.NodeOutputs[k]; has && !o.Replace {
			return RecordResult{}, errs.E(CodeNodeOutputExists, "output for "+k+" exists (use --replace)", "key", k)
		}
		kind, _ := m.deps.Kinds.Kind(node.Kind)
		if _, err := kind.Decode(o.Data); err != nil {
			return RecordResult{}, errs.Wrap(errs.E(CodeNodeOutputInvalid, err.Error(), "key", k), err)
		}
		canon, err := run.Canonical(o.Data)
		if err != nil {
			return RecordResult{}, errs.E(CodeNodeOutputInvalid, err.Error(), "key", k)
		}
		if err := sess.append(run.TypeNodeOutput, run.NodeOutputData{Output: canon}, node.Name); err != nil {
			return RecordResult{}, err
		}
		return RecordResult{Seq: sess.st.Seq, Type: run.TypeNodeOutput, Key: k}, nil
	case RecordTokens:
		var tok run.TokenTotals
		dec := json.NewDecoder(bytes.NewReader(o.Data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&tok); err != nil || tok.Negative() || tok.TooLarge() {
			return RecordResult{}, errs.E(CodeRecordTokens, "tokens must be a {input, cache_read, cache_create, output, reasoning} object of non-negative counters")
		}
		if err := sess.append(run.TypeTokens, tok, ""); err != nil {
			return RecordResult{}, err
		}
		return RecordResult{Seq: sess.st.Seq, Type: run.TypeTokens}, nil
	case RecordEvent:
		switch {
		case !recordName.MatchString(o.Name):
			return RecordResult{}, errs.E(CodeRecordName, "record names match ^[a-z][a-z0-9_-]{0,63}$", "reason", "syntax", "name", o.Name)
		case isEventType(o.Name):
			return RecordResult{}, errs.E(CodeRecordName, o.Name+" is a run event type", "reason", "event_type", "name", o.Name)
		case strings.HasPrefix(o.Name, "mrv_"):
			return RecordResult{}, errs.E(CodeRecordName, "mrv_* names are reserved for the machine", "reason", "reserved", "name", o.Name)
		}
		canon, err := run.Canonical(o.Data)
		if err != nil {
			return RecordResult{}, errs.E(CodeRecordTooLarge, "record data must be valid JSON", "name", o.Name)
		}
		if len(canon) > run.MaxPayload-128 {
			return RecordResult{}, errs.E(CodeRecordTooLarge, fmt.Sprintf("record data exceeds %d bytes", run.MaxPayload-128), "name", o.Name)
		}
		if err := sess.append(run.TypeRecord, run.RecordData{Name: o.Name, Data: canon}, ""); err != nil {
			return RecordResult{}, err
		}
		return RecordResult{Seq: sess.st.Seq, Type: run.TypeRecord, Key: o.Name}, nil
	}
	return RecordResult{}, errs.E(CodeRecordName, "record kind must be node-output, tokens, or event", "reason", "kind", "name", o.Kind)
}

func isEventType(name string) bool {
	i := sort.SearchStrings(sortedEventTypes, name)
	return i < len(sortedEventTypes) && sortedEventTypes[i] == name
}

var sortedEventTypes = func() []string {
	s := append([]string{}, run.EventTypes...)
	sort.Strings(s)
	return s
}()
