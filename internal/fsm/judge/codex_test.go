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

// fakeCodex records the invocation and replays a canned event stream.
type fakeCodex struct {
	args   []string
	stdin  string
	stdout string
	code   int
	err    error
	calls  int
}

func (f *fakeCodex) exec(_ context.Context, args []string, stdin string) ([]byte, int, error) {
	f.calls++
	f.args, f.stdin = args, stdin
	return []byte(f.stdout), f.code, f.err
}

func codexStream(text string) string {
	return `{"type":"thread.started","thread_id":"t"}` + "\n" +
		`{"type":"item.completed","item":{"id":"i0","type":"reasoning","text":"ignored"}}` + "\n" +
		`{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":` + quote(text) + `}}` + "\n" +
		"not json at all\n" + // a future CLI line this build does not know
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"cache_write_input_tokens":5,"output_tokens":7,"reasoning_output_tokens":3}}` + "\n"
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// codexClock is a tick-per-call clock; the package's own testClock takes a
// sleep recorder this provider never uses.
func codexClock() Clock {
	n := time.Unix(0, 0)
	return Clock{Now: func() time.Time { n = n.Add(time.Second); return n }, After: func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Unix(0, 0)
		return ch
	}}
}

func codexRequest() Request {
	return Request{Kind: KindAdjudicate, Model: "codex/gpt-5.6-sol", Effort: "medium",
		Input: AdjudicateInput{Diff: "diff --git a/a.go b/a.go", Candidate: run.Finding{IssueText: "nil deref"}}}
}

func TestCodexJudgeHappyPath(t *testing.T) {
	f := &fakeCodex{stdout: codexStream(`{"reasoning":"grounded","is_real":true,"confidence":0.91}`)}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}

	v, err := j.Call(context.Background(), codexRequest())
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !v.Decision || v.Confidence != 0.91 || v.ParseError != "" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.Attempts != 1 || v.Duration != time.Second {
		t.Fatalf("attempts/duration: %d %v", v.Attempts, v.Duration)
	}
	// codex reports input_tokens as the WHOLE prompt with cached_input_tokens a subset of it,
	// and output_tokens as the whole completion with reasoning a subset (verified against the
	// live CLI: two byte-identical prompts return identical input_tokens while cached varies).
	// TokenTotals.Total() sums every field, so the categories must be disjoint or the cached
	// half is billed twice into the convergence budget.
	want := run.TokenTotals{Input: 60, CacheRead: 40, CacheCreate: 5, Output: 4, Reasoning: 3}
	if v.Tokens != want {
		t.Fatalf("tokens: %+v want %+v", v.Tokens, want)
	}
	if got, wantTotal := v.Tokens.Total(), int64(100+5+7); got != wantTotal {
		t.Fatalf("Total() = %d, want %d (prompt %d + cache_write %d + completion %d)", got, wantTotal, 100, 5, 7)
	}
	if v.InputHash == "" || v.Kind != KindAdjudicate || v.Model != "codex/gpt-5.6-sol" {
		t.Fatalf("envelope: %+v", v)
	}
}

func TestCodexJudgeBuildsASafeInvocation(t *testing.T) {
	f := &fakeCodex{stdout: codexStream(`{"reasoning":"r","is_real":false,"confidence":0.2}`)}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.args, " ")
	for _, want := range []string{
		"exec --json",
		"--sandbox read-only",   // a judge must not edit the tree it judges
		"--skip-git-repo-check", // judging is not tied to a repository
		"--color never",
		"-m gpt-5.6-sol", // the codex/ prefix is stripped for the wire
		"-c model_reasoning_effort=medium",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if f.args[len(f.args)-1] != "-" {
		t.Fatalf("the prompt must arrive on stdin, not argv: %v", f.args)
	}
	if !strings.Contains(f.stdin, "code review") || !strings.Contains(f.stdin, "nil deref") {
		t.Fatalf("stdin did not carry the rendered prompt: %q", f.stdin)
	}
	if strings.Contains(joined, "nil deref") {
		t.Fatal("the prompt leaked into argv, where the process table would show it")
	}
}

func TestCodexJudgeFailures(t *testing.T) {
	cases := []struct {
		name string
		f    *fakeCodex
		req  func(Request) Request
		code string
	}{
		{"cannot run", &fakeCodex{err: errors.New("no such file")}, nil, CodeJudgeTransport},
		{"non-zero exit", &fakeCodex{stdout: "", code: 3}, nil, CodeJudgeTransport},
		{"no agent message", &fakeCodex{stdout: `{"type":"turn.completed","usage":{}}` + "\n"}, nil, CodeJudgeResponse},
		{"unknown effort", &fakeCodex{}, func(r Request) Request { r.Effort = "light"; return r }, CodeJudgeEffortUnsupported},
		{"calibration needs medium", &fakeCodex{}, func(r Request) Request { r.Calibration = true; r.Effort = "high"; return r }, CodeJudgeEffortUnsupported},
		{"empty model", &fakeCodex{}, func(r Request) Request { r.Model = CodexPrefix; return r }, CodeJudgeModel},
		{"unknown kind", &fakeCodex{stdout: codexStream("{}")}, func(r Request) Request { r.Kind = "nope"; return r }, CodePromptTemplate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := &codexJudge{exec: c.f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
			r := codexRequest()
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

func TestCodexJudgeReportsTokensEvenWhenTheCallFails(t *testing.T) {
	// A non-zero exit after a completed turn still cost tokens; losing them would
	// under-report the run's spend and weaken the budget convergence check. Each
	// retry costs again, so the totals accumulate across attempts exactly as the
	// HTTP arm's do.
	f := &fakeCodex{code: 1, stdout: `{"type":"turn.completed","usage":{"input_tokens":11,"output_tokens":2}}` + "\n"}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), codexRequest())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if f.calls != MaxAttempts {
		t.Fatalf("a transport failure must be retried: %d attempts, want %d", f.calls, MaxAttempts)
	}
	if v.Attempts != MaxAttempts {
		t.Fatalf("Attempts = %d, want %d", v.Attempts, MaxAttempts)
	}
	if want := int64(11 * MaxAttempts); v.Tokens.Input != want {
		t.Fatalf("tokens must accumulate across attempts: Input=%d want %d", v.Tokens.Input, want)
	}
}

// A codex exec that never returns must not stall the run. The provider had no
// deadline of its own and inherited only the caller's context, so a hung CLI
// hung the FSM.
func TestCodexJudgeBoundsEachAttempt(t *testing.T) {
	var gotDeadline bool
	exec := func(ctx context.Context, _ []string, _ string) ([]byte, int, error) {
		if _, ok := ctx.Deadline(); ok {
			gotDeadline = true
		}
		return []byte(codexStream(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}
	j := &codexJudge{exec: exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	if !gotDeadline {
		t.Fatal("each attempt must carry a deadline, or a hung codex exec stalls the run forever")
	}
}

// A cancelled caller stops the ladder rather than running it out.
func TestCodexJudgeStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeCodex{code: 1}
	// A timer that never fires, so ctx.Done() is the only ready case. codexClock's
	// After hands back a pre-filled channel, which left both select cases ready
	// and let Go pick at random — a 6% flake that was an artifact of the fake
	// clock rather than a real race. A test clock should decide which case is
	// ready; it should never leave that to chance.
	never := Clock{Now: codexClock().Now, After: func(time.Duration) <-chan time.Time { return make(chan time.Time) }}
	j := &codexJudge{exec: func(c context.Context, a []string, s string) ([]byte, int, error) {
		cancel()
		return f.exec(c, a, s)
	}, nonce: func() string { return "n0" }, clock: never}
	if _, err := j.Call(ctx, codexRequest()); err == nil {
		t.Fatal("a cancelled call must return an error")
	}
	if f.calls >= MaxAttempts {
		t.Fatalf("cancellation must stop the ladder early, got %d attempts", f.calls)
	}
}

// route lowercases the model but wireModel stripped the prefix case-sensitively,
// so "Codex/gpt-5.6-sol" routed to the CLI and then passed the unstripped prefix
// through as -m, which codex rejects.
func TestCodexPrefixStrippingMatchesRouting(t *testing.T) {
	for _, model := range []string{"codex/gpt-5.6-sol", "Codex/gpt-5.6-sol", "CODEX/gpt-5.6-sol"} {
		if route(model) != provCodex {
			t.Fatalf("%s must route to the CLI provider", model)
		}
		if got := wireModel(model); got != "gpt-5.6-sol" {
			t.Fatalf("wireModel(%q) = %q, want the bare id", model, got)
		}
	}
}

func TestCodexRoutingAndValidation(t *testing.T) {
	if route("codex/gpt-5.6-sol") != provCodex {
		t.Fatal("codex/ must route to the CLI provider")
	}
	if route("gpt-5.6-sol") != provOpenAI {
		t.Fatal("a bare OpenAI id still routes over HTTP")
	}
	if got := wireModel("codex/gpt-5.6-sol"); got != "gpt-5.6-sol" {
		t.Fatalf("wireModel: %q", got)
	}
	// No key of any kind: the CLI holds the OAuth session.
	if _, err := validate("codex/gpt-5.6-sol", "medium", false, Keys{}); err != nil {
		t.Fatalf("codex must not require an API key: %v", err)
	}
	if err := Preflight("codex/gpt-5.6-sol", "max", false, Keys{}); err != nil {
		t.Fatalf("max is in the CLI effort set: %v", err)
	}
	if err := Preflight("codex/gpt-5.6-sol", "light", false, Keys{}); !errs.Is(err, CodeJudgeEffortUnsupported) {
		t.Fatalf("light is not an effort the API accepts: %v", err)
	}
	// The wider set stays codex-only; HTTP providers keep their vocabulary.
	if err := Preflight("gpt-5.6-sol", "max", false, Keys{OpenAI: "k"}); !errs.Is(err, CodeJudgeEffortUnsupported) {
		t.Fatalf("max must not leak into the HTTP providers: %v", err)
	}
}

func TestCodexModelWithoutARunnerIsRefused(t *testing.T) {
	// Falling back to HTTP would need an API key the caller deliberately did not
	// supply, so this must fail loudly rather than quietly change provider.
	j, err := New(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock())
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), codexRequest())
	if !errs.Is(err, CodeJudgeModel) {
		t.Fatalf("got %v", err)
	}
	if v.Model != "codex/gpt-5.6-sol" {
		t.Fatalf("envelope: %+v", v)
	}
}

func TestNewWithCodexRoutesToTheCLI(t *testing.T) {
	f := &fakeCodex{stdout: codexStream(`{"reasoning":"r","is_real":true,"confidence":0.8}`)}
	j, err := NewWithCodex(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock(), f.exec)
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), codexRequest())
	if err != nil || !v.Decision || f.calls != 1 {
		t.Fatalf("v=%+v err=%v calls=%d", v, err, f.calls)
	}
}

func TestItoa(t *testing.T) {
	for in, want := range map[int]string{0: "0", 7: "7", 42: "42", -3: "-3", 1234567: "1234567"} {
		if got := itoa(in); got != want {
			t.Fatalf("itoa(%d)=%q want %q", in, got, want)
		}
	}
}

// The effort-capable table must match what the API accepts, or Preflight — whose
// whole job is to reject a bad model/effort pair before any network traffic —
// passes a request the provider then rejects with a 400.
//
// Per Anthropic's effort documentation, xhigh is available on Fable 5, Mythos 5,
// Opus 5, Opus 4.8, Opus 4.7 and Sonnet 5. It is NOT available on Opus 4.5,
// Opus 4.6 or Sonnet 4.6, which support max but predate xhigh: "xhigh is a newer
// level; some models that support max don't support xhigh."
func TestAnthropicEffortTableMatchesTheAPI(t *testing.T) {
	keys := Keys{Anthropic: "k"}

	for _, model := range []string{
		"claude-opus-5", "claude-opus-4-8", "claude-opus-4-7",
		"claude-sonnet-5", "claude-fable-5", "claude-mythos-5",
	} {
		if err := Preflight(model, "xhigh", false, keys); err != nil {
			t.Fatalf("%s supports xhigh: %v", model, err)
		}
	}

	for _, model := range []string{"claude-opus-4-5", "claude-opus-4-6", "claude-sonnet-4-6"} {
		err := Preflight(model, "xhigh", false, keys)
		if err == nil {
			t.Fatalf("%s does not support xhigh; Preflight must refuse it rather than let the API 400", model)
		}
		if !errs.Is(err, CodeJudgeEffortUnsupported) {
			t.Fatalf("%s: got %v, want %s", model, err, CodeJudgeEffortUnsupported)
		}
		// The levels they do support must keep working.
		for _, ok := range []string{"low", "medium", "high"} {
			if err := Preflight(model, ok, false, keys); err != nil {
				t.Fatalf("%s at %s: %v", model, ok, err)
			}
		}
	}
}

// A caller cancelled before the first attempt must not spawn codex at all. The
// loop only consulted the context between attempts, so attempt 0 skipped the
// check entirely and an already-cancelled call still ran the CLI once.
func TestCodexJudgeDoesNotStartWhenAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	f := &fakeCodex{stdout: codexStream(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}

	v, err := j.Call(ctx, codexRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if f.calls != 0 {
		t.Fatalf("codex was spawned %d times for an already-cancelled caller", f.calls)
	}
	if v.Kind == "" {
		t.Fatal("the envelope must survive the refusal")
	}
}

// Cancellation during the backoff wait must interrupt the wait, not sit through
// it. The top-of-loop check catches a caller cancelled between attempts; this
// covers the other case — cancelled while the ladder is already sleeping, which
// on a real clock can be a sixteen-second wait.
//
// select evaluates its channel expressions before blocking, so cancelling inside
// the clock seam puts the context in exactly that window, deterministically.
func TestCodexJudgeCancellationInterruptsTheBackoffWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeCodex{code: 1} // fail, so the ladder reaches its first backoff

	clock := Clock{
		Now: codexClock().Now,
		After: func(time.Duration) <-chan time.Time {
			cancel()                    // cancelled while the select is being set up
			return make(chan time.Time) // and this timer never fires
		},
	}
	j := &codexJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: clock}

	if _, err := j.Call(ctx, codexRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if f.calls != 1 {
		t.Fatalf("the ladder must stop in the wait after one attempt, got %d", f.calls)
	}
}

// A usage block whose cached count exceeds the prompt (a malformed or future CLI) must clamp
// rather than produce a negative counter: run/fold.go rejects a negative or oversize token
// field, which would turn a successful judge call into ERR_APPEND_REJECTED.
func TestParseCodexEventsClampsInconsistentUsage(t *testing.T) {
	stream := `{"type":"item.completed","item":{"type":"agent_message","text":"{}"}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":99,"cache_write_input_tokens":0,"output_tokens":3,"reasoning_output_tokens":50}}` + "\n"
	_, tok, found := parseCodexEvents([]byte(stream))
	if !found {
		t.Fatal("agent message not found")
	}
	if tok.Input != 0 {
		t.Errorf("Input = %d, want 0 (10-99 clamped)", tok.Input)
	}
	if tok.Output != 0 {
		t.Errorf("Output = %d, want 0 (3-50 clamped)", tok.Output)
	}
	if tok.CacheRead != 99 || tok.Reasoning != 50 {
		t.Errorf("subset counters not preserved: %+v", tok)
	}
	if tok.Total() < 0 {
		t.Errorf("Total() = %d, must never be negative", tok.Total())
	}
}

// Routing strips the codex/ prefix case-insensitively (trimPrefixFold), so the empty-model
// guard has to match the same way. Comparing against the literal lowercase prefix let
// "CODEX/" through, and the judge was then spawned with an empty -m value: five attempts and
// the backoff ladder wasted, or a verdict recorded under a model id that never answered.
func TestValidateCodexRejectsAnEmptyModelInAnyCase(t *testing.T) {
	for _, model := range []string{"codex/", "CODEX/", "Codex/", "cOdEx/"} {
		err := validateCodex(model, "medium", false)
		if !errs.Is(err, CodeJudgeModel) {
			t.Errorf("validateCodex(%q) = %v, want ERR_JUDGE_MODEL", model, err)
		}
	}
	if err := validateCodex("codex/gpt-5.6-sol", "medium", false); err != nil {
		t.Errorf("a real model must still validate: %v", err)
	}
}
