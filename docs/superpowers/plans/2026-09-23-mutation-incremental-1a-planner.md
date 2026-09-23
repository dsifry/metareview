# Mutation-Incremental Harness — Plan 1a: Planner Core

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the deterministic planning core of `templates/mutation-incremental/`: config validation, the glob dialect, the working-tree snapshot, the import graph, the content-based line diff, state reading and adoption, the §5.4/§11 planner, deferral classification, views, and a `plan` CLI command that prints the plan as JSON.

**Architecture:** Small zero-dependency ESM modules under `templates/mutation-incremental/lib/`. Everything in this plan is pure planning — nothing here runs Stryker, writes state or talks to a remote. `computePlan` takes a snapshot, an import graph and the adopted state and returns the plan; `lib/main.mjs` wires it into `cli.mjs plan`. Unit tests use `node:test`; a root-package Go test runs the suite under `go test` / `make cover` with 100% Node coverage thresholds.

**Tech Stack:** Node ≥ 22.8 (ESM, `node:test`, built-in coverage), git, Go 1.26 (wrapper test only).

**Spec:** `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r21, approved). Read §§1–10 through §11, which overrides the sections it names. Companion: `docs/superpowers/specs/2026-09-23-mutation-incremental-adoption-contract.md`.

**Plan series:** 1a (this plan) → 1b (`2026-09-23-mutation-incremental-1b-execution.md`: running Stryker, attestation writing and commit, lock, verifier, `run`/`seed`/`fetch-state`/`publish-state`/`break-lock`, `pending_cause` outputs) → 1c (`2026-09-23-mutation-incremental-1c-proof.md`: workflow template, fixture, e2e proof, docs) → 2 (`2026-09-23-mutation-incremental-gate.md`: the Go gate, spec §6 and §11.3/§11.5 gate parts). Each produces working, tested software. The advisories carried from the approving review (`docs/metareview/reviews/mrv-20260923-211009828247000-artifact-2026-09-23-mutation-incremental-design-54abffb7.md`, Orchestrator Notes) that fall in 1a's scope are handled here and marked "(review advisory)".

## Global Constraints

- Node ≥ 22.8, ESM `.mjs`, **no npm dependencies** in the template (spec §5).
- Template files live in `templates/mutation-incremental/`; tests in `templates/mutation-incremental/test/*.test.mjs`.
- Node coverage: 100% lines, branches and functions for every non-test `.mjs` in the template (spec §7.2); the Go wrapper asserts it. Go coverage stays 100% per package (`make cover`); the wrapper lives in the root package, which the gate excludes.
- Glob dialect exactly as spec §5.3; vectors in `testdata/mutation-incremental/glob-vectors.json`. Line diff exactly as §11.4.1; vectors in `testdata/mutation-incremental/diff-vectors.json`. Plan 2's Go implementations run the same vector files.
- Exit codes (spec §5.1): 0 ok, 1 threshold/verify failure, 2 config/usage/validation, 3 lock held, 4 engine failure, 130 interrupted; precedence 2 > 3 > 130 > 4 > 1 > 0. This plan only produces 0 and 2.
- Attestation constants: `schemaVersion` 1, `tool` `metareview-mutation-incremental`, `toolVersion` `0.13.0` (informational), `stateVersion` 1 (usability, §11.6).
- Unit tests may create repositories under `os.tmpdir()` with `MUTATION_ALLOW_TMP_STATE=1`; the real e2e (Plan 1c) never uses `/tmp`.
- Never read or run anything in `../thread*` repositories.
- Review stance (user): real workflows and real edge cases; assume trust; no engineering for rare races.
- Each task ends with a commit; messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Byte order for sorted output means JavaScript's default string comparison (UTF-16 code units), which equals byte order for ASCII paths.

## Review Focus

1. **Installed packages are not open importers.** `vitest`, `node:assert` and any installed dependency must never make a file an open importer, or every helper edit defers everything. Task 4, test `installed packages and builtins are not open importers`.
2. **Deleting or `git mv`-renaming a covered source file with its test must not exit 2.** Deleted paths are dropped from scope and forced. Task 7, tests `delete b.ts and b.test.ts together` and `git mv c.ts to d.ts`.
3. **Residual mode never re-attests a stale kill of the edited file.** The file stays whole in the unforced scope; the changed lines and every mutant with a boundary on a changed line are forced (budget-exempt); the coverage closure is forced as mapped ranges. Task 7, tests `residual: a.ts line 3`, `residual: a block mutant opening on an edited line is forced`, `residual: a block mutant closing on an edited line is forced`, `residual: e.ts closure is narrower than the file` and `residual: inserted line shifts mapped ranges`.
4. **A lockfile edit during main's pending window counts against the PR.** An identical deferral in main's state is inherited only while the named inputs have the same digests. Task 8, test `classify: identical deferral with a changed digest is counted`.
5. **A new `mutate` file that no view covers exits 2 at plan time** (the Keeper operational rule). Task 8, test `resolveViews: a mutate file in no view is exit 2`.

## Plan decisions beyond the spec text (flagged for review)

- **Diff cap.** `MAX_EDIT_DISTANCE = 2000`. Above that, `lineDiff` returns `null`, and `computePlan` treats the file exactly like the > 1 MiB fallback: it is forced whole-file and its importer tests are added (K4). This bounds memory, since Myers traces grow with D². Plan 2's Go diff uses the same constant, and a capped diff there is a blanket cause, like a zero-mutant file (gate plan review note).
- **Changed lines and boundary mutants are forced in `residual` mode (plan review findings, checked against StrykerJS source).** Stryker decides reuse at character level: a mutant is reused when no text inside its own span changed. A `--mutate file:a-b` range creates only mutants wholly inside it. So a mutant on an edited line whose own columns did not change is not re-run by the unforced scope. The same holds for a multi-line mutant that starts after the edited characters on its first line or ends before them on its last line (a block opening on an edited `if (…) {` line). For a residual-edited file, the harness therefore forces:
  - the new-side lines of every changed hunk;
  - every mutant of the file that intersects a changed hunk without strictly spanning it, over its span mapped to current lines. An endpoint on a changed line takes the hunk's new-side start or end.

  Both are mandatory and budget-exempt, as contract K5.1 requires, and are never dropped by the budget. Mutants that strictly enclose a hunk (and mutants containing a pure insertion) are re-run by the scope, because their text changed.
- **Unexpected internal errors exit 4** (plan review finding). `main` maps any error that is not a `UsageError` to exit 4 with its message. For `run` in Plan 1b, exit 1 means "ok, state committed", so an internal crash must never produce Node's default 1.
- **Unresolvable relative or alias specifiers make the file an open importer** (spec §5.4: anything that neither resolves nor is a package is unresolved).
- **Duplicate JSON keys.** The spec requires exit 2 on a duplicate view name. `JSON.parse` silently keeps the last duplicate, so `json.mjs` adds a small scanner, `duplicateKey(text)`. It runs on the harness config, the Stryker config, the core `package.json` and the views command's output.
- **Views command timeout** (review advisory). The command runs with a 60 s timeout. A timeout is exit 2, like any other views command failure.
- **Reason → named-key table** (review advisory). The table in `lib/deferrals.mjs` is the single source for §11.2 (ii), and an unknown reason is exit 2.

---

## File Structure

```
templates/mutation-incremental/
  cli.mjs                 # entry: `plan` in 1a; 1b adds run/seed/fetch-state/publish-state/break-lock
  lib/errors.mjs          # UsageError(2), LockHeldError(3), EngineError(4)
  lib/json.mjs            # canonicalJSON, duplicateKey
  lib/glob.mjs            # compileGlob, matchList, categorize (§5.3)
  lib/config.mjs          # loadConfig + validation (§5.2, §11.1–§11.5)
  lib/snapshot.mjs        # sha256, git, digestOf, takeSnapshot (§5.4 snapshot, runtime map)
  lib/report.mjs          # indexReport
  lib/graph.mjs           # specifiers, buildGraph, reachesFrom, importerTests, openImporters (§5.4)
  lib/diff.mjs            # normalizeText, splitLines, lineDiff, hunkIntersects (§11.4.1)
  lib/state.mjs           # constants, readCandidate, changeSet, adopt (§5.4 Usable/Adopt/Change set)
  lib/plan.mjs            # computePlan, publicPlan (§5.4 steps + normalise, §11.4, §11.5)
  lib/deferrals.mjs       # canonical/dedupe/sort, reasonInfo, classifyDeferrals, pendingCause (§11.2)
  lib/views.mjs           # resolveViews, viewFiles, viewSummaries (§11.3)
  lib/nodever.mjs         # nodeVersionWarning (§5.1)
  lib/main.mjs            # argument parsing and the `plan` command
  test/helpers.mjs        # makeRepo, baseConfig, writeState
  test/*.test.mjs
mutationtemplate_test.go  # root package: runs the Node suite with 100% thresholds
testdata/mutation-incremental/glob-vectors.json
testdata/mutation-incremental/diff-vectors.json
```

---

### Task 1: Scaffolding, canonical JSON, glob dialect, Go wrapper

**Files:**
- Create: `templates/mutation-incremental/lib/errors.mjs`, `lib/json.mjs`, `lib/glob.mjs`, `cli.mjs` (placeholder)
- Create: `testdata/mutation-incremental/glob-vectors.json`
- Create: `templates/mutation-incremental/test/glob.test.mjs`, `test/json.test.mjs`
- Create: `mutationtemplate_test.go`
- Modify: `.gitignore` (add `.e2e/`)

**Interfaces:**
- Produces:
  - `UsageError`, `LockHeldError`, `EngineError` (each has `.exitCode`);
  - `canonicalJSON(value): string`;
  - `compileGlob(pattern): RegExp`;
  - `matchList(path, list): boolean`;
  - `categorize(path, lists): 'global'|'support'|'test'|'mutate'|'ignore'|'unclassified'`;
  - `CATEGORY_ORDER`.

- [ ] **Step 1: Write the glob vectors (shared with Plan 2)**

`testdata/mutation-incremental/glob-vectors.json`:

```json
{
  "match": [
    { "pattern": "src/**/*.ts", "path": "src/a.ts", "match": true },
    { "pattern": "src/**/*.ts", "path": "src/x/y/a.ts", "match": true },
    { "pattern": "src/**/*.ts", "path": "src/.hidden/a.ts", "match": false },
    { "pattern": "src/**/*.ts", "path": "lib/a.ts", "match": false },
    { "pattern": "./src/*.ts", "path": "src/a.ts", "match": true },
    { "pattern": "src/*.ts", "path": "src/x/a.ts", "match": false },
    { "pattern": "tests/helpers/**", "path": "tests/helpers", "match": true },
    { "pattern": "tests/helpers/**", "path": "tests/helpers/a/b.ts", "match": true },
    { "pattern": "**/*.md", "path": "README.md", "match": true },
    { "pattern": "**/*.md", "path": "docs/x/y.md", "match": true },
    { "pattern": "**/.*", "path": ".eslintrc", "match": true },
    { "pattern": "**/.*", "path": "a/.nvmrc", "match": true },
    { "pattern": "*", "path": ".env", "match": false },
    { "pattern": "tsconfig*.json", "path": "tsconfig.build.json", "match": true },
    { "pattern": "src/{a,b}.ts", "path": "src/b.ts", "match": true },
    { "pattern": "src/{a,b}.ts", "path": "src/c.ts", "match": false },
    { "pattern": "src/?.ts", "path": "src/a.ts", "match": true },
    { "pattern": "src/?.ts", "path": "src/ab.ts", "match": false },
    { "pattern": "src/a?.ts", "path": "src/ab.ts", "match": true },
    { "pattern": "**", "path": "a/b/c", "match": true },
    { "pattern": "a.b", "path": "axb", "match": false },
    { "pattern": ".mutation/**", "path": ".mutation/attestation.json", "match": true }
  ],
  "list": [
    { "list": ["src/**/*.ts", "!src/**/*.d.ts"], "path": "src/a.ts", "match": true },
    { "list": ["src/**/*.ts", "!src/**/*.d.ts"], "path": "src/a.d.ts", "match": false },
    { "list": [], "path": "src/a.ts", "match": false }
  ],
  "categorize": {
    "lists": {
      "global": ["package.json"],
      "support": ["tests/helpers/**"],
      "test": ["tests/**/*.test.ts"],
      "mutate": ["src/**/*.ts"],
      "ignore": ["**/*.md"]
    },
    "cases": [
      { "path": "package.json", "category": "global" },
      { "path": "tests/helpers/x.md", "category": "support" },
      { "path": "tests/a.test.ts", "category": "test" },
      { "path": "src/a.ts", "category": "mutate" },
      { "path": "docs/a.md", "category": "ignore" },
      { "path": "scripts/x.sh", "category": "unclassified" }
    ]
  }
}
```

- [ ] **Step 2: Write the failing tests**

`templates/mutation-incremental/test/glob.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compileGlob, matchList, categorize } from '../lib/glob.mjs';

const vectors = JSON.parse(readFileSync(new URL('../../../testdata/mutation-incremental/glob-vectors.json', import.meta.url), 'utf8'));

test('glob vectors: single patterns', () => {
  for (const v of vectors.match) {
    assert.equal(compileGlob(v.pattern).test(v.path), v.match, `${v.pattern} vs ${v.path}`);
  }
});

test('glob vectors: lists with negation', () => {
  for (const v of vectors.list) assert.equal(matchList(v.path, v.list), v.match, JSON.stringify(v));
});

test('glob vectors: categorize precedence', () => {
  const { lists, cases } = vectors.categorize;
  for (const c of cases) assert.equal(categorize(c.path, lists), c.category, c.path);
});

test('compileGlob caches and handles unclosed braces literally', () => {
  assert.equal(compileGlob('a/{b'), compileGlob('a/{b'));
  assert.equal(compileGlob('a/{b').test('a/{b'), true);
  assert.equal(compileGlob('a**b').test('axxb'), true);
});
```

`templates/mutation-incremental/test/json.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { canonicalJSON } from '../lib/json.mjs';
import { UsageError, LockHeldError, EngineError } from '../lib/errors.mjs';

test('canonicalJSON sorts keys recursively and ends with a newline', () => {
  const out = canonicalJSON({ b: 1, a: { d: [{ z: 1, y: 2 }], c: null } });
  assert.equal(out, '{\n  "a": {\n    "c": null,\n    "d": [\n      {\n        "y": 2,\n        "z": 1\n      }\n    ]\n  },\n  "b": 1\n}\n');
});

test('errors carry exit codes', () => {
  assert.equal(new UsageError('x').exitCode, 2);
  assert.equal(new LockHeldError('x').exitCode, 3);
  assert.equal(new EngineError('x').exitCode, 4);
});

test('cli placeholder loads', async () => {
  await import('../cli.mjs');
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `node --test templates/mutation-incremental/test/*.test.mjs`
Expected: FAIL — `Cannot find module '../lib/glob.mjs'`.

- [ ] **Step 4: Implement**

`templates/mutation-incremental/lib/errors.mjs`:

```js
// Exit codes are part of the CLI contract (spec §5.1): 2 usage/config, 3 lock held, 4 engine failure.
export class UsageError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 2;
  }
}

export class LockHeldError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 3;
  }
}

export class EngineError extends Error {
  constructor(message) {
    super(message);
    this.exitCode = 4;
  }
}
```

`templates/mutation-incremental/lib/json.mjs`:

```js
// Deterministic JSON so attestations and plans byte-compare across runs and machines.
function sortKeys(value) {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, sortKeys(value[key])]));
  }
  return value;
}

export function canonicalJSON(value) {
  return `${JSON.stringify(sortKeys(value), null, 2)}\n`;
}
```

`templates/mutation-incremental/lib/glob.mjs`:

```js
// Glob dialect shared with metareview's Go gate (spec §5.3). Both implementations run
// testdata/mutation-incremental/glob-vectors.json, so a change here needs the same change there.
const SEG = '(?!\\.)[^/]+';
const compiled = new Map();

function segment(seg, atStart) {
  let out = '';
  for (let i = 0; i < seg.length; i++) {
    const c = seg[i];
    const start = atStart && i === 0;
    if (c === '*') {
      out += `${start ? '(?!\\.)' : ''}[^/]*`;
      while (seg[i + 1] === '*') i++;
    } else if (c === '?') {
      out += `${start ? '(?!\\.)' : ''}[^/]`;
    } else if (c === '{' && seg.indexOf('}', i) > i) {
      const end = seg.indexOf('}', i);
      const alternatives = seg.slice(i + 1, end).split(',');
      out += `(?:${alternatives.map((alt) => segment(alt, start)).join('|')})`;
      i = end;
    } else {
      out += c.replace(/[.+^$()|[\]\\{}]/g, '\\$&');
    }
  }
  return out;
}

export function compileGlob(pattern) {
  const p = pattern.startsWith('./') ? pattern.slice(2) : pattern;
  const cached = compiled.get(p);
  if (cached) return cached;
  const segs = p.split('/');
  let src = '';
  let needSep = false;
  segs.forEach((seg, i) => {
    const last = i === segs.length - 1;
    if (seg === '**') {
      if (last) src += i === 0 ? `(?:${SEG}(?:/${SEG})*)?` : `(?:/${SEG})*`;
      else src += `${needSep ? '/' : ''}(?:${SEG}/)*`;
      needSep = false;
    } else {
      src += `${needSep ? '/' : ''}${segment(seg, true)}`;
      needSep = true;
    }
  });
  const re = new RegExp(`^${src}$`);
  compiled.set(p, re);
  return re;
}

export function matchList(path, list) {
  let hit = false;
  for (const entry of list) {
    if (entry.startsWith('!')) {
      if (compileGlob(entry.slice(1)).test(path)) return false;
    } else if (!hit) {
      hit = compileGlob(entry).test(path);
    }
  }
  return hit;
}

export const CATEGORY_ORDER = ['global', 'support', 'test', 'mutate'];

export function categorize(path, lists) {
  for (const name of CATEGORY_ORDER) {
    if (matchList(path, lists[name])) return name;
  }
  return matchList(path, lists.ignore) ? 'ignore' : 'unclassified';
}
```

`templates/mutation-incremental/cli.mjs` (placeholder; Task 9 replaces it):

```js
export {};
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `node --test templates/mutation-incremental/test/*.test.mjs`
Expected: PASS (7 tests).

- [ ] **Step 6: Add the Go wrapper and the `.e2e/` ignore**

`mutationtemplate_test.go` (root package — excluded from the Go coverage gate; it adds no statements to gated packages):

```go
package metareview

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMutationIncrementalTemplate runs the Node test suite of templates/mutation-incremental with
// 100% coverage thresholds. It fails in CI when Node is missing (a skipped suite is a gate that
// passed because it looked at less) and skips only on a developer machine without Node.
func TestMutationIncrementalTemplate(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("node >= 22.8 is required in CI to test templates/mutation-incremental")
		}
		t.Skip("node not installed: templates/mutation-incremental tests were NOT run")
	}
	version, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Fatalf("node --version: %v", err)
	}
	if !nodeAtLeast(strings.TrimSpace(string(version)), 22, 8) {
		t.Fatalf("node >= 22.8 required, found %s", version)
	}
	cmd := exec.Command(node, "--test", "--experimental-test-coverage",
		"--test-coverage-include=templates/mutation-incremental/**/*.mjs",
		"--test-coverage-exclude=templates/mutation-incremental/test/**",
		"--test-coverage-lines=100", "--test-coverage-branches=100", "--test-coverage-functions=100",
		"templates/mutation-incremental/test/*.test.mjs")
	cmd.Env = append(os.Environ(), "MUTATION_ALLOW_TMP_STATE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node test suite failed: %v\n%s", err, out)
	}
	report := string(out)
	sources, err := filepath.Glob("templates/mutation-incremental/lib/*.mjs")
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, "templates/mutation-incremental/cli.mjs")
	for _, src := range sources {
		name := filepath.Base(src)
		line := regexp.MustCompile(`(?m)^.*\b` + regexp.QuoteMeta(name) + `\s*\|.*$`).FindString(report)
		if line == "" {
			t.Errorf("%s does not appear in the coverage report (never loaded by a test)", src)
			continue
		}
		if strings.Count(line, "100.00") < 3 {
			t.Errorf("%s is below 100%% coverage: %s", src, line)
		}
	}
}

