package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

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
	"github.com/dsifry/metareview/workflows"
)

// GoldensMaxBytes is the --goldens read cap (spec 2 §5.3).
const GoldensMaxBytes = 512 << 10

// Run is `metareview fsm <args>`; it prints one JSON envelope (or the agent prompt) and returns the exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string, deps Deps) int {
	c := &ctxDeps{ctx: ctx, deps: deps, cwd: cwd}
	inv := &invocation{c: c, stdin: stdin, out: stdout, err: stderr}
	if len(args) == 0 {
		return inv.usage("subcommand required: init|state|advance|record|gate|judge|converge|diff|export|workflows|--agent-prompt")
	}
	cmd := args[0]
	if cmd == "--agent-prompt" {
		_, _ = io.WriteString(stdout, AgentPrompt)
		return 0
	}
	p, err := parseArgs(args[1:])
	if err != nil {
		return inv.usage(err.Error())
	}
	inv.p = p
	c.escalate = p.bools["escalate"] // opt-in; escalation is OFF by default
	switch cmd {
	case "workflows":
		return inv.workflows()
	case "init":
		return inv.init()
	case "state":
		return inv.state()
	case "advance":
		if p.has("from") {
			return inv.fork()
		}
		return inv.advance()
	case "record":
		return inv.record()
	case "gate":
		return inv.gate()
	case "judge":
		return inv.judge()
	case "converge":
		return inv.converge()
	case "diff":
		return inv.diff()
	case "export":
		return inv.export()
	}
	return inv.usage("unknown subcommand " + cmd)
}

// ---- args ----------------------------------------------------------------------------------------

// parsed holds hand-parsed flags: single-valued `--k v`, boolean `--k`, repeated `--var K=V`, and positionals.
type parsed struct {
	flags map[string]string
	bools map[string]bool
	vars  map[string]string
	pos   []string
}

var boolFlags = map[string]bool{"--calibration": true, "--repair": true, "--replace": true, "--accept-workflow-change": true, "--include-vars": true, "--escalate": true}

func parseArgs(args []string) (*parsed, error) {
	p := &parsed{flags: map[string]string{}, bools: map[string]bool{}, vars: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			p.pos = append(p.pos, a)
			continue
		}
		if boolFlags[a] {
			p.bools[strings.TrimPrefix(a, "--")] = true
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, fmt.Errorf("missing value for %s", a)
		}
		v := args[i+1]
		i++
		switch a {
		case "--var":
			k, val, ok := strings.Cut(v, "=")
			if !ok || k == "" {
				return nil, fmt.Errorf("--var expects K=V, got %q", v)
			}
			p.vars[k] = val
		case "--workflow", "--base", "--goldens", "--repo-mode", "--allow-custom-cmds", "--mock-ai", "--work-dir", "--run-id", "--run", "--from", "--at-iter", "--node", "--data", "--input", "--kind", "--model", "--effort", "--context", "--check", "--a", "--b", "--out", "--max-bytes", "--judge-model", "--judge-effort":
			p.flags[strings.TrimPrefix(a, "--")] = v
		default:
			return nil, fmt.Errorf("unknown option %s", a)
		}
	}
	return p, nil
}

func (p *parsed) has(k string) bool { _, ok := p.flags[k]; return ok }

// ---- invocation ----------------------------------------------------------------------------------

type invocation struct {
	c     *ctxDeps
	p     *parsed
	stdin io.Reader
	out   io.Writer
	err   io.Writer
	warns []string // "CODE: detail"
}

func (in *invocation) usage(msg string) int {
	print(in.out, envelope{"ok": false, "status": "ERROR", "code": CodeUsage, "error": map[string]any{"code": CodeUsage, "detail": msg, "detail_truncated": false, "fields": map[string]any{}}, "untrusted": []string{"error.detail"}})
	_, _ = fmt.Fprintln(in.err, msg)
	return 2
}

func (in *invocation) fail(base envelope, err error, ph phase, repairMoved bool) int {
	if len(in.warns) > 0 {
		base["warnings"] = warnObj(in.warns)
	}
	return errEnvelope(in.out, base, err, ph, repairMoved)
}

func (in *invocation) ok(env envelope, status string, exit int) int {
	env["ok"] = exit == 0
	env["status"] = status
	env["warnings"] = warnObj(in.warns)
	print(in.out, env)
	return exit
}

// abs resolves a path flag against cwd.
func (in *invocation) abs(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(in.c.cwd, p)
}

