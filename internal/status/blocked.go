package status

import (
	"encoding/json"
	"os"
	"strings"
)

// RunOutcome classifies how a host session ended.
//
// A session a review gate refused to let finish is not a success and not a crash, and calling it
// either loses the thing worth knowing. Measured on 2026-08-30: with a Stop hook blocking, a
// `claude -p` run came back subtype "success", is_error false, stop_reason "end_turn" — and an
// EMPTY result, after the hook had fired nine times. Nothing in that envelope distinguishes it
// from a run that did the work, so a benchmark harness reading is_error scores a blocked run as
// a clean one, and scores it as clean precisely when the gate was working.
//
// This is the same distinction the rest of this codebase keeps arriving at: an engine that timed
// out has not proved a mutation, a lens that errored has not reviewed, a target nobody looked at
// is not passing. "Did not complete" is its own class, and it must never fold into "fine".
type RunOutcome string

const (
	// RunCompleted: the session ended on its own with an answer.
	RunCompleted RunOutcome = "completed"
	// RunBlocked: a gate refused to let it finish. The work is not done and no error was raised.
	RunBlocked RunOutcome = "blocked"
	// RunFailed: the session itself errored.
	RunFailed RunOutcome = "failed"
)

// RunAudit is what a harness needs to tell those apart, over and above the host's own envelope.
type RunAudit struct {
	Outcome RunOutcome `json:"outcome"`
	// StopHookBlocks is how many times a Stop hook refused the session. Non-zero with an empty
	// result is the signature of the case above.
	StopHookBlocks int `json:"stopHookBlocks"`
	// Result is the host's final answer, kept so "empty" is visible rather than inferred.
	Result string `json:"result"`
	// HostReportedError is what the host itself said, so a reader can see the disagreement.
	HostReportedError bool `json:"hostReportedError"`
}

// AuditRun classifies a completed host session from its result envelope and its transcript.
//
// resultJSON is the `--output-format json` envelope. transcriptPath is the session JSONL, which
// is where the blocking actually shows up: the envelope does not count hook refusals, and the
// transcript records one hook_blocking_error per refusal.
func AuditRun(resultJSON []byte, transcriptPath string) RunAudit {
	a := RunAudit{Outcome: RunCompleted}
	var env struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
		Subtype string `json:"subtype"`
	}
	// A malformed envelope is itself a failure to complete, not a pass.
	if err := json.Unmarshal(resultJSON, &env); err != nil {
		return RunAudit{Outcome: RunFailed}
	}
	a.Result, a.HostReportedError = env.Result, env.IsError
	a.StopHookBlocks = countStopHookBlocks(transcriptPath)

	switch {
	case env.IsError || env.Subtype == "error":
		a.Outcome = RunFailed
	case a.StopHookBlocks > 0:
		// Refused at least once, however it ended. Both shapes are RunBlocked: the measured case
		// gave up and reported success with an empty result, and a session that answered anyway
		// is if anything more worth surfacing — a gate said no and it finished regardless.
		// Result and StopHookBlocks carry the difference, so the outcome does not need to.
		a.Outcome = RunBlocked
	}
	return a
}

// countStopHookBlocks counts Stop-hook refusals in a session transcript. An unreadable transcript
// counts zero, and the caller keeps whatever the envelope said: this function's job is to find
// evidence of blocking, never to invent it.
func countStopHookBlocks(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path) // #nosec G304 -- a transcript path supplied by the caller
	if err != nil {
		return 0
	}
	var n int
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var row struct {
			Attachment struct {
				Type     string `json:"type"`
				HookName string `json:"hookName"`
			} `json:"attachment"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row.Attachment.Type == "hook_blocking_error" && row.Attachment.HookName == "Stop" {
			n++
		}
	}
	return n
}
