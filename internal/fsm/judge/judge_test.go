package judge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// ---------------------------------------------------------------- J1 provenance + goldens

func readPython(t *testing.T, kind string) (header, body string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "prompts", kind+".python.txt"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.IndexByte(s, '\n')
	return s[:i], s[i+1:]
}

func TestJ1Provenance(t *testing.T) {
	rewrite := map[string]func(string) string{
		"match":      func(s string) string { return s },
		"adjudicate": func(s string) string { return s },
		"still-present": func(s string) string {
			return strings.Replace(s, "{repo and _diff(repo, base_ref)[:30000]}", "{diff}", 1)
		},
	}
	constants := map[string]string{"match": TemplateMatch, "adjudicate": TemplateAdjudicate, "still-present": TemplateStillPresentCalibration}
	for kind, rw := range rewrite {
		header, body := readPython(t, kind)
		if strings.HasSuffix(body, "\n") || !strings.HasSuffix(body, "}}") {
			t.Fatalf("%s: body must end at the literal's }} with no newline", kind)
		}
		sum := sha256.Sum256([]byte(body))
		if !strings.HasSuffix(header, "sha256="+hex.EncodeToString(sum[:])) || !strings.Contains(header, "harnesseval@19ff9a8") {
			t.Fatalf("%s: header %q does not pin the body", kind, header)
		}
		if rw(body) != constants[kind] {
			t.Fatalf("%s: Go constant drifted from the vendored Python literal", kind)
		}
		// sibling layer: fail (not skip) when the repo is present but the literal moved
		src, ok := siblingSource(t, kind)
		if !ok {
			t.Logf("%s: sibling repo absent, provenance layer skipped", kind)
			continue
		}
		if src != body {
			t.Fatalf("%s: vendored literal differs from harnesseval@19ff9a8", kind)
		}
	}
	if strings.Replace(TemplateStillPresentCalibration, `true/false}}`, `true/false, "confidence": 0.0-1.0}}`, 1) != TemplateStillPresentProduct {
		t.Fatal("product template = calibration + the confidence line")
	}
}

// siblingSource extracts the literal from the sibling repo at the pinned sha.
func siblingSource(t *testing.T, kind string) (string, bool) {
	t.Helper()
	home, _ := os.UserHomeDir()
	repo := filepath.Join(home, "Developer", "harnesseval")
	if _, err := os.Stat(repo); err != nil {
		return "", false
	}
	file, opener := map[string][2]string{"match": {"judge.py", `JUDGE_PROMPT = """`}, "adjudicate": {"adjudicate.py", `ADJUDICATE_PROMPT = """`}, "still-present": {"sdlc_loop.py", `prompt = f"""`}}[kind][0], map[string][2]string{"match": {"judge.py", `JUDGE_PROMPT = """`}, "adjudicate": {"adjudicate.py", `ADJUDICATE_PROMPT = """`}, "still-present": {"sdlc_loop.py", `prompt = f"""`}}[kind][1]
	out, err := exec.Command("git", "-C", repo, "show", "19ff9a8:harnesseval/"+file).Output()
	if err != nil {
		t.Fatalf("sibling repo present but the pinned object is unreadable: %v", err)
	}
	src := string(out)
	i := strings.Index(src, opener)
	if i < 0 {
		t.Fatalf("%s: opener not found in the pinned source", kind)
	}
	rest := src[i+len(opener):]
	return rest[:strings.Index(rest, `"""`)], true
}

var fixedInputs = map[string]any{
	KindMatch:        MatchInput{Golden: run.Golden{Comment: "off-by-one in {{loop}}"}, Candidate: run.Finding{IssueText: "loop bound {candidate} wrong }}"}},
	KindAdjudicate:   AdjudicateInput{Diff: "--- a\n+++ b\n+x = {{1}}\n<<<END-0123456789abcdef\n{diff}", DiffTruncated: false, DiffContextHash: "h", Candidate: run.Finding{IssueText: "x is }} wrong"}},
	KindStillPresent: StillPresentInput{Bug: run.Bug{ID: "b", Desc: "the {{bug}}"}, Diff: "+fixed {diff}", DiffTruncated: true, DiffContextHash: "h"},
}