// readCapped reads a --data/--input style value from a file or stdin with a byte cap.
func (in *invocation) readCapped(what, path string, max int) ([]byte, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(io.LimitReader(in.stdin, int64(max)+1))
	} else {
		raw, err = in.c.deps.ReadFile(in.abs(path))
	}
	if err != nil {
		return nil, errs.E(CodeUsage, "cannot read "+what+": "+err.Error(), "what", what)
	}
	if len(raw) > max {
		return nil, errs.E(CodeInputTooLarge, fmt.Sprintf("%s exceeds %d bytes", what, max), "what", what, "max", strconv.Itoa(max))
	}
	return raw, nil
}

// openRun resolves --run, peeks the init line for the mock scenario, builds the wiring, and opens the run.
type opened struct {
	root     string
	id       string
	md       machine.Deps
	m        *machine.Machine
	scenario bool
}

func (in *invocation) openRun(mode judgeMode, readOnly, repair bool) (*opened, envelope, int, bool) {
	c := in.c
	base := envelope{}
	root, err := c.rootOf()
	if err != nil {
		return nil, base, in.fail(base, err, phaseOpen, false), false
	}
	store := c.deps.Store(root)
	id, fromEnv, err := c.resolveRun(store, in.p.flags["run"])
	if err != nil {
		return nil, base, in.fail(base, err, phaseOpen, false), false
	}
	base["run_id"] = id
	if fromEnv {
		in.warns = append(in.warns, WarnRunIDFromEnv+": --run defaulted to "+EnvRunID)
	}
	if in.p.has("mock-ai") {
		return nil, base, in.usage("--mock-ai is an init flag; later commands read the scenario from the run"), false
	}
	init, _ := c.peek(root, id)
	scenario, err := c.scenarioFor(root, init)
	if err != nil {
		return nil, base, in.fail(base, err, phaseOpen, false), false
	}
	md, err := c.machineDeps(root, scenario, mode)
	if err != nil {
		return nil, base, in.fail(base, err, phaseOpen, false), false
	}
	m, err := machine.Open(c.ctx, md, id, machine.OpenOptions{Repair: repair, ReadOnly: readOnly})
	if err != nil {
		moved := repair && errs.Is(err, run.CodeRunNotFound)
		return nil, base, in.fail(base, err, phaseOpen, moved), false
	}
	return &opened{root: root, id: id, md: md, m: m, scenario: scenario != nil}, base, 0, true
}

// ---- commands -------------------------------------------------------------------------------------

func (in *invocation) workflows() int {
	list := []map[string]any{}
	for _, name := range workflows.Names() {
		raw, _ := in.c.deps.Workflows(name)
		reg, _ := kind.New(kind.Deps{})
		w, err := workflow.Parse(raw, workflow.Options{Kinds: reg.Info()})
		if err != nil {
			return in.fail(envelope{}, err, phaseNone, false)
		}
		states := []string{}
		for _, s := range w.States {
			states = append(states, string(s))
		}
		list = append(list, map[string]any{"name": w.Name, "version": w.Version, "hash": w.Hash, "states": states, "source": "embedded"})
	}
	return in.ok(envelope{"workflows": list}, StatusOK, 0)
}

