// Package cmdexec runs consented commands: argv only (no shell), pinned
// absolute argv[0], hash re-verification, an exact environment allow-list,
// a process-group timeout, bounded output, typed decode, and one audited
// cmd_call per execution. Nothing here is reachable without consent.
package cmdexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Error codes.
const (
	CodeCmdNotAllowed    = "ERR_CMD_NOT_ALLOWED"
	CodeCmdTimeout       = "ERR_CMD_TIMEOUT"
	CodeCmdFailed        = "ERR_CMD_FAILED"
	CodeCmdOutputInvalid = "ERR_CMD_OUTPUT_INVALID"
	DefaultTimeout       = 60 * time.Second
	WaitDelay            = 2 * time.Second
	baseEnvNames         = "PATH HOME LANG TMPDIR"
	EnvRunID             = "MRV_RUN_ID"
	MaxOutput            = run.MaxPayload
	unknownExit          = -1
)

// Spec is one execution request.
type Spec struct {
	Name    string // declared cmd name; the fake runner keys on it, the exec runner ignores it
	Ordinal int    // prior cmd_call count for Name (durable mock ordinal)
	Argv    []string
	Dir     string
	Stdin   []byte
	Timeout time.Duration
	Env     []string // the full child environment, KEY=VALUE
}

// Result is what the process produced. Stdout/Stderr are capped at
// MaxOutput+1 bytes (the extra byte marks overflow).
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
	Duration       time.Duration
}

// Runner executes a Spec. Timeouts are reported as ERR_CMD_TIMEOUT; a parent
// context cancellation is returned as ctx.Err(); a process that could not
// start is ERR_CMD_FAILED{reason: spawn}.
type Runner interface {
	Run(ctx context.Context, s Spec) (Result, error)
}

// Guarded wraps a Runner with the consent guardrails.
type Guarded struct {
	Runner   Runner
	Allowed  []run.AllowedCmd
	Dir      string
	RunID    string
	FileHash func(string) (string, error)
	Audit    func(run.Event) error
	Environ  func() []string
	Clock    func() time.Time
	CmdCalls func(name string) int // prior cmd_call count (nil → 0)
}

var _ converge.Caller = Guarded{}

func (g Guarded) find(name string) (run.AllowedCmd, bool) {
	for _, c := range g.Allowed {
		if c.Name == name {
			return c, true
		}
	}
	return run.AllowedCmd{}, false
}

// env builds the exact child environment.
func (g Guarded) env(c run.AllowedCmd) []string {
	present := map[string]string{}
	if g.Environ != nil {
		for _, kv := range g.Environ() {
			k, v, ok := strings.Cut(kv, "=")
			if ok {
				present[k] = v
			}
		}
	}
	var out []string
	for _, k := range strings.Fields(baseEnvNames) {
		if v, ok := present[k]; ok {
			out = append(out, k+"="+v)
		}
	}
	out = append(out, EnvRunID+"="+g.RunID)
	for _, k := range c.Env {
		if v, ok := present[k]; ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// exec is the shared unaudited core; it returns the audit payload alongside.
func (g Guarded) exec(ctx context.Context, name string, stdin []byte) (converge.CmdResult, run.CmdCallData, error) {
	c, ok := g.find(name)
	if !ok {
		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "command "+name+" is not consented", "name", name)
	}
	if len(c.Argv) == 0 || !filepath.IsAbs(c.Argv[0]) {
		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "consented argv[0] is not an absolute path", "name", name, "reason", "relative")
	}
	if err := workflow.VerifyCmds([]run.AllowedCmd{c}, g.Dir, g.FileHash); err != nil {
		return converge.CmdResult{}, run.CmdCallData{}, err
	}
	timeout := DefaultTimeout
	if c.TimeoutMS > 0 {
		timeout = time.Duration(c.TimeoutMS) * time.Millisecond
	}
	ordinal := 0
	if g.CmdCalls != nil {
		ordinal = g.CmdCalls(name)
	}
	sum := sha256.Sum256(stdin)
	spec := Spec{Name: name, Ordinal: ordinal, Argv: append([]string(nil), c.Argv...), Dir: g.Dir, Stdin: stdin, Timeout: timeout, Env: g.env(c)}
	res, err := g.Runner.Run(ctx, spec)
	data := run.CmdCallData{Name: name, Argv: spec.Argv, InputHash: hex.EncodeToString(sum[:]), ExitCode: res.ExitCode, DurationMS: res.Duration.Milliseconds()}
	data.Stdout, data.StdoutTruncated = run.CapText(string(res.Stdout), run.MaxDetail)
	data.Stderr, data.StderrTruncated = run.CapText(string(res.Stderr), run.MaxStderr)
	out := converge.CmdResult{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}
	switch {
	case err != nil:
		data.ExitCode = unknownExit
		data.Error = errs.Code(err)
		if data.Error == "" {
			data.Error = CodeCmdFailed
			err = errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "name", name, "reason", "spawn"), err)
		}
	case len(res.Stdout) > MaxOutput || len(res.Stderr) > MaxOutput:
		data.Error = CodeCmdOutputInvalid
		err = errs.E(CodeCmdOutputInvalid, "command output exceeds MaxPayload", "name", name, "reason", "too_large")
	case res.ExitCode != 0:
		data.Error = CodeCmdFailed
		err = errs.E(CodeCmdFailed, fmt.Sprintf("command %s exited %d", name, res.ExitCode), "name", name, "exit", fmt.Sprint(res.ExitCode))
	}
	return out, data, err
}