func nodeAtLeast(v string, major, minor int) bool {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return false
	}
	ma, _ := strconv.Atoi(parts[0])
	mi, _ := strconv.Atoi(parts[1])
	return ma > major || (ma == major && mi >= minor)
}
```

Append to `.gitignore`:

```
# local e2e runs of templates/mutation-incremental (tests/e2e-mutation-incremental.sh)
.e2e/
```

- [ ] **Step 7: Run the Go wrapper**

Run: `go test -run TestMutationIncrementalTemplate .`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add templates/mutation-incremental testdata/mutation-incremental/glob-vectors.json mutationtemplate_test.go .gitignore
git commit -m "feat(mutation-incremental): scaffold template, glob dialect and Go test wrapper

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Config loading and validation (spec §5.2, §11)

**Files:**
- Modify: `templates/mutation-incremental/lib/json.mjs` (add `duplicateKey`)
- Create: `templates/mutation-incremental/lib/config.mjs`
- Create: `templates/mutation-incremental/mutation-incremental.example.json`
- Create: `templates/mutation-incremental/test/helpers.mjs`
- Create: `templates/mutation-incremental/test/config.test.mjs`
- Modify: `templates/mutation-incremental/test/json.test.mjs`

**Interfaces:**
- Consumes: `UsageError`, `matchList` (Task 1).
- Produces:
  - `duplicateKey(text): string|null`.
  - `loadConfig(top, configArg?) → Config`, where `Config` is:
    ```
    { top, configPath, stateDir, stateDirRaw, stateRel,
      lists: {mutate,test,support,global,ignore}, exclusions: string[],
      aliases, stryker: {command,configFile,extraArgs}, engineVersion,
      budget: {maxForcedShare,maxForcedMutants,maxMinutesPerInvocation},
      residualMode, runtime: {commands,env},
      editedFiles: 'residual'|'whole', pendingOnPr: 'allow'|'full'|'full-on-global',
      allowBail, verify: null|{command,timeoutMinutes}, views: null|{command}|{inline},
      thresholdBreak: number|null, disableBail: boolean }
    ```
  - `TOOL_DIR_GLOB`.
  - In `test/helpers.mjs`: `baseConfig()`, `makeRepo({config, stryker, files}) → {top, git, write}` and `writeState(dir, {report, attestation})`.

- [ ] **Step 1: Write the test helper**

`templates/mutation-incremental/test/helpers.mjs`:

```js
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';

export const baseConfig = () => ({
  schemaVersion: 1,
  stateDir: '.mutation',
  stryker: { command: [process.execPath, 'fake-stryker.mjs'], configFile: 'stryker.config.json', extraArgs: [] },
  mutate: ['src/**/*.ts'],
  test: ['tests/**/*.test.ts'],
  support: ['tests/helpers/**'],
  global: ['package.json'],
  ignore: ['**/*.md'],
  aliases: {},
  runtime: { commands: [], env: [] },
  budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 5 },
  residual: { mode: 'bounded' },
});

// A throwaway git repo with a valid harness config, a JSON Stryker config and a fake
// @stryker-mutator/core install, so loadConfig passes without real Stryker.
export function makeRepo({ config = {}, stryker = {}, files = {} } = {}) {
  const top = mkdtempSync(join(tmpdir(), 'mi-'));
  const git = (...args) => execFileSync('git', args, { cwd: top, encoding: 'utf8' });
  const write = (rel, text) => {
    mkdirSync(dirname(join(top, rel)), { recursive: true });
    writeFileSync(join(top, rel), text);
  };
  git('init', '-q');
  git('config', 'user.email', 't@example.com');
  git('config', 'user.name', 'T');
  const cfg = { ...baseConfig(), ...config };
  write('mutation-incremental.json', JSON.stringify(cfg));
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: cfg.mutate, ignorePatterns: [cfg.stateDir], disableBail: true, ...stryker }));
  write('node_modules/@stryker-mutator/core/package.json', JSON.stringify({ version: '10.0.0' }));
  write('.gitignore', 'node_modules/\n.mutation/\n');
  for (const [rel, text] of Object.entries(files)) write(rel, text);
  git('add', '-A');
  git('commit', '-q', '-m', 'init');
  return { top, git, write };
}

// Writes a usable state pair into dir: the report bytes and a minimal attestation that
// readCandidate accepts. Attestation writing proper is Plan 1b's job.
export function writeState(dir, { report, attestation = {} }) {
  mkdirSync(dir, { recursive: true });
  const text = JSON.stringify(report);
  writeFileSync(join(dir, 'incremental.json'), text);
  const att = {
    schemaVersion: 1,
    tool: 'metareview-mutation-incremental',
    stateVersion: 1,
    reportSha256: createHash('sha256').update(text).digest('hex'),
    files: {},
    runtime: {},
    deferrals: [],
    ...attestation,
  };
  writeFileSync(join(dir, 'attestation.json'), JSON.stringify(att));
  return att;
}
```

- [ ] **Step 2: Write the failing tests**

Append to `templates/mutation-incremental/test/json.test.mjs`:

```js
import { duplicateKey } from '../lib/json.mjs';

test('duplicateKey finds a repeated key at any depth and ignores keys in arrays and strings', () => {
  assert.equal(duplicateKey('{"a":1,"b":{"c":1,"c":2}}'), 'c');
  assert.equal(duplicateKey('{"a":1,"a":2}'), 'a');
  assert.equal(duplicateKey('{"a":["a","a"],"b":"a\\"a","c":{"a":1}}'), null);
  assert.equal(duplicateKey('"just a string"'), null);
  assert.equal(duplicateKey('{"x\\u0041":1,"xA":2}'), 'xA');
});
```

`templates/mutation-incremental/test/config.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rmSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig, TOOL_DIR_GLOB } from '../lib/config.mjs';
import { makeRepo, baseConfig } from './helpers.mjs';

const cfg = (extra) => JSON.stringify({ ...baseConfig(), ...extra });

test('loads a valid config with defaults for the optional keys', () => {
  const { top } = makeRepo({ stryker: { tempDirName: '.st', jsonReporter: { fileName: 'r/m.json' }, htmlReporter: { fileName: 'r/m.html' }, thresholds: { break: 80 } } });
  const c = loadConfig(top);
  assert.deepEqual(c.lists.global, ['package.json', 'mutation-incremental.json', TOOL_DIR_GLOB]);
  assert.deepEqual(c.exclusions, ['.mutation/**', '.st/**', 'r/m.json', 'r/m.html']);
  assert.equal(c.engineVersion, '10.0.0');
  assert.equal(c.stateRel, '.mutation');
  assert.equal(c.stateDirRaw, '.mutation');
  assert.equal(c.residualMode, 'bounded');
  assert.equal(c.editedFiles, 'residual');
  assert.equal(c.pendingOnPr, 'allow');
  assert.equal(c.allowBail, false);
  assert.equal(c.verify, null);
  assert.equal(c.views, null);
  assert.equal(c.thresholdBreak, 80);
  assert.equal(c.disableBail, true);
});

test('accepts the optional keys', () => {
  const { top, write } = makeRepo();
  write('mutation-incremental.json', cfg({
    editedFiles: 'whole', pendingOnPr: 'full-on-global', allowBail: true,
    verify: { command: ['node', 'v.mjs'], timeoutMinutes: 5 }, views: { inline: { u1: ['src/**'] } },
  }));
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'] }));
  const c = loadConfig(top);
  assert.equal(c.editedFiles, 'whole');
  assert.equal(c.pendingOnPr, 'full-on-global');
  assert.deepEqual(c.verify, { command: ['node', 'v.mjs'], timeoutMinutes: 5 });
  assert.deepEqual(c.views, { inline: { u1: ['src/**'] } });
  assert.equal(c.thresholdBreak, null);
  assert.equal(c.disableBail, false);
});

test('accepts views from a command and a verifier without a timeout', () => {
  const { top, write } = makeRepo();
  write('mutation-incremental.json', cfg({ views: { command: ['node', 'views.mjs'] }, verify: { command: ['node', 'v.mjs'] } }));
  const c = loadConfig(top);
  assert.deepEqual(c.views, { command: ['node', 'views.mjs'] });
  assert.deepEqual(c.verify, { command: ['node', 'v.mjs'], timeoutMinutes: null });
  assert.deepEqual(loadConfig(makeRepo().top).exclusions, ['.mutation/**', '.stryker-tmp/**', 'reports/mutation/mutation.json', 'reports/mutation/mutation.html']);
});

const invalid = [
  ['missing file', (r) => rmSync(join(r.top, 'mutation-incremental.json')), /not found/],
  ['unparseable', (r) => r.write('mutation-incremental.json', '{'), /cannot parse/],
  ['not an object', (r) => r.write('mutation-incremental.json', '[]'), /must be a JSON object/],
  ['duplicate key', (r) => r.write('mutation-incremental.json', cfg({}).replace('"schemaVersion":1', '"schemaVersion":1,"schemaVersion":1')), /duplicate key "schemaVersion"/],
  ['missing key', (r) => { const c = baseConfig(); delete c.budget; r.write('mutation-incremental.json', JSON.stringify(c)); }, /missing key budget/],
  ['unknown key', (r) => r.write('mutation-incremental.json', cfg({ extra: 1 })), /unknown key extra/],
  ['schemaVersion', (r) => r.write('mutation-incremental.json', cfg({ schemaVersion: 2 })), /schemaVersion/],
  ['list type', (r) => r.write('mutation-incremental.json', cfg({ test: 'x' })), /test must be an array/],
  ['aliases', (r) => r.write('mutation-incremental.json', cfg({ aliases: { a: 1 } })), /aliases/],
  ['residual', (r) => r.write('mutation-incremental.json', cfg({ residual: { mode: 'x' } })), /residual.mode/],
  ['residual null', (r) => r.write('mutation-incremental.json', cfg({ residual: null })), /residual.mode/],
  ['budget null', (r) => r.write('mutation-incremental.json', cfg({ budget: null })), /maxForcedShare/],
  ['stryker null', (r) => r.write('mutation-incremental.json', cfg({ stryker: null })), /stryker needs/],
  ['runtime null', (r) => r.write('mutation-incremental.json', cfg({ runtime: null })), /runtime needs/],
  ['share', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxForcedShare: 0 } })), /maxForcedShare/],
  ['maxForcedMutants', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxForcedMutants: 1.5 } })), /maxForcedMutants/],
  ['minutes', (r) => r.write('mutation-incremental.json', cfg({ budget: { ...baseConfig().budget, maxMinutesPerInvocation: 0 } })), /maxMinutesPerInvocation/],
  ['stryker shape', (r) => r.write('mutation-incremental.json', cfg({ stryker: { command: [] } })), /stryker needs/],
  ['runtime shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: {} })), /runtime needs/],
  ['runtime command shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: { commands: ['node'], env: [] } })), /runtime needs/],
  ['runtime env shape', (r) => r.write('mutation-incremental.json', cfg({ runtime: { commands: [], env: [1] } })), /runtime needs/],
  ['editedFiles', (r) => r.write('mutation-incremental.json', cfg({ editedFiles: 'x' })), /editedFiles/],
  ['pendingOnPr', (r) => r.write('mutation-incremental.json', cfg({ pendingOnPr: 'x' })), /pendingOnPr/],
  ['allowBail type', (r) => r.write('mutation-incremental.json', cfg({ allowBail: 'yes' })), /allowBail must be a boolean/],
  ['verify shape', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: 'x' } })), /verify must be/],
  ['verify extra key', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: ['x'], other: 1 } })), /verify must be/],
  ['verify not an object', (r) => r.write('mutation-incremental.json', cfg({ verify: 'x' })), /verify must be/],
  ['verify bad timeout', (r) => r.write('mutation-incremental.json', cfg({ verify: { command: ['x'], timeoutMinutes: 0 } })), /verify must be/],
  ['verify timeout required', (r) => r.write('mutation-incremental.json', cfg({ pendingOnPr: 'full', verify: { command: ['x'], timeoutMinutes: null } })), /timeoutMinutes is required/],
  ['views shape', (r) => r.write('mutation-incremental.json', cfg({ views: { command: ['x'], inline: {} } })), /views must be/],
  ['views array', (r) => r.write('mutation-incremental.json', cfg({ views: [] })), /views must be/],
  ['views inline array', (r) => r.write('mutation-incremental.json', cfg({ views: { inline: [] } })), /views must be/],
  ['off with full-on-global', (r) => r.write('mutation-incremental.json', cfg({ residual: { mode: 'off' }, pendingOnPr: 'full-on-global', verify: null })), /residual.mode off/],
  ['stateDir outside', (r) => r.write('mutation-incremental.json', cfg({ stateDir: '..' })), /inside the repository/],
  ['stateDir is top', (r) => r.write('mutation-incremental.json', cfg({ stateDir: '.' })), /inside the repository/],
  ['stateDir matches mutate', (r) => {
    r.write('mutation-incremental.json', cfg({ stateDir: 'src/state', mutate: ['src/**'] }));
    r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**'], ignorePatterns: ['src/state'], disableBail: true }));
  }, /must not match a mutate pattern/],
  ['stryker json', (r) => r.write('stryker.config.json', 'nope'), /readable JSON/],
  ['runner', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'jest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'], disableBail: true })), /testRunner "vitest"/],
  ['inPlace', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', inPlace: true, mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'], disableBail: true })), /inPlace/],
  ['mutate mismatch', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['lib/**'], ignorePatterns: ['.mutation'], disableBail: true })), /must equal config mutate/],
  ['ignorePatterns', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], disableBail: true })), /ignorePatterns must contain/],
  ['bail', (r) => r.write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation'] })), /disableBail: true/],
  ['allowBail with residual', (r) => r.write('mutation-incremental.json', cfg({ allowBail: true })), /allowBail is refused/],
  ['core missing', (r) => rmSync(join(r.top, 'node_modules'), { recursive: true }), /not installed/],
  ['core version', (r) => r.write('node_modules/@stryker-mutator/core/package.json', JSON.stringify({ version: '9.1.0' })), /StrykerJS 10.x/],
];

for (const [name, mutate, message] of invalid) {
  test(`rejects: ${name}`, () => {
    const repo = makeRepo();
    mutate(repo);
    assert.throws(() => loadConfig(repo.top), (e) => e.exitCode === 2 && message.test(e.message));
  });
}

test('rejects a stateDir under the OS temp dir unless allowed for tests', () => {
  const { top } = makeRepo();
  const saved = process.env.MUTATION_ALLOW_TMP_STATE;
  delete process.env.MUTATION_ALLOW_TMP_STATE;
  try {
    assert.throws(() => loadConfig(top), /temp directory/);
  } finally {
    if (saved !== undefined) process.env.MUTATION_ALLOW_TMP_STATE = saved;
  }
});

test('accepts ignorePatterns with a /** suffix and a relative --config path', () => {
  const { top, write } = makeRepo();
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: ['src/**/*.ts'], ignorePatterns: ['.mutation/**'], disableBail: true }));
  write('cfg/mi.json', JSON.stringify(baseConfig()));
  const c = loadConfig(top, 'cfg/mi.json');
  assert.ok(c.lists.global.includes('cfg/mi.json'));
});
```

- [ ] **Step 3: Run to verify failure**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/config.test.mjs templates/mutation-incremental/test/json.test.mjs`
Expected: FAIL — `Cannot find module '../lib/config.mjs'` and `duplicateKey` is not exported.

- [ ] **Step 4: Implement**

Append to `templates/mutation-incremental/lib/json.mjs`:

```js
// JSON.parse keeps the last of two duplicate keys silently; the spec treats a duplicate view name
// (and any duplicate config key) as an error. Call only on text that JSON.parse already accepted.
export function duplicateKey(text) {
  const stack = [];
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    if (c === '"') {
      let j = i + 1;
      while (text[j] !== '"') j += text[j] === '\\' ? 2 : 1;
      const top = stack[stack.length - 1];
      if (top !== undefined && top.object && top.expectKey) {
        const key = JSON.parse(text.slice(i, j + 1));
        if (top.keys.has(key)) return key;
        top.keys.add(key);
        top.expectKey = false;
      }
      i = j + 1;
      continue;
    }
    if (c === '{') stack.push({ object: true, keys: new Set(), expectKey: true });
    else if (c === '[') stack.push({ object: false });
    else if (c === '}' || c === ']') stack.pop();
    else if (c === ',') stack[stack.length - 1].expectKey = true;
    i++;
  }
  return null;
}
```

`templates/mutation-incremental/lib/config.mjs`:

