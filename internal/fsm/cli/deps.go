// Package cli is `metareview fsm …` (spec 5): a thin shell over machine, kind/judge/cmdexec/mockai, run, record,
// export and workflows. Every OS and provider effect sits behind Deps so Run is tested at 100% with fakes.
package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/cmdexec"
	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/export"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/mockai"
	"github.com/dsifry/metareview/internal/fsm/record"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/workflows"
)

// Deps are the seams (spec 5 §8). RealDeps binds them; tests inject fakes.
type Deps struct {
	Getenv   func(string) string
	Environ  func() []string
	Now      func() time.Time
	After    func(time.Duration) <-chan time.Time // the judge retry ladder's timer
	Rand     func([]byte) (int, error)
	LookPath func(string) (string, error)
	FileHash func(string) (string, error)
	ReadFile func(string) ([]byte, error)
	// TempDir makes the evidence sandbox root, outside the repository so the tree is a clean,
	// enumerable set rather than a view of the working tree. It is NOT a read boundary: the
	// working directory a judge is given is a default, not a confinement, and codex's
	// --sandbox read-only bounds writes and network, not reads. Verified: from a cwd outside
	// the repository, `codex sandbox -- /bin/ls <repo>/.git` succeeds. Treat the tree as what
	// the judge was OFFERED, not as the limit of what it could reach.
	TempDir    func(pattern string) (string, error)
	Exec       gate.Exec
	CodexExec  judge.CodexExec
	ClaudeExec judge.ClaudeExec
	GrokExec   judge.GrokExec
	HTTP       judge.Doer
	Store      func(root string) run.RunStore
	Sidecar    func(root string) machine.Sidecar
	ExportFS   export.FS
	MockLoad   func(dir string) (*mockai.Scenario, error)
	Workflows  func(name string) ([]byte, error)
	Terminal   func(root string, clock func() run.Time) func(context.Context, machine.View) error
	Exists     func(root, runID string) (bool, error)
	Runner     func(r machine.RunnerDeps, env func() []string, fileHash func(string) (string, error), now func() time.Time, real cmdexec.Runner) converge.Caller
}

// RealDeps binds every seam to its real implementation and nothing else; it cannot fail.
func RealDeps() Deps {
	return Deps{
		Getenv:     os.Getenv,
		Environ:    os.Environ,
		Now:        time.Now,
		After:      time.After,
		Rand:       rand.Read,
		LookPath:   exec.LookPath,
		FileHash:   workflow.FileSHA256,
		ReadFile:   os.ReadFile,
		TempDir:    func(pattern string) (string, error) { return os.MkdirTemp("", pattern) },
		Exec:       gate.RealExec,
		CodexExec:  realCodexExec,
		ClaudeExec: realClaudeExec,
		GrokExec:   realGrokExec,
		HTTP:       newHTTPClient(),
		Store:      func(common string) run.RunStore { return run.NewCommonDirStore(common, run.Options{}) },
		Sidecar:    func(common string) machine.Sidecar { return machine.FSSidecar{Root: common, CommonDir: true} },
		ExportFS:   export.OSFS{},
		MockLoad:   mockai.Load,
		Workflows:  workflows.Read,
		Terminal:   record.Terminal,
		Exists:     record.Exists,
		Runner:     guardedRunner,
	}
}

// newHTTPClient is judge.NewHTTPClient with proxy environment variables switched off (spec 5 §8). No
// blanket client timeout: per-attempt deadlines (and the cap-raise extension) are applied via the
// per-request context in attempt(), keyed off METAREVIEW_JUDGE_TIMEOUT.
func newHTTPClient() *http.Client {
	c := judge.NewHTTPClient()
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	c.Transport = t
	return c
}

func guardedRunner(r machine.RunnerDeps, env func() []string, fileHash func(string) (string, error), now func() time.Time, real cmdexec.Runner) converge.Caller {
	return cmdexec.Guarded{Runner: real, Allowed: r.Allowed, Dir: r.WorkDir, RunID: r.RunID, FileHash: fileHash, Audit: r.Audit, Environ: env, Clock: now, CmdCalls: r.CmdCalls}
}

// codexBin is the executable realCodexExec runs; a seam so the three exit paths
// can be tested without the Codex CLI installed.
var codexBin = judge.CodexBin

// claudeBin is the executable realClaudeExec runs; a seam for the same reason.
var claudeBin = judge.ClaudeBin

// grokBin is the executable realGrokExec runs; a seam for the same reason.
var grokBin = judge.GrokBin

// realGrokExec runs the Grok CLI. Unlike claude and codex, the prompt is not on
// stdin: grok reads it from --prompt-file, so stdin is unused. The environment
// is inherited (the logged-in session the CLI reads lives under the user's
// home, and metareview never handles the token itself) plus GROK_MEMORY=0, the
// documented switch that keeps a judge call out of the user's cross-session
// memory.
func realGrokExec(ctx context.Context, dir string, args []string, _ string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, grokBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GROK_MEMORY=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), ee.ExitCode(), nil
	}
	if err != nil {
		return out.Bytes(), 0, err
	}
	return out.Bytes(), 0, nil
}

// realClaudeExec runs the Claude Code CLI, mirroring realCodexExec: the user
// prompt goes in on stdin rather than as an argument so it never appears in the
// process table, and the environment is inherited: the logged-in session the
// CLI reads lives under the user's home, and metareview never handles the
// token itself.
func realClaudeExec(ctx context.Context, dir string, args []string, stdin string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, claudeBin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), ee.ExitCode(), nil
	}
	if err != nil {
		return out.Bytes(), 0, err
	}
	return out.Bytes(), 0, nil
}

// realCodexExec runs the Codex CLI. The prompt goes in on stdin rather than as
// an argument so it never appears in the process table, and the environment is
// inherited: the OAuth session the CLI reads lives under the user's home, and
// metareview never handles the token itself.
func realCodexExec(ctx context.Context, dir string, args []string, stdin string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, codexBin, args...)
	// The judge always passes a directory: a fresh isolated one (judge.isolatedDir), or a
	// materialized evidence tree a caller confined it to. This does not stop the CLI reading
	// elsewhere - cmd.Dir is a starting point, not a jail.
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), ee.ExitCode(), nil // ran and failed: the exit code is the answer
	}
	if err != nil {
		return out.Bytes(), 0, err // could not be run at all
	}
	return out.Bytes(), 0, nil
}
