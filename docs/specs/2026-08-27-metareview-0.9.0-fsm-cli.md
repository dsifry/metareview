# metareview 0.9.0 — spec 5: CLI, black-box tests, docs, and milestones

> **Status:** DRAFT r1 (2026-08-27). Fifth of the five split artifacts (ownership ledger: run spec §12 row 5). Owns plan
> r3 §3 (CLI envelope, exit codes, every subcommand), §4.1 black-box suite, §5 milestones, the M8 documentation set,
> `package.json` files, the forbidden-phrase grep, and CI. Everything here is a thin shell over `machine` (specs 2–3),
> `kind`/`judge`/`cmdexec`/`mockai` (spec 4), `run`, and `workflows`.

---

## 1. Package
`internal/fsm/cli` — `func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, deps Deps) int`. `cmd/metareview/main.go`
gains one branch: `fsm` → `fsmcli.Run(args[1:], os.Stdin, os.Stdout, os.Stderr, fsmcli.RealDeps(cwd, env))`. `Deps` bundles
`machine.Deps` + `Env func(string) string` + `Now`. All parsing is hand-rolled like the rest of `main.go` (no flag lib).

## 2. Commands (plan §3.1, final)
```
metareview fsm init      --workflow <name|path> [--var K=V]... [--base <ref>] [--goldens <file>] [--repo-mode advisory|enforcing]
                         [--allow-custom-cmds <sha256>] [--calibration] [--mock-ai <dir>] [--work-dir <dir>]
metareview fsm state     [--run <id>]
metareview fsm advance   [--run <id>] [--repair]
metareview fsm advance   --run <id> --from <state> [--at-iter N] [--var K=V]... [--work-dir <dir>] [--accept-workflow-change] [--allow-custom-cmds <sha256>]
metareview fsm record    node-output --node <n> --data <file|-> [--replace] [--run <id>]
metareview fsm record    tokens --data '<json>' [--run <id>]
metareview fsm record    <event> --data '<json>' [--run <id>]
metareview fsm gate      <name> [--run <id>] [--input <snapshot.json>]
metareview fsm judge     --kind <match|adjudicate|still-present> --model <m> --effort <e> --input <file> [--context <file>] [--run <id>] [--no-fence]
metareview fsm converge  [--run <id>] [--check <yaml>]
metareview fsm diff      --run <a> --run <b>
metareview fsm export    --run <id> [--out <dir>] [--include-vars] [--max-bytes N]
metareview fsm workflows
metareview fsm --agent-prompt
```
Rules: `--workflow <name>` resolves embedded only; `/` or `.yaml` → path. `--run` defaults to the newest run (`List()[0]`);
none → `ERR_NO_RUNS`. `MOCK_AI=<dir>` ≡ `--mock-ai`. `--no-fence` is accepted only with a calibration run (`ERR_FENCE_REQUIRED`).
`fsm judge --run <id>` appends the `llm_call` to that run under the lock (with `Node: "judge"`); without `--run` nothing is
persisted. `fsm converge --check` refuses `cmd` atoms unless the run consented to that exact argv (`ERR_CMDS_NOT_ALLOWED`).

## 3. Output and exit codes
One JSON object on stdout, always: `{"ok", "run_id", "state", "iteration", "status", "outcome", "mock", "warnings":[…], …}`;
errors add `"code"` and `"detail"`; human hints go to stderr. Statuses: `ADVANCED`, `NEEDS_INPUT`, `DONE`, `STOPPED`,
`GATE_FAILED`, `FORKED`, `OK`. Exit: **0** success (`ADVANCED`, `DONE` with `fixed|clean`, `OK`); **1** `DONE(reviewed)`,
`STOPPED`, `GATE_FAILED`, any `ERR_*` after init; **2** usage and init-time refusals (`ERR_JUDGE_UNSET`, `ERR_CMDS_NOT_ALLOWED`
— stdout carries `cmds` list + `cmds_sha256`, `ERR_WORKFLOW_NOT_FOUND`, unknown option); **3** `NEEDS_INPUT`.
`GATE_FAILED` includes `gate:{name,passed:false,code,detail}` and `resume_hint: "metareview fsm advance --run <id> --from <state>"`.
`NEEDS_INPUT` payload = plan §3.4 (`node`, `instructions`, `input`, `untrusted`, `output_schema`, `record`), with the instructions
rendered through the fenced envelope of spec 4 §3.2 (untrusted values JSON-encoded inside nonce fences).