```js
import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { isAbsolute, join, relative, resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { matchList } from './glob.mjs';
import { duplicateKey } from './json.mjs';

const REQUIRED = ['aliases', 'budget', 'global', 'ignore', 'mutate', 'residual', 'runtime', 'schemaVersion', 'stateDir', 'stryker', 'support', 'test'];
const OPTIONAL = ['allowBail', 'editedFiles', 'pendingOnPr', 'verify', 'views'];
const LIST_KEYS = ['mutate', 'test', 'support', 'global', 'ignore'];
export const TOOL_DIR_GLOB = 'tools/mutation-incremental/**';

const fail = (message) => {
  throw new UsageError(`config: ${message}`);
};
const toPosix = (p) => p.split('\\').join('/');
const inside = (root, p) => {
  const rel = relative(root, p);
  return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel));
};
const isArgv = (a) => Array.isArray(a) && a.length > 0 && a.every((s) => typeof s === 'string');
const positive = (n) => typeof n === 'number' && n > 0;
const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

function readJSON(path, what) {
  let text;
  let value;
  try {
    text = readFileSync(path, 'utf8');
    value = JSON.parse(text);
  } catch (e) {
    return fail(`${what}: ${e.message}`);
  }
  const dup = duplicateKey(text);
  if (dup !== null) fail(`${what}: duplicate key ${JSON.stringify(dup)}`);
  return value;
}

function validateOptional(raw) {
  const editedFiles = raw.editedFiles ?? 'residual';
  if (!['residual', 'whole'].includes(editedFiles)) fail('editedFiles must be residual or whole');
  const pendingOnPr = raw.pendingOnPr ?? 'allow';
  if (!['allow', 'full', 'full-on-global'].includes(pendingOnPr)) fail('pendingOnPr must be allow, full or full-on-global');
  const allowBail = raw.allowBail ?? false;
  if (typeof allowBail !== 'boolean') fail('allowBail must be a boolean');
  const verify = raw.verify ?? null;
  if (verify !== null) {
    const ok = isObject(verify)
      && Object.keys(verify).every((k) => k === 'command' || k === 'timeoutMinutes')
      && isArgv(verify.command)
      && (verify.timeoutMinutes === undefined || verify.timeoutMinutes === null || positive(verify.timeoutMinutes));
    if (!ok) fail('verify must be {"command": [argv], "timeoutMinutes": <positive number or null>}');
    // Spec §11.2: a job-level timeout must never pre-empt the harness, so the verifier is bounded.
    if (pendingOnPr !== 'allow' && !positive(verify.timeoutMinutes)) fail('verify.timeoutMinutes is required when pendingOnPr is not allow');
  }
  const views = raw.views ?? null;
  if (views !== null) {
    const keys = isObject(views) ? Object.keys(views) : [];
    const ok = keys.length === 1 && ((keys[0] === 'command' && isArgv(views.command)) || (keys[0] === 'inline' && isObject(views.inline)));
    if (!ok) fail('views must be {"command": [argv]} or {"inline": {name: [patterns]}}');
  }
  return { editedFiles, pendingOnPr, allowBail, verify: verify === null ? null : { command: verify.command, timeoutMinutes: verify.timeoutMinutes ?? null }, views };
}

export function loadConfig(top, configArg) {
  const configPath = resolve(top, configArg ?? 'mutation-incremental.json');
  if (!existsSync(configPath)) fail(`${configPath} not found`);
  const raw = readJSON(configPath, `cannot parse ${configPath}`);
  if (!isObject(raw)) fail('must be a JSON object');
  for (const key of REQUIRED) if (!(key in raw)) fail(`missing key ${key}`);
  for (const key of Object.keys(raw)) if (!REQUIRED.includes(key) && !OPTIONAL.includes(key)) fail(`unknown key ${key}`);
  if (raw.schemaVersion !== 1) fail('schemaVersion must be 1');
  for (const key of LIST_KEYS) {
    if (!Array.isArray(raw[key]) || !raw[key].every((s) => typeof s === 'string')) fail(`${key} must be an array of strings`);
  }
  const aliases = raw.aliases;
  if (!isObject(aliases) || !Object.values(aliases).every((v) => typeof v === 'string')) fail('aliases must map strings to strings');
  const mode = raw.residual?.mode;
  if (!['strict', 'bounded', 'off'].includes(mode)) fail('residual.mode must be strict, bounded or off');
  const budget = raw.budget ?? {};
  if (!(typeof budget.maxForcedShare === 'number' && budget.maxForcedShare > 0 && budget.maxForcedShare <= 1)) fail('budget.maxForcedShare must be in (0, 1]');
  if (!(budget.maxForcedMutants === null || (Number.isInteger(budget.maxForcedMutants) && budget.maxForcedMutants > 0))) {
    fail('budget.maxForcedMutants must be a positive integer or null');
  }
  if (!positive(budget.maxMinutesPerInvocation)) fail('budget.maxMinutesPerInvocation must be positive');
  const stryker = raw.stryker ?? {};
  if (!isArgv(stryker.command) || typeof stryker.configFile !== 'string' || !Array.isArray(stryker.extraArgs)) {
    fail('stryker needs command[], configFile and extraArgs[]');
  }
  const runtime = raw.runtime ?? {};
  if (!Array.isArray(runtime.commands) || !runtime.commands.every(isArgv) || !Array.isArray(runtime.env) || !runtime.env.every((n) => typeof n === 'string')) {
    fail('runtime needs commands[] (argv arrays) and env[] (variable names)');
  }
  const optional = validateOptional(raw);
  if (mode === 'off' && optional.pendingOnPr === 'full-on-global') fail('residual.mode off cannot be combined with pendingOnPr full-on-global');

  const stateDir = resolve(top, raw.stateDir);
  if (!inside(top, stateDir) || stateDir === resolve(top)) fail('stateDir must be inside the repository');
  if (!process.env.MUTATION_ALLOW_TMP_STATE) {
    for (const t of [tmpdir(), '/tmp']) {
      if (inside(realpathSync(t), realpathSync(top))) fail('stateDir must not be under the OS temp directory (it is purged)');
    }
  }
  const stateRel = toPosix(relative(top, stateDir));
  for (const probe of [stateRel, `${stateRel}/attestation.json`, `${stateRel}/incremental.json`]) {
    for (const key of ['mutate', 'test', 'support']) {
      if (matchList(probe, raw[key])) fail(`stateDir ${stateRel} must not match a ${key} pattern`);
    }
  }

  const sc = readJSON(resolve(top, stryker.configFile), 'stryker.configFile must be readable JSON');
  if (sc.testRunner !== 'vitest') fail('stryker config must use testRunner "vitest"');
  if (sc.inPlace === true) fail('stryker config must not set inPlace');
  if (JSON.stringify(sc.mutate) !== JSON.stringify(raw.mutate)) fail('stryker config mutate must equal config mutate');
  if (!Array.isArray(sc.ignorePatterns) || !sc.ignorePatterns.some((p) => p === stateRel || p === `${stateRel}/**`)) {
    fail(`stryker config ignorePatterns must contain ${stateRel}`);
  }
  const disableBail = sc.disableBail === true;
  if (!disableBail && !optional.allowBail) fail('stryker config must set disableBail: true (or set allowBail: true)');
  if (optional.allowBail && optional.editedFiles === 'residual') fail('allowBail is refused with editedFiles "residual" (its closure reads killedBy)');
  const corePkg = join(top, 'node_modules/@stryker-mutator/core/package.json');
  if (!existsSync(corePkg)) fail('@stryker-mutator/core is not installed');
  const engineVersion = readJSON(corePkg, 'cannot read @stryker-mutator/core').version;
  if (!String(engineVersion).startsWith('10.')) fail(`StrykerJS 10.x required, found ${engineVersion}`);

  const configRel = toPosix(relative(top, configPath));
  return {
    top,
    configPath,
    stateDir,
    stateDirRaw: raw.stateDir,
    stateRel,
    lists: { mutate: raw.mutate, test: raw.test, support: raw.support, global: [...raw.global, configRel, TOOL_DIR_GLOB], ignore: raw.ignore },
    exclusions: [
      `${stateRel}/**`,
      `${sc.tempDirName ?? '.stryker-tmp'}/**`,
      sc.jsonReporter?.fileName ?? 'reports/mutation/mutation.json',
      sc.htmlReporter?.fileName ?? 'reports/mutation/mutation.html',
    ],
    aliases,
    stryker,
    engineVersion,
    budget,
    residualMode: mode,
    runtime,
    ...optional,
    thresholdBreak: typeof sc.thresholds?.break === 'number' ? sc.thresholds.break : null,
    disableBail,
  };
}
```

`templates/mutation-incremental/mutation-incremental.example.json` (spec §5.2 defaults; §11 keys are optional and omitted):

```json
{
  "schemaVersion": 1,
  "stateDir": ".mutation",
  "stryker": { "command": ["npx", "--no-install", "stryker"], "configFile": "stryker.config.json", "extraArgs": [] },
  "mutate":  ["src/**/*.ts", "!src/**/*.d.ts"],
  "test":    ["tests/**/*.test.ts"],
  "support": ["tests/helpers/**", "tests/factories/**", "tests/fixtures/**"],
  "global":  ["package.json", "package-lock.json", "stryker.config.json", "vitest.config.ts", "tsconfig*.json", "migrations/**", ".env*", ".nvmrc", ".node-version"],
  "ignore":  ["docs/**", "**/*.md", ".github/**", ".vscode/**", ".gitignore", "LICENSE*", "Dockerfile", "Makefile", ".metareview/**", ".beads/**", ".claude/**"],
  "aliases": {},
  "runtime": { "commands": [], "env": [] },
  "budget":  { "maxForcedShare": 0.25, "maxForcedMutants": null, "maxMinutesPerInvocation": 20 },
  "residual": { "mode": "bounded" }
}
```

- [ ] **Step 5: Run to verify pass**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/config.test.mjs templates/mutation-incremental/test/json.test.mjs`
Expected: PASS. If the temp-dir test fails because the temp repo's realpath is not under `tmpdir()` on this machine, assert only when `realpathSync(tmpdir())` is a prefix of the repo path.

- [ ] **Step 6: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): config loading and validation incl. adoption options

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Snapshot, runtime map and report index

**Files:**
- Create: `templates/mutation-incremental/lib/snapshot.mjs`, `lib/report.mjs`
- Create: `templates/mutation-incremental/test/snapshot.test.mjs`

**Interfaces:**
- Consumes: `Config` (Task 2), `categorize` and `matchList` (Task 1), `UsageError`.
- Produces:
  - `sha256(data): hex` and `git(top, args): string`;
  - `digestOf(absPath): string|null`;
  - `takeSnapshot(config, {runCommand?}) → Snapshot`, where `Snapshot = { files: Record<path, {digest, category, tracked}>, runtime: Record<key, hex|'unset'> }`;
  - `indexReport(report) → { mutants: Mutant[], testIds: Record<testFile, string[]> }`, where `Mutant = { file, id, status, killedBy: string[], coveredBy: string[], startLine, endLine }`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/snapshot.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, rmSync, symlinkSync } from 'node:fs';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot, digestOf, sha256 } from '../lib/snapshot.mjs';
import { indexReport } from '../lib/report.mjs';
import { makeRepo } from './helpers.mjs';

test('snapshot classifies, excludes and records tracked', () => {
  const r = makeRepo({ files: { 'src/a.ts': 'a', 'tests/a.test.ts': 't', 'README.md': 'x', 'scripts/x.sh': 's' } });
  r.write('src/new.ts', 'n');
  r.write('.mutation/attestation.json', '{}');
  r.write('reports/mutation/mutation.json', '{}'); // not gitignored: excluded by the resolved exclusions
  const snap = takeSnapshot(loadConfig(r.top));
  assert.equal(snap.files['src/a.ts'].category, 'mutate');
  assert.equal(snap.files['src/a.ts'].digest, `sha256:${sha256('a')}`);
  assert.equal(snap.files['src/a.ts'].tracked, true);
  assert.equal(snap.files['src/new.ts'].tracked, false);
  assert.equal(snap.files['tests/a.test.ts'].category, 'test');
  assert.equal(snap.files['scripts/x.sh'].category, 'unclassified');
  assert.equal(snap.files['README.md'], undefined);
  assert.equal(snap.files['.mutation/attestation.json'], undefined);
  assert.equal(snap.files['reports/mutation/mutation.json'], undefined);
  assert.equal(snap.files['mutation-incremental.json'].category, 'global');
});

test('snapshot hashes symlinks by link text and skips gitlinks and deleted tracked files', () => {
  const r = makeRepo({ files: { 'src/a.ts': 'a' } });
  symlinkSync('a.ts', join(r.top, 'src/link.ts'));
  r.git('update-index', '--add', '--cacheinfo', `160000,${'1'.repeat(40)},vendor/sub`);
  r.write('src/gone.ts', 'g');
  r.git('add', 'src/gone.ts');
  rmSync(join(r.top, 'src/gone.ts')); // tracked in the index, deleted from the working tree
  const snap = takeSnapshot(loadConfig(r.top));
  assert.equal(snap.files['src/link.ts'].digest, `symlink:${sha256('a.ts')}`);
  assert.equal(snap.files['vendor/sub'], undefined);
  assert.equal(snap.files['src/gone.ts'], undefined);
});

test('digestOf returns null for directories and missing files, rethrows other errors', () => {
  const r = makeRepo();
  mkdirSync(join(r.top, 'dir'));
  assert.equal(digestOf(join(r.top, 'dir')), null);
  assert.equal(digestOf(join(r.top, 'missing')), null);
  assert.throws(() => digestOf('\0bad'));
});

test('runtime map hashes command output and env values', () => {
  const r = makeRepo({ config: { runtime: { commands: [['node', '--version']], env: ['MI_TEST_IMAGE', 'MI_UNSET'] } } });
  process.env.MI_TEST_IMAGE = 'pg16';
  delete process.env.MI_UNSET;
  const snap = takeSnapshot(loadConfig(r.top), { runCommand: () => Buffer.from('v22.9.0\n') });
  assert.equal(snap.runtime['cmd:node\u001f--version'], sha256('v22.9.0\n'));
  assert.equal(snap.runtime['env:MI_TEST_IMAGE'], sha256('pg16'));
  assert.equal(snap.runtime['env:MI_UNSET'], 'unset');
});

test('a failing runtime command is a usage error', () => {
  const r = makeRepo({ config: { runtime: { commands: [['definitely-not-a-command-xyz']], env: [] } } });
  assert.throws(() => takeSnapshot(loadConfig(r.top)), (e) => e.exitCode === 2);
});

test('indexReport flattens mutants and test ids', () => {
  const idx = indexReport({
    files: { 'src/a.ts': { mutants: [{ id: 1, status: 'Killed', killedBy: [0], coveredBy: [0, 1], location: { start: { line: 2 }, end: { line: 3 } } }, { id: '2', status: 'NoCoverage', location: { start: { line: 4 }, end: { line: 4 } } }] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 0 }, { id: '1' }] }, 'tests/b.test.ts': {} },
  });
  assert.deepEqual(idx.mutants[0], { file: 'src/a.ts', id: '1', status: 'Killed', killedBy: ['0'], coveredBy: ['0', '1'], startLine: 2, endLine: 3 });
  assert.deepEqual(idx.mutants[1].killedBy, []);
  assert.deepEqual(idx.testIds, { 'tests/a.test.ts': ['0', '1'], 'tests/b.test.ts': [] });
  assert.deepEqual(indexReport({}), { mutants: [], testIds: {} });
  assert.deepEqual(indexReport({ files: { 'x.ts': {} } }).mutants, []);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/snapshot.test.mjs`
Expected: FAIL — missing modules.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/snapshot.mjs`:

```js
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readlinkSync } from 'node:fs';
import { join } from 'node:path';
import { categorize, matchList } from './glob.mjs';
import { UsageError } from './errors.mjs';

export const sha256 = (data) => createHash('sha256').update(data).digest('hex');

export function git(top, args) {
  return execFileSync('git', args, {
    cwd: top,
    encoding: 'utf8',
    maxBuffer: 1 << 30,
    env: { ...process.env, GIT_LITERAL_PATHSPECS: '1' },
  });
}

const splitZ = (s) => s.split('\0').filter(Boolean);

// Spec §5.4: regular files by content, symlinks by link text (never followed), anything else skipped.
export function digestOf(absPath) {
  let st;
  try {
    st = lstatSync(absPath);
  } catch (e) {
    if (e.code === 'ENOENT') return null;
    throw e;
  }
  if (st.isSymbolicLink()) return `symlink:${sha256(readlinkSync(absPath))}`;
  if (st.isFile()) return `sha256:${sha256(readFileSync(absPath))}`;
  return null;
}

function listRepoPaths(top) {
  const all = splitZ(git(top, ['ls-files', '-c', '-o', '--exclude-standard', '--deduplicate', '-z']));
  const tracked = new Set();
  const gitlinks = new Set();
  for (const entry of splitZ(git(top, ['ls-files', '-c', '-s', '-z']))) {
    const path = entry.slice(entry.indexOf('\t') + 1);
    tracked.add(path);
    if (entry.startsWith('160000 ')) gitlinks.add(path);
  }
  return { paths: all.filter((p) => !gitlinks.has(p)), tracked };
}

function defaultRunCommand(argv, cwd) {
  try {
    return execFileSync(argv[0], argv.slice(1), { cwd });
  } catch (e) {
    throw new UsageError(`runtime command ${argv.join(' ')} failed: ${e.message}`);
  }
}

export function takeSnapshot(config, { runCommand = defaultRunCommand } = {}) {
  const { paths, tracked } = listRepoPaths(config.top);
  const files = {};
  for (const path of paths) {
    if (matchList(path, config.exclusions)) continue;
    const category = categorize(path, config.lists);
    if (category === 'ignore') continue;
    const digest = digestOf(join(config.top, path));
    if (digest === null) continue;
    files[path] = { digest, category, tracked: tracked.has(path) };
  }
  const runtime = {};
  for (const argv of config.runtime.commands) runtime[`cmd:${argv.join('\u001f')}`] = sha256(runCommand(argv, config.top));
  for (const name of config.runtime.env) {
    runtime[`env:${name}`] = process.env[name] === undefined ? 'unset' : sha256(process.env[name]);
  }
  return { files, runtime };
}
```

`templates/mutation-incremental/lib/report.mjs`:

```js
// Flattens a mutation-testing-report so the planner can query mutants by file and by test id.
// Ids are only meaningful inside this one report (spec F7).
export function indexReport(report) {
  const mutants = [];
  for (const [file, entry] of Object.entries(report.files ?? {})) {
    for (const m of entry.mutants ?? []) {
      mutants.push({
        file,
        id: String(m.id),
        status: m.status,
        killedBy: (m.killedBy ?? []).map(String),
        coveredBy: (m.coveredBy ?? []).map(String),
        startLine: m.location.start.line,
        endLine: m.location.end.line,
      });
    }
  }
  const testIds = {};
  for (const [file, entry] of Object.entries(report.testFiles ?? {})) {
    testIds[file] = (entry.tests ?? []).map((t) => String(t.id));
  }
  return { mutants, testIds };
}
```