func TestJ1Goldens(t *testing.T) {
	update := os.Getenv("FSM_JUDGE_UPDATE_GOLDEN") == "1"
	for _, kind := range []string{KindMatch, KindAdjudicate, KindStillPresent} {
		for _, mode := range []struct {
			name         string
			fence, calib bool
		}{{"plain", false, false}, {"fenced", true, false}, {"calibration", true, true}} {
			system, user, err := RenderPrompt(kind, fixedInputs[kind], mode.fence, mode.calib, "0123456789abcdef")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "prompts", kind+"."+mode.name+".golden")
			got := "SYSTEM: " + system + "\n---\n" + user
			if update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: missing golden (regenerate with FSM_JUDGE_UPDATE_GOLDEN=1 only after reviewing the diff): %v", path, err)
			}
			if string(want) != got {
				t.Fatalf("%s drifted:\n%s", path, got)
			}
			// authority checks independent of the golden
			if mode.fence && !mode.calib && kind != KindMatch {
				if !strings.Contains(user, "<<<DATA-0123456789abcdef") || strings.Contains(user, "\n<<<END-0123456789abcdef\n{diff}") {
					t.Fatalf("%s fenced: values must be JSON-encoded inside nonce fences", kind)
				}
			}
			if mode.calib && strings.Contains(user, "<<<DATA-") {
				t.Fatalf("%s: calibration is never fenced", kind)
			}
			if kind == KindMatch && strings.Contains(user, "<<<DATA-") {
				t.Fatal("match is never fenced")
			}
			if strings.Contains(user, "{{") && kind == KindMatch && mode.name == "plain" && !strings.Contains(user, "in {{loop}}") {
				t.Fatal("values are inserted verbatim, never rescanned")
			}
			if !strings.Contains(user, `{"reasoning":`) {
				t.Fatalf("%s: {{ }} must unescape to a single brace in the JSON hint", kind)
			}
		}
	}
	// match fenced == unfenced
	_, a, _ := RenderPrompt(KindMatch, fixedInputs[KindMatch], true, false, "n")
	_, b, _ := RenderPrompt(KindMatch, fixedInputs[KindMatch], false, false, "n")
	if a != b {
		t.Fatal("match fenced must equal unfenced")
	}
	// still-present calibration uses the 512 template without the confidence line
	_, c, _ := RenderPrompt(KindStillPresent, fixedInputs[KindStillPresent], true, true, "n")
	if strings.Contains(c, `"confidence"`) || strings.Contains(c, "<<<DATA") {
		t.Fatal("calibration still-present template")
	}
	// template errors
	for _, tpl := range []string{"a } b", "a {nope} b", "a {unterminated"} {
		if _, err := format(tpl, map[string]string{}); !errs.Is(err, CodePromptTemplate) {
			t.Fatalf("%q: %v", tpl, err)
		}
	}
	if out, _ := format("{{x}} {v} }}", map[string]string{"v": "{v}"}); out != "{x} {v} }" {
		t.Fatalf("format: %q", out)
	}
	// wrong input types / unknown kind
	if _, _, err := RenderPrompt(KindMatch, "nope", false, false, "n"); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("wrong input")
	}
	if _, _, err := RenderPrompt(KindAdjudicate, "nope", false, false, "n"); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("wrong input")
	}
	if _, _, err := RenderPrompt(KindStillPresent, "nope", false, false, "n"); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("wrong input")
	}
	if _, _, err := RenderPrompt("zzz", nil, false, false, "n"); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("unknown kind")
	}
}

// ---------------------------------------------------------------- J2/J3 parsing

func TestJ2StripFences(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                               `{"a":1}`,
		"```json\n{\"a\":1}\n```":               `{"a":1}`,
		"```\n{\"a\":1}\n```\ntrailing":         `{"a":1}`,
		"```{\"a\":1}":                          `{"a":1}`,
		" ```json\n{\"a\":1}\n```":              " ```json\n{\"a\":1}\n```",
		"Here you go:\n```json\n{\"a\":1}\n```": "Here you go:\n```json\n{\"a\":1}\n```",
	}
	for in, want := range cases {
		if got := stripFences(in); got != want {
			t.Errorf("%q → %q want %q", in, got, want)
		}
	}
}

func TestJ3Parse(t *testing.T) {
	p, d, c, perr := Parse(KindMatch, `{"reasoning":"r","match":true,"confidence":0.9,"extra":1}`)
	if string(p) != `{"reasoning":"r","match":true,"confidence":0.9}` || !d || c != 0.9 || perr != "" {
		t.Fatalf("match unknown fields ignored: %s %v %v %q", p, d, c, perr)
	}
	p, d, c, perr = Parse(KindAdjudicate, "```json\n{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.7,\"extra\":1}\n```")
	if string(p) != `{"reasoning":"r","is_real":true,"confidence":0.7}` || !d || c != 0.7 || perr != "" {
		t.Fatalf("adjudicate: %s", p)
	}
	p, d, c, perr = Parse(KindStillPresent, `{"reasoning":"r","still_present":false,"extra":1}`)
	if string(p) != `{"reasoning":"r","still_present":false,"confidence":0}` || d || c != 0 || perr != "" {
		t.Fatalf("still-present absent confidence → 0: %s", p)
	}
	// missing bools
	for _, kind := range []string{KindMatch, KindAdjudicate} {
		p, d, _, perr := Parse(kind, `{"reasoning":"r","confidence":0.9}`)
		if p != nil || d || !strings.HasPrefix(perr, "missing ") || !strings.Contains(perr, "raw: ") {
			t.Fatalf("%s missing bool: %s %v %q", kind, p, d, perr)
		}
	}
	p, d, c, perr = Parse(KindStillPresent, `{"reasoning":"r"}`)
	if string(p) != `{"reasoning":"r","still_present":null,"confidence":0}` || !d || c != 0 || !strings.HasPrefix(perr, "missing still_present") {
		t.Fatalf("still-present missing bool: %s %v %q", p, d, perr)
	}
	// non-JSON and string-typed bools
	for _, raw := range []string{`nope`, `{"match":"true"}`, ``} {
		if p, d, _, perr := Parse(KindMatch, raw); p != nil || d || perr == "" {
			t.Fatalf("match %q: %s %v %q", raw, p, d, perr)
		}
		if _, d, _, perr := Parse(KindStillPresent, raw); !d || perr == "" {
			t.Fatalf("still-present %q fails closed: %v %q", raw, d, perr)
		}
	}
	if p, _, _, _ := Parse(KindStillPresent, `{"still_present":"true"}`); p != nil {
		t.Fatal("string-typed bool is a decode error, not a typed null")
	}
	// raw capped in the parse error
	_, _, _, perr = Parse(KindAdjudicate, strings.Repeat("x", 5000))
	if len(perr) > run.MaxShort+64 {
		t.Fatalf("raw not capped: %d", len(perr))
	}
	// over MaxDetail
	big := `{"reasoning":"` + strings.Repeat("r", run.MaxDetail) + `","match":true}`
	if p, _, _, perr := Parse(KindMatch, big); p != nil || !strings.Contains(perr, "MaxDetail") {
		t.Fatal("match over MaxDetail")
	}
	big = `{"reasoning":"` + strings.Repeat("r", run.MaxDetail) + `","still_present":true}`
	if p, d, _, perr := Parse(KindStillPresent, big); p != nil || !d || !strings.Contains(perr, "MaxDetail") {
		t.Fatal("still-present over MaxDetail")
	}
	// adjudicate threshold rows are the kind's; confidence passthrough here
	if _, _, c, _ := Parse(KindAdjudicate, `{"is_real":true,"confidence":0.6999}`); c != 0.6999 {
		t.Fatal("confidence passthrough")
	}
}

