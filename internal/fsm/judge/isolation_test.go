package judge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
)

// dirProbe is a CLI exec seam that records the working directory each attempt was given and what
// that directory held at the moment the CLI would have started.
type dirProbe struct {
	dirs     []string
	contents [][]string
	stdout   string
	code     int
}

func (p *dirProbe) exec(_ context.Context, dir string, _ []string, _ string) ([]byte, int, error) {
	p.dirs = append(p.dirs, dir)
	entries, err := os.ReadDir(dir)
	names := []string{}
	if err != nil {
		names = append(names, "ERR: "+err.Error())
	}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	p.contents = append(p.contents, names)
	return []byte(p.stdout), p.code, nil
}

// The judge CLIs must never start inside the repository under review. A headless CLI still loads
// that directory's project configuration (Claude Code's .claude/settings.json hooks and
// CLAUDE.md, Codex's AGENTS.md), without the interactive trust prompt, and disabling tools does not
// switch that off. So every attempt runs in a fresh, empty, private directory that is removed after.
func assertIsolated(t *testing.T, p *dirProbe, wantAttempts int) {
	t.Helper()
	if len(p.dirs) != wantAttempts {
		t.Fatalf("attempts: got %d want %d", len(p.dirs), wantAttempts)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i, dir := range p.dirs {
		if dir == "" || !filepath.IsAbs(dir) {
			t.Fatalf("attempt %d ran in %q: an empty dir inherits the caller's cwd (the reviewed repo)", i+1, dir)
		}
		if rel, err := filepath.Rel(cwd, dir); err == nil && !strings.HasPrefix(rel, "..") {
			t.Fatalf("attempt %d ran inside the caller's tree: %s", i+1, dir)
		}
		if len(p.contents[i]) != 0 {
			t.Fatalf("attempt %d's directory was not empty: %v", i+1, p.contents[i])
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("attempt %d's directory %s was not removed (stat: %v)", i+1, dir, err)
		}
	}
}

func TestClaudeJudgeRunsIsolatedFromTheReviewedRepo(t *testing.T) {
	p := &dirProbe{stdout: claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	f := &fakeClaude{stdout: p.stdout}
	j := &claudeJudge{exec: func(ctx context.Context, dir string, args []string, stdin string) ([]byte, int, error) {
		f.args = args
		return p.exec(ctx, dir, args, stdin)
	}, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), claudeRequest()); err != nil {
		t.Fatal(err)
	}
	assertIsolated(t, p, 1)
	joined := strings.Join(f.args, " ")
	// Defence in depth behind the empty directory: user settings only (never a project's or a
	// local override), and no MCP server that is not passed explicitly (none is).
	for _, want := range []string{"--setting-sources user", "--strict-mcp-config"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

// Every retry gets its own fresh directory, and none is left behind.
func TestClaudeJudgeIsolatesEveryAttempt(t *testing.T) {
	p := &dirProbe{code: 1}
	j := &claudeJudge{exec: p.exec, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), claudeRequest()); err == nil {
		t.Fatal("a CLI that always exits 1 must fail the call")
	}
	assertIsolated(t, p, MaxAttempts)
	seen := map[string]bool{}
	for _, d := range p.dirs {
		if seen[d] {
			t.Fatalf("attempts shared a directory: %s", d)
		}
		seen[d] = true
	}
}

func TestCodexJudgeRunsIsolatedByDefault(t *testing.T) {
	var args []string
	p := &dirProbe{stdout: codexStream(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j := &codexJudge{exec: func(ctx context.Context, dir string, a []string, stdin string) ([]byte, int, error) {
		args = a
		return p.exec(ctx, dir, a, stdin)
	}, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := j.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	assertIsolated(t, p, 1)
	// AGENTS.md is read from the working directory up, including from a materialized evidence tree,
	// which can carry one from the change under review: project docs are off entirely.
	if !strings.Contains(strings.Join(args, " "), "-c project_doc_max_bytes=0") {
		t.Fatalf("args %q must disable project docs", args)
	}
}

// An explicit work dir (the escalation path's materialized evidence tree) is honoured as-is and
// never deleted by the judge.
func TestCodexJudgeHonoursAnExplicitWorkDir(t *testing.T) {
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(tree, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &dirProbe{stdout: codexStream(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	j := &codexJudge{exec: p.exec, nonce: func() string { return "n0" }, clock: codexClock(), workDir: tree}
	if _, err := j.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	if len(p.dirs) != 1 || p.dirs[0] != tree {
		t.Fatalf("explicit work dir not used: %v", p.dirs)
	}
	if _, err := os.Stat(filepath.Join(tree, "a.go")); err != nil {
		t.Fatalf("the judge must not remove a caller's tree: %v", err)
	}
}

// If no private directory can be made, nothing is spawned in the caller's directory as a fallback:
// the call fails as a transport error, which the ladder retries and the run records.
func TestJudgesRefuseToRunWithoutAnIsolatedDir(t *testing.T) {
	saved := isolatedDir
	t.Cleanup(func() { isolatedDir = saved })
	isolatedDir = func() (string, func(), error) { return "", func() {}, errors.New("disk full") }

	calls := 0
	spy := func(context.Context, string, []string, string) ([]byte, int, error) {
		calls++
		return nil, 0, nil
	}
	cj := &claudeJudge{exec: spy, nonce: func() string { return "n0" }, clock: codexClock()}
	_, cerr := cj.Call(context.Background(), claudeRequest())
	xj := &codexJudge{exec: spy, nonce: func() string { return "n0" }, clock: codexClock()}
	_, xerr := xj.Call(context.Background(), codexRequest())
	for name, err := range map[string]error{"claude": cerr, "codex": xerr} {
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != CodeJudgeTransport || !strings.Contains(e.Detail, "isolated") {
			t.Fatalf("%s: want an isolated-dir transport error, got %v", name, err)
		}
	}
	if calls != 0 {
		t.Fatalf("a CLI was spawned without an isolated directory (%d calls)", calls)
	}
}

// The real directory maker: private (0700), empty, under the user's own cache directory (not a
// shared temp dir, where another local user could plant a .git root and repo-scoped skills a CLI
// walks up to), and removed by its cleanup.
func TestIsolatedDir(t *testing.T) {
	cache := t.TempDir()
	saved := userCacheDir
	t.Cleanup(func() { userCacheDir = saved })
	userCacheDir = func() (string, error) { return cache, nil }

	dir, cleanup, err := isolatedDir()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("stat %s: %v %v", dir, info, err)
	}
	if want := filepath.Join(cache, "metareview", "judge") + string(filepath.Separator); !strings.HasPrefix(dir, want) {
		t.Fatalf("dir %s is not under the user cache base %s", dir, want)
	}
	base, err := os.Stat(filepath.Join(cache, "metareview", "judge"))
	if err != nil || base.Mode().Perm() != 0o700 {
		t.Fatalf("the base must be private: %v %v", base, err)
	}
	cleanup()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup left %s", dir)
	}
}

// No user cache directory (HOME unset), or one that cannot hold the base: fall back to the temp
// directory rather than refuse to judge.
func TestIsolatedDirFallsBackToTheTempDir(t *testing.T) {
	saved := userCacheDir
	t.Cleanup(func() { userCacheDir = saved })
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, cacheDir := range map[string]func() (string, error){
		"no cache dir":   func() (string, error) { return "", errors.New("$HOME is not defined") },
		"unusable cache": func() (string, error) { return blocked, nil }, // a file: MkdirAll fails
	} {
		userCacheDir = cacheDir
		dir, cleanup, err := isolatedDir()
		if err != nil || !strings.HasPrefix(dir, tmp) {
			t.Fatalf("%s: want a dir under %s, got %q %v", name, tmp, dir, err)
		}
		cleanup()
	}
}

// With no usable directory anywhere the maker reports the error rather than returning a directory
// the CLI would then be run in.
func TestIsolatedDirReportsAnUnusableTempDir(t *testing.T) {
	saved := userCacheDir
	t.Cleanup(func() { userCacheDir = saved })
	userCacheDir = func() (string, error) { return "", errors.New("no home") }
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))
	dir, cleanup, err := isolatedDir()
	if err == nil || dir != "" {
		t.Fatalf("want an error and no dir, got %q %v", dir, err)
	}
	cleanup() // a no-op, but always safe to call
}

// A panic inside the CLI seam still removes the attempt's directory.
func TestJudgesRemoveTheDirEvenOnPanic(t *testing.T) {
	for name, call := range map[string]func(ClaudeExec){
		"claude": func(e ClaudeExec) {
			_, _ = (&claudeJudge{exec: e, nonce: func() string { return "n0" }, clock: codexClock()}).Call(context.Background(), claudeRequest())
		},
		"codex": func(e ClaudeExec) {
			_, _ = (&codexJudge{exec: CodexExec(e), nonce: func() string { return "n0" }, clock: codexClock()}).Call(context.Background(), codexRequest())
		},
	} {
		var dir string
		func() {
			defer func() { _ = recover() }()
			call(func(_ context.Context, d string, _ []string, _ string) ([]byte, int, error) {
				dir = d
				panic("boom")
			})
		}()
		if dir == "" {
			t.Fatalf("%s: the seam was never called", name)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: a panic left %s behind", name, dir)
		}
	}
}

// Judge calls are not sessions: nothing is persisted (and with a fresh directory per attempt,
// persisting would leave one transcript directory behind per call).
func TestJudgesPersistNoSession(t *testing.T) {
	f := &fakeClaude{stdout: claudeJSON(`{"reasoning":"r","is_real":true,"confidence":0.9}`)}
	if _, err := (&claudeJudge{exec: f.exec, nonce: func() string { return "n0" }, clock: codexClock()}).Call(context.Background(), claudeRequest()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.args, " "), "--no-session-persistence") {
		t.Fatalf("claude args %q", f.args)
	}
	var args []string
	x := &codexJudge{exec: func(_ context.Context, _ string, a []string, _ string) ([]byte, int, error) {
		args = a
		return []byte(codexStream(`{"reasoning":"r","is_real":true,"confidence":0.9}`)), 0, nil
	}, nonce: func() string { return "n0" }, clock: codexClock()}
	if _, err := x.Call(context.Background(), codexRequest()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--ephemeral") {
		t.Fatalf("codex args %q", args)
	}
}
