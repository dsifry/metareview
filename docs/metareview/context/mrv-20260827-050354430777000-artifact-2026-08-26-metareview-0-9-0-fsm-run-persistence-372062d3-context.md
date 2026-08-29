# metareview context: docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md

Run ID: `mrv-20260827-050354430777000-artifact-2026-08-26-metareview-0-9-0-fsm-run-persistence-372062d3`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `520d7c6`

## Artifact Excerpt

````markdown
# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** DRAFT for review — the first of the split 0.9.0 build artifacts. Parent plan:
> [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md) (revision 3,
> escalated on 2026-08-26 as too large to converge in one document). This spec covers **only** the `run`
> package: wire types, the event log, the fold that derives a snapshot, the store/lock contract, and the
> trust boundary. It is the contract that `workflow`, `gate`, `converge`, `judge`, `kind`, `machine`, and
> `cli` will build against; those get their own specs after this one is locked.
>
> **Scope rule:** this document says what `run` *stores and derives*. It does not say when the machine
> appends an event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints.
> Where a later spec needs a field, the field is defined here; where a rule belongs to the machine, this
> document names the invariant `run` enforces and nothing more.
>
> **Resolves (from review `mrv-20260827-014544…`):** ARC3-1/2/3/4/6/10, DM3-2/4, CMP3-2/4/5/6, SEC-21/22
> (run part), FIN-1/2 (run part)/6/7, TQ3-1/9.

---

## 1. Principles

1. **`audit.jsonl` is the only authority.** There is no `state.json`. Every command derives the snapshot
   by folding the log. Read-only commands write nothing.
2. **`Fold` is pure and total over `run` types.** It takes events and returns a snapshot or an error. It
   imports no other `fsm` package. Anything the machine "sets" must be carried by an event payload or be
   derivable from earlier events, or it does not exist.
3. **Append-only, hash-chained, versioned.** Events are never rewritten. Each event carries the hash of the
   previous line. A version or type the reader does not know is an error, never a skip.
4. **Fail closed.** A log that violates an invariant folds to `ERR_AUDIT_INVALID`; the run is unusable
   until an operator inspects it. No "best effort" snapshots.
5. **Trust boundary (stated, not solved).** The run directory is `0700` and files are `0600`, and the hash
   chain detects accidental corruption and casual edits. It does **not** defend against the same-user
   process that runs the FSM — the host agent can rewrite the whole chain. The FSM's guarantees are
   *process* guarantees for a cooperating agent: "you cannot transition fix→verify without a commit" holds
   for an agent that drives the FSM through the CLI, not for one that edits `.metareview/`. This is the
   same class of limit as spec §1 ("does not guarantee the adjudication was correct") and is documented in
   the user-facing docs with the same prominence as the advisory/enforcing caveat.

---

## 2. Wire types

All persisted types carry snake_case JSON tags. `json:"…,omitempty"` only where marked. Sizes are enforced
at append time (`ERR_EVENT_TOO_LARGE`), never silently truncated, except `GateError.Detail`, which is
truncated to 64 KB with `detail_truncated: true`.

```go
package run

const SchemaVersion = 1

type State string       // any string the workflow declares; run treats "done" and "failed" as terminal only via HasOutgoing (§4.7)
type Outcome string     // fixed | clean | reviewed | stalled | overflow | custom | failed
type ExecMode string    // inline | subagent | fork

type Finding struct {
    IssueText string `json:"issue_text"`          // ≤ 4 KB
    File      string `json:"file,omitempty"`
    Line      int    `json:"line,omitempty"`
    Severity  string `json:"severity,omitempty"`
    Category  string `json:"category,omitempty"`
    Source    string `json:"source,omitempty"`
}
type Golden struct { Comment string `json:"comment"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"` }
type Bug struct {
    ID         string  `json:"id"`                  // BugID(issue_text) = hex(sha1(issue_text))[:12]
    Desc       string  `json:"desc"`                // ≤ 2 KB
    File       
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