// ---------------------------------------------------------------- J4/J5/J6 real judge

type fakeDoer struct {
	reqs   []*http.Request
	bodies [][]byte
	steps  []step
	i      int
}

type step struct {
	status int
	body   string
	err    error
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	f.reqs = append(f.reqs, r)
	f.bodies = append(f.bodies, b)
	s := f.steps[f.i]
	if f.i < len(f.steps)-1 {
		f.i++
	}
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{StatusCode: s.status, Body: io.NopCloser(strings.NewReader(s.body)), Header: http.Header{}}, nil
}

func testClock(sleeps *[]time.Duration) Clock {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	return Clock{
		Now: func() time.Time { now = now.Add(time.Second); return now },
		After: func(d time.Duration) <-chan time.Time {
			*sleeps = append(*sleeps, d)
			ch := make(chan time.Time, 1)
			ch <- now
			return ch
		},
	}
}

const anthOK = `{"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"{\"reasoning\":\"r\",\"match\":true,\"confidence\":0.8}"},{"type":"text","text":" "}],"usage":{"input_tokens":11,"cache_read_input_tokens":13,"cache_creation_input_tokens":17,"output_tokens":19}}`
const oaiOK = `{"choices":[{"message":{"content":"{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.9}"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":30},"completion_tokens_details":{"reasoning_tokens":20}}}`

