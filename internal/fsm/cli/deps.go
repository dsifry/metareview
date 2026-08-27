// Package cli is `metareview fsm …` (spec 5): a thin shell over machine, kind/judge/cmdexec/mockai, run, record,
// export and workflows. Every OS and provider effect sits behind Deps so Run is tested at 100% with fakes.
package cli

import (
	"context"
	"crypto/rand"
	"net/http"
	"os"
	"os/exec"
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
	Getenv    func(string) string
	Environ   func() []string
	Now       func() time.Time
	After     func(time.Duration) <-chan time.Time // the judge retry ladder's timer
	Rand      func([]byte) (int, error)
	LookPath  func(string) (string, error)
	FileHash  func(string) (string, error)
	ReadFile  func(string) ([]byte, error)
	Exec      gate.Exec
	HTTP      judge.Doer
	Store     func(root string) run.RunStore
	Sidecar   func(root string) machine.Sidecar
	ExportFS  export.FS
	MockLoad  func(dir string) (*mockai.Scenario, error)
	Workflows func(name string) ([]byte, error)
	Terminal  func(root string, clock func() run.Time) func(context.Context, machine.View) error
	Exists    func(root, runID string) (bool, error)
	Runner    func(r machine.RunnerDeps, env func() []string, fileHash func(string) (string, error), now func() time.Time, real cmdexec.Runner) converge.Caller
}

// HTTPTimeout is the judge client timeout.
const HTTPTimeout = 180 * time.Second

// RealDeps binds every seam to its real implementation and nothing else; it cannot fail.
func RealDeps() Deps {
	return Deps{
		Getenv:    os.Getenv,
		Environ:   os.Environ,
		Now:       time.Now,
		After:     time.After,
		Rand:      rand.Read,
		LookPath:  exec.LookPath,
		FileHash:  workflow.FileSHA256,
		ReadFile:  os.ReadFile,
		Exec:      gate.RealExec,
		HTTP:      newHTTPClient(),
		Store:     func(root string) run.RunStore { return run.NewJSONLStore(root, run.Options{}) },
		Sidecar:   func(root string) machine.Sidecar { return machine.FSSidecar{Root: root} },
		ExportFS:  export.OSFS{},
		MockLoad:  mockai.Load,
		Workflows: workflows.Read,
		Terminal:  record.Terminal,
		Exists:    record.Exists,
		Runner:    guardedRunner,
	}
}

// newHTTPClient is judge.NewHTTPClient with proxy environment variables switched off (spec 5 §8).
func newHTTPClient() *http.Client {
	c := judge.NewHTTPClient(HTTPTimeout)
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	c.Transport = t
	return c
}

func guardedRunner(r machine.RunnerDeps, env func() []string, fileHash func(string) (string, error), now func() time.Time, real cmdexec.Runner) converge.Caller {
	return cmdexec.Guarded{Runner: real, Allowed: r.Allowed, Dir: r.WorkDir, RunID: r.RunID, FileHash: fileHash, Audit: r.Audit, Environ: env, Clock: now, CmdCalls: r.CmdCalls}
}
