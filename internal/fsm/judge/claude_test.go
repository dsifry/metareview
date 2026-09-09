package judge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeClaude records the invocation and replays a canned JSON result document.
type fakeClaude struct {
	args   []string
	stdin  string
	stdout string
	code   int
	err    error
	calls  int
}

func (f *fakeClaude) exec(_ context.Context, _ string, args []string, stdin string) ([]byte, int, error) {
	f.calls++
	f.args, f.stdin = args, stdin
	return []byte(f.stdout), f.code, f.err
}

// claudeJSON is one --output-format json document: the shape live-verified
// against claude 2.1.265 on 2026-09-09 (result, is_error, usage with the four
// token fields and thinking as a subset of output).
func claudeJSON(text string) string {
	return `{"type":"result","subtype":"success","is_error":false,"result":` + quote(text) + "," +
		`"usage":{"input_tokens":9,"cache_creation_input_tokens":10306,"cache_read_input_tokens":13572,` +
		`"output_tokens":153,"output_tokens_details":{"thinking_tokens":145}}}` + "\n"
}

func claudeRequest() Request {
	return Request{Kind: KindAdjudicate, Model: "claude-cli/opus", Effort: "medium",
		Input: AdjudicateInput{Diff: "diff --git a/a.go b/a.go", Candidate: run.Finding{IssueText: "nil deref"}}}
}

func TestClaudeJudgeHappyPath(t *testing.T) {
	f := &fakeClaude{stdout: claudeJSON(`{"reasoning":"grounded","is_real":true,"confidence":0.91}`)}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}

	v, err := j.Call(context.Background(), claudeRequest())
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !v.Decision || v.Confidence != 0.91 || v.ParseError != "" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.Attempts != 1 || v.Duration != time.Second {
		t.Fatalf("attempts/duration: %d %v", v.Attempts, v.Duration)
	}
	// The CLI reports input_tokens as the UNCACHED prompt and output_tokens as
	// the whole completion with thinking a subset (live-verified: 9/10306/13572/
	// 153 incl. 145 thinking). TokenTotals.Total() sums every field, so the
	// categories are made disjoint here or the scaffolding tax is billed twice.
	want := run.TokenTotals{Input: 9, CacheRead: 13572, CacheCreate: 10306, Output: 8, Reasoning: 145}
	if v.Tokens != want {
		t.Fatalf("tokens: %+v want %+v", v.Tokens, want)
	}
	if got, wantTotal := v.Tokens.Total(), int64(9+13572+10306+153); got != wantTotal {
		t.Fatalf("Total() = %d, want %d (the full bill, scaffolding tax included)", got, wantTotal)
	}
	if v.InputHash == "" || v.Kind != KindAdjudicate || v.Model != "claude-cli/opus" {
		t.Fatalf("envelope: %+v", v)
	}
}

func TestClaudeJudgeBuildsASafeInvocation(t *testing.T) {
	f := &fakeClaude{stdout: claudeJSON(`{"reasoning":"r","is_real":false,"confidence":0.2}`)}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), claudeRequest()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.args, " ")
	for _, want := range []string{
		"-p",
		"--model opus", // the claude-cli/ prefix is stripped for the wire; aliases pass through
		"--effort medium",
		"--output-format json",
		"--max-turns 1", // a judge answers; it must never start a tool loop
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	// The Haiku-fallback trap: without --append-system-prompt the CLI can
	// silently fall back to Haiku for the work turn even with --model set, and
	// the verdict would be recorded under a model that never answered.
	var system string
	for i, a := range f.args {
		if a == "--append-system-prompt" && i+1 < len(f.args) {
			system = f.args[i+1]
		}
	}
	if system == "" || !strings.Contains(system, "code review") {
		t.Fatalf("the system prompt must always be passed, got %q", system)
	}
	if !strings.Contains(f.stdin, "nil deref") {
		t.Fatalf("stdin did not carry the rendered user prompt: %q", f.stdin)
	}
	if strings.Contains(joined, "nil deref") {
		t.Fatal("the user prompt leaked into argv, where the process table would show it")
	}
}

