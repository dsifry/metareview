package run

import (
	"bytes"
	"encoding/json"
	"sort"
)

// TornTail is an unterminated final line (§5.3).
type TornTail struct {
	Offset int64
	Bytes  []byte
}

// Log is what Events returns: the decoded, chain-verified events plus the chain head.
type Log struct {
	Events  []Event
	Head    string // LineHash of the last complete line
	Torn    *TornTail
	Version int
}

// RunSummary is one row of List().
type RunSummary struct {
	RunID       string
	Workflow    string
	CreatedAt   Time
	State       State
	Outcome     Outcome
	ParentRunID string
	Mock        string
	MockTainted bool
	Torn        bool
	Sidecars    int
	Error       string
}

// Options configures a store.
type Options struct {
	MaxEvents int // zero → DefaultMaxEvents
}

func (o Options) maxEvents() int {
	if o.MaxEvents <= 0 {
		return DefaultMaxEvents
	}
	return o.MaxEvents
}

// RunStore is the persistence seam (§5).
type RunStore interface {
	Create(runID string, first Event) (FoldState, error)
	Append(runID string, st FoldState, ev Event) (FoldState, error)
	Events(runID string) (Log, error)
	EventsWithLines(runID string) (Log, [][]byte, error)
	RepairTail(runID string) error
	List() ([]RunSummary, error)
	Lock(runID string) (unlock func(), err error)
	Root() string
}

func storeErrf(code string, seq int64, detail string) *StoreError {
	return &StoreError{Code: code, Seq: seq, Detail: detail}
}

// countedType reports whether an event type counts toward MaxEvents (§2.3).
func countedType(t string) bool {
	switch t {
	case TypeNeedsInput, TypeNodeOutput, TypeDeltaApplied, TypeLLMCall, TypeTokens, TypeRecord, TypeTree:
		return true
	}
	return false
}

// parseLines decodes complete stored lines and verifies seq contiguity and the hash chain.
func parseLines(lines [][]byte) (Log, error) {
	log := Log{Version: 1}
	prevHash := ""
	for i, line := range lines {
		seq := int64(i) + 1
		var ev Event
		dec := json.NewDecoder(bytes.NewReader(line))
		if err := dec.Decode(&ev); err != nil {
			return Log{}, storeErrf(CodeAuditChain, seq, "undecodable")
		}
		if ev.Seq != seq {
			return Log{}, storeErrf(CodeAuditChain, seq, "seq")
		}
		if ev.Prev != prevHash {
			return Log{}, storeErrf(CodeAuditChain, seq, "prev")
		}
		log.Events = append(log.Events, ev)
		prevHash = LineHash(line)
	}
	log.Head = prevHash
	return log, nil
}

// splitLines splits raw file bytes into complete lines and an optional torn tail.
func splitLines(raw []byte) ([][]byte, *TornTail) {
	var lines [][]byte
	start := 0
	for i, b := range raw {
		if b == '\n' {
			lines = append(lines, raw[start:i])
			start = i + 1
		}
	}
	if start < len(raw) {
		return lines, &TornTail{Offset: int64(start), Bytes: append([]byte{}, raw[start:]...)}
	}
	return lines, nil
}

// prepareAppend performs the shared, store-independent part of Append (§5.2 steps 3–6): CAS,
// canonicalization, Seq/Prev assignment, validate-before-write, size and count caps. It returns the
// line to write and the advanced state (ChainHead not yet set).
func prepareAppend(lines [][]byte, torn *TornTail, st FoldState, ev Event, maxEvents int) ([]byte, FoldState, error) {
	if torn != nil {
		return nil, FoldState{}, storeErrf(CodeAuditTorn, int64(len(lines))+1, "unterminated final line")
	}
	head := ""
	if len(lines) > 0 {
		head = LineHash(lines[len(lines)-1])
	}
	if head != st.ChainHead || int64(len(lines)) != st.Seq {
		return nil, FoldState{}, storeErrf(CodeAuditCAS, st.Seq, "state does not match the log tail")
	}
	canon, err := Canonical(ev.Data)
	if err != nil {
		return nil, FoldState{}, &StoreError{Code: CodeAppendRejected, Seq: st.Seq + 1, Cause: &FoldError{Code: CodeAuditInvalid, Reason: ReasonBadPayload, Seq: st.Seq + 1, Type: ev.Type}}
	}
	ev.Data = canon
	ev.Seq = st.Seq + 1
	ev.Prev = head
	next, err := Apply(st, ev)
	if err != nil {
		return nil, FoldState{}, &StoreError{Code: CodeAppendRejected, Seq: ev.Seq, Cause: err}
	}
	// MaxLine is always satisfied once Apply has enforced MaxPayload and the MaxShort envelope caps
	// (§2.3), so no separate line check exists; ERR_EVENT_TOO_LARGE is reserved for a future envelope.
	line := marshalCanonical(ev)
	if countedType(ev.Type) {
		n := 0
		for _, l := range lines {
			if countedType(lineType(l)) {
				n++
			}
		}
		if n+1 > maxEvents {
			return nil, FoldState{}, storeErrf(CodeAuditFull, ev.Seq, "MaxEvents reached")
		}
	}
	next.ChainHead = LineHash(line)
	return line, next, nil
}

// lineType extracts the type of a stored line cheaply.
func lineType(line []byte) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(line, &probe)
	return probe.Type
}

// prepareCreate validates and stamps the seq-1 init line (§5.2).
func prepareCreate(runID string, first Event) ([]byte, FoldState, error) {
	if first.Type != TypeInit {
		return nil, FoldState{}, &StoreError{Code: CodeAppendRejected, Seq: 1, Cause: &FoldError{Code: CodeAuditInvalid, Reason: ReasonFirstNotInit, Seq: 1, Type: first.Type}}
	}
	var d InitData
	if err := json.Unmarshal(first.Data, &d); err != nil || d.RunID != runID {
		return nil, FoldState{}, &StoreError{Code: CodeAppendRejected, Seq: 1, Cause: &FoldError{Code: CodeAuditInvalid, Reason: ReasonBadPayload, Seq: 1, Type: TypeInit}}
	}
	return prepareAppend(nil, nil, FoldState{}, first, DefaultMaxEvents)
}

// summarize builds a RunSummary from a parsed log.
func summarize(runID string, log Log, err error, sidecars int) RunSummary {
	s := RunSummary{RunID: runID, Sidecars: sidecars}
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.Torn = log.Torn != nil
	snap, ferr := Fold(log.Events)
	if ferr != nil {
		s.Error = ferr.Error()
		return s
	}
	s.Workflow, s.CreatedAt, s.State, s.Outcome = snap.Workflow, snap.CreatedAt, snap.State, snap.Outcome
	s.ParentRunID, s.Mock, s.MockTainted = snap.ParentRunID, snap.Mock, snap.MockTainted
	return s
}

func sortSummaries(list []RunSummary) {
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt.Time) {
			return list[i].CreatedAt.After(list[j].CreatedAt.Time)
		}
		return list[i].RunID < list[j].RunID
	})
}
