package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/cmdexec"
	"github.com/dsifry/metareview/internal/fsm/converge"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/export"
	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/mockai"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Env names the CLI reads (spec 5 §6: the closed set).
const (
	EnvAnthropicKey = "ANTHROPIC_API_KEY"
	EnvOpenAIKey    = "OPENAI_API_KEY"
	EnvAnthropicURL = "ANTHROPIC_BASE_URL"
	EnvOpenAIURL    = "OPENAI_BASE_URL"
	EnvMockAI       = "MOCK_AI"
	EnvJudgeModel   = "METAREVIEW_JUDGE_MODEL"
	EnvJudgeEffort  = "METAREVIEW_JUDGE_EFFORT"
	EnvRunID        = "MRV_RUN_ID"
	EnvHome         = "HOME"
)

// git runs one git command through the Exec seam and returns TRIMMED stdout. It is for short
// outputs - SHAs, ref names, worktree lines. Never use it for file content: the trim silently
// removes leading and trailing blank lines, which shifts every line number below them. Use
// gitRaw for bytes that are going to be read as a file.
func (c *ctxDeps) git(dir string, args ...string) (string, int, error) {
	out, _, code, err := c.deps.Exec(c.ctx, dir, nil, args...)
	return strings.TrimSpace(string(out)), code, err
}

// gitRaw runs one git command and returns stdout byte for byte.
func (c *ctxDeps) gitRaw(dir string, args ...string) ([]byte, int, error) {
	out, _, code, err := c.deps.Exec(c.ctx, dir, nil, args...)
	return out, code, err
}

// ctxDeps binds Deps to one invocation.
type ctxDeps struct {
	ctx  context.Context
	deps Deps
	cwd  string
	// escalate opts in to the sandbox second opinion for rejected cross-file candidates.
	// It is off by default; see escalation for why.
	escalate bool
	// sandboxRoots are the evidence trees this invocation materialized. They are removed when it
	// ends: one machine accumulated 1015 of them (37MB) in a day because nothing ever did.
	sandboxRoots []string
}

// removeSandboxes deletes the evidence trees this invocation created. The trees are written
// read-only (0444 files under 0555 directories) so that a judge cannot edit its own evidence,
// which also means they cannot be removed without restoring write permission first.
func (c *ctxDeps) removeSandboxes() {
	for _, root := range c.sandboxRoots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			_ = os.Chmod(path, 0o700)
			return nil
		})
		_ = os.RemoveAll(root)
	}
	c.sandboxRoots = nil
}

// rootOf resolves the main worktree of cwd (spec 5 §2): the first `worktree` line of `git worktree list --porcelain`;
// a bare main or a non-repository is ERR_NOT_A_REPO.
func (c *ctxDeps) rootOf() (string, error) {
	out, code, err := c.git(c.cwd, "worktree", "list", "--porcelain")
	if err != nil || code != 0 {
		return "", errs.E(CodeNotARepo, "not inside a git repository", "cwd", c.cwd)
	}
	block, _, _ := strings.Cut(out, "\n\n") // the first block is the main worktree; its first line is `worktree <path>`
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		if line == "bare" {
			return "", errs.E(CodeNotARepo, "the main worktree is bare", "reason", "bare")
		}
	}
	return strings.TrimPrefix(lines[0], "worktree "), nil
}

// toplevel is the current worktree (init's WorkDir default).
func (c *ctxDeps) toplevel() (string, error) {
	out, code, err := c.git(c.cwd, "rev-parse", "--show-toplevel")
	if err != nil || code != 0 || out == "" {
		return "", errs.E(CodeNotARepo, "not inside a git worktree", "cwd", c.cwd)
	}
	return out, nil
}

// runsIgnored reports whether .metareview/runs.jsonl is ignored in workDir (git check-ignore exits 0).
func (c *ctxDeps) runsIgnored(workDir string) bool {
	_, code, err := c.git(workDir, "check-ignore", "-q", ".metareview/runs.jsonl")
	return err == nil && code == 0
}

// peek reads the first line of a run's audit.jsonl leniently (spec 5 §8: advisory; Open re-verifies everything).
func (c *ctxDeps) peek(root, runID string) (run.InitData, bool) {
	raw, err := c.deps.ReadFile(filepath.Join(root, ".metareview", "runs", runID, "audit.jsonl"))
	if err != nil {
		return run.InitData{}, false
	}
	if len(raw) > run.MaxLine {
		raw = raw[:run.MaxLine]
	}
	line, _, _ := bytes.Cut(raw, []byte("\n"))
	var ev run.Event
	if json.Unmarshal(line, &ev) != nil || ev.Type != run.TypeInit {
		return run.InitData{}, false
	}
	var d run.InitData
	if json.Unmarshal(ev.Data, &d) != nil {
		return run.InitData{}, false
	}
	return d, true
}

