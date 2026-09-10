package judge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
)

// The output-cap retry corpus (0.12): some gateways answer a too-small max_tokens with a
// 400 instead of a truncation, and before the retry ladder that 400 was terminal — one
// unfinished verdict poisoned the whole call (metareview#159). These rows pin the ladder:
// a cap 400 is retried ONCE at 4× the cap, byte-identical otherwise; a second cap failure is
// terminal; a 400 that is not a cap failure stays immediate-fatal; and the bumped request
// really carries the raised cap on both provider shapes.

const lunarouteCap = `{"error":{"message":"Could not finish the message because max_tokens or model output limit was reached"}}`
const genericCap = `{"error":{"message":"Output limit was reached before the reply could finish"}}`

func TestCapRetryLadder(t *testing.T) {
	ctx := context.Background()
	rows := []struct {
		name     string
		model    string
		steps    []step
		capKey   string
		baseCap  int
		sleeps   []time.Duration
		code     string
		attempts int
	}{
		{
			name: "openai-cap-then-ok", model: "gpt-5.2",
			steps:  []step{{400, lunarouteCap, nil}, {200, oaiOK, nil}},
			capKey: "max_completion_tokens", baseCap: 2048,
			sleeps: nil, attempts: 1, // the raise re-runs the slot, immediately
		},
		{
			name: "anthropic-cap-then-ok", model: "claude-opus-4-5",
			steps:  []step{{400, genericCap, nil}, {200, anthOK, nil}},
			capKey: "max_tokens", baseCap: 2048,
			sleeps: nil, attempts: 1, // the raise re-runs the slot, immediately
		},
		{
			// GLM/Kimi requests are floored at 16384 by prepare; the raise is 4x the
			// EFFECTIVE cap, not the kind's base.
			name: "glm-floor-then-ok", model: "glm-5.3-background",
			steps:  []step{{400, lunarouteCap, nil}, {200, oaiOK, nil}},
			capKey: "max_completion_tokens", baseCap: 16384,
			// the raised retry re-runs the capped attempt's slot, immediately (the fix is
			// deterministic): one attempt slot, no backoff
			sleeps: nil, attempts: 1,
		},
		{
			name: "cap-twice-terminal", model: "gpt-5.2",
			steps:  []step{{400, lunarouteCap, nil}, {400, lunarouteCap, nil}},
			capKey: "max_completion_tokens", baseCap: 2048,
			sleeps: nil, code: CodeJudgeHTTP, attempts: 1,
		},
		{
			// A 400 that carries an effort marker but no cap phrasing must NOT enter the
			// cap ladder: misrouting it would burn the bump on a request that can never
			// succeed.
			name: "plain-400-stays-fatal", model: "gpt-5.2",
			steps:  []step{{400, `{"error":{"message":"bad request"}}`, nil}},
			capKey: "max_completion_tokens", baseCap: 2048,
			code: CodeJudgeHTTP, attempts: 1,
		},
	}
	for _, r := range rows {
		d := &fakeDoer{steps: r.steps}
		var sleeps []time.Duration
		j := newJudge(t, d, &sleeps)
		v, err := j.Call(ctx, Request{Kind: KindAdjudicate, Model: r.model, Effort: "medium", Input: fixedInputs[KindAdjudicate]})
		if (r.code == "") != (err == nil) || (r.code != "" && !errs.Is(err, r.code)) {
			t.Errorf("%s: err %v", r.name, err)
			continue
		}
		if v.Attempts != r.attempts {
			t.Errorf("%s: attempts %d want %d", r.name, v.Attempts, r.attempts)
		}
		if len(sleeps) != len(r.sleeps) {
			t.Errorf("%s: sleeps %v want %v", r.name, sleeps, r.sleeps)
			continue
		}
		for i := range sleeps {
			if sleeps[i] != r.sleeps[i] {
				t.Errorf("%s: sleep %d = %s want %s", r.name, i, sleeps[i], r.sleeps[i])
			}
		}
		// The bumped request carries 4x the original cap and nothing else changed.
		// (Single-step rows end on their first request — there is no second body.)
		if len(r.steps) > 1 {
			first, second := body(t, d, 0), body(t, d, 1)
			if first[r.capKey] != float64(r.baseCap) {
				t.Errorf("%s: first request cap %v want %d", r.name, first[r.capKey], r.baseCap)
			}
			if second[r.capKey] != float64(r.baseCap*4) {
				t.Errorf("%s: second request cap %v want %d", r.name, second[r.capKey], r.baseCap*4)
			}
			delete(first, r.capKey)
			delete(second, r.capKey)
			fb, _ := json.Marshal(first)
			sb, _ := json.Marshal(second)
			if string(fb) != string(sb) {
				t.Errorf("%s: request changed beyond the cap", r.name)
			}
		}
	}
}

