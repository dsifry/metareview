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
