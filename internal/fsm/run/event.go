package run

import "encoding/json"

// Event is one line of audit.jsonl (§3). The machine sets everything except Seq and Prev.
type Event struct {
	SchemaVersion int             `json:"schemaVersion"`
	Seq           int64           `json:"seq"`
	Prev          string          `json:"prev"`
	At            Time            `json:"at"`
	Type          string          `json:"type"`
	State         State           `json:"state,omitempty"`
	Iter          int             `json:"iter"`
	Node          string          `json:"node,omitempty"`
	Mock          bool            `json:"mock,omitempty"`
	Origin        *Origin         `json:"origin,omitempty"`
	Data          json.RawMessage `json:"data"`
}

// Origin names the immediate parent line a copied event came from.
type Origin struct {
	RunID   string `json:"run_id"`
	Seq     int64  `json:"seq"`
	Version int    `json:"version"`
	Hash    string `json:"hash"`
}

// Event types (§3.1).
const (
	TypeInit            = "init"
	TypeTree            = "tree"
	TypeNeedsInput      = "needs_input"
	TypeNodeOutput      = "node_output"
	TypeDeltaApplied    = "delta_applied"
	TypeLLMCall         = "llm_call"
	TypeCmdCall         = "cmd_call"
	TypeGate            = "gate"
	TypeConverge        = "converge"
	TypeTransition      = "transition"
	TypeFixBaseline     = "fix_baseline"
	TypeTokens          = "tokens"
	TypeRecord          = "record"
	TypeWarn            = "warn"
	TypeOverflowHandler = "overflow_handler"
	TypeFork            = "fork"
)

// EventTypes lists every known type in table order.
var EventTypes = []string{TypeInit, TypeTree, TypeNeedsInput, TypeNodeOutput, TypeDeltaApplied, TypeLLMCall, TypeCmdCall, TypeGate, TypeConverge, TypeTransition, TypeFixBaseline, TypeTokens, TypeRecord, TypeWarn, TypeOverflowHandler, TypeFork}

// InitData is the seq-1 payload.
type InitData struct {
	RunID          string            `json:"run_id"`
	CreatedAt      Time              `json:"created_at"`
	Workflow       string            `json:"workflow"`
	WorkflowHash   string            `json:"workflow_hash"`
	WorkflowSource string            `json:"workflow_source,omitempty"` // embedded|path|"" (spec 3 r5; "" = legacy)
	Vars           map[string]string `json:"vars"`
	Calibration    bool              `json:"calibration"`
	Mock           string            `json:"mock,omitempty"`
	RepoMode       string            `json:"repo_mode"`
	AllowedCmds    []AllowedCmd      `json:"allowed_cmds"`
	CmdsSHA256     string            `json:"cmds_sha256,omitempty"`
	RepoRoot       string            `json:"repo_root"`
	WorkDir        string            `json:"work_dir"`
	BaseSHA        string            `json:"base_sha"`
	Head           string            `json:"head"`
	InitialState   State             `json:"initial_state"`
	InitialKind    Kind              `json:"initial_kind,omitempty"`
	Goldens        []Golden          `json:"goldens"`
	ParentRunID    string            `json:"parent_run_id,omitempty"`
	Lineage        []string          `json:"lineage"`
	ForkedAtSeq    int64             `json:"forked_at_seq,omitempty"`
}

// TreeData is a working-tree snapshot carrier.
type TreeData struct {
	Head            string `json:"head"`
	TreeHash        string `json:"tree_hash"`
	Status          string `json:"status"`
	StatusTruncated bool   `json:"status_truncated,omitempty"`
}

// NodeOutputData carries a host- or executor-supplied node output.
type NodeOutputData struct {
	Output json.RawMessage `json:"output"`
}

// DeltaAppliedData records the Delta a Reduce produced from the recorded output.
type DeltaAppliedData struct {
	Delta
	OutputHash string `json:"output_hash"`
}