func (in *invocation) init() int {
	c, p := in.c, in.p
	base := envelope{}
	if !p.has("workflow") {
		return in.usage("--workflow is required")
	}
	root, err := c.rootOf()
	if err != nil {
		return in.fail(base, err, phaseInit, false)
	}
	workDir := in.abs(p.flags["work-dir"])
	if workDir == "" {
		if workDir, err = c.toplevel(); err != nil {
			return in.fail(base, err, phaseInit, false)
		}
	}
	mockDir := p.flags["mock-ai"]
	if mockDir == "" {
		mockDir = c.deps.Getenv(EnvMockAI)
	}
	var scenario *mockai.Scenario
	if mockDir != "" {
		dir := mockDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if _, inside := relInside(root, dir); !inside {
			return in.fail(base, errs.E(machine.CodeMockInvalid, "mock scenario must live inside the repository", "dir", mockDir, "reason", "outside"), phaseInit, false)
		}
		if scenario, err = c.deps.MockLoad(dir); err != nil {
			return in.fail(base, err, phaseInit, false)
		}
	}
	md, err := c.machineDeps(root, scenario, judgeReal)
	if err != nil {
		return in.fail(base, err, phaseInit, false)
	}
	if id := p.flags["run-id"]; id != "" {
		exists, err := c.deps.Exists(root, id)
		if err != nil {
			return in.fail(base, err, phaseInit, false)
		}
		if exists {
			return in.fail(base, errs.E("ERR_RUN_EXISTS", "a runs.jsonl row already uses this id", "reason", "row", "run", id), phaseInit, false)
		}
	}
	wf := p.flags["workflow"]
	if strings.Contains(wf, "/") || strings.HasSuffix(wf, ".yaml") {
		wf = in.abs(wf)
	}
	goldens := p.flags["goldens"]
	if goldens != "" {
		goldens = in.abs(goldens)
		if _, err := in.readCapped("--goldens", goldens, GoldensMaxBytes); err != nil {
			return in.fail(base, err, phaseInit, false)
		}
	}
	// The judge override is folded into the run's vars rather than applied at
	// call time, so the model that judged a run is visible in its snapshot and
	// its export. An override the audit cannot see would be worse than none.
	vars := c.applyJudgeOverrideFor(p.vars, p.flags["judge-model"], p.flags["judge-effort"], p.bools["calibration"])
	opts := machine.InitOptions{Workflow: wf, RunID: p.flags["run-id"], Vars: vars, Base: p.flags["base"], RepoMode: p.flags["repo-mode"], AllowCustomCmds: p.flags["allow-custom-cmds"], Calibration: p.bools["calibration"], MockDir: mockDir, GoldensPath: goldens, WorkDir: workDir, RepoRoot: root}
	m, err := machine.Init(c.ctx, md, opts)
	if err != nil {
		if errs.Is(err, machine.CodeCmdsNotAllowed) {
			return in.consentRefusal(base, err, workDir)
		}
		if errs.Is(err, workflow.CodeVarUnset) {
			e := errs.As(err)
			if e.Fields["name"] == "JUDGE" || e.Fields["name"] == "JUDGE_EFFORT" {
				return in.fail(base, errs.E(CodeJudgeUnset, "pass --var "+e.Fields["name"]+"=<value>", "name", e.Fields["name"]), phaseInit, false)
			}
		}
		var se *run.StoreError
		if errors.As(err, &se) && se.Code == run.CodeRunExists {
			return in.fail(base, errs.E(run.CodeRunExists, "a run directory already uses this id", "reason", "dir", "run", p.flags["run-id"]), phaseInit, false)
		}
		return in.fail(base, err, phaseInit, false)
	}
	v := m.View()
	env := envelope{}
	viewKeys(env, v)
	in.warns = append(in.warns, in.warnEvents(md.Store, v.RunID)...)
	if !c.runsIgnored(workDir) {
		in.warns = append(in.warns, WarnRunsNotIgnored+": .metareview/runs.jsonl is not ignored in "+workDir)
	}
	names := []string{}
	for _, a := range v.Snapshot.AllowedCmds {
		names = append(names, a.Name)
	}
	env["allowed_cmds"], env["cmds_sha256"] = names, v.Snapshot.CmdsSHA256
	return in.ok(env, StatusOK, 0)
}

// events reads a run's log for the envelope helpers (nil when the store cannot read it; the envelope then omits
// the derived key rather than failing a command that already succeeded).
func events(store run.RunStore, id string) []run.Event {
	log, err := store.Events(id)
	if err != nil {
		return nil
	}
	return log.Events
}

// warnEvents reads a run's warn events (init returns only a View).
func (in *invocation) warnEvents(store run.RunStore, id string) []string {
	var out []string
	for _, ev := range events(store, id) {
		if ev.Type == run.TypeWarn {
			var d run.WarnData
			_ = json.Unmarshal(ev.Data, &d)
			out = append(out, d.Code+": "+d.Detail)
		}
	}
	return out
}

// consentRefusal prints ERR_CMDS_NOT_ALLOWED with the structured cmds list decoded from cmds_json.
func (in *invocation) consentRefusal(base envelope, err error, workDir string) int {
	e := errs.As(err)
	var allowed []run.AllowedCmd
	_ = json.Unmarshal([]byte(e.Fields["cmds_json"]), &allowed)
	cmds := []map[string]any{}
	for _, a := range allowed {
		pinned := map[string]string{}
		for k, v := range a.FileHashes {
			pinned[k] = v
		}
		unpinned := []string{}
		for _, el := range a.Argv {
			full := el
			if !filepath.IsAbs(el) && workDir != "" {
				full = filepath.Join(workDir, el)
			}
			if _, ok := a.FileHashes[full]; !ok {
				unpinned = append(unpinned, el)
			}
		}
		cmds = append(cmds, map[string]any{"name": a.Name, "argv": a.Argv, "pinned": pinned, "unpinned": unpinned, "timeout_ms": a.TimeoutMS, "env": a.Env})
	}
	env := envelope{"cmds": cmds, "cmds_sha256": e.Fields["sha"], "untrusted": []string{"cmds[].argv", "cmds[].env"}}
	for k, v := range base {
		env[k] = v
	}
	_, _ = fmt.Fprintln(in.err, e.Detail)
	return in.fail(env, err, phaseInit, false)
}

