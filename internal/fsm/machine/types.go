// Package machine is the deterministic core: it turns a workflow plus a run
// log into the next event(s). Every LLM or shell effect sits behind the
// interfaces declared here and implemented by the kind/judge/cmdexec
// packages; the machine itself only decides.
package machine

import (
	"context"
	"encoding/json"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Instructions is what a host-executed node is told to do.
type Instructions struct {
	Text         string          `json:"text"`
	Input        map[string]any  `json:"input"`
	Untrusted    []string        `json:"untrusted"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// Diff is the base..head diff handed to kinds, cut at MaxDiffBytes.
type Diff struct {
	Text      string
	Truncated bool
}

// MaxDiffBytes bounds the diff handed to kinds.
const MaxDiffBytes = 1 << 20

// ExecInput is everything a fork executor gets.
type ExecInput struct {
	Snap       run.Snapshot
	Node       *workflow.Node
	Diff       Diff
	StartIndex int
	Audit      func(run.Event) error
	Runner     converge.Caller // the session's guarded runner (same audit closure and ordinal source)
}

// NodeKind describes one kind of node work.
type NodeKind interface {
	Name() string
	Info() workflow.KindInfo
	Instructions(snap run.Snapshot, node *workflow.Node, diff Diff, nonce string) (Instructions, error)
	Decode(raw json.RawMessage) (any, error)
	Reduce(snap run.Snapshot, out any) (run.Delta, error)
}

// Executor runs a fork node and returns Decode-valid output.
type Executor interface {
	Execute(ctx context.Context, in ExecInput) (json.RawMessage, error)
}

// Registry resolves kinds and executors.
type Registry interface {
	Kind(name string) (NodeKind, bool)
	Executor(name string) (Executor, bool)
	Info() map[string]workflow.KindInfo
	Mock() bool
}

// Clock supplies event timestamps.
type Clock func() run.Time

// Sidecar stores per-run files beside audit.jsonl (workflow.yaml).
type Sidecar interface {
	Write(runID, name string, b []byte) error
	Read(runID, name string) ([]byte, error)
	List(runID string) ([]string, error)
}

// RunnerDeps is what the single Guarded factory receives per session.
type RunnerDeps struct {
	Allowed  []run.AllowedCmd
	WorkDir  string
	RunID    string
	Audit    func(run.Event) error
	CmdCalls func(name string) int // prior cmd_call count for a command (mock ordinal)
}

// Deps wires the machine. Nothing here is optional except Terminal.
type Deps struct {
	Store     run.RunStore
	Sidecar   Sidecar
	Kinds     Registry
	Git       func(workDir string) gate.Git
	Runner    func(RunnerDeps) converge.Caller
	Clock     Clock
	LookPath  func(string) (string, error)
	FileHash  func(string) (string, error)
	Workflows func(name string) ([]byte, error)
	ReadFile  func(string) ([]byte, error)
	Nonce     func() string
	MockLoad  func(dir string) (hash string, err error)
	Terminal  func(ctx context.Context, v View) error
}

// InitOptions parameterizes Init.
type InitOptions struct {
	Workflow        string
	RunID           string
	Vars            map[string]string
	Base            string
	RepoMode        string
	AllowCustomCmds string
	Calibration     bool
	MockDir         string
	GoldensPath     string
	WorkDir         string
	RepoRoot        string
}

// OpenOptions parameterizes Open.
type OpenOptions struct {
	Repair bool
}

// Statuses of an Advance.
const (
	StatusAdvanced   = "ADVANCED"
	StatusNeedsInput = "NEEDS_INPUT"
	StatusDone       = "DONE"
	StatusStopped    = "STOPPED"
	StatusGateFailed = "GATE_FAILED"
)

// Next actions of a View.
const (
	NextAdvance = "advance"
	NextRecord  = "record"
	NextNone    = "none"
)

// Record kinds.
const (
	RecordNodeOutput = "node-output"
	RecordTokens     = "tokens"
	RecordEvent      = "event"
)

// AdvanceResult is what one Advance decided.
type AdvanceResult struct {
	Status     string
	From, To   run.State
	Gate       *run.GateData
	Outcome    run.Outcome
	StopReason string
	NeedsInput *NeedsInput
	Warnings   []string
	Untrusted  []string
	ExitCode   int
	RunID      string
}

// NeedsInput tells the host what to do.
type NeedsInput struct {
	Node         string
	Kind         string
	Exec         string
	Model        string
	Effort       string
	Instructions Instructions
	Record       string
}

// RecordOptions parameterizes Record.
type RecordOptions struct {
	Kind    string
	Node    string
	Data    json.RawMessage
	Replace bool
	Name    string
}

// RecordResult reports the appended event.
type RecordResult struct {
	Seq  int64
	Type string
	Key  string
}

// View is the read model of a run.
type View struct {
	RunID      string
	Workflow   string
	Snapshot   run.Snapshot
	Node       *NodeView
	NextAction string
	Torn       bool
	FailedGate *run.GateData
}

// NodeView describes the current state's node.
type NodeView struct {
	Name, Kind, Exec   string
	HasOutput, Applied bool
}

// Error codes owned by the machine.
const (
	CodeWorkflowNotFound   = "ERR_WORKFLOW_NOT_FOUND"
	CodeWorkflowTooLarge   = "ERR_WORKFLOW_TOO_LARGE"
	CodeCmdsNotAllowed     = "ERR_CMDS_NOT_ALLOWED"
	CodeWorkdirForeign     = "ERR_WORKDIR_FOREIGN"
	CodeGoldensInvalid     = "ERR_GOLDENS_INVALID"
	CodeMockInvalid        = "ERR_MOCK_INVALID"
	CodeMockMismatch       = "ERR_MOCK_MISMATCH"
	CodeBadRepoMode        = "ERR_BAD_REPO_MODE"
	CodeSidecar            = "ERR_SIDECAR"
	CodeRunTerminal        = "ERR_RUN_TERMINAL"
	CodeWorkflowChanged    = "ERR_WORKFLOW_CHANGED"
	CodeNodeMismatch       = "ERR_NODE_MISMATCH"
	CodeNodeNotHost        = "ERR_NODE_NOT_HOST"
	CodeNodeOutputApplied  = "ERR_NODE_OUTPUT_APPLIED"
	CodeNodeOutputExists   = "ERR_NODE_OUTPUT_EXISTS"
	CodeNodeOutputInvalid  = "ERR_NODE_OUTPUT_INVALID"
	CodeInstructionsFailed = "ERR_INSTRUCTIONS_FAILED"
	CodeRecordName         = "ERR_RECORD_NAME"
	CodeRecordTokens       = "ERR_RECORD_TOKENS"
	CodeRecordTooLarge     = "ERR_RECORD_TOO_LARGE"
	CodeUnsanctionedEdit   = "ERR_UNSANCTIONED_EDIT"
	CodeExecutorFailed     = "ERR_EXECUTOR_FAILED"
	CodeConvergeFailed     = "ERR_CONVERGE_FAILED"
)

// Warn codes.
const (
	WarnUnsanctionedEdit      = "UNSANCTIONED_EDIT"
	WarnAuditTornLineDropped  = "AUDIT_TORN_LINE_DROPPED"
	WarnOverflowHandlerFailed = "OVERFLOW_HANDLER_FAILED"
	WarnWorkflow              = "WORKFLOW_WARNING"
)

// Pseudo-gate names.
const (
	GateRepoMode   = "repo_mode"
	GateExecutor   = "executor"
	GateNodeOutput = "node_output"
	GateConverge   = "converge"
)

// Size limits for files Init reads.
const (
	MaxWorkflowBytes = 256 << 10
	MaxGoldensBytes  = 512 << 10
)

// SidecarWorkflow is the name of the workflow sidecar.
const SidecarWorkflow = "workflow.yaml"