func newJudge(t *testing.T, d *fakeDoer, sleeps *[]time.Duration) Judge {
	t.Helper()
	j, err := New(d, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(sleeps))
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func body(t *testing.T, d *fakeDoer, i int) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(d.bodies[i], &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestJ4RequestShapes(t *testing.T) {
	ctx := context.Background()
	type cell struct {
		model, effort string
		calib         bool
		want          map[string]any // literal fragments asserted on the body ("" → absent)
		code          string
	}
	cells := []cell{
		{"gpt-5.2", "medium", false, map[string]any{"reasoning_effort": "medium", "temperature": float64(1), "max_completion_tokens": float64(2048)}, ""},
		{"gpt-5.2", "xhigh", false, map[string]any{"reasoning_effort": "high"}, ""},
		{"openai/gpt-5.2", "low", false, map[string]any{"reasoning_effort": "low", "model": "gpt-5.2"}, ""},
		{"glm-4", "xhigh", false, map[string]any{"reasoning_effort": "high", "max_completion_tokens": float64(16384)}, ""},
		{"kimi-k2", "medium", false, map[string]any{"reasoning_effort": "high", "max_completion_tokens": float64(16384)}, ""},
		{"kimi-k2", "xhigh", false, map[string]any{"reasoning_effort": "max"}, ""},
		{"kimi-k2", "low", false, map[string]any{"reasoning_effort": "low"}, ""},
		{"gpt-5.2", "medium", true, map[string]any{"reasoning_effort": "medium", "max_completion_tokens": float64(2048)}, ""},
		{"gpt-5.2", "high", true, nil, CodeJudgeEffortUnsupported},
		{"gpt-5.2", "bogus", false, nil, CodeJudgeEffortUnsupported},
		{"claude-opus-4-5", "medium", true, map[string]any{"thinking": map[string]any{"type": "disabled"}, "temperature": float64(0), "output_config": nil, "max_tokens": float64(2048)}, ""},
		{"claude-opus-4-5", "high", false, map[string]any{"output_config": map[string]any{"effort": "high"}, "thinking": nil, "temperature": float64(0)}, ""},
		{"claude-opus-4-7", "xhigh", false, map[string]any{"output_config": map[string]any{"effort": "xhigh"}, "temperature": nil}, ""},
		{"anthropic/claude-opus-5", "low", false, map[string]any{"output_config": map[string]any{"effort": "low"}, "model": "claude-opus-5", "temperature": nil}, ""},
		{"claude-sonnet-4-5", "medium", false, map[string]any{"thinking": map[string]any{"type": "disabled"}, "temperature": float64(0), "output_config": nil}, ""},
		{"claude-sonnet-4-5", "high", false, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(8192)}, "max_tokens": float64(2048 + 8192), "temperature": float64(1)}, ""},
		{"claude-3-7-sonnet-latest", "xhigh", false, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": float64(32768)}, "max_tokens": float64(2048 + 32768), "temperature": float64(1), "output_config": nil}, ""},
		{"claude-zeta", "medium", false, nil, CodeJudgeModel},
		{"llama-3", "medium", false, nil, CodeJudgeModel},
		{strings.Repeat("claude-", 200), "medium", false, nil, CodeJudgeModel},
		{"", "medium", false, nil, CodeJudgeModel},
	}
	for _, c := range cells {
		d := &fakeDoer{steps: []step{{status: 200, body: oaiOK}}}
		if route(c.model) == provAnthropic {
			d.steps = []step{{status: 200, body: anthOK}}
		}
		var sleeps []time.Duration
		j := newJudge(t, d, &sleeps)
		_, err := j.Call(ctx, Request{Kind: KindAdjudicate, Model: c.model, Effort: c.effort, Input: fixedInputs[KindAdjudicate], Fence: true, Calibration: c.calib})
		if c.code != "" {
			if !errs.Is(err, c.code) || len(d.bodies) != 0 {
				t.Errorf("%s/%s/%v: want %s got %v (calls %d)", c.model, c.effort, c.calib, c.code, err, len(d.bodies))
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s/%s: %v", c.model, c.effort, err)
		}
		b := body(t, d, 0)
		for k, want := range c.want {
			got, present := b[k]
			if want == nil {
				if present {
					t.Errorf("%s/%s: %s must be absent, got %v", c.model, c.effort, k, got)
				}
				continue
			}
			if !present || string(run.MarshalCanonical(got)) != string(run.MarshalCanonical(want)) {
				t.Errorf("%s/%s: %s = %v want %v", c.model, c.effort, k, got, want)
			}
		}
		req := d.reqs[0]
		// calibration sends the reference system prompt unchanged; a judged call carries the addendum
		wantSystem := SystemAdjudicate
		if !c.calib {
			wantSystem += RubricAddendum
		}
		if route(c.model) == provAnthropic {
			if req.URL.String() != "https://api.anthropic.com/v1/messages" || req.Header.Get("x-api-key") != "sk-ant-test" || req.Header.Get("anthropic-version") != "2023-06-01" || req.Header.Get("anthropic-beta") != "" || b["system"] != wantSystem {
				t.Errorf("anthropic request: %s %v", req.URL, req.Header)
			}
			msgs := b["messages"].([]any)
			if len(msgs) != 1 || msgs[0].(map[string]any)["role"] != "user" {
				t.Error("anthropic messages")
			}
		} else {
			if req.URL.String() != "https://api.openai.com/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer sk-test" {
				t.Errorf("openai request: %s", req.URL)
			}
			msgs := b["messages"].([]any)
			if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[0].(map[string]any)["content"] != wantSystem {
				t.Error("openai messages")
			}
		}
		// calibration content: unfenced calibration body; product content: fenced
		user := lastUserContent(b)
		if c.calib && strings.Contains(user, "<<<DATA-") {
			t.Errorf("%s calibration must be unfenced", c.model)
		}
		if !c.calib && !strings.Contains(user, "<<<DATA-0123456789abcdef") {
			t.Errorf("%s product must be fenced", c.model)
		}
		if deadline, ok := req.Context().Deadline(); !ok || time.Until(deadline) > AttemptTimeout || time.Until(deadline) < AttemptTimeout-time.Minute {
			t.Errorf("attempt deadline %v", deadline)
		}
	}
	// still-present max_tokens per mode on both providers; match 1024
	for _, tc := range []struct {
		model string
		kind  string
		calib bool
		want  float64
		key   string
	}{{"gpt-5.2", KindStillPresent, true, 512, "max_completion_tokens"}, {"gpt-5.2", KindStillPresent, false, 1024, "max_completion_tokens"}, {"claude-opus-4-5", KindStillPresent, true, 512, "max_tokens"}, {"claude-opus-5", KindStillPresent, false, 1024, "max_tokens"}, {"claude-opus-5", KindMatch, false, 1024, "max_tokens"}, {"gpt-5.2", KindMatch, false, 1024, "max_completion_tokens"}} {
		d := &fakeDoer{steps: []step{{status: 200, body: oaiOK}}}
		if route(tc.model) == provAnthropic {
			d.steps = []step{{status: 200, body: anthOK}}
		}
		var sleeps []time.Duration
		j := newJudge(t, d, &sleeps)
		if _, err := j.Call(ctx, Request{Kind: tc.kind, Model: tc.model, Effort: "medium", Input: fixedInputs[tc.kind], Fence: true, Calibration: tc.calib}); err != nil {
			t.Fatal(err)
		}
		if b := body(t, d, 0); b[tc.key] != tc.want {
			t.Errorf("%s/%s/%v: %s = %v", tc.model, tc.kind, tc.calib, tc.key, b[tc.key])
		}
		if tc.kind == KindStillPresent {
			user := lastUserContent(body(t, d, 0))
			if tc.calib == strings.Contains(user, `"confidence": 0.0-1.0`) {
				t.Errorf("still-present template per mode (calib=%v)", tc.calib)
			}
		}
	}
	// token accounting + text extraction
	d := &fakeDoer{steps: []step{{status: 200, body: anthOK}}}
	var sleeps []time.Duration
	j := newJudge(t, d, &sleeps)
	v, err := j.Call(ctx, Request{Kind: KindMatch, Model: "claude-opus-5", Effort: "low", Input: fixedInputs[KindMatch]})
	if err != nil || v.Tokens != (run.TokenTotals{Input: 11, CacheRead: 13, CacheCreate: 17, Output: 19}) || !v.Decision || v.Confidence != 0.8 || v.Raw != `{"reasoning":"r","match":true,"confidence":0.8} ` || v.Attempts != 1 || v.Duration != time.Second || v.InputHash == "" || v.Mock {
		t.Fatalf("anthropic verdict: %+v %v", v, err)
	}
	d = &fakeDoer{steps: []step{{status: 200, body: oaiOK}}}
	j = newJudge(t, d, &sleeps)
	v, err = j.Call(ctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]})
	if err != nil || v.Tokens != (run.TokenTotals{Input: 70, CacheRead: 30, Output: 30, Reasoning: 20}) || !v.Decision {
		t.Fatalf("openai verdict: %+v %v", v, err)
	}
	// usage without details; cached > prompt and reasoning > completion clamp; over MaxTokenCounter clamps
	for _, tc := range []struct {
		usage string
		want  run.TokenTotals
	}{
		{`{"prompt_tokens":10,"completion_tokens":5}`, run.TokenTotals{Input: 10, Output: 5}},
		{`{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":20},"completion_tokens_details":{"reasoning_tokens":9}}`, run.TokenTotals{Input: 0, CacheRead: 20, Output: 0, Reasoning: 9}},
		{`{"prompt_tokens":2199023255552,"completion_tokens":1}`, run.TokenTotals{Input: run.MaxTokenCounter, Output: 1}},
	} {
		d = &fakeDoer{steps: []step{{status: 200, body: `{"choices":[{"message":{"content":"{\"is_real\":false}"}}],"usage":` + tc.usage + `}`}}}
		j = newJudge(t, d, &sleeps)
		v, err = j.Call(ctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]})
		if err != nil || v.Tokens != tc.want {
			t.Errorf("usage %s: %+v %v", tc.usage, v.Tokens, err)
		}
	}
	// response errors: missing usage, empty content, non-JSON, effort 400, over 4 MB
	for _, tc := range []struct {
		model  string
		st     step
		code   string
		reason string
	}{
		{"claude-opus-5", step{200, `{"content":[{"type":"text","text":"x"}]}`, nil}, CodeJudgeResponse, "usage"},
		{"claude-opus-5", step{200, `{"content":[{"type":"thinking","thinking":"x"}],"usage":{"input_tokens":1}}`, nil}, CodeJudgeResponse, "empty"},
		{"claude-opus-5", step{200, `not json`, nil}, CodeJudgeResponse, "json"},
		{"gpt-5.2", step{200, `{"choices":[]}`, nil}, CodeJudgeResponse, "usage"},
		{"gpt-5.2", step{200, `{"choices":[],"usage":{"prompt_tokens":1}}`, nil}, CodeJudgeResponse, "empty"},
		{"gpt-5.2", step{200, `nope`, nil}, CodeJudgeResponse, "json"},
		{"claude-opus-5", step{400, `{"error":{"type":"invalid_request_error","message":"output_config is not supported"}}`, nil}, CodeJudgeEffortUnsupported, ""},
		{"gpt-5.2", step{400, `{"error":{"message":"bad"}}`, nil}, CodeJudgeHTTP, ""},
		{"gpt-5.2", step{200, strings.Repeat("x", MaxBody+1), nil}, CodeJudgeResponse, "too_large"},
	} {
		d = &fakeDoer{steps: []step{tc.st}}
		j = newJudge(t, d, &sleeps)
		v, err = j.Call(ctx, Request{Kind: KindAdjudicate, Model: tc.model, Effort: "low", Input: fixedInputs[KindAdjudicate]})
		if !errs.Is(err, tc.code) || (tc.reason != "" && errs.As(err).Field("reason") != tc.reason) || v.Attempts != 1 || len(d.bodies) != 1 {
			t.Errorf("%s %d %q: %v attempts=%d", tc.model, tc.st.status, tc.st.body[:min(20, len(tc.st.body))], err, v.Attempts)
		}
	}
	// empty content keeps the tokens on the failing verdict
	d = &fakeDoer{steps: []step{{200, `{"content":[],"usage":{"input_tokens":7}}`, nil}}}
	j = newJudge(t, d, &sleeps)
	if v, err := j.Call(ctx, Request{Kind: KindMatch, Model: "claude-opus-5", Effort: "low", Input: fixedInputs[KindMatch]}); !errs.Is(err, CodeJudgeResponse) || v.Tokens.Input != 7 {
		t.Fatalf("tokens on failure: %+v %v", v, err)
	}
	// missing keys
	for _, tc := range []struct{ model, prov string }{{"claude-opus-5", "anthropic"}, {"gpt-5.2", "openai"}} {
		jn, _ := New(&fakeDoer{}, Keys{}, URLs{}, func() string { return "n" }, testClock(&sleeps))
		if _, err := jn.Call(ctx, Request{Kind: KindMatch, Model: tc.model, Effort: "low", Input: fixedInputs[KindMatch]}); !errs.Is(err, CodeJudgeKey) || errs.As(err).Field("provider") != tc.prov {
			t.Errorf("key %s: %v", tc.model, err)
		}
	}
	// prompt template error surfaces (wrong input type)
	if _, err := j.Call(ctx, Request{Kind: KindMatch, Model: "gpt-5.2", Effort: "low", Input: "bad"}); !errs.Is(err, CodePromptTemplate) {
		t.Fatal("render error")
	}
}

