# Spec: Go-native coverage gate (`cmd/covergate` + Makefile), retiring `tests/coverage.sh`

Status: **Design; maintainer-directed 2026-09-01. Phase 1 to build; Phase 2 incremental.**
Date: 2026-09-01
Retires: `tests/coverage.sh` (178 lines of bash gate logic). Keeps: the 37 behavioral shell tests
(`tests/run-all.sh` → `tests/go/*.sh`, `tests/manifest/*.sh`) — Phase 2 ports those incrementally.

> **Why.** metareview is a Go project driving its test gate through `bash tests/coverage.sh` and a
> `package.json` `scripts.test` — the JS convention, not the Go one (and the exact oddity that confused
> the zero-config detector, `docs/specs/2026-09-01-zero-config-setup.md`). The gate's parse+enforce logic
> is untested awk/bash. Porting it to a **tested Go program** invoked by a **Makefile** (the idiomatic Go
> entry point) makes the gate itself covered code, removes the package.json-for-Go oddity, and is the first
> brick toward "every script is Go, everything at 100%." **Risk is not danger but *silently weakening the
> gate*** — mitigated by porting test-first and running the new tool in parallel with `coverage.sh` until
> verdicts match, before deleting the script.

## 1. The 15 invariants `coverage.sh` enforces (every one must survive the port)

**Orchestration** (moves to the Makefile):
1. Pin `GOTOOLCHAIN` from go.mod's `toolchain` directive (coverage % differs by Go release → unpinned
   floors pass locally, fail on the runner).
2. Unit tests instrumented: `go test -cover -covermode=atomic ./... -args -test.gocoverdir=$COVDIR`;
   failure → exit 1, surface the log.
3. Behavioral suite instrumented: `GOFLAGS="-cover -covermode=atomic" GOCOVERDIR=$COVDIR bash
   tests/run-all.sh`; failure → exit 1, surface the log tail.
