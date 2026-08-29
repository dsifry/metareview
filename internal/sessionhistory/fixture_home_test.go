package sessionhistory

import (
	"os"
	"path/filepath"
	"testing"
)

// Collect falls back to os.UserHomeDir() when HomeDir is unset, so coverage of the discovery
// and signal-reading paths has been tracking whatever session data happens to sit in the
// developer's home. Measured 2026-08-29: this package reports 86.3% on a machine with real
// ~/.claude data and 79.1% on CI, from identical code — and the floor was set from the higher
// number, so CI could never pass. A supplied fixture makes the measurement mean the same thing
// everywhere, and exercises the .json and summary paths CI never reached.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a raw JSONL transcript (claude), the highest-confidence shape
	write(".claude/projects/proj-a/session.jsonl",
		`{"timestamp":"2026-08-01T10:00:00Z","message":"fixed the nil deref in f.go"}`+"\n"+
			`{"timestamp":"2026-08-01T10:05:00Z","content":"added a regression test"}`+"\n")
	// a single-object JSON transcript (codex) - readJSONSignal, which no test reached before
	write(".codex/sessions/one.json",
		`{"created_at":"2026-08-02T09:00:00Z","summary":"reviewed the fsm loop boundary"}`)
	// a generated summary (.md) - the summary record kind
	write(".codex/memories/rollout_summaries/s.md", "# rollout\n\nadjusted the judge prompt\n")
	// a history file at the top level of .codex
	write(".codex/history.jsonl", `{"time":"2026-08-03T08:00:00Z","text":"ran the coverage gate"}`+"\n")
	return home
}

func TestCollectReadsEverySupportedSessionShape(t *testing.T) {
	ctx, err := Collect(t.TempDir(), Options{HomeDir: fixtureHome(t)})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !ctx.Available {
		t.Fatal("a home carrying session files must be Available")
	}
	if len(ctx.Signals) == 0 {
		t.Fatal("want signals from the fixture home")
	}
	kinds := map[string]bool{}
	for _, s := range ctx.Signals {
		kinds[s.SourceType] = true
	}
	for _, want := range []string{"claude-jsonl", "codex-json", "codex-summary"} {
		if !kinds[want] {
			t.Errorf("no signal of type %q; got %v", want, kinds)
		}
	}
}

// A .json transcript is read whole and parsed as one object; a malformed one is skipped
// rather than failing the collection, because session files are other tools' output.
func TestMalformedJSONSessionIsSkippedNotFatal(t *testing.T) {
	home := fixtureHome(t)
	if err := os.WriteFile(filepath.Join(home, ".codex", "sessions", "bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := Collect(t.TempDir(), Options{HomeDir: home})
	if err != nil {
		t.Fatalf("a malformed session file must not fail collection: %v", err)
	}
	if !ctx.Available {
		t.Error("the good files must still be collected")
	}
}