// TestIsOutputCapBody pins the marker set: each observed phrasing matches; a body that
// merely says "finish the message because" with no cap token does NOT (the tightened
// contract: every marker carries "max_tokens" or "output limit", so content-filter and
// rejection bodies phrased that way stay immediate-fatal instead of burning a 4x retry).
func TestIsOutputCapBody(t *testing.T) {
	for _, body := range []string{
		lunarouteCap,
		genericCap,
		`{"error":{"message":"Could not FINISH THE MESSAGE because max_tokens or model output limit WAS REACHED"}}`,
	} {
		if !isOutputCapBody([]byte(body)) {
			t.Errorf("must match: %s", body)
		}
	}
	for _, body := range []string{
		`{"error":{"message":"bad request"}}`,
		`{"error":{"message":"effort level unsupported"}}`,
		`{}`,
		`{"error":{"message":"could not finish the message because the request was rejected"}}`,
		`{"error":{"message":"could not FINISH THE MESSAGE because the model ran out"}}`,
	} {
		if isOutputCapBody([]byte(body)) {
			t.Errorf("must not match: %s", body)
		}
	}
}

// TestNonCapFinishPhraseStaysFatal: a 400 whose body contains the loose phrase but no cap
// token must stay immediate-fatal at the Call level — no 4x retry, no wasted budget.
func TestNonCapFinishPhraseStaysFatal(t *testing.T) {
	d := &plain400Doer{body: `{"error":{"message":"could not finish the message because the request was rejected"}}`}
	j, err := New(d, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(nil))
	if err != nil {
		t.Fatal(err)
	}
	_, err = j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "medium", Input: fixedInputs[KindAdjudicate]})
	if err == nil {
		t.Fatal("a non-cap 400 is fatal")
	}
	if d.calls != 1 {
		t.Errorf("calls %d want 1 (no retry)", d.calls)
	}
}

// TestBothMarkersPrecedence: a 400 carrying BOTH a cap marker and effort text classifies
// as a cap retry (the cap branch is checked first) — pin the ordering, because a swap
// would reclassify cap failures as terminal effort rejections.
func TestBothMarkersPrecedence(t *testing.T) {
	var sleeps []time.Duration
	j, err := New(&bothMarkersDoer{}, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(&sleeps))
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "medium", Input: fixedInputs[KindAdjudicate]})
	if err != nil {
		t.Fatalf("cap marker must win: %v", err)
	}
	if !v.CapRaised || v.Attempts != 1 {
		t.Errorf("cap-first precedence: raised=%v attempts=%d", v.CapRaised, v.Attempts)
	}
}

type bothMarkersDoer struct{ i int }

func (b *bothMarkersDoer) Do(*http.Request) (*http.Response, error) {
	b.i++
	if b.i == 1 {
		body := `{"error":{"message":"max_tokens or model output limit was reached","type":"invalid_request_error","detail":"effort setting not supported"}}`
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(oaiOK)), Header: http.Header{}}, nil
}

type plain400Doer struct {
	body  string
	calls int
}

func (p *plain400Doer) Do(*http.Request) (*http.Response, error) {
	p.calls++
	return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(p.body)), Header: http.Header{}}, nil
}

// TestRaiseOutputCapLegacyThinking: the legacy-thinking Anthropic path carries
// maxTok+budget as its effective cap; the raise must multiply the EFFECTIVE cap so it can
// never lower it.
func TestRaiseOutputCapLegacyThinking(t *testing.T) {
	req := request{
		bodyMap:   map[string]any{"max_tokens": 2048 + 32768, "model": "m"},
		capKey:    "max_tokens",
		maxTokens: 2048 + 32768,
	}
	raised := raiseOutputCap(req)
	if raised.maxTokens != 4*(2048+32768) {
		t.Fatalf("raised to %d want %d", raised.maxTokens, 4*(2048+32768))
	}
	var m map[string]any
	if err := json.Unmarshal(raised.body, &m); err != nil {
		t.Fatal(err)
	}
	if m["max_tokens"] != float64(4*(2048+32768)) {
		t.Errorf("body cap %v", m["max_tokens"])
	}
	if m["model"] != "m" {
		t.Errorf("unrelated field changed: %v", m)
	}
}

// readErrDoer exists in judge_test.go; capRetryDoer replays one transport error between the
// cap 400 and the success so the ladder composes with ordinary backoff retries too.
type capRetryDoer struct{ i int }