## 4. `--agent-prompt`
Emits the tool-usage blurb for `--append-system-prompt`/`AGENTS.md`: the loop skeleton (`advance` → on 3 do the node's
work → `record node-output` → `advance`; on 1 read `code`/`resume_hint`), every subcommand with one line each, the
untrusted-data rule, and the wording rule (structure is deterministic; results are not). Test-fsm greps it.

## 5. Black-box suite — `tests/go/test-fsm.sh` (wired into `tests/run-all.sh`)
Temp git repo + `--mock-ai testdata/fsm/scenarios/...`: full `sdlc-loop` (init → 3 NEEDS_INPUT/record cycles per iteration,
2 iterations, `DONE fixed`, exit 0), `review-loop` (`clean` exit 0; `reviewed` exit 1), `GATE_FAILED` exit 1 + `resume_hint` +
fork recovery, `STOPPED` exit 1, usage exit 2 (`ERR_JUDGE_UNSET`, bad option), `ERR_CMDS_NOT_ALLOWED` prints list + sha then
succeeds with it, `--run` default + `ERR_NO_RUNS`, `record` on a terminal run, `ERR_NODE_OUTPUT_INVALID` leaves `audit.jsonl`
byte-identical, `MOCK_AI` env, `fsm gate --input`, `fsm converge`, `fsm diff` parent/child, `fsm export` redaction markers,
`fsm workflows` lists both, `--agent-prompt` names every subcommand, torn tail → `ERR_AUDIT_TORN` → `--repair`, forbidden
phrase ("deterministic results") absent from skill/command/docs/`--agent-prompt`; `metareview --help` lists `fsm`;
`metareview status` shows the FSM runs section.

## 6. Docs (M8)
`skills/fsm/SKILL.md` + `commands/fsm.md` (the loop skeleton, exit codes, untrusted-data rule, judge-swap recipe incl. the
`--at-iter 0` limitation on sdlc-loop, trust boundary, enforcing caveat, local-FS-only, `MaxEvents`, retention, `--repair`);
`docs/fsm/driving-a-workflow.md`, `docs/fsm/sdlc-loop-example.md` (a worked `claude -p` session); README/quickstart/INSTALL:
`.metareview/runs/` transient (the directory self-ignores) + `docs/metareview/fsm/` durable + `gitpolicy.go` whitelist line;
AGENTS.md/CLAUDE.md exit handling amended ("3 = the FSM needs the host to do a node's work; 1 with a JSON `code` = follow
`resume_hint`"); design-spec amendments (§10.1/§14.3/§17 judge-swap claim; `all_fixed` atom; `commit_exists` base);
`docs/README.codex.md`, `docs/README.claude.md`, `docs/index.html`; CHANGELOG 0.9.0; `package.json` files += `workflows/`,
`docs/fsm/`, `go.sum`; plugin manifests advertise "workflow"; `tests/manifest/test-skills.sh` += the new files. Version
bump to 0.9.0 last, after `npm test` passes.

## 7. Milestones (remaining)
M1 workflow+gate+converge (spec 2 §2–4) ∥ M4 cmdexec+judge+mockai (spec 4 §2,3,5) → M5 kinds (spec 4 §4) → M6 machine core
(spec 2 §5) → M7 fork/diff/export/record (spec 3) → M8 CLI+docs (this) → M9 smoke + release. Each milestone: TDD, 100% gate,
`review task-done` per commit range (≤ 120 KB), commit.

## 8. Tests for `cli` (100%): table-driven `Run` with fake deps for every subcommand's parse paths, envelope fields, exit codes, and
error mapping; `--agent-prompt` content; `RealDeps` wiring (env → keys, cwd → root via `git rev-parse --show-toplevel`).
