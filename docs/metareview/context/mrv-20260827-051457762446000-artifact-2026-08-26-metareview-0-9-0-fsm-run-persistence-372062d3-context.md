# metareview context: docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md

Run ID: `mrv-20260827-051457762446000-artifact-2026-08-26-metareview-0-9-0-fsm-run-persistence-372062d3`

## Target

- Path: `docs/specs/2026-08-26-metareview-0.9.0-fsm-run-persistence.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `bae4479`

## Artifact Excerpt

```markdown
# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** REVISION 2 (2026-08-26) — re-review pending. First of the split 0.9.0 build artifacts (parent plan:
> [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md), r3, escalated as too
> large). This spec covers **only** `internal/fsm/run`: wire types, the event log, `Fold`, the store/lock contract,
> and the trust boundary. Downstream specs (§10) build against it.
>
> **Scope rule:** this document says what `run` *stores and derives*. It does not say when the machine appends an
> event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints. Where a later spec needs
> a field or event, it is defined here; the rule that uses it belongs to that spec.
>
> **Revision 2** answers review `mrv-20260827-050354…` (8 lenses, NEEDS_REVISION). §0 maps each blocking finding to
> its change; §11 is the amendment ledger for parent-plan contracts this spec supersedes.

---

## 0. Attempt-1 resolution map

| cluster / findings | change |
|---|---|
| RunID/CreatedAt/InitialState unsourced — RC-1, RF-1, RF-2, RA-1, RA-10 | `InitData` carries `run_id`, `created_at`, `initial_state`, `initial_kind`; `Create(runID, first)`; init sets `FixEntryHead` when `initial_kind == agent-edit` (§3.1, §4.1, §5). |
| fork copy — RDM-2, RT-4, RSEC-5, RDM-6 | Copied prefix excludes the parent's `init`; `Origin{run_id, seq, hash}`; provenance invariants (§4.8); `MockTainted` derived; `At` retained (§3, §4.8). |
| outcome-last vs post-terminal events — RI-1, RA-3 | Post-terminal allow-list: `overflow_handler`, `warn`, `record`, `tokens`, `fork`, `tree` (§4.7). |
| torn tail — RSEC-2, RA-4, RDM-3, RDM-4, RF-4, RF-5, RC-2, RT-11 | Torn ≡ final line lacks `\n` only; a complete-but-undecodable line is `ERR_AUDIT_INVALID`/`ERR_AUDIT_VERSION`; `Events()` returns `Log{Events, Torn *TornTail}`; repair is a separate `RepairTail` under the lock that sidecars the bytes; the **machine** appends the `warn` with correct `Iter`/`State`; read-only commands surface `Torn` without repairing (§5.3). |
| `Prev` ownership/preimage — RF-3, RA-5, RSEC-7, RT-2 | Chain verified only in `Events()` over raw stored line bytes; `Fold` never sees `Prev`; `Data` compacted and duplicate keys rejected at append; external-oracle fixture (§3, §5.2, R2b). |
| `Status` invariant — RA-2, RI-6 | Single rule: `Unfixed` = bugs in `AllFound` **not** marked fixed by the current `Status`; unstatused bugs count as still present (fail-closed); coverage checked only when `Status` is supplied (§4.3). |
| `FixEntryHead` on fork — RI-2, RI-5 | New `fix_baseline{head}` event (§3.1, §4.6); trust-boundary example replaced (§1.5). |
| state-name leak — RS-1, RI-3, RA-9, RF-6 | `Outcome` comes only from the payload; no `"failed"` rule; `HasOutgoing` removed (§4.6). |
| lock/CAS/symlink/modes — RSEC-4, RSEC-8, RA-6, RSEC-10 | `flock`-based lock (no stale-pid logic); `Append(…, expectedPrev)` CAS; `O_NOFOLLOW` + `Lstat` on every path component; `runs/` created `0700` and fsync'd (§5.1, §5.2). |
| migration/gitignore/retention — RDM-1, RDM-5, RDM-7, RSEC-1 | `MinReadableVersion`; older-version runs readable, not appendable; self-ignoring `.metareview/runs/.gitignore`; retention "until deleted", `List()` = fold per run with `Error` rows (§5.4, §5.5). |
| caps — RC-6, RF-7, RSEC-6, RT-5, RDM-8 | All caps enforced at **decode** inside `Fold` (`ERR_AUDIT_INVALID` reason `oversize`) and at `Append` for the whole line; truncation of `Detail` is done by the machine via `run.CapDetail`; every payload field has a cap (§2.2). |
| non-loop `Iter`, `index` — RT-7, RA-8 | Enforced for every event (§4.9). |
| `delta_applied` ↔ output — RA-7 | `delta_applied.output_hash` checked against the recorded output (§4.3). |
| tests — RT-1, RT-3, RT-6, RT-8, RT-9, RT-10, RT-12, RT-13 | §8 rewritten: prefix-fold property, re-chained mutation fu
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
