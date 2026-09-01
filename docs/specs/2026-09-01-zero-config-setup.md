# Spec: Zero-config setup & config resolution (multi-language / multi-provider)

Status: **Design converged in-session with the maintainer (2026-09-01). Not yet decomposed into tasks.**
Date: 2026-09-01
Companion candidate: `docs/0.10.0-candidates.md` → "Zero-config setup & config resolution".
Builds on: the `internal/testconv` seam (PRs #48–#52), Gap #1 fix-node proof elicitation (#51), the
fix-scoped proof binding (#54), and the QA shakedown runbook (`docs/qa-shakedown-runbook.md`).

> **Provenance.** This spec is the written form of a design worked out conversationally with the
> maintainer on 2026-09-01, immediately after the four single-language conventions (Go, Jest, Vitest,
> pytest) were validated end-to-end via Docker. The transcript is the primary source; every design
> choice below was stated or ratified there. It is a *design path* accepted by the maintainer, not a
> completed artifact — it is decomposed and task-done-gated separately.

---

## 1. What problem this solves, and the evidence it is real

The shipped `sdlc-loop-proved` workflow is **Go-hardwired**: one `cmds.test` (`go test ./...`) and a
convention that defaults to `go`. Two facts, both established this session, make that a wall:

1. **Adoption is the wedge.** The easier metareview is to install, the more likely a human *or an AI
   agent* adopts it. A workflow that requires hand-editing YAML per language is friction exactly where
   we can least afford it. Maintainer's framing: *"the easier that it is to install, the more likely it
   is that people (and AI agents) will adopt metareview."*
2. **Real projects are multi-language and multi-provider.** Not just "Go xor TS xor Python," but one
   repo running **Vitest for some folders, Jest for others, and pytest for scripts/servers** — with
   tests laid out co-located, in `tests/**`, or in sibling subdirs. Maintainer: *"projects that may use
   vitest for some tests and Jest for others, and python/pytest for some scripts or servers."*

We proved the *proof machinery* is language-agnostic (the `testconv` seam; four runners validated live).
What is missing is **configuration**: selecting the right convention + test command for the right files,
without asking the user to write anything.

### The trap we are explicitly avoiding

Do **not** build a custom project-layout detector. Test layout (co-located vs `tests/**` vs subdir) is a
solved problem, and building our own sniffer is exactly the "bespoke parser" the maintainer has ruled
out three times this session for results parsing (*"as long as we aren't building a bespoke parser… use
other, robust parsers and have them plug in to metareview"*). The same principle applies to detection.

## 2. The two existing standards we lean on (no custom layout logic, ever)

**A. Ecosystem manifests → roots + provider + command.** Every ecosystem ships a standard,
machine-readable manifest with an off-the-shelf parser that already declares the project root and how to
test:

| Manifest | Parser | Signals |
| --- | --- | --- |
| `package.json` | `encoding/json` | `scripts.test`, `devDependencies` (jest vs vitest), `workspaces` |
| `pyproject.toml` / `tox.ini` / `setup.cfg` | TOML / INI | `[tool.pytest.ini_options]`, `[project]` |
| `go.mod` | `golang.org/x/mod/modfile` (already a dep) | module root ⇒ `go test ./...` |

**The manifest's *location* is the routing boundary.** Every ecosystem already defines its manifest as
"a project root," so the routing table is *derived from where manifests sit*, not from globs we invent.

**A manifest only makes a route if it actually configures a test runner.** The **provider comes from the
test-runner dependency**, never from the mere presence of the manifest or of a `scripts.test` — and a
manifest with no recognized runner yields **no route** (its files fall to another route or to the
auditable no-proof exemption). Found dogfooding metareview on itself (2026-09-01): its root
`package.json` has **no** jest/vitest dependency yet a `scripts.test: "bash tests/coverage.sh"` that runs
the **Go** suite. A detector keying a JS route off "package.json exists" or "has a `scripts.test`" would
fabricate a bogus JS route running Go tests under a JS convention. Correct result: **no JS route** — the
root `go.mod` is the only test route (a single-language `Path:""` table, §3.1). Symmetrically, a
`pyproject.toml`/`setup.cfg` with no `[tool.pytest…]` and no pytest dependency is not a python route.

**B. The runner's own discovery → layout + test ids.** We never parse directories to find tests. The
runner respects its own config (`jest.config`, `pytest.ini`, `vitest.config`) and therefore already
handles co-located / `tests/**` / subdir transparently:

- `pytest --collect-only -q` → every nodeid (test-level)
- `jest --listTests` → every test **file** (file-level only — Jest's list does not enumerate test names)
- `vitest list` → `file > test name` per test (test-level)
- `go test -list '.*' ./...` → every test name (test-level)

**The run report is the identity authority, not discovery.** Proof binding uses the id a convention's
`ParseReport` emits — validated live in the mixed-repo shakedown (2026-09-01): a pytest proof declared by
bare function name (`test_clamp_upper_bound`) came back **malformed** because `ParseReport` keys results by
nodeid; re-declared as the nodeid (`tests/test_clamp.py::test_clamp_upper_bound`) it proved. So the
per-convention **run-report selector** is: Go = test func name; pytest = `file::name` nodeid; Jest/Vitest
= the test's `fullName`. The fix-node proof instructions (today a generic `"<TestName>"`, which is
Go-centric) MUST state this selector per active convention — captured as a docs/UX fix in §8.

Discovery is a coarser **existence/coverage** aid layered on top, and its granularity is whatever the
runner offers: **test-level** where the runner enumerates tests (pytest `--collect-only`, `vitest list`,
`go test -list`) and only **file-level** where it does not (Jest `--listTests`). The contract is therefore
"discovery ids map into the run-report identity space at their available granularity" — a file-level
discovery maps to the run report's tests within that file — **not** the stronger (and for Jest false)
"discovery yields run-identical test ids."

By invoking the runner we inherit its config resolution *for free* — the same "the tool is the source of
truth" move used to read `--json` / JUnit results. Layout variance lives entirely inside the runner, not
in metareview.

## 3. The routing table (the detector's output)

The detector is deliberately thin. Its output is a **routing table**:

```
RoutingTable = [ Route... ]
Route = { Path (manifest root, most-specific-wins), Convention (testconv name), TestCmd (argv) }
```

A pure single-language repo is the **N=1** case; a multi-provider monorepo is N rows. This subsumes the
single-language model — single-language is not a separate code path, it is a one-row table.

**Per-finding selection.** `prove` / reproduction look up the finding's file against the table → use that
row's `Convention` + `TestCmd`. A file matched by **no** row owes no proof — an *auditable exemption*, the
same shape as today's "no added lines ⇒ owes no pin."

### 3.1 Route-matching semantics (fully specified — no ambiguity at selection time)

`Route.Path` is a **directory prefix**, expressed as the manifest's directory relative to the repo root
(e.g. `frontend`, `server`, `pkg`). The **repo root** is expressed as `Path: ""` (empty; `.` is accepted
and normalized to it) — the common single-language case, a `go.mod`/`package.json`/`pyproject.toml` at the
top level. A finding's file (repo-relative, forward slashes) **matches** a route iff the root is `""`
(matches every file — the empty prefix) **or** the file path is lexically within the directory: `file ==
Path` or `file` has `Path + "/"` as a prefix, compared **segment-wise** (so `serverx/a.py` does NOT match
route `server`). The `**` glob form in the examples is sugar for "this directory and everything under it"
— it desugars to the same prefix (`**` alone desugars to the root route `""`); metareview does not
implement general glob matching, and a `config check` error is raised if a `Path` contains glob
metacharacters other than a trailing `/**` (§8: no bespoke matcher).

- **Specificity = number of path segments in `Path`** (the root `""` has zero, so it always loses to any
  subtree route). The matching route with the **most segments** wins (nearest-enclosing-manifest).
- **A route also matches only files its convention owns** (by language/extension: a `go` route matches
  `*.go`, a `python` route `*.py`, a JS route `*.js`/`*.ts`/`*.jsx`/`*.tsx`). This is what makes
  **same-directory manifests of different ecosystems** — `go.mod` next to `package.json`, common in
  mixed repos — *not* a tie: a `.go` finding selects the go route, a `.ts` finding the JS route, even
  though both roots have equal path-specificity. Language is the tie-breaker before path length matters.
- **Same-ecosystem config duplicates in one directory** (`pyproject.toml` + `setup.cfg` + `tox.ini`, or
  `jest.config` beside `package.json`) collapse to **one** route via a fixed source precedence
  (`pyproject.toml` > `setup.cfg` > `tox.ini`; `package.json` is the JS root, its `jest.config`/
  `vitest.config` only disambiguate the provider). The analyzer emits one route per (directory, language),
  never two.
- **Genuine equal-specificity, same-language overlap** (only constructible by hand in `.metareview.yml`,
  never emitted by the analyzer) is a **configuration error**, rejected by `config check` naming both
  `Path`s — so there is no runtime "pick one of two equal routes" path.
- **No-match** is the auditable exemption above (owes no proof), recorded so it is visible, not silent.

**Per-subtree scope.** The whole-suite check and `IsTestFile` use the *finding's row's* convention, and
the runner is invoked rooted at that row's manifest directory. A single fix touching two subtrees
produces two proofs, each with its subtree's convention (this is a natural extension of the existing
per-finding proof loop, not new proof machinery).

## 4. Config resolution ladder (eslint / jest / tsconfig style)

1. **`--config <path>`** — explicit, wins (non-standard placement / CI).
2. Else a **conventional file at a standard location**, auto-discovered by walking up from CWD to the
   repo root (`.metareview.yml`). No flag needed.
3. Else the **analyzer** synthesizes the table dynamically (§2).

### 4.1 No-config is first-class, not a fallback

With no file present, the analyzer runs **every invocation and it just works** — the only cost is a few
seconds of detection. The file is a **cache + override + customization knob, never a prerequisite.**
Fresh install → run, no new files. This is a hard requirement, stated by the maintainer: *"we should run
even if there is no written config, it would just take longer as it would do it dynamically each time."*

**Optional internal cache (transparent, no committed file).** metareview may keep an internal detection
cache keyed on the manifests' content hashes, under `.metareview/` (self-ignoring, like `runs/`). Repeat
runs skip re-detection until a manifest changes. This buys the *speed* of a config file with **no
committed file and no user action** — so a committed `.metareview.yml` is purely for *determinism /
override*, not for speed.

## 5. The config file — thin, standard-located, validatable

The file fills only the two variable slots; the shipped workflow (the proof machinery) stays fixed.

```yaml
# .metareview.yml (repo root; walked-up-to like .git)
tests:
  - { path: "frontend/**", convention: vitest, cmd: "vitest run" }
  - { path: "server/**",   convention: python, cmd: "pytest" }
  - { path: "pkg/**",      convention: go,     cmd: "go test ./pkg/..." }
# defaults: { judge, repo_mode, ... }   # optional global knobs
```

### 5.1 `metareview config check` — prove the file correct

A hand-written file is not trusted on faith. `config check` **dry-runs each route** and reports ✓/✗ per
row — does the `cmd` run? does the runner discover tests? does the convention parse the report?

```
frontend/** → vitest : ✓ ran, 42 tests discovered, report parsed
server/**   → pytest : ✓ ran, 17 tests discovered
pkg/**      → go     : ✗ `go test ./pkg/...` found no tests — check path
```

Same don't-trust-verify spine as the rest of metareview: a maintainer writes it and metareview *proves*
it resolves. On any ✗, exit nonzero (CI-usable).

**Execution contract — `config check` *runs commands*, so it is gated exactly like a run.** The dry-run
label means "no fix is attempted and no finding is filed," **not** "no subprocess executes": it invokes
each route's `cmd` (plus the convention's discovery args) against the real tree. Therefore it (a) requires
**run consent** — the same `--allow-custom-cmds <sha>` / config-file-hash gate as §6, refusing with the
consent error if the commands are not consented; (b) runs each command in a throwaway worktree, never
mutating the source tree; (c) documents that it executes project test/discovery commands (side effect:
whatever those commands do). It is not a passive parse. An implementation that only wants to validate
*shape* (schema, path existence, glob-metacharacter rejection) without executing is a separate
`config check --no-exec` that makes no consent demand and discovers nothing — the executing form is the
default and is consent-gated.

## 6. Consent — two distinct surfaces

Running is not writing; keep the gates separate.

- **Run consent** — approving the detected/declared *commands*, via the **existing
  `workflow.CmdsSHA256` contract** — do not invent a new digest. Today `CmdsSHA256` hashes the canonical
  `AllowedCmd` list and refuses a changed digest; here the canonical input is **the resolved route table's
  commands** — i.e. every route's `TestCmd` (and discovery args) in canonical order. Two consequences,
  specified so no-config consent is not left undefined:
  - **No-config run:** the analyzer's synthesized table is reduced to its resolved command list, hashed
    with the *same* `CmdsSHA256` routine, and consent is required for that digest exactly as for a
    hand-declared `cmds:` block. Re-detection that yields the *same* commands yields the *same* digest ⇒
    no re-consent; a manifest change that changes a command changes the digest ⇒ re-consent (the intended
    guard). The digest's canonical input **includes the route `Path`s**, not only the command strings, so
    moving a command to a different subtree is itself a consent-relevant change.
  - **Committed `.metareview.yml`:** the digest is taken over the **same canonicalized resolved route
    table** — never the file's raw content hash. One canonical definition, used identically for the
    no-config and committed-file cases: the resolved routes as an ordered list of `(Path, convention,
    TestCmd argv, discovery argv)` tuples, sorted by the composite key **`(Path, convention)`** — `Path`
    alone is not a unique sort key, because same-directory multi-ecosystem routes share a `Path` and differ
    only by convention (§3.1), so a `Path`-only sort is unstable and two implementations could serialize
    the same table differently. `(Path, convention)` is unique per route (one route per `(directory,
    language)`), argv normalized, serialized canonically, then `CmdsSHA256`. Comments, key order, and the `defaults:` block do **not** affect it (they are not part
    of the resolved table); a changed command or a subtree move (a changed `Path`) does. This removes any
    "equivalently the content hash" ambiguity — two implementers computing the digest from the resolved
    table get the same value.
- **Write consent** — writing `.metareview.yml` **mutates the repo**, so it happens **only** on an
  explicit `--write-config` and **only** after affirmative consent: an interactive `[y/N]` by default,
  and a **`--consent` / `--yes` flag** so an agent or CI can pre-authorize it headlessly. **Never a side
  effect of a normal run** — a no-config run detects in memory and mutates nothing. Maintainer: *"we
  shouldn't write a config file unless we get affirmative consent from the user (or we include a new
  --consent or the like so it can be run programmatically with consent)."*

