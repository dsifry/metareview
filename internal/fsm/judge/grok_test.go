package judge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// fakeGrok records the invocation and replays a canned JSON envelope. It reads
// the --prompt-file during the call to prove the file existed then, and that it
// is gone afterwards.
type fakeGrok struct {
	dir     string
	args    []string
	stdin   string
	stdout  string
	code    int
	err     error
	calls   int
	present bool // the prompt file existed at call time
}

func (f *fakeGrok) exec(_ context.Context, dir string, args []string, stdin string) ([]byte, int, error) {
	f.calls++
	f.args, f.stdin, f.dir = args, stdin, dir
	if p := promptPathFrom(args); p != "" {
		if _, statErr := os.Stat(p); statErr == nil {
			f.present = true
		}
	}
	return []byte(f.stdout), f.code, f.err
}

func promptPathFrom(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--prompt-file" {
			return args[i+1]
		}
	}
	return ""
}

// grokEnvelope renders the --output-format json document with a spend block.
func grokEnvelope(text string) string {
	b, _ := json.Marshal(map[string]any{
		"text":       text,
		"stopReason": "end_turn",
		"usage": map[string]any{
			"input_tokens": 100, "cache_read_input_tokens": 40,
			"cache_creation_input_tokens": 5, "output_tokens": 7, "reasoning_tokens": 3,
		},
	})
	return string(b)
}

// grokEnvelopeStop renders an envelope with a chosen stop reason and
// structured-output error.
func grokEnvelopeStop(text, stop, soErr string) string {
	b, _ := json.Marshal(map[string]any{"text": text, "stopReason": stop, "structuredOutputError": soErr})
	return string(b)
}

func grokRequest() Request {
	return Request{Kind: KindAdjudicate, Model: "grok/grok-4.7", Effort: "medium",
		Input: AdjudicateInput{Diff: "diff --git a/a.go b/a.go", Candidate: run.Finding{IssueText: "nil deref"}}}
}

func TestGrokJudgeHappyPath(t *testing.T) {
	f := &fakeGrok{stdout: grokEnvelope(`{"reasoning":"grounded","is_real":true,"confidence":0.91}`)}
	j := &grokJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}

	v, err := j.Call(context.Background(), grokRequest())
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !v.Decision || v.Confidence != 0.91 || v.ParseError != "" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.Attempts != 1 || v.Duration != time.Second {
		t.Fatalf("attempts/duration: %d %v", v.Attempts, v.Duration)
	}
	if v.InputHash == "" || v.Kind != KindAdjudicate || v.Model != "grok/grok-4.7" {
		t.Fatalf("envelope: %+v", v)
	}
	// TokenTotals.Total() sums every field, so the buckets must be disjoint or
	// the reasoning half is billed twice into the convergence budget.
	want := run.TokenTotals{Input: 100, CacheRead: 40, CacheCreate: 5, Output: 4, Reasoning: 3}
	if v.Tokens != want {
		t.Fatalf("tokens: %+v want %+v", v.Tokens, want)
	}
	if got, wantTotal := v.Tokens.Total(), int64(100+40+5+7); got != wantTotal {
		t.Fatalf("Total() = %d, want %d", got, wantTotal)
	}
}

func TestGrokJudgeBuildsASafeInvocation(t *testing.T) {
	f := &fakeGrok{stdout: grokEnvelope(`{"reasoning":"r","is_real":false,"confidence":0.2}`)}
	j := &grokJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), grokRequest()); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.args, " ")
	for _, want := range []string{
		"--prompt-file",
		"--verbatim",
		"-m grok-4.7", // the grok/ prefix is stripped for the wire
		"--output-format json",
		"--json-schema",
		// A headless grok has full tool access by default; the allowlist plus the
		// denylist leave no tools (deny wins), without naming every built-in.
		"--tools read_file",
		"--disallowed-tools read_file,search_tool,use_tool",
		"--max-turns 1", // a judge answers; it must never start a tool loop
		"--no-subagents",
		"--disable-web-search",
		"--reasoning-effort medium",
		"--system-prompt-override",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	// The prompt (here, the diff under judgment) travels only in the file, never
	// in argv, where the process table would show it. The system prompt is a
	// static template, so it may travel in argv.
	if strings.Contains(joined, "nil deref") || strings.Contains(joined, "diff --git") {
		t.Fatal("the prompt leaked into argv, where the process table would show it")
	}
	if !f.present {
		t.Fatal("the prompt file must exist while the CLI runs")
	}
	if p := promptPathFrom(f.args); p != "" {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("the prompt file must be removed after the call: %v", err)
		}
	}
	if f.stdin != "" {
		t.Fatalf("grok reads the prompt from --prompt-file, not stdin: %q", f.stdin)
	}
}

