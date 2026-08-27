# metareview context: docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md

Run ID: `mrv-20260827-052714785199000-artifact-2026-08-26-metareview-0-9-0-fsm-run-persistence-372062d3`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `6f7cf67`

## Artifact Excerpt

```markdown
# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** REVISION 3 (2026-08-26) — re-review pending (attempt 3 of the chain). First of the five split
> 0.9.0 build artifacts (parent plan: [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md),
> r3, escalated as too large; Dave chose "split the target" on 2026-08-26). This spec covers **only** `internal/fsm/run`.
>
> **Scope rule:** this document says what `run` *stores and derives*, and what a writer must produce. It does not say
> when the machine appends an event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints.
>
> **Revision 3** answers review `mrv-20260827-051457…` (attempt 2). §0 maps each blocking finding to its change.
> §11 is the amendment ledger; §12 the ownership ledger.

---

## 0. Attempt-2 resolution map

| cluster / findings | change |
|---|---|
| validate-before-write, `Apply`, fold state — RA2-1, RA2-3, RF2-2, RF2-4, RT2-4, RT2-5 | `FoldState` (§4) carries the accumulators `Snapshot` lacks; `Apply(FoldState, Event) (FoldState, error)` declared; `Fold = reduce(Apply)`; `Append(runID, st FoldState, ev)` runs `Apply` **before** writing and refuses on any fold error (`ERR_APPEND_REJECTED`), returns the new head hash; `Snapshot.Seq` rule stated; no-op rows compare `Snapshot` minus `Seq` (§4, §5.2, §8). |
| HTML escaping / canonical bytes — RF2-1, RSEC2-9, RSEC2-5 | `Canonical(raw)` (validate, reject duplicate keys at every depth, compact, **`SetEscapeHTML(false)`**) is the only serialization path; `OutputHash = sha256(Canonical(output))`; stored bytes are exactly the canonical bytes; caps measure canonical bytes (§2.3, §5.2). |
| fork provenance — RA2-2, RDM2-4, RC2-2, RSEC2-3 | `ForkedAtSeq` = parent seq of the last copied event; copied block is exactly child seqs `2..ForkedAtSeq` with `Origin.Seq == child.Seq` (contiguous from parent 2) (§4.8). |
| `fix_baseline` fail-closed — RI2-1 | `FixEntryHead` is never set from an `Origin`-bearing `transition`/`init`; `fix_baseline.Head` must equal `Snapshot.Head` and `StateKind == agent-edit` (§4.6). |
| `Status` coverage — RI2-2, RC2-3(ii), RS2-1 | Restored: when `Delta.Status` is supplied it must cover `AllFound` exactly once (`status_incomplete`); `Unfixed` stays fail-closed between deltas (§4.3). |
| store errors, `Seq`/`SchemaVersion`, `init` base case, `NodesRun` — RC2-1, RC2-4, RC2-8, RF2-5, RA2-6, RA2-9 | `StoreError{Code, Seq, Detail}` with enumerated codes (§2.4); `Snapshot.Seq`/`SchemaVersion` rules (§4.1, §4.10); `init` stamp base case (§4.9); `NodesRun` holds node **names** (§2.1). |
| versioning — RF2-3, RDM2-1, RDM2-2, RI2-4 | v1 = exact match; the v2 contract is `Migrate` writing `audit.v2.jsonl` beside the original (never in place; originals and children's `Origin.Hash` untouched); no unreachable branch at v1 (§5.4). |
| torn tail — RSEC2-1, RSEC2-4, RDM2-3, RA2-8, RC2-5 | `Append` refuses a torn log (`ERR_AUDIT_TORN`); `RepairTail`: `O_EXCL` sidecar `audit.torn-<seq>-<unixnano>.bin`, fsync file + dir, then truncate + fsync; a run whose durable prefix is empty is deleted by `RepairTail` (it never existed); `RunSummary.Sidecars` (§5.3). |
| time encoding — RDM2-5 | `run.Time` marshals as UTC RFC3339Nano; zero `At` is an error; copied events keep the parent's `At` (non-monotonic across the seam, stated) (§2.2). |
| caps — RT2-7, RC2-7, RDM2-6 | Every string field capped (`MaxShort` for names/codes/paths); payload caps 256 KB so canonical bytes always fit `MaxLine`; `Goldens`/`AllowedCmds` counts capped; one test row at-cap and one over per cap (§2.3, R12). |
| `MockTainted`, allow-list — RSEC2-2, RI2-3 | Producer contract: every non-`init` event of a mock run carries `Mock: true` (invariant); `cmd_call` added to the post-terminal allow-list; mirrored test rows (§4.7, §4.8, §7). |
| tests — RT2-1..12 | §8 rewritten (see rows). `Builder` uses literal stamps w
```

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
