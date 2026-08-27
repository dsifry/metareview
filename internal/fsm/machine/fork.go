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
	"sort"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Fork-owned codes (spec 3 §7).
const (
	CodeRunEscalated         = "ERR_RUN_ESCALATED"
	CodeCheckpointNotFound   = "ERR_CHECKPOINT_NOT_FOUND"
	CodeWorkflowIncompatible = "ERR_WORKFLOW_INCOMPATIBLE"
	CodeVarFrozen            = "ERR_VAR_FROZEN"
	CodeTreeNotAtCheckpoint  = "ERR_TREE_NOT_AT_CHECKPOINT"
	CodeCopyInvalid          = "ERR_COPY_INVALID"
	CodeDiffIncompatible     = "ERR_DIFF_INCOMPATIBLE"
)

// MaxAttempts is the lineage-depth cap: the third non-PASS attempt on one branch escalates (spec 3 §6).
const MaxAttempts = 3

// PassOutcome reports whether an outcome maps to PASS in the runs.jsonl verdict map.
func PassOutcome(o run.Outcome) bool { return o == run.OutcomeFixed || o == run.OutcomeClean }

// Attempt is the run's attempt number: 1 for a root, len(Lineage)+1 for a fork.
func Attempt(snap run.Snapshot) int { return len(snap.Lineage) + 1 }

// Escalated reports whether a run's own chained snapshot says it is the escalated leaf of its lineage.
func Escalated(snap run.Snapshot) bool {
	return Attempt(snap) >= MaxAttempts && snap.Outcome != "" && !PassOutcome(snap.Outcome)
}

// ForkOptions parameterizes Fork (spec 3 §2).
type ForkOptions struct {
	From                 run.State
	AtIter               *int
	Vars                 map[string]string
	WorkDir              string
	AcceptWorkflowChange bool
	AllowCustomCmds      string
	WorkflowBytes        []byte
}

// ForkResult reports what Fork produced.
type ForkResult struct {
	ChildRunID  string
	ForkedAtSeq int64
	Copied      int
	CmdsSHA256  string
	DroppedVars []string
}