func (c *capRetryDoer) Do(*http.Request) (*http.Response, error) {
	c.i++
	switch c.i {
	case 1:
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(lunarouteCap)), Header: http.Header{}}, nil
	case 2:
		return nil, errors.New("dial")
	default:
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(oaiOK)), Header: http.Header{}}, nil
	}
}

func TestCapRetryComposesWithTransportRetry(t *testing.T) {
	var sleeps []time.Duration
	j, err := New(&capRetryDoer{}, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(&sleeps))
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "medium", Input: fixedInputs[KindAdjudicate]})
	if err != nil {
		t.Fatalf("must recover: %v", err)
	}
	// The cap raise repairs WITHIN its attempt slot (it must not eat the transient budget),
	// so the ladder's accounting is: slot 0 = capped call + raised call (the 5xx), slot 1 =
	// the successful retry. Three HTTP calls, two attempt slots.
	if v.Attempts != 2 {
		t.Errorf("attempts %d want 2 (slot 0: cap raise + its transport retry; slot 1: success)", v.Attempts)
	}
	if !v.CapRaised {
		t.Error("the verdict must record that a cap raise was in flight")
	}
}

// TestCapRaiseSurvivesExhaustedTransientBudget is the #159 poison ordering: four transient
// failures consume the whole attempt budget, and THEN the cap 400 arrives. The raised
// request must still be sent — a deterministic transport fix is never eaten by the
// transient budget.
func TestCapRaiseSurvivesExhaustedTransientBudget(t *testing.T) {
	var sleeps []time.Duration
	j, err := New(&lateCapDoer{}, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(&sleeps))
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "medium", Input: fixedInputs[KindAdjudicate]})
	if err != nil {
		t.Fatalf("must recover: %v", err)
	}
	if !lateCapDoerRaised {
		t.Fatal("the 4x-raised request must be sent even when transients exhausted the budget")
	}
	if !v.CapRaised {
		t.Error("the verdict must record the cap raise")
	}
}

// lateCapDoer: four 500s (the transient budget), then the cap 400, then success. The flag
// records whether the fifth call — the raised one — ever arrives.
var lateCapDoerRaised bool

type lateCapDoer struct{ i int }

func (c *lateCapDoer) Do(*http.Request) (*http.Response, error) {
	c.i++
	switch c.i {
	case 1, 2, 3, 4:
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("boom")), Header: http.Header{}}, nil
	case 5:
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(lunarouteCap)), Header: http.Header{}}, nil
	default:
		lateCapDoerRaised = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(oaiOK)), Header: http.Header{}}, nil
	}
}

// TestCapRaiseLegacyThinkingEndToEnd drives a legacy-thinking Anthropic model
// (claude-sonnet-4-5 at high effort — the path that writes maxTok+budget into max_tokens)
// through prepare → withBody → Call into a cap 400, and asserts the RETRIED request
// carries 4× the EFFECTIVE cap — not the kind's base cap. A refactor of withBody's cap
// read (reading maxTok instead of the body value) would make the raised request smaller
// than 4× the effective cap and this test fails.
func TestCapRaiseLegacyThinkingEndToEnd(t *testing.T) {
	var bodies []map[string]any
	d := &captureDoer{next: func(i int) *http.Response {
		if i == 0 {
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(genericCap)), Header: http.Header{}}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anthOK)), Header: http.Header{}}
	}, bodies: &bodies}
	j, err := New(d, Keys{Anthropic: "sk-ant-test", OpenAI: "sk-test"}, URLs{}, func() string { return "0123456789abcdef" }, testClock(nil))
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "claude-sonnet-4-5", Effort: "high", Input: fixedInputs[KindAdjudicate]})
	if err != nil {
		t.Fatal(err)
	}
	if !v.CapRaised || len(bodies) != 2 {
		t.Fatalf("raised=%v bodies=%d", v.CapRaised, len(bodies))
	}
	base, ok := bodies[0]["max_tokens"].(float64)
	if !ok || base != float64(MaxTokensAdjudicate+8192) {
		t.Fatalf("first request effective cap %v want %d", bodies[0]["max_tokens"], MaxTokensAdjudicate+8192)
	}
	raised, ok := bodies[1]["max_tokens"].(float64)
	if !ok || raised != 4*base {
		t.Fatalf("raised cap %v want 4x effective (%v)", bodies[1]["max_tokens"], 4*base)
	}
}

type captureDoer struct {
	i      int
	next   func(int) *http.Response
	bodies *[]map[string]any
}

func (c *captureDoer) Do(r *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	*c.bodies = append(*c.bodies, m)
	c.i++
	return c.next(c.i - 1), nil
}