func lastUserContent(b map[string]any) string {
	msgs := b["messages"].([]any)
	return msgs[len(msgs)-1].(map[string]any)["content"].(string)
}

func TestJ5Retry(t *testing.T) {
	ctx := context.Background()
	over := `{"error":{"type":"overloaded_error","message":"busy"}}`
	rows := []struct {
		name     string
		steps    []step
		sleeps   []time.Duration
		code     string
		attempts int
		tokens   int64
	}{
		{"429x4", []step{{429, "", nil}, {429, "", nil}, {429, "", nil}, {429, "", nil}, {200, oaiOK, nil}}, []time.Duration{10 * time.Second, 30 * time.Second, 90 * time.Second, 120 * time.Second}, "", 5, 70},
		{"overloaded-529x4", []step{{529, over, nil}, {529, over, nil}, {529, over, nil}, {529, over, nil}, {200, oaiOK, nil}}, []time.Duration{10 * time.Second, 30 * time.Second, 90 * time.Second, 120 * time.Second}, "", 5, 70},
		{"overloaded-503", []step{{503, over, nil}, {200, oaiOK, nil}}, []time.Duration{10 * time.Second}, "", 2, 70},
		{"5xx-plain-x4", []step{{529, "busy", nil}, {500, "", nil}, {502, "", nil}, {503, "", nil}, {200, oaiOK, nil}}, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, "", 5, 70},
		{"transport-x4", []step{{0, "", errors.New("dial")}, {0, "", errors.New("dial")}, {0, "", errors.New("dial")}, {0, "", errors.New("dial")}, {200, oaiOK, nil}}, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, "", 5, 70},
		{"mixed", []step{{429, "", nil}, {500, "", nil}, {429, "", nil}, {200, oaiOK, nil}}, []time.Duration{10 * time.Second, 2 * time.Second, 90 * time.Second}, "", 4, 70},
		{"5xx-x5", []step{{500, "", nil}, {500, "", nil}, {500, "", nil}, {500, "", nil}, {500, "", nil}}, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, CodeJudgeHTTP, 5, 0},
		{"transport-x5", []step{{0, "", errors.New("dial")}}, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, CodeJudgeTransport, 5, 0},
		{"200-overloaded-text", []step{{200, `{"choices":[{"message":{"content":"overloaded {\"is_real\":true}"}}],"usage":{"prompt_tokens":1}}`, nil}}, nil, "", 1, 1},
		{"400-immediate", []step{{400, `{"error":{"message":"bad request"}}`, nil}}, nil, CodeJudgeHTTP, 1, 0},
		{"redirect-terminal", []step{{0, "", errs.E(CodeJudgeRedirect, "refused")}}, nil, CodeJudgeRedirect, 1, 0},
	}
	for _, r := range rows {
		d := &fakeDoer{steps: r.steps}
		var sleeps []time.Duration
		j := newJudge(t, d, &sleeps)
		v, err := j.Call(ctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]})
		if (r.code == "") != (err == nil) || (r.code != "" && !errs.Is(err, r.code)) {
			t.Errorf("%s: err %v", r.name, err)
		}
		if len(sleeps) != len(r.sleeps) || v.Attempts != r.attempts || v.Tokens.Input != r.tokens {
			t.Errorf("%s: sleeps %v attempts %d tokens %d", r.name, sleeps, v.Attempts, v.Tokens.Input)
			continue
		}
		for i := range sleeps {
			if sleeps[i] != r.sleeps[i] {
				t.Errorf("%s: sleep %d = %s want %s", r.name, i, sleeps[i], r.sleeps[i])
			}
		}
	}
	// tokens sum over attempts when earlier attempts carried usage (empty content then success)
	d := &fakeDoer{steps: []step{{200, `{"choices":[],"usage":{"prompt_tokens":5}}`, nil}, {200, oaiOK, nil}}}
	var sleeps []time.Duration
	j := newJudge(t, d, &sleeps)
	if v, err := j.Call(ctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]}); !errs.Is(err, CodeJudgeResponse) || v.Tokens.Input != 5 {
		t.Fatalf("empty content is fatal, tokens kept: %+v %v", v, err)
	}
	// ctx cancelled during a backoff sleep returns ctx.Err() immediately
	cctx, cancel := context.WithCancel(ctx)
	blocking := Clock{Now: func() time.Time { return time.Unix(0, 0) }, After: func(time.Duration) <-chan time.Time { cancel(); return make(chan time.Time) }}
	jc, _ := New(&fakeDoer{steps: []step{{500, "", nil}}}, Keys{OpenAI: "k"}, URLs{}, func() string { return "n" }, blocking)
	if v, err := jc.Call(cctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]}); !errors.Is(err, context.Canceled) || v.Attempts != 2 {
		t.Fatalf("cancel during sleep: %v %d", err, v.Attempts)
	}
	// transport error while ctx is already cancelled → ctx.Err(), fatal
	cctx2, cancel2 := context.WithCancel(ctx)
	cancel2()
	d = &fakeDoer{steps: []step{{0, "", errors.New("dial")}}}
	j = newJudge(t, d, &sleeps)
	if _, err := j.Call(cctx2, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled transport: %v", err)
	}
	// body read error is a transport error (retried)
	d = &fakeDoer{steps: []step{{200, "", nil}}}
	rd := &readErrDoer{}
	jr, _ := New(rd, Keys{OpenAI: "k"}, URLs{}, func() string { return "n" }, testClock(&sleeps))
	sleeps = nil
	if _, err := jr.Call(ctx, Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]}); !errs.Is(err, CodeJudgeTransport) || len(sleeps) != 4 {
		t.Fatalf("body read error: %v %v", err, sleeps)
	}
	_ = d
}