func (in *invocation) state() int {
	o, base, code, ok := in.openRun(judgeNone, true, false)
	if !ok {
		return code
	}
	v := o.m.View()
	env := base
	viewKeys(env, v)
	s := v.Snapshot
	env["next_action"] = v.NextAction
	env["torn"] = v.Torn
	untrusted := []string{}
	if v.FailedGate != nil {
		g := map[string]any{"name": v.FailedGate.Name, "code": "", "detail": ""}
		if v.FailedGate.Error != nil {
			g["code"], g["detail"] = v.FailedGate.Error.Code, v.FailedGate.Error.Detail
		}
		env["failed_gate"] = g
		from, iter := in.failedFrom(o.md.Store, v.RunID)
		env["resume_hint"] = resumeHint(v.RunID, from, iter)
		untrusted = append(untrusted, "failed_gate.detail")
	} else {
		env["failed_gate"] = nil
	}
	if s.LastError != nil {
		env["last_error"] = map[string]any{"code": s.LastError.Code, "detail": s.LastError.Detail}
		untrusted = append(untrusted, "last_error.detail")
	} else {
		env["last_error"] = nil
	}
	outgoing := []map[string]any{}
	for _, e := range v.Outgoing {
		outgoing = append(outgoing, map[string]any{"to": string(e.To), "gate": e.Gate})
	}
	env["outgoing"] = outgoing
	env["lineage"] = s.Lineage
	env["parent_run_id"] = nilIfEmpty(s.ParentRunID)
	env["attempt"] = machine.Attempt(s)
	env["counts"] = counts(s)
	env["untrusted"] = appendSorted(untrusted)
	return in.ok(env, StatusOK, 0)
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func counts(s run.Snapshot) map[string]any {
	return map[string]any{"all_found": len(s.AllFound), "unfixed": s.Unfixed, "confirmed": len(s.Confirmed)}
}

func resumeHint(runID string, state run.State, iter int) string {
	return fmt.Sprintf("metareview fsm advance --run %s --from %s --at-iter %d", runID, state, iter)
}

func (in *invocation) advance() int {
	repair := in.p.bools["repair"]
	o, base, code, ok := in.openRun(judgeReal, false, repair)
	if !ok {
		return code
	}
	moved := false
	if repair {
		if w := in.lastWarn(o.md.Store, o.id, machine.WarnAuditTornLineDropped); w != "" {
			moved = true
			in.warns = append(in.warns, w)
		}
	}
	r, err := o.m.Advance(in.c.ctx)
	v := o.m.View()
	env := base
	viewKeys(env, v)
	if err != nil {
		return in.fail(env, err, phaseAdvance, moved)
	}
	in.warns = append(in.warns, r.Warnings...)
	untrusted := []string{}
	switch r.Status {
	case machine.StatusAdvanced:
		env["from"], env["to"], env["gate"] = string(r.From), string(r.To), in.lastTransitionGate(o.md.Store, v.RunID)
	case machine.StatusNeedsInput:
		ni := r.NeedsInput
		env["node"], env["kind"], env["exec"], env["model"], env["effort"] = ni.Node, ni.Kind, ni.Exec, ni.Model, ni.Effort
		env["instructions"] = ni.Instructions.Text
		env["input"] = ni.Instructions.Input
		env["output_schema"] = ni.Instructions.OutputSchema
		env["record"] = ni.Record
		untrusted = append(untrusted, "instructions")
		for _, k := range ni.Instructions.Untrusted {
			untrusted = append(untrusted, "input."+k)
		}
	case machine.StatusDone:
		env["counts"] = counts(v.Snapshot)
	case machine.StatusStopped:
		env["stop_reason"] = r.StopReason
		env["handler"] = in.handler(o.md.Store, v.RunID)
		untrusted = append(untrusted, "stop_reason", "handler.name")
	case machine.StatusGateFailed:
		g := map[string]any{"name": r.Gate.Name, "passed": false, "code": "", "detail": ""}
		if r.Gate.Error != nil {
			g["code"], g["detail"] = r.Gate.Error.Code, r.Gate.Error.Detail
		}
		env["gate"] = g
		from, iter := in.failedFrom(o.md.Store, v.RunID)
		env["resume_hint"] = resumeHint(v.RunID, from, iter)
		_, _ = fmt.Fprintln(in.err, "fork first, then commit: "+env["resume_hint"].(string))
		untrusted = append(untrusted, "gate.detail")
	}
	env["untrusted"] = appendSorted(untrusted)
	return in.ok(env, r.Status, r.ExitCode)
}

// failedFrom is the state (and iteration) the run was in when its gate failed: the From of the transition into failed.
func (in *invocation) failedFrom(store run.RunStore, id string) (run.State, int) {
	evs := events(store, id)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == run.TypeTransition { // the latest transition is the one into failed (FailedGate implies it)
			var d run.TransitionData
			_ = json.Unmarshal(evs[i].Data, &d)
			return d.From, evs[i].Iter
		}
	}
	return "", 0
}

