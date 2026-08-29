# metareview task-done context

Run ID: `mrv-20260827-083958687691000-task-done-m1-m6-fsm-packages-a99c72f1`

## Task

# M1–M6: internal/fsm core packages

Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).

## Acceptance

- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
- `bash tests/coverage.sh` passes (legacy floor held).
- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.


## Git

- Base: `3e183a964ed0efe80be24b553d6391d393bb36bc`
- Head: `a9f613bc09d1f340ff26014d3c06bd1e974c14c6`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `40976`
- Filtered diff bytes: `40976`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `43b2d0aa4542aa54`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/cmdexec/cmdexec.go
- internal/fsm/cmdexec/cmdexec_test.go
- internal/fsm/cmdexec/sha_test.go
- internal/fsm/converge/converge.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go
- internal/fsm/run/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/workflow_test.go

### Shards
- shard-01: docs/tasks/m1-m6-fsm-packages.md, internal/fsm/cmdexec/cmdexec.go, internal/fsm/cmdexec/cmdexec_test.go, internal/fsm/cmdexec/sha_test.go, internal/fsm/converge/converge.go, internal/fsm/machine/harness_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/sidecar.go, internal/fsm/machine/types.go, internal/fsm/run/types.go, internal/fsm/workflow/resolve.go, internal/fsm/workflow/workflow_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/cmdexec/cmdexec.go
- internal/fsm/cmdexec/cmdexec_test.go
- internal/fsm/cmdexec/sha_test.go
- internal/fsm/converge/converge.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go
- internal/fsm/run/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/workflow_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/internal/fsm/cmdexec/cmdexec.go b/internal/fsm/cmdexec/cmdexec.go
new file mode 100644
index 0000000..b5e4e87
--- /dev/null
+++ b/internal/fsm/cmdexec/cmdexec.go
@@ -0,0 +1,267 @@
+// Package cmdexec runs consented commands: argv only (no shell), pinned
+// absolute argv[0], hash re-verification, an exact environment allow-list,
+// a process-group timeout, bounded output, typed decode, and one audited
+// cmd_call per execution. Nothing here is reachable without consent.
+package cmdexec
+
+import (
+	"bytes"
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"syscall"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+// Error codes.
+const (
+	CodeCmdNotAllowed    = "ERR_CMD_NOT_ALLOWED"
+	CodeCmdTimeout       = "ERR_CMD_TIMEOUT"
+	CodeCmdFailed        = "ERR_CMD_FAILED"
+	CodeCmdOutputInvalid = "ERR_CMD_OUTPUT_INVALID"
+	DefaultTimeout       = 60 * time.Second
+	WaitDelay            = 2 * time.Second
+	baseEnvNames         = "PATH HOME LANG TMPDIR"
+	EnvRunID             = "MRV_RUN_ID"
+	MaxOutput            = run.MaxPayload
+	unknownExit          = -1
+)
+
+// Spec is one execution request.
+type Spec struct {
+	Name    string // declared cmd name; the fake runner keys on it, the exec runner ignores it
+	Ordinal int    // prior cmd_call count for Name (durable mock ordinal)
+	Argv    []string
+	Dir     string
+	Stdin   []byte
+	Timeout time.Duration
+	Env     []string // the full child environment, KEY=VALUE
+}
+
+// Result is what the process produced. Stdout/Stderr are capped at
+// MaxOutput+1 bytes (the extra byte marks overflow).
+type Result struct {
+	Stdout, Stderr []byte
+	ExitCode       int
+	Duration       time.Duration
+}
+
+// Runner executes a Spec. Timeouts are reported as ERR_CMD_TIMEOUT; a parent
+// context cancellation is returned as ctx.Err(); a process that could not
+// start is ERR_CMD_FAILED{reason: spawn}.
+type Runner interface {
+	Run(ctx context.Context, s Spec) (Result, error)
+}
+
+// Guarded wraps a Runner with the consent guardrails.
+type Guarded struct {
+	Runner   Runner
+	Allowed  []run.AllowedCmd
+	Dir      string
+	RunID    string
+	FileHash func(string) (string, error)
+	Audit    func(run.Event) error
+	Environ  func() []string
+	Clock    func() time.Time
+	CmdCalls func(name string) int // prior cmd_call count (nil → 0)
+}
+
+var _ converge.Caller = Guarded{}
+
+func (g Guarded) find(name string) (run.AllowedCmd, bool) {
+	for _, c := range g.Allowed {
+		if c.Name == name {
+			return c, true
+		}
+	}
+	return run.AllowedCmd{}, false
+}
+
+// env builds the exact child environment.
+func (g Guarded) env(c run.AllowedCmd) []string {
+	present := map[string]string{}
+	if g.Environ != nil {
+		for _, kv := range g.Environ() {
+			k, v, ok := strings.Cut(kv, "=")
+			if ok {
+				present[k] = v
+			}
+		}
+	}
+	var out []string
+	for _, k := range strings.Fields(baseEnvNames) {
+		if v, ok := present[k]; ok {
+			out = append(out, k+"="+v)
+		}
+	}
+	out = append(out, EnvRunID+"="+g.RunID)
+	for _, k := range c.Env {
+		if v, ok := present[k]; ok {
+			out = append(out, k+"="+v)
+		}
+	}
+	return out
+}
+
+// exec is the shared unaudited core; it returns the audit payload alongside.
+func (g Guarded) exec(ctx context.Context, name string, stdin []byte) (converge.CmdResult, run.CmdCallData, error) {
+	c, ok := g.find(name)
+	if !ok {
+		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "command "+name+" is not consented", "name", name)
+	}
+	if len(c.Argv) == 0 || !filepath.IsAbs(c.Argv[0]) {
+		return converge.CmdResult{}, run.CmdCallData{}, errs.E(CodeCmdNotAllowed, "consented argv[0] is not an absolute path", "name", name, "reason", "relative")
+	}
+	if err := workflow.VerifyCmds([]run.AllowedCmd{c}, g.Dir, g.FileHash); err != nil {
+		return converge.CmdResult{}, run.CmdCallData{}, err
+	}
+	timeout := DefaultTimeout
+	if c.TimeoutMS > 0 {
+		timeout = time.Duration(c.TimeoutMS) * time.Millisecond
+	}
+	ordinal := 0
+	if g.CmdCalls != nil {
+		ordinal = g.CmdCalls(name)
+	}
+	sum := sha256.Sum256(stdin)
+	spec := Spec{Name: name, Ordinal: ordinal, Argv: append([]string(nil), c.Argv...), Dir: g.Dir, Stdin: stdin, Timeout: timeout, Env: g.env(c)}
+	res, err := g.Runner.Run(ctx, spec)
+	data := run.CmdCallData{Name: name, Argv: spec.Argv, InputHash: hex.EncodeToString(sum[:]), ExitCode: res.ExitCode, DurationMS: res.Duration.Milliseconds()}
+	data.Stdout, data.StdoutTruncated = run.CapText(string(res.Stdout), run.MaxDetail)
+	data.Stderr, data.StderrTruncated = run.CapText(string(res.Stderr), run.MaxStderr)
+	out := converge.CmdResult{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode, Duration: res.Duration}
+	switch {
+	case err != nil:
+		data.ExitCode = unknownExit
+		data.Error = errs.Code(err)
+		if data.Error == "" {
+			data.Error = CodeCmdFailed
+			err = errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "name", name, "reason", "spawn"), err)
+		}
+	case len(res.Stdout) > MaxOutput || len(res.Stderr) > MaxOutput:
+		data.Error = CodeCmdOutputInvalid
+		err = errs.E(CodeCmdOutputInvalid, "command output exceeds MaxPayload", "name", name, "reason", "too_large")
+	case res.ExitCode != 0:
+		data.Error = CodeCmdFailed
+		err = errs.E(CodeCmdFailed, fmt.Sprintf("command %s exited %d", name, res.ExitCode), "name", name, "exit", fmt.Sprint(res.ExitCode))
+	}
+	return out, data, err
+}
+
+func (g Guarded) audit(data run.CmdCallData) error {
+	if g.Audit == nil {
+		return nil
+	}
+	return g.Audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(data)})
+}
+
+// Run executes a consented command and audits it (converge.Runner).
+func (g Guarded) Run(ctx context.Context, name string, stdin []byte) (converge.CmdResult, error) {
+	res, data, err := g.exec(ctx, name, stdin)
+	if data.Name == "" {
+		return converge.CmdResult{}, err // pre-exec refusal: never audited
+	}
+	if aerr := g.audit(data); aerr != nil {
+		return converge.CmdResult{}, aerr
+	}
+	return res, err
+}
+
+// Call runs and decodes the full stdout into out (DisallowUnknownFields);
+// the single cmd_call it audits carries the decode error when there is one.
+func (g Guarded) Call(ctx context.Context, name string, stdin []byte, out any) error {
+	res, data, err := g.exec(ctx, name, stdin)
+	if data.Name == "" {
+		return err
+	}
+	if err == nil {
+		dec := json.NewDecoder(bytes.NewReader(res.Stdout))
+		dec.DisallowUnknownFields()
+		if derr := dec.Decode(out); derr != nil {
+			err = errs.E(CodeCmdOutputInvalid, "command "+name+" stdout did not decode: "+derr.Error(), "name", name, "reason", "decode")
+			data.Error = CodeCmdOutputInvalid
+		}
+	}
+	if aerr := g.audit(data); aerr != nil {
+		return aerr
+	}
+	return err
+}
+
+// ---------------------------------------------------------------- exec runner
+
+type execRunner struct{}
+
+// NewExecRunner returns the real process runner.
+func NewExecRunner() Runner { return execRunner{} }
+
+// cappingWriter keeps at most MaxOutput+1 bytes and keeps draining.
+type cappingWriter struct{ buf bytes.Buffer }
+
+func (w *cappingWriter) Write(p []byte) (int, error) {
+	if room := MaxOutput + 1 - w.buf.Len(); room > 0 {
+		if len(p) > room {
+			w.buf.Write(p[:room])
+		} else {
+			w.buf.Write(p)
+		}
+	}
+	return len(p), nil
+}
+
+func (execRunner) Run(ctx context.Context, s Spec) (Result, error) {
+	timeout := s.Timeout
+	if timeout <= 0 {
+		timeout = DefaultTimeout
+	}
+	tctx, cancel := context.WithTimeout(ctx, timeout)
+	defer cancel()
+	cmd := exec.CommandContext(tctx, s.Argv[0], s.Argv[1:]...)
+	cmd.Dir = s.Dir
+	cmd.Env = s.Env
+	cmd.Stdin = bytes.NewReader(s.Stdin)
+	var stdout, stderr cappingWriter
+	cmd.Stdout, cmd.Stderr = &stdout, &stderr
+	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
+	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
+	cmd.WaitDelay = WaitDelay
+	start := time.Now()
+	err := cmd.Run()
+	res := Result{Stdout: stdout.buf.Bytes(), Stderr: stderr.buf.Bytes(), Duration: time.Since(start)}
+	switch {
+	case err == nil:
+		return res, nil
+	case ctx.Err() != nil:
+		res.ExitCode = unknownExit
+		return res, ctx.Err()
+	case tctx.Err() != nil:
+		res.ExitCode = unknownExit
+		return res, errs.E(CodeCmdTimeout, fmt.Sprintf("command exceeded %s", timeout), "timeout", timeout.String())
+	}
+	var ee *exec.ExitError
+	if errors.As(err, &ee) {
+		res.ExitCode = ee.ExitCode()
+		return res, nil
+	}
+	res.ExitCode = unknownExit
+	return res, errs.Wrap(errs.E(CodeCmdFailed, err.Error(), "reason", "spawn"), err)
+}
+
+// Executable returns the absolute path of the running binary (tests pin it
+// as argv[0]).
+func Executable() string {
+	p, _ := os.Executable()
+	return p
+}
diff --git a/internal/fsm/cmdexec/cmdexec_test.go b/internal/fsm/cmdexec/cmdexec_test.go
new file mode 100644
index 0000000..87a8f3b
--- /dev/null
+++ b/internal/fsm/cmdexec/cmdexec_test.go
@@ -0,0 +1,372 @@
+package cmdexec
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"sort"
+	"strconv"
+	"strings"
+	"syscall"
+	"testing"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// TestHelperProcess is the child side of the real-runner rows. Activation is
+// through argv (the environment is scrubbed): everything after "--" is the
+// mode and its arguments.
+func TestHelperProcess(t *testing.T) {
+	args := os.Args
+	i := 0
+	for ; i < len(args); i++ {
+		if args[i] == "--" {
+			break
+		}
+	}
+	if i == len(args) {
+		return
+	}
+	rest := args[i+1:]
+	switch rest[0] {
+	case "echo":
+		stdin, _ := readAll(os.Stdin)
+		env := os.Environ()
+		sort.Strings(env)
+		wd, _ := os.Getwd()
+		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": rest[1:], "env": env, "stdin": string(stdin), "wd": wd})
+	case "exit":
+		n, _ := strconv.Atoi(rest[1])
+		fmt.Fprint(os.Stderr, "failing")
+		os.Exit(n)
+	case "sleep-grandchild":
+		c := exec.Command("sleep", "30")
+		_ = c.Start()
+		fmt.Fprintln(os.Stdout, c.Process.Pid)
+		_ = c.Wait()
+	case "big":
+		n, _ := strconv.Atoi(rest[1])
+		os.Stdout.WriteString(strings.Repeat("x", n))
+	case "bigerr":
+		n, _ := strconv.Atoi(rest[1])
+		os.Stderr.WriteString(strings.Repeat("e", n))
+	case "json":
+		os.Stdout.WriteString(rest[1])
+	case "slow-ok":
+		time.Sleep(200 * time.Millisecond)
+		os.Stdout.WriteString("done")
+	}
+	os.Exit(0)
+}
+
+func readAll(f *os.File) ([]byte, error) {
+	var out []byte
+	buf := make([]byte, 4096)
+	for {
+		n, err := f.Read(buf)
+		out = append(out, buf[:n]...)
+		if err != nil {
+			return out, nil
+		}
+	}
+}
+
+func helperArgv(mode string, args ...string) []string {
+	return append([]string{Executable(), "-test.run=TestHelperProcess", "--", mode}, args...)
+}
+
+type recordingRunner struct {
+	specs []Spec
+	res   Result
+	err   error
+}
+
+func (r *recordingRunner) Run(_ context.Context, s Spec) (Result, error) {
+	r.specs = append(r.specs, s)
+	return r.res, r.err
+}
+
+func hashFor(m map[string]string) func(string) (string, error) {
+	return func(p string) (string, error) {
+		if h, ok := m[p]; ok {
+			return h, nil
+		}
+		return "", errors.New("no such file")
+	}
+}
+
+func TestX1RealRunner(t *testing.T) {
+	ctx := context.Background()
+	exe := Executable()
+	hashes := map[string]string{exe: "hexe"}
+	var audits []run.CmdCallData
+	g := Guarded{
+		Runner: NewExecRunner(), Dir: t.TempDir(), RunID: "mrv-x1-run", FileHash: hashFor(hashes),
+		Environ: func() []string {
+			return []string{"PATH=/usr/bin:/bin", "HOME=/home/t", "SECRET_TOKEN=s3cr3t", "TOKEN=tok", "LANG=C"}
+		},
+		Audit: func(ev run.Event) error {
+			var d run.CmdCallData
+			_ = json.Unmarshal(ev.Data, &d)
+			audits = append(audits, d)
+			return nil
+		},
+	}
+	t.Setenv("SECRET_TOKEN", "parent-secret")
+	g.Allowed = []run.AllowedCmd{
+		{Name: "echo", Argv: helperArgv("echo", "; rm -rf x", "$HOME", "*", "two words"), FileHashes: map[string]string{exe: "hexe"}, Env: []string{"TOKEN", "UNSET_NAME"}},
+		{Name: "exit3", Argv: helperArgv("exit", "3"), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "grand", Argv: helperArgv("sleep-grandchild"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 300},
+		{Name: "slow", Argv: helperArgv("slow-ok"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 2000},
+		{Name: "big", Argv: helperArgv("big", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "bigok", Argv: helperArgv("big", strconv.Itoa(MaxOutput)), FileHashes: map[string]string{exe: "hexe"}},
+		{Name: "bigerr", Argv: helperArgv("bigerr", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
+	}
+	// argv verbatim, exact env, stdin, dir
+	res, err := g.Run(ctx, "echo", []byte("input-bytes"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var got struct {
+		Args  []string `json:"args"`
+		Env   []string `json:"env"`
+		Stdin string   `json:"stdin"`
+		WD    string   `json:"wd"`
+	}
+	if err := json.Unmarshal(res.Stdout, &got); err != nil {
+		t.Fatalf("%v: %s", err, res.Stdout)
+	}
+	if strings.Join(got.Args, "|") != "; rm -rf x|$HOME|*|two words" {
+		t.Fatalf("argv not literal: %v", got.Args)
+	}
+	wantEnv := []string{"HOME=/home/t", "LANG=C", "MRV_RUN_ID=mrv-x1-run", "PATH=/usr/bin:/bin", "TOKEN=tok"}
+	if strings.Join(got.Env, ",") != strings.Join(wantEnv, ",") {
+		t.Fatalf("env set:\n%v\n%v", got.Env, wantEnv)
+	}
+	if got.Stdin != "input-bytes" {
+		t.Fatal("stdin")
+	}
+	if wd, _ := filepath.EvalSymlinks(g.Dir); got.WD != wd && got.WD != g.Dir {
+		t.Fatalf("dir %s vs %s", got.WD, g.Dir)
+	}
+	a := audits[0]
+	if a.Name != "echo" || a.Argv[0] != exe || a.ExitCode != 0 || a.Error != "" || a.InputHash != sha("input-bytes") || a.Stdout == "" || a.DurationMS < 0 {
+		t.Fatalf("audit: %+v", a)
+	}
+	// exit code
+	_, err = g.Run(ctx, "exit3", nil)
+	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("exit") != "3" || audits[1].ExitCode != 3 || !strings.HasPrefix(audits[1].Stderr, "failing") || audits[1].Error != CodeCmdFailed {
+		t.Fatalf("exit: %v %+v", err, audits[1])
+	}
+	// timeout kills the group: grandchild gone, elapsed within [timeout, timeout+WaitDelay+1s]
+	start := time.Now()
+	res, err = g.Run(ctx, "grand", nil)
+	elapsed := time.Since(start)
+	if !errs.Is(err, CodeCmdTimeout) || elapsed < 300*time.Millisecond || elapsed > 300*time.Millisecond+WaitDelay+time.Second {
+		t.Fatalf("timeout: %v after %s", err, elapsed)
+	}
+	pid, _ := strconv.Atoi(strings.TrimSpace(string(res.Stdout)))
+	if pid <= 0 {
+		t.Fatalf("grandchild pid not reported: %q", res.Stdout)
+	}
+	deadline := time.Now().Add(3 * time.Second)
+	for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
+		time.Sleep(50 * time.Millisecond)
+	}
+	if syscall.Kill(pid, 0) == nil {
+		_ = syscall.Kill(pid, syscall.SIGKILL)
+		t.Fatal("grandchild survived the timeout")
+	}
+	if audits[2].ExitCode != -1 || audits[2].Error != CodeCmdTimeout {
+		t.Fatalf("timeout audit: %+v", audits[2])
+	}
+	// positive timeout row: 2 s budget, 200 ms child
+	if res, err := g.Run(ctx, "slow", nil); err != nil || string(res.Stdout) != "done" {
+		t.Fatalf("slow-ok: %v", err)
+	}
+	// output caps: exactly MaxOutput accepted, over → too_large (stdout and stderr)
+	if res, err := g.Run(ctx, "bigok", nil); err != nil || len(res.Stdout) != MaxOutput {
+		t.Fatalf("at cap: %v %d", err, len(res.Stdout))
+	}
+	if _, err := g.Run(ctx, "big", nil); !errs.Is(err, CodeCmdOutputInvalid) || errs.As(err).Field("reason") != "too_large" {
+		t.Fatalf("over cap: %v", err)
+	}
+	if _, err := g.Run(ctx, "bigerr", nil); !errs.Is(err, CodeCmdOutputInvalid) {
+		t.Fatalf("stderr over cap: %v", err)
+	}
+	last := audits[len(audits)-1]
+	if !last.StderrTruncated || last.Error != CodeCmdOutputInvalid {
+		t.Fatalf("truncation flag: %+v", last)
+	}
+	// parent-context cancellation is returned as ctx.Err(), not a timeout
+	cctx, cancel := context.WithCancel(ctx)
+	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
+	if _, err := g.Run(cctx, "grand", nil); !errors.Is(err, context.Canceled) {
+		t.Fatalf("parent cancel: %v", err)
+	}
+	// spawn failure: pinned binary vanished
+	g.Allowed = append(g.Allowed, run.AllowedCmd{Name: "gone", Argv: []string{"/nonexistent/binary-xyz"}, FileHashes: map[string]string{}})
+	_, err = g.Run(ctx, "gone", nil)
+	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("reason") != "spawn" || audits[len(audits)-1].ExitCode != -1 {
+		t.Fatalf("spawn: %v", err)
+	}
+	// default timeout observed through a recording runner
+	rr := &recordingRunner{res: Result{Stdout: []byte("{}")}}
+	g2 := Guarded{Runner: rr, Allowed: []run.AllowedCmd{{Name: "d", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}}, {Name: "ms", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}, TimeoutMS: 1500}}, FileHash: hashFor(nil)}
+	if _, err := g2.Run(ctx, "d", nil); err != nil || rr.specs[0].Timeout != DefaultTimeout {
+		t.Fatalf("default timeout: %v %s", err, rr.specs[0].Timeout)
+	}
+	if _, err := g2.Run(ctx, "ms", nil); err != nil || rr.specs[1].Timeout != 1500*time.Millisecond {
+		t.Fatalf("timeout unit: %s", rr.specs[1].Timeout)
+	}
+	// the exec runner with a zero Timeout falls back to the default
+	er := NewExecRunner()
+	if _, err := er.Run(ctx, Spec{Argv: helperArgv("json", `{}`), Env: []string{}}); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func sha(s string) string {
+	h := run.OutputHash
+	_ = h
+	sum := sha256Sum([]byte(s))
+	return sum
+}
+
+func TestX2Guarded(t *testing.T) {
+	ctx := context.Background()
+	hashes := map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}
+	var audits []run.CmdCallData
+	auditErr := error(nil)
+	rr := &recordingRunner{res: Result{Stdout: []byte(`{"stop": true, "reason": "r"}`), Stderr: []byte("e"), Duration: 1500 * time.Millisecond}}
+	g := Guarded{
+		Runner: rr, Dir: "/w", RunID: "mrv-x2", FileHash: hashFor(hashes),
+		Allowed: []run.AllowedCmd{
+			{Name: "ok", Argv: []string{"/bin/sh", "./s.sh"}, FileHashes: map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}, TimeoutMS: 1500, Env: []string{"TOKEN"}},
+			{Name: "rel", Argv: []string{"sh"}, FileHashes: map[string]string{}},
+		},
+		Environ:  func() []string { return []string{"TOKEN=t", "PATH=/bin", "IGNORED=x"} },
+		CmdCalls: func(name string) int { return 7 },
+		Audit: func(ev run.Event) error {
+			if auditErr != nil {
+				return auditErr
+			}
+			var d run.CmdCallData
+			_ = json.Unmarshal(ev.Data, &d)
+			audits = append(audits, d)
+			return nil
+		},
+	}
+	// not-allowed and relative argv[0]: refused, not audited
+	if _, err := g.Run(ctx, "nope", nil); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
+		t.Fatalf("not allowed: %v", err)
+	}
+	if err := g.Call(ctx, "nope", nil, &struct{}{}); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
+		t.Fatalf("not allowed call: %v", err)
+	}
+	if _, err := g.Run(ctx, "rel", nil); !errs.Is(err, CodeCmdNotAllowed) || errs.As(err).Field("reason") != "relative" || len(audits) != 0 {
+		t.Fatalf("relative: %v", err)
+	}
+	// hash mismatch / missing / appeared: refused, not audited
+	hashes["/w/s.sh"] = "changed"
+	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != "ERR_CMD_CHANGED" || len(audits) != 0 {
+		t.Fatalf("changed: %v", err)
+	}
+	hashes["/w/s.sh"] = "h2"
+	// success: pinned argv executed verbatim, env exact, ordinal, audit fields
+	res, err := g.Run(ctx, "ok", []byte("in"))
+	if err != nil || string(res.Stdout) != `{"stop": true, "reason": "r"}` || res.Duration != 1500*time.Millisecond {
+		t.Fatalf("run: %v %+v", err, res)
+	}
+	sp := rr.specs[0]
+	if sp.Name != "ok" || sp.Ordinal != 7 || strings.Join(sp.Argv, " ") != "/bin/sh ./s.sh" || sp.Dir != "/w" || string(sp.Stdin) != "in" || sp.Timeout != 1500*time.Millisecond || strings.Join(sp.Env, ",") != "PATH=/bin,MRV_RUN_ID=mrv-x2,TOKEN=t" {
+		t.Fatalf("spec: %+v", sp)
+	}
+	a := audits[0]
+	if a.Name != "ok" || a.InputHash != sha256Sum([]byte("in")) || a.Stdout != `{"stop": true, "reason": "r"}` || a.Stderr != "e" || a.DurationMS != 1500 || a.Error != "" || a.ExitCode != 0 {
+		t.Fatalf("audit: %+v", a)
+	}
+	// Call decodes the full stdout; the audited copy is truncated when large
+	big := `{"stop": true, "reason": "` + strings.Repeat("r", run.MaxDetail) + `"}`
+	rr.res = Result{Stdout: []byte(big)}
+	var out struct {
+		Stop   bool   `json:"stop"`
+		Reason string `json:"reason"`
+	}
+	if err := g.Call(ctx, "ok", nil, &out); err != nil || !out.Stop || len(out.Reason) != run.MaxDetail {
+		t.Fatalf("call full stdout: %v", err)
+	}
+	if last := audits[len(audits)-1]; !last.StdoutTruncated || last.Error != "" || len(audits) != 2 {
+		t.Fatalf("one audit per call, truncated copy: %+v", last)
+	}
+	// decode failures carry the error in the same single cmd_call
+	for _, bad := range []string{`{"stop": true, "extra": 1}`, `nope`} {
+		rr.res = Result{Stdout: []byte(bad)}
+		err := g.Call(ctx, "ok", nil, &out)
+		if e := errs.As(err); e == nil || e.Code != CodeCmdOutputInvalid || e.Field("reason") != "decode" || audits[len(audits)-1].Error != CodeCmdOutputInvalid {
+			t.Fatalf("decode %q: %v", bad, err)
+		}
+	}
+	// failure exit through Call: audited with exit and no decode attempted
+	rr.res = Result{ExitCode: 4}
+	if err := g.Call(ctx, "ok", nil, &out); errs.Code(err) != CodeCmdFailed || audits[len(audits)-1].ExitCode != 4 {
+		t.Fatalf("call exit: %v", err)
+	}
+	// runner error with a code (timeout) and without (spawn)
+	rr.res, rr.err = Result{ExitCode: -1}, errs.E(CodeCmdTimeout, "slow")
+	if _, err := g.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdTimeout) || audits[len(audits)-1].Error != CodeCmdTimeout || audits[len(audits)-1].ExitCode != -1 {
+		t.Fatalf("timeout passthrough: %v", err)
+	}
+	rr.err = errors.New("fork failed")
+	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != CodeCmdFailed || errs.As(err).Field("reason") != "spawn" {
+		t.Fatalf("spawn wrap: %v", err)
+	}
+	rr.err = nil
+	// audit failure propagates from Run and Call
+	auditErr = errors.New("store full")
+	rr.res = Result{Stdout: []byte(`{}`)}
+	if _, err := g.Run(ctx, "ok", nil); err == nil || err.Error() != "store full" {
+		t.Fatal("run audit error")
+	}
+	if err := g.Call(ctx, "ok", nil, &struct{}{}); err == nil || err.Error() != "store full" {
+		t.Fatal("call audit error")
+	}
+	auditErr = nil
+	// nil Audit / nil CmdCalls / nil Environ are tolerated
+	g3 := Guarded{Runner: rr, Allowed: g.Allowed, Dir: "/w", FileHash: hashFor(hashes)}
+	if _, err := g3.Run(ctx, "ok", nil); err != nil || rr.specs[len(rr.specs)-1].Ordinal != 0 || strings.Join(rr.specs[len(rr.specs)-1].Env, ",") != "MRV_RUN_ID=" {
+		t.Fatalf("nil seams: %v %+v", err, rr.specs[len(rr.specs)-1])
+	}
+	// the Caller interface is satisfied and usable as a converge.Runner
+	var c converge.Caller = g
+	var _ converge.Runner = c
+	// output exactly at cap accepted; over cap refused through the recording runner too
+	rr.res = Result{Stdout: make([]byte, MaxOutput)}
+	if _, err := g3.Run(ctx, "ok", nil); err != nil {
+		t.Fatal("at cap")
+	}
+	rr.res = Result{Stderr: make([]byte, MaxOutput+1)}
+	if _, err := g3.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdOutputInvalid) {
+		t.Fatal("over cap")
+	}
+}
+
+func TestX3CappingWriter(t *testing.T) {
+	var w cappingWriter
+	n, _ := w.Write(make([]byte, MaxOutput))
+	m, _ := w.Write(make([]byte, 10))
+	k, _ := w.Write(make([]byte, 10))
+	if n != MaxOutput || m != 10 || k != 10 || w.buf.Len() != MaxOutput+1 {
+		t.Fatalf("capping writer keeps draining: %d", w.buf.Len())
+	}
+	if Executable() == "" || !filepath.IsAbs(Executable()) {
+		t.Fatal("Executable is absolute")
+	}
+}
diff --git a/internal/fsm/cmdexec/sha_test.go b/internal/fsm/cmdexec/sha_test.go
new file mode 100644
index 0000000..4043b3e
--- /dev/null
+++ b/internal/fsm/cmdexec/sha_test.go
@@ -0,0 +1,11 @@
+package cmdexec
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+)
+
+func sha256Sum(b []byte) string {
+	s := sha256.Sum256(b)
+	return hex.EncodeToString(s[:])
+}
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index c37f440..1e34295 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -49,6 +49,13 @@ type Result struct {
 	Reason string
 }
 