func TestClaudeJudgeFailures(t *testing.T) {
	cases := []struct {
		name string
		f    *fakeClaude
		req  func(Request) Request
		code string
	}{
		{"cannot run", &fakeClaude{err: errors.New("no such file")}, nil, CodeJudgeTransport},
		{"non-zero exit", &fakeClaude{stdout: "", code: 3}, nil, CodeJudgeTransport},
		{"no result", &fakeClaude{stdout: `{"type":"result","subtype":"success","is_error":false,"result":""}` + "\n"}, nil, CodeJudgeResponse},
		{"declared is_error", &fakeClaude{stdout: `{"type":"result","is_error":true,"result":"boom"}` + "\n"}, nil, CodeJudgeResponse},
		{"not json", &fakeClaude{stdout: "plain text on stdout\n"}, nil, CodeJudgeResponse},
		{"unknown effort", &fakeClaude{}, func(r Request) Request { r.Effort = "light"; return r }, CodeJudgeEffortUnsupported},
		{"calibration needs medium", &fakeClaude{}, func(r Request) Request { r.Calibration = true; r.Effort = "high"; return r }, CodeJudgeEffortUnsupported},
		{"empty model", &fakeClaude{}, func(r Request) Request { r.Model = ClaudeCLIPrefix; return r }, CodeJudgeModel},
		{"unknown kind", &fakeClaude{stdout: claudeJSON("{}")}, func(r Request) Request { r.Kind = "nope"; return r }, CodePromptTemplate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := &claudeJudge{exec: c.f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
			r := claudeRequest()
			if c.req != nil {
				r = c.req(r)
			}
			v, err := j.Call(context.Background(), r)
			if !errs.Is(err, c.code) {
				t.Fatalf("got %v, want %s", err, c.code)
			}
			if v.Kind == "" {
				t.Fatal("the envelope must survive an error")
			}
		})
	}
}