`metareview init --write-config` dumps the *detected* table to the standard file, so a maintainer starts
from detected-and-tweak (`tsc --init` / `eslint --init` style) rather than a blank page.

## 7. The enabler (first brick): `test_convention` as a workflow `--var`

The shipped `sdlc-loop-proved` currently hardcodes `go`. The first, smallest, independently-shippable
step toward all of the above is to make the `prove` node's `test_convention` a **workflow variable with
default `go`** — so one workflow serves every language by passing a var, instead of maintaining a
per-language YAML copy. This is a prerequisite for §3's per-finding selection (selection by value rather
than by hardcode) and is safe on its own (default preserves today's Go behavior). See the runbook's
"Workflow variants" section for the interim copy-the-YAML approach this replaces.

## 8. Honest hard edges (called out, not hand-waved)

- **Overlapping / ambiguous routes** — a file matched by two manifests → most-specific path wins; ties
  are a `config check` error, not a silent pick.
- **Non-standard setups** — a Makefile `test` target, a bespoke script: read the project's *own* declared
  command (`scripts.test`, the Makefile target), never guess. Truly exotic / no manifest → the analyzer
  does **not** guess; it surfaces "couldn't detect; supply the test command" (via the consent gate), and
  the human/agent provides it once.
- **Provider-within-a-language** — Jest and Vitest in one JS tree is exactly why a Route carries *both*
  convention *and* command; per-subtree `package.json` usually disambiguates which runner owns which
  folder.
- **Runner discovery has a cost** — `--collect-only` / `--listTests` require *running* the runner (cheap,
  and we already run it for tests). In Docker-isolated shakedowns this goes through the same wrapper
  script (`run-tests.sh`) as the test command.
- **Per-convention proof selector (docs/UX, found live 2026-09-01).** The fix-node proof instructions say
  a generic `"test":"<TestName>"`, which is Go-centric. The correct selector is the convention's
  run-report id (Go func name; pytest `file::name` nodeid; Jest/Vitest `fullName`). In the mixed-repo
  shakedown a bare-name pytest selector produced a *malformed* proof; the nodeid produced *proven*. Fix:
  make the fix-node instructions state the active convention's selector format. This is a docs/UX change,
  **not** a machinery bug — pytest-alone and pytest-in-a-mixed-tree both prove correctly once the id is
  right.
- **No meta-tool dependency** — Nx / Bazel solve this but only if the project adopts them; leaning on the
  project's own manifests + runners is universal (every project has both) and requires zero adoption.

## 9. Decomposition (proposed ship order)

- **Ship 1 — the enabler.** `test_convention` as a `--var` (default `go`) on `sdlc-loop-proved`. Retire
  the per-language YAML copies in the runbook. *Small, safe, unblocks everything.*
- **Ship 2 — routing table + per-finding selection.** Data model (`Route`, `RoutingTable`), most-specific
  match, per-finding convention/command selection in prove + reproduction, per-subtree scope. Fed by an
  explicit `.metareview.yml` (analyzer stubbed).
- **Ship 3 — the analyzer.** Manifest parsers (json/toml/go.mod) → roots + provider + command; runner
  discovery for enumeration. No-config-first-class path + optional manifest-hash cache.
- **Ship 4 — config file lifecycle.** Resolution ladder, `--config`, walk-up discovery, `config check`
  dry-run, `--write-config` + `--consent`, consent-by-file-hash.

Each ship is TDD / DI / mock-AI, `internal/*` at 100%, mutation-verified, `review task-done`-gated, and
shepherded through the bots — the standard methodology.

## 10. Non-goals (for this spec)

- Not building a layout detector (§1 trap). Not adopting Nx/Bazel/any meta-tool (§8).
- Not the mixed-language *shakedown* itself — that is the runbook's job once Ship 1–2 land; this spec is
  the machinery that makes a mixed-language run configurable.
- Not the QA-for-OSS auto-filing workflow (separate candidate in `docs/0.10.0-candidates.md`).
- Not new proof forms — pins/reproduction/deletion are unchanged; only *selection* of convention+command
  becomes per-finding.

## 11. Acceptance / validation

- A pure Go repo (N=1 table) behaves exactly as today with **no config file** (regression guard). Note
  its `go.mod` is at the **repo root** ⇒ `Path: ""` ⇒ matches every `.go` file (§3.1 root-route rule —
  the case a naive prefix rule would wrongly exempt).

> **Mixed-repo dry run (2026-09-01).** A 4-provider repo (`gopkg/` go · `web-jest/` jest · `web-vitest/`
> vitest · `pyserver/` pytest) was stood up and the detector run **by hand**: manifest discovery produced
> the four roots + providers + commands with zero custom layout logic, and each runner's own discovery
> (`go test -list`, `jest --listTests`, `vitest list`, `pytest --collect-only`) enumerated its tests — the
> routing table fell straight out. A pytest-subtree loop then ran end-to-end (recall 2/2 cold reviewers,
> codex-confirmed, reproduction proof **PROVEN**, `DONE(fixed)`), confirming per-subtree proof in a mixed
> tree is identical to single-language. Findings folded back into this spec: the root-route rule (§3.1),
> same-directory manifests (§3.1), Jest file-level discovery (§2), the per-convention proof selector (§8).

> **Dogfood on metareview itself (2026-09-01).** metareview is Go-only (root `go.mod`, 875 tests via
> `go test -list ./...`) with a `package.json` that is a launcher shim — **no** jest/vitest dep, but a
> `scripts.test` delegating to a bash script that runs the Go suite. The detector must therefore yield a
> **one-row `Path:""` Go table** and **no** JS route (§2 "a manifest only makes a route if it configures a
> test runner"; §3.1 root-route `Path:""`). This is the case a naive "package.json ⇒ JS route" detector
> gets wrong, and the reason provider detection keys on the runner dependency, not the manifest's presence.
- A constructed **multi-provider** repo (Vitest subtree + Jest subtree + pytest subtree + Go module),
  with **no config file**, runs the loop with each finding verified by its own runner — the runbook's
  mixed-language rung, now configurable.
- `config check` reports ✓ per route and exits nonzero on a broken route.
- `--write-config` refuses without `--consent` / interactive `y`; a normal run writes nothing.
- Detection leans only on manifests + runner discovery — no directory-layout heuristics in the code.