4. Merge: `go tool covdata textfmt -i=$COVDIR -o profile.txt`.
5. On exit: remove the covdir and **rebuild `bin/metareview` plainly** (the suite's `npm run build`/`go
   build` under `-cover` leaves an instrumented binary behind).

**Parse** (→ Go, pure):
6. Per-package **statement** coverage from the textfmt profile: package = directory of `<file>` relative
   to the module path; `pct = covered_stmts / total_stmts * 100`; `exact = (covered == total)`. Computed
   from statement counts, **not** parsed from `covdata percent` (whose output drops newlines for
   zero-statement packages — a documented trap).

**Enforce** (→ Go, pure):
7. `internal/fsm/*` and `workflows`: **exact 100%** (every statement). Not exact → FAIL.
8. Every other package: `pct ≥ floor` from `tests/coverage-floor.txt`. Below → FAIL.
9. A package with **no floor line** → FAIL (`no floor: add a line, or --update-floor`). New packages must
   be floored deliberately (a whole subsystem, `internal/mutation`, once shipped unmeasured this way).
10. The required set `go list ./internal/fsm/... ./workflows` must be **non-empty** unless those dirs are
    absent — a moved tree or a `go list` failure must not turn the gate into a no-op.
11. Every **required** package must be **present** in the profile (absent → FAIL) — a package with zero
    executed/zero total statements would otherwise vanish silently.
12. Every **floor-file** package must be present in the profile (vanished → FAIL).

**`--update-floor`** (→ Go):
13. Regenerate the floor = every non-`internal/fsm/*`, non-`workflows` package with its measured pct.
14. **Refuse to lower** any existing floor unless `--allow-floor-decrease` (list the lowered lines, exit 1).
15. The write **is** the remedy for rule 9, so after writing, forgive the missing-floor failures — but a
    package genuinely **below** its floor still fails, and `--update-floor` never lowers a floor anyway.

## 2. Architecture — split orchestration (make) from gate (Go)

**`cmd/covergate` is pure over its inputs** (so it reaches 100% itself, trivially, with fixture profiles):

```
covergate --profile <textfmt> --floor <file> --module <path> \
          --require-100 'internal/fsm/...,workflows' [--update-floor [--allow-floor-decrease]]
```

- It reads a **textfmt profile** (rule 6), a **floor file**, a **module path**, and the **required-100
  globs**; it needs the *list* of required packages (rule 11) and does **not** shell out — the Makefile
  passes `go list` output via `--require-list <file|->` (a seam; a test supplies a fixed list). No process
  execution in covergate ⇒ no exec seam to mock, 100% coverage is a file-and-flags exercise.
- Implements rules 6–15. Exit 0 on pass, 1 on FAIL (with the package table), 2 on usage error.
- Reuses existing primitives where they exist (the repo already parses profiles/floors nowhere yet — this
  is new, but it is plain `bufio`/`strings`, no new dependency).

**The Makefile owns orchestration** (rules 1–5, idiomatic make):

```makefile
build:  ; go build -o bin/metareview ./cmd/metareview
test cover: export GOTOOLCHAIN := $(shell awk '$$1=="toolchain"{print $$2}' go.mod)
cover:
	$(eval COVDIR := $(shell mktemp -d))
	go test -cover -covermode=atomic ./... -args -test.gocoverdir=$(COVDIR)
	GOFLAGS="-cover -covermode=atomic" GOCOVERDIR=$(COVDIR) bash tests/run-all.sh
	go tool covdata textfmt -i=$(COVDIR) -o $(COVDIR)/profile.txt
	go list ./internal/fsm/... ./workflows > $(COVDIR)/required.txt
	go run ./cmd/covergate --profile $(COVDIR)/profile.txt --floor tests/coverage-floor.txt \
	  --module $(shell go list -m) --require-list $(COVDIR)/required.txt
	@$(MAKE) build   # plain rebuild (rule 5)
test: cover
```

`package.json` drops to **distribution-only** (`bin: cli/metareview.js`), removing `scripts.test`/`build`
(or leaving them as thin `make` wrappers if npm-install flows still call them — decided in Phase 1).

## 3. Transition — prove verdict-equivalence before deleting the script

1. Land `cmd/covergate` + Makefile with covergate **unit-tested to 100%** (fixture profiles covering every
   rule 6–15 branch: exact-100 pass/fail, floor pass/fail, missing floor, absent required, absent floored,
   empty required set, update-floor write, refuse-lower, forgive-missing-after-write).
2. CI runs **both** `bash tests/coverage.sh` **and** `make cover` for a transition window and asserts they
   produce the **same** pass/fail verdict and the same per-package table (a small diff harness). Any
   divergence is a port bug, caught before it matters.
3. Once equivalent across a few real runs (including an intentional floor breach and a fresh unfloored
   package), **delete `tests/coverage.sh`**, point CI at `make cover` only, and update `CLAUDE.md`/docs.

## 4. Phase 2 (later, incremental — NOT this spec's build)

Port the 37 behavioral shell tests (`tests/go/*.sh`, `tests/manifest/*.sh`) to Go tests one at a time,
each keeping `make cover` green and shrinking `tests/run-all.sh` toward deletion. This is the "refactor the
scripts toward 100%" north star; it is independent of Phase 1 and done defect-by-defect, never big-bang.

## 5. Non-goals / honest limits

- **Not** rewriting the behavioral shell suite now (Phase 2). `make cover` still runs `bash run-all.sh`.
- **Not** removing `package.json` — metareview installs via npm (`cli/metareview.js` launcher); that role
  stays. Only its *test/build* scripts move to make.
- covergate does **no** process execution → it cannot itself run tests; the Makefile does. This is the
  point: the gate is pure and testable, orchestration is make's job.

## 6. Acceptance

- `cmd/covergate` at **100%** (`internal`-class rule already applies to new gate code by intent).
- `make cover` reproduces `coverage.sh`'s verdict on: a clean tree (pass), an `internal/fsm` package at
  99.x% (FAIL rule 7), a below-floor package (FAIL rule 8), a new unfloored package (FAIL rule 9), a
  vanished required/floored package (FAIL rules 11/12), and `--update-floor` (write + refuse-lower).
- The parallel-run diff harness reports zero divergence before `coverage.sh` is deleted.
- Every documented lesson in `coverage.sh` (toolchain pin, covdata-newline avoidance, floor semantics,
  plain rebuild-on-exit) is preserved and, for the gate rules, covered by a covergate test.
