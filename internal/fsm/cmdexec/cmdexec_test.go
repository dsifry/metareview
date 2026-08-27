package cmdexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// TestHelperProcess is the child side of the real-runner rows. Activation is
// through argv (the environment is scrubbed): everything after "--" is the
// mode and its arguments.
func TestHelperProcess(t *testing.T) {
	args := os.Args
	i := 0
	for ; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
	}
	if i == len(args) {
		return
	}
	rest := args[i+1:]
	switch rest[0] {
	case "echo":
		stdin, _ := readAll(os.Stdin)
		env := os.Environ()
		sort.Strings(env)
		wd, _ := os.Getwd()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": rest[1:], "env": env, "stdin": string(stdin), "wd": wd})
	case "exit":
		n, _ := strconv.Atoi(rest[1])
		fmt.Fprint(os.Stderr, "failing")
		os.Exit(n)
	case "sleep-grandchild":
		c := exec.Command("sleep", "30")
		_ = c.Start()
		fmt.Fprintln(os.Stdout, c.Process.Pid)
		_ = c.Wait()
	case "big":
		n, _ := strconv.Atoi(rest[1])
		os.Stdout.WriteString(strings.Repeat("x", n))
	case "bigerr":
		n, _ := strconv.Atoi(rest[1])
		os.Stderr.WriteString(strings.Repeat("e", n))
	case "json":
		os.Stdout.WriteString(rest[1])
	case "slow-ok":
		time.Sleep(200 * time.Millisecond)
		os.Stdout.WriteString("done")
	}
	os.Exit(0)
}

func readAll(f *os.File) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, nil
		}
	}
}

func helperArgv(mode string, args ...string) []string {
	return append([]string{Executable(), "-test.run=TestHelperProcess", "--", mode}, args...)
}

type recordingRunner struct {
	specs []Spec
	res   Result
	err   error
}

func (r *recordingRunner) Run(_ context.Context, s Spec) (Result, error) {
	r.specs = append(r.specs, s)
	return r.res, r.err
}

func hashFor(m map[string]string) func(string) (string, error) {
	return func(p string) (string, error) {
		if h, ok := m[p]; ok {
			return h, nil
		}
		return "", errors.New("no such file")
	}
}