// LLMCallData audits one judge call.
type LLMCallData struct {
	Kind       string          `json:"kind"`
	Model      string          `json:"model"`
	Effort     string          `json:"effort"`
	Index      int             `json:"index"`
	InputHash  string          `json:"input_hash"`
	Verdict    json.RawMessage `json:"verdict"`
	Confidence float64         `json:"confidence"`
	Tokens     TokenTotals     `json:"tokens"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
	// Evidence names HOW the judge saw the code: EvidenceExcerpt (the prompt carried
	// selected hunks) or EvidenceSandbox (the judge read a materialized tree itself).
	// Empty means EvidenceExcerpt, so existing rows keep their meaning.
	//
	// It is recorded because the two are not the same question. A verdict reached by
	// browsing and one reached from excerpts cannot be compared row-for-row, so fsm diff
	// has to see the difference rather than silently treat them as alike.
	Evidence string `json:"evidence,omitempty"`
	// TreeHash, BaseSHA and HeadSHA content-address what the judge COULD have read when
	// Evidence is EvidenceSandbox. Without them the recorded input no longer determines the
	// verdict and replay becomes a claim rather than a check.
	TreeHash string `json:"tree_hash,omitempty"`
	BaseSHA  string `json:"base_sha,omitempty"`
	HeadSHA  string `json:"head_sha,omitempty"`
}

// How a judge saw the code, recorded on every llm_call.
const (
	EvidenceExcerpt = "excerpt"
	EvidenceSandbox = "sandbox"
)

// CmdCallData audits one guarded command call.
type CmdCallData struct {
	Name            string   `json:"name"`
	Argv            []string `json:"argv"`
	InputHash       string   `json:"input_hash"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	StdoutTruncated bool     `json:"stdout_truncated,omitempty"`
	StderrTruncated bool     `json:"stderr_truncated,omitempty"`
	ExitCode        int      `json:"exit_code"`
	DurationMS      int64    `json:"duration_ms"`
	Error           string   `json:"error,omitempty"`
}

// GateData records one gate evaluation.
type GateData struct {
	Name   string     `json:"name"`
	Passed bool       `json:"passed"`
	Error  *GateError `json:"error,omitempty"`
}

// ConvergeData records one convergence evaluation.
type ConvergeData struct {
	Atom   string  `json:"atom"`
	Class  Outcome `json:"class"`
	Stop   bool    `json:"stop"`
	Reason string  `json:"reason"`
}

// TransitionData is the checkpoint event payload.
type TransitionData struct {
	From    State   `json:"from"`
	To      State   `json:"to"`
	Gate    string  `json:"gate"`
	Outcome Outcome `json:"outcome,omitempty"`
	Loop    bool    `json:"loop,omitempty"`
	ToKind  Kind    `json:"to_kind,omitempty"`
	Head    string  `json:"head"`
}

// FixBaselineData re-baselines FixEntryHead after a fork into an agent-edit state.
type FixBaselineData struct {
	Head string `json:"head"`
}

// RecordData is a user-recorded event.
type RecordData struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

// WarnData records a warning code.
type WarnData struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// OverflowHandlerData audits the on_overflow command.
type OverflowHandlerData struct {
	Name            string   `json:"name"`
	Argv            []string `json:"argv"`
	InputHash       string   `json:"input_hash"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	StdoutTruncated bool     `json:"stdout_truncated,omitempty"`
	StderrTruncated bool     `json:"stderr_truncated,omitempty"`
	ExitCode        int      `json:"exit_code"`
	DurationMS      int64    `json:"duration_ms"`
	Error           string   `json:"error,omitempty"`
}

// ForkData is appended to the parent when a child is forked.
type ForkData struct {
	ChildRunID string `json:"child_run_id"`
	AtSeq      int64  `json:"at_seq"`
}

// EmptyData is the payload of events that carry nothing.
type EmptyData struct{}