- [ ] **Step 4: Run to verify pass**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/snapshot.test.mjs`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): snapshot, runtime map and report index

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Import graph, packages and open importers

**Files:**
- Create: `templates/mutation-incremental/lib/graph.mjs`
- Create: `templates/mutation-incremental/test/graph.test.mjs`

**Interfaces:**
- Consumes: `Snapshot.files` (Task 3), `Config.aliases`, `Config.top`.
- Produces:
  - `specifiers(source): string[]`;
  - `buildGraph(files, {top, aliases}) → Graph`, where `Graph = { forward: Map<from, Set<to>>, reverse: Map<to, Set<from>>, open: Map<from, string[]> }`;
  - `reachesFrom(graph, start): Set<path>` (forward closure, start included);
  - `importerTests(graph, files, target): Set<testPath>`;
  - `openImporters(graph) → [{path, specifiers}]` (sorted).

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/graph.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot } from '../lib/snapshot.mjs';
import { specifiers, buildGraph, importerTests, openImporters, reachesFrom } from '../lib/graph.mjs';
import { makeRepo } from './helpers.mjs';

test('specifiers finds runtime imports and skips type-only and vi.mock', () => {
  const src = [
    "import a from './a';",
    "import type { T } from './types';",
    'export { b } from "./b";',
    "export type { U } from './u';",
    "import './side';",
    "const c = await import('./c');",
    "const d = require('./d');",
    "vi.mock('./mocked');",
    "const x = obj.import('./not');",
    'export type Id = string',
    "export { h } from './h'",
  ].join('\n');
  assert.deepEqual(specifiers(src).sort(), ['./a', './b', './c', './d', './h', './side']);
});

function graphRepo(extra = {}) {
  const r = makeRepo({
    config: { aliases: { '@app/': 'src/' }, ...extra },
    files: {
      'src/a.ts': 'export const a = 1;',
      'src/b.ts': "import { a } from './a.js';\nexport const b = a;",
      'src/c.ts': 'export const c = 3;',
      'src/idx/index.ts': "export * from '../a';",
      'tests/helpers/make.ts': 'export const make = 1;',
      'tests/a.test.ts': "import { a } from '../src/a';\nimport assert from 'node:assert';\nimport { test } from 'vitest';",
      'tests/b.test.ts': "import { b } from '../src/b';\nimport { make } from './helpers/make';\nimport fs from 'fs';",
      'tests/c.test.ts': "import { c } from '@app/c';\nimport data from './data.json';",
      'tests/data.json': '{}',
      'tests/i.test.ts': "import { a } from '../src/idx';",
    },
  });
  mkdirSync(join(r.top, 'node_modules/vitest'), { recursive: true });
  return r;
}

test('installed packages and builtins are not open importers', () => {
  const r = graphRepo();
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), []);
  assert.deepEqual([...importerTests(g, snap.files, 'src/a.ts')].sort(), ['tests/a.test.ts', 'tests/b.test.ts', 'tests/i.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/helpers/make.ts')], ['tests/b.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')], ['tests/c.test.ts']);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/data.json')], ['tests/c.test.ts']);
  assert.deepEqual([...reachesFrom(g, 'tests/b.test.ts')].sort(), ['src/a.ts', 'src/b.ts', 'tests/b.test.ts', 'tests/helpers/make.ts']);
});

test('an npm link to a directory outside node_modules makes an open importer', () => {
  const r = graphRepo();
  const linked = mkdtempSync(join(tmpdir(), 'mi-linked-'));
  mkdirSync(join(r.top, 'node_modules/@org'), { recursive: true });
  symlinkSync(linked, join(r.top, 'node_modules/@org/linked'));
  r.write('src/l.ts', "import x from '@org/linked';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  assert.deepEqual(openImporters(buildGraph(snap.files, cfg)), [{ path: 'src/l.ts', specifiers: ['@org/linked'] }]);
});

test('workspace links into the repo and unknown bare specifiers make open importers', () => {
  const r = graphRepo({ aliases: {} });
  mkdirSync(join(r.top, 'packages/ws'), { recursive: true });
  mkdirSync(join(r.top, 'node_modules/@org'), { recursive: true });
  symlinkSync(join(r.top, 'packages/ws'), join(r.top, 'node_modules/@org/ws'));
  r.write('src/w.ts', "import x from '@org/ws/deep';\nimport y from 'not-installed';");
  r.write('tests/w.test.ts', "import { w } from '../src/w';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), [
    { path: 'src/w.ts', specifiers: ['@org/ws/deep', 'not-installed'] },
    { path: 'tests/c.test.ts', specifiers: ['@app/c'] },
  ]);
  assert.deepEqual([...importerTests(g, snap.files, 'tests/helpers/make.ts')].sort(), ['tests/b.test.ts', 'tests/c.test.ts', 'tests/w.test.ts']);
});

test('relative and alias specifiers that resolve to no file in the snapshot make open importers', () => {
  const r = graphRepo();
  r.write('tests/u.test.ts', "import x from '../../outside';\nimport y from './missing';\nimport z from '@app/nope';");
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual(openImporters(g), [{ path: 'tests/u.test.ts', specifiers: ['../../outside', './missing', '@app/nope'] }]);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')].sort(), ['tests/c.test.ts', 'tests/u.test.ts']);
});

test('the longest alias key wins', () => {
  const r = graphRepo({ aliases: { '@app/': 'lib/', '@app/c': 'src/c' } });
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.deepEqual([...importerTests(g, snap.files, 'src/c.ts')], ['tests/c.test.ts']);
  assert.deepEqual(openImporters(g), []);
});

test('openImporters sorts by path and de-duplicates specifiers', () => {
  const open = new Map([['b.ts', ['y']], ['a.ts', ['x', 'x']], ['c.ts', ['z']]]);
  assert.deepEqual(openImporters({ open }), [
    { path: 'a.ts', specifiers: ['x'] },
    { path: 'b.ts', specifiers: ['y'] },
    { path: 'c.ts', specifiers: ['z'] },
  ]);
});

test('files over 1 MiB, symlinks and non-source files are not parsed', () => {
  const r = graphRepo();
  r.write('src/big.ts', `import './a';\n${'x'.repeat((1 << 20) + 1)}`);
  symlinkSync('b.ts', join(r.top, 'src/alias.ts'));
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  const g = buildGraph(snap.files, cfg);
  assert.equal([...(g.reverse.get('src/a.ts') ?? [])].includes('src/big.ts'), false);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/graph.test.mjs`
Expected: FAIL — missing module.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/graph.mjs`:

```js
import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { builtinModules } from 'node:module';
import { dirname, join, posix } from 'node:path';