// The silent failure mode: `claude -p` exits 0 and reports a 5xx overload as the
// result string. Treated as text it would fail closed for the wrong reason; the
// ladder must retry it and a later attempt can succeed.
func TestClaudeJudgeRetriesTransientResultErrors(t *testing.T) {
	f := &fakeClaude{}
	exec := func(_ context.Context, _ string, args []string, stdin string) ([]byte, int, error) {
		f.calls++
		f.args, f.stdin = args, stdin
		if f.calls == 1 {
			return []byte(claudeJSON("API Error: 529 Overloaded. This is a server-side issue, please retry.")), 0, nil
		}
		return []byte(claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.8}`)), 0, nil
	}
	j := &claudeJudge{exec: exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), claudeRequest())
	if err != nil {
		t.Fatalf("the ladder must recover a transient 529: %v", err)
	}
	if v.Attempts != 2 || !v.Decision {
		t.Fatalf("verdict: %+v", v)
	}
	// the failed attempt's usage still counts: tokens accumulate across attempts
	if v.Tokens.Input != 18 {
		t.Fatalf("tokens must accumulate across attempts: Input=%d want 18", v.Tokens.Input)
	}
}

// A caller that never stops feeding the ladder 529s gets a transport error, not
// a verdict parsed out of an error message.
func TestClaudeJudgeExhaustsOnTransientResultErrors(t *testing.T) {
	f := &fakeClaude{stdout: claudeJSON("API Error: 500 Internal Server Error")}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), claudeRequest())
	if !errs.Is(err, CodeJudgeTransport) {
		t.Fatalf("got %v, want %s", err, CodeJudgeTransport)
	}
	if f.calls != MaxAttempts {
		t.Fatalf("attempts = %d, want %d", f.calls, MaxAttempts)
	}
	if v.Decision {
		t.Fatal("an exhausted ladder must not record a decision")
	}
}

func TestClaudeJudgeReportsTokensEvenWhenTheCallFails(t *testing.T) {
	f := &fakeClaude{code: 1, stdout: claudeJSON("unused result text")}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), claudeRequest())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if f.calls != MaxAttempts {
		t.Fatalf("a transport failure must be retried: %d attempts, want %d", f.calls, MaxAttempts)
	}
	if want := int64(9 * MaxAttempts); v.Tokens.Input != want {
		t.Fatalf("tokens must accumulate across attempts: Input=%d want %d", v.Tokens.Input, want)
	}
}

// A claude -p that never returns must not stall the run (see the codex twin).
func TestClaudeJudgeBoundsEachAttempt(t *testing.T) {
	var gotDeadline bool
	exec := func(ctx context.Context, _ string, _ []string, _ string) ([]byte, int, error) {
		if _, ok := ctx.Deadline(); ok {
			gotDeadline = true
		}
		return []byte(claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}
	j := &claudeJudge{exec: exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), claudeRequest()); err != nil {
		t.Fatal(err)
	}
	if !gotDeadline {
		t.Fatal("each attempt must carry a deadline, or a hung claude -p stalls the run forever")
	}
}

// A cancelled caller stops the ladder rather than running it out.
func TestClaudeJudgeStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeClaude{code: 1}
	never := Clock{Now: codexClock().Now, After: func(time.Duration) <-chan time.Time { return make(chan time.Time) }}
	j := &claudeJudge{exec: func(c context.Context, d string, a []string, s string) ([]byte, int, error) {
		cancel()
		return f.exec(c, d, a, s)
	}, nonce: func() string { return "n0" }, clock: never}
	if _, err := j.Call(ctx, claudeRequest()); err == nil {
		t.Fatal("a cancelled call must return an error")
	}
	if f.calls >= MaxAttempts {
		t.Fatalf("cancellation must stop the ladder early, got %d attempts", f.calls)
	}
}

// A caller cancelled before the first attempt must not spawn claude at all.
func TestClaudeJudgeDoesNotStartWhenAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := &fakeClaude{stdout: claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}

	v, err := j.Call(ctx, claudeRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if f.calls != 0 {
		t.Fatalf("claude was spawned %d times for an already-cancelled caller", f.calls)
	}
	if v.Kind == "" {
		t.Fatal("the envelope must survive the refusal")
	}
}

// Cancellation during the backoff wait must interrupt the wait, not sit through it.
func TestClaudeJudgeCancellationInterruptsTheBackoffWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeClaude{code: 1} // fail, so the ladder reaches its first backoff

	clock := Clock{
		Now: codexClock().Now,
		After: func(time.Duration) <-chan time.Time {
			cancel()                    // cancelled while the select is being set up
			return make(chan time.Time) // and this timer never fires
		},
	}
	j := &claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: clock}

	if _, err := j.Call(ctx, claudeRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if f.calls != 1 {
		t.Fatalf("the ladder must stop in the wait after one attempt, got %d calls", f.calls)
	}
}

// "claude-cli/" starts with "claude", so the routing order matters: the CLI
// prefix must win over the Anthropic HTTP family, and a bare claude id must
// keep routing to HTTP. Case-insensitive, matching route's lowercasing.
func TestClaudeCLIRouting(t *testing.T) {
	for _, model := range []string{"claude-cli/opus", "Claude-CLI/opus", "CLAUDE-CLI/claude-opus-4-8"} {
		if route(model) != provClaudeCLI {
			t.Fatalf("%s must route to the CLI provider", model)
		}
		if got := wireModel(model); got != strings.ToLower(strings.TrimPrefix(strings.ToLower(model), "claude-cli/")) {
			t.Fatalf("wireModel(%q) = %q, want the bare id", model, got)
		}
	}
	for _, model := range []string{"claude-opus-4-8", "claude-3-haiku", "anthropic/claude-opus-4-8"} {
		if route(model) != provAnthropic {
			t.Fatalf("%s must still route to the Anthropic HTTP API", model)
		}
	}
	// No key of any kind: the CLI holds the logged-in session.
	if _, err := validate("claude-cli/opus", "medium", false, Keys{}); err != nil {
		t.Fatalf("claude-cli must not require an API key: %v", err)
	}
	if err := Preflight("claude-cli/opus", "max", false, Keys{}); err != nil {
		t.Fatalf("max is in the CLI effort set: %v", err)
	}
	if err := Preflight("claude-cli/opus", "light", false, Keys{}); !errs.Is(err, CodeJudgeEffortUnsupported) {
		t.Fatalf("light is not an effort the CLI accepts: %v", err)
	}
	// The wider set stays CLI-only; HTTP providers keep their vocabulary.
	if err := Preflight("claude-opus-4-8", "max", false, Keys{Anthropic: "k"}); !errs.Is(err, CodeJudgeEffortUnsupported) {
		t.Fatalf("max must not leak into the HTTP providers: %v", err)
	}
}

// The empty-model guard must match the prefix stripping's case-insensitivity
// (the codex twin's trap): "CLAUDE-CLI/" must be refused, not spawned empty.
func TestValidateClaudeRejectsAnEmptyModelInAnyCase(t *testing.T) {
	for _, model := range []string{"claude-cli/", "CLAUDE-CLI/", "Claude-Cli/", "cLaUdE-cLi/"} {
		err := validateClaude(model, "medium", false)
		if !errs.Is(err, CodeJudgeModel) {
			t.Errorf("validateClaude(%q) = %v, want ERR_JUDGE_MODEL", model, err)
		}
	}
	if err := validateClaude("claude-cli/opus", "medium", false); err != nil {
		t.Errorf("a real model must still validate: %v", err)
	}
}

// Falling back to HTTP would need an API key the caller deliberately did not
// supply, so a claude-cli/ model without a runner must fail loudly.
func TestClaudeModelWithoutARunnerIsRefused(t *testing.T) {
	j, err := New(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock())
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), claudeRequest())
	if !errs.Is(err, CodeJudgeModel) {
		t.Fatalf("got %v, want ERR_JUDGE_MODEL", err)
	}
	if v.Model != "claude-cli/opus" {
		t.Fatalf("envelope: %+v", v)
	}
}

func TestNewWithCodexAndClaudeRoutesToTheCLI(t *testing.T) {
	f := &fakeClaude{stdout: claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.8}`)}
	j, err := NewWithCodexAndClaude(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock(), nil, f.exec)
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), claudeRequest())
	if err != nil || !v.Decision || f.calls != 1 {
		t.Fatalf("v=%+v err=%v calls=%d", v, err, f.calls)
	}
}