// lastTransitionGate names the gate of the latest transition (the machine result carries gates only on failure).
func (in *invocation) lastTransitionGate(store run.RunStore, id string) any {
	evs := events(store, id)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == run.TypeTransition {
			var d run.TransitionData
			_ = json.Unmarshal(evs[i].Data, &d)
			return d.Gate
		}
	}
	return nil
}

// lastWarn returns the latest warn event with code as "CODE: detail" ("" when none).
func (in *invocation) lastWarn(store run.RunStore, id, code string) string {
	evs := events(store, id)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == run.TypeWarn {
			var d run.WarnData
			_ = json.Unmarshal(evs[i].Data, &d)
			if d.Code == code {
				return d.Code + ": " + d.Detail
			}
		}
	}
	return ""
}

// handler reads the overflow_handler event's summary for the STOPPED envelope.
func (in *invocation) handler(store run.RunStore, id string) any {
	evs := events(store, id)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == run.TypeOverflowHandler {
			var d run.OverflowHandlerData
			_ = json.Unmarshal(evs[i].Data, &d)
			return map[string]any{"name": d.Name, "stdout_truncated": d.StdoutTruncated, "stderr_truncated": d.StderrTruncated}
		}
	}
	return nil
}

func (in *invocation) fork() int {
	p := in.p
	if !p.has("run") {
		return in.usage("--from needs --run <id>")
	}
	o, base, code, ok := in.openRun(judgeReal, false, false)
	if !ok {
		return code
	}
	fo := machine.ForkOptions{From: run.State(p.flags["from"]), Vars: p.vars, WorkDir: in.abs(p.flags["work-dir"]), AcceptWorkflowChange: p.bools["accept-workflow-change"], AllowCustomCmds: p.flags["allow-custom-cmds"]}
	if p.has("at-iter") {
		n, err := strconv.Atoi(p.flags["at-iter"])
		if err != nil || n < 0 {
			return in.usage("--at-iter must be a non-negative integer")
		}
		fo.AtIter = &n
	}
	if p.has("workflow") {
		wf := p.flags["workflow"]
		var raw []byte
		var err error
		if strings.Contains(wf, "/") || strings.HasSuffix(wf, ".yaml") {
			raw, err = in.readCapped("--workflow", wf, machine.MaxWorkflowBytes+1)
			if errs.Is(err, CodeInputTooLarge) {
				err = errs.E(machine.CodeWorkflowTooLarge, fmt.Sprintf("workflow exceeds %d bytes", machine.MaxWorkflowBytes))
			}
		} else {
			raw, err = in.c.deps.Workflows(wf)
			if err != nil {
				err = errs.E(machine.CodeWorkflowNotFound, err.Error(), "workflow", wf)
			}
		}
		if err != nil {
			return in.fail(base, err, phaseFork, false)
		}
		fo.WorkflowBytes = raw
	}
	child, res, err := o.m.Fork(in.c.ctx, fo)
	if err != nil {
		if errs.Is(err, machine.CodeCmdsNotAllowed) {
			wd := fo.WorkDir
			if wd == "" {
				wd = o.m.View().Snapshot.WorkDir
			}
			return in.consentRefusal(base, err, wd)
		}
		return in.fail(base, err, phaseFork, false)
	}
	env := envelope{}
	viewKeys(env, child.View())
	env["parent_run_id"], env["forked_at_seq"], env["copied"], env["cmds_sha256"], env["dropped_vars"] = o.id, res.ForkedAtSeq, res.Copied, res.CmdsSHA256, res.DroppedVars
	return in.ok(env, StatusForked, 0)
}