export const SOURCE_EXTS = ['.ts', '.tsx', '.mts', '.cts', '.js', '.jsx', '.mjs', '.cjs'];
const TS_EXTS = ['.ts', '.tsx', '.mts', '.cts'];
const MAX_BYTES = 1 << 20;
// String-literal specifiers only. `import type` / `export type` are elided at runtime (spec F4),
// and `vi.mock('./x')` replaces a module rather than depending on it.
// `=` is excluded so that, in code without semicolons, `export type X = …` cannot swallow the next
// statement's `from` (a real import/export clause never contains `=`).
const FROM = /(?:^|[^\w$.])(?:import|export)\s+(type\s+)?[^'"`;=]*?\bfrom\s*(['"])([^'"\n]+)\2/g;
const OTHERS = [
  /(?:^|[^\w$.])import\s*(['"])([^'"\n]+)\1/g,
  /(?:^|[^\w$.])import\s*\(\s*(['"])([^'"\n]+)\1\s*\)/g,
  /(?:^|[^\w$.])require\s*\(\s*(['"])([^'"\n]+)\1\s*\)/g,
];

export function specifiers(source) {
  const out = new Set();
  for (const m of source.matchAll(FROM)) if (!m[1]) out.add(m[3]);
  for (const re of OTHERS) for (const m of source.matchAll(re)) out.add(m[2]);
  return [...out];
}

function resolveFile(base, files) {
  const candidates = [base, ...SOURCE_EXTS.map((e) => base + e), ...SOURCE_EXTS.map((e) => `${base}/index${e}`)];
  const js = base.match(/^(.*)\.[mc]?jsx?$/);
  if (js) candidates.push(...TS_EXTS.map((e) => js[1] + e));
  return candidates.find((c) => files[c]?.digest.startsWith('sha256:')) ?? null;
}

// Spec §5.4: a bare specifier is a package when node_modules holds it and its realpath still has a
// node_modules segment (npm, pnpm, yarn node-modules). Workspace links into the repo's own sources,
// and `npm link` targets elsewhere, are unresolved, so the importing file is an open importer.
function isInstalledPackage(spec, fromFile, top) {
  const name = spec.startsWith('@') ? spec.split('/').slice(0, 2).join('/') : spec.split('/')[0];
  let dir = dirname(join(top, fromFile));
  for (;;) {
    const candidate = join(dir, 'node_modules', name);
    if (existsSync(candidate)) return realpathSync(candidate).split(/[\\/]/).includes('node_modules');
    if (dir === top) return false;
    dir = dirname(dir);
  }
}

function classify(spec, fromFile, ctx) {
  let base = null;
  if (spec.startsWith('./') || spec.startsWith('../')) {
    base = posix.normalize(posix.join(posix.dirname(fromFile), spec));
  } else {
    const key = Object.keys(ctx.aliases).filter((k) => spec.startsWith(k)).sort((a, b) => b.length - a.length)[0];
    if (key !== undefined) base = posix.normalize(ctx.aliases[key] + spec.slice(key.length));
  }
  if (base !== null) {
    // Spec §5.4: a relative or alias specifier that reaches no regular file in the snapshot (outside
    // the repo, gitignored or generated, excluded) is unresolved, so its file is an open importer.
    const to = base.startsWith('../') ? null : resolveFile(base, ctx.files);
    return to ? { edge: to } : { unresolved: true };
  }
  if (spec.startsWith('node:') || builtinModules.includes(spec.split('/')[0])) return {};
  return isInstalledPackage(spec, fromFile, ctx.top) ? {} : { unresolved: true };
}

const addTo = (map, key, value) => {
  if (!map.has(key)) map.set(key, new Set());
  map.get(key).add(value);
};

export function buildGraph(files, { top, aliases }) {
  const forward = new Map();
  const reverse = new Map();
  const open = new Map();
  const ctx = { files, aliases, top };
  for (const [path, info] of Object.entries(files)) {
    if (!info.digest.startsWith('sha256:') || !SOURCE_EXTS.some((e) => path.endsWith(e))) continue;
    const source = readFileSync(join(top, path), 'utf8');
    if (source.length > MAX_BYTES) continue;
    for (const spec of specifiers(source)) {
      const r = classify(spec, path, ctx);
      if (r.edge) {
        addTo(forward, path, r.edge);
        addTo(reverse, r.edge, path);
      } else if (r.unresolved) {
        if (!open.has(path)) open.set(path, []);
        open.get(path).push(spec);
      }
    }
  }
  return { forward, reverse, open };
}

function closure(map, start) {
  const seen = new Set();
  const queue = [start];
  while (queue.length > 0) {
    for (const next of map.get(queue.pop()) ?? []) {
      if (!seen.has(next)) {
        seen.add(next);
        queue.push(next);
      }
    }
  }
  return seen;
}

// Files `start` imports, transitively, plus `start` itself.
export function reachesFrom(graph, start) {
  const out = closure(graph.forward, start);
  out.add(start);
  return out;
}

// Spec §5.4: tests that reach the target, plus every test that reaches an open importer
// (it may import the target through a specifier we could not resolve).
export function importerTests(graph, files, target) {
  const tests = new Set();
  const addTests = (paths) => {
    for (const p of paths) if (files[p].category === 'test') tests.add(p);
  };
  addTests(closure(graph.reverse, target));
  for (const opener of graph.open.keys()) addTests([opener, ...closure(graph.reverse, opener)]);
  return tests;
}

export function openImporters(graph) {
  return [...graph.open.entries()]
    .map(([path, specs]) => ({ path, specifiers: [...new Set(specs)].sort() }))
    .sort((a, b) => (a.path < b.path ? -1 : 1));
}
```

- [ ] **Step 4: Run to verify pass**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/graph.test.mjs`
Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): import graph with packages, open importers and forward reach

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Content-based line diff (spec §11.4.1)

**Files:**
- Create: `templates/mutation-incremental/lib/diff.mjs`
- Create: `testdata/mutation-incremental/diff-vectors.json`
- Create: `templates/mutation-incremental/test/diff.test.mjs`

**Interfaces:**
- Produces:
  - `MAX_EDIT_DISTANCE` (2000);
  - `normalizeText(text): string` (strips a leading BOM, converts CRLF/CR to LF);
  - `splitLines(text): string[]`;
  - `lineDiff(oldText, newText) → { hunks: Hunk[], oldToNew: (number|null)[] } | null` (`null` above `MAX_EDIT_DISTANCE`; the caller treats the file as whole), where `Hunk = { oldStart, oldEnd, newStart, newEnd }` (1-based; an empty side has `end = start − 1`; a pure insertion goes after old line `oldStart − 1`) and `oldToNew[oldLine]` is the new line of an unchanged old line, otherwise `null`;
  - `hunkIntersects(start, end, hunk): boolean` (the §11.4.3 rule).

- [ ] **Step 1: Write the vectors (shared with Plan 2's Go diff)**

`testdata/mutation-incremental/diff-vectors.json`. Each expected value was derived by hand from Myers' algorithm with the tie rule "insert when `k == -d` or `v[k-1] < v[k+1]`":

```json
{
  "maxEditDistance": 2000,
  "cases": [
    { "name": "identical", "old": "a\nb\n", "new": "a\nb\n", "hunks": [] },
    { "name": "replace middle line", "old": "a\nb\nc\n", "new": "a\nB\nc\n", "hunks": [{ "oldStart": 2, "oldEnd": 2, "newStart": 2, "newEnd": 2 }] },
    { "name": "pure insertion", "old": "a\nc\n", "new": "a\nb\nc\n", "hunks": [{ "oldStart": 2, "oldEnd": 1, "newStart": 2, "newEnd": 2 }] },
    { "name": "pure deletion", "old": "a\nb\nc\n", "new": "a\nc\n", "hunks": [{ "oldStart": 2, "oldEnd": 2, "newStart": 2, "newEnd": 1 }] },
    { "name": "swap uses the tie rule", "old": "a\nb\n", "new": "b\na\n", "hunks": [{ "oldStart": 1, "oldEnd": 1, "newStart": 1, "newEnd": 0 }, { "oldStart": 3, "oldEnd": 2, "newStart": 2, "newEnd": 2 }] },
    { "name": "empty old", "old": "", "new": "a\n", "hunks": [{ "oldStart": 1, "oldEnd": 0, "newStart": 1, "newEnd": 1 }] },
    { "name": "CRLF and BOM normalised", "old": "\ufeffa\r\nb\r\n", "new": "a\nb\n", "hunks": [] },
    { "name": "missing final newline is not a change", "old": "a\nb", "new": "a\nb\n", "hunks": [] }
  ]
}
```

- [ ] **Step 2: Write the failing tests**

`templates/mutation-incremental/test/diff.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { lineDiff, hunkIntersects, splitLines, normalizeText, MAX_EDIT_DISTANCE } from '../lib/diff.mjs';

const vectors = JSON.parse(readFileSync(new URL('../../../testdata/mutation-incremental/diff-vectors.json', import.meta.url), 'utf8'));

test('diff vectors', () => {
  assert.equal(vectors.maxEditDistance, MAX_EDIT_DISTANCE);
  for (const v of vectors.cases) assert.deepEqual(lineDiff(v.old, v.new).hunks, v.hunks, v.name);
});

test('normalisation and line splitting', () => {
  assert.equal(normalizeText('\ufeffa\r\nb\rc'), 'a\nb\nc');
  assert.deepEqual(splitLines(''), []);
  assert.deepEqual(splitLines('a\n\nb\n'), ['a', '', 'b']);
});

test('oldToNew maps unchanged lines across an insertion', () => {
  const { oldToNew } = lineDiff('a\nb\nc\n', 'x\na\nb\nc\n');
  assert.deepEqual(oldToNew, [null, 2, 3, 4]);
  const del = lineDiff('a\nb\nc\n', 'a\nc\n');
  assert.deepEqual(del.oldToNew, [null, 1, null, 2]);
});

test('hunkIntersects: overlap for a changed range, containment for an insertion point', () => {
  const changed = { oldStart: 3, oldEnd: 3, newStart: 3, newEnd: 3 };
  assert.equal(hunkIntersects(1, 5, changed), true);
  assert.equal(hunkIntersects(2, 2, changed), false);
  const insertAfter3 = { oldStart: 4, oldEnd: 3, newStart: 4, newEnd: 4 };
  assert.equal(hunkIntersects(1, 5, insertAfter3), true);
  assert.equal(hunkIntersects(1, 3, insertAfter3), false);
  assert.equal(hunkIntersects(4, 5, insertAfter3), false);
});

test('above MAX_EDIT_DISTANCE lineDiff returns null (the planner treats the file as whole)', () => {
  const a = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `a${i}`).join('\n');
  const b = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `b${i}`).join('\n');
  assert.equal(lineDiff(a, b), null);
});
```

- [ ] **Step 3: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/diff.test.mjs`
Expected: FAIL — missing module.

- [ ] **Step 4: Implement**

`templates/mutation-incremental/lib/diff.mjs`:

```js
// Content-based line diff (spec §11.4.1), shared with the Go gate. Both implementations run
// testdata/mutation-incremental/diff-vectors.json, so a change here needs the same change there.
// Above MAX_EDIT_DISTANCE lineDiff returns null and the planner forces the file whole.
export const MAX_EDIT_DISTANCE = 2000;

export function normalizeText(text) {
  const noBom = text.startsWith('\ufeff') ? text.slice(1) : text;
  return noBom.replace(/\r\n?/g, '\n');
}

export function splitLines(text) {
  const t = normalizeText(text);
  if (t === '') return [];
  const lines = t.split('\n');
  if (lines[lines.length - 1] === '') lines.pop();
  return lines;
}

// Myers' O(ND) shortest edit script. Ties take the insertion ("down") move when k === -d or
// v[k-1] < v[k+1], otherwise the deletion ("right") move, so the result is deterministic.
// trace[d] keeps v for diagonals -d-1 .. d+1 as it was before step d.
function editOps(a, b) {
  const n = a.length;
  const m = b.length;
  const off = n + m + 1;
  const v = new Int32Array(2 * (n + m) + 3);
  const trace = [];
  for (let d = 0; d <= MAX_EDIT_DISTANCE; d++) {
    trace.push(v.slice(off - d - 1, off + d + 2));
    for (let k = -d; k <= d; k += 2) {
      let x = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]) ? v[off + k + 1] : v[off + k - 1] + 1;
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) {
        x++;
        y++;
      }
      v[off + k] = x;
      if (x >= n && y >= m) return backtrack(trace, n, m);
    }
  }
  return null;
}

function backtrack(trace, n, m) {
  const ops = [];
  let x = n;
  let y = m;
  for (let d = trace.length - 1; d >= 0; d--) {
    const w = trace[d];
    const at = (k) => w[k + d + 1];
    const k = x - y;
    const prevK = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
    const prevX = at(prevK);
    const prevY = prevX - prevK;
    while (x > prevX && y > prevY) {
      ops.push('=');
      x--;
      y--;
    }
    if (d > 0) ops.push(x === prevX ? '+' : '-');
    x = prevX;
    y = prevY;
  }
  return ops.reverse();
}

export function lineDiff(oldText, newText) {
  const a = splitLines(oldText);
  const b = splitLines(newText);
  const ops = editOps(a, b);
  if (ops === null) return null;
  const hunks = [];
  const oldToNew = [null];
  let i = 0;
  let j = 0;
  let cur = null;
  for (const op of ops) {
    if (op === '=') {
      i++;
      j++;
      oldToNew[i] = j;
      cur = null;
      continue;
    }
    if (cur === null) {
      cur = { oldStart: i + 1, oldEnd: i, newStart: j + 1, newEnd: j };
      hunks.push(cur);
    }
    if (op === '-') {
      i++;
      cur.oldEnd = i;
      oldToNew[i] = null;
    } else {
      j++;
      cur.newEnd = j;
    }
  }
  return { hunks, oldToNew };
}

// Spec §11.4.3: a mutant [start, end] (old lines) intersects a changed hunk when the ranges overlap,
// and a pure insertion when it contains the insertion point (strictly inside the mutant's span).
export function hunkIntersects(start, end, h) {
  if (h.oldEnd >= h.oldStart) return start <= h.oldEnd && end >= h.oldStart;
  const after = h.oldStart - 1;
  return start <= after && end >= after + 1;
}
```

- [ ] **Step 5: Run to verify pass**

Run: `node --test templates/mutation-incremental/test/diff.test.mjs`
Expected: PASS (5 tests). If a vector fails, re-derive it by hand from the tie rule. Do not change the vector to match the output without re-deriving it.

- [ ] **Step 6: Commit**

```bash
git add templates/mutation-incremental testdata/mutation-incremental/diff-vectors.json
git commit -m "feat(mutation-incremental): content-based Myers line diff with shared vectors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: State reading, change set and adoption (spec §5.4 Usable, Adopt, Change set; §11.6)

**Files:**
- Create: `templates/mutation-incremental/lib/state.mjs`
- Create: `templates/mutation-incremental/test/state.test.mjs`

**Interfaces:**
- Consumes: `sha256` (Task 3), `writeState` (Task 2 helpers).
- Produces:
  - `TOOL`, `TOOL_VERSION`, `STATE_VERSION`, `ATTESTATION_FILE`, `REPORT_FILE`;
  - `readCandidate(dir, {label, primary}) → Candidate`, where `Candidate = { label, dir, primary, usable, reason, attestation|null, report|null }`. The attestation is kept whenever it parses as an object, even when the candidate is unusable, because §11.2 (i) reads it;
  - `changeSet(attestation, snapshot) → Change[]` (`{path, category, kind}` sorted by path then category);
  - `adopt(candidates, snapshot) → (Candidate & {changes}) | null`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/state.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readCandidate, changeSet, adopt, STATE_VERSION, TOOL } from '../lib/state.mjs';
import { sha256 } from '../lib/snapshot.mjs';
import { writeState } from './helpers.mjs';

const dir = () => mkdtempSync(join(tmpdir(), 'mi-state-'));
const opts = { label: 'x', primary: false };

test('readCandidate: usable pair', () => {
  const d = dir();
  writeState(d, { report: { files: {} } });
  const c = readCandidate(d, opts);
  assert.equal(c.usable, true);
  assert.deepEqual(c.report, { files: {} });
  assert.equal(c.attestation.tool, TOOL);
});

test('readCandidate: each unusable reason', () => {
  const missing = readCandidate(join(dir(), 'nope'), opts);
  assert.deepEqual([missing.usable, missing.reason, missing.attestation], [false, 'missing attestation', null]);

  const bad = dir();
  writeFileSync(join(bad, 'attestation.json'), '{');
  assert.equal(readCandidate(bad, opts).reason, 'unparseable attestation');

  const arr = dir();
  writeFileSync(join(arr, 'attestation.json'), '[]');
  assert.equal(readCandidate(arr, opts).reason, 'unparseable attestation');

  const tool = dir();
  writeState(tool, { report: {}, attestation: { tool: 'other' } });
  assert.equal(readCandidate(tool, opts).reason, 'not a harness attestation');

  const version = dir();
  writeState(version, { report: {}, attestation: { stateVersion: STATE_VERSION + 1 } });
  const v = readCandidate(version, opts);
  assert.equal(v.reason, 'stateVersion mismatch');
  assert.equal(v.attestation.stateVersion, STATE_VERSION + 1);

  const noVersion = dir();
  writeState(noVersion, { report: {}, attestation: { stateVersion: undefined } });
  assert.equal(readCandidate(noVersion, opts).reason, 'stateVersion mismatch');

  const shape = dir();
  writeState(shape, { report: {}, attestation: { deferrals: 'x' } });
  assert.equal(readCandidate(shape, opts).reason, 'not a harness attestation');

  const noReport = dir();
  writeFileSync(join(noReport, 'attestation.json'), JSON.stringify({ schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: {}, runtime: {}, deferrals: [] }));
  assert.equal(readCandidate(noReport, opts).reason, 'missing report');

  const hash = dir();
  writeState(hash, { report: {}, attestation: { reportSha256: '0' } });
  assert.equal(readCandidate(hash, opts).reason, 'report hash mismatch');

  const junk = dir();
  mkdirSync(junk, { recursive: true });
  writeFileSync(join(junk, 'incremental.json'), 'nope');
  writeFileSync(join(junk, 'attestation.json'), JSON.stringify({ schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: {}, runtime: {}, deferrals: [], reportSha256: sha256('nope') }));
  assert.equal(readCandidate(junk, opts).reason, 'unparseable report');
});

const snap = (files, runtime = {}) => ({ files, runtime });
const f = (digest, category = 'mutate') => ({ digest, category, tracked: true });

test('changeSet: changed, new, deleted, category change, runtime', () => {
  const att = { files: { 'a.ts': f('1'), 'b.ts': f('1'), 'c.ts': f('1'), 'd.ts': f('1', 'support') }, runtime: { 'env:X': 'x', 'env:GONE': 'g', 'env:SAME': 's' } };
  const s = snap({ 'a.ts': f('1'), 'b.ts': f('2'), 'd.ts': f('1', 'test'), 'e.ts': f('1') }, { 'env:X': 'y', 'env:NEW': 'n', 'env:SAME': 's' });
  assert.deepEqual(changeSet(att, s), [
    { path: 'b.ts', category: 'mutate', kind: 'changed' },
    { path: 'c.ts', category: 'mutate', kind: 'deleted' },
    { path: 'd.ts', category: 'support', kind: 'deleted' },
    { path: 'd.ts', category: 'test', kind: 'new' },
    { path: 'e.ts', category: 'mutate', kind: 'new' },
    { path: 'env:GONE', category: 'global', kind: 'deleted' },
    { path: 'env:NEW', category: 'global', kind: 'new' },
    { path: 'env:X', category: 'global', kind: 'changed' },
  ]);
  assert.deepEqual(changeSet({ files: {}, runtime: {} }, snap({})), []);
});

test('adopt: no deferrals first, then smallest change set, then primary, then order', () => {
  const s = snap({ 'a.ts': f('2'), 'b.ts': f('1') });
  const cand = (label, primary, files, deferrals = []) => ({ label, primary, usable: true, attestation: { files, runtime: {}, deferrals } });
  const near = { 'a.ts': f('2'), 'b.ts': f('1') };
  const far = { 'a.ts': f('1'), 'b.ts': f('0') };
  assert.equal(adopt([cand('p', true, near, [{ reason: 'x', paths: ['*'] }]), cand('full', false, far)], s).label, 'full');
  assert.equal(adopt([cand('p', true, far), cand('inc', false, near)], s).label, 'inc');
  assert.equal(adopt([cand('p', true, near), cand('inc', false, near)], s).label, 'p');
  assert.equal(adopt([cand('a', false, near), cand('b', false, near)], s).label, 'a');
  assert.deepEqual(adopt([cand('p', true, near)], s).changes, []);
  assert.equal(adopt([{ label: 'u', usable: false }], s), null);
  assert.equal(adopt([], s), null);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/state.test.mjs`
Expected: FAIL — missing module.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/state.mjs`:

```js
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { sha256 } from './snapshot.mjs';

export const TOOL = 'metareview-mutation-incremental';
export const TOOL_VERSION = '0.13.0';
// Spec §11.6: bumped only when planning or attestation semantics change; usability checks this,
// not toolVersion, so patch releases keep state.
export const STATE_VERSION = 1;
export const ATTESTATION_FILE = 'attestation.json';
export const REPORT_FILE = 'incremental.json';

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

export function readCandidate(dir, { label, primary }) {
  const out = { label, dir, primary, usable: false, reason: '', attestation: null, report: null };
  let text;
  try {
    text = readFileSync(join(dir, ATTESTATION_FILE), 'utf8');
  } catch {
    out.reason = 'missing attestation';
    return out;
  }
  let att;
  try {
    att = JSON.parse(text);
  } catch {
    att = null;
  }
  if (!isObject(att)) {
    out.reason = 'unparseable attestation';
    return out;
  }
  out.attestation = att;
  if (att.schemaVersion !== 1 || att.tool !== TOOL || !isObject(att.files) || !isObject(att.runtime) || !Array.isArray(att.deferrals)) {
    out.reason = 'not a harness attestation';
    return out;
  }
  if (att.stateVersion !== STATE_VERSION) {
    out.reason = 'stateVersion mismatch';
    return out;
  }
  let bytes;
  try {
    bytes = readFileSync(join(dir, REPORT_FILE));
  } catch {
    out.reason = 'missing report';
    return out;
  }
  if (sha256(bytes) !== att.reportSha256) {
    out.reason = 'report hash mismatch';
    return out;
  }
  try {
    out.report = JSON.parse(bytes.toString('utf8'));
  } catch {
    out.reason = 'unparseable report';
    return out;
  }
  out.usable = true;
  return out;
}

const byPathThenCategory = (a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : a.category < b.category ? -1 : a.category > b.category ? 1 : 0);

// Spec §5.4 Change set. A path whose category changed appears under both categories: as deleted
// under the old one and new under the current one.
export function changeSet(attestation, snapshot) {
  const out = [];
  const old = attestation.files;
  for (const [path, info] of Object.entries(snapshot.files)) {
    const prev = old[path];
    if (!prev) out.push({ path, category: info.category, kind: 'new' });
    else if (prev.category !== info.category) {
      out.push({ path, category: prev.category, kind: 'deleted' });
      out.push({ path, category: info.category, kind: 'new' });
    } else if (prev.digest !== info.digest) out.push({ path, category: info.category, kind: 'changed' });
  }
  for (const [path, prev] of Object.entries(old)) if (!snapshot.files[path]) out.push({ path, category: prev.category, kind: 'deleted' });
  const oldRuntime = attestation.runtime;
  const keys = new Set([...Object.keys(oldRuntime), ...Object.keys(snapshot.runtime)]);
  for (const key of keys) {
    if (!(key in oldRuntime)) out.push({ path: key, category: 'global', kind: 'new' });
    else if (!(key in snapshot.runtime)) out.push({ path: key, category: 'global', kind: 'deleted' });
    else if (oldRuntime[key] !== snapshot.runtime[key]) out.push({ path: key, category: 'global', kind: 'changed' });
  }
  return out.sort(byPathThenCategory);
}

// Spec §5.4 Adopt: change-based, never time-based.
export function adopt(candidates, snapshot) {
  const ranked = candidates
    .map((c, order) => ({ c, order }))
    .filter(({ c }) => c.usable)
    .map((r) => ({ ...r, changes: changeSet(r.c.attestation, snapshot), deferred: r.c.attestation.deferrals.length > 0 ? 1 : 0 }))
    .sort((x, y) => x.deferred - y.deferred || x.changes.length - y.changes.length || Number(y.c.primary) - Number(x.c.primary) || x.order - y.order);
  return ranked.length === 0 ? null : { ...ranked[0].c, changes: ranked[0].changes };
}
```

- [ ] **Step 4: Run to verify pass**

Run: `node --test templates/mutation-incremental/test/state.test.mjs`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): state reading, change set and change-based adoption

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: The planner (spec §5.4 steps and Normalise, §11.4, §11.5)

**Files:**
- Create: `templates/mutation-incremental/lib/deferrals.mjs` (only the canonical/dedupe/sort part; Task 8 adds classification)
- Create: `templates/mutation-incremental/lib/plan.mjs`
- Create: `templates/mutation-incremental/test/plan.test.mjs`

**Interfaces:**
- Consumes: `indexReport` (Task 3); `importerTests`, `openImporters` and `reachesFrom` (Task 4); `lineDiff` and `hunkIntersects` (Task 5); `changeSet`, `TOOL` and `STATE_VERSION` (Task 6); `UsageError`.
- Produces:
  - `canonicalDeferral(d)`, `deferralKey(d)`, `dedupeDeferrals(prev, next)`, `sortDeferrals(list)`;
  - `RESIDUAL_MAX_BYTES`;
  - `computePlan({config, snapshot, graph, chosen, disableResidual?, unbudgeted?, readText?}) → Plan`, where `Plan` is:
    ```
    { baseline, cold, pendingFull, scope: string[], forced: string[], forcedCount,
      deferrals: {reason,paths}[], added: {reason,paths}[], changes, openImporters,
      unclassified: string[], invocation1: string[], invocation2: string[] }
    ```
  - `publicPlan(plan)` returns the §5.4 Plan JSON keys only.
  - `chosen` is `adopt(...)`'s result, or `null` when cold.
  - `config` needs `top`, `editedFiles`, `residualMode` and `budget`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/plan.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { computePlan, publicPlan, RESIDUAL_MAX_BYTES } from '../lib/plan.mjs';
import { changeSet, TOOL, STATE_VERSION } from '../lib/state.mjs';
import { dedupeDeferrals, sortDeferrals, deferralKey } from '../lib/deferrals.mjs';
import { MAX_EDIT_DISTANCE } from '../lib/diff.mjs';

// The spec §7.1 fixture in miniature (same line numbers), plus src/e.ts from §11.7.
const A = 'export function clamp(n, lo, hi) {\n  if (n < lo) return lo;\n  if (n > hi) return hi;\n  return n;\n}\n';
const B = "import { clamp } from './a';\nimport type { Range } from './types';\n\nexport const inRange = (n: number, r: Range) => clamp(n, r.lo, r.hi);\n";
const C = "import { LIMIT } from './limits';\nexport const twice = (n: number) => Math.min(n * 2, LIMIT);\n";
const E = 'export function inc(n) {\n  const r = n + 1;\n  return r;\n}\n\nexport function dec(n) {\n  const r = n - 1;\n  return r;\n}\n';
const TEXTS = {
  'src/a.ts': A, 'src/b.ts': B, 'src/c.ts': C, 'src/e.ts': E,
  'src/limits.ts': 'export const LIMIT = 100;\n',
  'src/types.ts': 'export interface Range { lo: number; hi: number }\n',
  'src/index.ts': "export * from './a';\n",
};
const CATEGORIES = {
  'src/a.ts': 'mutate', 'src/b.ts': 'mutate', 'src/c.ts': 'mutate', 'src/e.ts': 'mutate',
  'src/limits.ts': 'mutate', 'src/types.ts': 'mutate', 'src/index.ts': 'mutate',
  'tests/a.test.ts': 'test', 'tests/b.test.ts': 'test', 'tests/c.test.ts': 'test', 'tests/index.test.ts': 'test',
  'tests/e-inc.test.ts': 'test', 'tests/e-dec.test.ts': 'test',
  'tests/helpers/make.ts': 'support', 'tests/helpers/fmt.ts': 'support',
  'package-lock.json': 'global',
};
const EDGES = {
  'tests/a.test.ts': ['src/a.ts'],
  'tests/b.test.ts': ['src/b.ts', 'tests/helpers/make.ts'],
  'src/b.ts': ['src/a.ts'],
  'tests/c.test.ts': ['src/c.ts', 'tests/helpers/fmt.ts'],
  'src/c.ts': ['src/limits.ts'],
  'tests/index.test.ts': ['src/index.ts'],
  'src/index.ts': ['src/a.ts'],
  'tests/e-inc.test.ts': ['src/e.ts'],
  'tests/e-dec.test.ts': ['src/e.ts'],
};
const mk = (id, start, end, status, killedBy, coveredBy) => ({ id, status, killedBy, coveredBy, mutatorName: 'M', replacement: 'r', location: { start: { line: start, column: 1 }, end: { line: end, column: 2 } } });
function report() {
  return {
    files: {
      'src/a.ts': { source: A, mutants: [mk('a1', 1, 5, 'Killed', ['ta', 'ti'], ['ta', 'tb', 'ti']), mk('a2', 2, 2, 'Killed', ['ta', 'tb'], ['ta', 'tb', 'ti']), mk('a3', 3, 3, 'Killed', ['ta'], ['ta', 'tb'])] },
      'src/b.ts': { source: B, mutants: [mk('b1', 4, 4, 'Killed', ['tb'], ['tb'])] },
      'src/c.ts': { source: C, mutants: [mk('c1', 2, 2, 'Killed', ['tc'], ['tc'])] },
      'src/e.ts': { source: E, mutants: [mk('e1', 1, 4, 'Killed', ['tei'], ['tei']), mk('e2', 2, 2, 'Killed', ['tei'], ['tei']), mk('e3', 6, 9, 'Killed', ['ted'], ['ted']), mk('e4', 7, 7, 'Killed', ['ted'], ['ted'])] },
    },
    testFiles: {
      'tests/a.test.ts': { tests: [{ id: 'ta' }] }, 'tests/b.test.ts': { tests: [{ id: 'tb' }] }, 'tests/c.test.ts': { tests: [{ id: 'tc' }] },
      'tests/index.test.ts': { tests: [{ id: 'ti' }] }, 'tests/e-inc.test.ts': { tests: [{ id: 'tei' }] }, 'tests/e-dec.test.ts': { tests: [{ id: 'ted' }] },
    },
  };
}
function graphOf(edges, open) {
  const forward = new Map();
  const reverse = new Map();
  for (const [from, tos] of Object.entries(edges)) {
    for (const to of tos) {
      if (!forward.has(from)) forward.set(from, new Set());
      forward.get(from).add(to);
      if (!reverse.has(to)) reverse.set(to, new Set());
      reverse.get(to).add(from);
    }
  }
  return { forward, reverse, open: new Map(Object.entries(open)) };
}
// edit: {path: newText}; remove: [path]; add: {path: {category, text?, digest?}} (new since the state);
// base: {path: {category, text?}} (present in the state and unchanged unless also edited).
function scenario({ edit = {}, remove = [], add = {}, base = {}, config = {}, edges = {}, open = {}, deferrals = [], mutate } = {}) {
  const texts = { ...TEXTS };
  const baseFiles = {};
  for (const [p, category] of Object.entries(CATEGORIES)) baseFiles[p] = { digest: `sha256:${p}`, category, tracked: true };
  for (const [p, { category, text = '' }] of Object.entries(base)) {
    texts[p] = text;
    baseFiles[p] = { digest: `sha256:${p}`, category, tracked: true };
  }
  const files = structuredClone(baseFiles);
  for (const [p, text] of Object.entries(edit)) {
    texts[p] = text;
    files[p] = { ...files[p], digest: `sha256:${p}:edited` };
  }
  for (const p of remove) delete files[p];
  for (const [p, { category, text = '', digest = `sha256:${p}:new` }] of Object.entries(add)) {
    texts[p] = text;
    files[p] = { digest, category, tracked: true };
  }
  const attestation = { schemaVersion: 1, tool: TOOL, stateVersion: STATE_VERSION, files: baseFiles, runtime: {}, deferrals };
  const snapshot = { files, runtime: {} };
  const rep = report();
  if (mutate) mutate(rep);
  return {
    config: { top: '/unused', editedFiles: 'whole', residualMode: 'bounded', budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 5 }, ...config },
    snapshot,
    graph: graphOf({ ...EDGES, ...edges }, open),
    chosen: { label: '.mutation', report: rep, attestation, changes: changeSet(attestation, snapshot) },
    readText: (f) => texts[f],
  };
}
const plan = (opts, extra = {}) => computePlan({ ...scenario(opts), ...extra });
const residual = { editedFiles: 'residual' };
const A3 = A.replace('n > hi', 'n >= hi');

test('whole: behaviour-preserving edit on a.ts line 3 forces the file and the residual', () => {
  const p = plan({ edit: { 'src/a.ts': A3 } });
  assert.deepEqual([p.scope, p.forced, p.deferrals, p.pendingFull], [[], ['src/a.ts', 'src/b.ts:4-4'], [], false]);
  assert.deepEqual([p.invocation1, p.invocation2], [[], ['src/a.ts', 'src/b.ts:4-4']]);
  assert.equal(p.baseline, '.mutation');
  assert.equal(p.forcedCount, 1);
});

test('residual: a.ts line 3 keeps the file in scope, forces the changed line and the mapped closure', () => {
  const p = plan({ edit: { 'src/a.ts': A3 }, config: residual });
  // a2 (line 2, closure) and the changed line 3 merge; forcedCount counts closure mutants only.
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:2-3', 'src/b.ts:4-4'], 2]);
});

test('residual: a block mutant opening on an edited line is forced', () => {
  // a1 (lines 1–5) starts on the edited line 1: its own text may be unchanged, so it is forced.
  const p = plan({ edit: { 'src/a.ts': A.replace('clamp(n, lo, hi)', 'clamp(n, lo, max)') }, config: residual });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:1-5', 'src/b.ts:4-4'], 3]);
});

test('residual: a block mutant closing on an edited line is forced', () => {
  const p = plan({ edit: { 'src/a.ts': A.replace('  return n;\n}\n', '  return n;\n};\n') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:1-5', 'src/b.ts:4-4']]);
});

test('residual: a line inserted inside a block is re-run by the scope, the rest is mapped', () => {
  const p = plan({ edit: { 'src/a.ts': A.replace('  if (n > hi)', '  n = n;\n  if (n > hi)') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:2-4', 'src/b.ts:4-4']]);
});

test('residual: support forcing skips mutants on the changed hunk of a residual-edited file', () => {
  const p = plan({ edit: { 'src/a.ts': A3, 'tests/helpers/make.ts': 'x' }, config: residual });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [['src/a.ts'], ['src/a.ts:2-3', 'src/b.ts:4-4'], 2]);
});

test('residual: e.ts closure is narrower than the file', () => {
  const p = plan({ edit: { 'src/e.ts': E.replace('n + 1', 'n + 2') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/e.ts'], ['src/e.ts:2-2']]);
});

test('residual: a pure deletion forces no new-side lines, only the closure', () => {
  const p = plan({ edit: { 'src/e.ts': E.replace('  return r;\n}\n\nexport function dec', '}\n\nexport function dec') }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/e.ts'], ['src/e.ts:2-2']]);
});

test('residual: inserted line shifts mapped ranges; a hunk outside every mutant adds importer tests', () => {
  const p = plan({ edit: { 'src/a.ts': `// note\n${A}` }, config: residual });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:1-6', 'src/b.ts:4-4']]);
});

test('residual: a capped diff is handled like the whole-file fallback', () => {
  const many = Array.from({ length: MAX_EDIT_DISTANCE + 1 }, (_, i) => `// ${i}`).join('\n');
  const p = plan({ edit: { 'src/a.ts': `${A}${many}\n` }, config: residual });
  assert.deepEqual([p.scope, p.forced], [[], ['src/a.ts', 'src/b.ts:4-4']]);
});

test('residual with residual.mode off records only the closure it skipped', () => {
  const p = plan({ edit: { 'src/a.ts': A3 }, config: { ...residual, residualMode: 'off' } });
  assert.deepEqual([p.scope, p.forced], [['src/a.ts'], ['src/a.ts:3-3']]);
  assert.deepEqual(p.deferrals, [{ reason: 'residual off: src/a.ts', paths: ['src/a.ts', 'src/b.ts'] }]);
});

test('whole and residual: a hunk outside every mutant adds the file importer tests (K4)', () => {
  const edges = { 'tests/c.test.ts': ['src/c.ts', 'tests/helpers/fmt.ts', 'src/a.ts'] };
  const appended = `${A}export const extra = 1;\n`;
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3 }, edges }).forced, ['src/a.ts', 'src/b.ts:4-4']);
  assert.deepEqual(plan({ edit: { 'src/a.ts': appended }, edges }).forced, ['src/a.ts', 'src/b.ts:4-4', 'src/c.ts:2-2']);
  const r = plan({ edit: { 'src/a.ts': appended }, edges, config: residual });
  assert.deepEqual([r.scope, r.forced], [['src/a.ts'], ['src/a.ts:1-6', 'src/b.ts:4-4', 'src/c.ts:2-2']]);
});

test('residual falls back to whole above RESIDUAL_MAX_BYTES', () => {
  const s = scenario({ edit: { 'src/a.ts': A3 }, config: residual });
  const p = computePlan({ ...s, readText: (f) => (f === 'src/a.ts' ? 'x'.repeat(RESIDUAL_MAX_BYTES + 1) : '') });
  assert.equal(p.forced[0], 'src/a.ts');
  assert.deepEqual(p.scope, []);
});

test('zero-mutant module: importer tests drive the residual (limits.ts, index.ts)', () => {
  assert.deepEqual(plan({ edit: { 'src/limits.ts': 'export const LIMIT = 50;\n' } }).forced, ['src/c.ts:2-2', 'src/limits.ts']);
  assert.deepEqual(plan({ edit: { 'src/index.ts': "export * from './a.js';\n" }, config: residual }).forced, ['src/a.ts:1-5', 'src/index.ts']);
  // A new test importing the module has no ids in the report yet; it adds nothing to T_Y.
  const withNewTest = plan({ edit: { 'src/limits.ts': 'export const LIMIT = 50;\n' }, add: { 'tests/l.test.ts': { category: 'test' } }, edges: { 'tests/l.test.ts': ['src/limits.ts'] } });
  assert.deepEqual(withNewTest.forced, ['src/c.ts:2-2', 'src/limits.ts']);
});

test('support forcing skips a file that is already edited whole', () => {
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3, 'tests/helpers/make.ts': 'x' } }).forced, ['src/a.ts', 'src/b.ts:4-4']);
});

test('forced spans merge per file and sort by start, then end', () => {
  const p = plan({
    edit: { 'tests/helpers/make.ts': 'x' },
    mutate: (r) => { r.files['src/e.ts'].mutants = [mk('x1', 2, 2, 'Killed', ['tb'], ['tb']), mk('x2', 7, 9, 'Killed', ['tb'], ['tb']), mk('x3', 7, 7, 'Killed', ['tb'], ['tb'])]; },
  });
  assert.deepEqual(p.forced, ['src/a.ts:1-5', 'src/b.ts:4-4', 'src/e.ts:2-2', 'src/e.ts:7-9']);
});

test('edit tests/b.test.ts scopes the files its tests cover', () => {
  assert.deepEqual(plan({ edit: { 'tests/b.test.ts': 'x' } }).scope, ['src/a.ts', 'src/b.ts']);
});

test('support edit forces its importer tests mutants as merged ranges', () => {
  const p = plan({ edit: { 'tests/helpers/make.ts': 'x' } });
  assert.deepEqual([p.forced, p.forcedCount], [['src/a.ts:1-5', 'src/b.ts:4-4'], 4]);
});

test('budget overflow drops forced entries and returns moved files to scope', () => {
  const budget = { maxForcedShare: 1, maxForcedMutants: 1, maxMinutesPerInvocation: 5 };
  const p = plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget } });
  assert.deepEqual([p.forced, p.scope, p.forcedCount], [[], [], 4]);
  assert.deepEqual(p.deferrals, [{ reason: 'forced set 4 exceeds budget 1', paths: ['src/a.ts', 'src/b.ts'] }]);
  const both = plan({ edit: { 'tests/helpers/make.ts': 'x', 'tests/b.test.ts': 'y' }, config: { budget } });
  assert.deepEqual([both.forced, both.scope], [[], ['src/a.ts', 'src/b.ts']]);
  const share = plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget: { ...budget, maxForcedShare: 0.25, maxForcedMutants: 100 } } });
  assert.equal(share.deferrals[0].reason, 'forced set 4 exceeds budget 2');
});

