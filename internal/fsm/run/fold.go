package run

import (
	"bytes"
	"encoding/json"
)

// FoldState is the accumulator Apply threads through a log. It embeds the derived Snapshot plus the
// bookkeeping the snapshot does not expose (§4).
type FoldState struct {
	Snapshot
	// ChainHead is the LineHash of the last stored line. It is maintained by the store, never by Apply.
	ChainHead string

	indexes      map[string]int // node@iter → next llm_call index
	prevType     string
	prevTreeHead string
}

// Apply folds one event into st and returns the next state. It is pure: st is never mutated, and on
// error the returned state is the zero value. Errors are always *FoldError.
func Apply(st FoldState, ev Event) (FoldState, error) {
	if ev.SchemaVersion != SchemaVersion {
		return FoldState{}, foldErr(ReasonVersion, ev)
	}
	if st.Seq == 0 {
		if ev.Type != TypeInit {
			return FoldState{}, foldErr(ReasonFirstNotInit, ev)
		}
	} else if ev.Type == TypeInit {
		return FoldState{}, foldErr(ReasonSecondInit, ev)
	}
	if ev.Seq != st.Seq+1 {
		return FoldState{}, foldErr(ReasonSeqGap, ev)
	}
	if !knownType(ev.Type) {
		return FoldState{}, foldErr(ReasonUnknownType, ev)
	}
	canon, err := Canonical(ev.Data)
	if err != nil {
		return FoldState{}, foldErr(ReasonBadPayload, ev)
	}
	if len(canon) > MaxPayload || canonLenStr(ev.Type) > MaxShort || canonLenStr(ev.Node) > MaxShort || canonLenStr(string(ev.State)) > MaxShort {
		return FoldState{}, foldErr(ReasonOversize, ev)
	}
	payload, err := decodePayload(ev.Type, canon)
	if err != nil {
		return FoldState{}, foldErr(ReasonBadPayload, ev)
	}
	if !withinCaps(payload) {
		return FoldState{}, foldErr(ReasonOversize, ev)
	}
	if nodeScoped(ev.Type) && ev.Node == "" {
		return FoldState{}, foldErr(ReasonNodeScope, ev)
	}

	next := st.cow()
	if ev.Type == TypeInit {
		if ev.State != "" || ev.Iter != 0 || ev.At.IsZero() {
			return FoldState{}, foldErr(ReasonInitStamp, ev)
		}
		next.applyInit(payload.(*InitData))
		next.Seq = ev.Seq
		next.prevType = ev.Type
		return next, nil
	}

	// stamps (§4.9)
	expectIter := st.Iteration
	if td, ok := payload.(*TransitionData); ok {
		if td.Loop {
			expectIter = st.Iteration + 1
		}
		if td.From != st.State {
			return FoldState{}, foldErr(ReasonStamp, ev)
		}
	}
	if ev.Iter != expectIter || ev.State != st.State || ev.At.IsZero() {
		return FoldState{}, foldErr(ReasonStamp, ev)
	}
	// provenance (§4.8)
	if reason := checkProvenance(st, ev); reason != "" {
		return FoldState{}, foldErr(reason, ev)
	}
	if st.Mock != "" && ev.Origin == nil && !ev.Mock {
		return FoldState{}, foldErr(ReasonMockStamp, ev)
	}
	if st.Mock == "" && ev.Mock {
		next.MockTainted = true
	}
	// terminal rule (§4.7)
	if st.Outcome != "" && !postTerminalAllowed(ev.Type) {
		return FoldState{}, foldErr(ReasonPostTerminal, ev)
	}

	k := Key(ev.Node, ev.Iter)
	switch p := payload.(type) {
	case *TreeData:
		next.Head, next.TreeHash, next.TreeStatus = p.Head, p.TreeHash, p.Status
		next.prevTreeHead = p.Head
	case *EmptyData, *RecordData, *ForkData:
		// no change
	case *NodeOutputData:
		if st.Applied[k] {
			return FoldState{}, foldErr(ReasonOutputAfterDelta, ev)
		}
		// The error is not discardable: the "sub-document of canon, hence valid"
		// assumption fails for a payload with no output at all. Canonical(nil)
		// errors, and storing the empty result left a present key holding an
		// invalid document — which the matching delta_applied then accepted,
		// since OutputHash(nil) is the sha256 of the empty string. The fold has
		// to refuse it here or the corruption is not caught until kind.Decode.
		out, cerr := Canonical(p.Output)
		if cerr != nil {
			return FoldState{}, foldErr(ReasonBadPayload, ev)
		}
		next.NodeOutputs[k] = out
		if !containsString(next.NodesRun, ev.Node) {
			next.NodesRun = append(next.NodesRun, ev.Node)
		}
	case *DeltaAppliedData:
		recorded, ok := st.NodeOutputs[k]
		if !ok {
			return FoldState{}, foldErr(ReasonDeltaWithoutOutput, ev)
		}
		if st.Applied[k] {
			return FoldState{}, foldErr(ReasonSecondDelta, ev)
		}
		if p.OutputHash != OutputHash(recorded) {
			return FoldState{}, foldErr(ReasonOutputHash, ev)
		}
		if p.Findings != nil {
			next.Findings = p.Findings
		}
		if p.Confirmed != nil {
			next.Confirmed = p.Confirmed
			seen := idSet(next.AllFound)
			for _, b := range p.Confirmed {
				if _, dup := seen[b.ID]; !dup {
					next.AllFound = append(next.AllFound, b)
					seen[b.ID] = struct{}{}
				}
			}
		}
		if p.PinResults != nil {
			next.Unproven = foldUnproven(next.Unproven, p.PinResults)
		}
		if p.Pins != nil {
			// The fix node's claims carry forward to verify. Replaced rather than appended: each
			// fix round makes its own claims, and a stale pin from an earlier iteration would be
			// verified against a tree it no longer describes.
			next.Pins = append([]Pin{}, p.Pins...)
		}
		if p.Status != nil {
			if reason := checkStatus(next.AllFound, p.Status); reason != "" {
				return FoldState{}, foldErr(reason, ev)
			}
			next.Status = p.Status
		}
		next.Unfixed = countUnfixed(next.AllFound, next.Status)
		next.Applied[k] = true
	case *LLMCallData:
		if p.Index != st.indexes[k] {
			return FoldState{}, foldErr(ReasonStamp, ev)
		}
		if p.Tokens.Negative() {
			return FoldState{}, foldErr(ReasonTokensNegative, ev)
		}
		if p.Tokens.TooLarge() {
			return FoldState{}, foldErr(ReasonTokensTooLarge, ev)
		}
		next.indexes[k] = p.Index + 1
		next.Tokens = next.Tokens.Add(p.Tokens)
	case *TokenTotals:
		if p.Negative() {
			return FoldState{}, foldErr(ReasonTokensNegative, ev)
		}
		if p.TooLarge() {
			return FoldState{}, foldErr(ReasonTokensTooLarge, ev)
		}
		next.Tokens = next.Tokens.Add(*p)
	case *CmdCallData:
		if !sanctioned(st.AllowedCmds, p.Name) {
			return FoldState{}, foldErr(ReasonUnsanctionedCmd, ev)
		}
	case *OverflowHandlerData:
		if !sanctioned(st.AllowedCmds, p.Name) {
			return FoldState{}, foldErr(ReasonUnsanctionedCmd, ev)
		}
		next.OverflowHandled = true
	case *GateData:
		if !p.Passed {
			if p.Error == nil {
				return FoldState{}, foldErr(ReasonBadPayload, ev)
			}
			e := *p.Error
			next.LastError = &e
		}
	case *ConvergeData:
		if p.Stop {
			next.StopReason = p.Atom
		}
	case *WarnData:
		if len(st.Warnings) >= MaxWarnings {
			return FoldState{}, foldErr(ReasonOversize, ev)
		}
		next.Warnings = append(next.Warnings, p.Code)
	case *TransitionData:
		if p.Outcome != "" && !validOutcome(p.Outcome) {
			return FoldState{}, foldErr(ReasonBadOutcome, ev)
		}
		next.State, next.StateKind, next.Head, next.LastError = p.To, p.ToKind, p.Head, nil
		if p.Loop {
			next.Iteration = st.Iteration + 1
			v := st.Unfixed
			next.PrevUnfixed = &v
			next.Findings = []Finding{}
			next.Confirmed = []Bug{}
		}
		if p.ToKind == KindAgentEdit && ev.Origin == nil {
			next.FixEntryHead = p.Head
		}
		if p.Outcome != "" {
			next.Outcome = p.Outcome
		}
	case *FixBaselineData:
		if st.StateKind != KindAgentEdit {
			return FoldState{}, foldErr(ReasonFixBaselineKind, ev)
		}
		if p.Head != st.Head {
			return FoldState{}, foldErr(ReasonFixBaselineHead, ev)
		}
		if st.prevType != TypeTree || st.prevTreeHead != p.Head {
			return FoldState{}, foldErr(ReasonFixBaselineOrder, ev)
		}
		next.FixEntryHead = p.Head
	}
	next.Seq = ev.Seq
	next.prevType = ev.Type
	return next, nil
}