type readErrDoer struct{}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("conn reset") }

func (readErrDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(errReader{})}, nil
}

func TestJ6URLsRoutingRedirect(t *testing.T) {
	ok := []string{"https://api.example.com", "https://api.example.com/", "http://localhost:8080", "http://127.0.0.1:1", "http://[::1]:9"}
	for _, u := range ok {
		if _, err := New(&fakeDoer{}, Keys{}, URLs{Anthropic: u, OpenAI: u}, nil, Clock{}); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	bad := map[string]string{"http://LOCALHOST": "scheme", "http://localhost.evil.com": "scheme", "http://user:pw@localhost": "userinfo", "https://api.example.com/v1": "path", "https://api.example.com/?x=1": "path", "https://api.example.com/#f": "path", "ftp://x": "scheme", "http://[::1": "parse", "https://other.host": ""}
	for u, reason := range bad {
		_, err := New(&fakeDoer{}, Keys{}, URLs{OpenAI: u}, nil, Clock{})
		if u == "https://other.host" {
			if err != nil {
				t.Errorf("https other host must be accepted: %v", err)
			}
			continue
		}
		if !errs.Is(err, CodeJudgeURL) || errs.As(err).Field("reason") != reason {
			t.Errorf("%s: %v", u, err)
		}
	}
	// trailing slash stripped
	j, _ := New(&fakeDoer{steps: []step{{200, oaiOK, nil}}}, Keys{OpenAI: "k"}, URLs{OpenAI: "http://localhost:1/"}, func() string { return "n" }, testClock(new([]time.Duration)))
	rj := j.(*realJudge)
	if rj.urls.OpenAI != "http://localhost:1" || rj.urls.Anthropic != DefaultURLs.Anthropic {
		t.Fatalf("urls: %+v", rj.urls)
	}
	// routing
	for m, p := range map[string]provider{"claude-x": provAnthropic, "anthropic/x": provAnthropic, "gpt-5": provOpenAI, "openai/x": provOpenAI, "glm-4": provOpenAI, "kimi-k2": provOpenAI, "llama": provUnknown} {
		if route(m) != p {
			t.Errorf("route %s", m)
		}
	}
	// real client refuses same-host and cross-host redirects
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()
	client := NewHTTPClient(5 * time.Second)
	for _, path := range []string{"/same", "/cross"} {
		_, err := client.Get(srv.URL + path)
		if err == nil || !errs.Is(err, CodeJudgeRedirect) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	// through the judge: redirect refused → terminal, one attempt, no sleeps
	var sleeps []time.Duration
	jr, _ := New(client, Keys{OpenAI: "k"}, URLs{OpenAI: strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)}, func() string { return "n" }, testClock(&sleeps))
	_ = jr
	jr2, _ := New(NewHTTPClient(5*time.Second), Keys{OpenAI: "k"}, URLs{OpenAI: srv.URL}, func() string { return "n" }, testClock(&sleeps))
	v, err := jr2.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate]})
	if !errs.Is(err, CodeJudgeRedirect) || v.Attempts != 1 || len(sleeps) != 0 {
		t.Fatalf("redirect through judge: %v %d %v", err, v.Attempts, sleeps)
	}
}

