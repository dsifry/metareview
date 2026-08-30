package status

import (
	"os"
	"path/filepath"
	"testing"
)

// The fixtures are a REAL run, captured 2026-08-30 from a session with a Stop hook registered and
// metareview blocking on 73 unresolved review blockers. The task was "reply with the word DONE
// and use no tools" — a request needing no work at all — and the host still reported:
//
//	subtype "success"   is_error false   stop_reason "end_turn"   result ""
//
// after refusing the session nine times. Nothing in that envelope separates it from a run that
// did the work. A benchmark harness reading is_error scores it clean, and scores it clean exactly
// when the gate was doing its job.
func TestABlockedRunIsNotASuccess(t *testing.T) {
	env, err := os.ReadFile(filepath.Join("testdata", "blocked-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join("testdata", "blocked-session.jsonl")

	got := AuditRun(env, transcript)
	if got.Outcome != RunBlocked {
		t.Errorf("outcome = %q, want %q: the host called this a success", got.Outcome, RunBlocked)
	}
	if got.StopHookBlocks != 9 {
		t.Errorf("stop-hook blocks = %d, want 9", got.StopHookBlocks)
	}
	if got.HostReportedError {
		t.Error("the fixture's value is that the host reported NO error; keep that visible")
	}
	if got.Result != "" {
		t.Errorf("the run produced no answer, so Result must show empty: %q", got.Result)
	}
}

func TestAuditRunSeparatesTheThreeOutcomes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	const oneBlock = `{"type":"attachment","attachment":{"type":"hook_blocking_error","hookName":"Stop"}}`
	clean := write("clean.jsonl", `{"type":"assistant"}`+"\n")
	blocked := write("blocked.jsonl", oneBlock+"\n")

	for name, tc := range map[string]struct {
		env, transcript string
		want            RunOutcome
	}{
		"finished with an answer":     {`{"subtype":"success","is_error":false,"result":"DONE"}`, clean, RunCompleted},
		"host reported an error":      {`{"subtype":"error","is_error":true,"result":""}`, clean, RunFailed},
		"blocked and gave up":         {`{"subtype":"success","is_error":false,"result":""}`, blocked, RunBlocked},
		"blocked but answered":        {`{"subtype":"success","is_error":false,"result":"DONE"}`, blocked, RunBlocked},
		"empty answer, never blocked": {`{"subtype":"success","is_error":false,"result":""}`, clean, RunCompleted},
	} {
		if got := AuditRun([]byte(tc.env), tc.transcript); got.Outcome != tc.want {
			t.Errorf("%s: outcome = %q, want %q", name, got.Outcome, tc.want)
		}
	}

	// A gate said no and the session finished anyway is the case a supervisor most needs to see,
	// so it does not get to be "completed" just because there was output.
	if got := AuditRun([]byte(`{"subtype":"success","is_error":false,"result":"DONE"}`), blocked); got.StopHookBlocks != 1 {
		t.Errorf("the refusal count must survive even when the run produced an answer: %+v", got)
	}

	// Absent or unreadable evidence never invents blocking, and a malformed envelope is a
	// failure to complete rather than a pass.
	if got := AuditRun([]byte(`{"subtype":"success","is_error":false,"result":"DONE"}`), filepath.Join(dir, "absent.jsonl")); got.Outcome != RunCompleted || got.StopHookBlocks != 0 {
		t.Errorf("a missing transcript must not invent a block: %+v", got)
	}
	if got := AuditRun([]byte(`not json`), clean); got.Outcome != RunFailed {
		t.Errorf("an unreadable envelope is not a success: %+v", got)
	}
	// A hook that is not the Stop hook does not mean the session was refused.
	other := write("other.jsonl", `{"type":"attachment","attachment":{"type":"hook_blocking_error","hookName":"PreToolUse"}}`+"\n")
	if got := AuditRun([]byte(`{"subtype":"success","is_error":false,"result":""}`), other); got.Outcome != RunCompleted {
		t.Errorf("only a Stop-hook refusal blocks a session: %+v", got)
	}
}

// The transcript is a log written by another process, so it is read defensively: a blank line, a
// line that is not JSON, and an empty path each mean "no evidence of blocking here", never an
// error and never an invented block. A transcript that cannot be parsed must not be able to
// manufacture a RunBlocked verdict any more than it can hide one.
func TestCountStopHookBlocksReadsAnImperfectTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "messy.jsonl")
	body := "" +
		`{"type":"attachment","attachment":{"type":"hook_blocking_error","hookName":"Stop"}}` + "\n" +
		"\n" + // blank line
		"not json at all\n" +
		`{"type":"assistant"}` + "\n" +
		`{"type":"attachment","attachment":{"type":"hook_blocking_error","hookName":"Stop"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countStopHookBlocks(path); got != 2 {
		t.Errorf("got %d refusals, want 2 (junk lines are skipped, not counted or fatal)", got)
	}
	if got := countStopHookBlocks(""); got != 0 {
		t.Errorf("no transcript is no evidence, got %d", got)
	}
	if got := countStopHookBlocks(filepath.Join(dir, "nope.jsonl")); got != 0 {
		t.Errorf("an unreadable transcript is no evidence, got %d", got)
	}
}
