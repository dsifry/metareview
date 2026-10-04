//go:build smoke

package cli

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// TestSmokeGrokTransport is W1's live gate: it runs only under -tags smoke, skips
// when the grok CLI is absent, and proves the end-to-end call shape the unit
// tests can only fake — grok answers a judge prompt as a single no-tool turn,
// the verdict parses, and real spend comes back. Run it before merging the
// transport:
//
//	go test -tags smoke -run TestSmokeGrokTransport ./internal/fsm/cli/
func TestSmokeGrokTransport(t *testing.T) {
	if _, err := exec.LookPath(judge.GrokBin); err != nil {
		t.Skip("grok CLI not installed")
	}
	j, err := judge.NewWithCLIs(nil, judge.Keys{}, judge.URLs{},
		func() string { return "0123456789abcdef" }, judge.Clock{Now: time.Now, After: time.After}, nil, nil, realGrokExec)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	v, err := j.Call(ctx, judge.Request{
		Kind:   judge.KindAdjudicate,
		Model:  "grok/grok-4.7",
		Effort: "low",
		Input: judge.AdjudicateInput{
			Diff:      "+x := nil\n",
			Candidate: run.Finding{IssueText: "nil deref", File: "f.go", Line: 1},
		},
		Node: "smoke", Fence: true,
	})
	if err != nil {
		t.Fatalf("live grok call failed: %v", err)
	}
	if v.Parsed == nil {
		t.Fatalf("live grok returned no parseable verdict: %+v", v)
	}
	if v.Tokens.Total() == 0 {
		t.Fatal("live grok reported no spend: the usage block is not being parsed")
	}
}