// ---------------------------------------------------------------- J7 mock, J9 hashes, CutDiff

func TestJ7Mock(t *testing.T) {
	ctx := context.Background()
	hash := InputHash(fixedInputs[KindMatch])
	m := NewMock(Script{Calls: map[ScriptKey]ScriptRow{
		{KindMatch, "adjudicate", 2, 5}:    {Raw: `{"match":true,"confidence":0.4}`, Tokens: run.TokenTotals{Input: 3}, ExpectModel: "gpt-5.2", ExpectInputHash: hash},
		{KindMatch, "adjudicate", 2, 6}:    {Error: CodeJudgeHTTP},
		{KindStillPresent, "verify", 0, 0}: {Raw: `garbage`},
	}})
	v, err := m.Call(ctx, Request{Kind: KindMatch, Node: "adjudicate", Iter: 2, Index: 5, Model: "gpt-5.2", Input: fixedInputs[KindMatch]})
	if err != nil || !v.Decision || v.Confidence != 0.4 || v.Tokens.Input != 3 || !v.Mock || v.Attempts != 1 || v.InputHash != hash {
		t.Fatalf("scripted: %+v %v", v, err)
	}
	_, err = m.Call(ctx, Request{Kind: KindMatch, Node: "adjudicate", Iter: 2, Index: 6, Model: "gpt-5.2", Input: fixedInputs[KindMatch]})
	if !errs.Is(err, CodeJudgeHTTP) {
		t.Fatalf("error row: %v", err)
	}
	if v, err := m.Call(ctx, Request{Kind: KindStillPresent, Node: "verify", Index: 0, Input: fixedInputs[KindStillPresent]}); err != nil || !v.Decision || v.ParseError == "" {
		t.Fatalf("raw through the real parser: %+v %v", v, err)
	}
	// near-miss keys
	for _, r := range []Request{{Kind: KindAdjudicate, Node: "adjudicate", Iter: 2, Index: 5}, {Kind: KindMatch, Node: "discover", Iter: 2, Index: 5}, {Kind: KindMatch, Node: "adjudicate", Iter: 1, Index: 5}, {Kind: KindMatch, Node: "adjudicate", Iter: 2, Index: 4}} {
		if _, err := m.Call(ctx, r); !errs.Is(err, CodeMockUnscripted) {
			t.Fatalf("near miss %+v: %v", r, err)
		}
	}
	// expect mismatches
	if _, err := m.Call(ctx, Request{Kind: KindMatch, Node: "adjudicate", Iter: 2, Index: 5, Model: "other", Input: fixedInputs[KindMatch]}); !errs.Is(err, CodeMockExpect) || errs.As(err).Field("field") != "model" {
		t.Fatalf("expect model: %v", err)
	}
	if _, err := m.Call(ctx, Request{Kind: KindMatch, Node: "adjudicate", Iter: 2, Index: 5, Model: "gpt-5.2", Input: "other"}); !errs.Is(err, CodeMockExpect) || errs.As(err).Field("field") != "input_hash" {
		t.Fatalf("expect hash: %v", err)
	}
	if calls := m.Calls(); len(calls) != 9 || calls[1].Index != 6 {
		t.Fatalf("Calls includes errored requests: %d", len(calls))
	}
}

func TestJ9HashesAndCut(t *testing.T) {
	pins := map[string]string{
		KindMatch:        "5302bfa551e9c8ae8cae9d8eccc5a68f8b884cd6ca174bfe5f16d33bc98434be",
		KindAdjudicate:   "e9e5cbe2dbc4de0a494a9b3f1ad2bad897e2df3da0ad3ff312e81b560ca3e6cc",
		KindStillPresent: "5e26ce999b9e4c22f353fc756d19178f5287001017933b36a5722cd02d866770",
	}
	inputs := map[string]any{
		KindMatch:        MatchInput{Golden: run.Golden{Comment: "g"}, Candidate: run.Finding{IssueText: "c"}},
		KindAdjudicate:   AdjudicateInput{Diff: "d", DiffContextHash: "h", Candidate: run.Finding{IssueText: "c"}},
		KindStillPresent: StillPresentInput{Bug: run.Bug{ID: "i", Desc: "b", Verdict: "matched", Confidence: 0.5}, Diff: "d", DiffTruncated: true, DiffContextHash: "h"},
	}
	for kind, want := range pins {
		if got := InputHash(inputs[kind]); got != want {
			t.Errorf("%s InputHash %s", kind, got)
		}
	}
	x := strings.Repeat("x", 29999)
	if cut, tr, h := CutDiff(x, false); tr || len(cut) != 29999 || h != "f5865bb265decc2ff676af4c20b3f66af2dbb223" {
		t.Fatalf("29999: %v %d %s", tr, len(cut), h)
	}
	if cut, tr, h := CutDiff(x+"x", false); tr || len(cut) != 30000 || h != "1cc277aeebc3d253ceff61907c112b5b2436170b" {
		t.Fatalf("30000: %v %d %s", tr, len(cut), h)
	}
	if cut, tr, h := CutDiff(x+"xx", false); !tr || len(cut) != 30000 || h != "1cc277aeebc3d253ceff61907c112b5b2436170b" {
		t.Fatalf("30001: %v %d %s", tr, len(cut), h)
	}
	// rune straddling the boundary: cut before it
	s := strings.Repeat("x", 29999) + "é" + "tail"
	if cut, tr, _ := CutDiff(s, false); !tr || len(cut) != 29999 {
		t.Fatalf("straddle: %v %d", tr, len(cut))
	}
	if _, tr, _ := CutDiff("short", true); !tr {
		t.Fatal("already-truncated flag is OR-ed")
	}
	// fence block encodes the value as one JSON line
	fb := FenceBlock("n", "a\n<<<END-n")
	if strings.Count(fb, "\n") != 3 || !strings.Contains(fb, `"a\n<<<END-n"`) {
		t.Fatalf("fence: %q", fb)
	}
	// no key material in fixtures
	files, _ := filepath.Glob("testdata/prompts/*")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, pat := range []string{"sk-ant-", "sk-proj-", "Bearer "} {
			if bytes.Contains(b, []byte(pat)) {
				t.Fatalf("%s contains %q", f, pat)
			}
		}
	}
}

