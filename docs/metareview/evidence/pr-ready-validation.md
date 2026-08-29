# PR-ready validation evidence

Repo: metareview  Branch: fsm-enhancements  Base: main
Commit: 1dd0730abe8002b6a28ac09a7eedb32d5e0a36b9
Toolchain: go1.27.0 (pinned in go.mod; the gate exports GOTOOLCHAIN from it)

## go vet ./...
exit=0 (no output above means clean)

## golangci-lint run
0 issues.

## shellcheck (all shell scripts)
shellcheck exit=0

## npm test (behavioral suite + coverage gate)
internal/reviewlog                                  91.1%  ok
internal/reviewmanifest                             97.0%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   92.5%  ok
internal/sessionhistory                             86.3%  ok
internal/setup                                      88.5%  ok
internal/shardpack                                 100.0%  ok
internal/state                                      86.2%  ok
internal/taskdone                                   89.4%  ok
internal/tasksource                                100.0%  ok
workflows                                          100.0%  ok
coverage gate passed

## Chain provenance

Two chains have been reset on this target, each after exhausting its attempts
while shard review was still converging. Both resets are human decisions, taken
deliberately and recorded here rather than left to be inferred from a gap in the
run ids.

  - chain 1 escalated 2026-08-28 06:30 at attempt 6/6
  - chain 2 escalated 2026-08-28 15:49 at attempt 3/3

Neither escalation meant the target was unreviewable. Both meant the branch was
still moving: each round of fixes re-cuts the shard plan, so a chain can run out
of attempts while the evidence it needs is still being produced. Five rounds of
sharded review have now run, covering every shard of every plan, and thirty
blocking findings have been fixed — the last ten of them defects in the fixes
themselves, which is the reason the rounds continued rather than a reason to
doubt them.

Round-five state: 51 shards plus cross-shard, all reviewed against plan
014639e60cb3cab9. Results are committed under
docs/metareview/shards/pr-ready/fsm-enhancements-b73f409f/.