// Fork creates a child run from a checkpoint of this run (spec 3 §2). The parent's lock is held for steps 0–9.
func (m *Machine) Fork(ctx context.Context, o ForkOptions) (*Machine, ForkResult, error) {
	deps := m.deps
	sess, err := m.load(ctx, false)
	if err != nil {
		return nil, ForkResult{}, err
	}
	defer sess.unlock()
	// 0. preconditions
	if err := ctx.Err(); err != nil {
		return nil, ForkResult{}, err
	}
	if m.torn {
		return nil, ForkResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
	}
	snap := sess.st.Snapshot
	if Escalated(snap) {
		return nil, ForkResult{}, errs.E(CodeRunEscalated, "this run is the escalated leaf of its lineage; a human must narrow, split, or redesign", "run", m.runID, "attempt", fmt.Sprint(Attempt(snap)))
	}
	pw := sess.w // P's resolved workflow — the one that ran
	// 1. checkpoint
	if pw.NodeFor(o.From) == nil && !pw.IsTerminal(o.From) || !hasState(pw, o.From) {
		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "unknown state", "from", string(o.From), "reason", "unknown_state")
	}
	if pw.IsTerminal(o.From) {
		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "cannot fork into a terminal state", "from", string(o.From), "reason", "terminal_state")
	}
	log, lines, err := deps.Store.EventsWithLines(m.runID)
	if err != nil {
		return nil, ForkResult{}, err
	}
	seq, cpIter, ch, found := checkpoint(log.Events, o.From, o.AtIter)
	if !found {
		kv := []string{"from", string(o.From)}
		if o.AtIter != nil {
			kv = append(kv, "at_iter", fmt.Sprint(*o.AtIter))
		}
		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "no transition into that state", kv...)
	}
	// 2. workflow bytes
	raw, err := deps.Sidecar.Read(m.runID, SidecarWorkflow)
	if err != nil {
		return nil, ForkResult{}, err
	}
	source := snap.WorkflowSource
	if o.WorkflowBytes != nil && bytes.Equal(o.WorkflowBytes, raw) {
		o.WorkflowBytes = nil // the same bytes are not a change
	}
	if o.WorkflowBytes != nil {
		sum := sha256.Sum256(o.WorkflowBytes)
		got := hex.EncodeToString(sum[:])
		if !o.AcceptWorkflowChange {
			return nil, ForkResult{}, errs.E(CodeWorkflowChanged, "workflow bytes differ from the run's; pass --accept-workflow-change", "expected", snap.WorkflowHash, "got", got)
		}
		if len(o.WorkflowBytes) > MaxWorkflowBytes {
			return nil, ForkResult{}, errs.E(CodeWorkflowTooLarge, fmt.Sprintf("workflow is %d bytes (max %d)", len(o.WorkflowBytes), MaxWorkflowBytes))
		}
		raw, source = o.WorkflowBytes, "path"
	}
	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
	if err != nil {
		return nil, ForkResult{}, err
	}
	parsed.RepoMode = snap.RepoMode
	copied := log.Events[1:seq]
	if o.WorkflowBytes != nil {
		if err := compatible(pw, parsed, copied, o.From); err != nil {
			return nil, ForkResult{}, err
		}
	}
	// 3. vars
	effective := map[string]string{}
	var dropped []string
	for k, v := range snap.Vars {
		if _, ok := parsed.Vars[k]; ok {
			effective[k] = v
		} else {
			dropped = append(dropped, k)
		}
	}
	sort.Strings(dropped)
	if dropped == nil {
		dropped = []string{}
	}
	if snap.Calibration {
		delete(effective, "JUDGE")
		delete(effective, "JUDGE_EFFORT")
	}
	prefix, _ := run.Fold(log.Events[:seq])
	for _, k := range sortedKeys(o.Vars) {
		if _, ok := parsed.Vars[k]; !ok {
			return nil, ForkResult{}, errs.E(workflow.CodeVarUnknown, "var is not declared by the workflow", "name", k)
		}
		if state, frozen := frozenBy(pw, prefix, copied, k); frozen {
			return nil, ForkResult{}, errs.E(CodeVarFrozen, "var was used by a node that already ran in the copied prefix", "name", k, "state", string(state))
		}
		effective[k] = o.Vars[k]
	}
	w, vars, err := parsed.Resolve(effective, snap.Calibration)
	if err != nil {
		return nil, ForkResult{}, err
	}
	// 4. work dir
	workDir := o.WorkDir
	if workDir == "" {
		workDir = snap.WorkDir
	}
	if !filepath.IsAbs(workDir) {
		return nil, ForkResult{}, errs.E(CodeWorkdirForeign, "work dir must be absolute", "reason", "relative", "work_dir", workDir)
	}
	g := deps.Git(workDir)
	common, err := g.CommonDir(ctx)
	if err != nil {
		return nil, ForkResult{}, err
	}
	rootCommon, err := deps.Git(snap.RepoRoot).CommonDir(ctx)
	if err != nil {
		return nil, ForkResult{}, err
	}
	if common != rootCommon {
		return nil, ForkResult{}, errs.E(CodeWorkdirForeign, "work dir is not a worktree of the repository", "reason", "other_repo", "work_dir", workDir, "repo_root", snap.RepoRoot)
	}
	// 5. commands
	allowed, sha, err := workflow.ResolveCmds(w, workDir, deps.LookPath, deps.FileHash)
	if err != nil {
		return nil, ForkResult{}, err
	}
	if len(allowed) > 0 && sha != snap.CmdsSHA256 && o.AllowCustomCmds != sha {
		return nil, ForkResult{}, errs.E(CodeCmdsNotAllowed, consentList(allowed, workDir), "sha", sha, "cmds_json", string(run.MarshalCanonical(allowed)))
	}
	if allowed == nil {
		allowed = []run.AllowedCmd{}
	}
	// 6. git precondition: HEAD == checkpoint head, every state
	head, err := g.Head(ctx)
	if err != nil {
		return nil, ForkResult{}, err
	}
	if head != ch {
		return nil, ForkResult{}, errs.E(CodeTreeNotAtCheckpoint, "fork first, then commit: git worktree add <dir> "+ch+", fork with --work-dir <dir>, then commit (or git cherry-pick) there", "expected", ch, "got", head)
	}
	_, porcelain, err := g.Status(ctx)
	if err != nil {
		return nil, ForkResult{}, err
	}
	wt, err := g.WorkTree(ctx)
	if err != nil {
		return nil, ForkResult{}, err
	}
	// 7. build the child in memory
	now := deps.Clock()
	childID := run.RunID(w.Name, now.Time)
	var pd run.InitData
	_ = json.Unmarshal(log.Events[0].Data, &pd) // decodable: the fold just validated it
	cd := pd
	cd.RunID, cd.CreatedAt, cd.ParentRunID = childID, now, m.runID
	cd.Lineage = append(append([]string{}, pd.Lineage...), m.runID)
	cd.ForkedAtSeq, cd.WorkDir, cd.Vars = seq, workDir, vars
	cd.AllowedCmds, cd.CmdsSHA256, cd.WorkflowHash, cd.WorkflowSource, cd.Head = allowed, sha, w.Hash, source, ch
	events := []run.Event{{SchemaVersion: run.SchemaVersion, Seq: 1, At: now, Type: run.TypeInit, Data: run.MarshalCanonical(cd)}}
	for _, ev := range copied {
		c := ev
		c.Seq = int64(len(events)) + 1
		c.Prev = ""
		c.Origin = &run.Origin{RunID: m.runID, Seq: ev.Seq, Version: log.Version, Hash: run.LineHash(lines[ev.Seq-1])}
		events = append(events, c)
	}
	mock := snap.Mock != ""
	status, truncated := run.CapDetail(porcelain)
	stampAt := func(typ string, data any) run.Event {
		return run.Event{SchemaVersion: run.SchemaVersion, Seq: int64(len(events)) + 1, At: deps.Clock(), Type: typ, State: o.From, Iter: cpIter, Mock: mock, Data: run.MarshalCanonical(data)}
	}
	events = append(events, stampAt(run.TypeTree, run.TreeData{Head: ch, TreeHash: gate.TreeHash(ch, wt), Status: status, StatusTruncated: truncated}))
	fromNode := w.NodeFor(o.From)
	if fromNode != nil && run.Kind(fromNode.Kind) == run.KindAgentEdit {
		events = append(events, stampAt(run.TypeFixBaseline, run.FixBaselineData{Head: ch}))
	}
	// validate before any I/O: (a) re-Decode copied node outputs, (b) fold, (c) count
	for _, ev := range events[1:] {
		if ev.Type != run.TypeNodeOutput {
			continue
		}
		var nd run.NodeOutputData
		_ = json.Unmarshal(ev.Data, &nd)
		n := w.NodeFor(ev.State)        // non-nil: the compatibility invariant (or the unchanged workflow) guarantees it
		k, _ := deps.Kinds.Kind(n.Kind) // present: Parse validated every kind against the registry
		if _, err := k.Decode(nd.Output); err != nil {
			return nil, ForkResult{}, errs.E(CodeCopyInvalid, "copied node output no longer decodes: "+err.Error(), "seq", fmt.Sprint(ev.Seq), "reason", "decode")
		}
	}
	if err := validateChild(events, deps.Store.MaxEvents()); err != nil {
		return nil, ForkResult{}, err
	}
	// 8. write the child
	st, err := deps.Store.Create(childID, events[0])
	if err != nil {
		return nil, ForkResult{}, err
	}
	if err := deps.Sidecar.Write(childID, SidecarWorkflow, raw); err != nil {
		return nil, ForkResult{}, err
	}
	unlockChild, err := deps.Store.Lock(childID)
	if err != nil {
		return nil, ForkResult{}, err
	}
	for _, ev := range events[1:] {
		if st, err = deps.Store.Append(childID, st, ev); err != nil {
			unlockChild()
			return nil, ForkResult{}, err
		}
	}
	unlockChild()
	// 9. parent
	if err := sess.append(run.TypeFork, run.ForkData{ChildRunID: childID, AtSeq: seq}, ""); err != nil {
		return nil, ForkResult{}, err
	}
	res := ForkResult{ChildRunID: childID, ForkedAtSeq: seq, Copied: len(copied), CmdsSHA256: sha, DroppedVars: dropped}
	// 10. the child machine (P's lock is released by the deferred unlock before the caller advances it)
	child := &Machine{deps: deps, runID: childID}
	csess, err := child.load(ctx, false)
	if err != nil {
		return nil, res, err
	}
	csess.unlock()
	child.view = csess.viewOf()
	return child, res, nil
}