func (in *invocation) record() int {
	p := in.p
	if len(p.pos) != 1 || !p.has("data") {
		return in.usage("record needs <node-output|tokens|event> and --data")
	}
	o, base, code, ok := in.openRun(judgeNone, false, false)
	if !ok {
		return code
	}
	kindName := p.pos[0]
	var ro machine.RecordOptions
	switch kindName {
	case machine.RecordNodeOutput:
		if !p.has("node") {
			return in.usage("record node-output needs --node")
		}
		raw, err := in.readCapped("--data", p.flags["data"], run.MaxPayload)
		if err != nil {
			return in.fail(base, err, phaseRecord, false)
		}
		ro = machine.RecordOptions{Kind: machine.RecordNodeOutput, Node: p.flags["node"], Data: raw, Replace: p.bools["replace"]}
	case machine.RecordTokens:
		ro = machine.RecordOptions{Kind: machine.RecordTokens, Data: json.RawMessage(p.flags["data"])}
	default:
		ro = machine.RecordOptions{Kind: machine.RecordEvent, Name: kindName, Data: json.RawMessage(p.flags["data"])}
	}
	r, err := o.m.Record(in.c.ctx, ro)
	env := base
	viewKeys(env, o.m.View())
	if err != nil {
		return in.fail(env, err, phaseRecord, false)
	}
	env["seq"], env["type"], env["key"] = r.Seq, r.Type, r.Key
	return in.ok(env, StatusOK, 0)
}

func (in *invocation) gate() int {
	p := in.p
	if len(p.pos) != 1 {
		return in.usage("gate needs a name")
	}
	name := p.pos[0]
	g, ok := gate.Builtin(name)
	if !ok {
		return in.usage("unknown gate " + name + " (one of " + strings.Join(gate.Names(), ", ") + ")")
	}
	var snap run.Snapshot
	var git gate.Git
	env := envelope{}
	if p.has("input") {
		if !p.has("run") && (name == "commit_exists" || strings.HasPrefix(name, "nothing_")) {
			return in.usage(name + " needs the run's git/snapshot: pass --run")
		}
		raw, err := in.readCapped("--input", p.flags["input"], run.MaxLine)
		if err != nil {
			return in.fail(env, err, phaseNone, false)
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&snap); err != nil || snap.FixEntryHead != "" && !gate.ValidSHA(snap.FixEntryHead) {
			return in.usage("--input is not a snapshot: " + fmt.Sprint(err))
		}
	}
	if p.has("run") || !p.has("input") {
		o, base, code, ok := in.openRun(judgeNone, true, false)
		if !ok {
			return code
		}
		env = base
		if !p.has("input") {
			snap = o.m.View().Snapshot
		}
		git = o.md.Git(snap.WorkDir)
		viewKeys(env, o.m.View())
	}
	gerr := g(in.c.ctx, snap, git)
	res := map[string]any{"name": name, "passed": gerr == nil, "code": "", "detail": ""}
	exit := 0
	if gerr != nil {
		res["code"], res["detail"] = gerr.Code, gerr.Detail
		exit = 1
	}
	env["gate"] = res
	env["untrusted"] = []string{"gate.detail"}
	return in.ok(env, StatusOK, exit)
}