func relInside(root, dir string) (string, bool) {
	rootC, dirC := filepath.Clean(root), filepath.Clean(dir)
	if dirC == rootC {
		return ".", true
	}
	prefix := rootC + string(filepath.Separator)
	if !strings.HasPrefix(dirC, prefix) {
		return "", false
	}
	return strings.TrimPrefix(dirC, prefix), true
}

// scenarioFor loads the mock scenario a run's init names (nil for a product run).
func (c *ctxDeps) scenarioFor(root string, d run.InitData) (*mockai.Scenario, error) {
	if d.Mock == "" {
		return nil, nil
	}
	if d.RepoRoot != root {
		return nil, errs.E(CodeRepoRootMismatch, "the run was created in another checkout; mock runs are path-bound", "stored", d.RepoRoot, "root", root)
	}
	rel, _, _ := strings.Cut(d.Mock, "#")
	dir := filepath.Join(root, rel)
	if _, inside := relInside(root, dir); !inside {
		return nil, errs.E(machine.CodeMockInvalid, "mock scenario must live inside the repository", "dir", rel, "reason", "outside")
	}
	return c.deps.MockLoad(dir)
}

// judgeMode selects how the registry gets its judge.
type judgeMode int

const (
	judgeNone judgeMode = iota // judge-less commands: never read judge env
	judgeReal
)

func (c *ctxDeps) keys() judge.Keys {
	return judge.Keys{Anthropic: c.deps.Getenv(EnvAnthropicKey), OpenAI: c.deps.Getenv(EnvOpenAIKey)}
}

func (c *ctxDeps) newJudge() (judge.Judge, error) {
	return judge.NewWithCodex(c.deps.HTTP, c.keys(),
		judge.URLs{Anthropic: c.deps.Getenv(EnvAnthropicURL), OpenAI: c.deps.Getenv(EnvOpenAIURL)},
		c.nonce, judge.Clock{Now: c.deps.Now, After: c.deps.After}, c.deps.CodexExec)
}

// judgeOverride is the model/effort the operator chose, by flag or environment.
// Precedence is flag → env → the workflow's own var, so a workflow keeps working
// unchanged while an operator can retarget the judge without editing it.
type judgeOverride struct{ Model, Effort string }

func (c *ctxDeps) judgeOverride(modelFlag, effortFlag string) judgeOverride {
	pick := func(flag, env string) string {
		if strings.TrimSpace(flag) != "" {
			return strings.TrimSpace(flag)
		}
		return strings.TrimSpace(c.deps.Getenv(env))
	}
	return judgeOverride{
		Model:  pick(modelFlag, EnvJudgeModel),
		Effort: pick(effortFlag, EnvJudgeEffort),
	}
}

func (c *ctxDeps) nonce() string {
	var b [8]byte
	if _, err := c.deps.Rand(b[:]); err != nil {
		panic(errs.E(CodeInternal, "crypto/rand failed: "+err.Error()))
	}
	return hex.EncodeToString(b[:])
}

// escalation returns the second-opinion provider for this run, or nil to disable it.
//
// Off unless --escalate. The asymmetry that motivates it is real - unattended, a false reject
// drops a real finding and nothing says so, while a false confirm only costs a human a look -
// but the implementation shipped default-on and a review found it both inert and harmful: the
// escalated judge is handed the identical prompt (nothing tells it a tree exists), the tree's
// contents are whitespace-trimmed so line numbers shift, and a git failure is reported as
// "escalation unavailable", which records the finding as a hallucination. A guardrail that
// silently converts real findings into hallucinations is worse than none, so it is opt-in until
// those are fixed. The intent is to return it to default-on; docs/fsm/escalation-reenable.md is
// the checklist for that. Also off for mock and unaudited runs, whose verdicts are fixtures.
func (c *ctxDeps) escalation(root string, scenario *mockai.Scenario, mode judgeMode) kind.EscalateFunc {
	if !c.escalate || scenario != nil || mode != judgeReal {
		return nil
	}
	return c.escalationFor(root)
}

