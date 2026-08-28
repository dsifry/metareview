package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// ---- harness: a real git repo under a temp root, the real store, a mock scenario, fake env/HTTP -------------------

const sdlcScenario = `calls:
  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5}, expect_model: gpt-5.2}
  - {kind: still-present, node: verify, iter: 0, index: 0, raw: '{"reasoning":"r","still_present":false,"confidence":0.9}', tokens: {input: 3, output: 1}}
  - {kind: adjudicate, node: judge, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.8}', tokens: {input: 1, output: 1}}
  - {kind: adjudicate, node: adjudicate, iter: 1, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5}}
  - {kind: still-present, node: verify, iter: 1, index: 0, raw: '{"reasoning":"r","still_present":false,"confidence":0.9}', tokens: {input: 3, output: 1}}
`

const findingsData = `{"findings":[{"issue_text":"nil deref in f.go","file":"f.go","line":3,"severity":"high","category":"bug","source":"lens"}]}`

type fakeDoer struct {
	reqs []*http.Request
	body string
	code int
	onDo func()
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	f.reqs = append(f.reqs, r)
	if f.onDo != nil {
		f.onDo()
	}
	if f.body == "" {
		return nil, errors.New("no network")
	}
	return &http.Response{StatusCode: f.code, Body: io.NopCloser(strings.NewReader(f.body)), Header: http.Header{}}, nil
}

type harness struct {
	t     *testing.T
	root  string
	cwd   string
	env   map[string]string
	doer  *fakeDoer
	deps  Deps
	stdin string
	out   bytes.Buffer
	errb  bytes.Buffer
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, root, "init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(root, "f.go"), []byte("package f\n"), 0o644)
	git(t, root, "add", "f.go")
	git(t, root, "commit", "-q", "-m", "base")
	_ = os.MkdirAll(filepath.Join(root, "mock"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "mock", "judge.yaml"), []byte(sdlcScenario), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("mock/\nfixtures/\nexp/\nsmall/\ndocs/\n"), 0o644)
	git(t, root, "add", ".gitignore")
	git(t, root, "commit", "-q", "-m", "ignore mock")
	h := &harness{t: t, root: root, cwd: root, env: map[string]string{}, doer: &fakeDoer{}}
	d := RealDeps()
	d.Getenv = func(k string) string { return h.env[k] }
	d.Environ = func() []string { return []string{"PATH=" + os.Getenv("PATH")} }
	tick := 0
	d.Now = func() time.Time {
		tick++
		return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC).Add(time.Duration(tick) * time.Second)
	}
	d.HTTP = h.doer
	d.After = func(time.Duration) <-chan time.Time { ch := make(chan time.Time, 1); ch <- time.Time{}; return ch }
	h.deps = d
	return h
}

func (h *harness) run(args ...string) (map[string]any, int) {
	h.t.Helper()
	h.out.Reset()
	h.errb.Reset()
	code := Run(context.Background(), args, strings.NewReader(h.stdin), &h.out, &h.errb, h.cwd, h.deps)
	var env map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &env); err != nil && args[0] != "--agent-prompt" {
		h.t.Fatalf("stdout is not one JSON object: %q (%v)", h.out.String(), err)
	}
	return env, code
}

func (h *harness) must(status string, exit int, args ...string) map[string]any {
	h.t.Helper()
	env, code := h.run(args...)
	if code != exit || env["status"] != status {
		h.t.Fatalf("%v → status %v exit %d (want %s/%d): %s", args, env["status"], code, status, exit, h.out.String())
	}
	return env
}

func (h *harness) mustErr(code string, exit int, args ...string) map[string]any {
	h.t.Helper()
	env, got := h.run(args...)
	if got != exit || env["code"] != code {
		h.t.Fatalf("%v → code %v exit %d (want %s/%d): %s", args, env["code"], got, code, exit, h.out.String())
	}
	return env
}

