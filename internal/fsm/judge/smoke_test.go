//go:build smoke

package judge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// TestSmokeProvider is the real-provider smoke test (spec 5 §7): it runs only under -tags smoke and skips
// without a key. `tests/run-all.sh` only vets and lists it.
func TestSmokeProvider(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY unset")
	}
	j, err := New(NewHTTPClient(60*time.Second), Keys{OpenAI: key}, URLs{}, func() string { return "0123456789abcdef" }, Clock{Now: time.Now, After: time.After})
	if err != nil {
		t.Fatal(err)
	}
	v, err := j.Call(context.Background(), Request{Kind: KindAdjudicate, Model: "gpt-5.2", Effort: "low", Input: AdjudicateInput{Diff: "+x := nil\n", Candidate: run.Finding{IssueText: "nil deref", File: "f.go", Line: 1}}, Node: "smoke", Fence: true})
	if err != nil || v.Parsed == nil {
		t.Fatalf("smoke: %v %+v", err, v)
	}
}
