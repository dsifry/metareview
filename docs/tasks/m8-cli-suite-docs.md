# M8 — CLI, judge/mockai/converge handoffs, black-box suite, docs

Implements spec 4 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-cli.md`) plus the spec 2/5 handoffs:
`judge.Preflight`, `mockai.MaxFileBytes`, `converge.Describe`; `machine` `OpenOptions`, `Deps.Preflight`, `NodeView`,
`View.Outgoing`, `RecordLLMCall`, `Init` workflow-source stamps; `internal/fsm/cli` (`Deps` seams, `RealDeps`, `Run`,
envelopes, `exitFor`, `StatusLines`, `AgentPrompt`) wired into `cmd/metareview` (`fsm` branch, status section);
`tests/go/test-fsm.sh` over the mock scenarios under `testdata/fsm/scenarios`; `/fsm` skill, `commands/fsm.md`,
`docs/fsm/`, README/INSTALL/quickstart/AGENTS/CLAUDE/CHANGELOG/manifest amendments.

Done when every `internal/fsm/*` package and `workflows/` is at exactly 100% statement coverage and the legacy
packages hold their recorded floor (`tests/coverage.sh`), `tests/run-all.sh` is green, and `go vet` is clean.
