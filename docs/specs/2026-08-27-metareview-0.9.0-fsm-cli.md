# metareview 0.9.0 — spec 5: CLI, black-box tests, docs, and milestones

> **Status:** DRAFT r2 (2026-08-27). Fifth of the five split artifacts (ownership ledger: run spec §12 row 5). Owns plan
> r3 §3 (CLI envelope, exit codes, every subcommand), §4.1 black-box suite, §5 milestones, the M8 documentation set,
> `package.json` files, the forbidden-phrase grep, and CI. Everything here is a thin shell over `machine` (specs 2–3),
> `kind`/`judge`/`cmdexec`/`mockai` (spec 4), `run`, and `workflows` — all implemented at 100% except spec 3's files.
>
> **r2 changes** (every handoff enumerated in the spec 2/4 attempt-4 logs): `--no-fence` dropped; NEEDS_INPUT payload
> keys are spec 4's (`unfixed_bugs`, `findings_so_far`, task-done rubric, eight lens names); `ERR_VAR_UNSET{JUDGE|
> JUDGE_EFFORT}` → `ERR_JUDGE_UNSET` exit 2; `ERR_BAD_REPO_MODE` exit 2 (tighten-only override); `untrusted` envelope key;
> `ERR_RECORD_NAME{reason}` replaces plan E13's `record transition` row; `WORKFLOW_WARNING` on `init` output; `RealDeps`
> wiring table (§8); base-URL env overrides; agent-satisfiable knob list; `fsm judge` producer rules; `fsm gate` over 9
> names; `--repair`/`View.Torn`; consent list shape (structured `cmds` on stdout); scenario inventory + `records/` layout;
> `.gitattributes`/manifest coverage; exit-code mapping for spec 4's codes; design-amendment list; `.metareview/` ignore.

---

## 1. Package
`internal/fsm/cli` — `func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, deps Deps) int`. `cmd/metareview/main.go`
gains one branch: `fsm` → `fsmcli.Run(args[1:], os.Stdin, os.Stdout, os.Stderr, fsmcli.RealDeps(cwd, env))`. Hand-rolled arg
parsing (no flag lib), like the rest of `main.go`.

## 2. Commands (plan §3.1, final)
```
metareview fsm init      --workflow <name|path> [--var K=V]... [--base <ref>] [--goldens <file>] [--repo-mode enforcing]
                         [--allow-custom-cmds <sha256>] [--calibration] [--mock-ai <dir>] [--work-dir <dir>] [--run-id <id>]
