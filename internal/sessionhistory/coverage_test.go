package sessionhistory

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- homeDir ---

func TestHomeDirConfiguredAndFallback(t *testing.T) {
	if got, err := homeDir("/explicit/home"); err != nil || got != "/explicit/home" {
		t.Fatalf("configured home should be used verbatim: %q, %v", got, err)
	}
	t.Setenv("HOME", "/tmp/fake-home")
	if got, err := homeDir(""); err != nil || got == "" {
		t.Fatalf("fallback to $HOME should succeed: %q, %v", got, err)
	}
}

func TestHomeDirErrorWhenHomeUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home source on Windows")
	}
	t.Setenv("HOME", "")
	if _, err := homeDir(""); err == nil {
		t.Fatal("an unset HOME must surface an error")
	}
}

// Collect returns the homeDir error rather than swallowing it.
func TestCollectHomeDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not the home source on Windows")
	}
	t.Setenv("HOME", "")
	if _, err := Collect(t.TempDir(), Options{}); err == nil {
		t.Fatal("Collect must surface the home-unavailable error")
	}
}

// --- Collect via explicit session root ---

func TestCollectFromExplicitRoot(t *testing.T) {
	sessionRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sessionRoot, "s.jsonl"),
		[]byte(`{"timestamp":"2026-08-01T00:00:00Z","message":"explicit root record"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := Collect(t.TempDir(), Options{SessionRoot: sessionRoot, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !ctx.Available || len(ctx.Signals) == 0 {
		t.Fatalf("expected signals from the explicit root, got %+v", ctx)
	}
	if !strings.Contains(ctx.Signals[0].SourceType, "explicit") {
		t.Errorf("expected an explicit-source signal, got %q", ctx.Signals[0].SourceType)
	}
}

// A home with a session directory but no usable records is Available:false with the
// no-usable-session-records reason and an introspection request.
func TestCollectNoUsableRecords(t *testing.T) {
	home := t.TempDir()
	// An empty .jsonl yields no signal.
	p := filepath.Join(home, ".claude", "projects", "empty.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := Collect(t.TempDir(), Options{HomeDir: home})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if ctx.Available || ctx.UnavailableReason != "no-usable-session-records" {
		t.Fatalf("expected no-usable-session-records, got %+v", ctx)
	}
	if ctx.IntrospectionRequest == nil {
		t.Error("an unavailable context must carry an introspection request")
	}
}

// A walk error during discovery propagates out of Collect (discoverCandidates -> Collect), and the
// walk callback's own error branch is exercised by invoking it with a non-nil error.
func TestCollectDiscoverWalkError(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "projects", "proj", "s.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := walkDir
	t.Cleanup(func() { walkDir = orig })
	sentinel := errors.New("walk boom")
	walkDir = func(root string, fn fs.WalkDirFunc) error {
		// Drive the callback's error branch, then let scanSessionPath surface it.
		return fn(root, nil, sentinel)
	}
	if _, err := Collect(t.TempDir(), Options{HomeDir: home}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the walk error to propagate, got %v", err)
	}
}

// The same walk error on an explicit session root also propagates.
func TestCollectExplicitRootWalkError(t *testing.T) {
	sessionRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(sessionRoot, "s.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := walkDir
	t.Cleanup(func() { walkDir = orig })
	sentinel := errors.New("explicit walk boom")
	walkDir = func(root string, fn fs.WalkDirFunc) error { return sentinel }
	if _, err := Collect(t.TempDir(), Options{SessionRoot: sessionRoot, HomeDir: t.TempDir()}); !errors.Is(err, sentinel) {
		t.Fatalf("expected the explicit-root walk error, got %v", err)
	}
}

// A candidate whose file cannot be read fails the whole collection (collectSignals -> Collect).
func TestCollectSignalReadError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("unreadable-file permissions do not hold for root or on Windows")
	}
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "projects", "locked.jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"message":"x"}`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := Collect(t.TempDir(), Options{HomeDir: home}); err == nil {
		t.Fatal("an unreadable candidate file must fail collection")
	}
}

// --- scanSessionPath / candidateFor ---

func TestScanSessionPathStatError(t *testing.T) {
	if _, err := scanSessionPath(filepath.Join(t.TempDir(), "nope"), "x", 0); err == nil {
		t.Fatal("a nonexistent path must return a stat error")
	}
}

