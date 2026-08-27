# metareview task-done context

Run ID: `mrv-20260827-104832895746000-task-done-m7-fork-record-export-9dc62119`

## Task

# M7 — fork/resume, diff, export, runs.jsonl record

Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).

Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
`go vet` is clean.


## Git

- Base: `f5e09af38bc15a1ec08cabe14fd069309917ced2`
- Head: `45cb5577d5fdd639ccdbbeaa35501494e32c908b`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `80868`
- Filtered diff bytes: `80868`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `569b9e494e6be744`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md
- docs/tasks/m7-fork-record-export.md
- internal/fsm/machine/diff.go
- internal/fsm/machine/fork.go
- internal/fsm/machine/fork_test.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/sidecar.go

### Shards
- shard-01: docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md, internal/fsm/machine/diff.go, internal/fsm/machine/fork_test.go
- shard-02: docs/tasks/m7-fork-record-export.md, internal/fsm/machine/fork.go, internal/fsm/machine/harness_test.go, internal/fsm/machine/sidecar.go

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02

## Changed Files

- docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md
- internal/fsm/machine/diff.go
- internal/fsm/machine/fork.go
- internal/fsm/machine/fork_test.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/sidecar.go
- docs/tasks/m7-fork-record-export.md

## Diff

````diff
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md
index c48341f..0bcaef7 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md
@@ -41,7 +41,7 @@
 
 ## 1. Packages and owned amendments
 ```
-internal/fsm/machine   fork.go (Fork, VerifyOrigin), diff.go (Diff); load: ERR_FORK_INCOMPLETE check
+internal/fsm/machine   fork.go (Fork, VerifyOrigin), diff.go (DiffRuns); load: ERR_FORK_INCOMPLETE check
 internal/fsm/record    record.go (Terminal, Exists)                — imports machine (View), internal/runchain (Record type); its own writer, no internal/state
 internal/fsm/export    export.go (Export, redaction)               — imports run, machine (types), workflow (parse sidecar)
 internal/fsm/kind      decision.go (Decision)                      — spec 4's package; this spec owns the addition (returns machine.Decision)
@@ -159,7 +159,7 @@ this order: `parent_missing` (`ERR_RUN_NOT_FOUND`) | `parent_unreadable` (any ot
 ## 4. `Diff`
 ```go
 type Decision struct { Raw, Effective *bool }   // declared in machine; produced by kind.Decision
-func Diff(a, b run.Log, decide func(kind string, verdict json.RawMessage) Decision) (Report, error)   // the CLI passes kind.Decision
+func DiffRuns(a, b run.Log, decide func(kind string, verdict json.RawMessage) Decision) (Report, error)   // named DiffRuns: machine.Diff is the git-diff input type; the CLI passes kind.Decision
 type Report struct { A, B string; SameWorkflow bool /* WorkflowHash equal */; CommonPrefixSeq int64; Outcomes [2]run.Outcome; Calls []CallRow; Transitions []TransRow }
 type CallRow struct { Node string; Iter int; Kind, InputHash string; A, B *CallSide; RawSame, DecisionSame, ConfidenceSame, Same bool }
 type CallSide struct { Index int; Model, Effort string; Raw, Effective *bool; Confidence float64; Error string }
