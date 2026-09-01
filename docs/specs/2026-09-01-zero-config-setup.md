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

**B. The runner's own discovery → layout + test ids.** We never parse directories to find tests. The
runner respects its own config (`jest.config`, `pytest.ini`, `vitest.config`) and therefore already
handles co-located / `tests/**` / subdir transparently:

- `pytest --collect-only -q` → every nodeid
- `jest --listTests` → every test file
- `vitest list --json` → every test (Vitest's discovery subcommand; JSON array of
  `{file, name, …}`, mapped to test ids the same way `vitestConvention.ParseReport` maps run results —
  discovery reuses the convention's id derivation, it does not invent a second one)
- `go test -list '.*' ./...` → every test

Each convention owns **both** a run contract (`RunArgs`/`ParseReport`, already built) **and** a discovery
contract (`DiscoverArgs`/`ParseDiscovery`) added here; discovery ids MUST agree with run ids (same
derivation), so a discovered test and a failing test are the same identity.

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
(e.g. `frontend`, `server`, `pkg`). A finding's file (repo-relative, forward slashes) **matches** a route
iff the file path is lexically within that directory: `file == Path` or `file` has `Path + "/"` as a
prefix, compared **segment-wise** (so `serverx/a.py` does NOT match route `server`). The `**` glob form in
the human-facing examples is sugar for "this directory and everything under it" — it desugars to the same
prefix; metareview does not implement general glob matching, and a `config check` error is raised if a
`Path` contains glob metacharacters other than a trailing `/**` (§8: no bespoke matcher).

- **Specificity = number of path segments in `Path`.** The matching route with the **most segments** wins
  (nearest-enclosing-manifest, the same rule ecosystems use for nested projects).
- **Deterministic tie-breaker.** Two matching routes with equal segment count is a **configuration
  error**, not a silent pick: the analyzer never emits overlapping equal-specificity roots (manifests nest,
  they don't collide), and a hand-written `.metareview.yml` that does is rejected by `config check` with
  both offending `Path`s named. There is therefore no runtime "pick one of two equal routes" path — the
  ambiguity is resolved at config-validation time, before any finding is selected.
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
  - **Committed `.metareview.yml`:** the digest is taken over the same canonicalized route table the file
    resolves to (equivalently, the file's content hash, since the file *is* the canonical source) ⇒
    consent-once, stable in CI until the file changes.
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

- A pure Go repo (N=1 table) behaves exactly as today with **no config file** (regression guard).
- A constructed **multi-provider** repo (Vitest subtree + Jest subtree + pytest subtree + Go module),
  with **no config file**, runs the loop with each finding verified by its own runner — the runbook's
  mixed-language rung, now configurable.
- `config check` reports ✓ per route and exits nonzero on a broken route.
- `--write-config` refuses without `--consent` / interactive `y`; a normal run writes nothing.
- Detection leans only on manifests + runner discovery — no directory-layout heuristics in the code.
