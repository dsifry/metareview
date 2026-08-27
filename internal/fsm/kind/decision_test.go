package kind

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
)

func bp(b bool) *bool { return &b }

func TestDecision(t *testing.T) {
	eq := func(a, b *bool) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	cases := []struct {
		name, kind, verdict string
		raw, eff            *bool
	}{
		{"match true", judge.KindMatch, `{"match":true,"confidence":0.2}`, bp(true), bp(true)},
		{"match false", judge.KindMatch, `{"match":false,"confidence":0.9}`, bp(false), bp(false)},
		{"adjudicate real above threshold", judge.KindAdjudicate, `{"is_real":true,"confidence":0.7}`, bp(true), bp(true)},
		{"adjudicate real below threshold", judge.KindAdjudicate, `{"is_real":true,"confidence":0.6}`, bp(true), bp(false)},
		{"adjudicate real no confidence", judge.KindAdjudicate, `{"is_real":true}`, bp(true), bp(false)},
		{"adjudicate not real", judge.KindAdjudicate, `{"is_real":false,"confidence":0.99}`, bp(false), bp(false)},
		{"still-present", judge.KindStillPresent, `{"still_present":true,"confidence":0.5}`, bp(true), bp(true)},
		{"still-present null bool", judge.KindStillPresent, `{"still_present":null,"confidence":0.5}`, nil, nil},
		{"null verdict", judge.KindAdjudicate, `null`, nil, nil},
		{"empty verdict", judge.KindAdjudicate, ``, nil, nil},
		{"absent field", judge.KindAdjudicate, `{"match":true}`, nil, nil},
		{"unknown kind", "other", `{"match":true}`, nil, nil},
		{"garbage", judge.KindMatch, `{`, nil, nil},
	}
	for _, c := range cases {
		d := Decision(c.kind, json.RawMessage(c.verdict))
		if !eq(d.Raw, c.raw) || !eq(d.Effective, c.eff) {
			t.Fatalf("%s: got raw=%v eff=%v", c.name, d.Raw, d.Effective)
		}
	}
}

func TestNilJudgeExecutors(t *testing.T) {
	r, err := New(Deps{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{MatchThenAdjudicate, StillPresent} {
		e, ok := r.Executor(name)
		if !ok {
			t.Fatalf("%s executor", name)
		}
		_, err := e.Execute(context.Background(), machine.ExecInput{})
		if !errs.Is(err, machine.CodeExecutorFailed) {
			t.Fatalf("%s without a judge: %v", name, err)
		}
		if e2 := errs.As(err); e2 == nil || e2.Fields["reason"] != "no_judge" {
			t.Fatalf("reason: %v", err)
		}
	}
}