// Fold derives the snapshot of a whole log.
func Fold(events []Event) (Snapshot, error) {
	st, err := FoldFull(events)
	if err != nil {
		return Snapshot{}, err
	}
	return st.Snapshot, nil
}

// FoldFull is reduce(Apply) from the zero state. ChainHead is left empty; the store fills it.
func FoldFull(events []Event) (FoldState, error) {
	if len(events) == 0 {
		return FoldState{}, &FoldError{Code: CodeAuditEmpty, Reason: ReasonEmpty}
	}
	var st FoldState
	for _, ev := range events {
		next, err := Apply(st, ev)
		if err != nil {
			return FoldState{}, err
		}
		st = next
	}
	return st, nil
}

// cow returns a state whose containers are fresh so that mutations never reach st. RawMessage
// values (node outputs) are shared: they are never mutated in place.
// NextIndex returns the llm_call index the next call under key must carry.
func (st FoldState) NextIndex(key string) int { return st.indexes[key] }

func (st FoldState) cow() FoldState {
	next := st
	next.Vars = cloneStringMap(st.Vars)
	next.Lineage = cloneStrings(st.Lineage)
	next.AllowedCmds = append([]AllowedCmd{}, st.AllowedCmds...)
	next.Goldens = append([]Golden{}, st.Goldens...)
	next.Findings = append([]Finding{}, st.Findings...)
	next.Confirmed = append([]Bug{}, st.Confirmed...)
	next.AllFound = append([]Bug{}, st.AllFound...)
	next.Status = append([]BugStatus{}, st.Status...)
	next.PrevUnfixed = cloneInt(st.PrevUnfixed)
	next.NodeOutputs = make(map[string]json.RawMessage, len(st.NodeOutputs)+1)
	for k, v := range st.NodeOutputs {
		next.NodeOutputs[k] = v
	}
	next.Applied = make(map[string]bool, len(st.Applied)+1)
	for k, v := range st.Applied {
		next.Applied[k] = v
	}
	next.NodesRun = append([]string{}, st.NodesRun...)
	if st.LastError != nil {
		e := *st.LastError
		next.LastError = &e
	}
	next.Warnings = append([]string{}, st.Warnings...)
	next.indexes = make(map[string]int, len(st.indexes)+1)
	for k, v := range st.indexes {
		next.indexes[k] = v
	}
	return next
}

