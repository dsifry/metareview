package judge

import (
	"context"
	"strings"
	"testing"
)

// A verdict without reasoning cannot be audited, so it is not a judgment (#193): every kind reads it as a
// parse error, which each caller already treats fail-closed.
func TestParseRejectsAVerdictWithoutReasoning(t *testing.T) {
	for kind, body := range map[string]string{
		KindMatch:        `"match":true,"confidence":0.9`,
		KindAdjudicate:   `"is_real":false,"confidence":0.9`,
		KindSymptom:      `"matches":true,"confidence":0.9`,
		KindStillPresent: `"still_present":false,"confidence":0.9`,
	} {
		for _, reasoning := range []string{``, `"reasoning":"",`, `"reasoning":"  \n\t",`} {
			parsed, decision, _, perr := Parse(kind, "{"+reasoning+body+"}")
			if !strings.Contains(perr, "missing reasoning") {
				t.Errorf("%s with %q: want a missing-reasoning parse error, got %q", kind, reasoning, perr)
			}
			if parsed != nil || decision != (kind == KindStillPresent) {
				t.Errorf("%s with %q: a verdict without reasoning must fail closed, got parsed=%s decision=%v", kind, reasoning, parsed, decision)
			}
		}
		if _, _, _, perr := Parse(kind, `{"reasoning":"line 3 derefs nil",`+body+`}`); perr != "" {
			t.Errorf("%s with reasoning: %s", kind, perr)
		}
	}
}

// codexStreamOf is codexStream with several agent messages in one turn.
func codexStreamOf(texts ...string) string {
	s := `{"type":"thread.started","thread_id":"t"}` + "\n"
	for _, text := range texts {
		s += `{"type":"item.completed","item":{"id":"i","type":"agent_message","text":` + quote(text) + `}}` + "\n"
	}
	return s + `{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}` + "\n"
}

// The #193 audits: the judge answered, a Stop hook in its session blocked, and the turn went on to a second
// verdict whose reasoning was the hook's notice. The last message was recorded as the judgment. A turn that
// continues past a complete verdict is retried, and fails closed if it never stops continuing.
func TestCodexJudgeRejectsATurnContinuedPastItsVerdict(t *testing.T) {
	hooked := codexStreamOf(
		`{"reasoning":"line 3 derefs a nil map","is_real":true,"confidence":0.9}`,
		`{"reasoning":"The metareview hook could not run because metareview is not installed.","is_real":false,"confidence":0.9}`,
	)
	f := &fakeCodex{stdout: hooked}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), codexRequest())
	if err != nil {
		t.Fatalf("a continued turn is a verdict that fails closed, not a transport error: %v", err)
	}
	if f.calls != MaxAttempts || v.Attempts != MaxAttempts {
		t.Fatalf("a continued turn is retried: calls=%d attempts=%d", f.calls, v.Attempts)
	}
	if !strings.Contains(v.ParseError, "continued past") || v.Parsed != nil || v.Decision {
		t.Fatalf("the hook's verdict must not be recorded: %+v", v)
	}

	// A retry that answers once is taken.
	streams := []string{hooked, codexStream(`{"reasoning":"line 3 derefs a nil map","is_real":true,"confidence":0.9}`)}
	calls := 0
	j.exec = func(context.Context, string, []string, string) ([]byte, int, error) {
		calls++
		return []byte(streams[min(calls-1, len(streams)-1)]), 0, nil
	}
	if v, err := j.Call(context.Background(), codexRequest()); err != nil || v.ParseError != "" || !v.Decision || v.Attempts != 2 {
		t.Fatalf("the clean retry must be recorded: %+v err=%v", v, err)
	}

	// Commentary before the verdict (a model narrating a tool call) is not a verdict, so the turn did not continue.
	f = &fakeCodex{stdout: codexStreamOf("Let me read the diff first.", `{"reasoning":"line 3 derefs a nil map","is_real":true,"confidence":0.9}`)}
	j = &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if v, err := j.Call(context.Background(), codexRequest()); err != nil || v.ParseError != "" || !v.Decision || f.calls != 1 {
		t.Fatalf("commentary then a verdict is one answer: %+v err=%v calls=%d", v, err, f.calls)
	}
}

// No hook or plugin of the user's may speak in the judge's session (#193).
func TestCodexJudgeRunsWithoutUserHooksOrPlugins(t *testing.T) {
	f := &fakeCodex{stdout: codexStream(`{"reasoning":"r","is_real":false,"confidence":0.2}`)}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.args, " ")
	for _, want := range []string{"-c features.hooks=false", "-c features.plugins=false"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
}