func (h *harness) file(name, content string) string {
	h.t.Helper()
	p := filepath.Join(h.root, "fixtures", name)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *harness) commit(msg string) string {
	h.t.Helper()
	_ = os.WriteFile(filepath.Join(h.root, "f.go"), []byte("package f\n// "+msg+"\n"), 0o644)
	git(h.t, h.root, "add", "f.go")
	git(h.t, h.root, "commit", "-q", "-m", msg)
	return git(h.t, h.root, "rev-parse", "HEAD")
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

var mockInit = []string{"init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock"}

// ---- rows ----------------------------------------------------------------------------------------

func TestUsageAndPrompt(t *testing.T) {
	h := newHarness(t)
	h.mustErr(CodeUsage, 2)
	h.mustErr(CodeUsage, 2, "bogus")
	h.mustErr(CodeUsage, 2, "init", "--nope", "x")
	h.mustErr(CodeUsage, 2, "init", "--workflow")
	h.mustErr(CodeUsage, 2, "init", "--var", "novalue")
	h.mustErr(CodeUsage, 2, "init")
	if _, code := h.run("--agent-prompt"); code != 0 || !strings.Contains(h.out.String(), "ERR_RUN_ESCALATED") || !strings.Contains(h.out.String(), "do not delegate it to a sub-agent") {
		t.Fatalf("agent prompt: %d %s", code, h.out.String()[:80])
	}
	env := h.must(StatusOK, 0, "workflows")
	list := env["workflows"].([]any)
	if len(list) != 2 || list[1].(map[string]any)["name"] != "sdlc-loop" || len(list[1].(map[string]any)["states"].([]any)) != 6 {
		t.Fatalf("workflows: %v", list)
	}
	// outside a repository: state refuses, workflows works
	h.cwd = t.TempDir()
	h.mustErr(CodeNotARepo, 2, "state")
	h.must(StatusOK, 0, "workflows")
}

func TestHappySdlcLoop(t *testing.T) {
	h := newHarness(t)
	env := h.must(StatusOK, 0, mockInit...)
	if env["mock"] != true || env["workflow_source"] != "embedded" || env["state"] != "discover" || env["iteration"] != float64(0) || env["outcome"] != nil || env["schema_version"] != float64(1) {
		t.Fatalf("init: %v", env)
	}
	if w := env["warnings"].([]any); len(w) != 1 || w[0].(map[string]any)["code"] != WarnRunsNotIgnored || strs(env["untrusted"])[0] != "warnings[].detail" {
		t.Fatalf("not-ignored warning: %v", env)
	}
	// the flow run starts after the ignore rule is committed (the parent's terminal row must not dirty the tree)
	h.file("../.gitignore", "mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/runs.jsonl\n")
	git(t, h.root, "add", ".gitignore")
	git(t, h.root, "commit", "-q", "-m", "ignore runs.jsonl")
	base := git(t, h.root, "rev-parse", "HEAD")
	env = h.must(StatusOK, 0, mockInit...)
	if len(env["warnings"].([]any)) != 0 {
		t.Fatalf("warnings must be empty with an ignore rule: %v", env["warnings"])
	}
	id := env["run_id"].(string)
	env = h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	if env["node"] != "discover" || env["kind"] != "review-lenses" || env["exec"] != "subagent" || env["model"] != "claude-opus-5" || env["effort"] != "low" || !strings.Contains(env["record"].(string), "record node-output") || !strings.Contains(env["record"].(string), "--node discover") {
		t.Fatalf("needs input: %v", env)
	}
	if got := strings.Join(strs(env["untrusted"]), ","); got != "input.diff,input.findings_so_far,instructions" {
		t.Fatalf("untrusted: %s", got)
	}
	in := env["input"].(map[string]any)
	if in["base_sha"] != base || in["head_sha"] != git(t, h.root, "rev-parse", "HEAD") {
		t.Fatalf("input shas: %v", in)
	}
	// idempotent at NEEDS_INPUT
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	// record from stdin
	h.stdin = findingsData
	env = h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", "-", "--run", id)
	if env["type"] != "node_output" || env["key"] != "discover@0" {
		t.Fatalf("record: %v", env)
	}
	h.stdin = ""
	env = h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	if env["to"] != "adjudicate" || env["gate"] != "findings_nonempty" {
		t.Fatalf("→adjudicate: %v", env)
	}
	env = h.must(machine.StatusAdvanced, 0, "advance", "--run", id) // adjudicate runs against the scenario
	if env["to"] != "fix" {
		t.Fatalf("→fix: %v", env)
	}
	env = h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	if strings.Join(strs(env["untrusted"]), ",") != "input.unfixed_bugs,instructions" {
		t.Fatalf("fix untrusted: %v", env["untrusted"])
	}
	// state while parked
	env = h.must(StatusOK, 0, "state", "--run", id)
	if env["next_action"] != "record" || env["attempt"] != float64(1) || env["counts"].(map[string]any)["confirmed"] != float64(1) || len(env["outgoing"].([]any)) != 1 || env["failed_gate"] != nil {
		t.Fatalf("state: %v", env)
	}
	// no commit → GATE_FAILED with a concrete resume hint, exit 1
	fixData := h.file("fix.json", `{"commit":"`+git(t, h.root, "rev-parse", "HEAD")+`","summary":"fixed"}`)
	h.must(StatusOK, 0, "record", "node-output", "--node", "fix", "--data", fixData, "--run", id)
	env = h.must(machine.StatusGateFailed, 1, "advance", "--run", id)
	gate := env["gate"].(map[string]any)
	if gate["name"] != "commit_exists" || gate["code"] != "ERR_NO_COMMIT" || env["resume_hint"] != "metareview fsm advance --run "+id+" --from fix --at-iter 0" || strs(env["untrusted"])[0] != "gate.detail" || !strings.Contains(h.errb.String(), "fork first") {
		t.Fatalf("gate failed: %v", env)
	}
	env = h.must(StatusOK, 0, "state", "--run", id)
	if env["failed_gate"].(map[string]any)["name"] != "commit_exists" || env["resume_hint"] == nil || env["next_action"] != "none" {
		t.Fatalf("failed state: %v", env)
	}
	// fork → child; commit; verify; done
	env = h.must(StatusForked, 0, "advance", "--run", id, "--from", "fix", "--at-iter", "0")
	child := env["run_id"].(string)
	if child == id || env["parent_run_id"] != id || env["state"] != "fix" || env["copied"] == float64(0) {
		t.Fatalf("forked: %v", env)
	}
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", child)
	sha := h.commit("fix it")
	fixData = h.file("fix2.json", `{"commit":"`+sha+`","summary":"fixed"}`)
	h.must(StatusOK, 0, "record", "node-output", "--node", "fix", "--data", fixData, "--run", child)
	env = h.must(machine.StatusAdvanced, 0, "advance", "--run", child)
	if env["to"] != "verify" {
		t.Fatalf("→verify: %v", env)
	}
	env = h.must(machine.StatusDone, 0, "advance", "--run", child)
	if env["outcome"] != "fixed" || env["counts"].(map[string]any)["unfixed"] != float64(0) {
		t.Fatalf("done: %v", env)
	}
	// terminal: tokens ok, node-output refused (exit 2), advance → ERR_RUN_TERMINAL exit 1
	h.must(StatusOK, 0, "record", "tokens", "--data", `{"output":5}`, "--run", child)
	h.mustErr("ERR_RUN_TERMINAL", 2, "record", "node-output", "--node", "fix", "--data", fixData, "--run", child)
	h.mustErr("ERR_RUN_TERMINAL", 1, "advance", "--run", child)
	h.mustErr("ERR_RUN_TERMINAL", 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "medium", "--input", h.file("cand.json", `{"candidate":{"issue_text":"x"}}`), "--context", h.file("d.diff", "x"), "--run", child)
	// runs.jsonl rows: the child passed, the parent failed
	rows, _ := os.ReadFile(filepath.Join(h.root, ".metareview", "runs.jsonl"))
	if !strings.Contains(string(rows), `"id":"`+child+`"`) || !strings.Contains(string(rows), `"verdict":"PASS"`) || !strings.Contains(string(rows), `"mock":true`) {
		t.Fatalf("rows: %s", rows)
	}
	// diff parent/child and export
	env = h.must(StatusOK, 0, "diff", "--a", id, "--b", child)
	rep := env["report"].(map[string]any)
	if rep["a"] != id || rep["b"] != child || rep["common_prefix_seq"] == float64(0) {
		t.Fatalf("diff: %v", rep)
	}
	env = h.must(StatusOK, 0, "export", "--run", child)
	if !strings.HasSuffix(env["out"].(string), child) {
		t.Fatalf("export: %v", env)
	}
	if _, err := os.Stat(filepath.Join(h.root, "docs", "metareview", "fsm", child, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	h.mustErr("ERR_EXPORT_DEST", 2, "export", "--run", child, "--include-vars")
	h.mustErr(CodeUsage, 2, "export", "--run", child, "--max-bytes", "x")
	h.mustErr("ERR_EXPORT_TOO_LARGE", 2, "export", "--run", child, "--out", filepath.Join(h.root, "small"), "--max-bytes", "10")
	// escalation over three failed forks of the parent lineage is refused with the human-only code
	h.mustErr("ERR_CHECKPOINT_NOT_FOUND", 2, "advance", "--run", id, "--from", "nope")
	// status lines
	lines := StatusLines(context.Background(), h.deps, h.root)
	if len(lines) < 3 || lines[0] != "fsm runs:" || !strings.Contains(strings.Join(lines, "\n"), child+"  done  fixed  mock") {
		t.Fatalf("status: %v", lines)
	}
}

func TestRunResolutionAndRecords(t *testing.T) {
	h := newHarness(t)
	h.mustErr(CodeNoRuns, 2, "state")
	env := h.must(StatusOK, 0, mockInit...)
	id := env["run_id"].(string)
	// env default warns; flag beats env; malformed and unknown ids
	h.env[EnvRunID] = id
	env = h.must(StatusOK, 0, "state")
	if w := env["warnings"].([]any); len(w) != 1 || w[0].(map[string]any)["code"] != WarnRunIDFromEnv {
		t.Fatalf("env warning: %v", env)
	}
	h.env[EnvRunID] = "mrv-doesnotexist00"
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "state")
	h.must(StatusOK, 0, "state", "--run", id)
	h.env[EnvRunID] = "../x"
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "state")
	delete(h.env, EnvRunID)
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "state", "--run", "../x")
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "state", "--run", "mrv-doesnotexist00")
	// a corrupt valid-shaped newest run is skipped by the default
	_ = os.MkdirAll(filepath.Join(h.root, ".metareview", "runs", "mrv-zzzzzzzzzzzz"), 0o700)
	_ = os.WriteFile(filepath.Join(h.root, ".metareview", "runs", "mrv-zzzzzzzzzzzz", "audit.jsonl"), []byte("garbage\n"), 0o600)
	if env := h.must(StatusOK, 0, "state"); env["run_id"] != id {
		t.Fatalf("newest skip: %v", env["run_id"])
	}
	// --mock-ai after init is a usage error; MOCK_AI env on advance is ignored
	h.mustErr(CodeUsage, 2, "advance", "--run", id, "--mock-ai", "mock")
	h.env[EnvMockAI] = "other"
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	delete(h.env, EnvMockAI)
	// record variants
	h.mustErr(CodeUsage, 2, "record")
	h.mustErr(CodeUsage, 2, "record", "node-output", "--data", "x")
	h.must(StatusOK, 0, "record", "note", "--data", `{"n":1}`, "--run", id)
	h.mustErr("ERR_RECORD_NAME", 2, "record", "transition", "--data", `{}`, "--run", id)
	h.mustErr("ERR_RECORD_TOKENS", 2, "record", "tokens", "--data", `{"output":-1}`, "--run", id)
	big := h.file("big.json", strings.Repeat("x", run.MaxPayload+1))
	h.mustErr(CodeInputTooLarge, 2, "record", "node-output", "--node", "discover", "--data", big, "--run", id)
	h.mustErr(CodeUsage, 2, "record", "node-output", "--node", "discover", "--data", "/nope/missing.json", "--run", id)
	h.mustErr("ERR_NODE_OUTPUT_INVALID", 2, "record", "node-output", "--node", "discover", "--data", h.file("bad.json", `{"nope":1}`), "--run", id)
	h.mustErr("ERR_NODE_MISMATCH", 2, "record", "node-output", "--node", "fix", "--data", h.file("f.json", findingsData), "--run", id)
	// gates
	h.mustErr(CodeUsage, 2, "gate")
	h.mustErr(CodeUsage, 2, "gate", "bogus", "--run", id)
	env = h.must(StatusOK, 1, "gate", "findings_nonempty", "--run", id)
	if env["gate"].(map[string]any)["passed"] != false || strs(env["untrusted"])[0] != "gate.detail" {
		t.Fatalf("gate: %v", env)
	}
	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h.file("f.json", findingsData), "--run", id)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	h.must(StatusOK, 0, "gate", "findings_nonempty", "--run", id)
	snap := h.file("snap.json", `{"schemaVersion":1,"run_id":"x","lineage":[],"created_at":"2026-08-27T00:00:00Z","seq":1,"workflow":"w","workflow_hash":"h","vars":{},"calibration":false,"mock_tainted":false,"repo_mode":"advisory","allowed_cmds":[],"repo_root":"/r","work_dir":"/r","state":"discover","iteration":0,"base_sha":"b","head":"h","goldens":[],"findings":[{"issue_text":"x"}],"confirmed":[],"all_found":[],"status":[],"unfixed":0,"prev_unfixed":null,"tokens":{"input":0,"cache_read":0,"cache_create":0,"output":0,"reasoning":0},"node_outputs":{},"applied":{},"nodes_run":[],"overflow_handled":false,"warnings":[]}`)
	h.must(StatusOK, 0, "gate", "findings_nonempty", "--input", snap)
	h.mustErr(CodeUsage, 2, "gate", "commit_exists", "--input", snap)
	h.mustErr(CodeUsage, 2, "gate", "nothing_found", "--input", snap)
	h.mustErr(CodeUsage, 2, "gate", "findings_nonempty", "--input", h.file("bad.json", `{"unknown":1}`))
	h.mustErr(CodeUsage, 2, "gate", "findings_nonempty", "--input", "/nope/missing.json")
	// converge --check
	pred := h.file("pred.yaml", "any: [no_fixation_progress, {max_iterations: 5}]")
	env = h.must(StatusOK, 0, "converge", "--check", pred)
	if env["atoms"] != float64(2) || env["depth"] != float64(1) {
		t.Fatalf("converge: %v", env)
	}
	h.must(StatusOK, 0, "converge", "--check", pred, "--run", id)
	h.mustErr("ERR_CMD_NOT_ALLOWED", 2, "converge", "--check", h.file("cmd.yaml", "any: [{cmd: notify}]"))
	h.mustErr("ERR_BAD_CONVERGENCE", 2, "converge", "--check", h.file("bad.yaml", "any: ["))
	h.mustErr("ERR_BAD_CONVERGENCE", 2, "converge", "--check", h.file("bad2.yaml", "bogus: 1"))
	h.mustErr(CodeUsage, 2, "converge")
	h.mustErr(CodeUsage, 2, "diff", "--a", id)
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "diff", "--a", "../x", "--b", id)
	h.mustErr("ERR_RUN_NOT_FOUND", 2, "diff", "--a", "mrv-doesnotexist00", "--b", id)
	h.mustErr("ERR_DIFF_INCOMPATIBLE", 2, "diff", "--a", id, "--b", h.must(StatusOK, 0, "init", "--workflow", "review-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")["run_id"].(string))
}