test('strict and unbudgeted runs ignore the budget', () => {
  const budget = { maxForcedShare: 1, maxForcedMutants: 1, maxMinutesPerInvocation: 5 };
  assert.deepEqual(plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget, residualMode: 'strict' } }).forced, ['src/a.ts:1-5', 'src/b.ts:4-4']);
  assert.deepEqual(plan({ edit: { 'tests/helpers/make.ts': 'x' }, config: { budget } }, { unbudgeted: true }).forced, ['src/a.ts:1-5', 'src/b.ts:4-4']);
});

test('delete b.ts and b.test.ts together', () => {
  const p = plan({ remove: ['src/b.ts', 'tests/b.test.ts'] });
  assert.deepEqual([p.scope, p.forced, p.forcedCount], [[], ['src/a.ts'], 3]);
});

test('git mv c.ts to d.ts with its test updated', () => {
  const p = plan({
    remove: ['src/c.ts'],
    add: { 'src/d.ts': { category: 'mutate', text: C } },
    edit: { 'tests/c.test.ts': 'x' },
    edges: { 'tests/c.test.ts': ['src/d.ts', 'tests/helpers/fmt.ts'], 'src/d.ts': ['src/limits.ts'] },
  });
  assert.deepEqual([p.forced, p.scope], [['src/d.ts'], ['src/limits.ts']]);
});

test('residual off records what it would have forced; disableResidual forces nothing', () => {
  const off = plan({ edit: { 'src/a.ts': A3 }, config: { residualMode: 'off' } });
  assert.deepEqual([off.forced, off.deferrals], [['src/a.ts'], [{ reason: 'residual off: src/a.ts', paths: ['src/b.ts'] }]]);
  assert.deepEqual(plan({ edit: { 'src/a.ts': A3 } }, { disableResidual: true }).forced, ['src/a.ts']);
});

test('cold plan', () => {
  const p = computePlan({ ...scenario(), chosen: null });
  assert.deepEqual(publicPlan(p), {
    baseline: null, cold: true, pendingFull: true, scope: [], forced: [], forcedCount: 0,
    deferrals: [{ reason: 'no usable state', paths: ['*'] }], changes: [], openImporters: [], unclassified: [],
  });
});

test('global, support and unclassified deferrals', () => {
  const lock = plan({ edit: { 'package-lock.json': 'x' } });
  assert.deepEqual([lock.deferrals, lock.pendingFull], [[{ reason: 'global input changed: package-lock.json', paths: ['*'] }], true]);
  assert.deepEqual(plan({ add: { 'tests/helpers/orphan.ts': { category: 'support' } } }).deferrals, [{ reason: 'support tests/helpers/orphan.ts has no resolvable importer', paths: ['*'] }]);
  assert.deepEqual(plan({ remove: ['tests/helpers/fmt.ts'] }).deferrals, [{ reason: 'support tests/helpers/fmt.ts deleted', paths: ['*'] }]);
  const orphan = plan({ add: { 'scripts/x.sh': { category: 'unclassified' } } });
  assert.deepEqual([orphan.deferrals, orphan.unclassified], [[{ reason: 'unclassified scripts/x.sh has no resolvable importer', paths: ['*'] }], ['scripts/x.sh']]);
  assert.deepEqual(plan({ base: { 'scripts/y.sh': { category: 'unclassified' } }, remove: ['scripts/y.sh'] }).deferrals, []);
});

test('unclassified input with an importer forces the kills of its importer tests', () => {
  const opts = { base: { 'fixtures/x.json': { category: 'unclassified' } }, edit: { 'fixtures/x.json': '{}' }, edges: { 'tests/a.test.ts': ['src/a.ts', 'fixtures/x.json'] } };
  const p = plan(opts);
  assert.deepEqual([p.forced, p.unclassified], [['src/a.ts:1-5'], ['fixtures/x.json']]);
  assert.deepEqual(plan({ ...opts, config: { residualMode: 'off' } }).deferrals, [{ reason: 'residual off: fixtures/x.json', paths: ['src/a.ts'] }]);
});

test('a test that newly reaches an uncovered module scopes it', () => {
  const p = plan({ base: { 'src/u.ts': { category: 'mutate' } }, add: { 'tests/u.test.ts': { category: 'test' } }, edges: { 'tests/u.test.ts': ['src/u.ts'] } });
  assert.deepEqual(p.scope, ['src/u.ts']);
});

test('a new module arriving with its first test is forced whole; its new test has no ids yet', () => {
  const p = plan({ add: { 'src/v.ts': { category: 'mutate' }, 'tests/v.test.ts': { category: 'test' } }, edges: { 'tests/v.test.ts': ['src/v.ts'] } });
  assert.deepEqual([p.forced, p.scope], [['src/v.ts'], []]);
});

test('a test that reaches an open importer scopes every uncovered or weak file', () => {
  const p = plan({ add: { 'tests/w.test.ts': { category: 'test' } }, open: { 'tests/w.test.ts': ['@x/y'] } });
  assert.deepEqual(p.scope, ['src/index.ts', 'src/limits.ts', 'src/types.ts']);
  assert.deepEqual(p.openImporters, [{ path: 'tests/w.test.ts', specifiers: ['@x/y'] }]);
});

test('a test reaching an open importer also scopes reported files with survivors', () => {
  const p = plan({ add: { 'tests/w.test.ts': { category: 'test' } }, open: { 'tests/w.test.ts': ['@x/y'] }, mutate: (r) => { r.files['src/c.ts'].mutants[0].status = 'Survived'; } });
  assert.deepEqual(p.scope, ['src/c.ts', 'src/index.ts', 'src/limits.ts', 'src/types.ts']);
});

test('a new case in an existing test scopes a file with a survivor', () => {
  const p = plan({ edit: { 'tests/a.test.ts': 'x' }, mutate: (r) => { r.files['src/a.ts'].mutants[2].status = 'Survived'; } });
  assert.deepEqual(p.scope, ['src/a.ts']);
});

test('prior deferrals carry into the plan; pendingFull follows them', () => {
  const prior = [{ reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] }];
  const p = plan({ deferrals: prior });
  assert.deepEqual([p.deferrals, p.pendingFull, p.added], [prior, true, []]);
});

test('a symlinked mutate file and a path Stryker cannot take', () => {
  assert.deepEqual(plan({ add: { 'src/s.ts': { category: 'mutate', digest: 'symlink:abc' } } }).forced, []);
  assert.throws(() => plan({ add: { 'src/x,y.ts': { category: 'mutate' } } }), (e) => e.exitCode === 2 && /--mutate/.test(e.message));
});