func (st *FoldState) applyInit(d *InitData) {
	st.SchemaVersion = SchemaVersion
	st.RunID, st.ParentRunID, st.ForkedAtSeq, st.CreatedAt = d.RunID, d.ParentRunID, d.ForkedAtSeq, d.CreatedAt
	st.Lineage = nonNilStrings(d.Lineage)
	st.Workflow, st.WorkflowHash, st.Calibration, st.Mock = d.Workflow, d.WorkflowHash, d.Calibration, d.Mock
	st.WorkflowSource = d.WorkflowSource
	st.Vars = cloneStringMap(d.Vars)
	if st.Vars == nil {
		st.Vars = map[string]string{}
	}
	st.RepoMode, st.CmdsSHA256, st.RepoRoot, st.WorkDir = d.RepoMode, d.CmdsSHA256, d.RepoRoot, d.WorkDir
	st.AllowedCmds = append([]AllowedCmd{}, d.AllowedCmds...)
	st.Goldens = append([]Golden{}, d.Goldens...)
	st.State, st.StateKind, st.Iteration = d.InitialState, d.InitialKind, 0
	st.BaseSHA, st.Head = d.BaseSHA, d.Head
	if d.InitialKind == KindAgentEdit && d.ParentRunID == "" {
		st.FixEntryHead = d.Head
	}
	st.Findings, st.Confirmed, st.AllFound, st.Status = []Finding{}, []Bug{}, []Bug{}, []BugStatus{}
	st.PrevUnfixed = nil
	st.NodeOutputs = map[string]json.RawMessage{}
	st.Applied = map[string]bool{}
	st.NodesRun = []string{}
	st.Warnings = []string{}
	st.indexes = map[string]int{}
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return append([]string{}, in...)
}