// validateChild is step 7's pre-I/O check: the in-memory log folds and its counted events fit the store's cap.
func validateChild(events []run.Event, maxEvents int) error {
	if _, err := run.FoldFull(events); err != nil {
		return copyInvalid(err)
	}
	counted := 0
	for _, ev := range events {
		if run.Counted(ev.Type) {
			counted++
		}
	}
	if counted > maxEvents {
		return errs.E(CodeCopyInvalid, fmt.Sprintf("child would hold %d counted events (max %d)", counted, maxEvents), "seq", fmt.Sprint(len(events)), "reason", "max_events")
	}
	return nil
}

// copyInvalid maps a fold error of the in-memory child log to ERR_COPY_INVALID{seq, reason}. Every check the fold
// makes is already guaranteed by construction (copies are verbatim, the rebaseline is stamped from the checkpoint), so
// this is defence in depth.
func copyInvalid(err error) error {
	var fe *run.FoldError
	if errors.As(err, &fe) {
		return errs.E(CodeCopyInvalid, "child log does not fold: "+fe.Error(), "seq", fmt.Sprint(fe.Seq), "reason", fe.Reason)
	}
	return errs.E(CodeCopyInvalid, "child log does not fold: "+err.Error(), "reason", "fold")
}

func hasState(w *workflow.Workflow, s run.State) bool {
	for _, st := range w.States {
		if st == s {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkpoint selects the transition into from (spec 3 §2 step 1): search first, restart only when nothing matches.
func checkpoint(events []run.Event, from run.State, atIter *int) (seq int64, iter int, head string, ok bool) {
	for _, ev := range events {
		if ev.Type != run.TypeTransition {
			continue
		}
		var td run.TransitionData
		_ = json.Unmarshal(ev.Data, &td)
		if td.To != from || (atIter != nil && ev.Iter != *atIter) {
			continue
		}
		seq, iter, head, ok = ev.Seq, ev.Iter, td.Head, true
	}
	if ok {
		return seq, iter, head, true
	}
	var id run.InitData
	_ = json.Unmarshal(events[0].Data, &id)
	if from == id.InitialState && (atIter == nil || *atIter == 0) {
		return 1, 0, id.Head, true
	}
	return 0, 0, "", false
}

// compatible enforces the workflow-change invariant (spec 3 §2 step 2).
func compatible(pw, w *workflow.Workflow, copied []run.Event, from run.State) error {
	if w.Name != pw.Name {
		return errs.E(CodeWorkflowIncompatible, "a fork keeps its parent's workflow name", "reason", "name")
	}
	if w.Initial != pw.Initial {
		return errs.E(CodeWorkflowIncompatible, "the initial state may not change", "reason", "initial")
	}
	states := map[run.State]bool{from: true}
	cmds := map[string]bool{}
	for _, ev := range copied {
		if ev.State != "" {
			states[ev.State] = true
		}
		if ev.Type == run.TypeCmdCall { // overflow handlers run post-terminal and are never in a copied prefix
			var cd run.CmdCallData
			_ = json.Unmarshal(ev.Data, &cd)
			cmds[cd.Name] = true
		}
	}
	for _, s := range pw.States {
		if !states[s] {
			continue
		}
		pn, n := pw.NodeFor(s), w.NodeFor(s) // pn != nil: copied states and From are non-terminal in pw
		if !hasState(w, s) || n == nil || pn.Kind != n.Kind || pn.Exec != n.Exec {
			return errs.E(CodeWorkflowIncompatible, "a state in the copied prefix changed or disappeared", "reason", "state", "state", string(s))
		}
	}
	names := make([]string, 0, len(cmds))
	for c := range cmds {
		names = append(names, c)
	}
	sort.Strings(names)
	for _, c := range names {
		if _, ok := w.Cmds[c]; !ok {
			return errs.E(CodeWorkflowIncompatible, "a command that ran in the copied prefix is no longer declared", "reason", "cmd", "cmd", c)
		}
	}
	return nil
}

// frozenBy reports whether var k was consumed in the copied prefix (spec 3 §2 step 3), on P's resolved workflow.
func frozenBy(pw *workflow.Workflow, prefix run.Snapshot, copied []run.Event, k string) (run.State, bool) {
	ran := map[string]bool{}
	for _, n := range prefix.NodesRun {
		ran[n] = true
	}
	for _, s := range pw.States {
		n := pw.NodeFor(s)
		if n == nil || !ran[n.Name] {
			continue
		}
		for _, v := range pw.VarsReferencedBy(s) {
			if v == k {
				return s, true
			}
		}
	}
	for _, ev := range copied {
		if ev.Type != run.TypeCmdCall { // overflow handlers run post-terminal and are never copied
			continue
		}
		var cd run.CmdCallData
		_ = json.Unmarshal(ev.Data, &cd)
		for _, v := range pw.CmdRefs[cd.Name] {
			if v == k {
				return ev.State, true
			}
		}
	}
	return "", false
}

// OriginCheck is one row of VerifyOrigin (spec 3 §3).
type OriginCheck struct {
	Seq    int64
	OK     bool
	Reason string
}

// VerifyOrigin reads the parent once and checks every Origin-bearing child event against it.
func VerifyOrigin(ctx context.Context, store run.RunStore, child run.Log) []OriginCheck {
	out := []OriginCheck{}
	type parent struct {
		log   run.Log
		lines [][]byte
		err   error
	}
	parents := map[string]*parent{}
	for _, ev := range child.Events {
		if ev.Origin == nil {
			continue
		}
		p, ok := parents[ev.Origin.RunID]
		if !ok {
			p = &parent{}
			p.log, p.lines, p.err = store.EventsWithLines(ev.Origin.RunID)
			parents[ev.Origin.RunID] = p
		}
		out = append(out, OriginCheck{Seq: ev.Seq, OK: false, Reason: originReason(ev, p.log, p.lines, p.err)})
		if out[len(out)-1].Reason == "ok" {
			out[len(out)-1].OK = true
		}
	}
	return out
}

func originReason(ev run.Event, plog run.Log, lines [][]byte, perr error) string {
	if perr != nil {
		if isStoreCode(perr, run.CodeRunNotFound) {
			return "parent_missing"
		}
		return "parent_unreadable"
	}
	if ev.Origin.Version != plog.Version {
		return "version_unavailable"
	}
	o := ev.Origin
	if o.Seq < 1 || int(o.Seq) > len(lines) || run.LineHash(lines[o.Seq-1]) != o.Hash {
		return "hash_mismatch"
	}
	pe := plog.Events[o.Seq-1]
	if string(pe.Data) != string(ev.Data) || !pe.At.Equal(ev.At.Time) || pe.State != ev.State || pe.Iter != ev.Iter || pe.Node != ev.Node || pe.Mock != ev.Mock {
		return "content_mismatch"
	}
	return "ok"
}
