package machine

import (
	"encoding/json"
	"sort"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Report is the output of Diff (spec 3 §4).
type Report struct {
	A               string     `json:"a"`
	B               string     `json:"b"`
	SameWorkflow    bool       `json:"same_workflow"`
	CommonPrefixSeq int64      `json:"common_prefix_seq"`
	Outcomes        [2]string  `json:"outcomes"`
	Calls           []CallRow  `json:"calls"`
	Transitions     []TransRow `json:"transitions"`
	// EvidenceMismatch counts rows whose two sides saw different evidence (see
	// run.LLMCallData.Evidence). Those rows are not a model comparison: a judge that read a
	// materialized tree and one handed excerpts were asked different questions, so a
	// consumer must be able to notice rather than read the difference as disagreement.
	EvidenceMismatch int `json:"evidence_mismatch"`
}

// CallRow aligns one judge call across the two runs by (node, iter, kind, input_hash).
type CallRow struct {
	Node           string    `json:"node"`
	Iter           int       `json:"iter"`
	Kind           string    `json:"kind"`
	InputHash      string    `json:"input_hash"`
	A              *CallSide `json:"a"`
	B              *CallSide `json:"b"`
	RawSame        bool      `json:"raw_same"`
	DecisionSame   bool      `json:"decision_same"`
	ConfidenceSame bool      `json:"confidence_same"`
	// EvidenceSame reports whether both sides saw the same KIND of evidence. When it is
	// false Same is forced false: equal verdicts from unequal evidence are not agreement.
	EvidenceSame bool `json:"evidence_same"`
	Same         bool `json:"same"`
	// occurrence is the within-side repeat index; it orders rows and never ships.
	occurrence int
}

// compare fills in one row's agreement flags and counts a cross-evidence pair on the report.
// It is separate from DiffRuns so the comparison rules can be exercised directly, without
// having to hand-build a log that satisfies every fold invariant.
func (r *Report) compare(row *CallRow) {
	if row.A == nil || row.B == nil {
		return // one-sided: nothing to compare
	}
	row.RawSame = boolEq(row.A.Raw, row.B.Raw)
	row.DecisionSame = boolEq(row.A.Effective, row.B.Effective)
	row.ConfidenceSame = row.A.Confidence == row.B.Confidence
	row.EvidenceSame = evidenceOf(row.A.Evidence) == evidenceOf(row.B.Evidence)
	// Equal verdicts from unequal evidence are not agreement: the two judges were asked
	// different questions, so Same is forced false and the report carries a count.
	row.Same = row.EvidenceSame && row.DecisionSame && row.ConfidenceSame && row.A.Error == "" && row.B.Error == ""
	if !row.EvidenceSame {
		r.EvidenceMismatch++
	}
}

// evidenceOf normalises the empty value, which predates the field and means excerpt.
func evidenceOf(s string) string {
	if s == "" {
		return run.EvidenceExcerpt
	}
	return s
}

// CallSide is one run's side of a CallRow.
type CallSide struct {
	Index      int     `json:"index"`
	Model      string  `json:"model"`
	Effort     string  `json:"effort"`
	Raw        *bool   `json:"raw"`
	Effective  *bool   `json:"effective"`
	Confidence float64 `json:"confidence"`
	Error      string  `json:"error,omitempty"`
	// Evidence is how this side's judge saw the code (run.EvidenceExcerpt or
	// run.EvidenceSandbox). Empty means excerpt, so older rows keep their meaning.
	Evidence string `json:"evidence,omitempty"`
}

// TransRow aligns transitions by ordinal.
type TransRow struct {
	SeqA    int64       `json:"seq_a"`
	SeqB    int64       `json:"seq_b"`
	To      run.State   `json:"to"`
	Gate    string      `json:"gate"`
	Outcome run.Outcome `json:"outcome,omitempty"`
	Same    bool        `json:"same"`
}

// DiffRuns compares two runs' judge calls and transitions; reasoning never participates (spec 3 §4).
func DiffRuns(a, b run.Log, decide func(kind string, verdict json.RawMessage) Decision) (Report, error) {
	sa, err := run.Fold(a.Events)
	if err != nil {
		return Report{}, err
	}
	sb, err := run.Fold(b.Events)
	if err != nil {
		return Report{}, err
	}
	if sa.Workflow != sb.Workflow {
		return Report{}, errs.E(CodeDiffIncompatible, "runs use different workflows", "a", sa.Workflow, "b", sb.Workflow)
	}
	r := Report{A: sa.RunID, B: sb.RunID, SameWorkflow: sa.WorkflowHash == sb.WorkflowHash, CommonPrefixSeq: commonPrefix(a.Events, b.Events), Outcomes: [2]string{string(sa.Outcome), string(sb.Outcome)}, Calls: []CallRow{}, Transitions: []TransRow{}}
	// Rows align across runs by (node, iter, kind, input_hash) — the same question
	// asked of both runs — which is why the call index is deliberately NOT part of
	// the identity: the two runs assign indices independently.
	//
	// occurrence disambiguates repeats within one side. A node@iter can hold
	// several calls of one kind with the same input, and keying without it made
	// the second call overwrite the first, dropping it from the report entirely.
	// The nth such call on side A pairs with the nth on side B.
	type key struct {
		node       string
		iter       int
		kind, ih   string
		occurrence int
	}
	rows := map[key]*CallRow{}
	collect := func(events []run.Event, side int) {
		seen := map[key]int{} // per-side repeat counter, reset for each side
		for _, ev := range events {
			if ev.Type != run.TypeLLMCall {
				continue
			}
			var d run.LLMCallData
			_ = json.Unmarshal(ev.Data, &d)
			k := key{node: ev.Node, iter: ev.Iter, kind: d.Kind, ih: d.InputHash}
			k.occurrence = seen[k]
			seen[k]++
			row, ok := rows[k]
			if !ok {
				row = &CallRow{Node: ev.Node, Iter: ev.Iter, Kind: d.Kind, InputHash: d.InputHash, occurrence: k.occurrence}
				rows[k] = row
			}
			dec := decide(d.Kind, d.Verdict)
			cs := &CallSide{Index: d.Index, Model: d.Model, Effort: d.Effort, Raw: dec.Raw, Effective: dec.Effective, Confidence: d.Confidence, Error: d.Error, Evidence: d.Evidence}
			if side == 0 {
				row.A = cs
			} else {
				row.B = cs
			}
		}
	}
	collect(a.Events, 0)
	collect(b.Events, 1)
	for _, row := range rows {
		r.compare(row)
		r.Calls = append(r.Calls, *row)
	}
	sort.Slice(r.Calls, func(i, j int) bool {
		x, y := r.Calls[i], r.Calls[j]
		if x.Node != y.Node {
			return x.Node < y.Node
		}
		if x.Iter != y.Iter {
			return x.Iter < y.Iter
		}
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		if x.InputHash != y.InputHash {
			return x.InputHash < y.InputHash
		}
		return x.occurrence < y.occurrence
	})
	ta, tb := transitions(a.Events), transitions(b.Events)
	for i := 0; i < len(ta) || i < len(tb); i++ {
		row := TransRow{}
		if i < len(ta) {
			row.SeqA, row.To, row.Gate, row.Outcome = ta[i].seq, ta[i].d.To, ta[i].d.Gate, ta[i].d.Outcome
		}
		if i < len(tb) {
			row.SeqB = tb[i].seq
			if i >= len(ta) {
				row.To, row.Gate, row.Outcome = tb[i].d.To, tb[i].d.Gate, tb[i].d.Outcome
			}
		}
		if i < len(ta) && i < len(tb) {
			row.Same = ta[i].d.To == tb[i].d.To && ta[i].d.Gate == tb[i].d.Gate && ta[i].d.Outcome == tb[i].d.Outcome
		}
		r.Transitions = append(r.Transitions, row)
	}
	return r, nil
}

type trans struct {
	seq int64
	d   run.TransitionData
}

func transitions(events []run.Event) []trans {
	var out []trans
	for _, ev := range events {
		if ev.Type == run.TypeTransition {
			var d run.TransitionData
			_ = json.Unmarshal(ev.Data, &d)
			out = append(out, trans{ev.Seq, d})
		}
	}
	return out
}

// commonPrefix is the largest n such that events 2..n of both logs have identical (Type, Node, At, Data);
// Origin, Seq and Prev are ignored. Returns 1 when they diverge at seq 2.
func commonPrefix(a, b []run.Event) int64 {
	n := int64(1)
	for i := 1; i < len(a) && i < len(b); i++ {
		x, y := a[i], b[i]
		if x.Type != y.Type || x.Node != y.Node || !x.At.Equal(y.At.Time) || string(x.Data) != string(y.Data) {
			break
		}
		n = int64(i) + 1
	}
	return n
}

func boolEq(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