// WithTimeout propagates through routing to the claude arm: the exec must run
// under a context whose deadline reflects the override.
func TestClaudeAttemptTimeoutOverride(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool
	exec := func(ctx context.Context, _ string, _ []string, _ string) ([]byte, int, error) {
		deadline, hasDeadline = ctx.Deadline()
		return []byte(claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}
	j, err := NewWithCodexAndClaude(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock(), nil, exec)
	if err != nil {
		t.Fatal(err)
	}
	j = WithTimeout(j, 500*time.Second)
	before := time.Now()
	if _, err := j.Call(context.Background(), claudeRequest()); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Fatal("claude exec ran without a context deadline")
	}
	if got := deadline.Sub(before); got < 495*time.Second || got > 505*time.Second {
		t.Errorf("claude deadline ~%v, want ~500s", got)
	}
}

// A usage block whose thinking exceeds output (a malformed or future CLI) clamps
// rather than producing a negative counter (see the codex twin).
func TestParseClaudeResultClampsInconsistentUsage(t *testing.T) {
	doc := `{"is_error":false,"result":"{}","usage":{"input_tokens":10,"cache_creation_input_tokens":0,` +
		`"cache_read_input_tokens":99,"output_tokens":3,"output_tokens_details":{"thinking_tokens":50}}}` + "\n"
	_, tok, found, transient := parseClaudeResult([]byte(doc))
	if !found || transient {
		t.Fatalf("found=%v transient=%v, want true/false", found, transient)
	}
	if tok.Input != 10 || tok.CacheRead != 99 {
		t.Errorf("uncached counters not preserved: %+v", tok)
	}
	if tok.Output != 0 {
		t.Errorf("Output = %d, want 0 (3-50 clamped)", tok.Output)
	}
	if tok.Reasoning != 50 {
		t.Errorf("Reasoning = %d, want 50", tok.Reasoning)
	}
}

// The transient detector is the harnesseval rule: only a 5xx server error
// reported as the result string counts; a 4xx or ordinary text does not.
func TestParseClaudeResultTransientDetection(t *testing.T) {
	for _, tc := range []struct {
		result    string
		transient bool
	}{
		{"API Error: 529 Overloaded. This is a server-side issue.", true},
		{"API Error: 500 Internal Server Error", true},
		{"  API Error: 503 Service Unavailable", true}, // leading whitespace tolerated
		{"API Error: 400 Bad Request", false},          // client-side: not transient
		{"The diff introduces a nil dereference", false},
		{"", false},
	} {
		doc := `{"is_error":false,"result":` + quote(tc.result) + `,"usage":{}}` + "\n"
		_, _, _, transient := parseClaudeResult([]byte(doc))
		if transient != tc.transient {
			t.Errorf("result %q: transient=%v want %v", tc.result, transient, tc.transient)
		}
	}
}

// A missing usage block (a future CLI shape or a truncated document) must not
// fabricate counters, and a missing output_tokens_details must not eat output.
func TestParseClaudeResultWithoutUsage(t *testing.T) {
	_, tok, found, _ := parseClaudeResult([]byte(`{"is_error":false,"result":"text"}`))
	if !found {
		t.Fatal("result text not found")
	}
	if tok != (run.TokenTotals{}) {
		t.Fatalf("tokens fabricated: %+v", tok)
	}
	_, tok, _, _ = parseClaudeResult([]byte(`{"is_error":false,"result":"t","usage":{"input_tokens":5,"output_tokens":7}}`))
	if tok.Output != 7 || tok.Reasoning != 0 {
		t.Fatalf("output without thinking details: %+v", tok)
	}
}