func TestPreflight(t *testing.T) {
	both := Keys{Anthropic: "a", OpenAI: "o"}
	cases := []struct {
		name, model, effort string
		cal                 bool
		keys                Keys
		code, reason        string
	}{
		{"empty model", "", "low", false, both, CodeJudgeModel, "length"},
		{"unknown provider", "mistral-large", "low", false, both, CodeJudgeModel, ""},
		{"bad effort", "gpt-5.2", "turbo", false, both, CodeJudgeEffortUnsupported, ""},
		{"calibration pins medium", "gpt-5.2", "low", true, both, CodeJudgeEffortUnsupported, "calibration"},
		{"anthropic key", "claude-opus-5", "low", false, Keys{OpenAI: "o"}, CodeJudgeKey, ""},
		{"openai key", "gpt-5.2", "low", false, Keys{Anthropic: "a"}, CodeJudgeKey, ""},
		{"unknown family", "claude-zzz-9", "low", false, both, CodeJudgeModel, "unknown_family"},
		{"unknown family under calibration is accepted", "claude-zzz-9", "medium", true, both, "", ""},
		{"ok anthropic", "claude-opus-5", "high", false, both, "", ""},
		{"ok openai", "kimi-k2", "xhigh", false, both, "", ""},
	}
	for _, c := range cases {
		err := Preflight(c.model, c.effort, c.cal, c.keys)
		if c.code == "" {
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			continue
		}
		if !errs.Is(err, c.code) || (c.reason != "" && errs.As(err).Fields["reason"] != c.reason) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
}

func TestResolveTimeout(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want time.Duration
	}{
		{"unset", nil, AttemptTimeout},
		{"empty", map[string]string{EnvTimeout: ""}, AttemptTimeout},
		{"positive", map[string]string{EnvTimeout: "600"}, 600 * time.Second},
		{"whitespace-trimmed", map[string]string{EnvTimeout: " 45 "}, 45 * time.Second},
		{"zero-ignored", map[string]string{EnvTimeout: "0"}, AttemptTimeout},
		{"negative-ignored", map[string]string{EnvTimeout: "-30"}, AttemptTimeout},
		{"malformed-ignored", map[string]string{EnvTimeout: "abc"}, AttemptTimeout},
	}
	for _, c := range cases {
		getenv := func(k string) string { return c.env[k] }
		if got := ResolveTimeout(getenv); got != c.want {
			t.Errorf("%s: ResolveTimeout = %v, want %v", c.name, got, c.want)
		}
	}
	// A nil getenv falls back to the default rather than panicking.
	if got := ResolveTimeout(nil); got != AttemptTimeout {
		t.Errorf("nil getenv: %v, want %v", got, AttemptTimeout)
	}
}

// The per-attempt HTTP request must carry a context deadline that reflects the configured timeout:
// the AttemptTimeout default with no override, and the WithTimeout value when set. WithTimeout on a
// non-HTTP judge, or with a non-positive duration, is a no-op.
func TestWithTimeoutSetsAttemptDeadline(t *testing.T) {
	cases := []struct {
		name  string
		apply time.Duration
		want  time.Duration
	}{
		{"default", 0, AttemptTimeout},
		{"override", 500 * time.Second, 500 * time.Second},
		{"non-positive-noop", -1, AttemptTimeout},
	}
	for _, c := range cases {
		d := &fakeDoer{steps: []step{{status: 200, body: oaiOK}}}
		var sleeps []time.Duration
		j := newJudge(t, d, &sleeps)
		if c.apply != 0 {
			j = WithTimeout(j, c.apply)
		}
		before := time.Now()
		if _, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: fixedInputs[KindAdjudicate], Fence: true}); err != nil {
			t.Fatalf("%s: Call: %v", c.name, err)
		}
		dl, ok := d.reqs[0].Context().Deadline()
		if !ok {
			t.Fatalf("%s: request carried no deadline", c.name)
		}
		got := dl.Sub(before)
		if got < c.want-5*time.Second || got > c.want+5*time.Second {
			t.Errorf("%s: attempt deadline ~%v, want ~%v", c.name, got, c.want)
		}
	}
	// WithTimeout on a non-HTTP judge (a mock) returns it unchanged.
	m := NewMock(Script{})
	if WithTimeout(m, 500*time.Second) != Judge(m) {
		t.Error("WithTimeout must return a non-HTTP judge unchanged")
	}
	// A non-positive duration is a no-op: the SAME judge is returned, not a clone. This pins the
	// `d <= 0` boundary (d == 0 must be treated as no-op, which `d < 0` would not).
	base := newJudge(t, &fakeDoer{steps: []step{{status: 200, body: oaiOK}}}, new([]time.Duration))
	if WithTimeout(base, 0) != base {
		t.Error("WithTimeout(j, 0) must return the same judge unchanged")
	}
	if WithTimeout(base, -1) != base {
		t.Error("WithTimeout(j, -1) must return the same judge unchanged")
	}
}
