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

## 1. The 16 invariants `coverage.sh` enforces (every one must survive the port)

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
   zero-statement packages — a documented trap). **Zero-statement packages** (review §2.2) emit no profile
   lines, so they never appear in the per-package map; they are handled entirely by presence checks against
   `--packages`/`--require-100`/floor (rules 9/11/12), never assigned a percentage. A required package with
   zero statements is therefore *absent* → FAIL (rule 11), never a vacuous `0==0` pass.

**Enforce** (→ Go, pure):
7. `internal/fsm/*` and `workflows`: **exact 100%** (every statement). Not exact → FAIL.
8. Every other package: `pct ≥ floor` from `tests/coverage-floor.txt`. Below → FAIL.
9. A package with **no floor line** → FAIL (`no floor: add a line, or --update-floor`). New packages must
   be floored deliberately (a whole subsystem, `internal/mutation`, once shipped unmeasured this way).
   Evaluated over the **`--packages` universe**, not the profile (review §2.3/§2.6): this matches
   `coverage.sh`'s *code* — whose stale header comment still says "absent from the floor file are reported
   but not enforced," a line the code overrode (see its own "UNGATED, and silently so" comment). Parity is
   against the code, which FAILs; the port fixes the universe hole the profile-only version left open.
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
16. **`--update-floor` must not silently drop a vanished floored package** (review §2.4). Rule 13
    regenerates from *measured* packages only, so a package that disappeared from the profile (rule 12)
    would lose its floor line before rule 12's failure is acted on. `--update-floor` therefore **refuses to
    write** while any floored package is absent from the profile (same class as refuse-to-lower, rule 14),
    reporting the vanished package(s); the maintainer removes the line deliberately or restores the package.

## 2. Architecture — split orchestration (make) from gate (Go)

**`cmd/covergate` is pure over its inputs** (so it reaches 100% itself, trivially, with fixture profiles):

```text
covergate --profile <textfmt> --floor <file> --module <path> \
          --packages <universe-list-file> --require-100 <require-list-file> \
          [--update-floor [--allow-floor-decrease]]
```

covergate takes **resolved package lists as files, not globs, and never shells out** (⇒ no exec seam, 100%
coverage is a file-and-flags exercise). The Makefile resolves both lists via `go list` and passes them:

- `--packages` = the **complete package universe** (`go list ./...`). This closes the package-universe
  hole (review §2.6): `covdata textfmt` only emits packages that were *instrumented and present in
  coverage metadata*, so a non-required package covdata omits would never be floor-checked. Rule 9 is
  evaluated over **`--packages`, not over the profile's packages** — every package in the universe must be
  required-100, floored, or explicitly exempt; one covdata dropped cannot pass unfloored.
- `--require-100` = the packages that must be exactly 100% — resolved by the Makefile as
  `go list ./internal/fsm/... ./workflows` **plus `cmd/covergate` itself** (the gate tool is held to the
  same bar it enforces; review §2.11). covergate does no globbing.
- `--module` = `go list -m`, to strip the module prefix from profile paths (rule 6).
- Implements rules 6–16. Exit 0 on pass, 1 on FAIL (with the package table), 2 on usage error.
- Plain `bufio`/`strings`, no new dependency.

**The Makefile owns orchestration** (rules 1–5, idiomatic make; `.ONESHELL` so one shell runs the whole
recipe under a `trap` that makes cleanup + plain rebuild **unconditional** — on success, failure, or
interrupt (review §2.1/§2.7), matching `coverage.sh`'s EXIT trap):

```makefile
.ONESHELL:
export GOTOOLCHAIN := $(shell awk '$$1=="toolchain"{print $$2}' go.mod)
build: ; go build -o bin/metareview ./cmd/metareview
cover:
	set -e
	COVDIR=$$(mktemp -d)
	trap 'rm -rf "$$COVDIR"; go build -o bin/metareview ./cmd/metareview || echo "post-run rebuild FAILED" >&2' EXIT
	go test -cover -covermode=atomic ./... -args -test.gocoverdir=$$COVDIR
	GOFLAGS="-cover -covermode=atomic" GOCOVERDIR=$$COVDIR bash tests/run-all.sh
	go tool covdata textfmt -i=$$COVDIR -o $$COVDIR/profile.txt
	go list ./... > $$COVDIR/universe.txt
	# tolerate an absent required dir (rule 10) but never swallow a real go-list error
	{ go list ./internal/fsm/... ./workflows 2>/dev/null; echo cmd/covergate | sed 's#^#$(shell go list -m)/#'; } | sort -u > $$COVDIR/require100.txt
	go run ./cmd/covergate --profile $$COVDIR/profile.txt --floor tests/coverage-floor.txt \
	  --module $$(go list -m) --packages $$COVDIR/universe.txt --require-100 $$COVDIR/require100.txt
test: cover
```

`package.json` **keeps `scripts.build` during Phase 1** (the behavioral suite still runs `npm run build`
under coverage — removing it before Phase 2 migrates the suite would break `make cover`, review §2.10);
`scripts.test` becomes a thin `make cover` wrapper. The full drop to distribution-only lands in Phase 2
once the shell suite no longer calls `npm run build`.

## 3. Transition — prove verdict-equivalence before deleting the script

1. Land `cmd/covergate` + Makefile with covergate **unit-tested to 100%** (fixture profiles covering every
   rule 6–16 branch: exact-100 pass/fail, floor pass/fail, missing floor (over the universe, incl. a
   package covdata omits), absent required, absent floored, zero-statement required, refuse-update-on-vanished,
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

- `cmd/covergate` at **100%**, enforced by adding it to `--require-100` (not left to a floor line).
- `make cover` reproduces `coverage.sh`'s verdict on: a clean tree (pass), an `internal/fsm` package at
  99.x% (FAIL rule 7), a below-floor package (FAIL rule 8), a new unfloored package (FAIL rule 9), a
  package in the universe that covdata omitted (FAIL rule 9 — the universe hole), a zero-statement required
  package (FAIL rule 11), a vanished required/floored package (FAIL rules 11/12), `--update-floor` (write +
  refuse-lower), and `--update-floor` with a vanished floored package (refuse-write, rule 16).
- The parallel-run diff harness reports zero divergence before `coverage.sh` is deleted.
- Every documented lesson in `coverage.sh` (toolchain pin, covdata-newline avoidance, floor semantics,
  plain rebuild-on-exit) is preserved and, for the gate rules, covered by a covergate test.