func TestX1RealRunner(t *testing.T) {
	ctx := context.Background()
	exe := Executable()
	hashes := map[string]string{exe: "hexe"}
	var audits []run.CmdCallData
	g := Guarded{
		Runner: NewExecRunner(), Dir: t.TempDir(), RunID: "mrv-x1-run", FileHash: hashFor(hashes),
		Environ: func() []string {
			return []string{"PATH=/usr/bin:/bin", "HOME=/home/t", "SECRET_TOKEN=s3cr3t", "TOKEN=tok", "LANG=C"}
		},
		Audit: func(ev run.Event) error {
			var d run.CmdCallData
			_ = json.Unmarshal(ev.Data, &d)
			audits = append(audits, d)
			return nil
		},
	}
	t.Setenv("SECRET_TOKEN", "parent-secret")
	g.Allowed = []run.AllowedCmd{
		{Name: "echo", Argv: helperArgv("echo", "; rm -rf x", "$HOME", "*", "two words"), FileHashes: map[string]string{exe: "hexe"}, Env: []string{"TOKEN", "UNSET_NAME"}},
		{Name: "exit3", Argv: helperArgv("exit", "3"), FileHashes: map[string]string{exe: "hexe"}},
		{Name: "grand", Argv: helperArgv("sleep-grandchild"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 300},
		{Name: "slow", Argv: helperArgv("slow-ok"), FileHashes: map[string]string{exe: "hexe"}, TimeoutMS: 2000},
		{Name: "big", Argv: helperArgv("big", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
		{Name: "bigok", Argv: helperArgv("big", strconv.Itoa(MaxOutput)), FileHashes: map[string]string{exe: "hexe"}},
		{Name: "bigerr", Argv: helperArgv("bigerr", strconv.Itoa(MaxOutput+1)), FileHashes: map[string]string{exe: "hexe"}},
	}
	// argv verbatim, exact env, stdin, dir
	res, err := g.Run(ctx, "echo", []byte("input-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args  []string `json:"args"`
		Env   []string `json:"env"`
		Stdin string   `json:"stdin"`
		WD    string   `json:"wd"`
	}
	if err := json.Unmarshal(res.Stdout, &got); err != nil {
		t.Fatalf("%v: %s", err, res.Stdout)
	}
	if strings.Join(got.Args, "|") != "; rm -rf x|$HOME|*|two words" {
		t.Fatalf("argv not literal: %v", got.Args)
	}
	wantEnv := []string{"HOME=/home/t", "LANG=C", "MRV_RUN_ID=mrv-x1-run", "PATH=/usr/bin:/bin", "TOKEN=tok"}
	if strings.Join(got.Env, ",") != strings.Join(wantEnv, ",") {
		t.Fatalf("env set:\n%v\n%v", got.Env, wantEnv)
	}
	if got.Stdin != "input-bytes" {
		t.Fatal("stdin")
	}
	if wd, _ := filepath.EvalSymlinks(g.Dir); got.WD != wd && got.WD != g.Dir {
		t.Fatalf("dir %s vs %s", got.WD, g.Dir)
	}
	a := audits[0]
	if a.Name != "echo" || a.Argv[0] != exe || a.ExitCode != 0 || a.Error != "" || a.InputHash != sha("input-bytes") || a.Stdout == "" || a.DurationMS < 0 {
		t.Fatalf("audit: %+v", a)
	}
	// exit code
	_, err = g.Run(ctx, "exit3", nil)
	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("exit") != "3" || audits[1].ExitCode != 3 || !strings.HasPrefix(audits[1].Stderr, "failing") || audits[1].Error != CodeCmdFailed {
		t.Fatalf("exit: %v %+v", err, audits[1])
	}
	// timeout kills the group: grandchild gone, elapsed within [timeout, timeout+WaitDelay+1s]
	start := time.Now()
	res, err = g.Run(ctx, "grand", nil)
	elapsed := time.Since(start)
	if !errs.Is(err, CodeCmdTimeout) || elapsed < 300*time.Millisecond || elapsed > 300*time.Millisecond+WaitDelay+time.Second {
		t.Fatalf("timeout: %v after %s", err, elapsed)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(res.Stdout)))
	if pid <= 0 {
		t.Fatalf("grandchild pid not reported: %q", res.Stdout)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("grandchild survived the timeout")
	}
	if audits[2].ExitCode != -1 || audits[2].Error != CodeCmdTimeout {
		t.Fatalf("timeout audit: %+v", audits[2])
	}
	// positive timeout row: 2 s budget, 200 ms child
	if res, err := g.Run(ctx, "slow", nil); err != nil || string(res.Stdout) != "done" {
		t.Fatalf("slow-ok: %v", err)
	}
	// output caps: exactly MaxOutput accepted, over → too_large (stdout and stderr)
	if res, err := g.Run(ctx, "bigok", nil); err != nil || len(res.Stdout) != MaxOutput {
		t.Fatalf("at cap: %v %d", err, len(res.Stdout))
	}
	if _, err := g.Run(ctx, "big", nil); !errs.Is(err, CodeCmdOutputInvalid) || errs.As(err).Field("reason") != "too_large" {
		t.Fatalf("over cap: %v", err)
	}
	if _, err := g.Run(ctx, "bigerr", nil); !errs.Is(err, CodeCmdOutputInvalid) {
		t.Fatalf("stderr over cap: %v", err)
	}
	last := audits[len(audits)-1]
	if !last.StderrTruncated || last.Error != CodeCmdOutputInvalid {
		t.Fatalf("truncation flag: %+v", last)
	}
	// parent-context cancellation is returned as ctx.Err(), not a timeout
	cctx, cancel := context.WithCancel(ctx)
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	if _, err := g.Run(cctx, "grand", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancel: %v", err)
	}
	// spawn failure: pinned binary vanished
	g.Allowed = append(g.Allowed, run.AllowedCmd{Name: "gone", Argv: []string{"/nonexistent/binary-xyz"}, FileHashes: map[string]string{}})
	_, err = g.Run(ctx, "gone", nil)
	if e := errs.As(err); e == nil || e.Code != CodeCmdFailed || e.Field("reason") != "spawn" || audits[len(audits)-1].ExitCode != -1 {
		t.Fatalf("spawn: %v", err)
	}
	// default timeout observed through a recording runner
	rr := &recordingRunner{res: Result{Stdout: []byte("{}")}}
	g2 := Guarded{Runner: rr, Allowed: []run.AllowedCmd{{Name: "d", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}}, {Name: "ms", Argv: []string{"/bin/true"}, FileHashes: map[string]string{}, TimeoutMS: 1500}}, FileHash: hashFor(nil)}
	if _, err := g2.Run(ctx, "d", nil); err != nil || rr.specs[0].Timeout != DefaultTimeout {
		t.Fatalf("default timeout: %v %s", err, rr.specs[0].Timeout)
	}
	if _, err := g2.Run(ctx, "ms", nil); err != nil || rr.specs[1].Timeout != 1500*time.Millisecond {
		t.Fatalf("timeout unit: %s", rr.specs[1].Timeout)
	}
	// the exec runner with a zero Timeout falls back to the default
	er := NewExecRunner()
	if _, err := er.Run(ctx, Spec{Argv: helperArgv("json", `{}`), Env: []string{}}); err != nil {
		t.Fatal(err)
	}
}

func sha(s string) string {
	h := run.OutputHash
	_ = h
	sum := sha256Sum([]byte(s))
	return sum
}

func TestX2Guarded(t *testing.T) {
	ctx := context.Background()
	hashes := map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}
	var audits []run.CmdCallData
	auditErr := error(nil)
	rr := &recordingRunner{res: Result{Stdout: []byte(`{"stop": true, "reason": "r"}`), Stderr: []byte("e"), Duration: 1500 * time.Millisecond}}
	g := Guarded{
		Runner: rr, Dir: "/w", RunID: "mrv-x2", FileHash: hashFor(hashes),
		Allowed: []run.AllowedCmd{
			{Name: "ok", Argv: []string{"/bin/sh", "./s.sh"}, FileHashes: map[string]string{"/bin/sh": "h1", "/w/s.sh": "h2"}, TimeoutMS: 1500, Env: []string{"TOKEN"}},
			{Name: "rel", Argv: []string{"sh"}, FileHashes: map[string]string{}},
		},
		Environ:  func() []string { return []string{"TOKEN=t", "PATH=/bin", "IGNORED=x"} },
		CmdCalls: func(name string) int { return 7 },
		Audit: func(ev run.Event) error {
			if auditErr != nil {
				return auditErr
			}
			var d run.CmdCallData
			_ = json.Unmarshal(ev.Data, &d)
			audits = append(audits, d)
			return nil
		},
	}
	// not-allowed and relative argv[0]: refused, not audited
	if _, err := g.Run(ctx, "nope", nil); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
		t.Fatalf("not allowed: %v", err)
	}
	if err := g.Call(ctx, "nope", nil, &struct{}{}); !errs.Is(err, CodeCmdNotAllowed) || len(audits) != 0 {
		t.Fatalf("not allowed call: %v", err)
	}
	if _, err := g.Run(ctx, "rel", nil); !errs.Is(err, CodeCmdNotAllowed) || errs.As(err).Field("reason") != "relative" || len(audits) != 0 {
		t.Fatalf("relative: %v", err)
	}
	// hash mismatch / missing / appeared: refused, not audited
	hashes["/w/s.sh"] = "changed"
	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != "ERR_CMD_CHANGED" || len(audits) != 0 {
		t.Fatalf("changed: %v", err)
	}
	hashes["/w/s.sh"] = "h2"
	// success: pinned argv executed verbatim, env exact, ordinal, audit fields
	res, err := g.Run(ctx, "ok", []byte("in"))
	if err != nil || string(res.Stdout) != `{"stop": true, "reason": "r"}` || res.Duration != 1500*time.Millisecond {
		t.Fatalf("run: %v %+v", err, res)
	}
	sp := rr.specs[0]
	if sp.Name != "ok" || sp.Ordinal != 7 || strings.Join(sp.Argv, " ") != "/bin/sh ./s.sh" || sp.Dir != "/w" || string(sp.Stdin) != "in" || sp.Timeout != 1500*time.Millisecond || strings.Join(sp.Env, ",") != "PATH=/bin,MRV_RUN_ID=mrv-x2,TOKEN=t" {
		t.Fatalf("spec: %+v", sp)
	}
	a := audits[0]
	if a.Name != "ok" || a.InputHash != sha256Sum([]byte("in")) || a.Stdout != `{"stop": true, "reason": "r"}` || a.Stderr != "e" || a.DurationMS != 1500 || a.Error != "" || a.ExitCode != 0 {
		t.Fatalf("audit: %+v", a)
	}
	// Call decodes the full stdout; the audited copy is truncated when large
	big := `{"stop": true, "reason": "` + strings.Repeat("r", run.MaxDetail) + `"}`
	rr.res = Result{Stdout: []byte(big)}
	var out struct {
		Stop   bool   `json:"stop"`
		Reason string `json:"reason"`
	}
	if err := g.Call(ctx, "ok", nil, &out); err != nil || !out.Stop || len(out.Reason) != run.MaxDetail {
		t.Fatalf("call full stdout: %v", err)
	}
	if last := audits[len(audits)-1]; !last.StdoutTruncated || last.Error != "" || len(audits) != 2 {
		t.Fatalf("one audit per call, truncated copy: %+v", last)
	}
	// decode failures carry the error in the same single cmd_call
	for _, bad := range []string{`{"stop": true, "extra": 1}`, `nope`} {
		rr.res = Result{Stdout: []byte(bad)}
		err := g.Call(ctx, "ok", nil, &out)
		if e := errs.As(err); e == nil || e.Code != CodeCmdOutputInvalid || e.Field("reason") != "decode" || audits[len(audits)-1].Error != CodeCmdOutputInvalid {
			t.Fatalf("decode %q: %v", bad, err)
		}
	}
	// failure exit through Call: audited with exit and no decode attempted
	rr.res = Result{ExitCode: 4}
	if err := g.Call(ctx, "ok", nil, &out); errs.Code(err) != CodeCmdFailed || audits[len(audits)-1].ExitCode != 4 {
		t.Fatalf("call exit: %v", err)
	}
	// runner error with a code (timeout) and without (spawn)
	rr.res, rr.err = Result{ExitCode: -1}, errs.E(CodeCmdTimeout, "slow")
	if _, err := g.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdTimeout) || audits[len(audits)-1].Error != CodeCmdTimeout || audits[len(audits)-1].ExitCode != -1 {
		t.Fatalf("timeout passthrough: %v", err)
	}
	rr.err = errors.New("fork failed")
	if _, err := g.Run(ctx, "ok", nil); errs.Code(err) != CodeCmdFailed || errs.As(err).Field("reason") != "spawn" {
		t.Fatalf("spawn wrap: %v", err)
	}
	rr.err = nil
	// audit failure propagates from Run and Call
	auditErr = errors.New("store full")
	rr.res = Result{Stdout: []byte(`{}`)}
	if _, err := g.Run(ctx, "ok", nil); err == nil || err.Error() != "store full" {
		t.Fatal("run audit error")
	}
	if err := g.Call(ctx, "ok", nil, &struct{}{}); err == nil || err.Error() != "store full" {
		t.Fatal("call audit error")
	}
	auditErr = nil
	// nil Audit / nil CmdCalls / nil Environ are tolerated
	g3 := Guarded{Runner: rr, Allowed: g.Allowed, Dir: "/w", FileHash: hashFor(hashes)}
	if _, err := g3.Run(ctx, "ok", nil); err != nil || rr.specs[len(rr.specs)-1].Ordinal != 0 || strings.Join(rr.specs[len(rr.specs)-1].Env, ",") != "MRV_RUN_ID=" {
		t.Fatalf("nil seams: %v %+v", err, rr.specs[len(rr.specs)-1])
	}
	// the Caller interface is satisfied and usable as a converge.Runner
	var c converge.Caller = g
	var _ converge.Runner = c
	// output exactly at cap accepted; over cap refused through the recording runner too
	rr.res = Result{Stdout: make([]byte, MaxOutput)}
	if _, err := g3.Run(ctx, "ok", nil); err != nil {
		t.Fatal("at cap")
	}
	rr.res = Result{Stderr: make([]byte, MaxOutput+1)}
	if _, err := g3.Run(ctx, "ok", nil); !errs.Is(err, CodeCmdOutputInvalid) {
		t.Fatal("over cap")
	}
}

func TestX3CappingWriter(t *testing.T) {
	var w cappingWriter
	n, _ := w.Write(make([]byte, MaxOutput))
	m, _ := w.Write(make([]byte, 10))
	k, _ := w.Write(make([]byte, 10))
	if n != MaxOutput || m != 10 || k != 10 || w.buf.Len() != MaxOutput+1 {
		t.Fatalf("capping writer keeps draining: %d", w.buf.Len())
	}
	if Executable() == "" || !filepath.IsAbs(Executable()) {
		t.Fatal("Executable is absolute")
	}
}