func TestScanSessionPathSingleFile(t *testing.T) {
	dir := t.TempDir()
	jsonlFile := filepath.Join(dir, "one.jsonl")
	if err := os.WriteFile(jsonlFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := scanSessionPath(jsonlFile, "codex", 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("a single .jsonl file should yield one candidate: %v, %v", got, err)
	}
	// A single non-session file yields no candidate and no error.
	txtFile := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := scanSessionPath(txtFile, "codex", 1); err != nil || got != nil {
		t.Fatalf("a non-session file should yield no candidate: %v, %v", got, err)
	}
}

func TestCandidateForRejectsUnknownExt(t *testing.T) {
	if _, ok := candidateFor("/x/notes.txt", "codex", 0); ok {
		t.Error(".txt must not be a candidate")
	}
	if c, ok := candidateFor("/x/s.md", "codex", 0); !ok || c.recordKind != "generated-summary" {
		t.Errorf(".md should be a generated-summary candidate: %+v ok=%v", c, ok)
	}
	if c, ok := candidateFor("/x/s.jsonl", "codex", 0); !ok || c.confidence != "high" {
		t.Errorf(".jsonl raw transcript should be high confidence: %+v ok=%v", c, ok)
	}
}

// discoverCandidates truncates to maxCandidateFiles when a directory holds more.
func TestDiscoverCandidatesTruncates(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxCandidateFiles+5; i++ {
		if err := os.WriteFile(filepath.Join(dir, "s"+itoa(i)+".jsonl"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, found, err := discoverCandidates(t.TempDir(), home, "")
	if err != nil || !found {
		t.Fatalf("discoverCandidates: found=%v err=%v", found, err)
	}
	if len(got) != maxCandidateFiles {
		t.Fatalf("expected truncation to %d, got %d", maxCandidateFiles, len(got))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// --- signalFromCandidate ---

func TestSignalFromCandidateUnknownExt(t *testing.T) {
	// candidateFor never yields a non-session ext, but signalFromCandidate defends against it.
	_, ok, err := signalFromCandidate("/root", "/home", candidate{path: "/x/s.txt"})
	if ok || err != nil {
		t.Fatalf("an unknown ext should be a clean no-signal: ok=%v err=%v", ok, err)
	}
}

func TestSignalFromCandidateMarkdownReadError(t *testing.T) {
	_, _, err := signalFromCandidate("/root", "/home", candidate{path: filepath.Join(t.TempDir(), "ghost.md")})
	if err == nil {
		t.Fatal("a missing .md file must surface a read error")
	}
}

func TestSignalFromCandidateMarkdown(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.md")
	if err := os.WriteFile(p, []byte("# summary\n\nreviewed the loop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sig, ok, err := signalFromCandidate(t.TempDir(), t.TempDir(), candidate{path: p, sourceType: "codex-summary"})
	if err != nil || !ok || !strings.Contains(sig.Excerpt, "reviewed the loop") {
		t.Fatalf("expected a markdown signal: %+v ok=%v err=%v", sig, ok, err)
	}
}

// --- readJSONLSignal / readJSONSignal ---

func TestReadJSONLSignalOpenError(t *testing.T) {
	if _, _, _, err := readJSONLSignal(filepath.Join(t.TempDir(), "nope.jsonl")); err == nil {
		t.Fatal("a missing file must surface an open error")
	}
}

func TestReadJSONLSignalScannerError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "huge.jsonl")
	// A line longer than the scanner's token cap makes scanner.Err() report ErrTooLong.
	if err := os.WriteFile(p, []byte(strings.Repeat("a", (1<<20)+16)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := readJSONLSignal(p); ok || err == nil {
		t.Fatalf("an over-long line must surface a scanner error: ok=%v err=%v", ok, err)
	}
}

func TestReadJSONSignalReadError(t *testing.T) {
	if _, _, _, err := readJSONSignal(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("a missing file must surface a read error")
	}
}

// --- contentString / parseJSONObject ---

func TestParseJSONObjectContentArrayAndNested(t *testing.T) {
	// content as an array of items: the first non-empty string wins.
	ts, text, ok := parseJSONObject(`{"timestamp":"t1","content":["","nested-array-value"]}`)
	if !ok || ts != "t1" || text != "nested-array-value" {
		t.Fatalf("array content not walked: ts=%q text=%q ok=%v", ts, text, ok)
	}
	// content as a nested object.
	_, text, ok = parseJSONObject(`{"message":{"text":"nested-map-value"}}`)
	if !ok || text != "nested-map-value" {
		t.Fatalf("nested-map content not walked: text=%q ok=%v", text, ok)
	}
	// a non-string, non-map, non-array content value contributes nothing.
	if _, _, ok := parseJSONObject(`{"message":42}`); ok {
		t.Fatalf("a numeric content value must not yield a body")
	}
}

// --- displayPath / relativeTo ---

func TestDisplayPath(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	// Under root -> repo-relative.
	if got := displayPath(root, home, filepath.Join(root, "a", "b.jsonl")); got != "a/b.jsonl" {
		t.Errorf("under-root path should be repo-relative, got %q", got)
	}
	// Under home -> ~/rel.
	if got := displayPath(root, home, filepath.Join(home, "c.jsonl")); got != "~/c.jsonl" {
		t.Errorf("under-home path should be ~-relative, got %q", got)
	}
	// Under neither -> the path itself.
	outside := filepath.Join(t.TempDir(), "d.jsonl")
	if got := displayPath(root, home, outside); got != filepath.ToSlash(outside) {
		t.Errorf("outside path should be returned as-is, got %q", got)
	}
}

func TestRelativeToEmptyBase(t *testing.T) {
	if _, ok := relativeTo("", "/x/y"); ok {
		t.Error("an empty base must not resolve")
	}
}

func TestRelativeToAbsErrors(t *testing.T) {
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	sentinel := errors.New("abs boom")
	calls := 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 1 {
			return "", sentinel
		}
		return orig(p)
	}
	if _, ok := relativeTo("/base", "/base/x"); ok {
		t.Error("a base Abs failure must make relativeTo fail closed")
	}
	// Now fail only the second (path) Abs call.
	calls = 0
	filepathAbs = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", sentinel
		}
		return orig(p)
	}
	if _, ok := relativeTo("/base", "/base/x"); ok {
		t.Error("a path Abs failure must make relativeTo fail closed")
	}
}

// --- excerpt ---

func TestExcerptCapsAtMaxRunes(t *testing.T) {
	// A value longer than MaxExcerptRunes (here multibyte runes) is truncated on a rune boundary.
	long := strings.Repeat("é", MaxExcerptRunes+50)
	got := excerpt(long)
	if len([]rune(got)) > MaxExcerptRunes {
		t.Fatalf("excerpt must cap at %d runes, got %d", MaxExcerptRunes, len([]rune(got)))
	}
	if got == "" {
		t.Fatal("truncated excerpt should not be empty")
	}
}