// machineDeps builds the per-run machine wiring (spec 5 §8).
func (c *ctxDeps) machineDeps(root string, scenario *mockai.Scenario, mode judgeMode) (machine.Deps, error) {
	var j judge.Judge
	real := cmdexec.NewExecRunner()
	switch {
	case scenario != nil:
		j = judge.NewMock(scenario.Script())
		real = scenario.Runner()
	case mode == judgeReal:
		var err error
		if j, err = c.newJudge(); err != nil {
			return machine.Deps{}, err
		}
	}
	kinds, _ := kind.New(kind.Deps{Judge: j, Mock: scenario != nil, Escalate: c.escalation(root, scenario, mode)}) // consistent by construction: a mock judge iff a scenario
	d := c.deps
	md := machine.Deps{
		Store: d.Store(root), Sidecar: d.Sidecar(root), Kinds: kinds,
		Git:      func(dir string) gate.Git { return gate.NewExec(dir, d.Exec) },
		Runner:   func(r machine.RunnerDeps) converge.Caller { return d.Runner(r, d.Environ, d.FileHash, d.Now, real) },
		Clock:    func() run.Time { return run.Time{Time: d.Now()} },
		LookPath: d.LookPath, FileHash: d.FileHash, Workflows: d.Workflows, ReadFile: d.ReadFile, Nonce: c.nonce,
		MockLoad: func(dir string) (string, error) {
			s, err := d.MockLoad(dir)
			if err != nil {
				return "", err
			}
			return s.Hash(), nil
		},
		Terminal: d.Terminal(root, func() run.Time { return run.Time{Time: d.Now()} }),
	}
	if scenario == nil && mode == judgeReal {
		keys := c.keys()
		md.Preflight = func(n *workflow.Node, calibration bool) error {
			return judge.Preflight(n.Model, n.Effort, calibration, keys)
		}
	}
	return md, nil
}

func (c *ctxDeps) exportDeps(root string, md machine.Deps) export.Deps {
	return export.Deps{Store: md.Store, Sidecar: md.Sidecar, Kinds: md.Kinds, FS: c.deps.ExportFS, Clock: md.Clock, RepoRoot: root, Home: c.deps.Getenv(EnvHome)}
}

// resolveRun applies the --run precedence: flag → MRV_RUN_ID → newest run without an Error summary.
func (c *ctxDeps) resolveRun(store run.RunStore, flag string) (id string, fromEnv bool, err error) {
	if flag != "" {
		if err := run.ValidateRunID(flag); err != nil {
			return "", false, errs.E(run.CodeRunNotFound, err.Error(), "detail", flag)
		}
		return flag, false, nil
	}
	if env := c.deps.Getenv(EnvRunID); env != "" {
		if err := run.ValidateRunID(env); err != nil {
			return "", false, errs.E(run.CodeRunNotFound, err.Error(), "detail", env)
		}
		return env, true, nil
	}
	list, err := store.List()
	if err != nil {
		return "", false, err
	}
	for _, s := range list {
		if s.Error == "" {
			return s.RunID, false, nil
		}
	}
	return "", false, errs.E(CodeNoRuns, "no FSM runs in this repository; run `metareview fsm init`")
}

// JudgeVar and JudgeEffortVar are the workflow variables an operator retargets.
const (
	JudgeVar       = "JUDGE"
	JudgeEffortVar = "JUDGE_EFFORT"
)

// applyJudgeOverride returns vars with JUDGE and JUDGE_EFFORT replaced by the
// operator's choice, taking a flag first and the environment second. The map is
// copied: the caller's parsed vars are not a place to leave side effects.
func (c *ctxDeps) applyJudgeOverride(vars map[string]string, modelFlag, effortFlag string) map[string]string {
	return c.applyJudgeOverrideFor(vars, modelFlag, effortFlag, false)
}

// applyJudgeOverrideFor is applyJudgeOverride with the calibration rule.
//
// --calibration pins JUDGE and JUDGE_EFFORT so calibration runs stay comparable,
// and workflow.Resolve refuses a run that also supplies them. A flag is a real
// conflict and must still reach Resolve to be reported. The environment is not:
// exporting METAREVIEW_JUDGE_MODEL once would otherwise make every later
// calibration run fail with ERR_CALIBRATION_PINNED naming a flag the operator
// never passed.
func (c *ctxDeps) applyJudgeOverrideFor(vars map[string]string, modelFlag, effortFlag string, calibration bool) map[string]string {
	o := c.judgeOverride(modelFlag, effortFlag)
	if calibration {
		if strings.TrimSpace(modelFlag) == "" {
			o.Model = ""
		}
		if strings.TrimSpace(effortFlag) == "" {
			o.Effort = ""
		}
	}
	if o.Model == "" && o.Effort == "" {
		return vars
	}
	out := make(map[string]string, len(vars)+2)
	for k, v := range vars {
		out[k] = v
	}
	if o.Model != "" {
		out[JudgeVar] = o.Model
	}
	if o.Effort != "" {
		out[JudgeEffortVar] = o.Effort
	}
	return out
}