func TestGrokJudgeFailures(t *testing.T) {
	verdict := `{"reasoning":"r","is_real":true,"confidence":0.5}`
	cases := []struct {
		name string
		f    *fakeGrok
		req  func(Request) Request
		code string
	}{
		{"cannot run", &fakeGrok{err: errors.New("no such file")}, nil, CodeJudgeTransport},
		{"non-zero exit", &fakeGrok{stdout: "", code: 3}, nil, CodeJudgeTransport},
		{"no result", &fakeGrok{stdout: grokEnvelopeStop("", "end_turn", "")}, nil, CodeJudgeResponse},
		{"not json", &fakeGrok{stdout: "not json"}, nil, CodeJudgeResponse},
		{"max tokens", &fakeGrok{stdout: grokEnvelopeStop(verdict, "max_tokens", "")}, nil, CodeJudgeResponse},
		{"refusal", &fakeGrok{stdout: grokEnvelopeStop(verdict, "refusal", "")}, nil, CodeJudgeResponse},
		{"max turn requests", &fakeGrok{stdout: grokEnvelopeStop(verdict, "max_turn_requests", "")}, nil, CodeJudgeResponse},
		{"cancelled", &fakeGrok{stdout: grokEnvelopeStop(verdict, "cancelled", "")}, nil, CodeJudgeTransport},
		{"structured output error", &fakeGrok{stdout: grokEnvelopeStop(verdict, "end_turn", "invalid schema")}, nil, CodeJudgeTransport},
		{"unknown effort", &fakeGrok{}, func(r Request) Request { r.Effort = "light"; return r }, CodeJudgeEffortUnsupported},
		{"out-of-vocabulary effort", &fakeGrok{}, func(r Request) Request { r.Effort = "max"; return r }, CodeJudgeEffortUnsupported},
		{"calibration needs medium", &fakeGrok{}, func(r Request) Request { r.Calibration = true; r.Effort = "high"; return r }, CodeJudgeEffortUnsupported},
		{"empty model", &fakeGrok{}, func(r Request) Request { r.Model = GrokPrefix; return r }, CodeJudgeModel},
		{"unknown kind", &fakeGrok{stdout: grokEnvelope("{}")}, func(r Request) Request { r.Kind = "nope"; return r }, CodePromptTemplate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := &grokJudge{exec: c.f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
			r := grokRequest()
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

// A structured-output failure is transport, not judgment: it must be retried,
// and a later success must be the verdict.
func TestGrokJudgeRetriesStructuredOutputError(t *testing.T) {
	calls := 0
	j := &grokJudge{exec: func(_ context.Context, _ string, _ []string, _ string) ([]byte, int, error) {
		calls++
		if calls == 1 {
			return []byte(`{"structuredOutputError":"invalid schema"}`), 0, nil
		}
		return []byte(grokEnvelope(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), grokRequest())
	if err != nil || !v.Decision || calls != 2 {
		t.Fatalf("v=%+v err=%v calls=%d", v, err, calls)
	}
}

// A transport failure still spends tokens; each retry costs again, so the
// totals accumulate across attempts exactly as the HTTP arm's do.
func TestGrokJudgeReportsTokensAcrossAttempts(t *testing.T) {
	f := &fakeGrok{code: 1, stdout: grokEnvelope(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j := &grokJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	v, err := j.Call(context.Background(), grokRequest())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if f.calls != MaxAttempts || v.Attempts != MaxAttempts {
		t.Fatalf("a transport failure must be retried: %d calls, %d attempts", f.calls, v.Attempts)
	}
	if want := int64(100 * MaxAttempts); v.Tokens.Input != want {
		t.Fatalf("tokens must accumulate across attempts: Input=%d want %d", v.Tokens.Input, want)
	}
}

func TestGrokJudgeStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeGrok{code: 1}
	never := Clock{Now: codexClock().Now, After: func(time.Duration) <-chan time.Time { return make(chan time.Time) }}
	j := &grokJudge{exec: func(c context.Context, d string, a []string, s string) ([]byte, int, error) {
		cancel()
		return f.exec(c, d, a, s)
	}, nonce: func() string { return "n0" }, clock: never}
	if _, err := j.Call(ctx, grokRequest()); err == nil {
		t.Fatal("a cancelled call must return an error")
	}
	if f.calls >= MaxAttempts {
		t.Fatalf("cancellation must stop the ladder early, got %d attempts", f.calls)
	}
}

func TestGrokJudgeBackoffHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeGrok{code: 1} // attempt 0 fails, so the ladder moves to the backoff wait
	clk := Clock{Now: codexClock().Now, After: func(time.Duration) <-chan time.Time {
		cancel() // cancel while the ladder waits, after the top-of-loop check passed
		return make(chan time.Time)
	}}
	j := &grokJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: clk, attemptTimeout: 5 * time.Second}
	if _, err := j.Call(ctx, grokRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if f.calls != 1 {
		t.Fatalf("cancellation during backoff must stop the ladder, got %d calls", f.calls)
	}
}

func TestGrokJudgeBoundsEachAttempt(t *testing.T) {
	var gotDeadline bool
	exec := func(ctx context.Context, _ string, _ []string, _ string) ([]byte, int, error) {
		if _, ok := ctx.Deadline(); ok {
			gotDeadline = true
		}
		return []byte(grokEnvelope(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}
	j := &grokJudge{exec: exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), grokRequest()); err != nil {
		t.Fatal(err)
	}
	if !gotDeadline {
		t.Fatal("each attempt must carry a deadline, or a hung grok exec stalls the run forever")
	}
}

func TestGrokIsolationAndPromptFileFailures(t *testing.T) {
	// A working directory that cannot be made is a transport failure, retried.
	savedDir := isolatedDir
	t.Cleanup(func() { isolatedDir = savedDir })
	isolatedDir = func() (string, func(), error) { return "", func() {}, errors.New("disk full") }
	j := &grokJudge{exec: func(context.Context, string, []string, string) ([]byte, int, error) {
		t.Fatal("no exec when there is no directory")
		return nil, 0, nil
	}, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), grokRequest()); !errs.Is(err, CodeJudgeTransport) {
		t.Fatalf("got %v", err)
	}
	isolatedDir = savedDir

	// A prompt file that cannot be written is a transport failure, retried.
	savedTemp := grokCreateTemp
	t.Cleanup(func() { grokCreateTemp = savedTemp })
	grokCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("no space") }
	f := &fakeGrok{stdout: grokEnvelope(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j = &grokJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), grokRequest()); !errs.Is(err, CodeJudgeTransport) {
		t.Fatalf("got %v", err)
	}
	if f.calls != 0 {
		t.Fatal("no CLI may run when its prompt file was not written")
	}
}

func TestWriteGrokPromptFileErrors(t *testing.T) {
	savedTemp, savedRemove := grokCreateTemp, grokRemoveFile
	t.Cleanup(func() { grokCreateTemp, grokRemoveFile = savedTemp, savedRemove })

	grokCreateTemp = func(string, string) (*os.File, error) { return nil, errors.New("nope") }
	if _, err := writeGrokPromptFile(t.TempDir(), []byte("x")); err == nil {
		t.Fatal("a failed create must error")
	}

	// A write to a closed file fails; the file must still be removed and the
	// error returned, never a half-written prompt path.
	closed, err := os.CreateTemp(t.TempDir(), "closed-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	removed := ""
	grokCreateTemp = func(string, string) (*os.File, error) { return closed, nil }
	grokRemoveFile = func(name string) error { removed = name; return nil }
	if _, err := writeGrokPromptFile(t.TempDir(), []byte("x")); err == nil {
		t.Fatal("a failed write must error")
	}
	if removed == "" {
		t.Fatal("a failed write must remove the file it created")
	}
}

// grokSchema constrains grok to the one boolean its kind carries.
func TestGrokSchema(t *testing.T) {
	for kind, field := range grokVerdictField {
		schema := grokSchema(kind)
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal([]byte(schema), &doc); err != nil {
			t.Fatalf("%s schema is not JSON: %v", kind, err)
		}
		if _, ok := doc.Properties[field]; !ok {
			t.Fatalf("%s schema is missing %q: %s", kind, field, schema)
		}
		if _, ok := doc.Properties["reasoning"]; !ok {
			t.Fatalf("%s schema is missing reasoning: %s", kind, schema)
		}
	}
	// An unknown kind falls back to still-present's field rather than emitting an
	// empty property name.
	if !strings.Contains(grokSchema("nope"), `"still_present"`) {
		t.Fatal("an unknown kind must fall back to the still-present field")
	}
}

func TestGrokRoutingAndValidation(t *testing.T) {
	for _, model := range []string{"grok/grok-4.7", "Grok/grok-4.7", "GROK/grok-4.7"} {
		if route(model) != provGrok {
			t.Fatalf("%s must route to the CLI provider", model)
		}
		if got := wireModel(model); got != "grok-4.7" {
			t.Fatalf("wireModel(%q) = %q, want the bare id", model, got)
		}
	}
	if route("grok") != provUnknown {
		t.Fatal("a bare grok id must not route (the CLI model is namespaced)")
	}
	// No key of any kind: the CLI holds the session.
	if _, err := validate("grok/grok-4.7", "medium", false, Keys{}); err != nil {
		t.Fatalf("grok must not require an API key: %v", err)
	}
	// Every canonical tier the judge asks for is accepted.
	for _, effort := range []string{"low", "medium", "high", "xhigh"} {
		if err := Preflight("grok/grok-4.7", effort, false, Keys{}); err != nil {
			t.Fatalf("%s must be accepted: %v", effort, err)
		}
	}
	// A level the judge never asks for, and one grok does not list, are refused.
	if err := Preflight("grok/grok-4.7", "max", false, Keys{}); !errs.Is(err, CodeJudgeEffortUnsupported) {
		t.Fatalf("max is outside the judge vocabulary and must be refused: %v", err)
	}
}

func TestGrokModelWithoutARunnerIsRefused(t *testing.T) {
	// Falling back to HTTP would need an API key the caller deliberately did not
	// supply, so this must fail loudly rather than quietly change provider.
	j, err := New(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock())
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), grokRequest())
	if !errs.Is(err, CodeJudgeModel) {
		t.Fatalf("got %v", err)
	}
	if v.Model != "grok/grok-4.7" {
		t.Fatalf("envelope: %+v", v)
	}
}

func TestNewWithCLIRoutesToGrok(t *testing.T) {
	f := &fakeGrok{stdout: grokEnvelope(`{"reasoning":"r","is_real":true,"confidence":0.8}`)}
	j, err := NewWithCLIs(nil, Keys{}, URLs{}, func() string { return "n" }, codexClock(), nil, nil, f.exec)
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), grokRequest())
	if err != nil || !v.Decision || f.calls != 1 {
		t.Fatalf("v=%+v err=%v calls=%d", v, err, f.calls)
	}
}

// writeGrokPromptFile must create the file inside the attempt's isolated dir,
// not a shared temp directory, and it must be private (0600).
func TestGrokPromptFileIsPrivateAndInDir(t *testing.T) {
	dir := t.TempDir()
	path, err := writeGrokPromptFile(dir, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	if filepath.Dir(path) != dir {
		t.Fatalf("prompt file %q is not inside %q", path, dir)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("prompt file mode = %o, want 0600", info.Mode().Perm())
	}
}