func (in *invocation) judge() int {
	p := in.p
	for _, f := range []string{"kind", "model", "effort", "input"} {
		if !p.has(f) {
			return in.usage("judge needs --" + f)
		}
	}
	k := p.flags["kind"]
	if k == "still-present" {
		k = judge.KindStillPresent
	}
	if k != judge.KindMatch && k != judge.KindAdjudicate && k != judge.KindStillPresent {
		return in.usage("--kind must be match|adjudicate|still-present")
	}
	if (k == judge.KindMatch) == p.has("context") {
		return in.usage("--context is required for adjudicate and still-present and refused for match")
	}
	ph := phaseNone
	if p.has("run") {
		ph = phaseJudge
	}
	raw, err := in.readCapped("--input", p.flags["input"], run.MaxLine)
	if err != nil {
		return in.fail(envelope{}, err, ph, false)
	}
	var diff string
	var diffTruncated bool
	var diffHash string
	if p.has("context") {
		ctxRaw, err := in.readCapped("--context", p.flags["context"], machine.MaxDiffBytes)
		if err != nil {
			return in.fail(envelope{}, err, ph, false)
		}
		diff, diffTruncated, diffHash = judge.CutDiff(string(ctxRaw), false)
	}
	input, err := judgeInput(k, raw, diff, diffTruncated, diffHash)
	if err != nil {
		return in.usage(err.Error())
	}
	if k == "still-present" {
		k = judge.KindStillPresent
	}
	calibration := false
	base := envelope{}
	var o *opened
	if p.has("run") {
		var code int
		var ok bool
		o, base, code, ok = in.openRun(judgeReal, false, false)
		if !ok {
			return code
		}
		calibration = o.m.View().Snapshot.Calibration
		if o.scenario {
			// the scenario answers; pre-flight is skipped for mock runs
		} else if err := judge.Preflight(p.flags["model"], p.flags["effort"], calibration, in.c.keys()); err != nil {
			return in.fail(base, err, ph, false)
		}
	} else if err := judge.Preflight(p.flags["model"], p.flags["effort"], false, in.c.keys()); err != nil {
		return in.fail(base, err, ph, false)
	}
	var j judge.Judge
	if o != nil && o.scenario {
		j = o.md.Kinds.(*kind.Registry).Judge()
	} else if j, err = in.c.newJudge(); err != nil {
		return in.fail(base, err, ph, false)
	}
	req := judge.Request{Kind: k, Model: p.flags["model"], Effort: p.flags["effort"], Input: input, Node: machine.JudgeNode, Fence: true}
	var verdict judge.Verdict
	var callErr error
	env := base
	if o == nil {
		verdict, callErr = j.Call(in.c.ctx, req)
	} else {
		req.RunID = o.id
		viewKeys(env, o.m.View())
		seq, err := o.m.RecordLLMCall(in.c.ctx, func(ctx context.Context, st machine.Stamp) (run.LLMCallData, error) {
			req.Iter, req.Index, req.Fence, req.Calibration = st.Iter, st.Index, st.Fence, st.Calibration
			verdict, callErr = j.Call(ctx, req)
			if ctx.Err() != nil {
				return run.LLMCallData{}, callErr
			}
			d := run.LLMCallData{Kind: k, Model: req.Model, Effort: req.Effort, Index: st.Index, InputHash: verdict.InputHash, Verdict: verdict.Parsed, Confidence: verdict.Confidence, Tokens: verdict.Tokens, DurationMS: verdict.Duration.Milliseconds()}
			if callErr != nil {
				d.Error = errs.Code(callErr)
			}
			return d, callErr
		})
		if err != nil && callErr == nil {
			return in.fail(env, err, phaseJudge, false)
		}
		if err == nil {
			env["seq"], env["index"] = seq, req.Index
		}
	}
	untrusted := appendSorted([]string{"verdict.parsed", "verdict.parse_error"})
	env["verdict"] = map[string]any{"parsed": verdict.Parsed, "parse_error": verdict.ParseError, "decision": verdict.Decision, "confidence": verdict.Confidence, "tokens": verdict.Tokens, "input_hash": verdict.InputHash, "diff_truncated": diffTruncated}
	if callErr != nil {
		env["untrusted"] = untrusted
		return in.fail(env, callErr, ph, false)
	}
	env["error"] = nil
	env["untrusted"] = untrusted
	return in.ok(env, StatusOK, 0)
}

// judgeInput builds the typed input per kind from --input (the diff slot comes from --context).
func judgeInput(k string, raw []byte, diff string, truncated bool, hash string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	switch k {
	case judge.KindMatch:
		var in judge.MatchInput
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf("--input must be {golden, candidate}: %v", err)
		}
		return in, nil
	case judge.KindAdjudicate:
		var in struct {
			Candidate run.Finding `json:"candidate"`
		}
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf("--input must be {candidate}: %v", err)
		}
		return judge.AdjudicateInput{Diff: diff, DiffTruncated: truncated, DiffContextHash: hash, Candidate: in.Candidate}, nil
	default:
		var in struct {
			Bug run.Bug `json:"bug"`
		}
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf("--input must be {bug}: %v", err)
		}
		return judge.StillPresentInput{Bug: in.Bug, Diff: diff, DiffTruncated: truncated, DiffContextHash: hash}, nil
	}
}

func (in *invocation) converge() int {
	p := in.p
	if !p.has("check") {
		return in.usage("converge needs --check <yaml>")
	}
	raw, err := in.readCapped("--check", p.flags["check"], machine.MaxWorkflowBytes)
	if err != nil {
		return in.fail(envelope{}, err, phaseNone, false)
	}
	names := []string{}
	env := envelope{}
	if p.has("run") {
		o, base, code, ok := in.openRun(judgeNone, true, false)
		if !ok {
			return code
		}
		env = base
		viewKeys(env, o.m.View())
		for _, a := range o.m.View().Snapshot.AllowedCmds {
			names = append(names, a.Name)
		}
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return in.fail(env, errs.E(converge.CodeBadConvergence, err.Error(), "detail", err.Error()), phaseNone, false)
	}
	st, err := converge.Describe(&node, names)
	if err != nil {
		if errs.Is(err, converge.CodeBadConvergence) && strings.Contains(errs.As(err).Detail, "unknown cmd") {
			return in.fail(env, errs.E("ERR_CMD_NOT_ALLOWED", errs.As(err).Detail, "name", cmdNameIn(errs.As(err).Detail)), phaseNone, false)
		}
		return in.fail(env, err, phaseNone, false)
	}
	env["atoms"], env["depth"], env["cmds"] = st.Atoms, st.Depth, st.Cmds
	return in.ok(env, StatusOK, 0)
}

