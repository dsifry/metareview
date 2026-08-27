package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// CLI-owned codes (spec 5 §9).
const (
	CodeUsage            = "ERR_USAGE"
	CodeNotARepo         = "ERR_NOT_A_REPO"
	CodeNoRuns           = "ERR_NO_RUNS"
	CodeInputTooLarge    = "ERR_INPUT_TOO_LARGE"
	CodeRepoRootMismatch = "ERR_REPO_ROOT_MISMATCH"
	CodeJudgeUnset       = "ERR_JUDGE_UNSET"
	CodeInterrupted      = "ERR_INTERRUPTED"
	CodeInternal         = "ERR_INTERNAL"
	WarnRunIDFromEnv     = "RUN_ID_FROM_ENV"
	WarnRunsNotIgnored   = "RUNS_JSONL_NOT_IGNORED"
)

// Statuses.
const (
	StatusOK     = "OK"
	StatusForked = "FORKED"
)

// SchemaVersion is the envelope's version (keys only ever added within 0.9.x).
const SchemaVersion = 1

// phase names the machine call that returned an error (spec 5 §3: exitFor(command, phase, code)).
type phase string

const (
	phaseOpen    phase = "open"
	phaseAdvance phase = "advance"
	phaseFork    phase = "fork"
	phaseRecord  phase = "record"
	phaseJudge   phase = "judge"
	phaseInit    phase = "init"
	phaseNone    phase = "none" // run-less commands and judge without --run: nothing persisted
)

// exit2 lists the codes that are refusals before any mutation whatever the phase.
var exit2 = map[string]bool{
	CodeUsage: true, CodeNotARepo: true, CodeNoRuns: true, CodeRepoRootMismatch: true, CodeJudgeUnset: true, CodeInputTooLarge: true,
	"ERR_RUN_EXISTS": true, "ERR_RUN_ESCALATED": true, "ERR_MOCK_MISMATCH": true, "ERR_MOCK_INVALID": true,
	"ERR_WORKFLOW_NOT_FOUND": true, "ERR_WORKFLOW_INVALID": true, "ERR_WORKFLOW_TOO_LARGE": true, "ERR_WORKFLOW_CHANGED": true, "ERR_WORKFLOW_INCOMPATIBLE": true,
	"ERR_VAR_UNSET": true, "ERR_VAR_UNKNOWN": true, "ERR_VAR_FROZEN": true, "ERR_CALIBRATION_PINNED": true, "ERR_BAD_REPO_MODE": true,
	"ERR_CMDS_NOT_ALLOWED": true, "ERR_CMD_NOT_ALLOWED": true, "ERR_WORKDIR_FOREIGN": true, "ERR_GOLDENS_INVALID": true,
	"ERR_CHECKPOINT_NOT_FOUND": true, "ERR_TREE_NOT_AT_CHECKPOINT": true, "ERR_COPY_INVALID": true,
	"ERR_AUDIT_NOT_TORN": true, "ERR_EXPORT_DEST": true, "ERR_EXPORT_TOO_LARGE": true, "ERR_DIFF_INCOMPATIBLE": true, "ERR_BAD_CONVERGENCE": true,
	"ERR_RECORD_NAME": true, "ERR_RECORD_TOKENS": true, "ERR_RECORD_TOO_LARGE": true, "ERR_EVENT_TOO_LARGE": true,
	"ERR_NODE_OUTPUT_INVALID": true, "ERR_NODE_OUTPUT_EXISTS": true, "ERR_NODE_OUTPUT_APPLIED": true, "ERR_NODE_MISMATCH": true, "ERR_NODE_NOT_HOST": true,
	"ERR_GATE_INAPPLICABLE": true,
}

// exitFor is the sole authority on exit codes (spec 5 §3).
func exitFor(code string, ph phase, repairMoved bool) int {
	if ph == phaseNone {
		return 2
	}
	switch code {
	case "ERR_RUN_NOT_FOUND":
		if repairMoved {
			return 1
		}
		return 2
	case "ERR_GIT", "ERR_GIT_REF", "ERR_CMD_CHANGED", "ERR_CMD_NOT_FOUND":
		if ph == phaseAdvance {
			return 1
		}
		return 2
	case "ERR_JUDGE_KEY", "ERR_JUDGE_MODEL", "ERR_JUDGE_EFFORT_UNSUPPORTED", "ERR_JUDGE_URL":
		if repairMoved {
			return 1
		}
		return 2
	case "ERR_RUN_TERMINAL":
		if ph == phaseAdvance {
			return 1
		}
		return 2
	case "ERR_RUN_LOCKED":
		if ph == phaseOpen {
			return 2
		}
		return 1
	case "ERR_STORE_PATH":
		if ph == phaseOpen || ph == phaseInit {
			return 2
		}
		return 1
	case "ERR_RUNS_JSONL":
		if ph == phaseInit {
			return 2
		}
		return 1
	}
	if exit2[code] {
		return 2
	}
	return 1
}