func TestInitVariants(t *testing.T) {
	h := newHarness(t)
	// unset judge var → ERR_JUDGE_UNSET; path workflows; reserved name; run-id collisions; mock outside root; env MOCK_AI
	h.mustErr(CodeJudgeUnset, 2, "init", "--workflow", "sdlc-loop", "--mock-ai", "mock")
	raw, _ := h.deps.Workflows("sdlc-loop")
	same := h.file("same.yaml", string(raw))
	env := h.must(StatusOK, 0, "init", "--workflow", same, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	if env["workflow_source"] != "path" {
		t.Fatalf("path source: %v", env)
	}
	fake := h.file("fake.yaml", strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
	if e := h.mustErr("ERR_WORKFLOW_INVALID", 2, "init", "--workflow", fake, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock"); e["error"].(map[string]any)["fields"].(map[string]any)["reason"] != "reserved_name" {
		t.Fatalf("reserved: %v", e)
	}
	h.mustErr("ERR_WORKFLOW_NOT_FOUND", 2, "init", "--workflow", "nope", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	h.mustErr("ERR_MOCK_INVALID", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "/elsewhere")
	h.mustErr("ERR_MOCK_INVALID", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "nodir")
	h.env[EnvMockAI] = "mock"
	env = h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--run-id", "mrv-explicit-000001")
	if env["run_id"] != "mrv-explicit-000001" || env["mock"] != true {
		t.Fatalf("env mock: %v", env)
	}
	if e := h.mustErr("ERR_RUN_EXISTS", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--run-id", "mrv-explicit-000001"); e["error"].(map[string]any)["fields"].(map[string]any)["reason"] != "dir" {
		t.Fatalf("dir collision: %v", e)
	}
	_ = os.WriteFile(filepath.Join(h.root, ".metareview", "runs.jsonl"), []byte(`{"id":"mrv-rowonly-000001","scope":"task-done","status":"passed","verdict":"PASS"}`+"\n"), 0o644)
	if e := h.mustErr("ERR_RUN_EXISTS", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--run-id", "mrv-rowonly-000001"); e["error"].(map[string]any)["fields"].(map[string]any)["reason"] != "row" {
		t.Fatalf("row collision: %v", e)
	}
	_ = os.WriteFile(filepath.Join(h.root, ".metareview", "runs.jsonl"), []byte("not json\n"), 0o644)
	h.mustErr("ERR_RUNS_JSONL", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--run-id", "mrv-rowonly-000002")
	_ = os.Remove(filepath.Join(h.root, ".metareview", "runs.jsonl"))
	delete(h.env, EnvMockAI)
	// goldens cap and missing file; work-dir relative; bad repo mode
	h.mustErr(CodeInputTooLarge, 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock", "--goldens", h.file("g.json", strings.Repeat("x", GoldensMaxBytes+1)))
	h.mustErr(CodeUsage, 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock", "--goldens", "missing.json")
	h.mustErr("ERR_BAD_REPO_MODE", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock", "--repo-mode", "lax")
	env = h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--mock-ai", "mock", "--work-dir", ".", "--repo-mode", "enforcing", "--calibration")
	if env["run_id"] == "" {
		t.Fatal("calibration init")
	}
	// consent refusal carries the structured list; then succeeds with the sha
	wf := h.file("cmds.yaml", strings.Replace(strings.Replace(string(raw), "workflow: sdlc-loop", "workflow: sdlc-cmds", 1), "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./n.sh, --x]}\non_overflow: notify\nrepo_mode: advisory", 1))
	_ = os.WriteFile(filepath.Join(h.root, "n.sh"), []byte("#!/bin/bash\necho ok\n"), 0o755)
	e := h.mustErr("ERR_CMDS_NOT_ALLOWED", 2, "init", "--workflow", wf, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	cmds := e["cmds"].([]any)
	c0 := cmds[0].(map[string]any)
	if len(cmds) != 1 || c0["name"] != "notify" || len(c0["unpinned"].([]any)) != 1 || c0["unpinned"].([]any)[0] != "--x" || e["cmds_sha256"] == "" || !strings.Contains(strings.Join(strs(e["untrusted"]), ","), "cmds[].argv") {
		t.Fatalf("consent list: %v", e)
	}
	env = h.must(StatusOK, 0, "init", "--workflow", wf, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock", "--allow-custom-cmds", e["cmds_sha256"].(string))
	if env["cmds_sha256"] != e["cmds_sha256"] || strs(env["allowed_cmds"])[0] != "notify" {
		t.Fatalf("with consent: %v", env)
	}
	// a workflow with an init-time warning surfaces it on the init envelope
	noClean := strings.Replace(strings.Replace(string(raw), "workflow: sdlc-loop", "workflow: sdlc-noclean", 1), "  - {from: discover,   to: done,       gate: nothing_found,      outcome: clean}   # iteration 0 only: refuses once bugs are known\n", "", 1)
	noClean = strings.Replace(noClean, "  - {from: adjudicate, to: done,       gate: nothing_confirmed,  outcome: clean}\n", "", 1)
	env = h.must(StatusOK, 0, "init", "--workflow", h.file("noclean.yaml", noClean), "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	if w := env["warnings"].([]any); len(w) == 0 || w[0].(map[string]any)["code"] != machine.WarnWorkflow {
		t.Fatalf("workflow warning: %v", env["warnings"])
	}
	// a moved checkout: the stored repo_root differs
	other, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, other, "init", "-q", "-b", "main")
	_ = os.CopyFS(filepath.Join(other, ".metareview"), os.DirFS(filepath.Join(h.root, ".metareview")))
	_ = os.CopyFS(filepath.Join(other, "mock"), os.DirFS(filepath.Join(h.root, "mock")))
	h.cwd = other
	h.mustErr(CodeRepoRootMismatch, 2, "state", "--run", env["run_id"].(string))
	h.cwd = h.root
}

func TestProductRunAndJudge(t *testing.T) {
	h := newHarness(t)
	// no key → pre-flight refuses init before anything is created
	h.mustErr("ERR_JUDGE_KEY", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	if _, err := os.Stat(filepath.Join(h.root, ".metareview", "runs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nothing created")
	}
	h.env[EnvOpenAIKey] = "sekret"
	h.mustErr("ERR_JUDGE_MODEL", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=bogus", "--var", "JUDGE_EFFORT=medium")
	env := h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	id := env["run_id"].(string)
	if env["mock"] != false {
		t.Fatal("product run")
	}
	// judge-less commands never build a judge: a bad base URL is harmless for them
	h.env[EnvOpenAIURL] = "not-a-url"
	h.must(StatusOK, 0, "state", "--run", id)
	h.must(StatusOK, 0, "record", "tokens", "--data", `{"output":1}`, "--run", id)
	h.must(StatusOK, 0, "export", "--run", id, "--out", filepath.Join(h.root, "exp"))
	if len(h.doer.reqs) != 0 {
		t.Fatal("no HTTP traffic from judge-less commands")
	}
	h.mustErr("ERR_JUDGE_URL", 2, "advance", "--run", id)
	if strings.Contains(h.out.String()+h.errb.String(), "sekret") {
		t.Fatal("secret leaked")
	}
	delete(h.env, EnvOpenAIURL)
	// drive to adjudicate against the fake provider; the key reaches the header
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h.file("f.json", findingsData), "--run", id)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	delete(h.env, EnvOpenAIKey)
	h.mustErr("ERR_JUDGE_KEY", 2, "advance", "--run", id)
	h.env[EnvOpenAIKey] = "sekret"
	h.doer.body, h.doer.code = `{"choices":[{"message":{"content":"{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.9}"}}],"usage":{"prompt_tokens":100,"completion_tokens":50}}`, 200
	env = h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	if env["to"] != "fix" || len(h.doer.reqs) != 1 || h.doer.reqs[0].Header.Get("Authorization") != "Bearer sekret" || !strings.Contains(h.doer.reqs[0].URL.Host, "openai.com") {
		t.Fatalf("product adjudicate: %v %v", env, h.doer.reqs)
	}
	// a provider failure surfaces as GATE_FAILED{executor}, exit 1
	h.file("f2.json", findingsData)
	env2 := h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	id2 := env2["run_id"].(string)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id2)
	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h.file("f2.json", findingsData), "--run", id2)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id2)
	h.doer.body = ""
	env = h.must(machine.StatusGateFailed, 1, "advance", "--run", id2)
	if env["gate"].(map[string]any)["code"] != machine.CodeExecutorFailed {
		t.Fatalf("executor gate: %v", env)
	}
	// standalone judge: usage, pre-flight, then a call through the fake provider
	h.mustErr(CodeUsage, 2, "judge", "--kind", "match")
	h.mustErr(CodeUsage, 2, "judge", "--kind", "bogus", "--model", "gpt-5.2", "--effort", "low", "--input", "x")
	h.mustErr(CodeUsage, 2, "judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", "x", "--context", "y")
	h.mustErr(CodeUsage, 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", "x")
	cand := h.file("cand.json", `{"candidate":{"issue_text":"x","file":"f.go","line":1}}`)
	diff := h.file("d.diff", "--- a\n+++ b\n")
	h.mustErr("ERR_JUDGE_EFFORT_UNSUPPORTED", 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "turbo", "--input", cand, "--context", diff)
	h.mustErr(CodeUsage, 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("badc.json", `{"nope":1}`), "--context", diff)
	h.mustErr(CodeUsage, 2, "judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("badm.json", `{"nope":1}`))
	h.mustErr(CodeUsage, 2, "judge", "--kind", "still-present", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("bads.json", `{"nope":1}`), "--context", diff)
	h.mustErr(CodeUsage, 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", "/nope", "--context", diff)
	h.mustErr(CodeUsage, 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", cand, "--context", "/nope")
	h.doer.body = `{"choices":[{"message":{"content":"{\"reasoning\":\"r\",\"is_real\":false,\"confidence\":0.2}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	env = h.must(StatusOK, 0, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", cand, "--context", diff)
	if env["verdict"].(map[string]any)["decision"] != false || strs(env["untrusted"])[0] != "verdict.parse_error" {
		t.Fatalf("judge: %v", env)
	}
	h.must(StatusOK, 0, "judge", "--kind", "still-present", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("bug.json", `{"bug":{"id":"b","desc":"d","verdict":"real_but_ungold","confidence":0.9}}`), "--context", diff)
	h.doer.body = `{"choices":[{"message":{"content":"{\"reasoning\":\"r\",\"match\":true,\"confidence\":0.9}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	h.must(StatusOK, 0, "judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("m.json", `{"golden":{"comment":"g"},"candidate":{"issue_text":"x"}}`))
	h.env[EnvOpenAIURL] = "not-a-url"
	h.mustErr("ERR_JUDGE_URL", 2, "judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("m.json", `{"golden":{"comment":"g"},"candidate":{"issue_text":"x"}}`))
	delete(h.env, EnvOpenAIURL)
	h.doer.body = ""
	h.mustErr("ERR_JUDGE_TRANSPORT", 2, "judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("m.json", `{"golden":{"comment":"g"},"candidate":{"issue_text":"x"}}`))
	// judge --run on a mock run: recorded under the run, index continues; terminal run refused
	h.env = map[string]string{}
	env = h.must(StatusOK, 0, mockInit...)
	mid := env["run_id"].(string)
	env = h.must(StatusOK, 0, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "medium", "--input", cand, "--context", diff, "--run", mid)
	if env["index"] != float64(0) || env["seq"] == nil || env["verdict"].(map[string]any)["decision"] != true {
		t.Fatalf("judge --run: %v", env)
	}
	h.mustErr("ERR_MOCK_UNSCRIPTED", 1, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "medium", "--input", cand, "--context", diff, "--run", mid)
	// product run + judge --run pre-flight refuses without a key
	h.mustErr("ERR_JUDGE_KEY", 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "low", "--input", cand, "--context", diff, "--run", id2)
}

func TestTornRepairAndForkErrors(t *testing.T) {
	h := newHarness(t)
	env := h.must(StatusOK, 0, mockInit...)
	id := env["run_id"].(string)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	audit := filepath.Join(h.root, ".metareview", "runs", id, "audit.jsonl")
	f, _ := os.OpenFile(audit, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"torn`)
	_ = f.Close()
	h.mustErr("ERR_AUDIT_TORN", 1, "advance", "--run", id)
	if env := h.must(StatusOK, 0, "state", "--run", id); env["torn"] != true {
		t.Fatalf("torn state: %v", env)
	}
	env = h.must(machine.StatusNeedsInput, 3, "advance", "--run", id, "--repair")
	if w := env["warnings"].([]any); len(w) != 1 || w[0].(map[string]any)["code"] != machine.WarnAuditTornLineDropped {
		t.Fatalf("repair warning: %v", env)
	}
	h.mustErr("ERR_AUDIT_NOT_TORN", 2, "advance", "--run", id, "--repair")
	// fork usage and refusals
	h.mustErr(CodeUsage, 2, "advance", "--from", "fix")
	h.mustErr(CodeUsage, 2, "advance", "--run", id, "--from", "fix", "--at-iter", "-1")
	h.mustErr("ERR_CHECKPOINT_NOT_FOUND", 2, "advance", "--run", id, "--from", "fix")
	h.mustErr("ERR_WORKFLOW_NOT_FOUND", 2, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--accept-workflow-change", "--workflow", "nope")
	h.mustErr(machine.CodeWorkflowTooLarge, 2, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--accept-workflow-change", "--workflow", h.file("huge.yaml", strings.Repeat("#", machine.MaxWorkflowBytes+1)))
	h.mustErr(CodeUsage, 2, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--accept-workflow-change", "--workflow", "missing/x.yaml")
	raw, _ := h.deps.Workflows("sdlc-loop")
	changed := h.file("changed.yaml", strings.Replace(string(raw), "effort: $REV_EFFORT", "effort: high", 1))
	h.mustErr("ERR_WORKFLOW_CHANGED", 2, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--workflow", changed)
	env = h.must(StatusForked, 0, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--accept-workflow-change", "--workflow", changed)
	if env["workflow_source"] != "path" {
		t.Fatalf("fork with new bytes: %v", env)
	}
	// an embedded name for --workflow on fork
	h.must(StatusForked, 0, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--workflow", "sdlc-loop")
}

func TestExitTableAndHelpers(t *testing.T) {
	cases := []struct {
		code string
		ph   phase
		mov  bool
		want int
	}{
		{"ERR_RUN_NOT_FOUND", phaseOpen, false, 2}, {"ERR_RUN_NOT_FOUND", phaseOpen, true, 1},
		{"ERR_GIT", phaseFork, false, 2}, {"ERR_GIT", phaseAdvance, false, 1}, {"ERR_CMD_CHANGED", phaseOpen, false, 2}, {"ERR_CMD_CHANGED", phaseAdvance, false, 1},
		{"ERR_JUDGE_KEY", phaseAdvance, false, 2}, {"ERR_JUDGE_KEY", phaseAdvance, true, 1},
		{"ERR_RUN_TERMINAL", phaseAdvance, false, 1}, {"ERR_RUN_TERMINAL", phaseRecord, false, 2},
		{"ERR_RUN_LOCKED", phaseOpen, false, 2}, {"ERR_RUN_LOCKED", phaseInit, false, 1},
		{"ERR_STORE_PATH", phaseInit, false, 2}, {"ERR_STORE_PATH", phaseAdvance, false, 1},
		{"ERR_RUNS_JSONL", phaseInit, false, 2}, {"ERR_RUNS_JSONL", phaseAdvance, false, 1},
		{"ERR_AUDIT_TORN", phaseOpen, false, 1}, {"ERR_AUDIT_NOT_TORN", phaseOpen, false, 2}, {"ERR_WHATEVER", phaseNone, false, 2}, {"ERR_INTERNAL", phaseAdvance, false, 1},
	}
	for _, c := range cases {
		if got := exitFor(c.code, c.ph, c.mov); got != c.want {
			t.Fatalf("%s/%s/%v: %d", c.code, c.ph, c.mov, got)
		}
	}
	// error mapping shapes
	for _, c := range []struct {
		err  error
		code string
	}{
		{&run.StoreError{Code: "ERR_AUDIT_CAS", Seq: 3}, "ERR_AUDIT_CAS"},
		{&run.FoldError{Code: "ERR_AUDIT_INVALID", Reason: "x", Seq: 2, Type: "gate"}, "ERR_AUDIT_INVALID"},
		{context.Canceled, CodeInterrupted},
		{errors.New("boom"), CodeInternal},
	} {
		if code, obj, untrusted := failure(c.err); code != c.code || obj["code"] != c.code || untrusted[0] != "error.detail" {
			t.Fatalf("%v: %s %v", c.err, code, obj)
		}
	}
	if w := warnObj([]string{"CODE: detail", "BARE"}); w[0]["detail"] != "detail" || w[1]["code"] != "BARE" {
		t.Fatalf("warnObj: %v", w)
	}
	if cmdNameIn(`undeclared cmd "x"`) != "x" || cmdNameIn("nospace") != "nospace" {
		t.Fatal("cmdNameIn")
	}
	if StatusLines(context.Background(), RealDeps(), t.TempDir()) != nil {
		t.Fatal("status outside a repo prints nothing")
	}
	if newHTTPClient().Transport.(*http.Transport).Proxy != nil {
		t.Fatal("proxy must be off")
	}
	h := newHarness(t)
	h.deps.Rand = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("rand failure panics into ERR_INTERNAL")
		}
	}()
	(&ctxDeps{deps: h.deps}).nonce()
}

const cmdsScenario = `calls:
  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5}}
  - {kind: still-present, node: verify, iter: 0, index: 0, raw: '{"reasoning":"r","still_present":true,"confidence":0.9}', tokens: {input: 3, output: 1}}
cmds:
  - {name: notify, call: 0, stdout: '{"stop": false, "reason": ""}', stderr: "", exit: 0, repeat: true}
`

// cmdsWorkflow derives an sdlc workflow with a declared command, an overflow handler and a one-iteration cap.
func cmdsWorkflow(h *harness) string {
	raw, _ := h.deps.Workflows("sdlc-loop")
	body := strings.Replace(string(raw), "workflow: sdlc-loop", "workflow: sdlc-cmds", 1)
	body = strings.Replace(body, "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "  any: [{max_iterations: 1}]", 1)
	body = strings.Replace(body, "repo_mode: advisory", "cmds:\n  notify: {argv: [bash, ./n.sh]}\non_overflow: notify\nrepo_mode: advisory", 1)
	_ = os.WriteFile(filepath.Join(h.root, "n.sh"), []byte("#!/bin/bash\necho ok\n"), 0o755)
	git(h.t, h.root, "add", "n.sh")
	git(h.t, h.root, "commit", "-q", "-m", "handler script")
	return h.file("cmds.yaml", body)
}

func TestOverflowHandlerForkConsentAndStatus(t *testing.T) {
	h := newHarness(t)
	_ = os.WriteFile(filepath.Join(h.root, "mock", "judge.yaml"), []byte(cmdsScenario), 0o644)
	wf := cmdsWorkflow(h)
	e := h.mustErr("ERR_CMDS_NOT_ALLOWED", 2, "init", "--workflow", wf, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	sha := e["cmds_sha256"].(string)
	env := h.must(StatusOK, 0, "init", "--workflow", wf, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock", "--allow-custom-cmds", sha)
	id := env["run_id"].(string)
	// converge --check --run sees the run's allowed names
	pred := h.file("pred.yaml", "any: [{cmd: notify}]")
	env = h.must(StatusOK, 0, "converge", "--check", pred, "--run", id)
	if strs(env["cmds"])[0] != "notify" {
		t.Fatalf("converge with run: %v", env)
	}
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h.file("f.json", findingsData), "--run", id)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
	sha1 := h.commit("fix")
	h.must(StatusOK, 0, "record", "node-output", "--node", "fix", "--data", h.file("fix.json", `{"commit":"`+sha1+`","summary":"s"}`), "--run", id)
	h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
	// verify says still present → loop → max_iterations 1 → overflow → STOPPED with the handler
	env = h.must(machine.StatusStopped, 1, "advance", "--run", id)
	if env["stop_reason"] == nil || env["handler"] == nil || env["handler"].(map[string]any)["name"] != "notify" || strings.Join(strs(env["untrusted"]), ",") != "handler.name,stop_reason" {
		t.Fatalf("stopped: %v", env)
	}
	// a workflow change that adds a command asks for consent through the fork
	body, _ := os.ReadFile(wf)
	more := h.file("more.yaml", strings.Replace(string(body), "  notify: {argv: [bash, ./n.sh]}", "  notify: {argv: [bash, ./n.sh]}\n  extra: {argv: [bash, ./n.sh, --extra]}", 1))
	e = h.mustErr("ERR_CMDS_NOT_ALLOWED", 2, "advance", "--run", id, "--from", "verify", "--at-iter", "0", "--accept-workflow-change", "--workflow", more)
	if len(e["cmds"].([]any)) != 2 {
		t.Fatalf("fork consent: %v", e)
	}
	env = h.must(StatusForked, 0, "advance", "--run", id, "--from", "verify", "--at-iter", "0", "--accept-workflow-change", "--workflow", more, "--allow-custom-cmds", e["cmds_sha256"].(string))
	child := env["run_id"].(string)
	if env := h.must(StatusOK, 0, "state", "--run", child); env["parent_run_id"] != id || env["attempt"] != float64(2) {
		t.Fatalf("child state: %v", env)
	}
	// diff of a run against itself with an errored llm_call marks both sides
	h2 := newHarness(t)
	h2.env[EnvOpenAIKey] = "k"
	env = h2.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	pid := env["run_id"].(string)
	h2.must(machine.StatusNeedsInput, 3, "advance", "--run", pid)
	h2.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h2.file("f.json", findingsData), "--run", pid)
	h2.must(machine.StatusAdvanced, 0, "advance", "--run", pid)
	h2.must(machine.StatusGateFailed, 1, "advance", "--run", pid)
	env = h2.must(StatusOK, 0, "diff", "--a", pid, "--b", pid)
	if u := strs(env["untrusted"]); len(u) != 2 || u[0] != "report.calls[0].a.error" {
		t.Fatalf("diff untrusted: %v", u)
	}
	// status lines: none, then a torn and a corrupt run
	h3 := newHarness(t)
	if l := StatusLines(context.Background(), h3.deps, h3.root); len(l) != 1 || l[0] != "fsm runs: none" {
		t.Fatalf("none: %v", l)
	}
	env = h3.must(StatusOK, 0, mockInit...)
	tid := env["run_id"].(string)
	f, _ := os.OpenFile(filepath.Join(h3.root, ".metareview", "runs", tid, "audit.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"torn`)
	_ = f.Close()
	_ = os.MkdirAll(filepath.Join(h3.root, ".metareview", "runs", "mrv-corrupt-000001"), 0o700)
	_ = os.WriteFile(filepath.Join(h3.root, ".metareview", "runs", "mrv-corrupt-000001", "audit.jsonl"), []byte("garbage\n"), 0o600)
	l := strings.Join(StatusLines(context.Background(), h3.deps, h3.root), "\n")
	if !strings.Contains(l, tid+"  (unreadable: torn tail)") || !strings.Contains(l, "mrv-corrupt-000001  (unreadable:") {
		t.Fatalf("status with damage: %s", l)
	}
	_ = os.RemoveAll(filepath.Join(h3.root, ".metareview", "runs"))
	_ = os.WriteFile(filepath.Join(h3.root, ".metareview", "runs"), []byte("x"), 0o644)
	if l := StatusLines(context.Background(), h3.deps, h3.root); len(l) != 1 || !strings.HasPrefix(l[0], "fsm runs: ERR_") {
		t.Fatalf("list error: %v", l)
	}
	h3.mustErr("ERR_STORE_PATH", 2, "state")
}

func TestOpenFailuresAndCraftedRuns(t *testing.T) {
	h := newHarness(t)
	// every run-bound command refuses on an empty repository
	for _, args := range [][]string{{"record", "tokens", "--data", "{}"}, {"gate", "findings_nonempty", "--run", "mrv-nothing-000001"}, {"judge", "--kind", "match", "--model", "gpt-5.2", "--effort", "low", "--input", h.file("m.json", `{"golden":{"comment":"g"},"candidate":{"issue_text":"x"}}`), "--run", "mrv-nothing-000001"}, {"converge", "--check", h.file("p.yaml", "no_fixation_progress"), "--run", "mrv-nothing-000001"}, {"export"}, {"advance", "--run", "mrv-nothing-000001", "--from", "fix"}, {"advance"}} {
		if _, code := h.run(args...); code != 2 {
			t.Fatalf("%v must refuse with exit 2: %s", args, h.out.String())
		}
	}
	h.mustErr(CodeUsage, 2, "converge", "--check", "/nope/missing.yaml")
	// outside a repository: init and diff refuse; cwd inside .git has no toplevel
	h.cwd = t.TempDir()
	h.mustErr(CodeNotARepo, 2, "init", "--workflow", "sdlc-loop")
	h.mustErr(CodeNotARepo, 2, "diff", "--a", "mrv-nothing-000001", "--b", "mrv-nothing-000001")
	h.cwd = filepath.Join(h.root, ".git")
	h.mustErr(CodeNotARepo, 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")
	h.cwd = h.root
	// a bare main worktree
	bare, _ := filepath.EvalSymlinks(t.TempDir())
	git(t, bare, "init", "-q", "--bare")
	h.cwd = bare
	if e := h.mustErr(CodeNotARepo, 2, "state"); e["error"].(map[string]any)["fields"].(map[string]any)["reason"] != "bare" {
		t.Fatalf("bare: %v", e)
	}
	h.cwd = h.root
	// bad judge URL at init on a product run
	h.env[EnvOpenAIKey], h.env[EnvOpenAIURL] = "k", "not-a-url"
	h.mustErr("ERR_JUDGE_URL", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	h.env = map[string]string{}
	// a bad embedded workflow (seam) fails `workflows`
	good := h.deps.Workflows
	h.deps.Workflows = func(name string) ([]byte, error) { return []byte("workflow: ["), nil }
	h.mustErr("ERR_WORKFLOW_INVALID", 2, "workflows")
	h.deps.Workflows = good
	// crafted run directories: peek tolerates garbage, a non-init first line, and an undecodable init; Open reports the damage
	runs := filepath.Join(h.root, ".metareview", "runs")
	for name, first := range map[string]string{"mrv-garbage-0000001": strings.Repeat("x", run.MaxLine+10), "mrv-notinit-0000001": `{"schemaVersion":1,"seq":1,"type":"tree","data":{}}`, "mrv-badinit-0000001": `{"schemaVersion":1,"seq":1,"type":"init","data":"str"}`} {
		_ = os.MkdirAll(filepath.Join(runs, name), 0o700)
		_ = os.WriteFile(filepath.Join(runs, name, "audit.jsonl"), []byte(first+"\n"), 0o600)
		if _, code := h.run("state", "--run", name); code != 1 {
			t.Fatalf("%s: exit %d %s", name, code, h.out.String())
		}
	}
	// a mock run whose init names a scenario outside the root
	env := h.must(StatusOK, 0, mockInit...)
	id := env["run_id"].(string)
	raw, _ := os.ReadFile(filepath.Join(runs, id, "audit.jsonl"))
	lines := strings.SplitN(string(raw), "\n", 2)
	lines[0] = strings.Replace(lines[0], `"mock":"mock#`, `"mock":"../evil#`, 1)
	_ = os.MkdirAll(filepath.Join(runs, "mrv-outside-000001"), 0o700)
	_ = os.WriteFile(filepath.Join(runs, "mrv-outside-000001", "audit.jsonl"), []byte(strings.Join(lines, "\n")), 0o600)
	if e := h.mustErr("ERR_MOCK_INVALID", 2, "state", "--run", "mrv-outside-000001"); e["error"].(map[string]any)["fields"].(map[string]any)["reason"] != "outside" {
		t.Fatalf("outside: %v", e)
	}
	// the scenario dir itself as the root (relInside equal branch) has no judge.yaml
	h.mustErr("ERR_MOCK_INVALID", 2, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", ".")
	// a run whose scenario disappeared after init
	_ = os.Remove(filepath.Join(h.root, "mock", "judge.yaml"))
	h.mustErr("ERR_MOCK_INVALID", 2, "advance", "--run", id)
	_ = os.WriteFile(filepath.Join(h.root, "mock", "judge.yaml"), []byte(sdlcScenario), 0o644)
	// machineDeps' MockLoad closure surfaces load errors (unreachable through the CLI: the peek loads the same file)
	c := &ctxDeps{ctx: context.Background(), deps: h.deps}
	md, _ := c.machineDeps(h.root, nil, judgeNone)
	if _, err := md.MockLoad("/nope"); err == nil {
		t.Fatal("MockLoad closure")
	}
	// a fork workflow over the cap maps to ERR_WORKFLOW_TOO_LARGE; judge on a terminal run; cancelled context
	h.mustErr(machine.CodeWorkflowTooLarge, 2, "advance", "--run", id, "--from", "discover", "--at-iter", "0", "--accept-workflow-change", "--workflow", h.file("huge.yaml", strings.Repeat("#", machine.MaxWorkflowBytes+2)))
	cand := h.file("cand.json", `{"candidate":{"issue_text":"x"}}`)
	diff := h.file("d.diff", "x")
	// a product run: judge --run with a bad base URL refuses; a call that is cancelled mid-flight appends nothing
	h.env[EnvOpenAIKey] = "k"
	env = h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium")
	pid := env["run_id"].(string)
	h.env[EnvOpenAIURL] = "not-a-url"
	h.mustErr("ERR_JUDGE_URL", 2, "judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "medium", "--input", cand, "--context", diff, "--run", pid)
	delete(h.env, EnvOpenAIURL)
	ctx, cancel := context.WithCancel(context.Background())
	h.doer.onDo = cancel
	h.out.Reset()
	code := Run(ctx, []string{"judge", "--kind", "adjudicate", "--model", "gpt-5.2", "--effort", "medium", "--input", cand, "--context", diff, "--run", pid}, strings.NewReader(""), &h.out, &h.errb, h.cwd, h.deps)
	if code != 1 || strings.Contains(h.out.String(), `"seq"`) {
		t.Fatalf("cancelled judge: %d %s", code, h.out.String())
	}
	h.doer.onDo = nil
	h.env = map[string]string{}
	// state on a run with a gate failure but no transition yet (last_error) — crafted through the store
	store := h.deps.Store(h.root)
	log, _ := store.Events(id)
	st, _ := run.FoldFull(log.Events)
	st.ChainHead = log.Head
	unlock, _ := store.Lock(id)
	ge := run.GateData{Name: "findings_nonempty", Passed: false, Error: &run.GateError{Code: "ERR_NO_FINDINGS", Gate: "findings_nonempty", Detail: "crafted"}}
	ev := run.Event{SchemaVersion: run.SchemaVersion, At: run.Time{Time: time.Now().UTC()}, Type: run.TypeGate, State: st.State, Iter: st.Iteration, Mock: true, Data: run.MarshalCanonical(ge)}
	if _, err := store.Append(id, st, ev); err != nil {
		t.Fatal(err)
	}
	unlock()
	env = h.must(StatusOK, 0, "state", "--run", id)
	if env["last_error"] == nil || env["last_error"].(map[string]any)["detail"] != "crafted" || strings.Join(strs(env["untrusted"]), ",") != "last_error.detail" {
		t.Fatalf("last_error: %v", env)
	}
	// helpers on a run without transitions, warnings or handlers, and on an unreadable store
	fresh := h.must(StatusOK, 0, mockInit...)["run_id"].(string)
	inv := &invocation{}
	if inv.lastTransitionGate(store, fresh) != nil || inv.handler(store, fresh) != nil || inv.lastWarn(store, fresh, "X") != "" {
		t.Fatal("helpers on a fresh run")
	}
	if from, _ := inv.failedFrom(store, fresh); from != "" {
		t.Fatal("failedFrom on a fresh run")
	}
	if events(&failStore{}, "x") != nil || (&invocation{}).lastWarn(&failStore{}, "x", "c") != "" {
		t.Fatal("events on a failing store")
	}
	if from, _ := (&invocation{}).failedFrom(&failStore{}, "x"); from != "" {
		t.Fatal("failedFrom")
	}
	if (&invocation{}).handler(&failStore{}, "x") != nil || (&invocation{}).lastTransitionGate(&failStore{}, "x") != nil || (&invocation{}).warnEvents(&failStore{}, "x") != nil {
		t.Fatal("helpers")
	}
}

type failStore struct{ run.RunStore }

func (failStore) Events(string) (run.Log, error) { return run.Log{}, errors.New("unreadable") }
