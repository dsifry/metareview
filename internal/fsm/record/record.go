// Package record writes the runs.jsonl row for a terminal FSM run (spec 3 §6) and answers the two questions the CLI
// asks of that file: does a row exist for an id, and the recorder itself as machine.Deps.Terminal.
package record

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dsifry/metareview/internal/jsonl"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// CodeRunsJSONL is raised for an undecodable or conflicting runs.jsonl line.
const CodeRunsJSONL = "ERR_RUNS_JSONL"

// Row is the runs.jsonl record for an FSM run. Keys are camelCase (the existing writers' file); new keys are additive.
type Row struct {
	SchemaVersion    int               `json:"schemaVersion"`
	ID               string            `json:"id"`
	Scope            string            `json:"scope"`
	Target           map[string]string `json:"target"`
	Status           string            `json:"status"`
	Verdict          string            `json:"verdict"`
	ExecutionMode    string            `json:"executionMode"`
	PreviousRunID    string            `json:"previousRunId,omitempty"`
	AttemptNumber    int               `json:"attemptNumber"`
	MaxAttempts      int               `json:"maxAttempts"`
	BaseSHA          string            `json:"baseSha"`
	HeadSHA          string            `json:"headSha"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
	RepoRoot         string            `json:"repoRoot"`
	ContextPackPath  string            `json:"contextPackPath"`
	ReviewLogPath    string            `json:"reviewLogPath"`
	Mock             bool              `json:"mock"`
	Outcome          string            `json:"outcome"`
	FSMRunDir        string            `json:"fsmRunDir"`
	WorkflowHash     string            `json:"workflowHash"`
	WorkflowSource   string            `json:"workflowSource"`
	EscalationReason string            `json:"escalationReason"`
}

// Verdicts and statuses (the existing writers' vocabulary).
const (
	VerdictPass          = "PASS"
	VerdictNeedsRevision = "NEEDS_REVISION"
	VerdictEscalated     = "ESCALATED"
	StatusPassed         = "passed"
	StatusNeedsRevision  = "needs-revision"
	StatusEscalated      = "escalated"
)

func path(root string) string { return filepath.Join(root, ".metareview", "runs.jsonl") }

// RowFor maps a terminal view to its row (spec 3 §6).
func RowFor(v machine.View, now run.Time) Row {
	s := v.Snapshot
	attempt := machine.Attempt(s)
	verdict, status := VerdictNeedsRevision, StatusNeedsRevision
	reason := ""
	if machine.PassOutcome(s.Outcome) {
		verdict, status = VerdictPass, StatusPassed
	} else if attempt >= machine.MaxAttempts {
		verdict, status = VerdictEscalated, StatusEscalated
		reason = fmt.Sprintf("attempt %d of a fork lineage ended %s", attempt, s.Outcome)
	}
	base := s.BaseSHA
	if len(base) > 12 {
		base = base[:12]
	}
	source := s.WorkflowSource
	if source == "" {
		source = "unknown"
	}
	return Row{
		SchemaVersion: 1, ID: v.RunID, Scope: "fsm-" + s.Workflow, Target: map[string]string{"type": "fsm", "id": s.Workflow + "@" + base},
		Status: status, Verdict: verdict, ExecutionMode: "fsm", PreviousRunID: s.ParentRunID, AttemptNumber: attempt, MaxAttempts: machine.MaxAttempts,
		BaseSHA: s.BaseSHA, HeadSHA: s.Head, CreatedAt: s.CreatedAt.UTC().Format(rfc3339Nano), UpdatedAt: now.UTC().Format(rfc3339Nano),
		RepoRoot: s.RepoRoot, Mock: s.Mock != "" || s.MockTainted, Outcome: string(s.Outcome), FSMRunDir: ".metareview/runs/" + v.RunID + "/",
		WorkflowHash: s.WorkflowHash, WorkflowSource: source, EscalationReason: reason,
	}
}

const rfc3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

// Terminal returns the machine.Deps.Terminal implementation: one row per run, appended at terminal only, idempotent
// by id; a pre-existing row for the id with a different head or workflow hash is an error, never a silent skip.
func Terminal(root string, clock func() run.Time) func(context.Context, machine.View) error {
	return func(ctx context.Context, v machine.View) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := RowFor(v, clock())
		return appendRow(root, row)
	}
}

// Exists reports whether runs.jsonl already holds a row for id (read-only; a torn tail is tolerated, never repaired).
func Exists(root, id string) (bool, error) {
	rows, _, err := readRows(root)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// tail describes what readRows found after the last newline.
type tail struct {
	fragment []byte // an undecodable unterminated fragment (a torn append); nil when the file ends cleanly
	newline  bool   // a decodable newline-less final row was found: the writer must add "\n" first
	offset   int64  // byte offset where the fragment starts (== file size when there is none)
}

// readRows decodes runs.jsonl with the tolerant scanner of spec 3 §6: blank lines are skipped, a final line without a
// trailing newline that decodes is a row, an undecodable unterminated fragment is reported as the tail, any other
// undecodable line is ERR_RUNS_JSONL{malformed}.
func readRows(root string) ([]Row, tail, error) {
	raw, err := os.ReadFile(path(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, tail{}, nil
	}
	if err != nil {
		return nil, tail{}, err
	}
	var rows []Row
	complete := raw
	t := tail{offset: int64(len(raw))}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		i := bytes.LastIndexByte(raw, '\n') + 1
		last := raw[i:]
		var r Row
		if json.Unmarshal(last, &r) == nil {
			t.newline = true
			complete = append(append([]byte{}, raw[:i]...), last...)
			complete = append(complete, '\n')
		} else {
			t.fragment, t.offset = last, int64(i)
			complete = raw[:i]
		}
	}
	sc := jsonl.NewScanner(bytes.NewReader(complete))
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var r Row
		if err := json.Unmarshal([]byte(text), &r); err != nil {
			return nil, tail{}, errs.E(CodeRunsJSONL, "runs.jsonl line "+fmt.Sprint(line)+" is not a JSON object", "line", fmt.Sprint(line), "reason", "malformed")
		}
		rows = append(rows, r)
	}
	if err := sc.Err(); err != nil {
		return nil, tail{}, err
	}
	return rows, t, nil
}

// appendRow is record's own writer: flock, read, repair or terminate the tail, check the id, append + fsync.
func appendRow(root string, row Row) error {
	p := path(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// O_APPEND, not a seek: runs.jsonl has a second writer, state.AppendJSONL,
	// used by the artifact, epic, task-done and pr-ready gates, and it takes no
	// lock. An advisory flock only excludes writers that also take it, so the
	// only thing that keeps the two from colliding is the kernel placing each
	// write at the end atomically. Positioning with Seek(0,2) left a window in
	// which the other writer's row landed first and this one overwrote it.
	// O_RDWR is still needed for the torn-tail Truncate below.
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	// The append below ends in f.Sync, so the row is durable before this runs
	// and a Close error cannot mean a lost write.
	defer func() { _ = f.Close() }()
	if err := flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = flock(int(f.Fd()), syscall.LOCK_UN) }()
	rows, t, err := readRows(root)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != row.ID {
			continue
		}
		if r.HeadSHA != row.HeadSHA || r.WorkflowHash != row.WorkflowHash {
			return errs.E(CodeRunsJSONL, "a row with this id already exists for a different head or workflow", "line", row.ID, "reason", "id_conflict")
		}
		return nil // idempotent
	}
	line := run.MarshalCanonical(row)
	if t.newline {
		line = append([]byte{'\n'}, line...)
	}
	var steps []func() error
	if t.fragment != nil {
		torn := filepath.Join(root, ".metareview", "runs", ".torn")
		steps = append(steps,
			func() error { return os.MkdirAll(torn, 0o700) },
			func() error {
				return os.WriteFile(filepath.Join(torn, fmt.Sprintf("runs.jsonl-%d", nanos())), t.fragment, 0o600)
			},
			func() error { return f.Truncate(t.offset) })
	}
	steps = append(steps,
		// No seek: with O_APPEND the kernel positions every write at the end.
		func() error { _, err := f.Write(append(line, '\n')); return err },
		f.Sync)
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// flock is the advisory lock seam (tests inject a failing one).
var flock = syscall.Flock

// nanos is the torn-fragment name clock; tests may override it.
var nanos = nowNanos
