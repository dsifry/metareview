package sourcereview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestClaudeStructuredOutput(t *testing.T) {
	for _, envelope := range []string{
		`{"structured_output":{"findings":[]}}`,
		`{"result":"","structured_output":{"findings":[]}}`,
		`{"result":"Review complete","structured_output":{"findings":[]}}`,
		`{"result":"{\"findings\":[]}","structured_output":null}`,
	} {
		got, err := claudeModelText([]byte(envelope))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := Parse(got, keptFiles()); err != nil {
			t.Fatalf("valid output rejected: %s: %v", envelope, err)
		}
	}
	for _, envelope := range []string{
		`{"structured_output":"not an object","result":"{\"findings\":[]}"}`,
		`{"structured_output":{"findings":[]},"is_error":true}`,
		`{"structured_output":{"findings":[]},"stop_reason":"max_tokens"}`,
	} {
		if _, err := claudeModelText([]byte(envelope)); err == nil {
			t.Fatalf("accepted failed/malformed response: %s", envelope)
		}
	}
}

func TestEmptyPathIsRejected(t *testing.T) {
	for _, files := range [][]File{nil, {{Path: "a.go"}}} {
		if _, err := onlyPaths(files, []string{""}); err == nil {
			t.Fatal("empty filter accepted")
		}
	}
	if _, _, _, err := selectPaths("", nil, []string{""}); err == nil {
		t.Fatal("empty filter reached git")
	}
	if _, err := parseArgs([]string{"--model", "grok", "--output", "/tmp/out", "--path", ""}); err == nil {
		t.Fatal("CLI accepted empty filter")
	}
}

func TestSelectPinnedCommit(t *testing.T) {
	old := []byte("package a\n// old revision\n")
	root, first := newRepoBytes(t, map[string][]byte{"a.go": old})
	changed := false
	git := func(dir string, args []string, stdin []byte) ([]byte, error) {
		if args[0] == "ls-tree" && !changed {
			changed = true
			if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n// new revision\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gitRun(t, root, "add", "a.go")
			gitRun(t, root, "commit", "-m", "concurrent update")
		}
		return realGit(dir, args, stdin)
	}
	kept, _, commit, err := Select(root, git)
	if err != nil || commit != first || len(kept) != 1 || !bytes.Equal(kept[0].Body, old) {
		t.Fatalf("snapshot changed: %v %s %v", err, commit, kept)
	}
}

