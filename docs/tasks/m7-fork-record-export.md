# M7 — fork/resume, diff, export, runs.jsonl record

Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).

Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
`go vet` is clean.