func knownType(t string) bool {
	for _, k := range EventTypes {
		if k == t {
			return true
		}
	}
	return false
}

func nodeScoped(t string) bool {
	return t == TypeNeedsInput || t == TypeNodeOutput || t == TypeDeltaApplied || t == TypeLLMCall
}

func postTerminalAllowed(t string) bool {
	switch t {
	case TypeOverflowHandler, TypeCmdCall, TypeWarn, TypeRecord, TypeTokens, TypeTree, TypeFork:
		return true
	}
	return false
}

func validOutcome(o Outcome) bool {
	for _, x := range Outcomes {
		if x == o {
			return true
		}
	}
	return false
}

func sanctioned(cmds []AllowedCmd, name string) bool {
	for _, c := range cmds {
		if c.Name == name {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func idSet(bugs []Bug) map[string]struct{} {
	m := make(map[string]struct{}, len(bugs))
	for _, b := range bugs {
		m[b.ID] = struct{}{}
	}
	return m
}

// checkStatus enforces exact coverage of AllFound by a supplied Status (§4.3).
func checkStatus(all []Bug, status []BugStatus) string {
	known := idSet(all)
	seen := map[string]struct{}{}
	for _, s := range status {
		if _, ok := known[s.ID]; !ok {
			return ReasonStatusNotSubset
		}
		if _, dup := seen[s.ID]; dup {
			return ReasonStatusDuplicate
		}
		seen[s.ID] = struct{}{}
	}
	if len(seen) < len(known) {
		return ReasonStatusIncomplete
	}
	return ""
}

// countUnfixed counts bugs in all with no status entry marking them fixed (fail closed).
func countUnfixed(all []Bug, status []BugStatus) int {
	fixed := map[string]struct{}{}
	for _, s := range status {
		if !s.StillPresent {
			fixed[s.ID] = struct{}{}
		}
	}
	n := 0
	for _, b := range all {
		if _, ok := fixed[b.ID]; !ok {
			n++
		}
	}
	return n
}

// checkProvenance enforces §4.8 for a non-init event.
func checkProvenance(st FoldState, ev Event) string {
	inRange := st.ParentRunID != "" && ev.Seq >= 2 && ev.Seq <= st.ForkedAtSeq
	if !inRange {
		if ev.Origin != nil {
			return ReasonProvenance
		}
		return ""
	}
	o := ev.Origin
	if o == nil || o.RunID != st.ParentRunID || o.Seq != ev.Seq || o.Version < 1 || o.Hash == "" {
		return ReasonProvenance
	}
	return ""
}

// decodePayload decodes canonical bytes into the typed payload for t with DisallowUnknownFields.
func decodePayload(t string, canon []byte) (any, error) {
	var target any
	switch t {
	case TypeInit:
		target = &InitData{}
	case TypeTree:
		target = &TreeData{}
	case TypeNeedsInput:
		target = &EmptyData{}
	case TypeNodeOutput:
		target = &NodeOutputData{}
	case TypeDeltaApplied:
		target = &DeltaAppliedData{}
	case TypeLLMCall:
		target = &LLMCallData{}
	case TypeCmdCall:
		target = &CmdCallData{}
	case TypeGate:
		target = &GateData{}
	case TypeConverge:
		target = &ConvergeData{}
	case TypeTransition:
		target = &TransitionData{}
	case TypeFixBaseline:
		target = &FixBaselineData{}
	case TypeTokens:
		target = &TokenTotals{}
	case TypeRecord:
		target = &RecordData{}
	case TypeWarn:
		target = &WarnData{}
	case TypeOverflowHandler:
		target = &OverflowHandlerData{}
	default: // TypeFork; knownType already filtered the rest
		target = &ForkData{}
	}
	dec := json.NewDecoder(bytes.NewReader(canon))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return nil, err
	}
	return target, nil
}

func canonLenStr(s string) int {
	return len(marshalCanonical(s))
}

func shortOK(ss ...string) bool {
	for _, s := range ss {
		if canonLenStr(s) > MaxShort {
			return false
		}
	}
	return true
}

func textOK(ss ...string) bool {
	for _, s := range ss {
		if canonLenStr(s) > MaxText {
			return false
		}
	}
	return true
}

func argvOK(argv []string) bool {
	return len(argv) <= MaxArgv && textOK(argv...)
}

// withinCaps enforces every per-field cap of §2.3 on a decoded payload.
func withinCaps(p any) bool {
	switch d := p.(type) {
	case *InitData:
		if !shortOK(d.RunID, d.Workflow, d.WorkflowHash, d.Mock, d.RepoMode, d.CmdsSHA256, d.RepoRoot, d.WorkDir, d.BaseSHA, d.Head, string(d.InitialState), string(d.InitialKind), d.ParentRunID, d.WorkflowSource) || !shortOK(d.Lineage...) {
			return false
		}
		if len(d.Vars) > MaxVars || len(d.Goldens) > MaxGoldens || len(d.AllowedCmds) > MaxAllowedCmds {
			return false
		}
		for k, v := range d.Vars {
			if !shortOK(k) || !textOK(v) {
				return false
			}
		}
		for _, g := range d.Goldens {
			if !textOK(g.Comment) || !shortOK(g.Severity, g.Category) {
				return false
			}
		}
		for _, c := range d.AllowedCmds {
			if !shortOK(c.Name) || !argvOK(c.Argv) || len(c.FileHashes) > MaxFileHashes || len(c.Env) > MaxEnv || !shortOK(c.Env...) {
				return false
			}
			for k, v := range c.FileHashes {
				if !shortOK(k, v) {
					return false
				}
			}
		}
	case *TreeData:
		return shortOK(d.Head, d.TreeHash) && canonLenStr(d.Status) <= MaxDetail
	case *DeltaAppliedData:
		if len(d.Findings) > MaxDeltaList || len(d.Confirmed) > MaxDeltaList || len(d.Status) > MaxDeltaList || !shortOK(d.Commit, d.OutputHash) {
			return false
		}
		// Pins were added to the Delta without a clause here, so MaxPins was enforced by the
		// agent-edit validator alone and not by the cap check every recorded event passes
		// through. Each pin carries two source fragments, so an uncapped list is also the
		// cheapest way to make a snapshot too large to serialise.

		if len(d.Pins) > MaxPins || len(d.PinResults) > MaxPins {
			return false
		}
		for _, p := range d.Pins {
			if !pinOK(p) {
				return false
			}
		}
		for _, r := range d.PinResults {
			if !pinOK(r.Pin) || canonLenStr(r.Detail) > MaxDetail || !shortOK(string(r.Outcome)) {
				return false
			}
		}
		for _, f := range d.Findings {
			if !textOK(f.IssueText) || !shortOK(f.File, f.Severity, f.Category, f.Source) {
				return false
			}
		}
		for _, b := range d.Confirmed {
			if canonLenStr(b.Desc) > MaxDesc || !shortOK(b.ID, b.File, b.Verdict) {
				return false
			}
		}
		for _, s := range d.Status {
			if !shortOK(s.ID) {
				return false
			}
		}
	case *LLMCallData:
		return shortOK(d.Kind, d.Model, d.Effort, d.InputHash, d.Error, d.Evidence, d.TreeHash, d.BaseSHA, d.HeadSHA)
	case *CmdCallData:
		return shortOK(d.Name, d.InputHash, d.Error) && argvOK(d.Argv) && canonLenStr(d.Stdout) <= MaxDetail && canonLenStr(d.Stderr) <= MaxStderr
	case *OverflowHandlerData:
		return shortOK(d.Name, d.InputHash, d.Error) && argvOK(d.Argv) && canonLenStr(d.Stdout) <= MaxDetail && canonLenStr(d.Stderr) <= MaxStderr
	case *GateData:
		if !shortOK(d.Name) {
			return false
		}
		if d.Error != nil {
			return shortOK(d.Error.Code, d.Error.Gate) && canonLenStr(d.Error.Detail) <= MaxDetail
		}
	case *ConvergeData:
		return shortOK(d.Atom, string(d.Class)) && textOK(d.Reason)
	case *TransitionData:
		return shortOK(string(d.From), string(d.To), d.Gate, string(d.Outcome), string(d.ToKind), d.Head)
	case *FixBaselineData:
		return shortOK(d.Head)
	case *RecordData:
		return shortOK(d.Name)
	case *WarnData:
		return shortOK(d.Code) && textOK(d.Detail)
	case *ForkData:
		return shortOK(d.ChildRunID)
	}
	return true
}

// pinKey identifies the mutation a pin describes. The test name is deliberately not part of it:
// the same unprotected line reached by a different test is the same gap, and keying on the test
// would let a fix round "clear" it by naming another one.
func pinKey(p Pin) string { return p.File + "\x00" + p.From + "\x00" + p.To }

// foldUnproven accumulates the mutations no test caught.
//
// Only PinProven clears an entry, and that asymmetry is the whole point. A survived pin adds or
// refreshes the gap. A proven one closes it — the mutation now fails a test, which is the only
// evidence that actually settles the question. Anything else (malformed, unverifiable) means the
// round learned nothing, so a gap already established must stand: dropping it there would let a
// fix round retire real evidence by submitting a pin that does not compile, which is both the
// easiest thing to do by accident and the easiest to do on purpose.
func foldUnproven(current []Pin, results []PinResult) []Pin {
	out := append([]Pin(nil), current...)
	index := make(map[string]int, len(out))
	for i, p := range out {
		index[pinKey(p)] = i
	}
	for _, r := range results {
		key := pinKey(r.Pin)
		switch r.Outcome {
		case PinProven:
			if i, ok := index[key]; ok {
				out = append(out[:i], out[i+1:]...)
				index = make(map[string]int, len(out))
				for j, p := range out {
					index[pinKey(p)] = j
				}
			}
		case PinSurvived:
			if i, ok := index[key]; ok {
				out[i] = r.Pin
				continue
			}
			// Bounded like every other list the snapshot carries. Beyond the cap the run has a
			// problem no amount of extra evidence helps with, and an unbounded snapshot would
			// eventually fail to serialise, which loses the whole audit rather than one entry.
			if len(out) >= MaxDeltaList {
				continue
			}
			index[key] = len(out)
			out = append(out, r.Pin)
		}
	}
	return out
}

// pinOK bounds one pin's text. The fragments are source, so they use the same allowance as a
// finding's prose rather than the short-field one.
func pinOK(p Pin) bool {
	return shortOK(p.File, p.Test) && canonLenStr(p.From) <= MaxDesc && canonLenStr(p.To) <= MaxDesc
}