func (g Guarded) audit(data run.CmdCallData) error {
	if g.Audit == nil {
		return nil
	}
	return g.Audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(data)})
}

// Run executes a consented command and audits it (converge.Runner).
func (g Guarded) Run(ctx context.Context, name string, stdin []byte) (converge.CmdResult, error) {
	res, data, err := g.exec(ctx, name, stdin)
	if data.Name == "" {
		return converge.CmdResult{}, err // pre-exec refusal: never audited
	}
	if aerr := g.audit(data); aerr != nil {
		return converge.CmdResult{}, aerr
	}
	return res, err
}

// Call runs and decodes the full stdout into out (DisallowUnknownFields);
// the single cmd_call it audits carries the decode error when there is one.
func (g Guarded) Call(ctx context.Context, name string, stdin []byte, out any) error {
	res, data, err := g.exec(ctx, name, stdin)
	if data.Name == "" {
		return err
	}
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(res.Stdout))
		dec.DisallowUnknownFields()
		if derr := dec.Decode(out); derr != nil {
			err = errs.E(CodeCmdOutputInvalid, "command "+name+" stdout did not decode: "+derr.Error(), "name", name, "reason", "decode")
			data.Error = CodeCmdOutputInvalid
		}
	}
	if aerr := g.audit(data); aerr != nil {
		return aerr
	}
	return err
}

// ---------------------------------------------------------------- exec runner

type execRunner struct{}

// NewExecRunner returns the real process runner.
func NewExecRunner() Runner { return execRunner{} }

// cappingWriter keeps at most MaxOutput+1 bytes and keeps draining.
type cappingWriter struct{ buf bytes.Buffer }

func (w *cappingWriter) Write(p []byte) (int, error) {
	if room := MaxOutput + 1 - w.buf.Len(); room > 0 {
		if len(p) > room {
			w.buf.Write(p[:room])
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

func (execRunner) Run(ctx context.Context, s Spec) (Result, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(tctx, s.Argv[0], s.Argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.Stdin = bytes.NewReader(s.Stdin)
	var stdout, stderr cappingWriter
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = WaitDelay
	start := time.Now()
	err := cmd.Run()
	res := Result{Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes(), Duration: time.Since(start)}
	switch {
	case err == nil:
		return res, nil
	case ctx.Err() != nil:
		res.ExitCode = unknownExit
		return res, ctx.Err()
	case tctx.Err() != nil:
		res.ExitCode = unknownExit
		return res, errs.E(CodeCmdTimeout, fmt.Sprintf("command exceeded %s", timeout), "timeout", timeout.String())
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	res.ExitCode = unknownExit
	return res, errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "reason", "spawn"), err)
}

// Executable returns the absolute path of the running binary (tests pin it
// as argv[0]).
func Executable() string {
	p, _ := os.Executable()
	return p
}