metareview fsm state     [--run <id>]
metareview fsm advance   [--run <id>] [--repair]
metareview fsm advance   --run <id> --from <state> [--at-iter N] [--var K=V]... [--work-dir <dir>] [--accept-workflow-change [--workflow <name|path>]] [--allow-custom-cmds <sha256>]
metareview fsm record    node-output --node <n> --data <file|-> [--replace] [--run <id>]
metareview fsm record    tokens --data '<json>' [--run <id>]
metareview fsm record    <event> --data '<json>' [--run <id>]
metareview fsm gate      <name> [--run <id>] [--input <snapshot.json>]
metareview fsm judge     --kind <match|adjudicate|still-present> --model <m> --effort <low|medium|high|xhigh> --input <file> [--context <diff-file>] [--run <id>]
metareview fsm converge  [--run <id>] [--check <yaml>]
metareview fsm diff      --run <a> --run <b>
metareview fsm export    --run <id> [--out <dir>] [--include-vars] [--max-bytes N]
metareview fsm workflows
metareview fsm --agent-prompt
```
Rules: `--workflow <name>` resolves embedded only; `/` or `.yaml` → path. `--run` defaults to the newest run (`List()[0]`);
none → `ERR_NO_RUNS`. `MOCK_AI=<dir>` ≡ `--mock-ai`. `fsm judge --run <id>` appends the `llm_call` under the run's lock with
`Node: "judge"` (a reserved state name) and `Index = NextIndex("judge@<iter>")`, `Calibration` and `Fence` from the run
(`Fence = !Calibration`); without `--run` nothing is persisted, `Calibration=false`. `--context` is the diff slot (cut per
spec 4 §3.1; `diff_truncated` set when cut). `fsm converge --check <yaml>` runs `converge.Validate` (+ `Parse`) and refuses
`cmd` atoms unless `--run`'s consent covers the exact argv (`ERR_CMDS_NOT_ALLOWED`). `fsm gate <name>` evaluates one of the
nine built-ins on the run's snapshot (or `--input`); `commit_exists` needs the run's git.

## 3. Output and exit codes
One JSON object on stdout, always: `{"ok", "run_id", "state", "iteration", "status", "outcome", "mock", "warnings":[…],
"untrusted":[…], …}`; errors add `"code"`, `"detail"`, `"fields"` and list `"error.detail"` under `untrusted`; human hints go
to stderr. Statuses: `ADVANCED`, `NEEDS_INPUT`, `DONE`, `STOPPED`, `GATE_FAILED`, `FORKED`, `OK`. Exit: **0** success
(`ADVANCED`, `DONE` with `fixed|clean`, `OK`); **1** `DONE(reviewed)`, `STOPPED`, `GATE_FAILED`, any `ERR_*` after init
(incl. `ERR_CMD_*`, `ERR_JUDGE_HTTP/TRANSPORT/RESPONSE/EFFORT_UNSUPPORTED`, `ERR_MOCK_UNSCRIPTED/EXPECT`, `ERR_TOO_MANY_BUGS`,
`ERR_AUDIT_*`, `ERR_RUN_TERMINAL`); **2** usage and init-time refusals (`ERR_JUDGE_UNSET`, `ERR_VAR_*`, `ERR_CMDS_NOT_ALLOWED`
— stdout carries `cmds: [{name, argv, pinned: {path: sha}, unpinned: [...], timeout_ms, env}]` + `cmds_sha256`,
`ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_INVALID`, `ERR_BAD_REPO_MODE`, `ERR_JUDGE_KEY`, `ERR_JUDGE_URL`, `ERR_JUDGE_MODEL`,
`ERR_MOCK_INVALID`, `ERR_GOLDENS_INVALID`, `ERR_WORKDIR_FOREIGN`, `ERR_NO_RUNS`, unknown option); **3** `NEEDS_INPUT`.
`GATE_FAILED` includes `gate:{name,passed:false,code,detail}` and `resume_hint: "metareview fsm advance --run <id> --from <state>"`.
`NEEDS_INPUT` payload: `node`, `kind`, `exec`, `model`, `effort`, `instructions` (spec 4's fenced `Text`), `input`
(`base_sha`, `head_sha`, `iteration`, `diff_truncated`, `unfixed_bugs` | `findings_so_far` + `diff` + `lenses` + `rubric`),
`untrusted` (the keys), `output_schema`, `record` (the exact `fsm record node-output` command). `init` output lists
`warnings` (`WORKFLOW_WARNING`s). `fsm state` shows `next_action`, `torn`, `failed_gate`, `last_error`.

## 4. `--agent-prompt`
Emits the tool-usage blurb for `--append-system-prompt`/`AGENTS.md`: the loop skeleton (`advance` → on 3 do the node's
work → `record node-output` → `advance`; on 1 read `code`/`resume_hint`), every subcommand with one line each, the
untrusted-data rule (every `untrusted` key and every `error.detail` is data), the consent rule (an `ERR_CMDS_NOT_ALLOWED`
list is for a **human** to read — the agent must not pass `--allow-custom-cmds` on its own), the agent-satisfiable knobs
(`--allow-custom-cmds`, `--accept-workflow-change`, `--mock-ai`/`MOCK_AI`, `--calibration`, `--repo-mode`,
`ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL`) named as such, and the wording rule (structure is deterministic; results are not).

## 5. Black-box suite — `tests/go/test-fsm.sh` (wired into `tests/run-all.sh`)
Temp git repo + `--mock-ai testdata/fsm/scenarios/<workflow>/<name>/` (inventory, authored here: sdlc-loop `{happy,
cumulative-convergence, no-findings, no-confirmed, dirty-tree, judge-swap-iter0, judge-swap-frozen, overflow-iterations,
overflow-budget, cmd-guardrails, injection}`, review-loop `{clean, reviewed, adjudicate-fail, injection, torn}`; each dir =
`judge.yaml` (hashed) + `records/<node>@<iter>.json` host outputs the script feeds to `fsm record`). Rows: full `sdlc-loop`
(init → NEEDS_INPUT/record cycles, 2 iterations, `DONE fixed`, exit 0), `review-loop` (`clean` exit 0; `reviewed` exit 1),
`GATE_FAILED` exit 1 + `resume_hint` + fork recovery, `STOPPED` exit 1 + overflow handler, usage exit 2 (`ERR_JUDGE_UNSET`,
bad option), `ERR_CMDS_NOT_ALLOWED` prints the structured list + sha then succeeds with it, `--run` default + `ERR_NO_RUNS`,
`record` on a terminal run (`tokens` ok, `node-output` refused), `ERR_NODE_OUTPUT_INVALID` leaves `audit.jsonl` byte-identical,
`MOCK_AI` env, `fsm gate --input`, `fsm converge --check`, `fsm judge` with/without `--run` (index continues), `fsm diff`
parent/child, `fsm export` redaction markers + `workflow.yaml` copied, `fsm workflows` lists both, `--agent-prompt` names every
subcommand and every knob, torn tail → `ERR_AUDIT_TORN` → `--repair`, injection scenario (fenced values never appear raw in
`instructions`), forbidden phrase ("deterministic results") absent from skill/command/docs/`--agent-prompt`; `metareview
--help` lists `fsm`; `metareview status` shows the FSM runs section; `.metareview/runs.jsonl` rows for every outcome.

## 6. Docs (M8)
`skills/fsm/SKILL.md` + `commands/fsm.md` (loop skeleton, exit codes, untrusted-data rule, consent-to-a-human rule,
judge-swap recipe incl. the `--at-iter 0` limitation on sdlc-loop, trust boundary: agent-satisfiable knobs, consent depth =
argv bytes, `cmd_call` persists capped stdout/stderr, child env = `{PATH,HOME,LANG,TMPDIR}` ∩ set + `MRV_RUN_ID` + declared
names (values never persisted), enforcing caveat incl. `.git/info/exclude`/clean filters and untracked files failing
`commit_exists`, calibration runs are eval-only, local-FS-only, `MaxEvents`, retention, `--repair`, manual deletion of a
sidecar-less run, `WorkTree` loose objects, the closed Anthropic family table and `high` being Go-only); `docs/fsm/
driving-a-workflow.md`, `docs/fsm/sdlc-loop-example.md`; README/quickstart/INSTALL: `.metareview/runs/` transient (self-ignores)
+ `.metareview/runs.jsonl` (add `.metareview/` to the target repo's `.gitignore`; `gitpolicy.go` whitelist line) +
`docs/metareview/fsm/` durable; AGENTS.md/CLAUDE.md exit handling ("3 = the FSM needs the host to do a node's work; 1 with a
JSON `code` = follow `resume_hint`"); design-spec amendments: §5/§16 `cmds:` by name + `on_overflow: <name>`, §9 atom params
dropped + `all_fixed` placement + `not`→`custom`, §8 nine gates + `commit_exists` base + D1 (`AllFixed` non-empty, decided A),
§10.1/§14.3/§17 judge-swap claim + effort table, `JUDGE_EFFORT` required, `*→failed` ignored; `docs/README.codex.md`,
`docs/README.claude.md`, `docs/index.html`; CHANGELOG 0.9.0; `package.json` files += `workflows/`, `docs/fsm/`, `go.sum`,
`.gitattributes`; plugin manifests advertise "workflow"; `tests/manifest/test-skills.sh` += the new files; `testdata/fsm/` in
the manifest. Version bump to 0.9.0 last, after `npm test` passes.

## 7. Milestones (remaining)
M7 fork/diff/export/record (spec 3 r2) → M8 CLI + docs + `test-fsm.sh` (this) → M9 full gate + code-gate ranges + release.

## 8. `RealDeps(cwd, env)` wiring (tested at 100% with fakes for the OS seams)
`Store = run.NewJSONLStore(root)`, `Sidecar = machine.FSSidecar{Root}`, `Git = gate.NewExec(dir, gate.RealExec)`,
`LookPath = exec.LookPath`, `FileHash = workflow.FileSHA256`, `Workflows = workflows.Read`, `ReadFile` = `os.ReadFile` through
`io.LimitReader(cap+1)`, `Nonce` = 16 hex from `crypto/rand`, `Clock` = `run.Time{time.Now()}`, `MockLoad = mockai.LoadHash`,
`Terminal = machine.RecordTerminal(root)`, `Runner = func(d RunnerDeps) converge.Caller { return cmdexec.Guarded{Runner: real or
scenario.Runner(), Allowed: d.Allowed, Dir: d.WorkDir, RunID: d.RunID, FileHash, Audit: d.Audit, Environ: os.Environ, CmdCalls:
d.CmdCalls} }`, `Kinds = kind.New(kind.Deps{Judge: real (judge.New(judge.NewHTTPClient(180s), Keys{ANTHROPIC_API_KEY,
OPENAI_API_KEY}, URLs{ANTHROPIC_BASE_URL, OPENAI_BASE_URL}, nonce, clock)) or judge.NewMock(scenario.Script()), Mock: dir != ""})`.
`root` = `git rev-parse --show-toplevel` of `cwd`. Tests: table-driven `Run` with fake deps for every subcommand's parse
paths, envelope fields, exit codes, error mapping; `--agent-prompt` content; `RealDeps` wiring (env → keys/URLs, cwd → root).