test('deferral helpers: canonical, dedupe and sort', () => {
  const d1 = { reason: 'b', paths: ['z', 'a'] };
  const d2 = { reason: 'b', paths: ['a', 'z'], inherited: true };
  const d3 = { reason: 'a', paths: ['*'] };
  assert.equal(deferralKey(d1), deferralKey(d2));
  assert.deepEqual(dedupeDeferrals([d1], [d2, d3]), [{ reason: 'b', paths: ['a', 'z'] }, { reason: 'a', paths: ['*'] }]);
  assert.deepEqual(sortDeferrals([d1, d3, { reason: 'b', paths: ['0'] }]).map((d) => d.paths[0]), ['*', '0', 'z']);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/plan.test.mjs`
Expected: FAIL — missing modules.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/deferrals.mjs` (first part; Task 8 appends to it):

```js
// Deferrals are {reason, paths}; paths are file paths or ["*"] (spec §4). Reasons and paths are
// canonical (fixed formats, byte-ordered paths) so equality comparisons are exact (spec §11.2).
const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

export const canonicalDeferral = (d) => ({ reason: d.reason, paths: [...d.paths].sort(cmp) });

export const deferralKey = (d) => `${d.reason}\u0000${[...d.paths].sort(cmp).join('\u0001')}`;

// Previous then new, de-duplicated by reason and sorted paths (spec §5.5).
export function dedupeDeferrals(prev, next) {
  const seen = new Set();
  const out = [];
  for (const d of [...prev, ...next]) {
    const key = deferralKey(d);
    if (!seen.has(key)) {
      seen.add(key);
      out.push(canonicalDeferral(d));
    }
  }
  return out;
}

// Plan JSON order (spec §5.4): by reason, then first path.
export const sortDeferrals = (list) => [...list].sort((a, b) => cmp(a.reason, b.reason) || cmp(a.paths[0], b.paths[0]));
```

`templates/mutation-incremental/lib/plan.mjs`:

```js
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { indexReport } from './report.mjs';
import { importerTests, openImporters, reachesFrom } from './graph.mjs';
import { lineDiff, hunkIntersects } from './diff.mjs';
import { canonicalDeferral, dedupeDeferrals, sortDeferrals } from './deferrals.mjs';

// Spec §11.4: larger edited files use the whole-file rule.
export const RESIDUAL_MAX_BYTES = 1 << 20;
const BAD_PATH = /[,{}[\]()!*?:\n]/;
const WEAK = new Set(['Survived', 'NoCoverage']);
const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

const PUBLIC_KEYS = ['baseline', 'cold', 'pendingFull', 'scope', 'forced', 'forcedCount', 'deferrals', 'changes', 'openImporters', 'unclassified'];
export const publicPlan = (plan) => Object.fromEntries(PUBLIC_KEYS.map((k) => [k, plan[k]]));

function coldPlan(open) {
  const deferrals = [{ reason: 'no usable state', paths: ['*'] }];
  return { baseline: null, cold: true, pendingFull: true, scope: [], forced: [], forcedCount: 0, deferrals, added: deferrals, changes: [], openImporters: open, unclassified: [], invocation1: [], invocation2: [] };
}

export function computePlan({ config, snapshot, graph, chosen, disableResidual = false, unbudgeted = false, readText }) {
  const read = readText ?? ((f) => readFileSync(join(config.top, f), 'utf8'));
  const open = openImporters(graph);
  if (!chosen) return coldPlan(open);

  const reportFiles = chosen.report.files;
  const R = indexReport(chosen.report);
  const byFile = new Map();
  for (const m of R.mutants) {
    if (!byFile.has(m.file)) byFile.set(m.file, []);
    byFile.get(m.file).push(m);
  }
  const own = (f) => byFile.get(f) ?? [];
  const inR = (f) => Object.hasOwn(reportFiles, f);
  // A path Stryker may be given: a regular file still in the mutate category (deleted, symlinked or
  // re-categorised files drop out, spec §5.4 Normalise).
  const present = (f) => snapshot.files[f]?.category === 'mutate' && snapshot.files[f].digest.startsWith('sha256:');
  const hits = (ids, set) => ids.some((id) => set.has(id));
  const idsOf = (tests) => new Set([...tests].flatMap((t) => R.testIds[t] ?? []));
  const importerIds = (p) => idsOf(importerTests(graph, snapshot.files, p));
  const changes = chosen.changes;

  const edited = new Set();           // forced whole-file (whole mode, or residual fallback)
  const residualEdited = new Map();   // file -> lineDiff result (unforced scope + mapped closure)
  const diffs = new Map();            // every changed mutate file with an attested source
  const scope = new Set();
  const forced = new Map();           // file -> Map(mutant id -> [start, end] in current lines)
  const added = [];

  // Step 1 (spec §5.4, §11.4): edited mutate files.
  for (const c of changes) {
    if (c.category !== 'mutate' || c.kind === 'deleted') continue;
    let d = null;
    if (inR(c.path) && present(c.path)) {
      const text = read(c.path);
      // Stryker embeds every file's source in the report (spec F3). null above the size or edit cap.
      if (Buffer.byteLength(text) <= RESIDUAL_MAX_BYTES) d = lineDiff(reportFiles[c.path].source, text);
    }
    if (d) diffs.set(c.path, d);
    if (config.editedFiles === 'residual' && d) residualEdited.set(c.path, d);
    else edited.add(c.path);
  }

  const addForced = (m) => {
    if (edited.has(m.file)) return;
    let start = m.startLine;
    let end = m.endLine;
    const d = residualEdited.get(m.file);
    if (d) {
      // Mutants touching a changed hunk are rebuilt and re-run by the unforced scope invocation.
      if (d.hunks.some((h) => hunkIntersects(start, end, h))) return;
      start = d.oldToNew[start];
      end = d.oldToNew[end];
    }
    if (!forced.has(m.file)) forced.set(m.file, new Map());
    forced.get(m.file).set(m.id, [start, end]);
  };

  // Step 2: tests.
  const weak = (f) => !inR(f) || own(f).some((m) => WEAK.has(m.status));
  const mutateFiles = Object.keys(snapshot.files).filter((f) => snapshot.files[f].category === 'mutate');
  for (const c of changes) {
    if (c.category !== 'test') continue;
    if (c.kind !== 'new' && R.testIds[c.path]) {
      const ids = new Set(R.testIds[c.path]);
      for (const m of R.mutants) if (hits(m.killedBy, ids) || hits(m.coveredBy, ids)) scope.add(m.file);
    }
    if (c.kind === 'deleted') continue;
    const reached = reachesFrom(graph, c.path);
    for (const f of reached) if (snapshot.files[f].category === 'mutate' && weak(f)) scope.add(f);
    if ([...reached].some((f) => graph.open.has(f))) {
      for (const f of mutateFiles) if (!inR(f)) scope.add(f);
      for (const f of byFile.keys()) if (weak(f)) scope.add(f);
    }
  }

  // Step 3: support.
  for (const c of changes) {
    if (c.category !== 'support') continue;
    if (c.kind === 'deleted') {
      added.push({ reason: `support ${c.path} deleted`, paths: ['*'] });
      continue;
    }
    const tests = importerTests(graph, snapshot.files, c.path);
    if (tests.size === 0) {
      added.push({ reason: `support ${c.path} has no resolvable importer`, paths: ['*'] });
      continue;
    }
    const ids = idsOf(tests);
    for (const m of R.mutants) if (hits(m.killedBy, ids) || hits(m.coveredBy, ids)) addForced(m);
  }

  // Step 4: residual (spec §5.4 step 4, §11.4.3, §11.5).
  const killedBy = (T, skip) => R.mutants.filter((m) => m.status === 'Killed' && m.file !== skip && hits(m.killedBy, T));
  // Mutants touching a changed hunk of a residual-edited file are re-run by the unforced scope and
  // the forced changed lines, so they are neither forced by the closure nor recorded under "off".
  const rerunInScope = (m) => residualEdited.get(m.file)?.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h)) === true;
  const applyResidual = (origin, targets) => {
    const live = targets.filter((m) => !rerunInScope(m));
    if (disableResidual || live.length === 0) return;
    if (config.residualMode === 'off') {
      added.push({ reason: `residual off: ${origin}`, paths: [...new Set(live.map((m) => m.file))] });
      return;
    }
    for (const m of live) addForced(m);
  };
  for (const c of changes) {
    if (c.category === 'mutate') {
      const Y = c.path;
      const mine = own(Y);
      const d = diffs.get(Y);
      const touching = residualEdited.has(Y) ? mine.filter((m) => d.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h))) : mine;
      const T = new Set(touching.flatMap((m) => m.coveredBy));
      const outsideEveryMutant = mine.length === 0 || !d || d.hunks.some((h) => !mine.some((m) => hunkIntersects(m.startLine, m.endLine, h)));
      // Spec §5.4 step 4 / §11.5: only a *changed* Y falls back to (or adds) its importer tests.
      if (c.kind === 'changed' && outsideEveryMutant) for (const id of importerIds(Y)) T.add(id);
      applyResidual(Y, killedBy(T, residualEdited.has(Y) ? null : Y));
    } else if (c.category === 'unclassified' && c.kind !== 'deleted') {
      const tests = importerTests(graph, snapshot.files, c.path);
      if (tests.size === 0) added.push({ reason: `unclassified ${c.path} has no resolvable importer`, paths: ['*'] });
      else applyResidual(c.path, killedBy(idsOf(tests), null));
    }
  }

  // Step 5: global and runtime inputs.
  for (const c of changes) if (c.category === 'global') added.push({ reason: `global input changed: ${c.path}`, paths: ['*'] });

  // Normalise.
  for (const f of [...edited]) if (!present(f)) edited.delete(f);
  for (const f of residualEdited.keys()) if (present(f)) scope.add(f);
  for (const f of [...scope]) if (!present(f) || edited.has(f)) scope.delete(f);
  for (const f of [...forced.keys()]) if (!present(f)) forced.delete(f);
  const whole = new Set();
  for (const f of forced.keys()) {
    if (scope.has(f) && !residualEdited.has(f)) {
      whole.add(f);
      scope.delete(f);
    }
  }
  const N = R.mutants.length;
  let b = Math.floor(config.budget.maxForcedShare * N);
  if (config.budget.maxForcedMutants !== null && config.budget.maxForcedMutants < b) b = config.budget.maxForcedMutants;
  const forcedIds = new Set();
  for (const [f, ms] of forced) for (const id of whole.has(f) ? own(f).map((m) => m.id) : ms.keys()) forcedIds.add(`${f}\u0000${id}`);
  const forcedCount = forcedIds.size;
  if (!unbudgeted && config.residualMode === 'bounded' && forcedCount > b) {
    added.push({ reason: `forced set ${forcedCount} exceeds budget ${b}`, paths: [...forced.keys()] });
    for (const f of whole) scope.add(f);
    forced.clear();
    whole.clear();
  }

  // Forced spans per file: the closure's mutants plus, for residual-edited files, the new-side lines
  // of every changed hunk (mandatory and budget-exempt: added after the budget check).
  const spansOf = new Map();
  for (const [f, ms] of forced) if (!whole.has(f)) spansOf.set(f, [...ms.values()]);
  // A mutant strictly enclosing a changed hunk (or containing a pure insertion) has changed text,
  // so Stryker re-runs it from the scope. Any other mutant touching a hunk may keep its own text
  // unchanged (Stryker diffs characters), so its current span is forced.
  const strictlySpans = (m, h) => h.oldEnd < h.oldStart || (m.startLine < h.oldStart && m.endLine > h.oldEnd);
  const containing = (d, line) => d.hunks.find((h) => h.oldStart <= line && line <= h.oldEnd);
  for (const [f, d] of residualEdited) {
    const spans = d.hunks.filter((h) => h.newEnd >= h.newStart).map((h) => [h.newStart, h.newEnd]);
    for (const m of own(f)) {
      if (!d.hunks.some((h) => hunkIntersects(m.startLine, m.endLine, h) && !strictlySpans(m, h))) continue;
      const start = d.oldToNew[m.startLine] ?? containing(d, m.startLine).newStart;
      spans.push([start, Math.max(start, d.oldToNew[m.endLine] ?? containing(d, m.endLine).newEnd)]);
    }
    if (spans.length > 0) spansOf.set(f, [...(spansOf.get(f) ?? []), ...spans]);
  }
  const entries = [...edited, ...whole].map((f) => ({ file: f, start: 0, text: f }));
  for (const [f, spans] of spansOf) {
    let cur = null;
    for (const [s, e] of spans.sort((x, y) => x[0] - y[0] || x[1] - y[1])) {
      if (cur && s <= cur.end + 1) cur.end = Math.max(cur.end, e);
      else {
        cur = { file: f, start: s, end: e };
        entries.push(cur);
      }
    }
  }
  const forcedStrings = entries
    .sort((x, y) => cmp(x.file, y.file) || x.start - y.start)
    .map((x) => x.text ?? `${x.file}:${x.start}-${x.end}`);
  for (const f of [...scope, ...entries.map((x) => x.file)]) {
    if (BAD_PATH.test(f)) throw new UsageError(`${JSON.stringify(f)} cannot be passed to Stryker --mutate (contains , { } [ ] ( ) ! * ? : or a newline)`);
  }

  const addedCanon = dedupeDeferrals([], added);
  const deferrals = sortDeferrals(dedupeDeferrals(chosen.attestation.deferrals.map(canonicalDeferral), addedCanon));
  const scopeList = [...scope].sort(cmp);
  return {
    baseline: chosen.label,
    cold: false,
    pendingFull: deferrals.length > 0,
    scope: scopeList,
    forced: forcedStrings,
    forcedCount,
    deferrals,
    added: sortDeferrals(addedCanon),
    changes,
    openImporters: open,
    unclassified: changes.filter((c) => c.category === 'unclassified' && c.kind !== 'deleted').map((c) => c.path).sort(cmp),
    invocation1: scopeList,
    invocation2: forcedStrings,
  };
}
```

- [ ] **Step 4: Run to verify pass**

Run: `node --test templates/mutation-incremental/test/plan.test.mjs`
Expected: PASS (34 tests). If an expected value fails, re-derive it by hand from spec §5.4/§11.4 before changing either side. Every expected value above was hand-derived from the rules and the miniature fixture.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): deterministic planner with residual and whole edited-file modes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Deferral classification, pending cause and views (spec §11.2, §11.3)

**Files:**
- Modify: `templates/mutation-incremental/lib/deferrals.mjs` (append)
- Create: `templates/mutation-incremental/lib/views.mjs`
- Create: `templates/mutation-incremental/test/deferrals.test.mjs`, `test/views.test.mjs`

**Interfaces:**
- Consumes: `deferralKey` and `canonicalDeferral` (Task 7); `matchList` (Task 1); `duplicateKey` (Task 2); `UsageError`.
- Produces:
  - `reasonInfo(reason) → {cause: 'global'|'unreachable'|'other'|'timeout', named: string|null}`, which throws exit 2 for an unknown reason;
  - `classifyDeferrals({deferrals, alsoAttestations, snapshot, producedKeys?, cold?}) → {reason, paths, inherited}[]`;
  - `pendingCause(classified) → {cause: 'none'|'global'|'unreachable'|'other'|'timeout', counted}`;
  - `resolveViews(config, snapshot, {run?}) → Record<name, patterns[]>|null`;
  - `viewFiles(views, snapshot) → Record<name, path[]>`;
  - `viewSummaries(report, views, classified) → Record<name, counts>|null`.
- Plan 1b calls `classifyDeferrals` twice: at plan time (for the shortcut) and after execution, with `producedKeys` set to the deferrals its invocations produced.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/deferrals.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reasonInfo, classifyDeferrals, pendingCause, deferralKey } from '../lib/deferrals.mjs';

test('reasonInfo: every canonical reason and its named key', () => {
  const cases = [
    ['global input changed: package-lock.json', 'global', 'package-lock.json'],
    ['global input changed: env:DB', 'global', 'env:DB'],
    ['support t/h.ts has no resolvable importer', 'global', 't/h.ts'],
    ['support t/h.ts deleted', 'global', 't/h.ts'],
    ['unclassified s/x.sh has no resolvable importer', 'global', 's/x.sh'],
    ['no usable state', 'global', null],
    ['seeded from r.json', 'global', null],
    ['no reachable tests: src/c.ts', 'unreachable', 'src/c.ts'],
    ['residual off: src/a.ts', 'other', 'src/a.ts'],
    ['time budget exceeded', 'timeout', null],
    ['blocked by deferred scope', 'timeout', null],
    ['forced set 4 exceeds budget 1', 'timeout', null],
  ];
  for (const [reason, cause, named] of cases) assert.deepEqual(reasonInfo(reason), { cause, named }, reason);
  assert.throws(() => reasonInfo('mystery'), (e) => e.exitCode === 2 && /unknown deferral reason/.test(e.message));
});

const snapshot = { files: { 'package-lock.json': { digest: 'sha256:new' }, 'src/c.ts': { digest: 'sha256:c' } }, runtime: { 'env:DB': 'd1' } };
const main = (deferrals, files = { 'package-lock.json': { digest: 'sha256:new' }, 'src/c.ts': { digest: 'sha256:c' } }, runtime = { 'env:DB': 'd1' }) => ({ deferrals, files, runtime });
const lock = { reason: 'global input changed: package-lock.json', paths: ['*'] };

test('classify: identical deferral with unchanged inputs is inherited', () => {
  const [d] = classifyDeferrals({ deferrals: [lock], alsoAttestations: [main([lock])], snapshot });
  assert.equal(d.inherited, true);
});

test('classify: identical deferral with a changed digest is counted', () => {
  const [d] = classifyDeferrals({ deferrals: [lock], alsoAttestations: [main([lock], { 'package-lock.json': { digest: 'sha256:old' } })], snapshot });
  assert.equal(d.inherited, false);
});

test('classify: runtime keys, absent on both sides, nameless reasons', () => {
  const env = { reason: 'global input changed: env:DB', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [main([env])], snapshot })[0].inherited, true);
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [main([env], undefined, { 'env:DB': 'd0' })], snapshot })[0].inherited, false);
  const gone = { reason: 'support t/gone.ts deleted', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [gone], alsoAttestations: [main([gone])], snapshot })[0].inherited, true);
  const nus = { reason: 'no usable state', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [nus], alsoAttestations: [main([nus], {}, {})], snapshot })[0].inherited, true);
  const perFile = { reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] };
  assert.equal(classifyDeferrals({ deferrals: [perFile], alsoAttestations: [main([perFile], { 'src/c.ts': { digest: 'sha256:other' } })], snapshot })[0].inherited, false);
  // An unusable main attestation may lack files or runtime: its digests read as absent.
  assert.equal(classifyDeferrals({ deferrals: [perFile], alsoAttestations: [{ deferrals: [perFile] }], snapshot })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [{ deferrals: [env] }], snapshot })[0].inherited, false);
});

test('classify: cold no-usable-state and deferrals produced by this run are always counted', () => {
  const nus = { reason: 'no usable state', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [nus], alsoAttestations: [main([nus])], snapshot, cold: true })[0].inherited, false);
  const tb = { reason: 'time budget exceeded', paths: ['src/c.ts'] };
  const produced = new Set([deferralKey(tb)]);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [main([tb])], snapshot, producedKeys: produced })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [main([tb])], snapshot })[0].inherited, true);
  assert.equal(classifyDeferrals({ deferrals: [tb], snapshot })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [{}], snapshot })[0].inherited, false);
});

test('pendingCause precedence: global > unreachable > other > timeout > none', () => {
  const c = (reason, paths, inherited = false) => ({ reason, paths, inherited });
  const cause = (...ds) => pendingCause(ds).cause;
  assert.equal(cause(), 'none');
  assert.equal(cause(c('global input changed: x', ['*'], true)), 'none');
  assert.equal(cause(c('time budget exceeded', ['a']), c('no usable state', ['*'])), 'global');
  assert.equal(cause(c('time budget exceeded', ['a']), c('no reachable tests: b', ['b'])), 'unreachable');
  assert.equal(cause(c('time budget exceeded', ['a']), c('residual off: b', ['c'])), 'other');
  assert.equal(cause(c('forced set 4 exceeds budget 1', ['a'])), 'timeout');
  assert.deepEqual(pendingCause([c('blocked by deferred scope', ['a']), c('x', ['*'], true)]).counted, [c('blocked by deferred scope', ['a'])]);
  assert.throws(() => pendingCause([c('mystery', ['a'])]), (e) => e.exitCode === 2);
});
```

`templates/mutation-incremental/test/views.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolveViews, viewFiles, viewSummaries } from '../lib/views.mjs';

const snapshot = { files: { 'src/a.ts': { category: 'mutate' }, 'src/b.ts': { category: 'mutate' }, 'tests/a.test.ts': { category: 'test' } } };
const config = (views) => ({ top: '/unused', views });
const ok = (stdout) => () => ({ status: 0, stdout });

test('resolveViews: absent, inline and command', () => {
  assert.equal(resolveViews(config(null), snapshot), null);
  assert.deepEqual(resolveViews(config({ inline: { u2: ['src/b.ts'], u1: ['src/**/*.ts'] } }), snapshot), { u1: ['src/**/*.ts'], u2: ['src/b.ts'] });
  assert.deepEqual(resolveViews(config({ command: ['x'] }), snapshot, { run: ok('{"u.1":["src/*.ts"]}') }), { 'u.1': ['src/*.ts'] });
});

test('resolveViews runs a real views command', () => {
  const command = [process.execPath, '-e', 'console.log(JSON.stringify({u:["src/**"]}))'];
  assert.deepEqual(resolveViews({ top: process.cwd(), views: { command } }, snapshot), { u: ['src/**'] });
});

test('resolveViews: command failures and bad shapes are exit 2', () => {
  const bad = [
    [() => ({ status: 1, stdout: '' }), /views command .* failed/],
    [() => ({ status: null, stdout: '', error: new Error('ETIMEDOUT') }), /ETIMEDOUT/],
    [ok('nope'), /did not print JSON/],
    [ok('[]'), /name → patterns/],
    [ok('{"u":["src/a.ts"],"u":["src/b.ts"]}'), /duplicate view name "u"/],
    [ok('{"":["src/a.ts"]}'), /invalid view name/],
    [ok('{"a b":["src/a.ts"]}'), /invalid view name/],
    [ok('{"u":"src/a.ts"}'), /array of patterns/],
  ];
  for (const [run, message] of bad) {
    assert.throws(() => resolveViews(config({ command: ['x'] }), snapshot, { run }), (e) => e.exitCode === 2 && message.test(e.message), String(message));
  }
});

test('resolveViews: a view matching a non-mutate file is exit 2', () => {
  assert.throws(() => resolveViews(config({ inline: { u: ['**/*.ts'] } }), snapshot), /view u matches tests\/a.test.ts/);
});

test('resolveViews: a mutate file in no view is exit 2', () => {
  assert.throws(() => resolveViews(config({ inline: { u: ['src/a.ts'] } }), snapshot), (e) => e.exitCode === 2 && /src\/b.ts belongs to no view/.test(e.message));
});

test('viewFiles resolves each view to its files', () => {
  assert.deepEqual(viewFiles({ u1: ['src/**'], u2: ['src/b.ts'] }, snapshot), { u1: ['src/a.ts', 'src/b.ts'], u2: ['src/b.ts'] });
});

