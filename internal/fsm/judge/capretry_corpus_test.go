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
			sleeps: []time.Duration{time.Second}, attempts: 2,
		},
		{
			name: "anthropic-cap-then-ok", model: "claude-opus-4-5",
			steps:  []step{{400, genericCap, nil}, {200, anthOK, nil}},
			capKey: "max_tokens", baseCap: 2048,
			sleeps: []time.Duration{time.Second}, attempts: 2,
		},
		{
			// GLM/Kimi requests are floored at 16384 by prepare; the raise is 4x the
			// EFFECTIVE cap, not the kind's base.
			name: "glm-floor-then-ok", model: "glm-5.3-background",
			steps:  []step{{400, lunarouteCap, nil}, {200, oaiOK, nil}},
			capKey: "max_completion_tokens", baseCap: 16384,
			sleeps: []time.Duration{time.Second}, attempts: 2,
		},
		{
			name: "cap-twice-terminal", model: "gpt-5.2",
			steps:  []step{{400, lunarouteCap, nil}, {400, lunarouteCap, nil}},
			capKey: "max_completion_tokens", baseCap: 2048,
			sleeps: []time.Duration{time.Second}, code: CodeJudgeHTTP, attempts: 2,
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

// TestIsOutputCapBody pins the marker set: each observed phrasing matches, near-miss
// effort-rejection text does not.
func TestIsOutputCapBody(t *testing.T) {
	for _, body := range []string{
		lunarouteCap,
		genericCap,
		`{"error":{"message":"could not FINISH THE MESSAGE because the model ran out"}}`,
	} {
		if !isOutputCapBody([]byte(body)) {
			t.Errorf("must match: %s", body)
		}
	}
	for _, body := range []string{
		`{"error":{"message":"bad request"}}`,
		`{"error":{"message":"effort level unsupported"}}`,
		`{}`,
	} {
		if isOutputCapBody([]byte(body)) {
			t.Errorf("must not match: %s", body)
		}
	}
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
	if v.Attempts != 3 {
		t.Errorf("attempts %d want 3 (cap raise, transport retry, success)", v.Attempts)
	}
}