func cmdNameIn(detail string) string {
	if i := strings.LastIndex(detail, " "); i >= 0 {
		return strings.Trim(detail[i+1:], `"`)
	}
	return detail
}

func (in *invocation) diff() int {
	p := in.p
	if !p.has("a") || !p.has("b") {
		return in.usage("diff needs --a <run> --b <run>")
	}
	c := in.c
	root, err := c.rootOf()
	if err != nil {
		return in.fail(envelope{}, err, phaseNone, false)
	}
	store := c.deps.Store(root)
	logs := [2]run.Log{}
	for i, id := range []string{p.flags["a"], p.flags["b"]} {
		if err := run.ValidateRunID(id); err != nil {
			return in.fail(envelope{}, errs.E(run.CodeRunNotFound, err.Error(), "detail", id), phaseNone, false)
		}
		if logs[i], err = store.Events(id); err != nil {
			return in.fail(envelope{}, err, phaseNone, false)
		}
	}
	rep, err := machine.DiffRuns(logs[0], logs[1], kind.Decision)
	if err != nil {
		return in.fail(envelope{}, err, phaseNone, false)
	}
	untrusted := []string{}
	for i, row := range rep.Calls {
		if row.A != nil && row.A.Error != "" {
			untrusted = append(untrusted, fmt.Sprintf("report.calls[%d].a.error", i))
		}
		if row.B != nil && row.B.Error != "" {
			untrusted = append(untrusted, fmt.Sprintf("report.calls[%d].b.error", i))
		}
	}
	return in.ok(envelope{"report": rep, "origin_checks": machine.VerifyOrigin(c.ctx, store, logs[1]), "untrusted": untrusted}, StatusOK, 0)
}

func (in *invocation) export() int {
	p := in.p
	o, base, code, ok := in.openRun(judgeNone, true, false)
	if !ok {
		return code
	}
	opts := export.Options{Out: in.abs(p.flags["out"]), IncludeVars: p.bools["include-vars"]}
	if p.has("max-bytes") {
		n, err := strconv.ParseInt(p.flags["max-bytes"], 10, 64)
		if err != nil || n <= 0 {
			return in.usage("--max-bytes must be a positive integer")
		}
		opts.MaxBytes = n
	}
	env := base
	viewKeys(env, o.m.View())
	m, err := export.Export(in.c.ctx, in.c.exportDeps(o.root, o.md), o.id, opts)
	if err != nil {
		return in.fail(env, err, phaseNone, false)
	}
	out := opts.Out
	if out == "" {
		out = filepath.Join(o.root, "docs", "metareview", "fsm", o.id)
	}
	env["manifest"], env["out"], env["untrusted"] = m, out, []string{}
	return in.ok(env, StatusOK, 0)
}

// StatusLines renders the `metareview status` FSM section (spec 5 §6): read-only over Store.List() at the main root.
func StatusLines(ctx context.Context, deps Deps, cwd string) []string {
	c := &ctxDeps{ctx: ctx, deps: deps, cwd: cwd}
	root, err := c.rootOf()
	if err != nil {
		return nil
	}
	list, err := deps.Store(root).List()
	if err != nil {
		code, _, _ := failure(err)
		return []string{"fsm runs: " + code}
	}
	if len(list) == 0 {
		return []string{"fsm runs: none"}
	}
	var good, bad []string
	for _, s := range list {
		if s.Error != "" || s.Torn {
			reason := s.Error
			if s.Torn {
				reason = "torn tail" + map[bool]string{true: "; " + s.Error, false: ""}[s.Error != ""]
			}
			d, _ := run.CapText(reason, run.MaxShort)
			bad = append(bad, fmt.Sprintf("%s  (unreadable: %s)", s.RunID, d))
			continue
		}
		outcome := "running"
		if s.Outcome != "" {
			outcome = string(s.Outcome)
		}
		mock := ""
		if s.Mock != "" || s.MockTainted {
			mock = "  mock"
		}
		good = append(good, fmt.Sprintf("%s  %s  %s%s", s.RunID, s.State, outcome, mock))
	}
	sort.Strings(bad)
	return append(append([]string{"fsm runs:"}, good...), bad...)
}