test('viewSummaries: overlapping views count a mutant in both; counted beats inherited', () => {
  const report = { files: {
    'src/a.ts': { mutants: [{ status: 'Killed' }, { status: 'Survived' }] },
    'src/b.ts': { mutants: [{ status: 'Killed' }, { status: 'Killed' }] },
    'src/c.ts': {},
  } };
  const classified = [
    { reason: 'global input changed: x', paths: ['*'], inherited: true },
    { reason: 'no reachable tests: src/b.ts', paths: ['src/b.ts'], inherited: false },
  ];
  assert.deepEqual(viewSummaries(report, { u1: ['src/**'], u2: ['src/b.ts'] }, classified), {
    u1: { Killed: 3, Survived: 1, pendingCounted: 2, pendingInherited: 1 },
    u2: { Killed: 2, pendingCounted: 2, pendingInherited: 0 },
  });
  assert.deepEqual(viewSummaries(report, { u: ['src/a.ts'] }, []), { u: { Killed: 1, Survived: 1, pendingCounted: 0, pendingInherited: 0 } });
  assert.equal(viewSummaries(report, null, classified), null);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/deferrals.test.mjs templates/mutation-incremental/test/views.test.mjs`
Expected: FAIL — missing exports and module.

- [ ] **Step 3: Implement**

Append to `templates/mutation-incremental/lib/deferrals.mjs`:

```js
import { UsageError } from './errors.mjs';

// The single table of canonical reasons (spec §11.2; review advisory "reason → named key").
// `named` is the path or runtime key the reason is about, for the inheritance digest check (ii).
const REASONS = [
  { re: /^global input changed: (.+)$/, cause: 'global', named: true },
  { re: /^support (.+) has no resolvable importer$/, cause: 'global', named: true },
  { re: /^support (.+) deleted$/, cause: 'global', named: true },
  { re: /^unclassified (.+) has no resolvable importer$/, cause: 'global', named: true },
  { re: /^no usable state$/, cause: 'global', named: false },
  { re: /^seeded from (.+)$/, cause: 'global', named: false },
  { re: /^no reachable tests: (.+)$/, cause: 'unreachable', named: true },
  { re: /^residual off: (.+)$/, cause: 'other', named: true },
  { re: /^time budget exceeded$/, cause: 'timeout', named: false },
  { re: /^blocked by deferred scope$/, cause: 'timeout', named: false },
  { re: /^forced set \d+ exceeds budget \d+$/, cause: 'timeout', named: false },
];

export function reasonInfo(reason) {
  for (const r of REASONS) {
    const m = reason.match(r.re);
    if (m) return { cause: r.cause, named: r.named ? m[1] : null };
  }
  throw new UsageError(`unknown deferral reason ${JSON.stringify(reason)} (harness bug)`);
}

const digestIn = (source, name) => (name.startsWith('cmd:') || name.startsWith('env:') ? source.runtime?.[name] : source.files?.[name]?.digest) ?? null;

function sameInputs(d, att, snapshot) {
  const { named } = reasonInfo(d.reason);
  const names = [...(named === null ? [] : [named]), ...d.paths.filter((p) => p !== '*')];
  return names.every((n) => digestIn(snapshot, n) === digestIn(att, n));
}

// Spec §11.2: a deferral is inherited only when main's published state carries the identical
// deferral (i) about inputs this run has not changed (ii). A cold run's "no usable state" and any
// deferral this run's own invocations produced are always counted.
export function classifyDeferrals({ deferrals, alsoAttestations = [], snapshot, producedKeys = new Set(), cold = false }) {
  return deferrals.map((d) => {
    const key = deferralKey(d);
    const forceCounted = producedKeys.has(key) || (cold && d.reason === 'no usable state');
    const inherited = !forceCounted && alsoAttestations.some((att) => (att.deferrals ?? []).some((x) => deferralKey(x) === key) && sameInputs(d, att, snapshot));
    return { ...canonicalDeferral(d), inherited };
  });
}

// Spec §11.2 Cause: global > unreachable > other > timeout > none, over counted deferrals only.
export function pendingCause(classified) {
  const counted = classified.filter((d) => !d.inherited);
  const infos = counted.map((d) => reasonInfo(d.reason));
  if (counted.some((d) => d.paths.length === 1 && d.paths[0] === '*')) return { cause: 'global', counted };
  const cause = ['unreachable', 'other', 'timeout'].find((c) => infos.some((i) => i.cause === c)) ?? 'none';
  return { cause, counted };
}
```

Move the new `import { UsageError } from './errors.mjs';` line to the top of `deferrals.mjs`, next to the other declarations. Imports must come before any other statement in the module.

`templates/mutation-incremental/lib/views.mjs`:

```js
import { spawnSync } from 'node:child_process';
import { UsageError } from './errors.mjs';
import { matchList } from './glob.mjs';
import { duplicateKey } from './json.mjs';

const NAME = /^[A-Za-z0-9._-]+$/;
const VIEWS_TIMEOUT_MS = 60_000; // review advisory: a hanging views command must not stall the job
const fail = (message) => {
  throw new UsageError(`views: ${message}`);
};

const defaultRun = (argv, cwd) => spawnSync(argv[0], argv.slice(1), { cwd, encoding: 'utf8', timeout: VIEWS_TIMEOUT_MS });

// Spec §11.3: views are read-time only; completeness is checked at plan time before any invocation.
export function resolveViews(config, snapshot, { run = defaultRun } = {}) {
  if (!config.views) return null;
  let map = config.views.inline;
  if (map === undefined) {
    const r = run(config.views.command, config.top);
    if (r.status !== 0) fail(`views command ${config.views.command.join(' ')} failed (exit ${r.status}${r.error ? `: ${r.error.message}` : ''})`);
    try {
      map = JSON.parse(r.stdout);
    } catch {
      fail('views command did not print JSON');
    }
    const dup = duplicateKey(r.stdout);
    if (dup !== null) fail(`duplicate view name ${JSON.stringify(dup)}`);
  }
  if (map === null || typeof map !== 'object' || Array.isArray(map)) fail('views must be an object of name → patterns');
  for (const [name, patterns] of Object.entries(map)) {
    if (!NAME.test(name)) fail(`invalid view name ${JSON.stringify(name)} (use [A-Za-z0-9._-]+)`);
    if (!Array.isArray(patterns) || !patterns.every((p) => typeof p === 'string')) fail(`view ${name} must be an array of patterns`);
  }
  const entries = Object.keys(map).sort().map((name) => [name, map[name]]);
  for (const [path, info] of Object.entries(snapshot.files)) {
    for (const [name, patterns] of entries) {
      if (info.category !== 'mutate' && matchList(path, patterns)) fail(`view ${name} matches ${path}, which is not a mutate file`);
    }
    if (info.category === 'mutate' && !entries.some(([, patterns]) => matchList(path, patterns))) {
      fail(`mutate file ${path} belongs to no view (assign it in the views source before it has mutants)`);
    }
  }
  return Object.fromEntries(entries);
}

export function viewFiles(views, snapshot) {
  const mutate = Object.keys(snapshot.files).filter((p) => snapshot.files[p].category === 'mutate').sort();
  return Object.fromEntries(Object.entries(views).map(([name, patterns]) => [name, mutate.filter((p) => matchList(p, patterns))]));
}

// Per-view counts are computed per view and never summed (spec §11.3). Pending = kills covered by
// a deferral; a kill covered by both a counted and an inherited deferral is counted (§11.2).
export function viewSummaries(report, views, classified) {
  if (!views) return null;
  const out = {};
  for (const [name, patterns] of Object.entries(views)) {
    const s = { pendingCounted: 0, pendingInherited: 0 };
    for (const [file, entry] of Object.entries(report.files)) {
      if (!matchList(file, patterns)) continue;
      const covering = classified.filter((d) => d.paths.includes('*') || d.paths.includes(file));
      for (const m of entry.mutants ?? []) {
        s[m.status] = (s[m.status] ?? 0) + 1;
        if (m.status !== 'Killed' || covering.length === 0) continue;
        if (covering.some((d) => !d.inherited)) s.pendingCounted++;
        else s.pendingInherited++;
      }
    }
    out[name] = s;
  }
  return out;
}
```

- [ ] **Step 4: Run to verify pass**

Run: `node --test templates/mutation-incremental/test/deferrals.test.mjs templates/mutation-incremental/test/views.test.mjs`
Expected: PASS (13 tests).

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): deferral classification, pending cause and views

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: The `plan` command (spec §5.1) and the Node-version warning

**Files:**
- Create: `templates/mutation-incremental/lib/nodever.mjs`, `lib/main.mjs`
- Replace: `templates/mutation-incremental/cli.mjs`
- Create: `templates/mutation-incremental/test/main.test.mjs`
- Modify: `templates/mutation-incremental/test/json.test.mjs` (remove the placeholder test)

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `nodeVersionWarning(top, running?) → string|null`;
  - `parseArgs(args) → {command, config, alsoState: string[], rest}`;
  - `main(args, io?) → Promise<exitCode>`, where `io = {stdout, stderr, cwd}`;
  - `summaryLine(fields) → string`;
  - `loadPlanInputs(io, options) → {top, config, snapshot, graph, candidates, chosen}`.
  - `cli.mjs` calls `main(process.argv.slice(2))`.
- Plan 1b extends `main` with the remaining commands and reuses `loadPlanInputs` and `summaryLine`.

- [ ] **Step 1: Write the failing tests**

`templates/mutation-incremental/test/main.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { main, parseArgs } from '../lib/main.mjs';
import { nodeVersionWarning } from '../lib/nodever.mjs';
import { loadConfig } from '../lib/config.mjs';
import { takeSnapshot } from '../lib/snapshot.mjs';
import { makeRepo, writeState } from './helpers.mjs';

const capture = () => {
  const out = { text: '', write(s) { this.text += s; } };
  return out;
};
async function run(args, cwd) {
  const stdout = capture();
  const stderr = capture();
  const code = await main(args, { stdout, stderr, cwd });
  return { code, stdout: stdout.text, stderr: stderr.text };
}

test('parseArgs', () => {
  assert.deepEqual(parseArgs(['plan', '--also-state', 'a', '--config', 'c.json', '--also-state', 'b']), { command: 'plan', config: 'c.json', alsoState: ['a', 'b'], rest: [] });
  assert.throws(() => parseArgs(['plan', '--config']), (e) => e.exitCode === 2);
  assert.throws(() => parseArgs(['plan', '--bogus']), (e) => e.exitCode === 2);
  assert.deepEqual(parseArgs([]).command, undefined);
});

test('plan on a repo without state is cold', async () => {
  const r = makeRepo({ files: { 'src/a.ts': 'export const a = 1;\n' } });
  const res = await run(['plan'], r.top);
  assert.equal(res.code, 0);
  const plan = JSON.parse(res.stdout);
  assert.equal(plan.cold, true);
  assert.deepEqual(plan.deferrals, [{ reason: 'no usable state', paths: ['*'] }]);
  assert.match(res.stderr, /mutation-incremental: command=plan invocations=0 scope=0 forced=0 deferrals=1 pending_full=true\n$/);
});

test('plan adopts a warm state and reports the change', async () => {
  const r = makeRepo({ files: { 'src/a.ts': 'export const a = 1;\n' } });
  const cfg = loadConfig(r.top);
  const snap = takeSnapshot(cfg);
  // The also-state lives under stateDir, which the snapshot excludes, so it is not itself a change.
  const report = {
    files: { 'src/a.ts': { language: 'typescript', source: 'export const a = 1;\n', mutants: [{ id: '1', mutatorName: 'M', replacement: 'r', status: 'Killed', killedBy: ['t'], coveredBy: ['t'], location: { start: { line: 1, column: 1 }, end: { line: 1, column: 20 } } }] } },
    testFiles: { 'tests/a.test.ts': { tests: [{ id: 't', name: 'a' }] } },
  };
  writeState(join(r.top, '.mutation/remote'), { report, attestation: { files: snap.files, runtime: snap.runtime } });
  r.write('src/a.ts', 'export const a = 2;\n');
  const res = await run(['plan', '--also-state', '.mutation/remote'], r.top);
  assert.equal(res.code, 0);
  const plan = JSON.parse(res.stdout);
  assert.equal(plan.baseline, '.mutation/remote');
  assert.deepEqual(plan.changes, [{ path: 'src/a.ts', category: 'mutate', kind: 'changed' }]);
  // Residual mode (the default): the edited file goes whole into the unforced scope and its
  // changed line is forced.
  assert.deepEqual([plan.scope, plan.forced], [['src/a.ts'], ['src/a.ts:1-1']]);
});

test('unexpected internal errors are exit 4, never 1', async () => {
  const r = makeRepo();
  const stderr = capture();
  const code = await main(['plan'], { stdout: { write() { throw new Error('boom'); } }, stderr, cwd: r.top });
  assert.equal(code, 4);
  assert.match(stderr.text, /internal error: boom/);
});

test('usage errors are exit 2 with a message', async () => {
  const r = makeRepo();
  assert.equal((await run(['nope'], r.top)).code, 2);
  assert.match((await run([], r.top)).stderr, /usage/);
  assert.equal((await run(['plan', '--config', 'missing.json'], r.top)).code, 2);
  const notRepo = await run(['plan'], mkdtempSync(join(tmpdir(), 'mi-norepo-')));
  assert.equal(notRepo.code, 2);
  assert.match(notRepo.stderr, /not inside a git repository/);
});

test('plan validates views before printing', async () => {
  const r = makeRepo({ config: { views: { inline: { u: ['src/a.ts'] } } }, files: { 'src/a.ts': 'a', 'src/b.ts': 'b' } });
  const res = await run(['plan'], r.top);
  assert.equal(res.code, 2);
  assert.match(res.stderr, /src\/b.ts belongs to no view/);
});

test('plan prints the Node-version warning before the summary', async () => {
  const r = makeRepo({ files: { '.nvmrc': '1.2\n' } });
  const res = await run(['plan'], r.top);
  assert.match(res.stderr, /warning: \.nvmrc pins Node 1\.2 but this is v\d+.*\nmutation-incremental: command=plan/);
});

test('nodeVersionWarning: prefix match on components, aliases skipped, first file wins', () => {
  const r = makeRepo({ files: { '.nvmrc': 'v22\n', '.node-version': '1.0.0' } });
  assert.equal(nodeVersionWarning(r.top, 'v22.9.0'), null);
  assert.match(nodeVersionWarning(r.top, 'v2.9.0'), /pins Node 22/);
  const alias = makeRepo({ files: { '.nvmrc': 'lts/iron\n', '.node-version': '1.0.0' } });
  assert.equal(nodeVersionWarning(alias.top, 'v22.9.0'), null);
  const nv = makeRepo({ files: { '.node-version': '20.1' } });
  assert.match(nodeVersionWarning(nv.top, 'v20.11.0'), /\.node-version pins Node 20\.1/);
  assert.equal(nodeVersionWarning(makeRepo().top, 'v22.9.0'), null);
});

test('cli.mjs runs main with process.argv', async () => {
  const saved = process.argv;
  process.argv = [process.execPath, 'cli.mjs', '--bogus'];
  try {
    await import('../cli.mjs');
    assert.equal(process.exitCode, 2);
  } finally {
    process.argv = saved;
    process.exitCode = 0;
  }
});
```

Remove the `cli placeholder loads` test from `test/json.test.mjs`.

- [ ] **Step 2: Run to verify failure**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/main.test.mjs`
Expected: FAIL — missing modules.

- [ ] **Step 3: Implement**

`templates/mutation-incremental/lib/nodever.mjs`:

```js
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

// Spec §5.1: the first of .nvmrc and .node-version that exists, trimmed; a numeric version whose
// dot-separated components are not a prefix of the running version's warns. Aliases are skipped.
export function nodeVersionWarning(top, running = process.version) {
  for (const file of ['.nvmrc', '.node-version']) {
    let text;
    try {
      text = readFileSync(join(top, file), 'utf8');
    } catch {
      continue;
    }
    const want = text.trim().replace(/^v/, '');
    if (!/^\d+(\.\d+)*$/.test(want)) return null;
    const have = running.replace(/^v/, '').split('.');
    const matches = want.split('.').every((part, i) => part === have[i]);
    return matches ? null : `mutation-incremental: warning: ${file} pins Node ${want} but this is ${running}`;
  }
  return null;
}
```

`templates/mutation-incremental/lib/main.mjs`:

```js
import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { loadConfig } from './config.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { buildGraph } from './graph.mjs';
import { readCandidate, adopt } from './state.mjs';
import { computePlan, publicPlan } from './plan.mjs';
import { resolveViews } from './views.mjs';
import { nodeVersionWarning } from './nodever.mjs';

const USAGE = 'usage: cli.mjs plan [--config <path>] [--also-state <dir>]...';

export function parseArgs(args) {
  const out = { command: args[0], config: undefined, alsoState: [], rest: [] };
  for (let i = 1; i < args.length; i++) {
    const a = args[i];
    const value = () => {
      if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
      return args[++i];
    };
    if (a === '--config') out.config = value();
    else if (a === '--also-state') out.alsoState.push(value());
    else throw new UsageError(`unknown option ${a}`);
  }
  return out;
}

export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;

function topLevel(cwd) {
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    throw new UsageError('not inside a git repository');
  }
}

// Shared with Plan 1b's run command: everything planning needs, with adoption in memory only.
export function loadPlanInputs(io, options) {
  const top = topLevel(io.cwd);
  const config = loadConfig(top, options.config);
  const warning = nodeVersionWarning(top);
  if (warning) io.stderr.write(`${warning}\n`);
  const snapshot = takeSnapshot(config);
  const graph = buildGraph(snapshot.files, config);
  const candidates = [
    readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true }),
    ...options.alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false })),
  ];
  return { top, config, snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
}

function planCommand(io, options) {
  const { config, snapshot, graph, chosen } = loadPlanInputs(io, options);
  const plan = computePlan({ config, snapshot, graph, chosen });
  resolveViews(config, snapshot);
  io.stdout.write(canonicalJSON(publicPlan(plan)));
  io.stderr.write(summaryLine({
    command: 'plan',
    invocations: Number(plan.invocation1.length > 0) + Number(plan.invocation2.length > 0),
    scope: plan.scope.length,
    forced: plan.forced.length,
    deferrals: plan.deferrals.length,
    pendingFull: plan.pendingFull,
  }));
  return 0;
}

const COMMANDS = { plan: planCommand };

export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
  try {
    const options = parseArgs(args);
    const command = COMMANDS[options.command];
    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}; ${USAGE}`);
    return await command(io, options);
  } catch (e) {
    // Exit 1 means "ok, state committed" for run (Plan 1b), so an internal crash is exit 4.
    const usage = e instanceof UsageError;
    io.stderr.write(`mutation-incremental: ${usage ? 'error' : 'internal error'}: ${e.message}\n`);
    io.stderr.write(summaryLine({ command: args[0] ?? 'none' }));
    return usage ? e.exitCode : 4;
  }
}
```

`templates/mutation-incremental/cli.mjs`:

```js
#!/usr/bin/env node
// Entry point (spec §5.1). All logic lives in lib/main.mjs so it can be tested in-process.
import { main } from './lib/main.mjs';

process.exitCode = await main(process.argv.slice(2));
```

- [ ] **Step 4: Run to verify pass**

Run: `MUTATION_ALLOW_TMP_STATE=1 node --test templates/mutation-incremental/test/*.test.mjs`
Expected: PASS.

- [ ] **Step 5: Run the full coverage gate**

Run: `go test -run TestMutationIncrementalTemplate .`
Expected: PASS: every `lib/*.mjs` and `cli.mjs` at 100% lines/branches/functions. If the report shows an uncovered line or branch, add a focused test in the test file for that module that exercises it. Keep the input realistic (a real repo layout or a real report shape), and do not add `c8 ignore` comments.

- [ ] **Step 6: Run the repository gates**

Run: `make cover` and `go vet ./...`
Expected: PASS (100% statement coverage per gated Go package is unaffected; the new Go test lives in the root package).

- [ ] **Step 7: Commit**

```bash
git add templates/mutation-incremental
git commit -m "feat(mutation-incremental): plan command with adoption, views validation and Node-version warning

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Follow-ups (recorded, not in 1a: no credible normal-usage failure)

- `stateDir` placed inside a `mutate` tree (e.g. `stateDir: "src"`) is not rejected by the probe check.
- `buildGraph`'s 1 MiB limit compares UTF-16 length, not bytes.
- Dot-files inside a non-gitignored leftover `.stryker-tmp/` enter the snapshot as unclassified (over-defers only).
- Residual mode may add a second invocation (second dry run) when only the changed lines are forced; measure in the external trial.
- Docs (1c): a gitignored generated module imported by relative path makes its importers open importers; `MUTATION_ALLOW_TMP_STATE` is test-only (comment it in `config.mjs`).

## Self-Review (done while writing)

- **Spec coverage in 1a:**

  | Spec section | Task |
  |---|---|
  | §5.1 (`plan`, exit 2, summary line, Node warning) | Task 9 |
  | §5.2 + §11.1–§11.5 validation | Task 2 |
  | §5.3 | Task 1 |
  | §5.4 snapshot / runtime / import graph | Tasks 3–4 |
  | §5.4 Usable / Adopt / Change set / steps 1–5 / Normalise / Plan JSON | Tasks 6–7 |
  | §11.2 classification and cause | Task 8 |
  | §11.3 views and summaries | Task 8 |
  | §11.4.1 diff | Task 5 |
  | §11.4.2–3 residual | Task 7 |
  | §11.5 K4 | Task 7 |
  | §11.6 `stateVersion` | Task 6 |

  Deferred to 1b: §5.5 attestation writing, §5.6 execution, verify, seed, fetch/publish, `pending_cause` outputs and the shortcut. Deferred to 1c: §5.7 workflow, §7.1 fixture/e2e, the golden-output guard, docs. Deferred to Plan 2: §6.
- **Placeholders:** none; every code step is complete.
- **Type consistency:** these names are used identically across tasks: `Candidate`, `adopt(...).changes`, `computePlan({config, snapshot, graph, chosen, ...})`, `classifyDeferrals({deferrals, alsoAttestations, snapshot, producedKeys, cold})`, `loadPlanInputs`.
- **Review Focus:** each of the five items has a named test in its owning task.