func TestSelectFiltersBeforeBlobReads(t *testing.T) {
	root, commit := newRepoBytes(t, map[string][]byte{
		"a.go":          []byte("package a\n"),
		"other.go":      []byte("package other\n"),
		"vendor/lib.go": bytes.Repeat([]byte("x"), 1<<20),
		"docs/a\nb.md":  []byte("documentation\n"),
	})
	id, err := realGit(root, []string{"rev-parse", commit + ":a.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var request []byte
	git := func(dir string, args []string, stdin []byte) ([]byte, error) {
		if args[0] == "cat-file" {
			request = append(request, stdin...)
		}
		return realGit(dir, args, stdin)
	}
	kept, _, _, err := selectPaths(root, git, []string{"a.go"})
	if err != nil || len(kept) != 1 || !bytes.Equal(request, id) {
		t.Fatalf("loaded blobs outside the selected scope: %q want %q; %v", request, id, err)
	}
}

func TestSelectNewlineFilenames(t *testing.T) {
	body := []byte("package a\n")
	root, _ := newRepoBytes(t, map[string][]byte{"pkg/a\nb.go": body, "docs/a\nb.md": []byte("docs\n")})
	kept, dropped, _, err := Select(root, nil)
	if err != nil || len(kept) != 1 || kept[0].Path != "pkg/a\nb.go" || !bytes.Equal(kept[0].Body, body) || len(dropped) != 1 {
		t.Fatalf("newline paths: kept=%v dropped=%v err=%v", kept, dropped, err)
	}
}

func TestExactFindingFilenameWinsOverNormalization(t *testing.T) {
	files := map[string][]byte{
		"\napp.go": []byte("package newline\n"), "app.go": []byte("package app\n"),
		"pkg/a\\b.go": []byte("package backslash\n"), "pkg/a/b.go": []byte("package slash\n"),
		" app.go ": []byte("package spaced\n"),
	}
	for cited := range files {
		text := fmt.Sprintf(`{"findings":[{"tag":"bug","file":%q,"start_line":1,"end_line":1,"issue":"i","consequence":"c","confidence":80,"severity":"P1"}]}`, cited)
		got, skipped, err := Parse(text, files)
		if err != nil || len(skipped) != 0 || len(got) != 1 || got[0].File != cited {
			t.Fatalf("exact filename changed: cited=%q got=%v skipped=%v err=%v", cited, got, skipped, err)
		}
	}
}

func TestRunContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(Options{Context: ctx}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Cancellation and an available worker slot can both be selected. Neither
	// outcome may launch a call or turn an already canceled review into success.
	for i := 0; i < 64; i++ {
		if _, err := review(Options{Context: ctx}, [][]byte{[]byte("unused")}, nil); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if err := writeOutputs(Options{Context: ctx}, "", "", "opus", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	root, _ := newRepoBytes(t, specFiles())
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	git := func(dir string, args []string, stdin []byte) ([]byte, error) {
		out, err := realGit(dir, args, stdin)
		if args[0] == "cat-file" {
			cancel()
		}
		return out, err
	}
	if err := Run(Options{Context: ctx, Repo: root, Model: "grok", OutputDir: t.TempDir(), Git: git}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestRunCancellationStopsQueuedCalls(t *testing.T) {
	root, _ := fivePrompts(t)
	out := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{wait: func(context.Context, []byte) error {
		cancel()
		return nil // Even a runner that ignores cancellation must not publish.
	}, reply: func(int, string, []string, []byte) ([]byte, error) {
		return cliStdout("opus", `{"findings":[]}`), nil
	}}
	err := Run(Options{Context: ctx, Repo: root, Model: "opus", OutputDir: out, Jobs: 1, Runner: runner})
	if !errors.Is(err, context.Canceled) || len(runner.callList()) != 1 {
		t.Fatalf("err=%v calls=%d", err, len(runner.callList()))
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatal("published canceled review")
	}
}

type cancelWriter struct{ cancel context.CancelFunc }

func (w cancelWriter) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestReviewCanceledAfterResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{reply: func(int, string, []string, []byte) ([]byte, error) {
		return cliStdout("opus", `{"findings":[]}`), nil
	}}
	_, err := review(Options{Context: ctx, Model: "opus", Runner: runner, Stdout: io.Discard, Stderr: cancelWriter{cancel}}, [][]byte{[]byte("prompt")}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSourceReviewSignalHelper(t *testing.T) {
	if os.Getenv("METAREVIEW_SIGNAL_CHILD") != "1" {
		return
	}
	os.Exit(CLI([]string{"--model", "grok", "--output", os.Getenv("METAREVIEW_SIGNAL_OUT"), "--call-timeout", "30s"}, os.Getenv("METAREVIEW_SIGNAL_ROOT"), io.Discard, os.Stderr))
}

func TestCLISignalsCleanUpModel(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			root, _ := newRepoBytes(t, map[string][]byte{"a.go": []byte("package a\n")})
			dir := t.TempDir()
			pidFile, promptFile := filepath.Join(dir, "pid"), filepath.Join(dir, "prompt")
			script := "#!/bin/sh\nprintf '%s\\n' \"$$\" > \"$METAREVIEW_SIGNAL_PID\"\nprintf '%s\\n' \"$2\" > \"$METAREVIEW_SIGNAL_PROMPT\"\nsleep 60\n"
			if err := os.WriteFile(filepath.Join(dir, "grok"), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestSourceReviewSignalHelper$")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			cmd.Env = append(os.Environ(), "METAREVIEW_SIGNAL_CHILD=1", "METAREVIEW_SIGNAL_ROOT="+root,
				"METAREVIEW_SIGNAL_OUT="+filepath.Join(dir, "out"), "METAREVIEW_SIGNAL_PID="+pidFile,
				"METAREVIEW_SIGNAL_PROMPT="+promptFile, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+dir)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			var pid int
			var prompt string
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				p, _ := os.ReadFile(pidFile)
				pid, _ = strconv.Atoi(strings.TrimSpace(string(p)))
				p, _ = os.ReadFile(promptFile)
				prompt = strings.TrimSpace(string(p))
				if pid > 0 && prompt != "" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid == 0 || prompt == "" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("fake model did not start: %s", stderr.Bytes())
			}
			defer func() { _ = syscall.Kill(-pid, syscall.SIGKILL) }()
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
					t.Fatalf("CLI exit: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("CLI failed to stop")
			}
			state, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
			if s := strings.TrimSpace(string(state)); s != "" && !strings.HasPrefix(s, "Z") {
				t.Fatalf("model still running: %s", state)
			}
			if _, err := os.Stat(prompt); !os.IsNotExist(err) {
				t.Fatalf("prompt not removed: %v", err)
			}
			workspaces, _ := filepath.Glob(filepath.Join(dir, "metareview-workspace-*"))
			if len(workspaces) != 0 {
				t.Fatalf("workspaces left behind: %v", workspaces)
			}
		})
	}
}

func TestPackPreservesPromptBytes(t *testing.T) {
	var files []File
	for i := 0; i < 24; i++ {
		body := bytes.Repeat([]byte("x\n"), i*3)
		if i%2 == 1 {
			body = append(body, 'x')
		}
		if numberedSize(body) != len(numberLines(body, 1)) {
			t.Fatal("wrong rendered size")
		}
		files = append(files, File{Path: fmt.Sprintf("%02d.go", i), Body: body})
	}
	const budget = 2000
	// Reference the previous packing algorithm to require identical prompt bytes,
	// including boundaries, ordering, separators, and source line numbers.
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].Body) == len(files[j].Body) {
			return files[i].Path < files[j].Path
		}
		return len(files[i].Body) > len(files[j].Body)
	})
	var bins [][]File
	for _, f := range files {
		placed := false
		for i := range bins {
			candidate := append(append([]File(nil), bins[i]...), f)
			if len(promptWith(instruction, candidate)) <= budget {
				bins[i], placed = candidate, true
				break
			}
		}
		if !placed {
			bins = append(bins, []File{f})
		}
	}
	got, err := Pack(files, budget)
	if err != nil || len(got) != len(bins) {
		t.Fatalf("prompt count changed: %v", err)
	}
	for i := range bins {
		if !bytes.Equal(got[i], promptWith(instruction, bins[i])) {
			t.Fatalf("prompt %d changed", i)
		}
	}
}

func packScalingFiles() []File {
	var files []File
	for i := 0; i < 30; i++ {
		files = append(files, File{Path: fmt.Sprintf("big-%03d.go", i), Body: bytes.Repeat([]byte("x"), 100000)})
	}
	for i := 0; i < 300; i++ {
		files = append(files, File{Path: fmt.Sprintf("small-%03d.go", i), Body: bytes.Repeat([]byte("y"), 4000)})
	}
	return files
}

func TestPackWholeRepoAllocation(t *testing.T) {
	files := packScalingFiles()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	prompts, err := Pack(files, MaxPromptBytes)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("files=330 source_bytes=4200000 prompts=%d allocated_bytes=%d elapsed=%s", len(prompts), allocated, time.Since(start))
	if err != nil || len(prompts) != 37 {
		t.Fatalf("pack: %d prompts, %v", len(prompts), err)
	}
	// Bound gross regressions generously, even with coverage instrumentation.
	if allocated > 128<<20 {
		t.Fatalf("packing allocated %d bytes for 4.2 MB of source", allocated)
	}
}

func BenchmarkPackWholeRepo(b *testing.B) {
	files := packScalingFiles()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Pack(files, MaxPromptBytes); err != nil {
			b.Fatal(err)
		}
	}
}