+// Caller is Runner plus the typed-decode Call the cmd kind needs; the single
+// Guarded factory returns one and the machine hands it to executors.
+type Caller interface {
+	Runner
+	Call(ctx context.Context, name string, stdin []byte, out any) error
+}
+
 // Predicate is one node of the convergence tree.
 type Predicate interface {
 	Name() string
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
index 59ea529..346c2c9 100644
--- a/internal/fsm/machine/harness_test.go
+++ b/internal/fsm/machine/harness_test.go
@@ -283,6 +283,14 @@ func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge
 	return f.res, f.err
 }
 
+func (f *fakeRunner) Call(ctx context.Context, name string, stdin []byte, out any) error {
+	res, err := f.Run(ctx, name, stdin)
+	if err != nil {
+		return err
+	}
+	return json.Unmarshal(res.Stdout, out)
+}
+
 // ---- harness ----
 
 type harness struct {
@@ -309,7 +317,7 @@ func newHarness(t *testing.T) *harness {
 	h.deps = Deps{
 		Store: h.store, Sidecar: h.sidecar, Kinds: h.reg,
 		Git: func(dir string) gate.Git { return h.git.get(dir) },
-		Runner: func(d RunnerDeps) converge.Runner {
+		Runner: func(d RunnerDeps) converge.Caller {
 			h.runner.audit = d.Audit
 			h.runner.ordinal = d.CmdCalls
 			return h.runner
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index 11b88bb..7a25fc4 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -37,7 +37,7 @@ type session struct {
 	log      run.Log
 	w        *workflow.Workflow // resolved
 	pred     converge.Predicate
-	runner   converge.Runner
+	runner   converge.Caller
 	git      gate.Git
 	warns    []string
 	auditErr error          // the first store error seen through the audit closure
@@ -148,15 +148,14 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 		if !filepath.IsAbs(dir) {
 			dir = filepath.Join(o.RepoRoot, dir)
 		}
-		h, err := deps.MockLoad(dir)
-		if err != nil {
-			return nil, errs.E(CodeMockInvalid, err.Error(), "dir", o.MockDir)
-		}
-		root := filepath.Clean(o.RepoRoot) + string(filepath.Separator)
-		if !strings.HasPrefix(filepath.Clean(dir)+string(filepath.Separator), root) {
+		rel, inside := relInside(o.RepoRoot, dir)
+		if !inside {
 			return nil, errs.E(CodeMockInvalid, "mock scenario must live inside the repository", "dir", o.MockDir, "reason", "outside")
 		}
-		rel := strings.TrimPrefix(filepath.Clean(dir), root)
+		h, err := deps.MockLoad(dir)
+		if err != nil || len(h) < 16 {
+			return nil, errs.E(CodeMockInvalid, fmt.Sprintf("scenario load failed: %v", err), "dir", o.MockDir)
+		}
 		mock = rel + "#" + h[:16]
 	}
 	if deps.Kinds.Mock() != (mock != "") {
@@ -208,6 +207,20 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	return m, nil
 }
 
+// relInside returns dir relative to root ("." for root itself) and whether
+// dir is inside root. Both must be absolute (Init checks).
+func relInside(root, dir string) (string, bool) {
+	rootC, dirC := filepath.Clean(root), filepath.Clean(dir)
+	if dirC == rootC {
+		return ".", true
+	}
+	prefix := rootC + string(filepath.Separator)
+	if !strings.HasPrefix(dirC, prefix) {
+		return "", false
+	}
+	return strings.TrimPrefix(dirC, prefix), true
+}
+
 // consentList renders the human-readable command list for ERR_CMDS_NOT_ALLOWED.
 func consentList(allowed []run.AllowedCmd, workDir string) string {
 	var b strings.Builder
@@ -355,9 +368,12 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 	}
 	if snap.Mock != "" {
 		i := strings.LastIndex(snap.Mock, "#")
+		if i < 0 {
+			return nil, errs.E(CodeMockMismatch, "malformed mock identity", "mock", snap.Mock)
+		}
 		rel, want := snap.Mock[:i], snap.Mock[i+1:]
 		h, err := deps.MockLoad(filepath.Join(snap.RepoRoot, rel))
-		if err != nil || h[:16] != want {
+		if err != nil || len(h) < 16 || h[:16] != want {
 			return nil, errs.E(CodeMockMismatch, "mock scenario changed or missing", "mock", snap.Mock)
 		}
 	}
@@ -552,7 +568,7 @@ func (s *session) advance() (AdvanceResult, error) {
 		if err := s.append(run.TypeTree, tree, ""); err != nil {
 			return AdvanceResult{}, err
 		}
-	case h != snap.TreeHash && (node == nil || node.Kind != "agent-edit"):
+	case h != snap.TreeHash && (node == nil || run.Kind(node.Kind) != run.KindAgentEdit):
 		if snap.RepoMode == "enforcing" {
 			ge := pseudoGate(GateRepoMode, CodeUnsanctionedEdit, "working tree changed outside an agent-edit node:\n"+porcelain)
 			if err := s.gateEvent(GateRepoMode, ge); err != nil {
@@ -596,7 +612,7 @@ func (s *session) runNode(node *workflow.Node, head string) (AdvanceResult, bool
 		diff := Diff{Text: text, Truncated: truncated}
 		if node.Exec == "fork" {
 			ex, _ := s.m.deps.Kinds.Executor(node.Kind)
-			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit})
+			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit, Runner: s.runner})
 			if err != nil {
 				if s.ctx.Err() != nil {
 					return AdvanceResult{}, true, err
@@ -697,6 +713,9 @@ func (s *session) transitions(head string) (AdvanceResult, error) {
 			if err != nil && s.auditErr != nil {
 				return AdvanceResult{}, s.auditErr // the atom's cmd_call could not be stored: abort, not a gate failure
 			}
+			if err != nil && s.ctx.Err() != nil {
+				return AdvanceResult{}, err // interrupted inside an atom: resumable, never a gate failure
+			}
 			if err != nil || (r.Stop && r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
 				detail := "convergence evaluation failed"
 				reason := "error"
@@ -828,6 +847,9 @@ func (s *session) overflowHandler() error {
 	if s.auditErr != nil {
 		return s.auditErr // the runner's own cmd_call could not be stored: abort, the handler is retried on resume
 	}
+	if s.ctx.Err() != nil {
+		return s.ctx.Err() // interrupted: nothing recorded, the handler is retried on resume
+	}
 	var argv []string
 	for _, c := range snap.AllowedCmds {
 		if c.Name == name {
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index f1ebc44..aab74a0 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -113,6 +113,7 @@ func TestM1InitErrors(t *testing.T) {
 		{"base-unknown", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "nope"}, nil, gate.CodeGit},
 		{"workdir-relative", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "rel"}, nil, CodeWorkdirForeign},
 		{"mock-outside-root", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "/other/scen"}, func() { h.mockHash["/other/scen"] = strings.Repeat("b", 64) }, CodeMockInvalid},
+		{"mock-short-hash", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/short"}, func() { h.mockHash["/repo/scen/short"] = "abc" }, CodeMockInvalid},
 		{"workdir-foreign", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "/elsewhere"}, func() {
 			h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git"}
 		}, CodeWorkdirForeign},
@@ -143,6 +144,18 @@ func TestM1InitErrors(t *testing.T) {
 	if m.View().Snapshot.Mock != "scen/ok#"+strings.Repeat("a", 16) {
 		t.Fatalf("mock id %q", m.View().Snapshot.Mock)
 	}
+	h.mockHash["/repo"] = strings.Repeat("c", 64)
+	if mr := h.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "."}); mr.View().Snapshot.Mock != ".#"+strings.Repeat("c", 16) {
+		t.Fatalf("mock at repo root: %q", mr.View().Snapshot.Mock)
+	}
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); err != nil {
+		t.Fatal(err)
+	}
+	h.mockHash["/repo/scen/ok"] = "short"
+	if _, err := Open(context.Background(), h.deps, m.runID, OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("short hash on open: %v", err)
+	}
+	h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64)
 	if !h.events(m)[1].Mock {
 		t.Fatal("mock runs stamp Mock on every later event")
 	}
@@ -274,7 +287,7 @@ func TestM2ReviewLoop(t *testing.T) {
 		t.Fatalf("reviewed sequence:\n%s\n%s", got, want)
 	}
 	ex := h.reg.execs["match-then-adjudicate"]
-	if len(ex.calls) != 1 || ex.calls[0].StartIndex != 0 || ex.calls[0].Diff.Text != "DIFF" || ex.calls[0].Node.Name != "adjudicate" || ex.calls[0].Node.Model != "gpt-5.2" {
+	if len(ex.calls) != 1 || ex.calls[0].StartIndex != 0 || ex.calls[0].Diff.Text != "DIFF" || ex.calls[0].Node.Name != "adjudicate" || ex.calls[0].Node.Model != "gpt-5.2" || ex.calls[0].Runner == nil {
 		t.Fatalf("exec input: %+v", ex.calls)
 	}
 	evs := h.events(m2)
@@ -1748,6 +1761,68 @@ func TestM9Residue(t *testing.T) {
 	if _, err := Open(ctx, hj.deps, "mrv-crafted-invalid-torn", OpenOptions{Repair: true}); err == nil || !strings.Contains(err.Error(), "stamp") {
 		t.Fatalf("fold-invalid after repair: %v", err)
 	}
+	// malformed mock identity in a crafted init → ERR_MOCK_MISMATCH, not a panic
+	initJ.RunID = "mrv-crafted-bad-mock"
+	initJ.Mock = "nohash"
+	if _, err := hj.store.Create("mrv-crafted-bad-mock", run.Event{SchemaVersion: run.SchemaVersion, At: initJ.CreatedAt, Type: run.TypeInit, Data: run.MarshalCanonical(initJ)}); err != nil {
+		t.Fatal(err)
+	}
+	_ = hj.deps.Sidecar.Write("mrv-crafted-bad-mock", SidecarWorkflow, raw)
+	hj.reg.mock = true
+	if _, err := Open(ctx, hj.deps, "mrv-crafted-bad-mock", OpenOptions{}); !errs.Is(err, CodeMockMismatch) {
+		t.Fatalf("malformed mock: %v", err)
+	}
+	hj.reg.mock = false
+	// on_overflow interrupted by the parent context: nothing recorded, handler retried on resume
+	ho := newHarness(t)
+	wf := sdlcWith(t, ho, "ov-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{max_iterations: 1}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\non_overflow: notify\nrepo_mode: advisory")
+	_, sha, _ := workflow.ResolveCmds(mustResolve(t, ho, wf), "/repo", ho.deps.LookPath, ho.deps.FileHash)
+	allPresent(ho)
+	ho.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	ho.runner.err = context.Canceled
+	mo := ho.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	ho.advance(mo)
+	ho.record(mo, "discover", findings(1))
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.advance(mo)
+	ho.record(mo, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	ho.advance(mo)
+	cctx, ccancel := context.WithCancel(ctx)
+	ccancel()
+	ho.runner.audit = nil
+	if _, err := mo.Advance(cctx); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted handler: %v", err)
+	}
+	if countType(ho.events(mo), run.TypeOverflowHandler) != 0 || mo.View().Snapshot.OverflowHandled {
+		t.Fatal("interrupted handler must not be recorded")
+	}
+	// interrupted inside a cmd atom: returned, never a converge pseudo-gate
+	hc := newHarness(t)
+	wfc := sdlcWith(t, hc, "cv-cancel.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  any: [{cmd: notify}]\ncmds:\n  notify: {argv: [bash, -c, echo]}\nrepo_mode: advisory")
+	_, shac, _ := workflow.ResolveCmds(mustResolve(t, hc, wfc), "/repo", hc.deps.LookPath, hc.deps.FileHash)
+	allPresent(hc)
+	hc.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	hc.runner.err = context.Canceled
+	mc := hc.mustInit(InitOptions{Workflow: wfc, Vars: sdlcVars, AllowCustomCmds: shac})
+	hc.advance(mc)
+	hc.record(mc, "discover", findings(1))
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.advance(mc)
+	hc.record(mc, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	hc.advance(mc)
+	cctx2, ccancel2 := context.WithCancel(ctx)
+	ccancel2()
+	hc.runner.audit = nil
+	if _, err := mc.Advance(cctx2); !errors.Is(err, context.Canceled) {
+		t.Fatalf("interrupted atom: %v", err)
+	}
+	if mc.View().Snapshot.Outcome != "" {
+		t.Fatal("interrupted atom must not fail the run")
+	}
 	// FS Read with bad args
 	if _, err := (FSSidecar{Root: root}).Read("bad id", "x"); errs.As(err).Field("reason") != "path" {
 		t.Fatal("read bad id")
diff --git a/internal/fsm/machine/sidecar.go b/internal/fsm/machine/sidecar.go
index db8c629..fc0bd52 100644
--- a/internal/fsm/machine/sidecar.go
+++ b/internal/fsm/machine/sidecar.go
@@ -69,7 +69,11 @@ func (f FSSidecar) Write(runID, name string, b []byte) error {
 		return sidecarErr("path", err.Error())
 	}
 	_, werr := fh.Write(b)
-	if err := errors.Join(werr, fh.Close()); err != nil {
+	var serr error
+	if syncer, ok := fh.(interface{ Sync() error }); ok {
+		serr = syncer.Sync()
+	}
+	if err := errors.Join(werr, serr, fh.Close()); err != nil {
 		return sidecarErr("path", err.Error())
 	}
 	return nil
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index a3758b5..9e87efe 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -38,6 +38,7 @@ type ExecInput struct {
 	Diff       Diff
 	StartIndex int
 	Audit      func(run.Event) error
+	Runner     converge.Caller // the session's guarded runner (same audit closure and ordinal source)
 }
 
 // NodeKind describes one kind of node work.
@@ -87,7 +88,7 @@ type Deps struct {
 	Sidecar   Sidecar
 	Kinds     Registry
 	Git       func(workDir string) gate.Git
-	Runner    func(RunnerDeps) converge.Runner
+	Runner    func(RunnerDeps) converge.Caller
 	Clock     Clock
 	LookPath  func(string) (string, error)
 	FileHash  func(string) (string, error)
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index f6bc894..56f1abd 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -76,6 +76,14 @@ type Golden struct {
 }
 
 // Bug is a confirmed bug (the fix/verify unit).
+// Bug.Verdict vocabulary (design §6).
+const (
+	VerdictMatched       = "matched"
+	VerdictRealButUngold = "real_but_ungold"
+	VerdictHallucination = "hallucination"
+)
+
+// Bug is a confirmed (or rejected) finding.
 type Bug struct {
 	ID         string  `json:"id"`
 	Desc       string  `json:"desc"`
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
index 8c22ca6..fa25ea7 100644
--- a/internal/fsm/workflow/resolve.go
+++ b/internal/fsm/workflow/resolve.go
@@ -59,6 +59,11 @@ func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow,
 	for s, n := range w.Nodes {
 		nn := *n
 		nn.Model, nn.Effort = sub(n.Model), sub(n.Effort)
+		for _, v := range []string{nn.Model, nn.Effort} {
+			if _, over := run.CapText(v, run.MaxShort); over {
+				return nil, nil, invalid("bad_var", "nodes."+n.Name, "resolved model/effort exceeds MaxShort")
+			}
+		}
 		nn.Params = make(map[string]any, len(n.Params))
 		for k, v := range n.Params {
 			switch x := v.(type) {
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index d513546..1d7c438 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -339,6 +339,9 @@ func TestW3Resolve(t *testing.T) {
 		t.Fatalf("params: %v", r2.Nodes["discover"].Params)
 	}
 	// errors
+	if _, _, err := w.Resolve(map[string]string{"JUDGE": strings.Repeat("m", run.MaxShort+1), "JUDGE_EFFORT": "b"}, false); !errs.Is(err, CodeWorkflowInvalid) || errs.As(err).Field("reason") != "bad_var" {
+		t.Fatalf("over-long model: %v", err)
+	}
 	if _, _, err := w.Resolve(map[string]string{"JUDGE": "a"}, false); !errs.Is(err, CodeVarUnset) || errs.As(err).Field("name") != "JUDGE_EFFORT" {
 		t.Fatalf("unset: %v", err)
 	}


--- docs/tasks/m1-m6-fsm-packages.md
+# M1–M6: internal/fsm core packages
+
+Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
+`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
+(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).
+
+## Acceptance
+
+- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
+- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
+- `bash tests/coverage.sh` passes (legacy floor held).
+- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
+- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

coverage gate run after commit 1d6284b (M4/M5/M6 complete):
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed

[exited with code 0]

