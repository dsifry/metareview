package sessionhistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectDiscoversExplicitCodexAndClaudeSignals(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	explicit := filepath.Join(root, "sessions")
	credentialValue := "redaction-test-value"
	mustWrite(t, filepath.Join(explicit, "explicit.jsonl"), `{"timestamp":"2026-05-26T10:00:00Z","message":"fixed duplicated service path token=`+credentialValue+`"}`+"\n")
	mustWrite(t, filepath.Join(home, ".codex", "sessions", "2026", "05", "session.jsonl"), `{"created_at":"2026-05-26T11:00:00Z","text":"reviewer correction from codex raw session"}`+"\n")
	mustWrite(t, filepath.Join(home, ".codex", "memories", "rollout_summaries", "summary.md"), "summary: generated rollout lesson\n")
	mustWrite(t, filepath.Join(home, ".claude", "projects", "project", "conversation.jsonl"), `{"timestamp":"2026-05-26T12:00:00Z","content":"claude transcript correction"}`+"\n")

	ctx, err := Collect(root, Options{SessionRoot: explicit, HomeDir: home})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if !ctx.Available {
		t.Fatalf("expected available session context: %+v", ctx)
	}
	if len(ctx.Signals) != 4 {
		t.Fatalf("expected 4 bounded signals, got %d: %+v", len(ctx.Signals), ctx.Signals)
	}
	assertSignal(t, ctx.Signals, "explicit-jsonl", "raw-transcript", "fixed duplicated service path")
	assertSignal(t, ctx.Signals, "codex-jsonl", "raw-transcript", "codex raw session")
	assertSignal(t, ctx.Signals, "codex-summary", "generated-summary", "generated rollout lesson")
	assertSignal(t, ctx.Signals, "claude-jsonl", "raw-transcript", "claude transcript correction")
	for _, signal := range ctx.Signals {
		if strings.Contains(signal.Excerpt, credentialValue) {
			t.Fatalf("signal leaked secret-like text: %+v", signal)
		}
		if len([]rune(signal.Excerpt)) > MaxExcerptRunes {
			t.Fatalf("excerpt not bounded: %d", len([]rune(signal.Excerpt)))
		}
		if signal.Path == "" || signal.Confidence == "" {
			t.Fatalf("signal missing provenance fields: %+v", signal)
		}
	}
	if ctx.IntrospectionRequest != nil {
		t.Fatalf("did not expect introspection request with usable records: %+v", ctx.IntrospectionRequest)
	}
}

func TestCollectRecordsIntrospectionContractWhenAdaptersFindNoUsableRecords(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".codex", "sessions", "2026", "05", "empty.jsonl"), "\n")
	mustWrite(t, filepath.Join(home, ".claude", "projects", "project", "empty.jsonl"), "{}\n")

	ctx, err := Collect(root, Options{HomeDir: home})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context without usable records: %+v", ctx)
	}
	if ctx.UnavailableReason != "no-usable-session-records" {
		t.Fatalf("unexpected unavailable reason: %s", ctx.UnavailableReason)
	}
	if ctx.IntrospectionRequest == nil {
		t.Fatalf("expected active-runtime introspection request")
	}
	if ctx.IntrospectionRequest.Confidence != "low" || !strings.Contains(ctx.IntrospectionRequest.Prompt, "session/project history") {
		t.Fatalf("unexpected introspection contract: %+v", ctx.IntrospectionRequest)
	}
}

func TestCollectUnavailableWhenNoSessionRootsExist(t *testing.T) {
	ctx, err := Collect(t.TempDir(), Options{HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if ctx.Available {
		t.Fatalf("expected unavailable context")
	}
	if ctx.UnavailableReason != "no-session-root" {
		t.Fatalf("unexpected unavailable reason: %s", ctx.UnavailableReason)
	}
	if ctx.IntrospectionRequest == nil {
		t.Fatalf("expected introspection contract so current runtime can disclose paths")
	}
}

func TestCollectBoundsFilesAndExcerpts(t *testing.T) {
	root := t.TempDir()
	sessionRoot := filepath.Join(root, "sessions")
	longText := strings.Repeat("x", MaxExcerptRunes+100)
	for i := 0; i < MaxSignals+5; i++ {
		mustWrite(t, filepath.Join(sessionRoot, "session-"+string(rune('a'+i))+".jsonl"), `{"message":"`+longText+`"}`+"\n")
	}

	ctx, err := Collect(root, Options{SessionRoot: sessionRoot, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(ctx.Signals) != MaxSignals {
		t.Fatalf("expected MaxSignals=%d, got %d", MaxSignals, len(ctx.Signals))
	}
	for _, signal := range ctx.Signals {
		if len([]rune(signal.Excerpt)) > MaxExcerptRunes {
			t.Fatalf("excerpt not bounded: %d", len([]rune(signal.Excerpt)))
		}
	}
}

func assertSignal(t *testing.T, signals []Signal, sourceType, recordKind, excerpt string) {
	t.Helper()
	for _, signal := range signals {
		if signal.SourceType == sourceType && signal.RecordKind == recordKind && strings.Contains(signal.Excerpt, excerpt) {
			return
		}
	}
	t.Fatalf("missing signal source=%s kind=%s excerpt=%q in %+v", sourceType, recordKind, excerpt, signals)
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
