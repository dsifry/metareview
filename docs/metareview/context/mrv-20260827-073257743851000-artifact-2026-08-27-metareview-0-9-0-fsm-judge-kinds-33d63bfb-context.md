# metareview context: docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md

Run ID: `mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `75eefe2`

## Artifact Excerpt

````markdown
# metareview 0.9.0 — spec 4: guardrails, judge, kinds, and mock AI

> **Status:** r4 — BUILD BASELINE after ESCALATION (2026-08-27). Attempt 3 (`mrv-20260827-070456908813000-…`) ended NEEDS_REVISION on 4/8 lenses with every blocker mechanical; the chain is ESCALATED and a human must accept this r4 (applied provisionally per the run-spec precedent). r3 note: Fourth of the five split artifacts (ownership ledger: run spec §12). Owns plan
> r3 §1.8 items 2–5, §2 (judge port), kinds/`Executor`/`Delta` producers, match-then-adjudicate composition +
> `Bug.Verdict` vocabulary, `index` assignment, the `llm_call`/`cmd_call` producer contract, mock scenarios, and the
> pinned harnesseval provenance of the prompts. Implements spec 2 r3's `machine.NodeKind`/`Executor`/`Registry`,
> `converge.Runner`, and `workflow.KindInfo`.
>
> **r3 changes** (review `mrv-20260827-064453475472000-…`, attempt 2): `Spec.Name` replaces the `MRV_CMD_NAME` env
> entry; not-allowed is refused without audit; Anthropic request bodies are the reference's (`thinking` mapping) in
> calibration mode and `output_config.effort` (GA, no beta header) only on models that support it; full effort table;
> retry on status/`error.type` only (never a 200 body); redirect terminal; greedy rule = Python's (no "taken" skip,
> superseded candidates neither confirmed nor adjudicated — the reference's bookkeeping); matched bugs carry the
> golden's text; executors self-validate and cap at `MaxPayload − 128`; `StartIndex`; `kind.New` constructor +
> `Registry.Mock()`; `judge.Script` (no `judge`↔`mockai` cycle); vendored Python literals for an unconditional
> provenance test; token clamps; bounded reads; scenario strict decode + file-bytes hash + typed `parsed`; every
> test row names its discriminating fixture.
>
> **r4 changes (attempt 3):** explicit Anthropic effort-capable model list (no globs) and a Go-owned product thinking
> table (the reference has no `high`); default base URLs; `Bug` field population (`Confidence`, `File`, `Line`); goldens
> capped at `MaxDesc` at init (run cap) so matched `Desc` always fits; `Rejected` shape; still-present `*bool`; `Input =
> prompt_tokens − cached_tokens`; every counter clamped ≥ 0 at the provider boundary; 529/`overloaded_error` precedence;
> body-read/ctx-cancel/URL classification; trailing slash; `.gitattributes -text` for prompt fixtures; `cmdexec`
> constructors; `Registry.Executor` for host kinds; `Verdict` fields on error; `diff_truncated` = the 30000-byte cut;
> `lenses` default 8; non-empty `issue_text`; mock cmd rows keyed by durable ordinal; `cmd` kind `Reduce` cliff check;
> `JUDGE` model id ≤ `MaxShort`; typed `Verdict` constants; duplicate golden comments refused; single Guarded factory;
> test rows for unknown-fields-ignored, calibration at the `Judge.Call` level, full-stdout decode, literal bodies.
>
> **Port spec:** `~/Developer/harnesseval/harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router,effort}.py` @
> `19ff9a8`. Slot sources: `match` `golden_comment = Golden.Comment`, `candidate = Finding.IssueText`
> (`sdlc_loop.py:264`); `adjudicate` `candidate = Finding.IssueText`; `still-present` `golden_comment = Bug.Desc`, where
> `Desc` is the golden's comment for matched bugs and the candidate text otherwise (`sdlc_loop.py:353 _desc`). `match`
> runs at `max_tokens=1024` in the loop (`sdlc_loop.py:274`).

---

## 1. Packages
```
internal/fsm/cmdexec   Runner (exec-backed + fake); Guarded (allow-list, pinned argv, hash re-verify, timeout, typed decode, audit)
internal/fsm/judge     Judge iface; Script; prompts + fencing; parsers; providers; retry; tokens; MockJudge; NewHTTPClient
internal/fsm/kind      NodeKind/Executor implementations; Registry (kind.New)
internal/fsm/mockai    scenario files → judge.Script + cmdexec fake rows; content hash
```
`errs`, `run` ← all. `converge` ← `cmdexec` (returns `converge.CmdResult`; payload = `converge.Payload`). `workflow`,
`machine` (interfaces), `judg
````

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