// envelope is one stdout JSON object.
type envelope map[string]any

// failure maps an error to the envelope's error object, its code, and the untrusted paths it adds.
func failure(err error) (code string, obj map[string]any, untrusted []string) {
	fields := map[string]any{}
	detail := ""
	var se *run.StoreError
	var fe *run.FoldError
	switch {
	case errs.As(err) != nil:
		e := errs.As(err)
		code, detail = e.Code, e.Detail
		for k, v := range e.Fields {
			if k == "cmds_json" {
				continue
			}
			s, _ := run.CapText(v, run.MaxShort)
			fields[k] = s
		}
	case errors.As(err, &se):
		code, detail = se.Code, se.Detail
		fields["seq"] = se.Seq
	case errors.As(err, &fe):
		code, detail = fe.Code, fe.Reason
		fields["seq"], fields["type"], fields["reason"] = fe.Seq, fe.Type, fe.Reason
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code, detail = CodeInterrupted, err.Error()
	default:
		code, detail = CodeInternal, err.Error()
	}
	d, truncated := run.CapText(detail, run.MaxDetail)
	obj = map[string]any{"code": code, "detail": d, "detail_truncated": truncated, "fields": fields}
	untrusted = []string{"error.detail"}
	for k := range fields {
		untrusted = append(untrusted, "error.fields."+k)
	}
	sort.Strings(untrusted)
	return code, obj, untrusted
}

// viewKeys fills the common run keys from a View.
func viewKeys(env envelope, v machine.View) {
	s := v.Snapshot
	env["run_id"] = v.RunID
	env["state"] = string(s.State)
	env["iteration"] = s.Iteration
	if s.Outcome == "" {
		env["outcome"] = nil
	} else {
		env["outcome"] = string(s.Outcome)
	}
	env["mock"] = s.Mock != "" || s.MockTainted
	env["workflow_source"] = s.WorkflowSource
	env["workflow_hash"] = s.WorkflowHash
}

// warnObj splits the machine's "CODE: detail" strings into {code, detail} objects.
func warnObj(items []string) []map[string]any {
	out := []map[string]any{}
	for _, it := range items {
		code, detail, _ := strings.Cut(it, ": ")
		out = append(out, map[string]any{"code": code, "detail": detail})
	}
	return out
}

func finish(env envelope) envelope {
	if _, ok := env["schema_version"]; !ok {
		env["schema_version"] = SchemaVersion
	}
	if _, ok := env["warnings"]; !ok {
		env["warnings"] = []map[string]any{}
	}
	if _, ok := env["untrusted"]; !ok {
		env["untrusted"] = []string{}
	}
	if w, ok := env["warnings"].([]map[string]any); ok && len(w) > 0 {
		env["untrusted"] = appendSorted(env["untrusted"].([]string), "warnings[].detail")
	}
	return env
}

func appendSorted(list []string, more ...string) []string {
	list = append(list, more...)
	sort.Strings(list)
	return list
}

// print writes the envelope as one JSON line.
func print(w io.Writer, env envelope) {
	b, _ := json.Marshal(finish(env)) // the envelope holds only JSON-safe values
	_, _ = w.Write(append(b, '\n'))
}

// errEnvelope prints an error envelope and returns its exit code.
func errEnvelope(w io.Writer, base envelope, err error, ph phase, repairMoved bool) int {
	code, obj, untrusted := failure(err)
	env := envelope{}
	for k, v := range base {
		env[k] = v
	}
	env["ok"] = false
	env["status"] = "ERROR"
	env["code"] = code
	env["error"] = obj
	if existing, ok := env["untrusted"].([]string); ok {
		untrusted = appendSorted(existing, untrusted...)
	}
	env["untrusted"] = untrusted
	print(w, env)
	return exitFor(code, ph, repairMoved)
}