diff --git a/internal/fsm/machine/diff.go b/internal/fsm/machine/diff.go
new file mode 100644
index 0000000..05fc825
--- /dev/null
+++ b/internal/fsm/machine/diff.go
@@ -0,0 +1,179 @@
+package machine
+
+import (
+	"encoding/json"
+	"sort"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// Report is the output of Diff (spec 3 §4).
+type Report struct {
+	A               string     `json:"a"`
+	B               string     `json:"b"`
+	SameWorkflow    bool       `json:"same_workflow"`
+	CommonPrefixSeq int64      `json:"common_prefix_seq"`
+	Outcomes        [2]string  `json:"outcomes"`
+	Calls           []CallRow  `json:"calls"`
+	Transitions     []TransRow `json:"transitions"`
+}
+
+// CallRow aligns one judge call across the two runs by (node, iter, kind, input_hash).
+type CallRow struct {
+	Node           string    `json:"node"`
+	Iter           int       `json:"iter"`
+	Kind           string    `json:"kind"`
+	InputHash      string    `json:"input_hash"`
+	A              *CallSide `json:"a"`
+	B              *CallSide `json:"b"`
+	RawSame        bool      `json:"raw_same"`
+	DecisionSame   bool      `json:"decision_same"`
+	ConfidenceSame bool      `json:"confidence_same"`
+	Same           bool      `json:"same"`
+}
+
+// CallSide is one run's side of a CallRow.
+type CallSide struct {
+	Index      int     `json:"index"`
+	Model      string  `json:"model"`
+	Effort     string  `json:"effort"`
+	Raw        *bool   `json:"raw"`
+	Effective  *bool   `json:"effective"`
+	Confidence float64 `json:"confidence"`
+	Error      string  `json:"error,omitempty"`
+}
+
+// TransRow aligns transitions by ordinal.
+type TransRow struct {
+	SeqA    int64       `json:"seq_a"`
+	SeqB    int64       `json:"seq_b"`
+	To      run.State   `json:"to"`
+	Gate    string      `json:"gate"`
+	Outcome run.Outcome `json:"outcome,omitempty"`
+	Same    bool        `json:"same"`
+}
+
+// DiffRuns compares two runs' judge calls and transitions; reasoning never participates (spec 3 §4).
+func DiffRuns(a, b run.Log, decide func(kind string, verdict json.RawMessage) Decision) (Report, error) {
+	sa, err := run.Fold(a.Events)
+	if err != nil {
+		return Report{}, err
+	}
+	sb, err := run.Fold(b.Events)
+	if err != nil {
+		return Report{}, err
+	}
+	if sa.Workflow != sb.Workflow {
+		return Report{}, errs.E(CodeDiffIncompatible, "runs use different workflows", "a", sa.Workflow, "b", sb.Workflow)
+	}
+	r := Report{A: sa.RunID, B: sb.RunID, SameWorkflow: sa.WorkflowHash == sb.WorkflowHash, CommonPrefixSeq: commonPrefix(a.Events, b.Events), Outcomes: [2]string{string(sa.Outcome), string(sb.Outcome)}, Calls: []CallRow{}, Transitions: []TransRow{}}
+	type key struct {
+		node     string
+		iter     int
+		kind, ih string
+	}
+	rows := map[key]*CallRow{}
+	collect := func(events []run.Event, side int) {
+		for _, ev := range events {
+			if ev.Type != run.TypeLLMCall {
+				continue
+			}
+			var d run.LLMCallData
+			_ = json.Unmarshal(ev.Data, &d)
+			k := key{ev.Node, ev.Iter, d.Kind, d.InputHash}
+			row, ok := rows[k]
+			if !ok {
+				row = &CallRow{Node: ev.Node, Iter: ev.Iter, Kind: d.Kind, InputHash: d.InputHash}
+				rows[k] = row
+			}
+			dec := decide(d.Kind, d.Verdict)
+			cs := &CallSide{Index: d.Index, Model: d.Model, Effort: d.Effort, Raw: dec.Raw, Effective: dec.Effective, Confidence: d.Confidence, Error: d.Error}
+			if side == 0 {
+				row.A = cs
+			} else {
+				row.B = cs
+			}
+		}
+	}
+	collect(a.Events, 0)
+	collect(b.Events, 1)
+	for _, row := range rows {
+		if row.A != nil && row.B != nil {
+			row.RawSame = boolEq(row.A.Raw, row.B.Raw)
+			row.DecisionSame = boolEq(row.A.Effective, row.B.Effective)
+			row.ConfidenceSame = row.A.Confidence == row.B.Confidence
+			row.Same = row.DecisionSame && row.ConfidenceSame && row.A.Error == "" && row.B.Error == ""
+		}
+		r.Calls = append(r.Calls, *row)
+	}
+	sort.Slice(r.Calls, func(i, j int) bool {
+		x, y := r.Calls[i], r.Calls[j]
+		if x.Node != y.Node {
+			return x.Node < y.Node
+		}
+		if x.Iter != y.Iter {
+			return x.Iter < y.Iter
+		}
+		if x.Kind != y.Kind {
+			return x.Kind < y.Kind
+		}
+		return x.InputHash < y.InputHash
+	})
+	ta, tb := transitions(a.Events), transitions(b.Events)
+	for i := 0; i < len(ta) || i < len(tb); i++ {
+		row := TransRow{}
+		if i < len(ta) {
+			row.SeqA, row.To, row.Gate, row.Outcome = ta[i].seq, ta[i].d.To, ta[i].d.Gate, ta[i].d.Outcome
+		}
+		if i < len(tb) {
+			row.SeqB = tb[i].seq
+			if i >= len(ta) {
+				row.To, row.Gate, row.Outcome = tb[i].d.To, tb[i].d.Gate, tb[i].d.Outcome
+			}
+		}
+		if i < len(ta) && i < len(tb) {
+			row.Same = ta[i].d.To == tb[i].d.To && ta[i].d.Gate == tb[i].d.Gate && ta[i].d.Outcome == tb[i].d.Outcome
+		}
+		r.Transitions = append(r.Transitions, row)
+	}
+	return r, nil
+}
+
+type trans struct {
+	seq int64
+	d   run.TransitionData
+}
+
+func transitions(events []run.Event) []trans {
+	var out []trans
+	for _, ev := range events {
+		if ev.Type == run.TypeTransition {
+			var d run.TransitionData
+			_ = json.Unmarshal(ev.Data, &d)
+			out = append(out, trans{ev.Seq, d})
+		}
+	}
+	return out
+}
+
+// commonPrefix is the largest n such that events 2..n of both logs have identical (Type, Node, At, Data);
+// Origin, Seq and Prev are ignored. Returns 1 when they diverge at seq 2.
+func commonPrefix(a, b []run.Event) int64 {
+	n := int64(1)
+	for i := 1; i < len(a) && i < len(b); i++ {
+		x, y := a[i], b[i]
+		if x.Type != y.Type || x.Node != y.Node || !x.At.Equal(y.At.Time) || string(x.Data) != string(y.Data) {
+			break
+		}
+		n = int64(i) + 1
+	}
+	return n
+}
+
+func boolEq(a, b *bool) bool {
+	if a == nil || b == nil {
+		return a == nil && b == nil
+	}
+	return *a == *b
+}
diff --git a/internal/fsm/machine/fork.go b/internal/fsm/machine/fork.go
new file mode 100644
index 0000000..9da702c
--- /dev/null
+++ b/internal/fsm/machine/fork.go
@@ -0,0 +1,486 @@
+package machine
+
+import (
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"path/filepath"
+	"sort"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+// Fork-owned codes (spec 3 §7).
+const (
+	CodeRunEscalated         = "ERR_RUN_ESCALATED"
+	CodeCheckpointNotFound   = "ERR_CHECKPOINT_NOT_FOUND"
+	CodeWorkflowIncompatible = "ERR_WORKFLOW_INCOMPATIBLE"
+	CodeVarFrozen            = "ERR_VAR_FROZEN"
+	CodeTreeNotAtCheckpoint  = "ERR_TREE_NOT_AT_CHECKPOINT"
+	CodeCopyInvalid          = "ERR_COPY_INVALID"
+	CodeDiffIncompatible     = "ERR_DIFF_INCOMPATIBLE"
+)
+
+// MaxAttempts is the lineage-depth cap: the third non-PASS attempt on one branch escalates (spec 3 §6).
+const MaxAttempts = 3
+
+// PassOutcome reports whether an outcome maps to PASS in the runs.jsonl verdict map.
+func PassOutcome(o run.Outcome) bool { return o == run.OutcomeFixed || o == run.OutcomeClean }
+
+// Attempt is the run's attempt number: 1 for a root, len(Lineage)+1 for a fork.
+func Attempt(snap run.Snapshot) int { return len(snap.Lineage) + 1 }
+
+// Escalated reports whether a run's own chained snapshot says it is the escalated leaf of its lineage.
+func Escalated(snap run.Snapshot) bool {
+	return Attempt(snap) >= MaxAttempts && snap.Outcome != "" && !PassOutcome(snap.Outcome)
+}
+
+// ForkOptions parameterizes Fork (spec 3 §2).
+type ForkOptions struct {
+	From                 run.State
+	AtIter               *int
+	Vars                 map[string]string
+	WorkDir              string
+	AcceptWorkflowChange bool
+	AllowCustomCmds      string
+	WorkflowBytes        []byte
+}
+
+// ForkResult reports what Fork produced.
+type ForkResult struct {
+	ChildRunID  string
+	ForkedAtSeq int64
+	Copied      int
+	CmdsSHA256  string
+	DroppedVars []string
+}
+
+// Fork creates a child run from a checkpoint of this run (spec 3 §2). The parent's lock is held for steps 0–9.
+func (m *Machine) Fork(ctx context.Context, o ForkOptions) (*Machine, ForkResult, error) {
+	deps := m.deps
+	sess, err := m.load(ctx, false)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	defer sess.unlock()
+	// 0. preconditions
+	if err := ctx.Err(); err != nil {
+		return nil, ForkResult{}, err
+	}
+	if m.torn {
+		return nil, ForkResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
+	}
+	snap := sess.st.Snapshot
+	if Escalated(snap) {
+		return nil, ForkResult{}, errs.E(CodeRunEscalated, "this run is the escalated leaf of its lineage; a human must narrow, split, or redesign", "run", m.runID, "attempt", fmt.Sprint(Attempt(snap)))
+	}
+	pw := sess.w // P's resolved workflow — the one that ran
+	// 1. checkpoint
+	if pw.NodeFor(o.From) == nil && !pw.IsTerminal(o.From) || !hasState(pw, o.From) {
+		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "unknown state", "from", string(o.From), "reason", "unknown_state")
+	}
+	if pw.IsTerminal(o.From) {
+		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "cannot fork into a terminal state", "from", string(o.From), "reason", "terminal_state")
+	}
+	log, lines, err := deps.Store.EventsWithLines(m.runID)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	seq, cpIter, ch, found := checkpoint(log.Events, o.From, o.AtIter)
+	if !found {
+		kv := []string{"from", string(o.From)}
+		if o.AtIter != nil {
+			kv = append(kv, "at_iter", fmt.Sprint(*o.AtIter))
+		}
+		return nil, ForkResult{}, errs.E(CodeCheckpointNotFound, "no transition into that state", kv...)
+	}
+	// 2. workflow bytes
+	raw, err := deps.Sidecar.Read(m.runID, SidecarWorkflow)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	source := snap.WorkflowSource
+	if o.WorkflowBytes != nil {
+		sum := sha256.Sum256(o.WorkflowBytes)
+		got := hex.EncodeToString(sum[:])
+		if !o.AcceptWorkflowChange {
+			return nil, ForkResult{}, errs.E(CodeWorkflowChanged, "workflow bytes differ from the run's; pass --accept-workflow-change", "expected", snap.WorkflowHash, "got", got)
+		}
+		if len(o.WorkflowBytes) > MaxWorkflowBytes {
+			return nil, ForkResult{}, errs.E(CodeWorkflowTooLarge, fmt.Sprintf("workflow is %d bytes (max %d)", len(o.WorkflowBytes), MaxWorkflowBytes))
+		}
+		raw, source = o.WorkflowBytes, "path"
+	}
+	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	parsed.RepoMode = snap.RepoMode
+	copied := log.Events[1:seq]
+	if o.WorkflowBytes != nil {
+		if err := compatible(pw, parsed, copied, o.From); err != nil {
+			return nil, ForkResult{}, err
+		}
+	}
+	// 3. vars
+	effective := map[string]string{}
+	var dropped []string
+	for k, v := range snap.Vars {
+		if _, ok := parsed.Vars[k]; ok {
+			effective[k] = v
+		} else {
+			dropped = append(dropped, k)
+		}
+	}
+	sort.Strings(dropped)
+	if dropped == nil {
+		dropped = []string{}
+	}
+	if snap.Calibration {
+		delete(effective, "JUDGE")
+		delete(effective, "JUDGE_EFFORT")
+	}
+	prefix, _ := run.Fold(log.Events[:seq])
+	for _, k := range sortedKeys(o.Vars) {
+		if _, ok := parsed.Vars[k]; !ok {
+			return nil, ForkResult{}, errs.E(workflow.CodeVarUnknown, "var is not declared by the workflow", "name", k)
+		}
+		if state, frozen := frozenBy(pw, prefix, copied, k); frozen {
+			return nil, ForkResult{}, errs.E(CodeVarFrozen, "var was used by a node that already ran in the copied prefix", "name", k, "state", string(state))
+		}
+		effective[k] = o.Vars[k]
+	}
+	w, vars, err := parsed.Resolve(effective, snap.Calibration)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	// 4. work dir
+	workDir := o.WorkDir
+	if workDir == "" {
+		workDir = snap.WorkDir
+	}
+	if !filepath.IsAbs(workDir) {
+		return nil, ForkResult{}, errs.E(CodeWorkdirForeign, "work dir must be absolute", "reason", "relative", "work_dir", workDir)
+	}
+	g := deps.Git(workDir)
+	common, err := g.CommonDir(ctx)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	rootCommon, err := deps.Git(snap.RepoRoot).CommonDir(ctx)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	if common != rootCommon {
+		return nil, ForkResult{}, errs.E(CodeWorkdirForeign, "work dir is not a worktree of the repository", "reason", "other_repo", "work_dir", workDir, "repo_root", snap.RepoRoot)
+	}
+	// 5. commands
+	allowed, sha, err := workflow.ResolveCmds(w, workDir, deps.LookPath, deps.FileHash)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	if len(allowed) > 0 && sha != snap.CmdsSHA256 && o.AllowCustomCmds != sha {
+		return nil, ForkResult{}, errs.E(CodeCmdsNotAllowed, consentList(allowed, workDir), "sha", sha, "cmds_json", string(run.MarshalCanonical(allowed)))
+	}
+	if allowed == nil {
+		allowed = []run.AllowedCmd{}
+	}
+	// 6. git precondition: HEAD == checkpoint head, every state
+	head, err := g.Head(ctx)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	if head != ch {
+		return nil, ForkResult{}, errs.E(CodeTreeNotAtCheckpoint, "fork first, then commit: git worktree add <dir> "+ch+", fork with --work-dir <dir>, then commit (or git cherry-pick) there", "expected", ch, "got", head)
+	}
+	_, porcelain, err := g.Status(ctx)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	wt, err := g.WorkTree(ctx)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	// 7. build the child in memory
+	now := deps.Clock()
+	childID := run.RunID(w.Name, now.Time)
+	var pd run.InitData
+	_ = json.Unmarshal(log.Events[0].Data, &pd) // decodable: the fold just validated it
+	cd := pd
+	cd.RunID, cd.CreatedAt, cd.ParentRunID = childID, now, m.runID
+	cd.Lineage = append(append([]string{}, pd.Lineage...), m.runID)
+	cd.ForkedAtSeq, cd.WorkDir, cd.Vars = seq, workDir, vars
+	cd.AllowedCmds, cd.CmdsSHA256, cd.WorkflowHash, cd.WorkflowSource, cd.Head = allowed, sha, w.Hash, source, ch
+	events := []run.Event{{SchemaVersion: run.SchemaVersion, Seq: 1, At: now, Type: run.TypeInit, Data: run.MarshalCanonical(cd)}}
+	for _, ev := range copied {
+		c := ev
+		c.Seq = int64(len(events)) + 1
+		c.Prev = ""
+		c.Origin = &run.Origin{RunID: m.runID, Seq: ev.Seq, Version: log.Version, Hash: run.LineHash(lines[ev.Seq-1])}
+		events = append(events, c)
+	}
+	mock := snap.Mock != ""
+	status, truncated := run.CapDetail(porcelain)
+	stampAt := func(typ string, data any) run.Event {
+		return run.Event{SchemaVersion: run.SchemaVersion, Seq: int64(len(events)) + 1, At: deps.Clock(), Type: typ, State: o.From, Iter: cpIter, Mock: mock, Data: run.MarshalCanonical(data)}
+	}
+	events = append(events, stampAt(run.TypeTree, run.TreeData{Head: ch, TreeHash: gate.TreeHash(ch, wt), Status: status, StatusTruncated: truncated}))
+	fromNode := w.NodeFor(o.From)
+	if fromNode != nil && run.Kind(fromNode.Kind) == run.KindAgentEdit {
+		events = append(events, stampAt(run.TypeFixBaseline, run.FixBaselineData{Head: ch}))
+	}
+	// validate before any I/O: (a) re-Decode copied node outputs, (b) fold, (c) count
+	for _, ev := range events[1:] {
+		if ev.Type != run.TypeNodeOutput {
+			continue
+		}
+		var nd run.NodeOutputData
+		_ = json.Unmarshal(ev.Data, &nd)
+		n := w.NodeFor(ev.State)        // non-nil: the compatibility invariant (or the unchanged workflow) guarantees it
+		k, _ := deps.Kinds.Kind(n.Kind) // present: Parse validated every kind against the registry
+		if _, err := k.Decode(nd.Output); err != nil {
+			return nil, ForkResult{}, errs.E(CodeCopyInvalid, "copied node output no longer decodes: "+err.Error(), "seq", fmt.Sprint(ev.Seq), "reason", "decode")
+		}
+	}
+	if err := validateChild(events, deps.Store.MaxEvents()); err != nil {
+		return nil, ForkResult{}, err
+	}
+	// 8. write the child
+	st, err := deps.Store.Create(childID, events[0])
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	if err := deps.Sidecar.Write(childID, SidecarWorkflow, raw); err != nil {
+		return nil, ForkResult{}, err
+	}
+	unlockChild, err := deps.Store.Lock(childID)
+	if err != nil {
+		return nil, ForkResult{}, err
+	}
+	for _, ev := range events[1:] {
+		if st, err = deps.Store.Append(childID, st, ev); err != nil {
+			unlockChild()
+			return nil, ForkResult{}, err
+		}
+	}
+	unlockChild()
+	// 9. parent
+	if err := sess.append(run.TypeFork, run.ForkData{ChildRunID: childID, AtSeq: seq}, ""); err != nil {
+		return nil, ForkResult{}, err
+	}
+	res := ForkResult{ChildRunID: childID, ForkedAtSeq: seq, Copied: len(copied), CmdsSHA256: sha, DroppedVars: dropped}
+	// 10. the child machine (P's lock is released by the deferred unlock before the caller advances it)
+	child := &Machine{deps: deps, runID: childID}
+	csess, err := child.load(ctx, false)
+	if err != nil {
+		return nil, res, err
+	}
+	csess.unlock()
+	child.view = csess.viewOf()
+	return child, res, nil
+}
+
+// validateChild is step 7's pre-I/O check: the in-memory log folds and its counted events fit the store's cap.
+func validateChild(events []run.Event, maxEvents int) error {
+	if _, err := run.FoldFull(events); err != nil {
+		return copyInvalid(err)
+	}
+	counted := 0
+	for _, ev := range events {
+		if run.Counted(ev.Type) {
+			counted++
+		}
+	}
+	if counted > maxEvents {
+		return errs.E(CodeCopyInvalid, fmt.Sprintf("child would hold %d counted events (max %d)", counted, maxEvents), "seq", fmt.Sprint(len(events)), "reason", "max_events")
+	}
+	return nil
+}
+
+// copyInvalid maps a fold error of the in-memory child log to ERR_COPY_INVALID{seq, reason}. Every check the fold
+// makes is already guaranteed by construction (copies are verbatim, the rebaseline is stamped from the checkpoint), so
+// this is defence in depth.
+func copyInvalid(err error) error {
+	var fe *run.FoldError
+	if errors.As(err, &fe) {
+		return errs.E(CodeCopyInvalid, "child log does not fold: "+fe.Error(), "seq", fmt.Sprint(fe.Seq), "reason", fe.Reason)
+	}
+	return errs.E(CodeCopyInvalid, "child log does not fold: "+err.Error(), "reason", "fold")
+}
+
+func hasState(w *workflow.Workflow, s run.State) bool {
+	for _, st := range w.States {
+		if st == s {
+			return true
+		}
+	}
+	return false
+}
+
+func sortedKeys(m map[string]string) []string {
+	keys := make([]string, 0, len(m))
+	for k := range m {
+		keys = append(keys, k)
+	}
+	sort.Strings(keys)
+	return keys
+}
+
+// checkpoint selects the transition into from (spec 3 §2 step 1): search first, restart only when nothing matches.
+func checkpoint(events []run.Event, from run.State, atIter *int) (seq int64, iter int, head string, ok bool) {
+	for _, ev := range events {
+		if ev.Type != run.TypeTransition {
+			continue
+		}
+		var td run.TransitionData
+		_ = json.Unmarshal(ev.Data, &td)
+		if td.To != from || (atIter != nil && ev.Iter != *atIter) {
+			continue
+		}
+		seq, iter, head, ok = ev.Seq, ev.Iter, td.Head, true
+	}
+	if ok {
+		return seq, iter, head, true
+	}
+	var id run.InitData
+	_ = json.Unmarshal(events[0].Data, &id)
+	if from == id.InitialState && (atIter == nil || *atIter == 0) {
+		return 1, 0, id.Head, true
+	}
+	return 0, 0, "", false
+}
+
+// compatible enforces the workflow-change invariant (spec 3 §2 step 2).
+func compatible(pw, w *workflow.Workflow, copied []run.Event, from run.State) error {
+	if w.Name != pw.Name {
+		return errs.E(CodeWorkflowIncompatible, "a fork keeps its parent's workflow name", "reason", "name")
+	}
+	if w.Initial != pw.Initial {
+		return errs.E(CodeWorkflowIncompatible, "the initial state may not change", "reason", "initial")
+	}
+	states := map[run.State]bool{from: true}
+	cmds := map[string]bool{}
+	for _, ev := range copied {
+		if ev.State != "" {
+			states[ev.State] = true
+		}
+		if ev.Type == run.TypeCmdCall { // overflow handlers run post-terminal and are never in a copied prefix
+			var cd run.CmdCallData
+			_ = json.Unmarshal(ev.Data, &cd)
+			cmds[cd.Name] = true
+		}
+	}
+	for _, s := range pw.States {
+		if !states[s] {
+			continue
+		}
+		pn, n := pw.NodeFor(s), w.NodeFor(s) // pn != nil: copied states and From are non-terminal in pw
+		if !hasState(w, s) || n == nil || pn.Kind != n.Kind || pn.Exec != n.Exec {
+			return errs.E(CodeWorkflowIncompatible, "a state in the copied prefix changed or disappeared", "reason", "state", "state", string(s))
+		}
+	}
+	names := make([]string, 0, len(cmds))
+	for c := range cmds {
+		names = append(names, c)
+	}
+	sort.Strings(names)
+	for _, c := range names {
+		if _, ok := w.Cmds[c]; !ok {
+			return errs.E(CodeWorkflowIncompatible, "a command that ran in the copied prefix is no longer declared", "reason", "cmd", "cmd", c)
+		}
+	}
+	return nil
+}
+
+// frozenBy reports whether var k was consumed in the copied prefix (spec 3 §2 step 3), on P's resolved workflow.
+func frozenBy(pw *workflow.Workflow, prefix run.Snapshot, copied []run.Event, k string) (run.State, bool) {
+	ran := map[string]bool{}
+	for _, n := range prefix.NodesRun {
+		ran[n] = true
+	}
+	for _, s := range pw.States {
+		n := pw.NodeFor(s)
+		if n == nil || !ran[n.Name] {
+			continue
+		}
+		for _, v := range pw.VarsReferencedBy(s) {
+			if v == k {
+				return s, true
+			}
+		}
+	}
+	for _, ev := range copied {
+		if ev.Type != run.TypeCmdCall { // overflow handlers run post-terminal and are never copied
+			continue
+		}
+		var cd run.CmdCallData
+		_ = json.Unmarshal(ev.Data, &cd)
+		for _, v := range pw.CmdRefs[cd.Name] {
+			if v == k {
+				return ev.State, true
+			}
+		}
+	}
+	return "", false
+}
+
+// OriginCheck is one row of VerifyOrigin (spec 3 §3).
+type OriginCheck struct {
+	Seq    int64
+	OK     bool
+	Reason string
+}
+
+// VerifyOrigin reads the parent once and checks every Origin-bearing child event against it.
+func VerifyOrigin(ctx context.Context, store run.RunStore, child run.Log) []OriginCheck {
+	out := []OriginCheck{}
+	type parent struct {
+		log   run.Log
+		lines [][]byte
+		err   error
+	}
+	parents := map[string]*parent{}
+	for _, ev := range child.Events {
+		if ev.Origin == nil {
+			continue
+		}
+		p, ok := parents[ev.Origin.RunID]
+		if !ok {
+			p = &parent{}
+			p.log, p.lines, p.err = store.EventsWithLines(ev.Origin.RunID)
+			parents[ev.Origin.RunID] = p
+		}
+		out = append(out, OriginCheck{Seq: ev.Seq, OK: false, Reason: originReason(ev, p.log, p.lines, p.err)})
+		if out[len(out)-1].Reason == "ok" {
+			out[len(out)-1].OK = true
+		}
+	}
+	return out
+}
+
+func originReason(ev run.Event, plog run.Log, lines [][]byte, perr error) string {
+	if perr != nil {
+		if isStoreCode(perr, run.CodeRunNotFound) {
+			return "parent_missing"
+		}
+		return "parent_unreadable"
+	}
+	if ev.Origin.Version != plog.Version {
+		return "version_unavailable"
+	}
+	o := ev.Origin
+	if o.Seq < 1 || int(o.Seq) > len(lines) || run.LineHash(lines[o.Seq-1]) != o.Hash {
+		return "hash_mismatch"
+	}
+	pe := plog.Events[o.Seq-1]
+	if string(pe.Data) != string(ev.Data) || !pe.At.Equal(ev.At.Time) || pe.State != ev.State || pe.Iter != ev.Iter || pe.Node != ev.Node || pe.Mock != ev.Mock {
+		return "content_mismatch"
+	}
+	return "ok"
+}
diff --git a/internal/fsm/machine/fork_test.go b/internal/fsm/machine/fork_test.go
new file mode 100644
index 0000000..a10e15a
--- /dev/null
+++ b/internal/fsm/machine/fork_test.go
@@ -0,0 +1,1110 @@
+package machine
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+	"github.com/dsifry/metareview/workflows"
+)
+
+// sdlcDone drives a one-iteration sdlc run to DONE fixed.
+func sdlcDone(h *harness, o InitOptions) *Machine {
+	h.t.Helper()
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m := h.mustInit(o)
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	h.advance(m) // → adjudicate
+	h.advance(m) // → fix
+	h.advance(m) // needs input
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	h.advance(m) // → verify
+	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeFixed {
+		h.t.Fatalf("sdlcDone: %+v", r)
+	}
+	return m
+}
+
+func seqOfTransitionInto(t *testing.T, evs []run.Event, to run.State, iter int) (int64, string) {
+	t.Helper()
+	for _, ev := range evs {
+		if ev.Type != run.TypeTransition || ev.Iter != iter {
+			continue
+		}
+		var td run.TransitionData
+		_ = json.Unmarshal(ev.Data, &td)
+		if td.To == to {
+			return ev.Seq, td.Head
+		}
+	}
+	t.Fatalf("no transition into %s@%d", to, iter)
+	return 0, ""
+}
+
+func intp(i int) *int { return &i }
+
+func decode[T any](t *testing.T, ev run.Event) T {
+	t.Helper()
+	var d T
+	if err := json.Unmarshal(ev.Data, &d); err != nil {
+		t.Fatal(err)
+	}
+	return d
+}
+
+func TestF1ForkCheckpointAndCopy(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	pevs := h.events(p)
+	praw, _ := h.sidecar.Read(p.runID, SidecarWorkflow)
+	wantSeq, wantHead := seqOfTransitionInto(t, pevs, "adjudicate", 0)
+
+	child, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if res.ForkedAtSeq != wantSeq || res.Copied != int(wantSeq-1) || res.ChildRunID != child.RunID() || res.CmdsSHA256 != "" || len(res.DroppedVars) != 0 {
+		t.Fatalf("result: %+v", res)
+	}
+	cevs := h.events(child)
+	// child init literal
+	cd := decode[run.InitData](t, cevs[0])
+	pd := decode[run.InitData](t, pevs[0])
+	if cd.ParentRunID != p.runID || len(cd.Lineage) != 1 || cd.Lineage[0] != p.runID || cd.ForkedAtSeq != wantSeq || cd.WorkDir != pd.WorkDir || cd.Head != wantHead || cd.Head != shaHead || cd.WorkflowHash != pd.WorkflowHash || cd.WorkflowSource != pd.WorkflowSource || cd.Mock != "" || cd.CmdsSHA256 != "" || cd.Vars["JUDGE"] != "gpt-5.2" || cd.RunID != child.runID {
+		t.Fatalf("child init: %+v", cd)
+	}
+	// copied prefix equals parent [2..seq] modulo origin and prev
+	_, plines, _ := h.store.EventsWithLines(p.runID)
+	for i := int64(2); i <= wantSeq; i++ {
+		pe, ce := pevs[i-1], cevs[i-1]
+		if ce.Origin == nil || ce.Origin.RunID != p.runID || ce.Origin.Seq != pe.Seq || ce.Origin.Version != 1 || ce.Origin.Hash != run.LineHash(plines[i-1]) {
+			t.Fatalf("origin at %d: %+v", i, ce.Origin)
+		}
+		if string(ce.Data) != string(pe.Data) || !ce.At.Equal(pe.At.Time) || ce.State != pe.State || ce.Iter != pe.Iter || ce.Node != pe.Node || ce.Mock != pe.Mock || ce.Type != pe.Type {
+			t.Fatalf("copy at %d differs", i)
+		}
+	}
+	// rebaseline tree with checkpoint stamps, no fix_baseline for adjudicate
+	tree := cevs[wantSeq]
+	if tree.Type != run.TypeTree || tree.State != "adjudicate" || tree.Iter != 0 || tree.Mock || tree.Origin != nil {
+		t.Fatalf("tree stamps: %+v", tree)
+	}
+	td := decode[run.TreeData](t, tree)
+	if td.Head != shaHead || td.TreeHash != gate.TreeHash(shaHead, "t1") {
+		t.Fatalf("tree data: %+v", td)
+	}
+	if int64(len(cevs)) != wantSeq+1 {
+		t.Fatalf("child length %d", len(cevs))
+	}
+	// child sidecar == parent's; child folds and is not incomplete
+	if craw, _ := h.sidecar.Read(child.runID, SidecarWorkflow); string(craw) != string(praw) {
+		t.Fatal("sidecar bytes")
+	}
+	if child.View().Snapshot.State != "adjudicate" || child.View().Snapshot.Head != shaHead {
+		t.Fatalf("child view: %+v", child.View().Snapshot)
+	}
+	// parent: byte-identical plus one fork event
+	pafter := h.events(p)
+	if len(pafter) != len(pevs)+1 {
+		t.Fatalf("parent grew by %d", len(pafter)-len(pevs))
+	}
+	for i := range pevs {
+		if string(run.MarshalCanonical(pafter[i])) != string(run.MarshalCanonical(pevs[i])) {
+			t.Fatalf("parent event %d changed", i+1)
+		}
+	}
+	fd := decode[run.ForkData](t, pafter[len(pafter)-1])
+	if pafter[len(pafter)-1].Type != run.TypeFork || fd.ChildRunID != child.runID || fd.AtSeq != wantSeq {
+		t.Fatalf("fork event: %+v", fd)
+	}
+	// child's first advance re-runs adjudicate from index 0 (the executor is invoked; discover is not)
+	if r := h.advance(child); r.Status != StatusAdvanced || r.To != "fix" {
+		t.Fatalf("child advance: %+v", r)
+	}
+	// fix checkpoint gets a fix_baseline
+	fixSeq, _ := seqOfTransitionInto(t, pevs, "fix", 0)
+	c2, res2, err := p.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil || res2.ForkedAtSeq != fixSeq {
+		t.Fatalf("fork at fix: %v %+v", err, res2)
+	}
+	c2evs := h.events(c2)
+	if c2evs[len(c2evs)-1].Type != run.TypeFixBaseline || c2evs[len(c2evs)-2].Type != run.TypeTree {
+		t.Fatalf("fix baseline order: %v", h.types(c2))
+	}
+	if c2.View().Snapshot.FixEntryHead != shaHead {
+		t.Fatalf("FixEntryHead: %+v", c2.View().Snapshot)
+	}
+	// restart: initial with --at-iter 0 → seq 1, nothing copied
+	c3, res3, err := p.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(0)})
+	if err != nil || res3.ForkedAtSeq != 1 || res3.Copied != 0 {
+		t.Fatalf("restart: %v %+v", err, res3)
+	}
+	if evs := h.events(c3); len(evs) != 2 || evs[1].Type != run.TypeTree || evs[1].State != "discover" {
+		t.Fatalf("restart events: %v", h.types(c3))
+	}
+	// fork of a fork: lineage grows, origin names the immediate parent
+	c4, _, err := c2.Fork(ctx, ForkOptions{From: "adjudicate"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	c4d := decode[run.InitData](t, h.events(c4)[0])
+	if len(c4d.Lineage) != 2 || c4d.Lineage[0] != p.runID || c4d.Lineage[1] != c2.runID || h.events(c4)[1].Origin.RunID != c2.runID {
+		t.Fatalf("lineage: %+v", c4d)
+	}
+	// refusals
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "nope"}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["reason"] != "unknown_state" {
+		t.Fatalf("unknown state: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "done"}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["reason"] != "terminal_state" {
+		t.Fatalf("terminal state: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(5)}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["at_iter"] != "5" {
+		t.Fatalf("at_iter out of range: %v", err)
+	}
+	cctx, cancel := context.WithCancel(ctx)
+	cancel()
+	if _, _, err := p.Fork(cctx, ForkOptions{From: "adjudicate"}); !errors.Is(err, context.Canceled) {
+		t.Fatalf("ctx: %v", err)
+	}
+	h.store.torn = true
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, run.CodeAuditTorn) {
+		t.Fatalf("torn: %v", err)
+	}
+	h.store.torn = false
+}
+
+// twoIterParent builds a sdlc run whose head advances at the iteration-0 fix and loops once.
+func twoIterParent(t *testing.T, h *harness) *Machine {
+	t.Helper()
+	const h1 = "1111111111111111111111111111111111111111"
+	iter := 0
+	h.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0 && i == 0, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	h.git.def.Counts = map[string]int{shaHead + ".." + h1: 1, h1 + ".." + h1: 1}
+	h.git.def.Diffs[shaBase+".."+h1] = "DIFF2"
+	h.git.def.Diffs[h1+".."+h1] = "DIFF2"
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m)
+	h.record(m, "discover", findings(2))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.git.def.HeadSHA = h1 // the fix commit moves HEAD
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	h.advance(m)
+	if r := h.advance(m); r.To != "discover" || m.View().Snapshot.Iteration != 1 {
+		t.Fatalf("loop: %+v", r)
+	}
+	iter = 1
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeFixed {
+		t.Fatalf("two-iter done: %+v", r)
+	}
+	return m
+}
+
+func TestF1TwoIterationCheckpoints(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := twoIterParent(t, h)
+	pevs := h.events(p)
+	const h1 = "1111111111111111111111111111111111111111"
+	adj0, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
+	adj1, _ := seqOfTransitionInto(t, pevs, "adjudicate", 1)
+	disc1, _ := seqOfTransitionInto(t, pevs, "discover", 1)
+	// --from adjudicate --at-iter 0 with HEAD at h1 → refused, expected shaHead (the checkpoint's head, not the latest)
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(0)}); !errs.Is(err, CodeTreeNotAtCheckpoint) || errs.As(err).Fields["expected"] != shaHead || errs.As(err).Fields["got"] != h1 || !strings.Contains(err.Error(), "git worktree add") {
+		t.Fatalf("ch discrimination: %v", err)
+	}
+	// worktree at shaHead accepted; child head/tree.Head == shaHead; not a restart
+	h.git.byDir["/wt0"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: true, Tree: "t0", Refs: map[string]string{"main": shaBase}, Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}
+	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(0), WorkDir: "/wt0"})
+	if err != nil || res.ForkedAtSeq != adj0 || res.Copied == 0 {
+		t.Fatalf("at-iter 0: %v %+v", err, res)
+	}
+	cevs := h.events(c)
+	td := decode[run.TreeData](t, cevs[len(cevs)-1])
+	if td.Head != shaHead || td.TreeHash != gate.TreeHash(shaHead, "t0") || decode[run.InitData](t, cevs[0]).Head != shaHead || decode[run.InitData](t, cevs[0]).WorkDir != "/wt0" {
+		t.Fatalf("child at h0: %+v", td)
+	}
+	before := len(h.reg.execs["match-then-adjudicate"].calls)
+	h.advance(c) // adjudicate re-runs; discover does not
+	if len(h.reg.execs["match-then-adjudicate"].calls) != before+1 {
+		t.Fatal("adjudicate must re-run on the child")
+	}
+	// nil at-iter → the latest re-entry
+	if _, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil || res.ForkedAtSeq != adj1 {
+		t.Fatalf("latest: %v %+v", err, res)
+	}
+	if _, res, err := p.Fork(ctx, ForkOptions{From: "discover"}); err != nil || res.ForkedAtSeq != disc1 {
+		t.Fatalf("initial nil → latest re-entry: %v %+v", err, res)
+	}
+	if _, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(1)}); err != nil || res.ForkedAtSeq != adj1 {
+		t.Fatalf("at-iter 1: %v %+v", err, res)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AtIter: intp(2)}); !errs.Is(err, CodeCheckpointNotFound) || errs.As(err).Fields["at_iter"] != "2" {
+		t.Fatalf("at-iter 2: %v", err)
+	}
+	// restart: expected == InitData.Head
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(0)}); !errs.Is(err, CodeTreeNotAtCheckpoint) || errs.As(err).Fields["expected"] != shaHead {
+		t.Fatalf("restart expected init head: %v", err)
+	}
+}
+
+func TestF2NoCommitRecovery(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	r := h.advance(m)
+	if r.Status != StatusGateFailed || r.Gate.Error.Code != gate.CodeNoCommit {
+		t.Fatalf("expected ERR_NO_COMMIT: %+v", r)
+	}
+	adjCalls := len(h.reg.execs["match-then-adjudicate"].calls)
+	c, _, err := m.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	// copied llm_calls carry Origin; discover/adjudicate are not re-run
+	for _, ev := range h.events(c) {
+		if ev.Type == run.TypeLLMCall && ev.Origin == nil {
+			t.Fatal("copied llm_call without origin")
+		}
+	}
+	if r := h.advance(c); r.Status != StatusNeedsInput || r.NeedsInput.Node != "fix" {
+		t.Fatalf("child re-runs fix: %+v", r)
+	}
+	h.record(c, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	if r := h.advance(c); r.Status != StatusGateFailed || r.Gate.Error.Code != gate.CodeNoCommit {
+		t.Fatalf("negative control: %+v", r)
+	}
+	c2, _, err := m.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h.advance(c2)
+	h.record(c2, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	if r := h.advance(c2); r.To != "verify" {
+		t.Fatalf("commit_exists passes on the child: %+v", r)
+	}
+	if len(h.reg.execs["match-then-adjudicate"].calls) != adjCalls {
+		t.Fatal("adjudicate must not re-run")
+	}
+}
+
+func TestF3JudgeSwapAndFreeze(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	// iter 0 swap at adjudicate is allowed: adjudicate has not run in the copied prefix
+	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if decode[run.InitData](t, h.events(c)[0]).Vars["JUDGE"] != "b" || c.View().Snapshot.Vars["JUDGE"] != "b" {
+		t.Fatal("swapped var must be the child's")
+	}
+	// after adjudicate ran: frozen, first frozen state in States order is adjudicate (verify also references JUDGE)
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "verify", Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "adjudicate" || errs.As(err).Fields["name"] != "JUDGE" {
+		t.Fatalf("frozen: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"REVIEWER": "x"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "discover" {
+		t.Fatalf("reviewer frozen: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"NOPE": "x"}}); !errs.Is(err, workflow.CodeVarUnknown) {
+		t.Fatalf("undeclared: %v", err)
+	}
+	// review-loop: judge swap re-runs adjudicate with the new model
+	h2 := newHarness(t)
+	rl := h2.mustInit(InitOptions{Workflow: "review-loop", Vars: sdlcVars, Base: "main"})
+	h2.advance(rl)
+	h2.record(rl, "discover", findings(1))
+	h2.advance(rl)
+	if r := h2.advance(rl); r.Status != StatusDone {
+		t.Fatalf("review-loop: %+v", r)
+	}
+	rc, _, err := rl.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	h2.reg.execs["match-then-adjudicate"].calls = nil
+	h2.advance(rc)
+	if calls := h2.reg.execs["match-then-adjudicate"].calls; len(calls) != 1 || calls[0].Node.Model != "b" {
+		t.Fatalf("swapped model must reach the executor: %+v", calls)
+	}
+	// calibration parent: no override ok (pins re-applied); override refused
+	h3 := newHarness(t)
+	cal := sdlcDone(h3, InitOptions{Workflow: "sdlc-loop", Base: "main", Calibration: true})
+	if cc, _, err := cal.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil || !cc.View().Snapshot.Calibration || cc.View().Snapshot.Vars["JUDGE"] == "" {
+		t.Fatalf("calibration fork: %v", err)
+	}
+	if _, _, err := cal.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, workflow.CodeCalibrationPinned) {
+		t.Fatalf("calibration override: %v", err)
+	}
+	// convergence cmd freeze: a var referenced only by a cmd that ran is frozen; one whose cmd never ran is not
+	h4 := newHarness(t)
+	wf := sdlcWith(t, h4, "cmdvars.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, $XRAN]}\n  idle: {argv: [bash, -c, $XIDLE]}\non_overflow: idle\nrepo_mode: advisory")
+	h4.files[wf] = []byte(strings.Replace(string(h4.files[wf]), "vars:\n", "vars:\n  XRAN: {default: a}\n  XIDLE: {default: b}\n", 1))
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h4, wf), "/repo", h4.deps.LookPath, h4.deps.FileHash)
+	h4.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	iter := 0
+	h4.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	pm := h4.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h4.advance(pm)
+	h4.record(pm, "discover", findings(1))
+	h4.advance(pm)
+	h4.advance(pm)
+	h4.advance(pm)
+	h4.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h4.advance(pm)
+	if r := h4.advance(pm); r.To != "discover" || countType(h4.events(pm), run.TypeCmdCall) != 1 {
+		t.Fatalf("loop with cmd atom: %+v", r)
+	}
+	if _, _, err := pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XRAN": "z"}, AllowCustomCmds: ""}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "verify" {
+		t.Fatalf("cmd var frozen with the converge state: %v", err)
+	}
+	// XIDLE's cmd never ran → not frozen (consent needed for the changed argv)
+	_, _, err = pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XIDLE": "z"}})
+	if !errs.Is(err, CodeCmdsNotAllowed) || errs.As(err).Fields["cmds_json"] == "" {
+		t.Fatalf("unfrozen var changes the consent sha: %v", err)
+	}
+	newSha := errs.As(err).Fields["sha"]
+	if _, res, err := pm.Fork(ctx, ForkOptions{From: "discover", Vars: map[string]string{"XIDLE": "z"}, AllowCustomCmds: newSha}); err != nil || res.CmdsSHA256 != newSha {
+		t.Fatalf("with consent: %v %+v", err, res)
+	}
+}
+
+func TestF4GitPreconditions(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "rel/dir"}); !errs.Is(err, CodeWorkdirForeign) || errs.As(err).Fields["reason"] != "relative" {
+		t.Fatalf("relative: %v", err)
+	}
+	h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git", Tree: "t9"}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/elsewhere"}); !errs.Is(err, CodeWorkdirForeign) || errs.As(err).Fields["reason"] != "other_repo" {
+		t.Fatalf("foreign: %v", err)
+	}
+	h.git.byDir["/wt"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: false, Porcelain: " M x.go\n", Tree: "t2", Refs: map[string]string{"main": shaBase}, Diffs: map[string]string{shaBase + ".." + shaHead: "DIFF", shaHead + ".." + shaHead: "DIFF"}}
+	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	td := decode[run.TreeData](t, h.events(c)[len(h.events(c))-1])
+	if td.TreeHash != gate.TreeHash(shaHead, "t2") || td.Status != " M x.go\n" || c.View().Snapshot.TreeHash != gate.TreeHash(shaHead, "t2") {
+		t.Fatalf("worktree baseline: %+v", td)
+	}
+	// git failures pass through
+	for _, method := range []string{"CommonDir", "Head", "Status", "WorkTree"} {
+		h.git.byDir["/wt"] = &failingGit{Git: h.git.def, at: method, err: errors.New("git broke " + method)}
+		_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"})
+		if err == nil || !strings.Contains(err.Error(), "git broke") {
+			t.Fatalf("%s: %v", method, err)
+		}
+	}
+	delete(h.git.byDir, "/wt")
+	// mock parent forks as mock and the child open re-verifies the scenario
+	hm := newHarness(t)
+	hm.reg.mock = true
+	hm.mockHash["/repo/mock"] = strings.Repeat("m", 16)
+	pm := sdlcDone(hm, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main", MockDir: "mock"})
+	before := 0
+	loads := &before
+	orig := hm.deps.MockLoad
+	hm.deps.MockLoad = func(d string) (string, error) { *loads++; return orig(d) }
+	pm.deps.MockLoad = hm.deps.MockLoad
+	cm, _, err := pm.Fork(ctx, ForkOptions{From: "adjudicate"})
+	if err != nil || cm.View().Snapshot.Mock != pm.View().Snapshot.Mock || *loads == 0 {
+		t.Fatalf("mock fork: %v", err)
+	}
+	if evs := hm.events(cm); !evs[len(evs)-1].Mock {
+		t.Fatal("the fork's own tree must carry the mock stamp")
+	}
+	hm.reg.mock = false
+	if _, _, err := pm.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("registry mismatch: %v", err)
+	}
+	// fork after fix in the same iteration: FixEntryHead is empty and commit_exists fails closed
+	h5 := newHarness(t)
+	p5 := sdlcDone(h5, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	c5, _, err := p5.Fork(ctx, ForkOptions{From: "verify"})
+	if err != nil || c5.View().Snapshot.FixEntryHead != "" {
+		t.Fatalf("verify fork: %v %+v", err, c5.View().Snapshot)
+	}
+	sn := c5.View().Snapshot
+	ce, _ := gate.Builtin("commit_exists")
+	if gerr := ce(ctx, sn, h5.git.def); gerr == nil || gerr.Code != gate.CodeGateInapplicable {
+		t.Fatalf("commit_exists must fail closed: %+v", gerr)
+	}
+}
+
+func TestF5WorkflowChangeAndCopyValidation(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	raw, _ := workflows.Read("sdlc-loop")
+	changed := []byte(strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkflowBytes: changed}); !errs.Is(err, CodeWorkflowChanged) {
+		t.Fatalf("without the flag: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(strings.Repeat("#", MaxWorkflowBytes+1))}); !errs.Is(err, CodeWorkflowTooLarge) {
+		t.Fatalf("too large: %v", err)
+	}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte("workflow: [")}); !errs.Is(err, workflow.CodeWorkflowInvalid) {
+		t.Fatalf("invalid: %v", err)
+	}
+	incompatible := map[string]string{
+		"name":    strings.Replace(string(raw), "workflow: sdlc-loop", "workflow: other-loop", 1),
+		"initial": strings.Replace(string(raw), "states: [discover, adjudicate, fix, verify, done, failed]", "states: [adjudicate, discover, fix, verify, done, failed]", 1),
+		"state":   strings.Replace(string(raw), "adjudicate: {kind: match-then-adjudicate, exec: fork,", "adjudicate: {kind: still-present, exec: fork,", 1),
+	}
+	for reason, body := range incompatible {
+		_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(body)})
+		if !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != reason {
+			t.Fatalf("%s: %v", reason, err)
+		}
+	}
+	// changed exec on a copied state
+	execChanged := strings.Replace(string(raw), "discover:   {kind: review-lenses,        exec: subagent,", "discover:   {kind: review-lenses,        exec: inline,", 1)
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(execChanged)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["state"] != "discover" {
+		t.Fatalf("exec: %v", err)
+	}
+	// no child directory after any refusal
+	if list, _ := h.store.List(); len(list) != 1 {
+		t.Fatalf("refusals must not create runs: %d", len(list))
+	}
+	// positive controls: changed gate/model on a not-yet-run state, dropped var → accepted, source path
+	accepted := strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1)
+	accepted = strings.Replace(accepted, "  REV_EFFORT:   {default: low}\n", "", 1)
+	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(accepted)})
+	if err != nil {
+		t.Fatal(err)
+	}
+	cd := decode[run.InitData](t, h.events(c)[0])
+	if cd.WorkflowSource != "path" || len(res.DroppedVars) != 1 || res.DroppedVars[0] != "REV_EFFORT" || cd.WorkflowHash == p.View().Snapshot.WorkflowHash {
+		t.Fatalf("accepted change: %+v %+v", res, cd)
+	}
+	if craw, _ := h.sidecar.Read(c.runID, SidecarWorkflow); string(craw) != accepted {
+		t.Fatal("child sidecar must be the new bytes")
+	}
+	// freeze rule evaluated on P's resolved workflow, not the new one
+	noJudge := strings.Replace(string(raw), "adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE,", "adjudicate: {kind: match-then-adjudicate, exec: fork,     model: fixed-model,", 1)
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "verify", AcceptWorkflowChange: true, WorkflowBytes: []byte(noJudge), Vars: map[string]string{"JUDGE": "b"}}); !errs.Is(err, CodeVarFrozen) || errs.As(err).Fields["state"] != "adjudicate" {
+		t.Fatalf("freeze on Pw: %v", err)
+	}
+	// tightened Decode → ERR_COPY_INVALID{decode}, no child dir
+	h.reg.kinds["review-lenses"].decode = func(json.RawMessage) (any, error) { return nil, errors.New("tightened") }
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["reason"] != "decode" {
+		t.Fatalf("decode: %v", err)
+	}
+	h.reg.kinds["review-lenses"].decode = nil
+	// max_events: the fork's count check, boundary exact
+	counted := 0
+	pevs := h.events(p)
+	adj, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
+	for _, ev := range pevs[:adj] {
+		if run.Counted(ev.Type) {
+			counted++
+		}
+	}
+	h.store.maxEvents = counted + 1 // parent prefix + the child's own tree
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err != nil {
+		t.Fatalf("boundary accepted: %v", err)
+	}
+	h.store.maxEvents = counted
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["reason"] != "max_events" {
+		t.Fatalf("max_events: %v", err)
+	}
+	h.store.maxEvents = 0
+	if list, _ := h.store.List(); len(list) != 3 {
+		t.Fatalf("only accepted forks create runs: %d", len(list))
+	}
+}
+
+func TestF5Escalation(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	failAtFix := func(m *Machine) {
+		h.git.def.Counts = map[string]int{}
+		h.advance(m)
+		if r := h.advance(m); r.Status == StatusNeedsInput {
+			h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+			h.advance(m)
+		}
+		if m.View().Snapshot.Outcome != run.OutcomeFailed {
+			t.Fatalf("expected failed: %+v", m.View().Snapshot)
+		}
+	}
+	p := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	h.advance(p)
+	h.record(p, "discover", findings(1))
+	h.advance(p)
+	h.advance(p)
+	failAtFix(p) // attempt 1, failed
+	c1, _, err := p.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	failAtFix(c1) // attempt 2, failed — still forkable
+	c2, _, err := c1.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	failAtFix(c2) // attempt 3, failed → escalated leaf
+	if _, _, err := c2.Fork(ctx, ForkOptions{From: "fix"}); !errs.Is(err, CodeRunEscalated) || errs.As(err).Fields["attempt"] != "3" {
+		t.Fatalf("escalated: %v", err)
+	}
+	if _, _, err := c1.Fork(ctx, ForkOptions{From: "fix"}); err != nil {
+		t.Fatalf("non-escalated parent still forkable: %v", err)
+	}
+	// a PASS attempt-3 leaf is forkable; its non-PASS child (attempt 4) escalates
+	c2b, _, err := c1.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	h.advance(c2b)
+	h.record(c2b, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(c2b)
+	if r := h.advance(c2b); r.Outcome != run.OutcomeFixed {
+		t.Fatalf("pass leaf: %+v", r)
+	}
+	c3, _, err := c2b.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatalf("pass leaf forkable: %v", err)
+	}
+	failAtFix(c3)
+	if _, _, err := c3.Fork(ctx, ForkOptions{From: "fix"}); !errs.Is(err, CodeRunEscalated) || errs.As(err).Fields["attempt"] != "4" {
+		t.Fatalf("attempt 4: %v", err)
+	}
+	// a non-terminal attempt-3 parent is forkable
+	c3n, _, err := c2b.Fork(ctx, ForkOptions{From: "fix"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if _, _, err := c3n.Fork(ctx, ForkOptions{From: "fix"}); err != nil {
+		t.Fatalf("non-terminal attempt-3: %v", err)
+	}
+}
+
+func TestF6VerifyOrigin(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	c, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	clog, _ := h.store.Events(c.runID)
+	h.store.events = 0
+	checks := VerifyOrigin(ctx, h.store, clog)
+	if len(checks) == 0 || h.store.events != 1 {
+		t.Fatalf("parent read once: %d reads, %d checks", h.store.events, len(checks))
+	}
+	for _, ck := range checks {
+		if !ck.OK || ck.Reason != "ok" {
+			t.Fatalf("expected ok: %+v", ck)
+		}
+	}
+	mutate := func(f func(ev *run.Event)) run.Log {
+		l := run.Log{Events: append([]run.Event{}, clog.Events...)}
+		o := *l.Events[1].Origin
+		l.Events[1].Origin = &o
+		f(&l.Events[1])
+		return l
+	}
+	cases := []struct {
+		name, want string
+		f          func(ev *run.Event)
+	}{
+		{"parent_missing", "parent_missing", func(ev *run.Event) { ev.Origin.RunID = "mrv-missing-parent-1" }},
+		{"version_unavailable", "version_unavailable", func(ev *run.Event) { ev.Origin.Version = 2 }},
+		{"hash_mismatch", "hash_mismatch", func(ev *run.Event) { ev.Origin.Hash = "0000" }},
+		{"hash seq out of range", "hash_mismatch", func(ev *run.Event) { ev.Origin.Seq = 999 }},
+		{"content_mismatch", "content_mismatch", func(ev *run.Event) { ev.Data = json.RawMessage(`{"x":1}`) }},
+		{"precedence version before hash", "version_unavailable", func(ev *run.Event) { ev.Origin.Version = 2; ev.Origin.Hash = "0000"; ev.Data = json.RawMessage(`{}`) }},
+	}
+	for _, cs := range cases {
+		got := VerifyOrigin(ctx, h.store, mutate(cs.f))
+		if got[0].OK || got[0].Reason != cs.want || got[0].Seq != 2 {
+			t.Fatalf("%s: %+v", cs.name, got[0])
+		}
+	}
+	h.store.failOp, h.store.err = "Events", errors.New("disk")
+	if got := VerifyOrigin(ctx, h.store, clog); got[0].Reason != "parent_unreadable" {
+		t.Fatalf("unreadable: %+v", got[0])
+	}
+	h.store.failOp = ""
+	if got := VerifyOrigin(ctx, h.store, run.Log{Events: clog.Events[:1]}); len(got) != 0 {
+		t.Fatal("no origins → no checks")
+	}
+}
+
+func TestF7DiffRuns(t *testing.T) {
+	h := newHarness(t)
+	ctx := context.Background()
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	c, res, err := p.Fork(ctx, ForkOptions{From: "adjudicate", Vars: map[string]string{"JUDGE": "b"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	// the child's adjudicate rejects finding 0 with a lower confidence and different verdict
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var bugs []run.Bug
+		for i, f := range in.Snap.Findings {
+			if err := llmCall(in, i, 10); err != nil {
+				return nil, err
+			}
+			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
+	}
+	h.advance(c)
+	plog, _ := h.store.Events(p.runID)
+	clog, _ := h.store.Events(c.runID)
+	decide := func(kind string, v json.RawMessage) Decision {
+		var d struct {
+			IsReal     *bool   `json:"is_real"`
+			Confidence float64 `json:"confidence"`
+		}
+		_ = json.Unmarshal(v, &d)
+		if d.IsReal == nil {
+			return Decision{}
+		}
+		eff := *d.IsReal && d.Confidence >= 0.7
+		return Decision{Raw: d.IsReal, Effective: &eff}
+	}
+	r, err := DiffRuns(plog, clog, decide)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if r.A != p.runID || r.B != c.runID || !r.SameWorkflow || r.CommonPrefixSeq != res.ForkedAtSeq || r.Outcomes[0] != "fixed" || r.Outcomes[1] != "" {
+		t.Fatalf("report: %+v", r)
+	}
+	if len(r.Calls) == 0 || len(r.Transitions) == 0 {
+		t.Fatalf("rows: %+v", r)
+	}
+	// determinism and sort order
+	r2, _ := DiffRuns(plog, clog, decide)
+	if string(run.MarshalCanonical(r)) != string(run.MarshalCanonical(r2)) {
+		t.Fatal("deterministic")
+	}
+	for i := 1; i < len(r.Calls); i++ {
+		x, y := r.Calls[i-1], r.Calls[i]
+		if x.Node > y.Node || (x.Node == y.Node && x.Iter > y.Iter) || (x.Node == y.Node && x.Iter == y.Iter && x.Kind > y.Kind) || (x.Node == y.Node && x.Iter == y.Iter && x.Kind == y.Kind && x.InputHash >= y.InputHash) {
+			t.Fatalf("sort order at %d", i)
+		}
+	}
+	// hand-built logs pin the row semantics
+	mk := func(id string, calls []run.LLMCallData, ats []int) run.Log {
+		b := run.NewBuilder(id)
+		d := run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}}
+		b.Init(d)
+		for i, c := range calls {
+			b.Event(run.TypeLLMCall, c, run.WithNode("n"), run.WithIter(ats[i]), run.WithState("s"))
+		}
+		return run.Log{Events: b.Events()}
+	}
+	bp := func(v bool) *bool { return &v }
+	raw := func(real bool, conf float64) json.RawMessage {
+		return json.RawMessage(run.MarshalCanonical(map[string]any{"is_real": real, "confidence": conf}))
+	}
+	a := mk("mrv-diff-a-0000001", []run.LLMCallData{
+		{Kind: "adjudicate", Model: "m1", Index: 0, InputHash: "i1", Verdict: raw(true, 0.9), Confidence: 0.9},
+		{Kind: "adjudicate", Model: "m1", Index: 1, InputHash: "i2", Verdict: raw(true, 0.9), Confidence: 0.9},
+		{Kind: "adjudicate", Model: "m1", Index: 2, InputHash: "i3", Verdict: json.RawMessage(`null`)},
+		{Kind: "adjudicate", Model: "m1", Index: 3, InputHash: "i4", Verdict: raw(true, 0.9), Confidence: 0.9, Error: "ERR_X"},
+		{Kind: "adjudicate", Model: "m1", Index: 4, InputHash: "i5", Verdict: raw(true, 0.9), Confidence: 0.9},
+	}, []int{0, 0, 0, 0, 0})
+	b := mk("mrv-diff-b-0000001", []run.LLMCallData{
+		{Kind: "adjudicate", Model: "m2", Index: 0, InputHash: "i2", Verdict: raw(true, 0.8), Confidence: 0.8},                 // decision same, confidence differs
+		{Kind: "adjudicate", Model: "m2", Index: 1, InputHash: "i3", Verdict: raw(false, 0.5), Confidence: 0.5},                // nil vs false
+		{Kind: "adjudicate", Model: "m2", Index: 2, InputHash: "i4", Verdict: raw(true, 0.9), Confidence: 0.9, Error: "ERR_X"}, // identical errors never Same
+		{Kind: "adjudicate", Model: "m2", Index: 3, InputHash: "i9", Verdict: raw(true, 0.9), Confidence: 0.9},                 // one-sided
+		{Kind: "adjudicate", Model: "m2", Index: 4, InputHash: "i1", Verdict: raw(true, 0.6), Confidence: 0.6},                 // raw same, effective flips; aligned by hash
+	}, []int{0, 0, 0, 0, 0})
+	rep, err := DiffRuns(a, b, decide)
+	if err != nil {
+		t.Fatal(err)
+	}
+	byHash := map[string]CallRow{}
+	for _, row := range rep.Calls {
+		byHash[row.InputHash] = row
+	}
+	if r1 := byHash["i1"]; !r1.RawSame || r1.DecisionSame || r1.ConfidenceSame || r1.Same || r1.A.Index != 0 || r1.B.Index != 4 {
+		t.Fatalf("i1: %+v", r1)
+	}
+	if r2 := byHash["i2"]; !r2.RawSame || !r2.DecisionSame || r2.ConfidenceSame || r2.Same {
+		t.Fatalf("i2: %+v", r2)
+	}
+	if r3 := byHash["i3"]; r3.RawSame || r3.DecisionSame || r3.A.Raw != nil || *r3.B.Raw {
+		t.Fatalf("i3: %+v", r3)
+	}
+	if r4 := byHash["i4"]; !r4.DecisionSame || !r4.ConfidenceSame || r4.Same {
+		t.Fatalf("i4 identical errors: %+v", r4)
+	}
+	if r5, r9 := byHash["i5"], byHash["i9"]; r5.B != nil || r9.A != nil || r5.Same || r9.Same {
+		t.Fatalf("one-sided: %+v %+v", r5, r9)
+	}
+	_ = bp
+	if rep.CommonPrefixSeq != 1 {
+		t.Fatalf("diverge at seq 2: %d", rep.CommonPrefixSeq)
+	}
+	// nil vs nil is DecisionSame; a type-equal data-equal event with a different At does not extend the prefix
+	a2 := mk("mrv-diff-a-0000002", []run.LLMCallData{{Kind: "adjudicate", InputHash: "i3", Verdict: json.RawMessage(`null`)}}, []int{0})
+	b2 := mk("mrv-diff-b-0000002", []run.LLMCallData{{Kind: "adjudicate", InputHash: "i3", Verdict: json.RawMessage(`null`)}}, []int{0})
+	rep2, _ := DiffRuns(a2, b2, decide)
+	if !rep2.Calls[0].DecisionSame || !rep2.Calls[0].RawSame || rep2.CommonPrefixSeq != 2 {
+		t.Fatalf("nil/nil: %+v", rep2)
+	}
+	// incompatible workflows; fold errors
+	other := mk("mrv-diff-c-0000001", nil, nil)
+	var od run.InitData
+	_ = json.Unmarshal(other.Events[0].Data, &od)
+	od.Workflow = "different"
+	other.Events[0].Data = run.MarshalCanonical(od)
+	if _, err := DiffRuns(a, other, decide); !errs.Is(err, CodeDiffIncompatible) {
+		t.Fatalf("incompatible: %v", err)
+	}
+	bad := run.Log{Events: []run.Event{{Type: "bogus"}}}
+	if _, err := DiffRuns(bad, a, decide); err == nil {
+		t.Fatal("fold error a")
+	}
+	if _, err := DiffRuns(a, bad, decide); err == nil {
+		t.Fatal("fold error b")
+	}
+	// transitions: same / diverging / one-sided
+	tr := func(id string, tos []run.State) run.Log {
+		b := run.NewBuilder(id)
+		b.Init(run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
+		from := run.State("s")
+		for _, to := range tos {
+			b.Event(run.TypeTransition, run.TransitionData{From: from, To: to, Gate: "g", Head: shaHead}, run.WithState(from))
+			from = to
+		}
+		return run.Log{Events: b.Events()}
+	}
+	ta := tr("mrv-diff-t-0000001", []run.State{"x", "y"})
+	tb := tr("mrv-diff-t-0000002", []run.State{"x", "z", "q"})
+	rt, err := DiffRuns(ta, tb, decide)
+	if err != nil || len(rt.Transitions) != 3 || !rt.Transitions[0].Same || rt.Transitions[1].Same || rt.Transitions[2].Same || rt.Transitions[2].To != "q" || rt.Transitions[2].SeqA != 0 {
+		t.Fatalf("transitions: %v %+v", err, rt.Transitions)
+	}
+}
+
+func TestF10ForkFailureSweeps(t *testing.T) {
+	ctx := context.Background()
+	snapshotOf := func(h *harness, m *Machine) string {
+		var b strings.Builder
+		for _, ev := range h.events(m) {
+			b.Write(run.MarshalCanonical(ev))
+		}
+		return b.String()
+	}
+	// every refusal in steps 0–7 leaves P byte-identical and creates no child
+	h := newHarness(t)
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	before := snapshotOf(h, p)
+	refusals := []ForkOptions{
+		{From: "nope"}, {From: "done"}, {From: "adjudicate", AtIter: intp(7)},
+		{From: "adjudicate", WorkDir: "rel"}, {From: "verify", Vars: map[string]string{"JUDGE": "b"}},
+		{From: "adjudicate", Vars: map[string]string{"NOPE": "b"}}, {From: "adjudicate", WorkflowBytes: []byte("x")},
+	}
+	for _, o := range refusals {
+		if _, _, err := p.Fork(ctx, o); err == nil {
+			t.Fatalf("expected refusal for %+v", o)
+		}
+	}
+	if snapshotOf(h, p) != before {
+		t.Fatal("parent changed on refusal")
+	}
+	if list, _ := h.store.List(); len(list) != 1 {
+		t.Fatal("child created on refusal")
+	}
+	// step-8 seam failures: sidecar write, child create, child lock, each append; P stays byte-identical
+	h.sidecar.WriteErr = errors.New("sidecar write failed")
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "sidecar write failed") {
+		t.Fatalf("sidecar: %v", err)
+	}
+	h.sidecar.WriteErr = nil
+	if snapshotOf(h, p) != before {
+		t.Fatal("parent changed on sidecar failure")
+	}
+	h.store.failOp, h.store.err = "Create", errors.New("create failed")
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "create failed") {
+		t.Fatalf("create: %v", err)
+	}
+	h.store.failOp = ""
+	pevs := h.events(p)
+	adj, _ := seqOfTransitionInto(t, pevs, "adjudicate", 0)
+	h.store.failLockRun = "child"
+	h.store.err = errors.New("child lock failed")
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "child lock failed") {
+		t.Fatalf("child lock: %v", err)
+	}
+	h.store.failLockRun = ""
+	// an append failure mid-copy leaves an incomplete child and P untouched (no fork event)
+	h.store.appends = 0
+	h.store.failAt, h.store.err = 2, errors.New("append failed")
+	_, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"})
+	if err == nil || !strings.Contains(err.Error(), "append failed") {
+		t.Fatalf("append: %v", err)
+	}
+	h.store.failAt = 0
+	if snapshotOf(h, p) != before {
+		t.Fatal("parent changed on append failure")
+	}
+	list, _ := h.store.List()
+	var incomplete string
+	for _, s := range list {
+		if s.RunID != p.runID {
+			incomplete = s.RunID
+			if !strings.Contains(s.Error, "incomplete fork") {
+				t.Fatalf("incomplete child must be flagged: %+v", s)
+			}
+		}
+	}
+	if _, err := Open(ctx, h.deps, incomplete, OpenOptions{}); !errs.Is(err, CodeForkIncomplete) {
+		t.Fatalf("open incomplete: %v", err)
+	}
+	// a failure at the parent's fork append: child complete, P without the fork event
+	h.store.failType, h.store.err = run.TypeFork, errors.New("fork append failed")
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "fork append failed") {
+		t.Fatalf("fork append: %v", err)
+	}
+	h.store.failType = ""
+	if snapshotOf(h, p) != before {
+		t.Fatal("parent changed")
+	}
+	// the child machine's own load failure is returned with the result
+	h.store.failEvAt, h.store.err = 3, errors.New("events failed")
+	h.store.events = 0
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "events failed") {
+		t.Fatalf("child load: %v", err)
+	}
+	h.store.failEvAt = 0
+	// parent lock / events failures
+	h.store.failOp = "Lock"
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil {
+		t.Fatal("parent lock")
+	}
+	h.store.failOp = ""
+	h.store.failEvAt, h.store.events = 2, 0
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil {
+		t.Fatal("second events read")
+	}
+	h.store.failEvAt = 0
+	// ordinal continuity: copied cmd_calls count toward the child's ordinals
+	h4 := newHarness(t)
+	wf := sdlcWith(t, h4, "ord.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h4, wf), "/repo", h4.deps.LookPath, h4.deps.FileHash)
+	h4.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	iter := 0
+	h4.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	pm := h4.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h4.advance(pm)
+	h4.record(pm, "discover", findings(1))
+	h4.advance(pm)
+	h4.advance(pm)
+	h4.advance(pm)
+	h4.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h4.advance(pm)
+	h4.advance(pm) // loop: cmd_call #1
+	c, _, err := pm.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(1)})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := h4.runner.ordinal("chk"); got != 1 {
+		t.Fatalf("copied cmd_call must count: ordinal %d", got)
+	}
+	_ = c
+	_ = adj
+}
+
+func TestF7DiffSortAndPrefixEdges(t *testing.T) {
+	decide := func(kind string, v json.RawMessage) Decision { return Decision{} }
+	mk := func(id string, calls []run.LLMCallData, nodes []string) run.Log {
+		b := run.NewBuilder(id)
+		b.Init(run.InitData{RunID: id, Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
+		for i, c := range calls {
+			b.Event(run.TypeLLMCall, c, run.WithNode(nodes[i]), run.WithState("s"))
+		}
+		return run.Log{Events: b.Events()}
+	}
+	// rows differing only in kind, then only in input hash, then node: literal order on out-of-order insertion
+	a := mk("mrv-diff-s-0000001", []run.LLMCallData{
+		{Kind: "still-present", Index: 0, InputHash: "z"},
+		{Kind: "adjudicate", Index: 1, InputHash: "b"},
+		{Kind: "adjudicate", Index: 2, InputHash: "a"},
+		{Kind: "adjudicate", Index: 0, InputHash: "a"},
+	}, []string{"n", "n", "n", "m"})
+	rep, err := DiffRuns(a, mk("mrv-diff-s-0000002", nil, nil), decide)
+	if err != nil {
+		t.Fatal(err)
+	}
+	got := ""
+	for _, r := range rep.Calls {
+		got += r.Node + "/" + r.Kind + "/" + r.InputHash + " "
+	}
+	if got != "m/adjudicate/a n/adjudicate/a n/adjudicate/b n/still-present/z " {
+		t.Fatalf("order: %s", got)
+	}
+	// iteration ordering through a loop transition
+	b := run.NewBuilder("mrv-diff-s-0000003")
+	b.Init(run.InitData{RunID: "mrv-diff-s-0000003", Workflow: "w", WorkflowHash: "h", Vars: map[string]string{}, RepoMode: "advisory", AllowedCmds: []run.AllowedCmd{}, RepoRoot: "/r", WorkDir: "/r", BaseSHA: shaBase, Head: shaHead, InitialState: "s", Lineage: []string{}, Goldens: []run.Golden{}})
+	b.Event(run.TypeTransition, run.TransitionData{From: "s", To: "s", Gate: "g", Head: shaHead, Loop: true}, run.WithState("s"))
+	b.Event(run.TypeLLMCall, run.LLMCallData{Kind: "adjudicate", Index: 0, InputHash: "a"}, run.WithNode("n"), run.WithState("s"), run.WithIter(1))
+	looped := run.Log{Events: b.Events()}
+	rep2, err := DiffRuns(looped, a, decide)
+	if err != nil || rep2.Calls[len(rep2.Calls)-1].Iter != 1 || rep2.Calls[len(rep2.Calls)-1].Node != "n" {
+		t.Fatalf("iter order: %v %+v", err, rep2.Calls)
+	}
+}
+
+func TestF10ForkSeamErrors(t *testing.T) {
+	ctx := context.Background()
+	h := newHarness(t)
+	p := sdlcDone(h, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	// sidecar read failure (after load verified it once)
+	reads := 0
+	p.deps.Sidecar = &failingSidecar{Sidecar: h.sidecar, readErr: errors.New("sidecar read failed"), after: &reads, failAt: 2}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "sidecar read failed") {
+		t.Fatalf("sidecar read: %v", err)
+	}
+	p.deps.Sidecar = h.sidecar
+	// repo root's CommonDir fails while the work dir's succeeds
+	h.git.byDir["/wt"] = &gate.Fake{HeadSHA: shaHead, Common: "/repo/.git", Clean: true, Tree: "t2"}
+	h.git.byDir["/repo"] = &failingGit{Git: h.git.def, at: "CommonDir", err: errors.New("root common failed")}
+	if _, _, err := p.Fork(ctx, ForkOptions{From: "adjudicate", WorkDir: "/wt"}); err == nil || !strings.Contains(err.Error(), "root common failed") {
+		t.Fatalf("root common: %v", err)
+	}
+	delete(h.git.byDir, "/repo")
+	delete(h.git.byDir, "/wt")
+	// ResolveCmds failure on a workflow with commands
+	h2 := newHarness(t)
+	wf := sdlcWith(t, h2, "cmds.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, h2, wf), "/repo", h2.deps.LookPath, h2.deps.FileHash)
+	p2 := sdlcDone(h2, InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	p2.deps.LookPath = func(string) (string, error) { return "", errors.New("lookpath failed") }
+	if _, _, err := p2.Fork(ctx, ForkOptions{From: "adjudicate"}); err == nil || !strings.Contains(err.Error(), "bash not found") {
+		t.Fatalf("resolve cmds: %v", err)
+	}
+	p2.deps.LookPath = h2.deps.LookPath
+	// a workflow change that drops a command whose cmd_call was copied → reason cmd (needs a copied cmd_call: loop once)
+	raw := h2.files[wf]
+	noCmd := strings.Replace(string(raw), "  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\n", "  any: [no_fixation_progress, {max_iterations: 5}]\n", 1)
+	h3 := newHarness(t)
+	wf3 := sdlcWith(t, h3, "cmds3.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [no_fixation_progress, {cmd: chk}, {max_iterations: 5}]\ncmds:\n  chk: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, sha3, _ := workflow.ResolveCmds(mustResolve(t, h3, wf3), "/repo", h3.deps.LookPath, h3.deps.FileHash)
+	h3.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	iter := 0
+	h3.reg.execs["still-present"].fn = func(in ExecInput) (json.RawMessage, error) {
+		var st []run.BugStatus
+		for i, b := range in.Snap.AllFound {
+			if err := llmCall(in, i, 5); err != nil {
+				return nil, err
+			}
+			st = append(st, run.BugStatus{ID: b.ID, StillPresent: iter == 0, Confidence: 1})
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{Status: st})), nil
+	}
+	pm := h3.mustInit(InitOptions{Workflow: wf3, Vars: sdlcVars, AllowCustomCmds: sha3})
+	h3.advance(pm)
+	h3.record(pm, "discover", findings(1))
+	h3.advance(pm)
+	h3.advance(pm)
+	h3.advance(pm)
+	h3.record(pm, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h3.advance(pm)
+	h3.advance(pm) // loop → cmd_call
+	if _, _, err := pm.Fork(ctx, ForkOptions{From: "discover", AtIter: intp(1), AcceptWorkflowChange: true, WorkflowBytes: []byte(noCmd)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "cmd" {
+		t.Fatalf("removed cmd: %v", err)
+	}
+	// a renamed state: From no longer exists → reason state
+	h4 := newHarness(t)
+	p4 := sdlcDone(h4, InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "main"})
+	base, _ := workflows.Read("sdlc-loop")
+	renamed := string(base)
+	for _, r := range [][2]string{{"from: adjudicate", "from: review"}, {"to: adjudicate", "to: review"}, {"adjudicate: {kind", "review: {kind"}, {"[discover, adjudicate,", "[discover, review,"}} {
+		renamed = strings.ReplaceAll(renamed, r[0], r[1])
+	}
+	if _, _, err := p4.Fork(ctx, ForkOptions{From: "adjudicate", AcceptWorkflowChange: true, WorkflowBytes: []byte(renamed)}); !errs.Is(err, CodeWorkflowIncompatible) || errs.As(err).Fields["reason"] != "state" || errs.As(err).Fields["state"] != "adjudicate" {
+		t.Fatalf("renamed state: %v", err)
+	}
+	// validateChild: a log that does not fold is ERR_COPY_INVALID with the fold's seq and reason
+	if err := validateChild([]run.Event{{Type: "bogus"}}, 100); !errs.Is(err, CodeCopyInvalid) || errs.As(err).Fields["seq"] == "" {
+		t.Fatalf("validateChild fold: %v", err)
+	}
+	// copyInvalid maps fold errors and plain errors
+	if e := errs.As(copyInvalid(&run.FoldError{Code: run.CodeAuditInvalid, Reason: "x", Seq: 4})); e.Code != CodeCopyInvalid || e.Fields["seq"] != "4" || e.Fields["reason"] != "x" {
+		t.Fatalf("copyInvalid fold: %+v", e)
+	}
+	if e := errs.As(copyInvalid(errors.New("plain"))); e.Code != CodeCopyInvalid || e.Fields["reason"] != "fold" {
+		t.Fatalf("copyInvalid plain: %+v", e)
+	}
+}
+
+type failingSidecar struct {
+	Sidecar
+	readErr error
+	after   *int
+	failAt  int
+}
+
+func (f *failingSidecar) Read(runID, name string) ([]byte, error) {
+	*f.after++
+	if *f.after == f.failAt {
+		return nil, f.readErr
+	}
+	return f.Sidecar.Read(runID, name)
+}
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
index 346c2c9..1dc6c65 100644
--- a/internal/fsm/machine/harness_test.go
+++ b/internal/fsm/machine/harness_test.go
@@ -213,14 +213,31 @@ func (f *failingGit) CommonDir(ctx context.Context) (string, error) {
 // countingStore fails the Nth append (1-based) or a named method.
 type countingStore struct {
 	run.RunStore
-	mu       sync.Mutex
-	appends  int
-	failAt   int
-	failType string
-	failOp   string
-	events   int
-	failEvAt int // fail the Nth EventsWithLines call
-	err      error
+	mu          sync.Mutex
+	appends     int
+	failAt      int
+	failType    string
+	failOp      string
+	events      int
+	failEvAt    int // fail the Nth EventsWithLines call
+	err         error
+	failLockRun string // "child": fail Lock for any run other than the first locked one
+	firstLock   string
+	maxEvents   int  // overrides MaxEvents() when non-zero (the fork's in-memory count check)
+	torn        bool // report a torn tail on every read (the memory store is never torn)
+}
+
+func (c *countingStore) MaxEvents() int {
+	if c.maxEvents != 0 {
+		return c.maxEvents
+	}
+	return c.RunStore.MaxEvents()
+}
+func (c *countingStore) Create(id string, first run.Event) (run.FoldState, error) {
+	if c.failOp == "Create" {
+		return run.FoldState{}, c.err
+	}
+	return c.RunStore.Create(id, first)
 }
 
 func (c *countingStore) Append(id string, st run.FoldState, ev run.Event) (run.FoldState, error) {
@@ -237,6 +254,12 @@ func (c *countingStore) Lock(id string) (func(), error) {
 	if c.failOp == "Lock" {
 		return nil, c.err
 	}
+	if c.firstLock == "" {
+		c.firstLock = id
+	}
+	if c.failLockRun == "child" && id != c.firstLock {
+		return nil, c.err
+	}
 	return c.RunStore.Lock(id)
 }
 func (c *countingStore) EventsWithLines(id string) (run.Log, [][]byte, error) {
@@ -247,7 +270,11 @@ func (c *countingStore) EventsWithLines(id string) (run.Log, [][]byte, error) {
 	if c.failOp == "Events" || (c.failEvAt != 0 && n == c.failEvAt) {
 		return run.Log{}, nil, c.err
 	}
-	return c.RunStore.EventsWithLines(id)
+	log, lines, err := c.RunStore.EventsWithLines(id)
+	if c.torn && err == nil {
+		log.Torn = &run.TornTail{Offset: 1, Bytes: []byte("{")}
+	}
+	return log, lines, err
 }
 func (c *countingStore) RepairTail(id string) error {
 	if c.failOp == "Repair" {
diff --git a/internal/fsm/machine/sidecar.go b/internal/fsm/machine/sidecar.go
index fc0bd52..8f88be3 100644
--- a/internal/fsm/machine/sidecar.go
+++ b/internal/fsm/machine/sidecar.go
@@ -125,6 +125,8 @@ func (f FSSidecar) List(runID string) ([]string, error) {
 type MemSidecar struct {
 	mu    sync.Mutex
 	files map[string][]byte
+	// WriteErr, when set, is returned by Write (a test seam for the fork's step-8 sweep).
+	WriteErr error
 }
 
 func (m *MemSidecar) key(runID, name string) string { return runID + "/" + name }
@@ -134,6 +136,9 @@ func (m *MemSidecar) Write(runID, name string, b []byte) error {
 	if err := checkSidecarArgs(runID, name); err != nil {
 		return err
 	}
+	if m.WriteErr != nil {
+		return m.WriteErr
+	}
 	m.mu.Lock()
 	defer m.mu.Unlock()
 	if m.files == nil {


--- docs/tasks/m7-fork-record-export.md
+# M7 — fork/resume, diff, export, runs.jsonl record
+
+Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
+`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
+`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
+torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).
+
+Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
+`go vet` is clean.
````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence


> metareview@0.8.2 prepack
> npm run build


> metareview@0.8.2 build
> go build -o bin/metareview ./cmd/metareview

cmd/metareview                                      80.4%  ok
internal/artifactreview                             80.4%  ok
internal/contextpack                                76.1%  ok
internal/contextprofile                             84.6%  ok
internal/epicready                                  81.6%  ok
internal/epicsource                                 83.1%  ok
internal/evidence                                   85.2%  ok
internal/findings                                   90.5%  ok
internal/fsm/cmdexec                               100.0%  ok
internal/fsm/converge                              100.0%  ok
internal/fsm/errs                                  100.0%  ok
internal/fsm/export                                100.0%  ok
internal/fsm/gate                                  100.0%  ok
internal/fsm/judge                                 100.0%  ok
internal/fsm/kind                                  100.0%  ok
internal/fsm/machine                               100.0%  ok
internal/fsm/mockai                                100.0%  ok
internal/fsm/record                                100.0%  ok
internal/fsm/run                                   100.0%  ok
internal/fsm/workflow                              100.0%  ok
internal/gitcontext                                 83.7%  ok
internal/githubcontext                              95.9%  ok
internal/integration                               100.0%  ok
internal/knowledge                                  77.8%  ok
internal/learning                                   88.0%  ok
internal/learnsource                                70.8%  ok
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed
coverage exit=0

