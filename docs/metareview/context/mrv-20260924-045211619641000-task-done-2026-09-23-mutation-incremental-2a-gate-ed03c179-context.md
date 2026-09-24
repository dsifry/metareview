# metareview task-done context

Run ID: `mrv-20260924-045211619641000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179`

## Task

# Mutation Evidence Freshness Gate — Plan 2a: Core (spec §6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** metareview's `task-done`, `pr-ready` and `epic-ready` gates judge whether the kills in each `--mutation-report` still describe the code under review.
- Each kill is classified from the harness's attestation as verified, stale, pending, unbound or unattested.
- The gates raise stale, pending and unattested findings (advisory or enforced) and write a "Mutation Evidence Freshness" log section.
- The findings ledger supersedes stale rows once fresh evidence arrives.
- An escalation caused only by stale evidence waits for a refresh.

**Architecture:**
- **`internal/mutationfresh`** (new, pure Go, 100% covered) holds the logic:
  - the §5.3 glob dialect;
  - the §5.4 content digests, including the config file digested without `views`;
  - working-tree and HEAD content readers;
  - attestation validation (§6.1), kill classification (§6.3), findings and the log section (§6.5, §6.6), and the mode (§6.4).
- **`internal/mutation`** parses the extra Stryker fields it needs (§6.1). They are tagged `json:"-"`, so existing fingerprints and digests are unchanged.
- **`internal/reviewers.LoadMutationContext`** replaces the three copies of `mutationContextFor`. Its new `Mode`/`Freshness` fields feed pr-ready's reviewer-input digest (§6.7).
- **`internal/findings`** gains the superseded lifecycle for the three freshness prefixes (§6.5).
- **pr-ready and epic-ready** gain the stale-only escalation exemption (§6.8).
- **`internal/learning`** skips freshness records (§6.9).

**Tech Stack:** Go 1.26 (module `github.com/dsifry/metareview`; 100% statement coverage per gated package via `make cover`), git, and the harness template's Node tests (100% coverage) for the one harness change.

**Spec:** `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r21): §6, plus §11.2's `inherited` flag (read-only here).

**Plan series:** 1a–1c (harness, done) → **2a (this plan)** → 2b (`2026-09-23-mutation-incremental-2b-views.md`):
- §11.3 views (`--mutation-view`, per-view fingerprints, view-aware ledger);
- §11.5's out-of-mutant-hunk cause (a Go port of the line diff on the shared vectors);
- the §7.1 gate rows.

## Global Constraints

- Go: every gated package stays at 100% statement coverage (`make cover`; the new package is gated automatically). `go vet ./...` is clean, and `gofmt` is clean.
- Harness template: Node 100% line/branch/function coverage (`go test -run TestMutationIncrementalTemplate .`).
- A run **without** `--mutation-report` behaves exactly as before: no section, no findings, and pr-ready's reviewer-input digest is unchanged (`omitempty`).
- A read error other than absence stops the review (§6.2). An invalid `METAREVIEW_MUTATION_FRESHNESS` is a usage error, exit 2 (§6.4).
- Glob semantics must equal the harness's on `testdata/mutation-incremental/glob-vectors.json`. The config digest must equal the harness's on the new `testdata/mutation-incremental/config-digest-vectors.json`.
- `make cover` stops at golangci-lint on this machine (the toolchain was built with Go 1.26; the module targets 1.27). Where a step says `make cover`, run `go test -cover ./...` and `go vet ./...` instead, and read each gated package's coverage. This was ruled environmental in Plan 1a.
- Review stance (user): real workflows and real edge cases; assume trust; no engineering for rare races. Block only for a credible normal-usage failure.
- Each task ends with a commit; messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Real harness reports classify correctly.** On the committed real reports, an unchanged tree is all verified. Editing `src/a.ts` makes the `a.ts` and `b.ts` kills stale (cause `src/a.ts`) and leaves the `c.ts`/`e.ts` kills verified. A lockfile edit makes every kill stale. A `"*"` deferral makes every kill pending. Task 4, tests `TestClassifyRealReports…`.
2. **Editing only the view map never stales anything.** The config digest matches the harness byte for byte. Task 2, `TestConfigDigestVectors`; Task 1's Node test pins the same vectors.
3. **pr-ready reads HEAD.** An uncommitted edit does not stale a kill until it is committed (or `--include-working-tree` is passed). Task 3, `TestHeadContent…`; Task 6, the pr-ready integration test.
4. **Fresh evidence clears an enforced stale blocker.** A later run with fresh reports supersedes the row. A run without reports leaves it alone. A recurring stale fingerprint opens a new row. Task 7.
5. **A stale-only chain does not escalate at `maxAttempts`.** It escalates at 2 × `maxAttempts` with the stated reason. Task 8.

## Plan decisions beyond the spec text (flagged for review)

- **The attestation records its config path** (`"config": "<repo-relative config path>"`). The gate needs to know which file to digest without `views` (Plan 1a's decision). Without the field, the gate would digest the raw bytes, so the config would always read as changed and every kill would be stale.
  - This is an additive field. Nothing has shipped, so there is no `stateVersion` bump.
  - An attestation without the field is digested raw.
  - Go and Node share digest vectors. The residual is that Go's `encoding/json` escapes U+2028/U+2029 and Node does not; such characters do not occur in configs.
- **Glob matching is a segment matcher, not a regular expression.** RE2 has no lookahead. The dot rule — `*`, `?` and `**` do not match a segment that starts with `.` unless the pattern segment does — is implemented directly. The shared vectors prove the two agree.
- **HEAD content is read with one `git cat-file --batch --filters` process**, not one process per file. A thousand-file repository would otherwise spawn a thousand processes per gate run.
- **"The score summary excludes stale, pending, unbound and unattested kills" (§6.5)** is delivered by the section's `Kills:` line. The existing honest-score text in the unresolved finding is untouched. metareview applies no score threshold, so no verdict changes.
- **Finding texts** (the spec leaves them to this plan):
  - stale: "`<n>` kill(s) in `<reports>` were verified against a different `<cause>` …";
  - pending: "`<n>` kill(s) in `<report>` are covered by a deferral (`<reasons>`) …";
  - unattested: "`<report>` has no valid attestation (`<reason>`) …".

  The exact strings are in Task 5. The reviewer is `mutation-freshness`.
- **`activeExisting` excludes every `superseded` row, not only the freshness prefixes.** The only other superseded rows are pre-0.8.3 context-risk aliases, whose fingerprints no current run produces, so this changes nothing else.
- **A report whose parse carries no Stryker detail (gremlins) but has a harness attestation is unattested with reason `engine`.** The harness only attests Stryker.

---

## File Structure

```
internal/mutationfresh/
  glob.go          # MatchGlob, MatchList, Categorize (§5.3)
  digest.go        # Entry, digest, configDigest (§5.4)
  content.go       # Content, Worktree(root), Head(root) (§6.2)
  attest.go        # Attestation, readAttestation (§6.1)
  classify.go      # ReportFreshness, Classify (§6.3)
  build.go         # Result, Build, findings and the log section (§6.5, §6.6)
  mode.go          # Advisory, Enforce, ModeFromEnv, EffectiveMode (§6.4)
  *_test.go
internal/mutation/{stryker.go,parse.go,report.go}   # StrykerDetail, Report.SHA256/Detail (json:"-")
internal/reviewers/mutation.go                      # MutationContext.Mode/Freshness, LoadMutationContext
internal/{taskdone,prready,epicready}/review.go      # use LoadMutationContext; section; engines to Reconcile
internal/findings/{findings.go,override.go}          # freshness lifecycle
internal/learning/candidates.go                      # skip freshness records
cmd/metareview/main.go                               # mode validation (exit 2), usage text
templates/mutation-incremental/lib/{attest.mjs,snapshot.mjs}  # "config" field; export configDigest
testdata/mutation-incremental/config-digest-vectors.json      # shared Node/Go vectors
```

---

### Task 1: The harness records its config path; shared config-digest vectors

**Files:**
- Modify: `templates/mutation-incremental/lib/attest.mjs` (the `config` field), `lib/snapshot.mjs` (export `configDigest`)
- Modify: `templates/mutation-incremental/test/attest.test.mjs` (the key list)
- Create: `templates/mutation-incremental/test/configdigest.test.mjs`, `testdata/mutation-incremental/config-digest-vectors.json`

**Interfaces:**
- Produces:
  - the attestation key `config` (the value of `config.configRel`);
  - `configDigest(absPath): string`, exported;
  - a vector file `{ "cases": [ { "name", "text", "digest" } ] }`.

- [ ] **Step 1: Write the failing tests**

In `templates/mutation-incremental/test/attest.test.mjs`, the expected key list in `buildAttestation writes the §5.5 contract` gains `'config'` (between `'completedAt'` and `'deferrals'`). Append to the same test:

```js
  assert.equal(att.config, 'mutation-incremental.json');
```

`templates/mutation-incremental/test/configdigest.test.mjs`:

```js
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configDigest } from '../lib/snapshot.mjs';

// Shared with metareview's Go gate (internal/mutationfresh): both must digest the config file
// identically, or every kill reads stale whenever the attested config is compared.
const VECTORS = new URL('../../../testdata/mutation-incremental/config-digest-vectors.json', import.meta.url);

test('config digest vectors (shared with the Go gate)', () => {
  const { cases } = JSON.parse(readFileSync(VECTORS, 'utf8'));
  assert.ok(cases.length >= 5);
  const dir = mkdtempSync(join(tmpdir(), 'mi-cfgdigest-'));
  for (const c of cases) {
    const path = join(dir, 'config.json');
    writeFileSync(path, c.text);
    assert.equal(configDigest(path), c.digest, c.name);
  }
});

test('the view map does not change the digest; every other key does', () => {
  const { cases } = JSON.parse(readFileSync(VECTORS, 'utf8'));
  const byName = Object.fromEntries(cases.map((c) => [c.name, c.digest]));
  assert.equal(byName['example with inline views'], byName.example);
  assert.equal(byName['example reformatted'], byName.example);
  assert.notEqual(byName['example with another budget'], byName.example);
});
```

`testdata/mutation-incremental/config-digest-vectors.json`: in Step 3, generate the digests with Node from these texts; the `text` values are exact:

```json
{
  "cases": [
    { "name": "example", "text": "{\n  \"schemaVersion\": 1,\n  \"stateDir\": \".mutation\",\n  \"stryker\": { \"command\": [\"npx\", \"--no-install\", \"stryker\"], \"configFile\": \"stryker.config.json\", \"extraArgs\": [] },\n  \"mutate\": [\"src/**/*.ts\", \"!src/**/*.d.ts\"],\n  \"test\": [\"tests/**/*.test.ts\"],\n  \"support\": [\"tests/helpers/**\"],\n  \"global\": [\"package.json\", \"package-lock.json\"],\n  \"ignore\": [\"docs/**\", \"**/*.md\"],\n  \"aliases\": { \"@app/\": \"src/\" },\n  \"runtime\": { \"commands\": [], \"env\": [] },\n  \"budget\": { \"maxForcedShare\": 0.25, \"maxForcedMutants\": null, \"maxMinutesPerInvocation\": 20 },\n  \"residual\": { \"mode\": \"bounded\" }\n}\n", "digest": "" },
    { "name": "example with inline views", "text": "{\n  \"schemaVersion\": 1,\n  \"stateDir\": \".mutation\",\n  \"stryker\": { \"command\": [\"npx\", \"--no-install\", \"stryker\"], \"configFile\": \"stryker.config.json\", \"extraArgs\": [] },\n  \"mutate\": [\"src/**/*.ts\", \"!src/**/*.d.ts\"],\n  \"test\": [\"tests/**/*.test.ts\"],\n  \"support\": [\"tests/helpers/**\"],\n  \"global\": [\"package.json\", \"package-lock.json\"],\n  \"ignore\": [\"docs/**\", \"**/*.md\"],\n  \"aliases\": { \"@app/\": \"src/\" },\n  \"runtime\": { \"commands\": [], \"env\": [] },\n  \"budget\": { \"maxForcedShare\": 0.25, \"maxForcedMutants\": null, \"maxMinutesPerInvocation\": 20 },\n  \"residual\": { \"mode\": \"bounded\" },\n  \"views\": { \"inline\": { \"core\": [\"src/**\"] } }\n}\n", "digest": "" },
    { "name": "example reformatted", "text": "{\"residual\":{\"mode\":\"bounded\"},\"budget\":{\"maxMinutesPerInvocation\":20,\"maxForcedMutants\":null,\"maxForcedShare\":0.25},\"runtime\":{\"env\":[],\"commands\":[]},\"aliases\":{\"@app/\":\"src/\"},\"ignore\":[\"docs/**\",\"**/*.md\"],\"global\":[\"package.json\",\"package-lock.json\"],\"support\":[\"tests/helpers/**\"],\"test\":[\"tests/**/*.test.ts\"],\"mutate\":[\"src/**/*.ts\",\"!src/**/*.d.ts\"],\"stryker\":{\"extraArgs\":[],\"configFile\":\"stryker.config.json\",\"command\":[\"npx\",\"--no-install\",\"stryker\"]},\"stateDir\":\".mutation\",\"schemaVersion\":1}", "digest": "" },
    { "name": "example with another budget", "text": "{\n  \"schemaVersion\": 1,\n  \"stateDir\": \".mutation\",\n  \"stryker\": { \"command\": [\"npx\", \"--no-install\", \"stryker\"], \"configFile\": \"stryker.config.json\", \"extraArgs\": [] },\n  \"mutate\": [\"src/**/*.ts\", \"!src/**/*.d.ts\"],\n  \"test\": [\"tests/**/*.test.ts\"],\n  \"support\": [\"tests/helpers/**\"],\n  \"global\": [\"package.json\", \"package-lock.json\"],\n  \"ignore\": [\"docs/**\", \"**/*.md\"],\n  \"aliases\": { \"@app/\": \"src/\" },\n  \"runtime\": { \"commands\": [], \"env\": [] },\n  \"budget\": { \"maxForcedShare\": 0.5, \"maxForcedMutants\": 1000, \"maxMinutesPerInvocation\": 7.5 },\n  \"residual\": { \"mode\": \"bounded\" }\n}\n", "digest": "" },
    { "name": "non-ascii, escapes and html characters", "text": "{\"schemaVersion\":1,\"note\":\"café <b>&</b> \\\"quoted\\\" \\\\ tab\\there\",\"views\":{\"command\":[\"node\",\"views.mjs\"]}}\n", "digest": "" },
    { "name": "numbers", "text": "{\"a\":1.0,\"b\":1e3,\"c\":0.000001,\"e\":123456789012,\"f\":[0.1,2.50]}\n", "digest": "" }
  ]
}
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test templates/mutation-incremental/test/configdigest.test.mjs templates/mutation-incremental/test/attest.test.mjs`
Expected: FAIL. `configDigest` is not exported, and the attestation has no `config` key.

- [ ] **Step 3: Implement, then fill in the vectors**

In `templates/mutation-incremental/lib/snapshot.mjs`, export the function: `function configDigest(absPath)` becomes `export function configDigest(absPath)`.

In `templates/mutation-incremental/lib/attest.mjs`, inside `buildAttestation`'s object, add after `completedAt,`:

```js
    // The config file the gate digests without its views (spec K3.4; metareview's mutationfresh).
    config: config.configRel,
```

Fill in the digests with this one-off script. It is not committed; keep it in the workspace:

```js
// fill-config-vectors.mjs — run from the repository root: node <workspace>/fill-config-vectors.mjs
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configDigest } from '../../../templates/mutation-incremental/lib/snapshot.mjs';
const file = 'testdata/mutation-incremental/config-digest-vectors.json';
const v = JSON.parse(readFileSync(file, 'utf8'));
const dir = mkdtempSync(join(tmpdir(), 'mi-fill-'));
for (const c of v.cases) { writeFileSync(join(dir, 'c.json'), c.text); c.digest = configDigest(join(dir, 'c.json')); }
writeFileSync(file, `${JSON.stringify(v, null, 2)}\n`);
```

- [ ] **Step 4: Run the template suite with coverage**

Run: `sh <workspace>/cov.sh`
Expected: all pass; no uncovered entries. The golden plan outputs are unchanged, because this changes the attestation, not the plan.

- [ ] **Step 5: Commit**

```bash
git add templates/mutation-incremental/lib/attest.mjs templates/mutation-incremental/lib/snapshot.mjs templates/mutation-incremental/test/attest.test.mjs templates/mutation-incremental/test/configdigest.test.mjs testdata/mutation-incremental/config-digest-vectors.json
git commit -m "feat(mutation-incremental): attestation records its config path; shared config-digest vectors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `mutationfresh` — glob dialect and digests

**Files:**
- Create: `internal/mutationfresh/glob.go`, `internal/mutationfresh/digest.go`, `internal/mutationfresh/glob_test.go`, `internal/mutationfresh/digest_test.go`

**Interfaces:**
- Produces:
  - `MatchGlob(pattern, path string) bool`;
  - `MatchList(path string, list []string) bool`;
  - `Lists{Mutate, Test, Support, Global, Ignore []string}`;
  - `Categorize(path string, lists Lists) string`;
  - `Entry{Data []byte; Symlink bool}`;
  - `Absent` (`"absent"`);
  - `digestOf(e Entry) string`;
  - `configDigest(data []byte) string`.

- [ ] **Step 1: Write the failing tests**

`internal/mutationfresh/glob_test.go`:

```go
package mutationfresh

import (
	"encoding/json"
	"os"
	"testing"
)

// The glob dialect is shared with the harness (spec §5.3). Both implementations run the same vectors,
// so a change to either must change both.
func TestGlobVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mutation-incremental/glob-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Match []struct {
			Pattern, Path string
			Match         bool
		} `json:"match"`
		List []struct {
			List  []string `json:"list"`
			Path  string   `json:"path"`
			Match bool     `json:"match"`
		} `json:"list"`
		Categorize struct {
			Lists Lists `json:"lists"`
			Cases []struct{ Path, Category string } `json:"cases"`
		} `json:"categorize"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Match {
		if got := MatchGlob(c.Pattern, c.Path); got != c.Match {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.Pattern, c.Path, got, c.Match)
		}
	}
	for _, c := range v.List {
		if got := MatchList(c.Path, c.List); got != c.Match {
			t.Errorf("MatchList(%q, %v) = %v, want %v", c.Path, c.List, got, c.Match)
		}
	}
	for _, c := range v.Categorize.Cases {
		if got := Categorize(c.Path, v.Categorize.Lists); got != c.Category {
			t.Errorf("Categorize(%q) = %q, want %q", c.Path, got, c.Category)
		}
	}
}

func TestGlobDotRuleAndAlternation(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"?.ts", ".ts", false},      // ? at the start of a segment does not match a dot
		{"a?.ts", "a..ts", true},    // ? elsewhere does
		{"?", "", false},            // ? needs a character
		{"x*", "x.ts", true},        // * after a literal matches a dot
		{"{a,b}.ts", "b.ts", true},  // alternation
		{"{a,*}.ts", ".x.ts", false}, // an alternative at the start keeps the dot rule
		{"x{a,b", "x{a,b", true},    // an unclosed brace is literal
		{"src/**", "src/.git/x", false},
		{"src/.git/*", "src/.git/x", true},
		{"a**b", "axxb", true}, // consecutive stars inside a segment are one star
	}
	for _, c := range cases {
		if got := MatchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
	if !MatchList("src/a.ts", []string{"lib/**", "src/**"}) || MatchList("src/a.d.ts", []string{"src/**", "!src/*.d.ts"}) {
		t.Error("list rule: some positive entry matches and no negated entry matches")
	}
}
```

`internal/mutationfresh/digest_test.go`:

```go
package mutationfresh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestDigestOf(t *testing.T) {
	sum := sha256.Sum256([]byte("x"))
	if got := digestOf(Entry{Data: []byte("x")}); got != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("file digest %q", got)
	}
	if got := digestOf(Entry{Data: []byte("x"), Symlink: true}); got != "symlink:"+hex.EncodeToString(sum[:]) {
		t.Errorf("symlink digest %q", got)
	}
}

// Spec K3.4 and Plan 1a: the harness digests its config file as canonical JSON without `views`.
// The gate must produce the same digest byte for byte, or the config reads as changed forever.
func TestConfigDigestVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mutation-incremental/config-digest-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct{ Name, Text, Digest string } `json:"cases"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) < 5 {
		t.Fatalf("expected the shared vectors, got %d cases", len(v.Cases))
	}
	for _, c := range v.Cases {
		if got := configDigest([]byte(c.Text)); got != c.Digest {
			t.Errorf("%s: got %s, want %s", c.Name, got, c.Digest)
		}
	}
}

func TestConfigDigestOfUnparseableTextIsTheRawDigest(t *testing.T) {
	if got, want := configDigest([]byte("not json")), digestOf(Entry{Data: []byte("not json")}); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mutationfresh/`
Expected: FAIL. The package does not exist.

- [ ] **Step 3: Implement**

`internal/mutationfresh/glob.go`:

```go
// Package mutationfresh judges whether the kills in a mutation report still describe the code under
// review (spec §6 of docs/superpowers/specs/2026-09-23-mutation-incremental-design.md). The harness
// (templates/mutation-incremental/) attests what it verified; this package re-derives, from content
// digests only, which of those kills a later edit has made stale.
package mutationfresh

import "strings"

// The glob dialect is shared with the harness (spec §5.3; templates/mutation-incremental/lib/glob.mjs).
// Both implementations run testdata/mutation-incremental/glob-vectors.json. Paths are repo-relative
// POSIX paths, case-sensitive. `*` matches a run of characters except `/`; `?` matches one; `**` as a
// whole segment matches zero or more segments; `{a,b}` is alternation (no nesting). `*`, `?` and `**`
// never match a segment that starts with `.` unless the pattern segment does (as in Stryker). RE2 has
// no lookahead, so this is a small matcher rather than a translated regular expression.

type globToken struct {
	kind  byte // 'l' literal, '*', '?', '{'
	lit   byte
	start bool // at the start of a path segment: the dot rule applies
	alts  [][]globToken
}

func compileSegment(seg string, atStart bool) []globToken {
	var out []globToken
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		start := atStart && i == 0
		switch {
		case c == '*':
			out = append(out, globToken{kind: '*', start: start})
			for i+1 < len(seg) && seg[i+1] == '*' {
				i++
			}
		case c == '?':
			out = append(out, globToken{kind: '?', start: start})
		case c == '{' && strings.IndexByte(seg[i:], '}') > 0:
			end := i + strings.IndexByte(seg[i:], '}')
			var alts [][]globToken
			for _, alt := range strings.Split(seg[i+1:end], ",") {
				alts = append(alts, compileSegment(alt, start))
			}
			out = append(out, globToken{kind: '{', alts: alts})
			i = end
		default:
			out = append(out, globToken{kind: 'l', lit: c})
		}
	}
	return out
}

func matchTokens(tokens []globToken, s string) bool {
	if len(tokens) == 0 {
		return s == ""
	}
	t, rest := tokens[0], tokens[1:]
	switch t.kind {
	case '*':
		if t.start && strings.HasPrefix(s, ".") {
			return false
		}
		for k := 0; k <= len(s); k++ {
			if matchTokens(rest, s[k:]) {
				return true
			}
		}
		return false
	case '?':
		if s == "" || (t.start && s[0] == '.') {
			return false
		}
		return matchTokens(rest, s[1:])
	case '{':
		for _, alt := range t.alts {
			if matchTokens(append(append([]globToken(nil), alt...), rest...), s) {
				return true
			}
		}
		return false
	default:
		return s != "" && s[0] == t.lit && matchTokens(rest, s[1:])
	}
}

func matchSegments(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		for k := 0; k <= len(path); k++ {
			if matchSegments(pattern[1:], path[k:]) {
				return true
			}
			if k < len(path) && strings.HasPrefix(path[k], ".") {
				return false
			}
		}
		return false
	}
	return len(path) > 0 && matchTokens(compileSegment(pattern[0], true), path[0]) && matchSegments(pattern[1:], path[1:])
}

// MatchGlob reports whether path matches one pattern. A leading "./" in the pattern is removed.
func MatchGlob(pattern, path string) bool {
	return matchSegments(strings.Split(strings.TrimPrefix(pattern, "./"), "/"), strings.Split(path, "/"))
}

// MatchList: some non-`!` entry matches and no `!` entry matches.
func MatchList(path string, list []string) bool {
	hit := false
	for _, entry := range list {
		if strings.HasPrefix(entry, "!") {
			if MatchGlob(entry[1:], path) {
				return false
			}
		} else if !hit {
			hit = MatchGlob(entry, path)
		}
	}
	return hit
}

// Lists are the attested category lists (the implicit global entries included).
type Lists struct {
	Mutate  []string `json:"mutate"`
	Test    []string `json:"test"`
	Support []string `json:"support"`
	Global  []string `json:"global"`
	Ignore  []string `json:"ignore"`
}

// Categorize returns the first list that matches in the order global, support, test, mutate; else
// "ignore" for an ignored path, else "unclassified".
func Categorize(path string, lists Lists) string {
	ordered := []struct {
		name string
		list []string
	}{{"global", lists.Global}, {"support", lists.Support}, {"test", lists.Test}, {"mutate", lists.Mutate}}
	for _, l := range ordered {
		if MatchList(path, l.list) {
			return l.name
		}
	}
	if MatchList(path, lists.Ignore) {
		return "ignore"
	}
	return "unclassified"
}
```

`internal/mutationfresh/digest.go`:

```go
package mutationfresh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Absent is the digest of a path that is missing or is neither a regular file nor a symlink (§6.2).
const Absent = "absent"

// Entry is one path's content under review: file bytes, or a symlink's link text.
type Entry struct {
	Data    []byte
	Symlink bool
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// digestOf is the §5.4 digest: sha256:<hex> of the bytes, or symlink:<hex> of the link text.
func digestOf(e Entry) string {
	if e.Symlink {
		return "symlink:" + sha256Hex(e.Data)
	}
	return "sha256:" + sha256Hex(e.Data)
}

// configDigest digests the harness config as the harness does (lib/snapshot.mjs configDigest):
// canonical JSON — keys sorted, two-space indent, a trailing newline — without `views`, so editing
// the view map is not a change. Text that is not a JSON object is digested raw (the harness would
// have refused it). The shared vectors pin the two implementations together.
func configDigest(data []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return digestOf(Entry{Data: data})
	}
	delete(raw, "views")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(raw) // a value decoded from JSON always encodes
	return "sha256:" + sha256Hex(buf.Bytes())
}
```

- [ ] **Step 4: Run to verify they pass with full coverage**

Run: `go test -cover ./internal/mutationfresh/`
Expected: `ok`, coverage 100.0%. If a vector disagrees, the Go implementation is wrong. The harness is the reference: fix Go, never the vectors.

- [ ] **Step 5: Commit**

```bash
git add internal/mutationfresh
git commit -m "feat(mutationfresh): shared glob dialect and content digests

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 3: Content under review and attestation validation

**Files:**
- Create: `internal/mutationfresh/content.go`, `internal/mutationfresh/attest.go`, `internal/mutationfresh/content_test.go`, `internal/mutationfresh/attest_test.go`, `internal/mutationfresh/helpers_test.go`

**Interfaces:**
- Produces:
  - `Content` (`Read(paths []string) (map[string]Entry, error)`, `Paths() ([]string, error)`, `Head() bool`), with constructors `Worktree(root) Content` and `Head(root) Content`;
  - `Attestation`, `FileEntry`, `Deferral`;
  - `readAttestation(reportPath, engine, reportSHA string) (Attestation, reason, attestationSHA string)`, whose reasons are `missing|unparseable|version|tool|engine|report mismatch|hash mismatch`.

- [ ] **Step 1: Write the failing tests**

`internal/mutationfresh/helpers_test.go`:

```go
package mutationfresh

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitRepo creates a repository with the given files committed, and returns its root.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, text := range files {
		write(t, root, path, text)
	}
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "t@example.com")
	git(t, root, "config", "user.name", "T")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "init")
	return root
}

func write(t *testing.T, root, path, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
```

`internal/mutationfresh/content_test.go`:

```go
package mutationfresh

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWorktreeReadsFilesSymlinksAndAbsence(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a", "dir/b.txt": "b"})
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	w := Worktree(root)
	if w.Head() {
		t.Error("the working tree is not HEAD mode")
	}
	got, err := w.Read([]string{"a.txt", "link", "missing", "dir", "a.txt/under-a-file"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["a.txt"].Data) != "a" || got["a.txt"].Symlink {
		t.Errorf("a.txt = %+v", got["a.txt"])
	}
	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
		t.Errorf("link = %+v", got["link"])
	}
	for _, absent := range []string{"missing", "dir", "a.txt/under-a-file"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s must be absent (missing, a directory, or under a file)", absent)
		}
	}
}

func TestWorktreeReadErrorsStopTheReview(t *testing.T) {
	root := gitRepo(t, map[string]string{"locked/a.txt": "a", "b.txt": "b"})
	if err := os.Chmod(filepath.Join(root, "b.txt"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "locked"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked"), 0o755) })
	if _, err := Worktree(root).Read([]string{"b.txt"}); err == nil {
		t.Error("an unreadable file stops the review")
	}
	if _, err := Worktree(root).Read([]string{"locked/a.txt"}); err == nil {
		t.Error("an unreadable directory stops the review")
	}
}

func TestWorktreePathsListsTrackedAndUntrackedButNotNestedRepositories(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a", ".gitignore": "ignored.txt\n"})
	write(t, root, "new.txt", "n")
	write(t, root, "ignored.txt", "i")
	nested := filepath.Join(root, "sub")
	_ = gitRepo(t, nil)
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, nested, "init", "-q")
	got, err := Worktree(root).Paths()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{".gitignore", "a.txt", "new.txt"}) {
		t.Errorf("paths = %v", got)
	}
	if _, err := Worktree(t.TempDir()).Paths(); err == nil {
		t.Error("outside a repository is an error")
	}
}

// §6.2 and §7.2: HEAD mode reads the committed content through the repository's filters, sees
// symlinks as link text, excludes gitlinks, and ignores uncommitted edits.
func TestHeadContentReadsCommittedContentThroughFilters(t *testing.T) {
	root := gitRepo(t, map[string]string{".gitattributes": "*.txt eol=crlf\n", "a.txt": "one\ntwo\n", "b.bin": "b"})
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	sha := git(t, root, "rev-parse", "HEAD")
	git(t, root, "update-index", "--add", "--cacheinfo", "160000,"+sha[:40]+",gitlink")
	git(t, root, "add", "link")
	git(t, root, "commit", "-qm", "link and gitlink")
	write(t, root, "b.bin", "uncommitted")
	h := Head(root)
	if !h.Head() {
		t.Error("HEAD mode")
	}
	got, err := h.Read([]string{"a.txt", "b.bin", "link", "gitlink", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["a.txt"].Data) != "one\r\ntwo\r\n" {
		t.Errorf("a.txt through eol=crlf = %q", got["a.txt"].Data)
	}
	if string(got["b.bin"].Data) != "b" {
		t.Errorf("b.bin must be the committed content, got %q", got["b.bin"].Data)
	}
	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
		t.Errorf("link = %+v", got["link"])
	}
	for _, absent := range []string{"gitlink", "missing"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s is absent in HEAD mode", absent)
		}
	}
	paths, err := h.Paths()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{".gitattributes", "a.txt", "b.bin", "link"}) {
		t.Errorf("HEAD paths = %v (gitlinks excluded, sorted)", paths)
	}
	if none, err := h.Read([]string{"missing"}); err != nil || len(none) != 0 {
		t.Errorf("reading only absent paths runs nothing: %v %v", none, err)
	}
}

func TestHeadContentErrors(t *testing.T) {
	if _, err := Head(t.TempDir()).Paths(); err == nil {
		t.Error("outside a repository is an error")
	}
	if _, err := Head(t.TempDir()).Read([]string{"a"}); err == nil {
		t.Error("outside a repository is an error")
	}
	root := gitRepo(t, map[string]string{"a.txt": "a"})
	saved := runGit
	t.Cleanup(func() { runGit = saved })
	runGit = func(dir string, stdin []byte, args ...string) ([]byte, error) {
		if args[0] == "cat-file" {
			return nil, errors.New("cat-file failed")
		}
		return saved(dir, stdin, args...)
	}
	if _, err := Head(root).Read([]string{"a.txt"}); err == nil {
		t.Error("a cat-file failure stops the review")
	}
}
```

`internal/mutationfresh/attest_test.go`:

```go
package mutationfresh

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func writeAttestation(t *testing.T, dir string, att map[string]any) {
	t.Helper()
	data, err := json.Marshal(att)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "attestation.json", string(data))
}

func validAttestation(reportSHA string) map[string]any {
	return map[string]any{
		"schemaVersion": 1, "tool": "metareview-mutation-incremental", "engine": "stryker",
		"report": "incremental.json", "reportSha256": reportSHA, "files": map[string]any{},
	}
}

// Spec §6.1: attested iff it parses, schemaVersion is 1, the tool and engine match, `report`
// resolves to the report path and reportSha256 equals the report's bytes; otherwise the first
// failing reason.
func TestReadAttestationReasons(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "incremental.json", "{}")
	report := filepath.Join(dir, "incremental.json")
	sha := sha256Hex([]byte("{}"))
	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "missing" || attSHA != "" {
		t.Errorf("no attestation: %q %q", reason, attSHA)
	}
	write(t, dir, "attestation.json", "not json")
	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "unparseable" || attSHA == "" {
		t.Errorf("not JSON: %q", reason)
	}
	write(t, dir, "attestation.json", `{"schemaVersion":1}`)
	if _, reason, _ := readAttestation(report, "stryker", sha); reason != "unparseable" {
		t.Errorf("no files: %q", reason)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"valid", func(map[string]any) {}, ""},
		{"version", func(a map[string]any) { a["schemaVersion"] = 2 }, "version"},
		{"tool", func(a map[string]any) { a["tool"] = "other" }, "tool"},
		{"engine", func(a map[string]any) { a["engine"] = "gremlins" }, "engine"},
		{"report", func(a map[string]any) { a["report"] = "other.json" }, "report mismatch"},
		{"hash", func(a map[string]any) { a["reportSha256"] = "00" }, "hash mismatch"},
	}
	for _, c := range cases {
		att := validAttestation(sha)
		c.mutate(att)
		writeAttestation(t, dir, att)
		got, reason, _ := readAttestation(report, "stryker", sha)
		if reason != c.want {
			t.Errorf("%s: reason %q, want %q", c.name, reason, c.want)
		}
		if c.want == "" && got.Report != "incremental.json" {
			t.Errorf("valid attestation not returned: %+v", got)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mutationfresh/`
Expected: FAIL. `Worktree`, `Head`, `runGit` and `readAttestation` are undefined.

- [ ] **Step 3: Implement**

`internal/mutationfresh/content.go`:

```go
package mutationfresh

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Content is the code under review (spec §6.2): the working tree for task-done and epic-ready, and
// HEAD for pr-ready unless --include-working-tree.
type Content interface {
	// Read returns the content of every path that exists as a regular file or symlink; any other
	// path is omitted (its digest is Absent). A read error other than absence stops the review.
	Read(paths []string) (map[string]Entry, error)
	// Paths lists every present path, excluding gitlinks.
	Paths() ([]string, error)
	// Head reports HEAD mode, in which an attested untracked path absent from HEAD is not a change.
	Head() bool
}

// runGit runs git in root. A variable so a test can make one subcommand fail.
var runGit = func(root string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mutationfresh: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func splitZ(out []byte) []string {
	var parts []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

type worktree struct{ root string }

// Worktree reads the working tree under root.
func Worktree(root string) Content { return worktree{root: root} }

func (worktree) Head() bool { return false }

// readEntry reads one path: a symlink's link text (never followed) or a regular file's bytes.
// Missing paths, paths under a file, and anything else (a directory) are absent.
func readEntry(full string) (Entry, bool, error) {
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		return Entry{Data: []byte(target), Symlink: true}, true, err
	}
	if !info.Mode().IsRegular() {
		return Entry{}, false, nil
	}
	data, err := os.ReadFile(full)
	return Entry{Data: data}, true, err
}

func (w worktree) Read(paths []string) (map[string]Entry, error) {
	out := map[string]Entry{}
	for _, p := range paths {
		e, ok, err := readEntry(filepath.Join(w.root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("mutationfresh: reading %s: %w", p, err)
		}
		if ok {
			out[p] = e
		}
	}
	return out, nil
}

// Paths is `git ls-files -c -o --exclude-standard --deduplicate` (spec §6.3). A gitlink or a nested
// repository checks out as a directory, which is not content.
func (w worktree) Paths() ([]string, error) {
	listed, err := runGit(w.root, nil, "ls-files", "-c", "-o", "--exclude-standard", "--deduplicate", "-z")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range splitZ(listed) {
		if info, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p))); err == nil && info.IsDir() {
			continue
		}
		out = append(out, strings.TrimSuffix(p, "/"))
	}
	return out, nil
}

type head struct {
	root  string
	modes map[string]string // path → git mode at HEAD
}

// Head reads the committed tree at HEAD through the repository's filters (spec §6.2).
func Head(root string) Content { return &head{root: root} }

func (*head) Head() bool { return true }

func (h *head) load() error {
	if h.modes != nil {
		return nil
	}
	out, err := runGit(h.root, nil, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if err != nil {
		return err
	}
	h.modes = map[string]string{}
	for _, line := range splitZ(out) {
		// "<mode> <type> <object>\t<path>"
		h.modes[line[strings.IndexByte(line, '\t')+1:]] = line[:strings.IndexByte(line, ' ')]
	}
	return nil
}

// Paths is `git ls-tree -r HEAD` without gitlinks (mode 160000), sorted.
func (h *head) Paths() ([]string, error) {
	if err := h.load(); err != nil {
		return nil, err
	}
	var out []string
	for p, mode := range h.modes {
		if mode != "160000" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Read streams every requested blob through one `git cat-file --batch --filters` process (the
// equivalent of `git cat-file --filters HEAD:<path>` per path). A symlink's blob is its link text.
func (h *head) Read(paths []string) (map[string]Entry, error) {
	if err := h.load(); err != nil {
		return nil, err
	}
	var want []string
	var input bytes.Buffer
	for _, p := range paths {
		if mode, ok := h.modes[p]; ok && mode != "160000" {
			want = append(want, p)
			input.WriteString("HEAD:" + p + "\n")
		}
	}
	out := map[string]Entry{}
	if len(want) == 0 {
		return out, nil
	}
	raw, err := runGit(h.root, input.Bytes(), "cat-file", "--batch", "--filters")
	if err != nil {
		return nil, err
	}
	r := bufio.NewReader(bytes.NewReader(raw))
	for _, p := range want {
		header, _ := r.ReadString('\n') // "<object> <type> <size>"
		fields := strings.Fields(header)
		size, _ := strconv.Atoi(fields[len(fields)-1])
		data := make([]byte, size)
		_, _ = io.ReadFull(r, data)
		_, _ = r.ReadByte() // the newline after the content
		out[p] = Entry{Data: data, Symlink: h.modes[p] == "120000"}
	}
	return out, nil
}
```

`internal/mutationfresh/attest.go`:

```go
package mutationfresh

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const harnessTool = "metareview-mutation-incremental"

// Attestation is the part of <stateDir>/attestation.json the gate reads (spec §5.5, §11.2).
type Attestation struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Tool          string               `json:"tool"`
	Engine        string               `json:"engine"`
	Report        string               `json:"report"`
	ReportSha256  string               `json:"reportSha256"`
	Config        string               `json:"config"`
	Lists         Lists                `json:"lists"`
	Exclusions    []string             `json:"exclusions"`
	Files         map[string]FileEntry `json:"files"`
	Deferrals     []Deferral           `json:"deferrals"`
}

// FileEntry is one attested path: its start-of-run digest, category and whether git tracked it.
type FileEntry struct {
	Digest   string `json:"digest"`
	Category string `json:"category"`
	Tracked  bool   `json:"tracked"`
}

// Deferral is recorded pending work: kills in files named by paths (or "*") are pending.
type Deferral struct {
	Reason    string   `json:"reason"`
	Paths     []string `json:"paths"`
	Inherited bool     `json:"inherited"`
}

// readAttestation reads attestation.json beside the report and validates it against the report
// (spec §6.1). It returns the attestation, the first failing reason ("" when attested) and the
// attestation file's sha256 ("" when it cannot be read).
func readAttestation(reportPath, engine, reportSHA string) (Attestation, string, string) {
	path := filepath.Join(filepath.Dir(reportPath), "attestation.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Attestation{}, "missing", ""
	}
	var att Attestation
	if err := json.Unmarshal(data, &att); err != nil || att.Files == nil {
		return Attestation{}, "unparseable", sha256Hex(data)
	}
	return att, attestationReason(att, path, reportPath, engine, reportSHA), sha256Hex(data)
}

func attestationReason(att Attestation, attPath, reportPath, engine, reportSHA string) string {
	switch {
	case att.SchemaVersion != 1:
		return "version"
	case att.Tool != harnessTool:
		return "tool"
	case att.Engine != engine:
		return "engine"
	case !sameFile(filepath.Join(filepath.Dir(attPath), filepath.FromSlash(att.Report)), reportPath):
		return "report mismatch"
	case att.ReportSha256 != reportSHA:
		return "hash mismatch"
	}
	return ""
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}
```

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/mutationfresh/`
Expected: `ok`, 100.0%. First confirm that the local git supports `cat-file --batch --filters`: run `git cat-file --batch --filters <<< HEAD:go.mod | head -1`. If it does not, record a ruling and fall back to per-path `git cat-file --filters HEAD:<path>`.

- [ ] **Step 5: Commit**

```bash
git add internal/mutationfresh
git commit -m "feat(mutationfresh): working-tree and HEAD content, attestation validation

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Stryker detail and kill classification

**Files:**
- Modify: `internal/mutation/stryker.go`, `internal/mutation/parse.go`, `internal/mutation/report.go`, `internal/mutation/stryker_test.go` (append)
- Create: `internal/mutationfresh/classify.go`, `internal/mutationfresh/classify_test.go`

**Interfaces:**
- Produces:
  - `mutation.StrykerDetail{Files map[string]StrykerFile; TestIDs map[string][]string}`;
  - `mutation.StrykerFile{Source string; Mutants []StrykerMutant}`;
  - `mutation.StrykerMutant{ID string; Status Status; KilledBy, CoveredBy []string; StartLine, EndLine int}`;
  - `mutation.Report.SHA256 string` and `mutation.Report.Detail *StrykerDetail`, both `json:"-"`;
  - `mutationfresh.ReportFreshness`, `CauseCount`, `ReRunRow`;
  - `mutationfresh.Classify(r mutation.Report, content Content) (ReportFreshness, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/mutation/stryker_test.go`:

```go
func TestParseKeepsFreshnessDetailOutOfJSON(t *testing.T) {
	data := []byte(`{"schemaVersion":"1","files":{"src/a.ts":{"source":"x","mutants":[
		{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1","t2"],"location":{"start":{"line":2,"column":3},"end":{"line":4,"column":1}}}]}},
		"testFiles":{"tests/a.test.ts":{"tests":[{"id":"t1"},{"id":"t2"}]}}}`)
	r, err := Parse(data, "r.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if r.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("SHA256 %q", r.SHA256)
	}
	m := r.Detail.Files["src/a.ts"].Mutants[0]
	if m.ID != "1" || m.Status != Killed || m.StartLine != 2 || m.EndLine != 4 || len(m.KilledBy) != 1 || len(m.CoveredBy) != 2 {
		t.Errorf("mutant detail %+v", m)
	}
	if r.Detail.Files["src/a.ts"].Source != "x" || len(r.Detail.TestIDs["tests/a.test.ts"]) != 2 {
		t.Errorf("detail %+v", r.Detail)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "coveredBy") || strings.Contains(string(encoded), r.SHA256) {
		t.Errorf("freshness detail must not reach JSON (fingerprints and digests are unchanged): %s", encoded)
	}
}
```

(Add `crypto/sha256`, `encoding/hex`, `encoding/json` and `strings` to that file's imports if they are missing.)

`internal/mutationfresh/classify_test.go`:

```go
package mutationfresh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

// realCase copies a committed real report (Plan 1c's e2e run) and the e2e fixture tree into a fresh
// repository. It rebases every attested path outside src/ and tests/ onto that repository, so only
// the fixture's sources and tests can change, and returns the report path and root.
func realCase(t *testing.T, row string) (report, root string) {
	t.Helper()
	files := map[string]string{}
	fixture := "../../testdata/mutation-incremental-e2e"
	err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(fixture, path)
		data, _ := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	root = gitRepo(t, files)
	src := filepath.Join("../../testdata/mutation-incremental/real", row)
	reportBytes, err := os.ReadFile(filepath.Join(src, "incremental.json"))
	if err != nil {
		t.Fatal(err)
	}
	var att map[string]any
	attBytes, _ := os.ReadFile(filepath.Join(src, "attestation.json"))
	if err := json.Unmarshal(attBytes, &att); err != nil {
		t.Fatal(err)
	}
	current, _ := Worktree(root).Read(nil)
	_ = current
	entries := att["files"].(map[string]any)
	for path, raw := range entries {
		if hasPrefix(path, "src/") || hasPrefix(path, "tests/") {
			continue
		}
		entry := raw.(map[string]any)
		got, _ := Worktree(root).Read([]string{path})
		if e, ok := got[path]; ok {
			entry["digest"] = digestOf(e)
		} else {
			entry["digest"] = Absent
		}
	}
	att["config"] = ""
	dir := t.TempDir()
	write(t, dir, "incremental.json", string(reportBytes))
	out, _ := json.Marshal(att)
	write(t, dir, "attestation.json", string(out))
	return filepath.Join(dir, "incremental.json"), root
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func classifyAt(t *testing.T, report, root string) ReportFreshness {
	t.Helper()
	r, err := mutation.Load(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Classify(r, Worktree(root))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func causes(f ReportFreshness) map[string]int {
	out := map[string]int{}
	for _, c := range f.Causes {
		out[c.Cause] = c.Kills
	}
	return out
}

// Spec §6.3 on real reports: 24 kills (a.ts 12, b.ts 5, c.ts 3, e.ts 4).
func TestClassifyRealReportsUnchangedTreeIsVerified(t *testing.T) {
	report, root := realCase(t, "full")
	f := classifyAt(t, report, root)
	if !f.Attested || f.Verified != 24 || f.Stale+f.Pending+f.Unbound+f.Unattested != 0 {
		t.Errorf("unchanged tree: %+v", f)
	}
}

func TestClassifyRealReportsEditedSourceStalesItsKillsAndTheirCoverageClosure(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "src/a.ts", "export function grade(score: number, pass: number, top: number): string {\n  return 'ok';\n}\n")
	f := classifyAt(t, report, root)
	// a.ts's own 12 kills, plus b.ts's 5: killed by b.test, whose ids cover a.ts's mutants.
	if f.Stale != 17 || f.Verified != 7 || causes(f)["src/a.ts"] != 17 {
		t.Errorf("edited a.ts: %+v", f)
	}
	if len(f.ReRun) != 2 || f.ReRun[0].File != "src/a.ts" || f.ReRun[1].File != "src/b.ts" || f.ReRun[1].Cause != "src/a.ts" {
		t.Errorf("re-run rows %+v", f.ReRun)
	}
}

func TestClassifyRealReportsEditedTestStalesWhatItKilled(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "tests/c.test.ts", "// rewritten\n")
	f := classifyAt(t, report, root)
	if f.Stale != 3 || causes(f)["tests/c.test.ts"] != 3 || f.Verified != 21 {
		t.Errorf("edited c.test: %+v", f)
	}
}

func TestClassifyRealReportsGlobalAndZeroMutantEditsStaleEverything(t *testing.T) {
	for path, text := range map[string]string{"package-lock.json": "{}\n", "src/limits.ts": "export const LIMIT = 99;\n"} {
		report, root := realCase(t, "full")
		write(t, root, path, text)
		f := classifyAt(t, report, root)
		if f.Stale != 24 || causes(f)[path] != 24 {
			t.Errorf("edited %s: %+v", path, f)
		}
	}
}

func TestClassifyRealReportsDeferralsArePending(t *testing.T) {
	report, root := realCase(t, "lockfile") // "global input changed: package-lock.json" on ["*"]
	f := classifyAt(t, report, root)
	if f.Pending != 24 || f.Deferrals[0] != "global input changed: package-lock.json" {
		t.Errorf("lockfile row: %+v", f)
	}
	report, root = realCase(t, "delete-c-test") // "no reachable tests: src/c.ts"
	f = classifyAt(t, report, root)
	if f.Pending != 3 || f.Verified != 21 {
		t.Errorf("delete-c-test row: %+v", f)
	}
}

// Rules the real reports do not reach: a changed test file ranks before a changed mutate file,
// the byte-smallest cause wins a tie, an unattested file or a kill without killedBy is unbound, a
// new support file present only in the tree is a blanket cause, a new mutate or test file is not,
// and in HEAD mode an untracked attested path absent from HEAD is not a change.
func TestClassifySyntheticRules(t *testing.T) {
	root := gitRepo(t, map[string]string{"src/a.ts": "a", "src/b.ts": "b", "tests/a.test.ts": "t", "tests/b.test.ts": "u"})
	dir := t.TempDir()
	report := `{"files":{
		"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1","t2"],"coveredBy":["t1","t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
		"src/b.ts":{"source":"b","mutants":[{"id":"2","mutatorName":"M","status":"Killed","killedBy":["t2"],"coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}},
		                                    {"id":"3","mutatorName":"M","status":"Killed","killedBy":[],"coveredBy":[],"location":{"start":{"line":1},"end":{"line":1}}},
		                                    {"id":"4","mutatorName":"M","status":"Survived","coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
		"gen/x.ts":{"source":"g","mutants":[{"id":"5","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},
		"testFiles":{"tests/a.test.ts":{"tests":[{"id":"t1"}]},"tests/b.test.ts":{"tests":[{"id":"t2"}]}}}`
	write(t, dir, "incremental.json", report)
	files := map[string]any{}
	digests, _ := Worktree(root).Read([]string{"src/a.ts", "src/b.ts", "tests/a.test.ts", "tests/b.test.ts"})
	for path, category := range map[string]string{"src/a.ts": "mutate", "src/b.ts": "mutate", "tests/a.test.ts": "test", "tests/b.test.ts": "test"} {
		files[path] = map[string]any{"digest": digestOf(digests[path]), "category": category, "tracked": true}
	}
	files["gone.txt"] = map[string]any{"digest": "sha256:0", "category": "unclassified", "tracked": false}
	att := validAttestation(sha256Hex([]byte(report)))
	att["files"] = files
	att["lists"] = map[string]any{"mutate": []string{"src/**"}, "test": []string{"tests/**"}, "support": []string{"helpers/**"}, "global": []string{}, "ignore": []string{"**/*.md"}}
	att["exclusions"] = []string{".mutation/**"}
	writeAttestation(t, dir, att)
	path := filepath.Join(dir, "incremental.json")

	// Worktree: gone.txt is attested but missing, so it is a changed unclassified path, a blanket cause.
	f := classifyAt(t, path, root)
	if f.Stale != 3 || causes(f)["gone.txt"] != 3 {
		t.Errorf("missing attested path: %+v", f)
	}
	// HEAD mode: an untracked attested path absent from HEAD is skipped; everything is unchanged.
	r, _ := mutation.Load(path)
	head, err := Classify(r, Head(root))
	if err != nil {
		t.Fatal(err)
	}
	if head.Stale != 0 || head.Verified != 2 || head.Unbound != 2 {
		t.Errorf("HEAD mode: %+v (a.ts, b.ts#2 verified; b.ts#3 has no killedBy and gen/x.ts is unattested: unbound)", head)
	}
	// Both tests changed: a.ts's kill takes the byte-smallest test; b.ts's kill takes b.test.
	write(t, root, "tests/a.test.ts", "t2")
	write(t, root, "tests/b.test.ts", "u2")
	write(t, root, "src/new.ts", "n")      // a new mutate file cannot invalidate a kill
	write(t, root, "tests/new.test.ts", "n") // nor can a new test
	write(t, root, "notes.md", "n")          // ignored
	r, _ = mutation.Load(path)
	f, _ = Classify(r, Head(root))
	if f.Stale != 0 {
		t.Errorf("uncommitted edits are not seen in HEAD mode: %+v", f)
	}
	f = classifyAt(t, path, root)
	if causes(f)["tests/a.test.ts"] != 2 || causes(f)["tests/b.test.ts"] != 1 {
		t.Errorf("test causes: %+v", f.Causes)
	}
	// A changed mutate file (rule 3): b.ts changed only; a.ts's kill is killed by t2, which covers b.ts.
	root2 := gitRepo(t, map[string]string{"src/a.ts": "a", "src/b.ts": "b2", "tests/a.test.ts": "t", "tests/b.test.ts": "u", "gone.txt": "x"})
	_ = root2
	write(t, root, "tests/a.test.ts", "t")
	write(t, root, "tests/b.test.ts", "u")
	write(t, root, "src/b.ts", "b2")
	write(t, root, "gone.txt", "g")
	f = classifyAt(t, path, root)
	if causes(f)["src/b.ts"] != 3 {
		t.Errorf("rule 3 and own-file: %+v", f.Causes) // b.ts's own kill, a.ts via coverage, gen/x.ts via gone.txt? no: see below
	}
	// A new support file present only in the tree is a blanket cause.
	write(t, root, "src/b.ts", "b")
	write(t, root, "helpers/h.ts", "h")
	f = classifyAt(t, path, root)
	if causes(f)["gone.txt"] == 0 && causes(f)["helpers/h.ts"] == 0 {
		t.Errorf("a new support path is a blanket cause: %+v", f.Causes)
	}
}

func TestClassifyUnattestedAndGremlins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	r, _ := mutation.Load(filepath.Join(dir, "incremental.json"))
	f, err := Classify(r, Worktree(t.TempDir()))
	if err != nil || f.Attested || f.UnattestedReason != "missing" || f.Unattested != 1 {
		t.Errorf("unattested stryker: %+v %v", f, err)
	}
	// A parse without Stryker detail (gremlins) is never attested, even beside a harness attestation.
	g := mutation.Report{Engine: "stryker", Target: filepath.Join(dir, "incremental.json"), SHA256: r.SHA256,
		Mutants: []mutation.Mutant{{Status: mutation.Killed}}}
	writeAttestation(t, dir, validAttestation(r.SHA256))
	f, _ = Classify(g, Worktree(t.TempDir()))
	if f.UnattestedReason != "engine" {
		t.Errorf("no detail: %+v", f)
	}
}

func TestClassifyStopsOnContentErrors(t *testing.T) {
	report, _ := realCase(t, "full")
	r, _ := mutation.Load(report)
	if _, err := Classify(r, Worktree(t.TempDir())); err == nil {
		t.Error("listing paths outside a repository stops the review")
	}
	root := gitRepo(t, map[string]string{"src/a.ts": "a"})
	if err := os.Chmod(filepath.Join(root, "src/a.ts"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := Classify(r, Worktree(root)); err == nil {
		t.Error("an unreadable attested path stops the review")
	}
}
```

Note for the executor: the synthetic test's middle assertions are marked with comments where the expected counts depend on rule order. When a count differs, first derive the expected value by hand from §6.3's rule order, then correct the assertion. Never change `Classify` to fit an assertion without that derivation. Record a ruling for any assertion corrected.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mutation/ ./internal/mutationfresh/`
Expected: FAIL. `SHA256`, `Detail` and `Classify` are undefined.

- [ ] **Step 3: Implement**

In `internal/mutation/report.go`, add to `Report`:

```go
	// SHA256 is the report file's sha256 (lowercase hex); Detail is the Stryker data the freshness
	// gate reads (spec §6.1). Neither reaches JSON, so fingerprints and digests are unchanged.
	SHA256 string         `json:"-"`
	Detail *StrykerDetail `json:"-"`
```

In `internal/mutation/stryker.go`:
- Extend `strykerReport`. Each file gains `Source string \`json:"source"\``. Each mutant gains `ID string \`json:"id"\``, `KilledBy []string \`json:"killedBy"\``, `CoveredBy []string \`json:"coveredBy"\`` and `Location.End struct{ Line int \`json:"line"\` } \`json:"end"\``. Add a top-level `TestFiles map[string]struct{ Tests []struct{ ID string \`json:"id"\` } \`json:"tests"\` } \`json:"testFiles"\``.
- Add the detail types below, and fill them in `ParseStryker`.

```go
// StrykerDetail is what the freshness gate reads from a mutation-testing-report-schema report
// (spec §6.1). Ids are meaningful only within one report.
type StrykerDetail struct {
	Files   map[string]StrykerFile
	TestIDs map[string][]string // test file → its test ids
}

// StrykerFile is one mutated file: the source Stryker read and its mutants.
type StrykerFile struct {
	Source  string
	Mutants []StrykerMutant
}

// StrykerMutant carries the ids and span freshness needs.
type StrykerMutant struct {
	ID        string
	Status    Status
	KilledBy  []string
	CoveredBy []string
	StartLine int
	EndLine   int
}
```

In `ParseStryker`, after building `r`:

```go
	r.Detail = &StrykerDetail{Files: map[string]StrykerFile{}, TestIDs: map[string][]string{}}
	for path, f := range raw.Files {
		file := StrykerFile{Source: f.Source}
		for _, m := range f.Mutants {
			file.Mutants = append(file.Mutants, StrykerMutant{
				ID: m.ID, Status: strykerStatus(m.Status), KilledBy: m.KilledBy, CoveredBy: m.CoveredBy,
				StartLine: m.Location.Start.Line, EndLine: m.Location.End.Line,
			})
		}
		r.Detail.Files[path] = file
	}
	for path, tf := range raw.TestFiles {
		for _, test := range tf.Tests {
			r.Detail.TestIDs[path] = append(r.Detail.TestIDs[path], test.ID)
		}
	}
```

In `internal/mutation/parse.go` `Parse`, set the digest on every successful parse. Replace each `return checkMeasured(r, target)` with:

```go
			r, err = checkMeasured(r, target)
			r.SHA256 = reportSHA256(data)
			return r, err
```

and add:

```go
func reportSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
```

(Import `crypto/sha256` and `encoding/hex`. Setting the digest on a report that `checkMeasured` rejected is harmless: callers discard it along with the error.)

`internal/mutationfresh/classify.go`:

```go
package mutationfresh

import (
	"sort"

	"github.com/dsifry/metareview/internal/mutation"
)

// ReportFreshness is one report's classification (spec §6.3). It is serialized into pr-ready's
// reviewer-input digest (§6.7), so it holds no machine-dependent path.
type ReportFreshness struct {
	Path              string       `json:"-"`
	Engine            string       `json:"engine"`
	Attested          bool         `json:"attested"`
	UnattestedReason  string       `json:"unattestedReason,omitempty"`
	ReportSha256      string       `json:"reportSha256"`
	AttestationSha256 string       `json:"attestationSha256,omitempty"`
	Verified          int          `json:"verified"`
	Stale             int          `json:"stale"`
	Pending           int          `json:"pending"`
	Unbound           int          `json:"unbound"`
	Unattested        int          `json:"unattested"`
	Causes            []CauseCount `json:"staleCauses,omitempty"`
	ReRun             []ReRunRow   `json:"reRun,omitempty"`
	Deferrals         []string     `json:"deferrals,omitempty"`
}

// CauseCount is one recorded cause: the changed path, its current digest and the kills it stales.
type CauseCount struct {
	Cause  string `json:"cause"`
	Digest string `json:"digest"`
	Kills  int    `json:"kills"`
}

// ReRunRow is one (file, cause) row of the re-run list (§6.6).
type ReRunRow struct {
	File  string `json:"file"`
	Cause string `json:"cause"`
	Kills int    `json:"kills"`
}

type change struct{ category, digest string }

// Classify reads the report's attestation and gives every kill exactly one class (spec §6.3):
// stale (first cause wins), pending, unbound or verified. Kills in an unattested report are
// unattested. Survivors and every other status are never reclassified.
func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
	out := ReportFreshness{Path: r.Target, Engine: r.Engine, ReportSha256: r.SHA256}
	att, reason, attSHA := readAttestation(r.Target, r.Engine, r.SHA256)
	out.AttestationSha256 = attSHA
	if reason == "" && r.Detail == nil {
		reason = "engine" // only the harness attests, and it attests Stryker reports
	}
	if reason != "" {
		out.UnattestedReason = reason
		out.Unattested = r.Score().Killed
		return out, nil
	}
	out.Attested = true
	for _, d := range att.Deferrals {
		out.Deferrals = append(out.Deferrals, d.Reason)
	}
	changed, err := changedPaths(att, content)
	if err != nil {
		return ReportFreshness{}, err
	}
	causeOf := causeFinder(r.Detail, changed)
	stale := map[string]int{}
	rows := map[ReRunRow]int{}
	for _, file := range sortedKeys(r.Detail.Files) {
		for _, m := range r.Detail.Files[file].Mutants {
			if m.Status != mutation.Killed {
				continue
			}
			if cause := causeOf(file, m.KilledBy); cause != "" {
				out.Stale++
				stale[cause]++
				rows[ReRunRow{File: file, Cause: cause}]++
				continue
			}
			_, attested := att.Files[file]
			switch {
			case deferred(att.Deferrals, file):
				out.Pending++
			case !attested || len(m.KilledBy) == 0:
				out.Unbound++
			default:
				out.Verified++
			}
		}
	}
	for _, cause := range sortedKeys(stale) {
		out.Causes = append(out.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: stale[cause]})
	}
	for row, kills := range rows {
		out.ReRun = append(out.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
	}
	sort.Slice(out.ReRun, func(i, j int) bool {
		if out.ReRun[i].File != out.ReRun[j].File {
			return out.ReRun[i].File < out.ReRun[j].File
		}
		return out.ReRun[i].Cause < out.ReRun[j].Cause
	})
	return out, nil
}

// changedPaths: attested paths whose current digest differs (in HEAD mode, an untracked attested
// path absent from HEAD is skipped), plus present paths that are not attested, not excluded, not
// ignored, and neither mutate nor test (which cannot invalidate a kill). An unknown category counts
// as a blanket cause, as global does.
func changedPaths(att Attestation, content Content) (map[string]change, error) {
	present, err := content.Paths()
	if err != nil {
		return nil, err
	}
	var fresh []string
	for _, p := range present {
		if _, attested := att.Files[p]; attested || MatchList(p, att.Exclusions) {
			continue
		}
		switch Categorize(p, att.Lists) {
		case "ignore", "mutate", "test":
			continue
		}
		fresh = append(fresh, p)
	}
	paths := append(sortedKeys(att.Files), fresh...)
	entries, err := content.Read(paths)
	if err != nil {
		return nil, err
	}
	changed := map[string]change{}
	for _, p := range paths {
		digest := Absent
		if e, ok := entries[p]; ok {
			digest = digestOf(e)
			if p == att.Config && !e.Symlink {
				digest = configDigest(e.Data)
			}
		}
		entry, attested := att.Files[p]
		if !attested {
			changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
			continue
		}
		if digest == entry.Digest || (content.Head() && !entry.Tracked && digest == Absent) {
			continue
		}
		changed[p] = change{category: entry.Category, digest: digest}
	}
	return changed, nil
}

// causeFinder returns a kill's recorded cause, in the §6.3 order: its own file; the byte-smallest
// changed test whose ids it was killed by; the byte-smallest changed mutate file with mutants whose
// coverage includes a killing test; else the byte-smallest changed support, global, unclassified or
// zero-mutant mutate path (the gate has no import graph, so these invalidate every kill).
func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file string, killedBy []string) string {
	testOf := map[string]string{}
	type covering struct {
		path string
		ids  map[string]bool
	}
	var mutated []covering
	var blanket []string
	for _, p := range sortedKeys(changed) {
		switch c := changed[p]; {
		case c.category == "test":
			for _, id := range d.TestIDs[p] {
				if _, seen := testOf[id]; !seen {
					testOf[id] = p
				}
			}
		case c.category == "mutate" && len(d.Files[p].Mutants) > 0:
			ids := map[string]bool{}
			for _, m := range d.Files[p].Mutants {
				for _, id := range m.CoveredBy {
					ids[id] = true
				}
			}
			mutated = append(mutated, covering{path: p, ids: ids})
		default:
			blanket = append(blanket, p)
		}
	}
	return func(file string, killedBy []string) string {
		if _, ok := changed[file]; ok {
			return file
		}
		best := ""
		for _, id := range killedBy {
			if p, ok := testOf[id]; ok && (best == "" || p < best) {
				best = p
			}
		}
		if best != "" {
			return best
		}
		for _, y := range mutated {
			for _, id := range killedBy {
				if y.ids[id] {
					return y.path
				}
			}
		}
		if len(blanket) > 0 {
			return blanket[0]
		}
		return ""
	}
}

func deferred(deferrals []Deferral, file string) bool {
	for _, d := range deferrals {
		for _, p := range d.Paths {
			if p == "*" || p == file {
				return true
			}
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/mutation/ ./internal/mutationfresh/`
Expected: `ok`, both at 100.0%. The real-report tests are the Review Focus 1 evidence.

- [ ] **Step 5: Commit**

```bash
git add internal/mutation internal/mutationfresh
git commit -m "feat(mutationfresh): classify kills against the attestation (verified, stale, pending, unbound)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Findings, the review-log section and the mode

**Files:**
- Create: `internal/mutationfresh/build.go`, `internal/mutationfresh/mode.go`, `internal/mutationfresh/build_test.go`

**Interfaces:**
- Produces:
  - the constants `Advisory`, `Enforce` and `ModeEnv`;
  - `ModeFromEnv(getenv func(string) string) (string, error)`;
  - `EffectiveMode(scope, mode string) string`;
  - `Result{Mode string; Freshness []ReportFreshness; Findings []findings.Input; Section string}`;
  - `Build(reports []mutation.Report, content Content, mode string) (Result, error)`;
  - `IsFreshnessFingerprint(fp string) bool`, which lives in `internal/findings`, not here (Task 6).

**Finding texts** (fixed here, spec §6.5):

| Kind | Title | Finding | Expected | Recommendation | Owner | Evidence path |
|---|---|---|---|---|---|---|
| stale | `Mutation evidence stale: <cause> changed` | `<n> kill(s) in <reports> were verified against a different <cause>, so they are not evidence for the code under review.` | `Every kill the gate counts was verified against the content under review.` | `Re-run the harness (node tools/mutation-incremental/cli.mjs run --mode incremental), commit before running pr-ready, and pass the refreshed <stateDir>/incremental.json.` | implementer | cause |
| pending | `Mutation evidence pending: <n> kills` | `<n> kill(s) in <report> are covered by a deferral (<reasons>) and wait for a full run, so they are not counted as evidence.` | `Deferred work is re-verified by a full run before it counts.` | `Let main's full run clear the deferrals, or run the harness with --mode full locally.` | reviewer | report |
| unattested | `Mutation report has no attestation` | `<report> has no valid attestation (<reason>), so the gate cannot tell whether its <n> kill(s) still describe the code under review.` | `Mutation evidence comes from the mutation-incremental harness, which attests what it verified.` | `Produce the report with the harness and pass <stateDir>/incremental.json (its attestation.json sits beside it).` | reviewer | report |

The `Found` field is `<cause> is now <digest>.` for stale, and empty otherwise.

- [ ] **Step 1: Write the failing tests**

`internal/mutationfresh/build_test.go`:

```go
package mutationfresh

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

func TestModeFromEnv(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	for value, want := range map[string]string{"": Advisory, "advisory": Advisory, "enforce": Enforce} {
		if got, err := ModeFromEnv(env(value)); err != nil || got != want {
			t.Errorf("%q: %q %v", value, got, err)
		}
	}
	if _, err := ModeFromEnv(env("strict")); err == nil || !strings.Contains(err.Error(), ModeEnv) {
		t.Errorf("an unknown mode is a usage error naming %s: %v", ModeEnv, err)
	}
	if EffectiveMode("task-done", Enforce) != Advisory || EffectiveMode("pr-ready", Enforce) != Enforce {
		t.Error("task-done is always advisory; the other gates use the mode")
	}
}

func buildAt(t *testing.T, root, mode string, reports ...string) Result {
	t.Helper()
	var loaded []mutation.Report
	for _, p := range reports {
		r, err := mutation.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		loaded = append(loaded, r)
	}
	res, err := Build(loaded, Worktree(root), mode)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestBuildStaleFindingsSumCausesAcrossReportsAndFollowTheMode(t *testing.T) {
	report, root := realCase(t, "full")
	// A second copy of the same report and attestation, elsewhere: same cause, summed counts.
	copyDir := t.TempDir()
	for _, name := range []string{"incremental.json", "attestation.json"} {
		data := readFile(t, filepath.Join(filepath.Dir(report), name))
		write(t, copyDir, name, data)
	}
	write(t, root, "src/a.ts", "changed\n")
	res := buildAt(t, root, Advisory, report, filepath.Join(copyDir, "incremental.json"))
	if len(res.Findings) != 1 {
		t.Fatalf("one finding per distinct cause, got %d: %+v", len(res.Findings), res.Findings)
	}
	f := res.Findings[0]
	if f.Title != "Mutation evidence stale: src/a.ts changed" || !strings.HasPrefix(f.Finding, "34 kill(s) in ") ||
		f.Classification != "advisory" || f.Severity != "medium" || f.Owner != "implementer" || f.Reviewer != "mutation-freshness" {
		t.Errorf("advisory stale finding %+v", f)
	}
	digest := res.Freshness[0].Causes[0].Digest
	if f.Fingerprint != "mutation:stale:advisory:stryker:src/a.ts:"+sha256Hex([]byte("src/a.ts="+digest))[:8] || f.Evidence[0].Path != "src/a.ts" {
		t.Errorf("fingerprint %q evidence %+v", f.Fingerprint, f.Evidence)
	}
	enforced := buildAt(t, root, Enforce, report)
	if e := enforced.Findings[0]; e.Classification != "blocking" || e.Severity != "high" || !strings.HasPrefix(e.Fingerprint, "mutation:stale:enforce:") {
		t.Errorf("enforce: %+v", e)
	}
}

func TestBuildPendingStaysAdvisoryAndUnattestedFollowsTheMode(t *testing.T) {
	report, root := realCase(t, "lockfile")
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	gremlins := t.TempDir()
	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
	res := buildAt(t, root, Enforce, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"))
	byPrefix := map[string]int{}
	for _, f := range res.Findings {
		byPrefix[strings.Join(strings.SplitN(f.Fingerprint, ":", 3)[:2], ":")]++
		switch {
		case strings.HasPrefix(f.Fingerprint, "mutation:pending:enforce:stryker:"):
			if f.Classification != "advisory" || f.Title != "Mutation evidence pending: 24 kills" || !strings.Contains(f.Finding, "global input changed: package-lock.json") {
				t.Errorf("pending stays advisory under enforce: %+v", f)
			}
		case strings.HasPrefix(f.Fingerprint, "mutation:unattested:enforce:stryker:"):
			if f.Classification != "blocking" || f.Severity != "high" || !strings.Contains(f.Finding, "(missing)") {
				t.Errorf("unattested stryker blocks under enforce: %+v", f)
			}
		}
	}
	if byPrefix["mutation:pending"] != 1 || byPrefix["mutation:unattested"] != 1 {
		t.Errorf("one pending, one unattested (gremlins gets none): %v", byPrefix)
	}
	advisory := buildAt(t, root, Advisory, filepath.Join(dir, "incremental.json"))
	if advisory.Findings[0].Classification != "advisory" || advisory.Findings[0].Severity != "medium" {
		t.Errorf("unattested is advisory by default: %+v", advisory.Findings[0])
	}
}

func TestBuildSection(t *testing.T) {
	report, root := realCase(t, "full")
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	gremlins := t.TempDir()
	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
	lockReport, _ := realCase(t, "lockfile")
	write(t, root, "tests/c.test.ts", "// changed\n")
	res := buildAt(t, root, Advisory, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"), lockReport)
	want := "## Mutation Evidence Freshness\n\n" +
		"- Mode: `advisory`\n" +
		"- Reports: 4 (2 attested, 2 unattested: missing, missing)\n" +
		"- Kills: 21 verified, 3 stale, 21 pending, 0 unbound, 2 unattested\n" +
		"- Deferrals: global input changed: package-lock.json\n" +
		"- Freshness not verifiable: gremlins\n\n" +
		"### Re-run list\n\n" +
		"| File | Cause | Stale kills |\n| --- | --- | ---: |\n" +
		"| `src/c.ts` | `tests/c.test.ts` | 6 |"
	if res.Section != want {
		t.Errorf("section:\n%s\n--- want ---\n%s", res.Section, want)
	}
	if empty, _ := Build(nil, Worktree(root), Advisory); empty.Section != "" || len(empty.Findings) != 0 {
		t.Errorf("no reports, no section: %+v", empty)
	}
}

func TestBuildStopsOnContentErrors(t *testing.T) {
	report, _ := realCase(t, "full")
	r, _ := mutation.Load(report)
	if _, err := Build([]mutation.Report{r}, Worktree(t.TempDir()), Advisory); err == nil {
		t.Error("a content error stops the review")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
```

(Add `"os"` to the imports.) The section test's expected counts are hand-derived:
- **`full` with `tests/c.test.ts` edited:** 3 stale (`c.ts`), 21 verified.
- **The lockfile row with `tests/c.test.ts` edited:** its `c.ts` kills are stale (stale is checked before pending), 3 more. That gives 6 stale rows for (`src/c.ts`, `tests/c.test.ts`), and the other 21 are pending.
- **The unattested Stryker report and gremlins:** 1 unattested kill each.

If a real count differs, re-derive it from §6.3 before touching either side, and record a ruling.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/mutationfresh/`
Expected: FAIL. `Build`, `ModeFromEnv` and `EffectiveMode` are undefined.

- [ ] **Step 3: Implement**

`internal/mutationfresh/mode.go`:

```go
package mutationfresh

import "fmt"

// Modes (spec §6.4).
const (
	Advisory = "advisory"
	Enforce  = "enforce"
	ModeEnv  = "METAREVIEW_MUTATION_FRESHNESS"
)

// ModeFromEnv reads METAREVIEW_MUTATION_FRESHNESS: unset, empty or advisory is advisory; enforce is
// enforce; anything else is a usage error.
func ModeFromEnv(getenv func(string) string) (string, error) {
	switch value := getenv(ModeEnv); value {
	case "", Advisory:
		return Advisory, nil
	case Enforce:
		return Enforce, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q, got %q", ModeEnv, Advisory, Enforce, value)
	}
}

// EffectiveMode: task-done is always advisory; pr-ready and epic-ready use the configured mode.
func EffectiveMode(scope, mode string) string {
	if scope == "task-done" {
		return Advisory
	}
	return mode
}
```

`internal/mutationfresh/build.go`:

```go
package mutationfresh

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/mutation"
)

const reviewer = "mutation-freshness"

// Result is what a gate needs from the freshness check: the classifications (for pr-ready's
// digest), the findings, and the review-log section.
type Result struct {
	Mode      string
	Freshness []ReportFreshness
	Findings  []findings.Input
	Section   string
}

// Build classifies every report, then derives the findings (spec §6.5) and the section (§6.6).
func Build(reports []mutation.Report, content Content, mode string) (Result, error) {
	res := Result{Mode: mode}
	for _, r := range reports {
		f, err := Classify(r, content)
		if err != nil {
			return Result{}, err
		}
		res.Freshness = append(res.Freshness, f)
	}
	res.Findings = freshnessFindings(res.Freshness, mode)
	res.Section = section(res.Freshness, mode)
	return res, nil
}

func gate(mode string) (classification, severity string) {
	if mode == Enforce {
		return "blocking", "high" // classForCount counts blocking only at high or critical
	}
	return "advisory", "medium"
}

func freshnessFindings(list []ReportFreshness, mode string) []findings.Input {
	type staleKey struct{ engine, cause string }
	type staleSum struct {
		digest  string
		kills   int
		reports []string
	}
	sums := map[staleKey]*staleSum{}
	for _, f := range list {
		for _, c := range f.Causes {
			key := staleKey{f.Engine, c.Cause}
			if sums[key] == nil {
				sums[key] = &staleSum{digest: c.Digest}
			}
			sums[key].kills += c.Kills
			sums[key].reports = append(sums[key].reports, f.Path)
		}
	}
	keys := make([]staleKey, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].engine != keys[j].engine {
			return keys[i].engine < keys[j].engine
		}
		return keys[i].cause < keys[j].cause
	})
	classification, severity := gate(mode)
	var out []findings.Input
	for _, k := range keys {
		s := sums[k]
		out = append(out, findings.Input{
			Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "implementer",
			Title:          "Mutation evidence stale: " + k.cause + " changed",
			Finding:        fmt.Sprintf("%d kill(s) in %s were verified against a different %s, so they are not evidence for the code under review.", s.kills, strings.Join(s.reports, ", "), k.cause),
			Expected:       "Every kill the gate counts was verified against the content under review.",
			Found:          fmt.Sprintf("%s is now %s.", k.cause, s.digest),
			Recommendation: "Re-run the harness (node tools/mutation-incremental/cli.mjs run --mode incremental), commit before running pr-ready, and pass the refreshed <stateDir>/incremental.json.",
			Evidence:       []findings.Evidence{{Type: "mutant", Path: k.cause}},
			Fingerprint:    fmt.Sprintf("mutation:stale:%s:%s:%s:%s", mode, k.engine, k.cause, sha256Hex([]byte(k.cause+"="+s.digest))[:8]),
		})
	}
	for _, f := range list {
		if f.Pending > 0 {
			out = append(out, findings.Input{
				Reviewer: reviewer, Severity: "medium", Classification: "advisory", Owner: "reviewer",
				Title:          fmt.Sprintf("Mutation evidence pending: %d kills", f.Pending),
				Finding:        fmt.Sprintf("%d kill(s) in %s are covered by a deferral (%s) and wait for a full run, so they are not counted as evidence.", f.Pending, f.Path, strings.Join(f.Deferrals, "; ")),
				Expected:       "Deferred work is re-verified by a full run before it counts.",
				Recommendation: "Let main's full run clear the deferrals, or run the harness with --mode full locally.",
				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
				Fingerprint:    fmt.Sprintf("mutation:pending:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
			})
		}
		if !f.Attested && f.Engine == "stryker" {
			out = append(out, findings.Input{
				Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "reviewer",
				Title:          "Mutation report has no attestation",
				Finding:        fmt.Sprintf("%s has no valid attestation (%s), so the gate cannot tell whether its %d kill(s) still describe the code under review.", f.Path, f.UnattestedReason, f.Unattested),
				Expected:       "Mutation evidence comes from the mutation-incremental harness, which attests what it verified.",
				Recommendation: "Produce the report with the harness and pass <stateDir>/incremental.json (its attestation.json sits beside it).",
				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
				Fingerprint:    fmt.Sprintf("mutation:unattested:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
			})
		}
	}
	return out
}

// section renders "## Mutation Evidence Freshness" (spec §6.6), or "" without reports.
func section(list []ReportFreshness, mode string) string {
	if len(list) == 0 {
		return ""
	}
	var attested, verified, stale, pending, unbound, unattested int
	var reasons, deferrals, unverifiable []string
	seenDeferral, seenEngine := map[string]bool{}, map[string]bool{}
	rows := map[[2]string]int{}
	for _, f := range list {
		verified += f.Verified
		stale += f.Stale
		pending += f.Pending
		unbound += f.Unbound
		unattested += f.Unattested
		if f.Attested {
			attested++
		} else {
			reasons = append(reasons, f.UnattestedReason)
			if f.Engine != "stryker" && !seenEngine[f.Engine] {
				seenEngine[f.Engine] = true
				unverifiable = append(unverifiable, f.Engine)
			}
		}
		for _, d := range f.Deferrals {
			if !seenDeferral[d] {
				seenDeferral[d] = true
				deferrals = append(deferrals, d)
			}
		}
		for _, row := range f.ReRun {
			rows[[2]string{row.File, row.Cause}] += row.Kills
		}
	}
	var b strings.Builder
	b.WriteString("## Mutation Evidence Freshness\n\n")
	fmt.Fprintf(&b, "- Mode: `%s`\n", mode)
	fmt.Fprintf(&b, "- Reports: %d (%d attested", len(list), attested)
	if len(reasons) > 0 {
		fmt.Fprintf(&b, ", %d unattested: %s", len(reasons), strings.Join(reasons, ", "))
	}
	b.WriteString(")\n")
	fmt.Fprintf(&b, "- Kills: %d verified, %d stale, %d pending, %d unbound, %d unattested\n", verified, stale, pending, unbound, unattested)
	if len(deferrals) > 0 {
		fmt.Fprintf(&b, "- Deferrals: %s\n", strings.Join(deferrals, "; "))
	}
	if len(unverifiable) > 0 {
		sort.Strings(unverifiable)
		fmt.Fprintf(&b, "- Freshness not verifiable: %s\n", strings.Join(unverifiable, ", "))
	}
	if len(rows) > 0 {
		keys := make([][2]string, 0, len(rows))
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})
		b.WriteString("\n### Re-run list\n\n| File | Cause | Stale kills |\n| --- | --- | ---: |\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "| `%s` | `%s` | %d |\n", k[0], k[1], rows[k])
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/mutationfresh/`
Expected: `ok`, 100.0%.

- [ ] **Step 5: Commit**

```bash
git add internal/mutationfresh
git commit -m "feat(mutationfresh): freshness findings, the review-log section and the mode

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 6: The ledger lifecycle for freshness findings (spec §6.5)

**Files:**
- Modify: `internal/findings/findings.go`, `internal/findings/override.go`
- Create: `internal/findings/freshness_test.go`

**Interfaces:**
- Produces:
  - `findings.Options.MutationEngines []string`: the engines of this run's reports; empty when none were supplied;
  - `findings.IsFreshnessFingerprint(fp string) bool`;
  - `findings.OnlyStaleBlockers(records []Record) bool` (used by Task 8).

**Behaviour.** The lifecycle applies only to the three prefixes `mutation:stale:`, `mutation:pending:` and `mutation:unattested:`:
- **Superseded.** A row is set to `superseded` when all of these hold:
  - this run supplied reports (`MutationEngines` is non-empty);
  - the row's `<engine>` is among them;
  - the row has the same scope and target as this run;
  - the row is `open` or `override-pending`;
  - this run did not produce the row's fingerprint.

  `FixedInRunID` stays empty.
- **Never fixed.** The fix branch (`--previous-run` and escalation reset) skips these prefixes.
- **Recurrence.** A superseded row is no longer active, so a recurring fingerprint opens a new `open` row.
- **Overridden rows** are never touched.
- **Overrides with an escalation.** `RequestOverride` and `GrantOverride` accept a superseded freshness row when the request carries an escalation, exactly as they accept a `fixed` one.
- **Rendering.** `overrideLines` renders a superseded row's override request, tagged `[superseded]`.

- [ ] **Step 1: Write the failing tests**

`internal/findings/freshness_test.go`:

```go
package findings

import (
	"strings"
	"testing"
)

const staleFP = "mutation:stale:enforce:stryker:src/a.ts:0123abcd"

func staleInput() Input {
	return Input{Reviewer: "mutation-freshness", Severity: "high", Classification: "blocking", Title: "Mutation evidence stale: src/a.ts changed",
		Evidence: []Evidence{{Type: "mutant", Path: "src/a.ts"}}, Fingerprint: staleFP}
}

func run(id string) Run {
	return Run{ID: id, Scope: "pr-ready", Target: map[string]string{"branch": "b"}, GitHead: "h-" + id}
}

func statusOf(t *testing.T, root, fingerprint string) []string {
	t.Helper()
	records, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range records {
		if r.Fingerprint == fingerprint {
			out = append(out, r.Status+"/"+r.FixedInRunID)
		}
	}
	return out
}

func TestIsFreshnessFingerprint(t *testing.T) {
	for fp, want := range map[string]bool{staleFP: true, "mutation:pending:advisory:stryker:0123abcd": true,
		"mutation:unattested:advisory:stryker:0123abcd": true, "mutation:survived:M:a.ts:1": false, "quality:todo": false} {
		if IsFreshnessFingerprint(fp) != want {
			t.Errorf("%s: want %v", fp, want)
		}
	}
	if freshnessEngine("mutation:stale:x") != "" || freshnessEngine(staleFP) != "stryker" {
		t.Error("engine field")
	}
}

func TestFreshEvidenceSupersedesAStaleRowNeverFixesIt(t *testing.T) {
	root := t.TempDir()
	engines := Options{MutationEngines: []string{"stryker"}}
	if _, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines); err != nil {
		t.Fatal(err)
	}
	// A run without reports leaves the row (and any override) alone.
	if _, err := Reconcile(root, run("r2"), nil, Options{PreviousRunID: "r1"}); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); len(got) != 1 || got[0] != "open/" {
		t.Fatalf("a run without reports: %v", got)
	}
	// A gremlins-only run leaves stryker rows alone.
	if _, err := Reconcile(root, run("r3"), nil, Options{MutationEngines: []string{"gremlins"}}); err != nil {
		t.Fatal(err)
	}
	// Another target is untouched.
	other := run("r4")
	other.Target = map[string]string{"branch": "other"}
	if _, err := Reconcile(root, other, nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != "open/" {
		t.Fatalf("gremlins-only or another target: %v", got)
	}
	// Fresh stryker evidence, with a previous-run chain: superseded, fixedInRunId empty.
	res, err := Reconcile(root, run("r5"), nil, Options{PreviousRunID: "r1", MutationEngines: []string{"stryker"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" || res.OpenBlockingCount != 0 {
		t.Fatalf("fresh evidence: %v (open blocking %d)", got, res.OpenBlockingCount)
	}
	// The fingerprint recurring opens a new row.
	if _, err := Reconcile(root, run("r6"), []Input{staleInput()}, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); len(got) != 2 || got[1] != "open/" {
		t.Fatalf("recurrence: %v", got)
	}
}

func TestOverriddenFreshnessRowsStayAndSupersededRowsTakeEscalatedOverrides(t *testing.T) {
	root := t.TempDir()
	engines := Options{MutationEngines: []string{"stryker"}}
	res, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines)
	if err != nil {
		t.Fatal(err)
	}
	id := res.NewFindings[0].ID
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "stale-only escalation"}); err != nil {
		t.Fatal(err)
	}
	// An override-pending row is superseded by fresh evidence, and keeps its request on the index.
	if _, err := Reconcile(root, run("r2"), nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" {
		t.Fatalf("override-pending: %v", got)
	}
	records, _ := All(root)
	if lines := overrideLines(records); len(lines) != 1 || !strings.Contains(lines[0], "[superseded]") {
		t.Errorf("override lines: %v", lines)
	}
	// A superseded freshness row accepts an override request only with an escalation.
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "again"}); err == nil {
		t.Error("no escalation: refused")
	}
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "lift", Escalation: "ESCALATED run r9"}); err != nil {
		t.Fatalf("with an escalation: %v", err)
	}
	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted"}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// An overridden row is never superseded.
	if _, err := Reconcile(root, run("r3"), nil, engines); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, staleFP); got[0] != StatusOverridden+"/" {
		t.Errorf("overridden: %v", got)
	}
}

func TestOnlyStaleBlockers(t *testing.T) {
	stale := Record{Classification: "blocking", Severity: "high", Fingerprint: staleFP}
	other := Record{Classification: "blocking", Severity: "high", Fingerprint: "quality:todo"}
	advisory := Record{Classification: "advisory", Severity: "medium", Fingerprint: "x"}
	if !OnlyStaleBlockers([]Record{stale, advisory}) || OnlyStaleBlockers([]Record{stale, other}) || OnlyStaleBlockers([]Record{advisory}) {
		t.Error("true only when every blocker is stale mutation evidence and there is one")
	}
}
```

Before writing, check `OverrideRequest`/`OverrideGrant`'s actual field names in `override.go` (`By`, `Reason`, `Escalation`), and whether `RequestOverride` refuses the actor that requested when it also grants. Adjust the test's field names to match, and record a ruling if they differ.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/findings/ -run 'Freshness|OnlyStale|Superseded'`
Expected: FAIL. `MutationEngines`, `IsFreshnessFingerprint`, `freshnessEngine` and `OnlyStaleBlockers` are undefined.

- [ ] **Step 3: Implement**

`internal/findings/findings.go`:

1. Add to `Options`:

```go
	// MutationEngines are the engines of this run's --mutation-report files, and empty when the run
	// supplied none, so mutation-freshness rows and their overrides are left alone (spec §6.5).
	MutationEngines []string
```

2. Add near `StatusSuperseded`:

```go
// freshnessPrefixes are the mutation-freshness findings (spec §6.5). Fresh evidence supersedes
// them; they are never "fixed", because a refresh is not a correction learning should read.
var freshnessPrefixes = []string{"mutation:stale:", "mutation:pending:", "mutation:unattested:"}

// IsFreshnessFingerprint reports a mutation-freshness finding.
func IsFreshnessFingerprint(fingerprint string) bool {
	for _, prefix := range freshnessPrefixes {
		if strings.HasPrefix(fingerprint, prefix) {
			return true
		}
	}
	return false
}

// freshnessEngine is the <engine> field of mutation:<kind>:<mode>:<engine>:….
func freshnessEngine(fingerprint string) string {
	parts := strings.SplitN(fingerprint, ":", 5)
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

// supersedesFreshness: this run supplied reports of the row's engine and did not reproduce it.
func supersedesFreshness(record Record, run Run, options Options, current map[string]bool) bool {
	return len(options.MutationEngines) > 0 && IsFreshnessFingerprint(record.Fingerprint) &&
		slices.Contains(options.MutationEngines, freshnessEngine(record.Fingerprint)) &&
		(record.Status == "open" || record.Status == StatusOverridePending) &&
		sameRunTarget(record, run) && !current[record.Fingerprint]
}

// OnlyStaleBlockers reports that every open blocking finding is stale mutation evidence (spec §6.8).
func OnlyStaleBlockers(records []Record) bool {
	blocking := 0
	for _, record := range records {
		if !IsBlockingClass(record) {
			continue
		}
		if !strings.HasPrefix(record.Fingerprint, "mutation:stale:") {
			return false
		}
		blocking++
	}
	return blocking > 0
}
```

(Import `slices`.)

3. In `Reconcile`'s loop over `existing`, add `!IsFreshnessFingerprint(record.Fingerprint) &&` to the fix branch's condition, just before `!currentFingerprints[record.Fingerprint]`. Then add right after that branch:

```go
		if supersedesFreshness(record, run, options, currentFingerprints) {
			record.Status = StatusSuperseded
			record.UpdatedAt = now
			record.GitHead = run.GitHead
		}
```

4. In the `activeExisting` loop, the condition `record.Status != "fixed"` becomes `record.Status != "fixed" && record.Status != StatusSuperseded`, so a recurring fingerprint opens a new row.

`internal/findings/override.go`:
- In `RequestOverride`, replace `record.Status == "fixed"` in `fixedWithEscalation` with `(record.Status == "fixed" || supersededFreshness(*record))`. Make the same change in `GrantOverride`.
- Add:

```go
// supersededFreshness: a freshness row that fresh evidence replaced. Like a fixed row, it takes an
// override only with an escalation, so a stale-only escalation (spec §6.8) can still be lifted.
func supersededFreshness(record Record) bool {
	return record.Status == StatusSuperseded && IsFreshnessFingerprint(record.Fingerprint)
}
```

- In `overrideLines`, add a case:

```go
		case StatusSuperseded:
			if record.OverrideRequestedBy != "" {
				lines = append(lines, withEscalation(fmt.Sprintf("- %s [superseded] %s — requested by %s at %s: %s",
					record.ID, title, reqBy, reqAt, reqReason), record))
			}
```

(`reqBy` etc. are the variables the function already declares; use the exact names in the file.)

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/findings/`
Expected: `ok`, 100.0%. The existing tests stay green, and no existing fingerprint carries these prefixes.

- [ ] **Step 5: Commit**

```bash
git add internal/findings
git commit -m "feat(findings): fresh mutation evidence supersedes stale, pending and unattested rows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Wire freshness into the three gates and the CLI

**Files:**
- Modify:
  - `internal/reviewers/mutation.go`, `internal/reviewers/mutation_test.go` (append);
  - `internal/taskdone/review.go`, `internal/prready/review.go`, `internal/epicready/review.go`, with a test in each package;
  - `cmd/metareview/main.go`, with a test.

**Interfaces:**
- Produces:
  - `reviewers.MutationContext.Mode string` (`json:"mode,omitempty"`) and `Freshness []mutationfresh.ReportFreshness` (`json:"freshness,omitempty"`);
  - `FreshnessFindings []findings.Input` and `FreshnessSection string`, both `json:"-"`;
  - `(MutationContext) Engines() []string`;
  - `reviewers.LoadMutationContext(root string, paths []string, scope string, head bool) (MutationContext, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/reviewers/mutation_test.go` (add `os`, `os/exec`, `path/filepath`, `crypto/sha256`, `encoding/hex` and `fmt` to its imports):

```go
// A repository with src/a.ts committed and an attested report whose one kill is in src/a.ts.
func freshnessRepo(t *testing.T) (root, report string) {
	t.Helper()
	root = t.TempDir()
	gitIn := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/a.ts"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn("init", "-q")
	gitIn("config", "user.email", "t@example.com")
	gitIn("config", "user.name", "T")
	gitIn("add", "-A")
	gitIn("commit", "-qm", "init")
	state := filepath.Join(root, ".state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	reportText := `{"files":{"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{"t.test.ts":{"tests":[{"id":"t1"}]}}}`
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
		"lists":{"mutate":["src/**"],"test":["*.test.ts"],"support":[],"global":[],"ignore":[".state/**"]},"exclusions":[".state/**"],
		"files":{"src/a.ts":{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum(reportText), sum("a"))
	report = filepath.Join(state, "incremental.json")
	if err := os.WriteFile(report, []byte(reportText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "attestation.json"), []byte(att), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, report
}

func TestLoadMutationContext(t *testing.T) {
	if ctx, err := LoadMutationContext(t.TempDir(), nil, "pr-ready", true); err != nil || ctx.Mode != "" || len(ctx.Freshness) != 0 {
		t.Errorf("no reports: nothing loaded, nothing serialized: %+v %v", ctx, err)
	}
	root, report := freshnessRepo(t)
	ctx, err := LoadMutationContext(root, []string{report}, "task-done", false)
	if err != nil || ctx.Mode != "advisory" || ctx.Freshness[0].Verified != 1 || !strings.Contains(ctx.FreshnessSection, "1 verified") {
		t.Fatalf("worktree: %+v %v", ctx, err)
	}
	if got := ctx.Engines(); len(got) != 1 || got[0] != "stryker" {
		t.Errorf("engines %v", got)
	}
	// Uncommitted: stale in the working tree, not in HEAD mode.
	if err := os.WriteFile(filepath.Join(root, "src/a.ts"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
	wt, _ := LoadMutationContext(root, []string{report}, "pr-ready", false)
	head, _ := LoadMutationContext(root, []string{report}, "pr-ready", true)
	if wt.Freshness[0].Stale != 1 || head.Freshness[0].Stale != 0 || wt.Mode != "enforce" {
		t.Errorf("working tree %+v, HEAD %+v", wt.Freshness[0], head.Freshness[0])
	}
	found := false
	for _, f := range wt.Findings() {
		found = found || (strings.HasPrefix(f.Fingerprint, "mutation:stale:enforce:stryker:src/a.ts:") && f.Classification == "blocking")
	}
	if !found {
		t.Errorf("the enforced stale finding joins the context's findings: %+v", wt.Findings())
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "bogus")
	if _, err := LoadMutationContext(root, []string{report}, "pr-ready", false); err == nil {
		t.Error("an invalid mode is an error")
	}
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "")
	if _, err := LoadMutationContext(root, []string{filepath.Join(root, "missing.json")}, "pr-ready", false); err == nil {
		t.Error("an unreadable report is an error")
	}
	if _, err := LoadMutationContext(t.TempDir(), []string{report}, "pr-ready", false); err == nil {
		t.Error("a content error (not a repository) is an error")
	}
}
```

Then add one integration test per gate. Model each on that package's existing test that exercises `--mutation-report`: search the package's `_test.go` files for `MutationReportPaths` and copy its repository setup. Each test builds the report with a `freshnessRepo`-style fixture inside that package's test repository. Assert:
- **task-done:** the review log contains `## Mutation Evidence Freshness` and `- Mode: \`advisory\``. With `src/a.ts` edited, it has an advisory `Mutation evidence stale: src/a.ts changed` finding, even with `METAREVIEW_MUTATION_FRESHNESS=enforce`.
- **pr-ready:** with `METAREVIEW_MUTATION_FRESHNESS=enforce` and `src/a.ts` edited *and committed*, the verdict is `NEEDS_REVISION` with the stale finding blocking. An uncommitted edit without `--include-working-tree` gives no stale finding. Two runs without reports produce the same reviewer-input digest as before this change: compare against a run on a context whose `MutationContext` is zero.
- **epic-ready:** the review log contains the section.
- **Supersede:** the pr-ready test's second run, with the edit re-attested (a fresh attestation digest for `src/a.ts`), leaves the first run's stale row `superseded`. This proves `MutationEngines` reaches `Reconcile`.

In `cmd/metareview`, add a test (following the existing CLI tests' `exit` stub) that `review pr-ready` with `METAREVIEW_MUTATION_FRESHNESS=bogus` exits 2 and names the variable on stderr.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/reviewers/ ./internal/taskdone/ ./internal/prready/ ./internal/epicready/ ./cmd/metareview/`
Expected: FAIL. `LoadMutationContext` and the other new names are undefined.

- [ ] **Step 3: Implement**

`internal/reviewers/mutation.go`:
- Add the fields and methods below to `MutationContext`.
- Append `m.FreshnessFindings` in `Findings()` after the per-report loop, using the same `seen` de-duplication.

```go
type MutationContext struct {
	Reports []mutation.Report
	// Freshness (spec §6.7). Serialized into pr-ready's reviewer-input digest only when reports were
	// supplied (omitempty), so a run without reports keeps its digest.
	Mode      string                          `json:"mode,omitempty"`
	Freshness []mutationfresh.ReportFreshness `json:"freshness,omitempty"`
	// FreshnessFindings and FreshnessSection are derived from the fields above.
	FreshnessFindings []findings.Input `json:"-"`
	FreshnessSection  string           `json:"-"`
}

// Engines lists the engines of this run's reports, in first-seen order (findings.Options.MutationEngines).
func (m MutationContext) Engines() []string {
	var out []string
	for _, r := range m.Reports {
		if !slices.Contains(out, r.Engine) {
			out = append(out, r.Engine)
		}
	}
	return out
}

// LoadMutationContext loads the declared reports and judges their freshness against the content
// under review: HEAD for pr-ready (unless --include-working-tree), the working tree otherwise. An
// unreadable report, an invalid mode or a content read error stops the review: a mutation gate
// that quietly drops a report passes because it looked at less.
func LoadMutationContext(root string, paths []string, scope string, head bool) (MutationContext, error) {
	if len(paths) == 0 {
		return MutationContext{}, nil
	}
	reports, err := mutation.LoadAll(paths)
	if err != nil {
		return MutationContext{}, err
	}
	mode, err := mutationfresh.ModeFromEnv(os.Getenv)
	if err != nil {
		return MutationContext{}, err
	}
	content := mutationfresh.Worktree(root)
	if head {
		content = mutationfresh.Head(root)
	}
	res, err := mutationfresh.Build(reports, content, mutationfresh.EffectiveMode(scope, mode))
	if err != nil {
		return MutationContext{}, err
	}
	return MutationContext{Reports: reports, Mode: res.Mode, Freshness: res.Freshness, FreshnessFindings: res.Findings, FreshnessSection: res.Section}, nil
}
```

(Imports: `os`, `slices`, `internal/findings`, `internal/mutationfresh`.)

Gates (all three, same pattern):
1. **Loader.** `mutationContextFor(paths)` becomes `mutationContextFor(root string, paths []string)`, and its body becomes `return reviewers.LoadMutationContext(root, paths, "<scope>", <head>)`. The head flag is `false` for task-done and epic-ready. pr-ready passes `head` as a third parameter: `!options.IncludeWorkingTree`. Update each call site. The `mutation` import may become unused; remove it if so.
2. **Engines.** In each gate's `findings.Options{…}` passed to `reconcileFindings`/`Reconcile`, add `MutationEngines: mutationContext.Engines()`.
3. **Log section.** The section goes after the `## Verdict` value line and any sharded block.
   - task-done and pr-ready: wherever `reviewmanifest.ShardedReviewMarkdown(manifest, aggregate)` is passed to `reviewMarkdown`, pass `joinSections(<that>, mutationContext.FreshnessSection)`. Add this helper to each package:

   ```go
   // joinSections joins the non-empty sections that follow the verdict line.
   func joinSections(sections ...string) string {
   	var kept []string
   	for _, s := range sections {
   		if s != "" {
   			kept = append(kept, s)
   		}
   	}
   	return strings.Join(kept, "\n\n")
   }
   ```

   - epic-ready: `reviewMarkdown` gains a `freshnessSection string` parameter, inserted after `"## Verdict\n\n" + verdict + "\n\n"` (followed by `"\n\n"` when non-empty).
   - pr-ready's reused-verdict path must render the section too: every `reviewMarkdown` call site in the file passes it.

`cmd/metareview/main.go`:
- Add the helper below. Call it at the start of the `review task-done`, `review pr-ready` and `review epic-ready` handlers, before flag parsing.
- Add a usage line after the pr-ready usage line: `  METAREVIEW_MUTATION_FRESHNESS=advisory|enforce   freshness of --mutation-report evidence (docs/mutation-harness.md)`.

```go
// mustFreshnessMode refuses an invalid METAREVIEW_MUTATION_FRESHNESS before any review runs (spec §6.4).
func mustFreshnessMode() {
	if _, err := mutationfresh.ModeFromEnv(os.Getenv); err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		exit(2)
	}
}
```

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/reviewers/ ./internal/taskdone/ ./internal/prready/ ./internal/epicready/ ./cmd/metareview/ ./internal/mutationfresh/ ./internal/findings/`
Expected: `ok` everywhere, each at 100.0%. Then run `go test ./...` for the whole module.

- [ ] **Step 5: Commit**

```bash
git add internal/reviewers internal/taskdone internal/prready internal/epicready cmd/metareview
git commit -m "feat(gates): judge mutation evidence freshness in task-done, pr-ready and epic-ready

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: The stale-only escalation exemption and learning (spec §6.8, §6.9)

**Files:**
- Modify: `internal/prready/review.go`, `internal/epicready/review.go` (`verdictForCounts`, with tests)
- Modify: `internal/learning/candidates.go` (with a test)

**Behaviour:**
- **Escalation.** A run at `attempt >= maxAttempts` whose open blocking findings are all `mutation:stale:` (`findings.OnlyStaleBlockers(reconciled.OpenFindings)`) returns `NEEDS_REVISION` while `attempt < 2 × maxAttempts`. At `attempt >= 2 × maxAttempts` it escalates with the reason `stale mutation evidence not refreshed after <attempt> attempts`. A chain with any other blocker escalates at `maxAttempts` as before.
- **Learning.** `knowledgeFromFindings` and `repeatedBlockerThemes` skip records whose fingerprint `findings.IsFreshnessFingerprint`. `confidenceForFinding` is only reached through them.

- [ ] **Step 1: Write the failing tests**

In each of `internal/prready` and `internal/epicready`, add a table test for `verdictForCounts`. Its signature gains a trailing `staleOnly bool`:

```go
func TestStaleOnlyChainsWaitForFreshEvidence(t *testing.T) {
	blocking := findings.ClassCounts{Blocking: 1}
	cases := []struct {
		attempt, max int
		staleOnly    bool
		verdict      string
		reason       string
	}{
		{3, 3, false, "ESCALATED", ""}, // any other blocker escalates at maxAttempts
		{3, 3, true, "NEEDS_REVISION", ""},
		{5, 3, true, "NEEDS_REVISION", ""},
		{6, 3, true, "ESCALATED", "stale mutation evidence not refreshed after 6 attempts"},
		{2, 3, true, "NEEDS_REVISION", ""},
	}
	for _, c := range cases {
		verdict, _, _, reason := verdictForCounts(blocking, "gate", c.attempt, c.max, c.staleOnly)
		if verdict != c.verdict || (c.reason != "" && reason != c.reason) {
			t.Errorf("%+v: got %s %q", c, verdict, reason)
		}
	}
}
```

In `internal/learning`, add a test. A `fixed` record with fingerprint `mutation:stale:enforce:stryker:src/a.ts:01234567` and `KnowledgeCandidate: true` yields no candidate from `knowledgeFromFindings`. Two blocking records with the same title and freshness fingerprints yield no theme from `repeatedBlockerThemes`.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/prready/ ./internal/epicready/ ./internal/learning/`
Expected: FAIL. `verdictForCounts` has the wrong arity, and learning still emits the candidates.

- [ ] **Step 3: Implement**

In both `verdictForCounts`: add the parameter `staleOnly bool`, and inside the `blocking && attemptNumber >= maxAttempts` branch, before building the escalation:

```go
		if staleOnly && attemptNumber < 2*maxAttempts {
			return "NEEDS_REVISION", "needs-revision", true, ""
		}
```

and when escalating with `staleOnly`, use `fmt.Sprintf("stale mutation evidence not refreshed after %d attempts", attemptNumber)` as the reason. Match the existing return values: use the exact status strings the function already returns for `NEEDS_REVISION`. Update both call sites to pass `findings.OnlyStaleBlockers(reconciled.OpenFindings)`, using the name the reconcile result has at each call site.

In `internal/learning/candidates.go`, add at the top of the loop in `knowledgeFromFindings` and in `repeatedBlockerThemes`:

```go
		if findings.IsFreshnessFingerprint(record.Fingerprint) {
			continue // freshness is evidence bookkeeping, not a lesson (spec §6.9)
		}
```

- [ ] **Step 4: Verify**

Run: `go test -cover ./internal/prready/ ./internal/epicready/ ./internal/learning/`
Expected: `ok`, each at 100.0%.

- [ ] **Step 5: Commit**

```bash
git add internal/prready internal/epicready internal/learning
git commit -m "feat(gates): stale-only chains wait for fresh evidence; learning skips freshness

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Refresh the real reports, then docs and changelog

**Files:**
- Regenerate: `testdata/mutation-incremental/real/**` (the e2e run; the attestations gain `config`)
- Modify: `docs/mutation-harness.md` (§11), `CHANGELOG.md`

- [ ] **Step 1: Re-run the real-Stryker e2e**

Run: `node tests/e2e-mutation-incremental.mjs > .e2e/e2e-last.log 2>&1; tail -1 .e2e/e2e-last.log`
Expected: `56/56 checks passed`. Every `real/<row>/attestation.json` now has `"config": "mutation-incremental.json"`.

- [ ] **Step 2: Re-run the Go suite on the refreshed reports**

Run: `go test -cover ./internal/mutationfresh/`
Expected: `ok`, 100.0%. `realCase` rebases `config` to `""`, so the classification is unchanged.

- [ ] **Step 3: Docs**

- `docs/mutation-harness.md` §11 "The gate": state the implemented behaviour.
  - Reports are judged per kill: verified, stale, pending, unbound or unattested.
  - `METAREVIEW_MUTATION_FRESHNESS=advisory|enforce`: task-done is always advisory.
  - Stale and unattested findings block under enforce; pending never blocks.
  - pr-ready reads HEAD, so commit before re-running.
  - The "Mutation Evidence Freshness" section and its re-run list.
  - Fresh evidence supersedes stale rows.
  - A stale-only chain waits for up to 2 × `maxAttempts` before escalating.

  Keep the paragraph on overrides, and remove "ship in the same 0.13.0 release" wording that the code now makes true. Say that `--mutation-view` arrives with views (Plan 2b).
- `CHANGELOG.md`, Unreleased: add an entry "**Mutation evidence freshness gate.** …" summarising the above, and change the harness entry's "Gate integration follows in 0.13.0" to point at it.

- [ ] **Step 4: Verify and commit**

Run: `go test ./... && go vet ./...`
Expected: `ok` for every package; vet clean.

```bash
git add testdata/mutation-incremental/real docs/mutation-harness.md CHANGELOG.md
git commit -m "docs(mutation-incremental): gate freshness docs; real reports carry the config path

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Follow-ups (recorded, not in 2a)

- Plan 2b:
  - views (§11.3): `--mutation-view`, per-view classification and fingerprints, the view-aware `openForRun` and supersede, and the rename sweep;
  - the out-of-mutant-hunk blanket cause (§11.5), with a Go port of the line diff on `diff-vectors.json`;
  - the §7.1 gate rows against the real e2e repository.
- A path containing a newline cannot be read in HEAD mode through `cat-file --batch` (line-based input). The harness already refuses such paths for `--mutate`.

## Self-Review (done while writing)

- **Spec coverage in 2a:**

  | Spec section | Task |
  |---|---|
  | §6.1 attestation validation, Stryker ids/killedBy/coveredBy/testFiles | 3, 4 |
  | §6.2 working tree vs HEAD, `cat-file --filters`, gitlinks, `absent`, read errors stop | 3 |
  | §6.3 changed paths, categories, cause order, pending, unbound, verified, unattested | 4 |
  | §6.4 modes, exit 2, task-done advisory | 5, 7 |
  | §6.5 fingerprints, severities, texts, summing, gremlins, ledger lifecycle, overrides | 5, 6 |
  | §6.6 log section and re-run list | 5, 7 |
  | §6.7 pr-ready digest (`omitempty`) | 7 |
  | §6.8 escalation exemption | 6, 8 |
  | §6.9 learning skip | 8 |
  | Plan 1a's config digest without views | 1, 2 |

- **Placeholders:** the pure package is complete. The gate wiring (Task 7) names exact functions and fields, and follows each package's existing `--mutation-report` test for the repository setup instead of guessing it. The synthetic classification test marks where hand derivation governs.
- **Type consistency:**
  - `mutationfresh.Build` → `Result{Mode, Freshness, Findings, Section}` → `reviewers.MutationContext{Mode, Freshness, FreshnessFindings, FreshnessSection}`;
  - `MutationContext.Engines()` → `findings.Options.MutationEngines`;
  - `findings.OnlyStaleBlockers` → `verdictForCounts(…, staleOnly)`.
- **Review Focus:** each item has a named test.


## Git

- Base: `3a40ca338b1895717ec1849e227dec3f29b7bf85`
- Head: `288482fed9460bbc0def7d59b081bf1fd46a8823`
- Branch: `mutation-incremental`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `1461529`
- Filtered diff bytes: `579202`
- Risk level: `context-risk`
- Risk reasons: `DIFF_TRUNCATED`, `LARGE_DIFF`
- Generated files excluded: docs/metareview/FINDINGS.md, docs/metareview/context/mrv-20260924-044110402907000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md, docs/metareview/context/mrv-20260924-044605632345000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md, docs/metareview/context/mrv-20260924-044639306140000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md, docs/metareview/context/mrv-20260924-044954439569000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md, docs/metareview/context/mrv-20260924-045030711559000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md, docs/metareview/reviews/mrv-20260924-044110402907000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md, docs/metareview/reviews/mrv-20260924-044605632345000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md, docs/metareview/reviews/mrv-20260924-044639306140000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md, docs/metareview/reviews/mrv-20260924-044954439569000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md, docs/metareview/reviews/mrv-20260924-045030711559000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.28a9de61df9c3434.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.6e4356555ecf2acd.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.edeacff8273fa5e4.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-0.d6fa682d05cc1b1f.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-1.478d32da4dd5c18a.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-2-2.a86a9abcaa38a33a.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-2.6350b7e7de7f5555.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-3-2.9e90245f150b754e.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-3.f00933331ab9f6f9.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-4.7229de6715a1160b.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-5-2.68a494e66e87ed49.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-5.3ebab16f0620de72.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-6.b079fc1531033ebd.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-7.4f4f1b5dca6b88c8.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-7.a6778c633dec1978.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-8.76893d92a38fba7f.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-9-2.c77c5c10d1ffeb54.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-9.87d842e09c0111d2.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-a.eb79a4bec784505c.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-b.fac8e12ccab401b5.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-c.722fb75f82589e8b.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-d-2.df236f3748f717bf.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-d.593a5c1c4729f659.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e-2.dbcff8f7debd56d8.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.0795d10edc9694ba.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.9624ab2fb9232c8e.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.fb6f2b2aee1472e1.result.json, docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-f.5888b6037e674230.result.json

## Context Shard Plan

- Source diff hash: `3cbaf50b2a30315c`
- Plan hash: `6e4356555ecf2acd`
- shard-0 (`d6fa682d05cc1b1f`, 15427 bytes): cmd/metareview/freshness_test.go, cmd/metareview/main.go, internal/findings/findings.go, internal/mutationfresh/attest.go, internal/prready/mutationctx_test.go, internal/prready/verdict_evidence_test.go, testdata/mutation-incremental/real/a-survivor/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-0.md`
- shard-1 (`478d32da4dd5c18a`, 12129 bytes): internal/prready/freshness_test.go, testdata/mutation-incremental/real/delete-c-test/attestation.json, testdata/mutation-incremental/real/lockfile/attestation.json, testdata/mutation-incremental/real/test-b/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-1.md`
- shard-2 (`6350b7e7de7f5555`, 28460 bytes): internal/epicready/review.go, internal/mutationfresh/build.go, internal/mutationfresh/classify_test.go, internal/mutationfresh/glob_test.go — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-2.md`
- shard-2-2 (`a86a9abcaa38a33a`, 35141 bytes): testdata/mutation-incremental/real/e-flip/incremental.json, testdata/mutation-incremental/real/e-residual/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-2-2.md`
- shard-3 (`f00933331ab9f6f9`, 33810 bytes): internal/epicready/freshness_test.go, internal/mutation/report.go, internal/prready/review_markdown_test.go, templates/mutation-incremental/test/configdigest.test.mjs, testdata/mutation-incremental/real/time-budget/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-3.md`
- shard-3-2 (`9e90245f150b754e`, 28598 bytes): testdata/mutation-incremental/real/types-only/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-3-2.md`
- shard-4 (`7229de6715a1160b`, 6620 bytes): internal/mutationfresh/content_test.go, internal/taskdone/mutationctx_test.go — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-4.md`
- shard-5 (`3ebab16f0620de72`, 44542 bytes): internal/mutationfresh/classify.go, internal/reviewers/mutation_test.go, testdata/mutation-incremental/real/a-preserving/incremental.json, testdata/mutation-incremental/real/e-flip/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-5.md`
- shard-5-2 (`68a494e66e87ed49`, 33276 bytes): testdata/mutation-incremental/real/limits/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-5-2.md`
- shard-6 (`b079fc1531033ebd`, 37737 bytes): docs/mutation-harness.md, templates/mutation-incremental/lib/attest.mjs, templates/mutation-incremental/lib/snapshot.mjs, testdata/mutation-incremental/real/e-residual/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-6.md`
- shard-7 (`4f4f1b5dca6b88c8`, 17194 bytes): internal/epicready/review_markdown_test.go, internal/epicready/stale_escalation_test.go, internal/learning/freshness_test.go, internal/mutationfresh/digest.go, internal/prready/review.go, testdata/mutation-incremental/config-digest-vectors.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-7.md`
- shard-8 (`76893d92a38fba7f`, 12661 bytes): internal/mutation/parse.go, internal/mutationfresh/build_test.go, internal/mutationfresh/helpers_test.go, testdata/mutation-incremental/real/types-only/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-8.md`
- shard-9 (`87d842e09c0111d2`, 35431 bytes): internal/epicready/mutationctx_test.go, internal/reviewers/mutation.go, testdata/mutation-incremental/real/a-survivor/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-9.md`
- shard-9-2 (`c77c5c10d1ffeb54`, 35453 bytes): testdata/mutation-incremental/real/e-flip-control/incremental.json, testdata/mutation-incremental/real/limits/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-9-2.md`
- shard-a (`eb79a4bec784505c`, 33363 bytes): internal/mutationfresh/digest_test.go, internal/taskdone/review.go, testdata/mutation-incremental/real/delete-c-test/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-a.md`
- shard-b (`fac8e12ccab401b5`, 8060 bytes): internal/learning/candidates.go, internal/mutation/stryker.go, internal/mutationfresh/headcache_test.go, internal/taskdone/freshness_test.go — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-b.md`
- shard-c (`722fb75f82589e8b`, 1672 bytes): internal/mutation/stryker_test.go — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-c.md`
- shard-d (`593a5c1c4729f659`, 36180 bytes): internal/epicready/coverage_test.go, internal/prready/stale_escalation_test.go, testdata/mutation-incremental/real/a-preserving/attestation.json, testdata/mutation-incremental/real/full/attestation.json, testdata/mutation-incremental/real/lockfile/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-d.md`
- shard-d-2 (`df236f3748f717bf`, 30324 bytes): testdata/mutation-incremental/real/test-b/incremental.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-d-2.md`
- shard-e (`fb6f2b2aee1472e1`, 54801 bytes): internal/findings/freshness_test.go, internal/findings/override.go, internal/mutationfresh/attest_test.go, internal/mutationfresh/content.go, internal/mutationfresh/glob.go, internal/mutationfresh/mode.go, testdata/mutation-incremental/real/full/incremental.json, testdata/mutation-incremental/real/helper-make/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-e.md`
- shard-e-2 (`dbcff8f7debd56d8`, 32611 bytes): testdata/mutation-incremental/real/helper-make/incremental.json, testdata/mutation-incremental/real/time-budget/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-e-2.md`
- shard-f (`5888b6037e674230`, 5712 bytes): CHANGELOG.md, templates/mutation-incremental/test/attest.test.mjs, testdata/mutation-incremental/real/e-flip-control/attestation.json — pack `.metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/6e4356555ecf2acd/shard-f.md`

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: `6e4356555ecf2acd`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- CHANGELOG.md
- cmd/metareview/freshness_test.go
- cmd/metareview/main.go
- docs/mutation-harness.md
- internal/epicready/coverage_test.go
- internal/epicready/freshness_test.go
- internal/epicready/mutationctx_test.go
- internal/epicready/review.go
- internal/epicready/review_markdown_test.go
- internal/epicready/stale_escalation_test.go
- internal/findings/findings.go
- internal/findings/freshness_test.go
- internal/findings/override.go
- internal/learning/candidates.go
- internal/learning/freshness_test.go
- internal/mutation/parse.go
- internal/mutation/report.go
- internal/mutation/stryker.go
- internal/mutation/stryker_test.go
- internal/mutationfresh/attest.go
- internal/mutationfresh/attest_test.go
- internal/mutationfresh/build.go
- internal/mutationfresh/build_test.go
- internal/mutationfresh/classify.go
- internal/mutationfresh/classify_test.go
- internal/mutationfresh/content.go
- internal/mutationfresh/content_test.go
- internal/mutationfresh/digest.go
- internal/mutationfresh/digest_test.go
- internal/mutationfresh/glob.go
- internal/mutationfresh/glob_test.go
- internal/mutationfresh/headcache_test.go
- internal/mutationfresh/helpers_test.go
- internal/mutationfresh/mode.go
- internal/prready/freshness_test.go
- internal/prready/mutationctx_test.go
- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/prready/stale_escalation_test.go
- internal/prready/verdict_evidence_test.go
- internal/reviewers/mutation.go
- internal/reviewers/mutation_test.go
- internal/taskdone/freshness_test.go
- internal/taskdone/mutationctx_test.go
- internal/taskdone/review.go
- templates/mutation-incremental/lib/attest.mjs
- templates/mutation-incremental/lib/snapshot.mjs
- templates/mutation-incremental/test/attest.test.mjs
- templates/mutation-incremental/test/configdigest.test.mjs
- testdata/mutation-incremental/config-digest-vectors.json
- testdata/mutation-incremental/real/a-preserving/attestation.json
- testdata/mutation-incremental/real/a-preserving/incremental.json
- testdata/mutation-incremental/real/a-survivor/attestation.json
- testdata/mutation-incremental/real/a-survivor/incremental.json
- testdata/mutation-incremental/real/delete-c-test/attestation.json
- testdata/mutation-incremental/real/delete-c-test/incremental.json
- testdata/mutation-incremental/real/e-flip-control/attestation.json
- testdata/mutation-incremental/real/e-flip-control/incremental.json
- testdata/mutation-incremental/real/e-flip/attestation.json
- testdata/mutation-incremental/real/e-flip/incremental.json
- testdata/mutation-incremental/real/e-residual/attestation.json
- testdata/mutation-incremental/real/e-residual/incremental.json
- testdata/mutation-incremental/real/full/attestation.json
- testdata/mutation-incremental/real/full/incremental.json
- testdata/mutation-incremental/real/helper-make/attestation.json
- testdata/mutation-incremental/real/helper-make/incremental.json
- testdata/mutation-incremental/real/limits/attestation.json
- testdata/mutation-incremental/real/limits/incremental.json
- testdata/mutation-incremental/real/lockfile/attestation.json
- testdata/mutation-incremental/real/lockfile/incremental.json
- testdata/mutation-incremental/real/test-b/attestation.json
- testdata/mutation-incremental/real/test-b/incremental.json
- testdata/mutation-incremental/real/time-budget/attestation.json
- testdata/mutation-incremental/real/time-budget/incremental.json
- testdata/mutation-incremental/real/types-only/attestation.json
- testdata/mutation-incremental/real/types-only/incremental.json

### Path Dispositions
- docs/metareview/FINDINGS.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260924-044110402907000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260924-044605632345000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260924-044639306140000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260924-044954439569000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260924-045030711559000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260924-044110402907000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260924-044605632345000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260924-044639306140000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260924-044954439569000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260924-045030711559000-task-done-2026-09-23-mutation-incremental-2a-gate-ed03c179.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.28a9de61df9c3434.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.6e4356555ecf2acd.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.edeacff8273fa5e4.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-0.d6fa682d05cc1b1f.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-1.478d32da4dd5c18a.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-2-2.a86a9abcaa38a33a.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-2.6350b7e7de7f5555.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-3-2.9e90245f150b754e.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-3.f00933331ab9f6f9.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-4.7229de6715a1160b.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-5-2.68a494e66e87ed49.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-5.3ebab16f0620de72.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-6.b079fc1531033ebd.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-7.4f4f1b5dca6b88c8.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-7.a6778c633dec1978.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-8.76893d92a38fba7f.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-9-2.c77c5c10d1ffeb54.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-9.87d842e09c0111d2.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-a.eb79a4bec784505c.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-b.fac8e12ccab401b5.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-c.722fb75f82589e8b.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-d-2.df236f3748f717bf.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-d.593a5c1c4729f659.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e-2.dbcff8f7debd56d8.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.0795d10edc9694ba.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.9624ab2fb9232c8e.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.fb6f2b2aee1472e1.result.json: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-f.5888b6037e674230.result.json: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-0: cmd/metareview/freshness_test.go, cmd/metareview/main.go, internal/findings/findings.go, internal/mutationfresh/attest.go, internal/prready/mutationctx_test.go, internal/prready/verdict_evidence_test.go, testdata/mutation-incremental/real/a-survivor/attestation.json
- shard-1: internal/prready/freshness_test.go, testdata/mutation-incremental/real/delete-c-test/attestation.json, testdata/mutation-incremental/real/lockfile/attestation.json, testdata/mutation-incremental/real/test-b/attestation.json
- shard-2: internal/epicready/review.go, internal/mutationfresh/build.go, internal/mutationfresh/classify_test.go, internal/mutationfresh/glob_test.go
- shard-2-2: testdata/mutation-incremental/real/e-flip/incremental.json, testdata/mutation-incremental/real/e-residual/attestation.json
- shard-3: internal/epicready/freshness_test.go, internal/mutation/report.go, internal/prready/review_markdown_test.go, templates/mutation-incremental/test/configdigest.test.mjs, testdata/mutation-incremental/real/time-budget/incremental.json
- shard-3-2: testdata/mutation-incremental/real/types-only/incremental.json
- shard-4: internal/mutationfresh/content_test.go, internal/taskdone/mutationctx_test.go
- shard-5: internal/mutationfresh/classify.go, internal/reviewers/mutation_test.go, testdata/mutation-incremental/real/a-preserving/incremental.json, testdata/mutation-incremental/real/e-flip/attestation.json
- shard-5-2: testdata/mutation-incremental/real/limits/incremental.json
- shard-6: docs/mutation-harness.md, templates/mutation-incremental/lib/attest.mjs, templates/mutation-incremental/lib/snapshot.mjs, testdata/mutation-incremental/real/e-residual/incremental.json
- shard-7: internal/epicready/review_markdown_test.go, internal/epicready/stale_escalation_test.go, internal/learning/freshness_test.go, internal/mutationfresh/digest.go, internal/prready/review.go, testdata/mutation-incremental/config-digest-vectors.json
- shard-8: internal/mutation/parse.go, internal/mutationfresh/build_test.go, internal/mutationfresh/helpers_test.go, testdata/mutation-incremental/real/types-only/attestation.json
- shard-9: internal/epicready/mutationctx_test.go, internal/reviewers/mutation.go, testdata/mutation-incremental/real/a-survivor/incremental.json
- shard-9-2: testdata/mutation-incremental/real/e-flip-control/incremental.json, testdata/mutation-incremental/real/limits/attestation.json
- shard-a: internal/mutationfresh/digest_test.go, internal/taskdone/review.go, testdata/mutation-incremental/real/delete-c-test/incremental.json
- shard-b: internal/learning/candidates.go, internal/mutation/stryker.go, internal/mutationfresh/headcache_test.go, internal/taskdone/freshness_test.go
- shard-c: internal/mutation/stryker_test.go
- shard-d: internal/epicready/coverage_test.go, internal/prready/stale_escalation_test.go, testdata/mutation-incremental/real/a-preserving/attestation.json, testdata/mutation-incremental/real/full/attestation.json, testdata/mutation-incremental/real/lockfile/incremental.json
- shard-d-2: testdata/mutation-incremental/real/test-b/incremental.json
- shard-e: internal/findings/freshness_test.go, internal/findings/override.go, internal/mutationfresh/attest_test.go, internal/mutationfresh/content.go, internal/mutationfresh/glob.go, internal/mutationfresh/mode.go, testdata/mutation-incremental/real/full/incremental.json, testdata/mutation-incremental/real/helper-make/attestation.json
- shard-e-2: testdata/mutation-incremental/real/helper-make/incremental.json, testdata/mutation-incremental/real/time-budget/attestation.json
- shard-f: CHANGELOG.md, templates/mutation-incremental/test/attest.test.mjs, testdata/mutation-incremental/real/e-flip-control/attestation.json

### Shard Results
- `shard-0` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-1` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-2-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-3` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-3-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-4` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-5` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-5-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-6` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-7` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-8` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-9` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-9-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-a` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-b` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-c` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-d` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-d-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-e` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-e-2` `PASS` by claude-sonnet-subagent (0 blocking)
- `shard-f` `PASS` by claude-sonnet-subagent (0 blocking)
- `cross-shard` `PASS_ADVISORY` by claude-sonnet-subagent (0 blocking)

### Ignored Result Files
- `docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.28a9de61df9c3434.result.json`: not the current plan hash
- `docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/cross-shard.edeacff8273fa5e4.result.json`: not the current plan hash
- `docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-7.a6778c633dec1978.result.json`: no current shard has this shard hash
- `docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.0795d10edc9694ba.result.json`: no current shard has this shard hash
- `docs/metareview/shards/task-done/docs-superpowers-plans-2026-09-23-mutation-incre-e5dcbbba/shard-e.9624ab2fb9232c8e.result.json`: no current shard has this shard hash

### Manifest Blockers
No manifest blockers.

## Changed Files

- CHANGELOG.md
- cmd/metareview/freshness_test.go
- cmd/metareview/main.go
- docs/mutation-harness.md
- internal/epicready/coverage_test.go
- internal/epicready/freshness_test.go
- internal/epicready/mutationctx_test.go
- internal/epicready/review.go
- internal/epicready/review_markdown_test.go
- internal/epicready/stale_escalation_test.go
- internal/findings/findings.go
- internal/findings/freshness_test.go
- internal/findings/override.go
- internal/learning/candidates.go
- internal/learning/freshness_test.go
- internal/mutation/parse.go
- internal/mutation/report.go
- internal/mutation/stryker.go
- internal/mutation/stryker_test.go
- internal/mutationfresh/attest.go
- internal/mutationfresh/attest_test.go
- internal/mutationfresh/build.go
- internal/mutationfresh/build_test.go
- internal/mutationfresh/classify.go
- internal/mutationfresh/classify_test.go
- internal/mutationfresh/content.go
- internal/mutationfresh/content_test.go
- internal/mutationfresh/digest.go
- internal/mutationfresh/digest_test.go
- internal/mutationfresh/glob.go
- internal/mutationfresh/glob_test.go
- internal/mutationfresh/headcache_test.go
- internal/mutationfresh/helpers_test.go
- internal/mutationfresh/mode.go
- internal/prready/freshness_test.go
- internal/prready/mutationctx_test.go
- internal/prready/review.go
- internal/prready/review_markdown_test.go
- internal/prready/stale_escalation_test.go
- internal/prready/verdict_evidence_test.go
- internal/reviewers/mutation.go
- internal/reviewers/mutation_test.go
- internal/taskdone/freshness_test.go
- internal/taskdone/mutationctx_test.go
- internal/taskdone/review.go
- templates/mutation-incremental/lib/attest.mjs
- templates/mutation-incremental/lib/snapshot.mjs
- templates/mutation-incremental/test/attest.test.mjs
- templates/mutation-incremental/test/configdigest.test.mjs
- testdata/mutation-incremental/config-digest-vectors.json
- testdata/mutation-incremental/real/a-preserving/attestation.json
- testdata/mutation-incremental/real/a-preserving/incremental.json
- testdata/mutation-incremental/real/a-survivor/attestation.json
- testdata/mutation-incremental/real/a-survivor/incremental.json
- testdata/mutation-incremental/real/delete-c-test/attestation.json
- testdata/mutation-incremental/real/delete-c-test/incremental.json
- testdata/mutation-incremental/real/e-flip-control/attestation.json
- testdata/mutation-incremental/real/e-flip-control/incremental.json
- testdata/mutation-incremental/real/e-flip/attestation.json
- testdata/mutation-incremental/real/e-flip/incremental.json
- testdata/mutation-incremental/real/e-residual/attestation.json
- testdata/mutation-incremental/real/e-residual/incremental.json
- testdata/mutation-incremental/real/full/attestation.json
- testdata/mutation-incremental/real/full/incremental.json
- testdata/mutation-incremental/real/helper-make/attestation.json
- testdata/mutation-incremental/real/helper-make/incremental.json
- testdata/mutation-incremental/real/limits/attestation.json
- testdata/mutation-incremental/real/limits/incremental.json
- testdata/mutation-incremental/real/lockfile/attestation.json
- testdata/mutation-incremental/real/lockfile/incremental.json
- testdata/mutation-incremental/real/test-b/attestation.json
- testdata/mutation-incremental/real/test-b/incremental.json
- testdata/mutation-incremental/real/time-budget/attestation.json
- testdata/mutation-incremental/real/time-budget/incremental.json
- testdata/mutation-incremental/real/types-only/attestation.json
- testdata/mutation-incremental/real/types-only/incremental.json

## Diff

```diff
diff --git a/CHANGELOG.md b/CHANGELOG.md
index 7ee14c1..bfc38c2 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -12,9 +12,21 @@
   and `summary`. A GitHub workflow template keeps state on `mutation-state/{inc,full}` branches
   (cache as a fast path), with PR routing (`pendingOnPr`), a per-view project verifier, and read-time
   views. Proven locally against real StrykerJS 10 and Vitest 4, including an equivalence check that
-  every incremental kill is a fresh kill. Gate integration (evidence freshness) follows in 0.13.0.
-  The ≤ 2 min p50 bar for a one-line edit is pending the external trial. Guide:
-  `docs/mutation-harness.md`.
+  every incremental kill is a fresh kill. The gate reads its attestation (next entry). The ≤ 2 min
+  p50 bar for a one-line edit is pending the external trial. Guide: `docs/mutation-harness.md`.
+- **Mutation evidence freshness gate.** `review task-done`, `pr-ready` and `epic-ready` now judge
+  whether the kills in each `--mutation-report` still describe the code under review. They read the
+  harness's `attestation.json` beside the report and classify every kill from content digests:
+  verified, stale (with its recorded cause), pending (covered by a deferral), unbound or unattested.
+  - pr-ready reads HEAD, and the other gates read the working tree.
+  - `METAREVIEW_MUTATION_FRESHNESS=advisory|enforce` (default `advisory`; task-done is always
+    advisory). Under `enforce`, stale and unattested findings block; pending never does.
+  - Each review log gains a "Mutation Evidence Freshness" section with a re-run list.
+  - Fresh evidence supersedes earlier freshness findings instead of marking them fixed, and
+    post-merge learning ignores them.
+  - A chain blocked only by stale evidence waits for a refresh for up to 2 × `maxAttempts` before
+    escalating.
+  - Runs without reports review exactly as before, and their pr-ready input digest is unchanged.
 
 ## 0.12.0 - 2026-09-10
 
diff --git a/cmd/metareview/freshness_test.go b/cmd/metareview/freshness_test.go
new file mode 100644
index 0000000..38e0a00
--- /dev/null
+++ b/cmd/metareview/freshness_test.go
@@ -0,0 +1,22 @@
+package main
+
+import (
+	"strings"
+	"testing"
+)
+
+// Spec §6.4: an invalid METAREVIEW_MUTATION_FRESHNESS stops every gate before it runs.
+func TestAnInvalidFreshnessModeIsAUsageError(t *testing.T) {
+	root := gitRepo(t)
+	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "strict")
+	for _, args := range [][]string{
+		{"review", "task-done", "docs/tasks/t.md", "--base", "main"},
+		{"review", "epic-ready", "docs/tasks/t.md", "--base", "main"},
+		{"review", "pr-ready", "--base", "main"},
+	} {
+		code, _, stderr := runCLI(t, root, nil, args...)
+		if code != 2 || !strings.Contains(stderr, "METAREVIEW_MUTATION_FRESHNESS") {
+			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
+		}
+	}
+}
diff --git a/cmd/metareview/main.go b/cmd/metareview/main.go
index 9d88cd4..34fb3ac 100644
--- a/cmd/metareview/main.go
+++ b/cmd/metareview/main.go
@@ -26,6 +26,7 @@ import (
 	"github.com/dsifry/metareview/internal/gitcontext"
 	"github.com/dsifry/metareview/internal/learning"
 	"github.com/dsifry/metareview/internal/mutation"
+	"github.com/dsifry/metareview/internal/mutationfresh"
 	"github.com/dsifry/metareview/internal/prready"
 	"github.com/dsifry/metareview/internal/repo"
 	"github.com/dsifry/metareview/internal/reviewmanifest"
@@ -118,6 +119,7 @@ Usage:
   metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--shard-result <path>]... [--cross-shard-result <path>]
   metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]...
   metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--github-pr <number>] [--include-working-tree] [--shard-result <path>]... [--cross-shard-result <path>]
+  METAREVIEW_MUTATION_FRESHNESS=advisory|enforce  freshness of --mutation-report evidence (docs/mutation-harness.md)
   metareview review record-lenses [--scope pr-ready|task-done|epic-ready] [--base <ref>] [--verdict <v>] [--mode subagent-adjudicated|in-session-emulated] [--lenses a,b,c] [--from-run <fsm-run-id>]
   metareview learn --post-merge <pr-number> [--base <ref>] [--github-pr <number>] [--session-root <path>]
 
@@ -298,6 +300,7 @@ func dispatch(args []string) {
 	}
 
 	if len(args) >= 3 && args[0] == "review" && args[1] == "task-done" {
+		mustFreshnessMode()
 		options := taskdone.Options{}
 		for i := 3; i < len(args); i++ {
 			switch args[i] {
@@ -337,6 +340,7 @@ func dispatch(args []string) {
 	}
 
 	if len(args) >= 3 && args[0] == "review" && args[1] == "epic-ready" {
+		mustFreshnessMode()
 		options := epicready.Options{}
 		for i := 3; i < len(args); i++ {
 			switch args[i] {
@@ -541,6 +545,7 @@ func dispatch(args []string) {
 		exit(0)
 	}
 	if len(args) >= 2 && args[0] == "review" && args[1] == "pr-ready" {
+		mustFreshnessMode()
 		options := prready.Options{}
 		for i := 2; i < len(args); i++ {
 			switch args[i] {
@@ -1175,6 +1180,14 @@ func mustResultFile(path string) string {
 	return path
 }
 
+// mustFreshnessMode refuses an invalid METAREVIEW_MUTATION_FRESHNESS before any review runs (spec §6.4).
+func mustFreshnessMode() {
+	if _, err := mutationfresh.ModeFromEnv(os.Getenv); err != nil {
+		_, _ = fmt.Fprintf(stderr, "%v\n", err)
+		exit(2)
+	}
+}
+
 // mustMutationReport rejects a --mutation-report file the review could not act on, at the point
 // the operator can still fix it. Parsing here as well as in the review is deliberate: a typo in a
 // path or a truncated report otherwise surfaces as a review that found nothing, which reads
diff --git a/docs/mutation-harness.md b/docs/mutation-harness.md
index 17d3b2d..fd7bbad 100644
--- a/docs/mutation-harness.md
+++ b/docs/mutation-harness.md
@@ -99,6 +99,9 @@ Sizing:
   - `tests/**` is `test`; helpers and factories are `support`.
   - `global` is only for inputs everything depends on: lockfile, Stryker/Vitest/TS config,
     migrations, `.nvmrc`.
+  - Keep metareview's own files (`.metareview/**`, `docs/metareview/**`) in `ignore`, as the
+    example config does. Otherwise a review log committed after the harness ran counts as a new
+    unclassified path, and the gate reads every kill as stale.
 - **Aliases and open importers.** A relative or alias specifier that reaches no file in the
   repository, and is not an installed package or a Node builtin, makes its file an **open
   importer**. Every test reaching an open importer is treated as reaching everything. `plan` lists
@@ -303,11 +306,28 @@ and main pays one full run (`stateVersion` changes make older states cold).
 ## 11. The gate
 
 - Pass `<stateDir>/incremental.json` — not Stryker's `reports/mutation/mutation.json` — and every
-  report on gated runs.
-- Commit before re-running the harness for `pr-ready`.
-- The metareview gate's freshness findings (stale, pending, unattested), `--mutation-view`, and the
-  enforcement modes ship in the same 0.13.0 release; see `metareview review --help` for
-  `--mutation-report`.
+  report on gated runs. The gate reads `attestation.json` beside each report.
+- **What the gate checks.** For every kill in an attested report, it re-derives from content
+  digests whether the kill still describes the code under review:
+  - **verified** — nothing it depends on changed;
+  - **stale** — its file changed, a test that killed it changed, a changed file's covering tests
+    killed it, or a support, global or unclassified input changed; the first cause is recorded;
+  - **pending** — its file is covered by a deferral;
+  - **unbound** — its file is not attested, or no test is recorded as killing it;
+  - **unattested** — the report has no valid attestation.
+- **What it reads.** task-done and epic-ready read the working tree. pr-ready reads HEAD (unless
+  `--include-working-tree`), so commit before re-running the harness.
+- **Modes.** Set `METAREVIEW_MUTATION_FRESHNESS=advisory|enforce`; the default is `advisory`, and
+  any other value is a usage error (exit 2). task-done is always advisory.
+  - Under `enforce` (pr-ready, epic-ready), stale and unattested Stryker findings block.
+  - Pending never blocks: deferred work is not evidence, and full runs never block PRs.
+- **The review log** gains a "Mutation Evidence Freshness" section: mode, reports, kill counts by
+  class, deferral reasons, and a re-run list of (file, cause) rows.
+- **Clearing.** A later run that supplies fresh reports of the same engine supersedes stale,
+  pending and unattested findings it no longer produces. A run without reports leaves them alone.
+- **Escalation.** A chain blocked only by stale evidence waits for a refresh for up to
+  2 × `maxAttempts` before it escalates ("stale mutation evidence not refreshed after N attempts").
+- **Views.** `--mutation-view` (per-view findings) arrives with views support.
 - An enforced stale finding clears only on a later run that supplies reports, or by override.
   Overrides on pending or unattested findings last only for that report. Every mutation override is
   tied to the mode in its fingerprint, and an override on one view does not cover the same cause
diff --git a/internal/epicready/coverage_test.go b/internal/epicready/coverage_test.go
index ffe04d9..fa9fd14 100644
--- a/internal/epicready/coverage_test.go
+++ b/internal/epicready/coverage_test.go
@@ -322,16 +322,16 @@ func TestFilterGeneratedHelpers(t *testing.T) {
 }
 
 func TestVerdictForCounts(t *testing.T) {
-	if v, _, blk, r := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 3, 3); v != "ESCALATED" || !blk || r == "" {
+	if v, _, blk, r := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 3, 3, false); v != "ESCALATED" || !blk || r == "" {
 		t.Fatalf("ESCALATED: %q %v %q", v, blk, r)
 	}
-	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 1, 3); v != "NEEDS_REVISION" || !blk {
+	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 1, 3, false); v != "NEEDS_REVISION" || !blk {
 		t.Fatalf("NEEDS_REVISION: %q %v", v, blk)
 	}
-	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Advisory: 1}, "gate", 1, 3); v != "PASS_ADVISORY" || blk {
+	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Advisory: 1}, "gate", 1, 3, false); v != "PASS_ADVISORY" || blk {
 		t.Fatalf("PASS_ADVISORY: %q %v", v, blk)
 	}
-	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{}, "gate", 1, 3); v != "PASS" || blk {
+	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{}, "gate", 1, 3, false); v != "PASS" || blk {
 		t.Fatalf("PASS: %q %v", v, blk)
 	}
 }
diff --git a/internal/epicready/freshness_test.go b/internal/epicready/freshness_test.go
new file mode 100644
index 0000000..f4cc827
--- /dev/null
+++ b/internal/epicready/freshness_test.go
@@ -0,0 +1,42 @@
+package epicready
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"fmt"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+// Spec §6.6 at epic-ready: the freshness section follows the verdict line.
+func TestCreateWritesTheMutationFreshnessSection(t *testing.T) {
+	root := epicRepo(t)
+	data, err := os.ReadFile(filepath.Join(root, "src/a.go"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
+	report := `{"files":{"src/a.go":{"source":"x","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{}}`
+	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
+		"lists":{"mutate":[],"test":[],"support":[],"global":[],"ignore":["**",".beads/**",".metareview/**","docs/**"]},"exclusions":[],
+		"files":{"src/a.go":{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum([]byte(report)), sum(data))
+	dir := t.TempDir()
+	for name, body := range map[string]string{"incremental.json": report, "attestation.json": att} {
+		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
+			t.Fatal(err)
+		}
+	}
+	res, err := Create(root, "epic-1", Options{Base: "main", MutationReportPaths: []string{filepath.Join(dir, "incremental.json")}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	log, err := os.ReadFile(filepath.Join(root, res.ReviewRel))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(string(log), "## Mutation Evidence Freshness\n\n- Mode: `advisory`") || !strings.Contains(string(log), "- Kills: 1 verified") {
+		t.Errorf("review log lacks the freshness section:\n%s", log)
+	}
+}
diff --git a/internal/epicready/mutationctx_test.go b/internal/epicready/mutationctx_test.go
index 6441e2d..c1d248c 100644
--- a/internal/epicready/mutationctx_test.go
+++ b/internal/epicready/mutationctx_test.go
@@ -12,7 +12,7 @@ import (
 // A skipped report is indistinguishable from a package with no survivors, so the failure mode of
 // getting this wrong is a gate that passes because it looked at less.
 func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
-	if _, err := mutationContextFor(nil); err != nil {
+	if _, err := mutationContextFor(t.TempDir(), nil); err != nil {
 		t.Errorf("no reports at all is the ordinary case: %v", err)
 	}
 	for name, path := range map[string]string{
@@ -21,7 +21,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 		"empty file":    writeTemp(t, ""),
 		"files is text": writeTemp(t, `{"files":"none"}`),
 	} {
-		ctx, err := mutationContextFor([]string{path})
+		ctx, err := mutationContextFor(t.TempDir(), []string{path})
 		if err == nil {
 			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
 			continue
@@ -35,7 +35,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 // A readable report reaches the reviewers, so the review actually acts on it.
 func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
 	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
-	ctx, err := mutationContextFor([]string{path})
+	ctx, err := mutationContextFor(t.TempDir(), []string{path})
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/internal/epicready/review.go b/internal/epicready/review.go
index 14c9897..c5b4fd7 100644
--- a/internal/epicready/review.go
+++ b/internal/epicready/review.go
@@ -2,7 +2,6 @@ package epicready
 
 import (
 	"fmt"
-	"github.com/dsifry/metareview/internal/mutation"
 	"os"
 	"path/filepath"
 	"strings"
@@ -147,7 +146,7 @@ func Create(root, target string, options Options) (Result, error) {
 	if report.Capabilities.Beads || report.Capabilities.Metaswarm {
 		gateEffect = "gate"
 	}
-	mutationContext, err := mutationContextFor(options.MutationReportPaths)
+	mutationContext, err := mutationContextFor(root, options.MutationReportPaths)
 	if err != nil {
 		return Result{}, err
 	}
@@ -190,15 +189,16 @@ func Create(root, target string, options Options) (Result, error) {
 			previousRunIDs = append(previousRunIDs, link.ID)
 		}
 		reconciled, err := reconcileFindings(root, run, rawFindings, findings.Options{
-			PreviousRunID:  options.PreviousRunID,
-			PreviousRunIDs: previousRunIDs,
-			ResetRunIDs:    chain.ResetRunIDs,
+			PreviousRunID:   options.PreviousRunID,
+			PreviousRunIDs:  previousRunIDs,
+			ResetRunIDs:     chain.ResetRunIDs,
+			MutationEngines: mutationContext.Engines(),
 		})
 		if err != nil {
 			return err
 		}
 		counts := findings.CountByClass(reconciled.OpenFindings)
-		verdict, status, blocking, escalationReason := verdictForCounts(counts, gateEffect, chain.AttemptNumber, chain.MaxAttempts)
+		verdict, status, blocking, escalationReason := verdictForCounts(counts, gateEffect, chain.AttemptNumber, chain.MaxAttempts, findings.OnlyStaleBlockers(reconciled.OpenFindings))
 		result.Verdict = verdict
 		result.Blocking = blocking
 		record := runRecord{
@@ -242,7 +242,7 @@ func Create(root, target string, options Options) (Result, error) {
 			FollowUpFindingCount: counts.FollowUp,
 			WarningFindingCount:  counts.Warnings,
 		}
-		return writeFile(reviewPath, []byte(reviewMarkdown(runID, target, contextRel, options.PreviousRunID, gateEffect, verdict, git.ChangedFiles, reconciled.OpenFindings, meta)), 0o644)
+		return writeFile(reviewPath, []byte(reviewMarkdown(runID, target, contextRel, options.PreviousRunID, gateEffect, verdict, git.ChangedFiles, reconciled.OpenFindings, mutationContext.FreshnessSection, meta)), 0o644)
 	}()
 	if err != nil {
 		restoreSnapshots(snapshots)
@@ -621,11 +621,19 @@ type reviewMetadata struct {
 	WarningFindingCount  int
 }
 
-func verdictForCounts(counts findings.ClassCounts, gateEffect string, attemptNumber, maxAttempts int) (string, string, bool, string) {
+func verdictForCounts(counts findings.ClassCounts, gateEffect string, attemptNumber, maxAttempts int, staleOnly bool) (string, string, bool, string) {
 	blocking := counts.Blocking > 0
 	nonBlocking := counts.Advisory > 0 || counts.FollowUp > 0 || counts.Warnings > 0
 	if blocking && attemptNumber >= maxAttempts {
+		// Spec §6.8: a chain blocked only by stale mutation evidence waits for a refresh, up to
+		// 2 × maxAttempts, before it escalates.
+		if staleOnly && attemptNumber < 2*maxAttempts {
+			return "NEEDS_REVISION", "needs-revision", true, ""
+		}
 		reason := fmt.Sprintf("blocking findings remain after attempt %d of %d", attemptNumber, maxAttempts)
+		if staleOnly {
+			reason = fmt.Sprintf("stale mutation evidence not refreshed after %d attempts", attemptNumber)
+		}
 		return "ESCALATED", "escalated", true, reason
 	}
 	if blocking {
@@ -637,7 +645,10 @@ func verdictForCounts(counts findings.ClassCounts, gateEffect string, attemptNum
 	return "PASS", "passed", false, ""
 }
 
-func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, coveredPaths []string, records []findings.Record, meta reviewMetadata) string {
+func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, coveredPaths []string, records []findings.Record, freshnessSection string, meta reviewMetadata) string {
+	if freshnessSection != "" {
+		freshnessSection += "\n\n"
+	}
 	// Covered paths: the exclude-filtered source files this review examined, so `status` can credit a
 	// clean review for the files it read (see reviewlog.DecodeCoveredPaths / status coverage accounting).
 	return "# metareview: epic-ready review\n\n" +
@@ -648,7 +659,7 @@ func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict
 		"Gate effect: " + markdown.InlineCode(gateEffect) + "\n\n" +
 		"Previous run: " + markdown.InlineCode(firstNonEmpty(previousRun, "none")) + "\n\n" +
 		reviewlog.CoveredPathsLabel + " " + markdown.InlineCode(reviewlog.EncodeCoveredPaths(coveredPaths)) + "\n\n" +
-		"## Verdict\n\n" + verdict + "\n\n" +
+		"## Verdict\n\n" + verdict + "\n\n" + freshnessSection +
 		"## Reviewer Results\n\n| Reviewer | Verdict | Blocking | Notes |\n| --- | --- | ---: | --- |\n" +
 		reviewerTable(records) + "\n\n" +
 		findingsMarkdown(records) + "\n" +
@@ -844,13 +855,6 @@ func firstNonEmpty(values ...string) string {
 // mutationContextFor loads the declared mutation reports. An unreadable or unrecognised report is
 // an error that stops the review, never a skipped file: a mutation gate that quietly drops a
 // report is a gate that passes because it looked at less.
-func mutationContextFor(paths []string) (reviewers.MutationContext, error) {
-	if len(paths) == 0 {
-		return reviewers.MutationContext{}, nil
-	}
-	reports, err := mutation.LoadAll(paths)
-	if err != nil {
-		return reviewers.MutationContext{}, err
-	}
-	return reviewers.MutationContext{Reports: reports}, nil
+func mutationContextFor(root string, paths []string) (reviewers.MutationContext, error) {
+	return reviewers.LoadMutationContext(root, paths, "epic-ready", false)
 }
diff --git a/internal/epicready/review_markdown_test.go b/internal/epicready/review_markdown_test.go
index 3e7b3f5..771a920 100644
--- a/internal/epicready/review_markdown_test.go
+++ b/internal/epicready/review_markdown_test.go
@@ -19,7 +19,7 @@ func TestEpicReadyReviewMarkdownEmitsRoundTrippableCoveredPaths(t *testing.T) {
 	if err := os.MkdirAll(dir, 0o755); err != nil {
 		t.Fatal(err)
 	}
-	md := reviewMarkdown("mrv-epic-cov", "epic-1", "ctx.md", "", "gate", "PASS", []string{"internal/foo.go", "a,b.go"}, nil, reviewMetadata{})
+	md := reviewMarkdown("mrv-epic-cov", "epic-1", "ctx.md", "", "gate", "PASS", []string{"internal/foo.go", "a,b.go"}, nil, "", reviewMetadata{})
 	if err := os.WriteFile(filepath.Join(dir, "mrv-epic-cov.md"), []byte(md), 0o644); err != nil {
 		t.Fatal(err)
 	}
@@ -39,11 +39,11 @@ func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
 		{Reviewer: "acceptance-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
 		{Reviewer: "architecture-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
 	}
-	md := reviewMarkdown("mrv-epic", "epic-1", "ctx.md", "", "gate", "PASS_ADVISORY", []string{"internal/foo.go"}, records, reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
+	md := reviewMarkdown("mrv-epic", "epic-1", "ctx.md", "", "gate", "PASS_ADVISORY", []string{"internal/foo.go"}, records, "## Mutation Evidence Freshness\n\n- Mode: `advisory`", reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
 	if strings.Contains(md, "| epic-integration-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| acceptance-reviewer | NEEDS_REVISION | 1 |") {
 		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
 	}
-	for _, required := range []string{"| epic-integration-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
+	for _, required := range []string{"PASS_ADVISORY\n\n## Mutation Evidence Freshness\n\n- Mode: `advisory`\n\n## Reviewer Results", "| epic-integration-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
 		if !strings.Contains(md, required) {
 			t.Fatalf("review markdown missing %q:\n%s", required, md)
 		}
@@ -52,7 +52,7 @@ func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
 
 func TestVerdictForNonBlockingFindingsIsPassAdvisory(t *testing.T) {
 	counts := findings.ClassCounts{Advisory: 1, FollowUp: 1}
-	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3)
+	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3, false)
 	if verdict != "PASS_ADVISORY" || status != "passed" || blocking || reason != "" {
 		t.Fatalf("non-blocking findings must produce PASS_ADVISORY, got verdict=%s status=%s blocking=%v reason=%q", verdict, status, blocking, reason)
 	}
diff --git a/internal/epicready/stale_escalation_test.go b/internal/epicready/stale_escalation_test.go
new file mode 100644
index 0000000..90be3d3
--- /dev/null
+++ b/internal/epicready/stale_escalation_test.go
@@ -0,0 +1,31 @@
+package epicready
+
+import (
+	"testing"
+
+	"github.com/dsifry/metareview/internal/findings"
+)
+
+// Spec §6.8: a chain blocked only by stale mutation evidence waits for fresh evidence up to
+// 2 × maxAttempts before escalating; any other blocker escalates at maxAttempts as before.
+func TestStaleOnlyChainsWaitForFreshEvidence(t *testing.T) {
+	blocking := findings.ClassCounts{Blocking: 1}
+	cases := []struct {
+		attempt, max int
+		staleOnly    bool
+		verdict      string
+		reason       string
+	}{
+		{3, 3, false, "ESCALATED", "blocking findings remain after attempt 3 of 3"},
+		{3, 3, true, "NEEDS_REVISION", ""},
+		{5, 3, true, "NEEDS_REVISION", ""},
+		{6, 3, true, "ESCALATED", "stale mutation evidence not refreshed after 6 attempts"},
+		{2, 3, true, "NEEDS_REVISION", ""},
+	}
+	for _, c := range cases {
+		verdict, _, _, reason := verdictForCounts(blocking, "gate", c.attempt, c.max, c.staleOnly)
+		if verdict != c.verdict || reason != c.reason {
+			t.Errorf("%+v: got %s %q", c, verdict, reason)
+		}
+	}
+}
diff --git a/internal/findings/findings.go b/internal/findings/findings.go
index 636e2d5..150f241 100644
--- a/internal/findings/findings.go
+++ b/internal/findings/findings.go
@@ -6,6 +6,7 @@ import (
 	"os"
 	"path/filepath"
 	"regexp"
+	"slices"
 	"strings"
 	"time"
 
@@ -30,6 +31,9 @@ type Options struct {
 	PreviousRunID  string
 	PreviousRunIDs []string
 	ResetRunIDs    []string
+	// MutationEngines are the engines of this run's --mutation-report files, and empty when the run
+	// supplied none, so mutation-freshness rows and their overrides are left alone (spec §6.5).
+	MutationEngines []string
 }
 
 type Evidence struct {
@@ -140,18 +144,24 @@ func Reconcile(root string, run Run, current []Input, options Options) (Result,
 			sameRunTarget(record, run) &&
 			(record.Status == "open" || record.Status == StatusOverridePending) &&
 			record.Fingerprint != "" &&
+			!IsFreshnessFingerprint(record.Fingerprint) &&
 			!currentFingerprints[record.Fingerprint] {
 			record.Status = "fixed"
 			record.FixedInRunID = run.ID
 			record.UpdatedAt = now
 			record.GitHead = run.GitHead
 		}
+		if supersedesFreshness(record, run, options, currentFingerprints) {
+			record.Status = StatusSuperseded
+			record.UpdatedAt = now
+			record.GitHead = run.GitHead
+		}
 		updated = append(updated, record)
 	}
 
 	activeExisting := map[string]bool{}
 	for _, record := range updated {
-		if record.Status != "fixed" && record.Fingerprint != "" && sameRunTarget(record, run) {
+		if record.Status != "fixed" && record.Status != StatusSuperseded && record.Fingerprint != "" && sameRunTarget(record, run) {
 			activeExisting[record.Fingerprint] = true
 		}
 	}
@@ -193,6 +203,52 @@ func Reconcile(root string, run Run, current []Input, options Options) (Result,
 // correction), and its fixedInRunId stays empty for the same reason.
 const StatusSuperseded = "superseded"
 
+// freshnessPrefixes are the mutation-freshness findings (spec §6.5). Fresh evidence supersedes
+// them; they are never "fixed", because a refresh is not a correction learning should read.
+var freshnessPrefixes = []string{"mutation:stale:", "mutation:pending:", "mutation:unattested:"}
+
+// IsFreshnessFingerprint reports a mutation-freshness finding.
+func IsFreshnessFingerprint(fingerprint string) bool {
+	for _, prefix := range freshnessPrefixes {
+		if strings.HasPrefix(fingerprint, prefix) {
+			return true
+		}
+	}
+	return false
+}
+
+// freshnessEngine is the <engine> field of mutation:<kind>:<mode>:<engine>:….
+func freshnessEngine(fingerprint string) string {
+	parts := strings.SplitN(fingerprint, ":", 5)
+	if len(parts) < 4 {
+		return ""
+	}
+	return parts[3]
+}
+
+// supersedesFreshness: this run supplied reports of the row's engine and did not reproduce it.
+func supersedesFreshness(record Record, run Run, options Options, current map[string]bool) bool {
+	return len(options.MutationEngines) > 0 && IsFreshnessFingerprint(record.Fingerprint) &&
+		slices.Contains(options.MutationEngines, freshnessEngine(record.Fingerprint)) &&
+		(record.Status == "open" || record.Status == StatusOverridePending) &&
+		sameRunTarget(record, run) && !current[record.Fingerprint]
+}
+
+// OnlyStaleBlockers reports that every open blocking finding is stale mutation evidence (spec §6.8).
+func OnlyStaleBlockers(records []Record) bool {
+	blocking := 0
+	for _, record := range records {
+		if !IsBlockingClass(record) {
+			continue
+		}
+		if !strings.HasPrefix(record.Fingerprint, "mutation:stale:") {
+			return false
+		}
+		blocking++
+	}
+	return blocking > 0
+}
+
 // legacyContextRiskPrefixes are the reason-bearing context-risk fingerprints
 // 0.8.3 replaced with reason-independent ones.
 var legacyContextRiskPrefixes = []string{
diff --git a/internal/findings/freshness_test.go b/internal/findings/freshness_test.go
new file mode 100644
index 0000000..a95f895
--- /dev/null
+++ b/internal/findings/freshness_test.go
@@ -0,0 +1,137 @@
+package findings
+
+import (
+	"strings"
+	"testing"
+)
+
+const staleFP = "mutation:stale:enforce:stryker:src/a.ts:0123abcd"
+
+func staleInput() Input {
+	return Input{Reviewer: "mutation-freshness", Severity: "high", Classification: "blocking", Title: "Mutation evidence stale: src/a.ts changed",
+		Evidence: []Evidence{{Type: "mutant", Path: "src/a.ts"}}, Fingerprint: staleFP}
+}
+
+func run(id string) Run {
+	return Run{ID: id, Scope: "pr-ready", Target: map[string]string{"branch": "b"}, GitHead: "h-" + id}
+}
+
+func statusOf(t *testing.T, root, fingerprint string) []string {
+	t.Helper()
+	records, err := All(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var out []string
+	for _, r := range records {
+		if r.Fingerprint == fingerprint {
+			out = append(out, r.Status+"/"+r.FixedInRunID)
+		}
+	}
+	return out
+}
+
+func TestIsFreshnessFingerprint(t *testing.T) {
+	for fp, want := range map[string]bool{staleFP: true, "mutation:pending:advisory:stryker:0123abcd": true,
+		"mutation:unattested:advisory:stryker:0123abcd": true, "mutation:survived:M:a.ts:1": false, "quality:lint-marker": false} {
+		if IsFreshnessFingerprint(fp) != want {
+			t.Errorf("%s: want %v", fp, want)
+		}
+	}
+	if freshnessEngine("mutation:stale:x") != "" || freshnessEngine(staleFP) != "stryker" {
+		t.Error("engine field")
+	}
+}
+
+func TestFreshEvidenceSupersedesAStaleRowNeverFixesIt(t *testing.T) {
+	root := t.TempDir()
+	engines := Options{MutationEngines: []string{"stryker"}}
+	if _, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines); err != nil {
+		t.Fatal(err)
+	}
+	// A run without reports leaves the row (and any override) alone.
+	if _, err := Reconcile(root, run("r2"), nil, Options{PreviousRunID: "r1"}); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); len(got) != 1 || got[0] != "open/" {
+		t.Fatalf("a run without reports: %v", got)
+	}
+	// A gremlins-only run leaves stryker rows alone.
+	if _, err := Reconcile(root, run("r3"), nil, Options{MutationEngines: []string{"gremlins"}}); err != nil {
+		t.Fatal(err)
+	}
+	// Another target is untouched.
+	other := run("r4")
+	other.Target = map[string]string{"branch": "other"}
+	if _, err := Reconcile(root, other, nil, engines); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); got[0] != "open/" {
+		t.Fatalf("gremlins-only or another target: %v", got)
+	}
+	// Fresh stryker evidence, with a previous-run chain: superseded, fixedInRunId empty.
+	res, err := Reconcile(root, run("r5"), nil, Options{PreviousRunID: "r1", MutationEngines: []string{"stryker"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" || res.OpenBlockingCount != 0 {
+		t.Fatalf("fresh evidence: %v (open blocking %d)", got, res.OpenBlockingCount)
+	}
+	// The fingerprint recurring opens a new row.
+	if _, err := Reconcile(root, run("r6"), []Input{staleInput()}, engines); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); len(got) != 2 || got[1] != "open/" {
+		t.Fatalf("recurrence: %v", got)
+	}
+}
+
+func TestOverriddenFreshnessRowsStayAndSupersededRowsTakeEscalatedOverrides(t *testing.T) {
+	root := t.TempDir()
+	engines := Options{MutationEngines: []string{"stryker"}}
+	res, err := Reconcile(root, run("r1"), []Input{staleInput()}, engines)
+	if err != nil {
+		t.Fatal(err)
+	}
+	id := res.NewFindings[0].ID
+	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "stale-only escalation", Now: "2026-09-24T00:00:00Z"}); err != nil {
+		t.Fatal(err)
+	}
+	// An override-pending row is superseded by fresh evidence, and keeps its request on the index.
+	if _, err := Reconcile(root, run("r2"), nil, engines); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); got[0] != StatusSuperseded+"/" {
+		t.Fatalf("override-pending: %v", got)
+	}
+	records, _ := All(root)
+	if lines := overrideLines(records); len(lines) != 1 || !strings.Contains(lines[0], "[superseded]") {
+		t.Errorf("override lines: %v", lines)
+	}
+	// A superseded freshness row accepts an override request only with an escalation.
+	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "a second request without escalation", Now: "2026-09-24T00:00:01Z"}); err == nil {
+		t.Error("no escalation: refused")
+	}
+	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "lift the stale-only escalation", Escalation: "ESCALATED run r9", Now: "2026-09-24T00:00:02Z"}); err != nil {
+		t.Fatalf("with an escalation: %v", err)
+	}
+	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted by the maintainer", Now: "2026-09-24T00:00:03Z"}); err != nil {
+		t.Fatalf("grant: %v", err)
+	}
+	// An overridden row is never superseded.
+	if _, err := Reconcile(root, run("r3"), nil, engines); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, staleFP); got[0] != StatusOverridden+"/" {
+		t.Errorf("overridden: %v", got)
+	}
+}
+
+func TestOnlyStaleBlockers(t *testing.T) {
+	stale := Record{Classification: "blocking", Severity: "high", Fingerprint: staleFP}
+	other := Record{Classification: "blocking", Severity: "high", Fingerprint: "quality:lint-marker"}
+	advisory := Record{Classification: "advisory", Severity: "medium", Fingerprint: "x"}
+	if !OnlyStaleBlockers([]Record{stale, advisory}) || OnlyStaleBlockers([]Record{stale, other}) || OnlyStaleBlockers([]Record{advisory}) {
+		t.Error("true only when every blocker is stale mutation evidence and there is one")
+	}
+}
diff --git a/internal/findings/override.go b/internal/findings/override.go
index c8734fc..2fa66de 100644
--- a/internal/findings/override.go
+++ b/internal/findings/override.go
@@ -71,7 +71,7 @@ func RequestOverride(root, findingID string, request OverrideRequest) error {
 		// escalation whose hard stop it asks to lift (request.Escalation — issue #147):
 		// the run-level stop can outlive the finding-level fix, and the recorded request
 		// is what makes the later grant two-phase (requester ≠ grantor).
-		fixedWithEscalation := record.Status == "fixed" && strings.TrimSpace(request.Escalation) != ""
+		fixedWithEscalation := (record.Status == "fixed" || supersededFreshness(*record)) && strings.TrimSpace(request.Escalation) != ""
 		if record.Status != "open" && !fixedWithEscalation {
 			return fmt.Errorf("finding %s is %s, not open", findingID, record.Status)
 		}
@@ -113,7 +113,7 @@ func GrantOverride(root, findingID string, grant OverrideGrant) error {
 		// escalation was already filed (record.OverrideEscalation — issue #147): the
 		// run-level stop can outlive the finding-level fix, and lifting it is the human
 		// decision the grant records — with requester ≠ grantor enforced below.
-		fixedWithEscalation := record.Status == "fixed" && strings.TrimSpace(record.OverrideEscalation) != ""
+		fixedWithEscalation := (record.Status == "fixed" || supersededFreshness(*record)) && strings.TrimSpace(record.OverrideEscalation) != ""
 		if record.Status != "open" && !fixedWithEscalation && record.Status != StatusOverridePending {
 			return fmt.Errorf("finding %s is %s and cannot be overridden", findingID, record.Status)
 		}
@@ -186,6 +186,12 @@ func mutateFinding(root, findingID string, apply func(*Record) error) error {
 	return RenderIndexWithRecords(root, records)
 }
 
+// supersededFreshness: a freshness row that fresh evidence replaced. Like a fixed row, it takes an
+// override only with an escalation, so a stale-only escalation (spec §6.8) can still be lifted.
+func supersededFreshness(record Record) bool {
+	return record.Status == StatusSuperseded && IsFreshnessFingerprint(record.Fingerprint)
+}
+
 // overrideLines renders the process-exception section of the findings index.
 func overrideLines(records []Record) []string {
 	var lines []string
@@ -201,6 +207,11 @@ func overrideLines(records []Record) []string {
 		case StatusOverridePending:
 			lines = append(lines, withEscalation(fmt.Sprintf("- %s [pending] %s — requested by %s at %s: %s",
 				record.ID, title, reqBy, reqAt, reqReason), record))
+		case StatusSuperseded:
+			if record.OverrideRequestedBy != "" {
+				lines = append(lines, withEscalation(fmt.Sprintf("- %s [superseded] %s — requested by %s at %s: %s",
+					record.ID, title, reqBy, reqAt, reqReason), record))
+			}
 		case StatusOverridden:
 			detail := fmt.Sprintf("- %s [granted] %s — granted by %s at %s: %s",
 				record.ID, title, grantedBy, grantedAt, singleLine(record.OverrideGrantReason))
diff --git a/internal/learning/candidates.go b/internal/learning/candidates.go
index 9d7565c..abf3b15 100644
--- a/internal/learning/candidates.go
+++ b/internal/learning/candidates.go
@@ -61,6 +61,9 @@ func ExtractCandidates(input Input) Result {
 func knowledgeFromFindings(records []findings.Record) []Candidate {
 	var candidates []Candidate
 	for _, record := range records {
+		if findings.IsFreshnessFingerprint(record.Fingerprint) {
+			continue // freshness is evidence bookkeeping, not a lesson (spec §6.9)
+		}
 		if record.KnowledgeCandidate {
 			candidates = append(candidates, Candidate{
 				Kind:           "knowledge-candidate",
@@ -90,6 +93,9 @@ func knowledgeFromFindings(records []findings.Record) []Candidate {
 func repeatedBlockerThemes(records []findings.Record) []Candidate {
 	groups := map[string][]findings.Record{}
 	for _, record := range records {
+		if findings.IsFreshnessFingerprint(record.Fingerprint) {
+			continue // freshness is evidence bookkeeping, not a lesson (spec §6.9)
+		}
 		if !isBlocking(record) {
 			continue
 		}
diff --git a/internal/learning/freshness_test.go b/internal/learning/freshness_test.go
new file mode 100644
index 0000000..3693f55
--- /dev/null
+++ b/internal/learning/freshness_test.go
@@ -0,0 +1,28 @@
+package learning
+
+import (
+	"testing"
+
+	"github.com/dsifry/metareview/internal/findings"
+)
+
+// Spec §6.9: mutation-freshness findings are evidence bookkeeping, not lessons — a stale row that
+// fresh evidence replaced is neither a fixed defect nor a repeated blocker theme.
+func TestLearningSkipsFreshnessFindings(t *testing.T) {
+	stale := findings.Record{ID: "f1", Status: "fixed", FixedInRunID: "r2", KnowledgeCandidate: true, Classification: "blocking", Severity: "high",
+		Title: "Mutation evidence stale: src/a.ts changed", Fingerprint: "mutation:stale:enforce:stryker:src/a.ts:01234567"}
+	again := stale
+	again.ID = "f2"
+	if got := knowledgeFromFindings([]findings.Record{stale}); len(got) != 0 {
+		t.Errorf("no knowledge candidate from a freshness row: %+v", got)
+	}
+	if got := repeatedBlockerThemes([]findings.Record{stale, again}); len(got) != 0 {
+		t.Errorf("no blocker theme from freshness rows: %+v", got)
+	}
+	// An ordinary finding is still learned from.
+	ordinary := stale
+	ordinary.Fingerprint = "quality:lint-marker"
+	if got := knowledgeFromFindings([]findings.Record{ordinary}); len(got) == 0 {
+		t.Error("ordinary findings still produce candidates")
+	}
+}
diff --git a/internal/mutation/parse.go b/internal/mutation/parse.go
index 9e484ba..a60412e 100644
--- a/internal/mutation/parse.go
+++ b/internal/mutation/parse.go
@@ -1,6 +1,8 @@
 package mutation
 
 import (
+	"crypto/sha256"
+	"encoding/hex"
 	"encoding/json"
 	"errors"
 	"fmt"
@@ -32,13 +34,13 @@ func Parse(data []byte, target string) (Report, error) {
 			if err != nil {
 				return r, err
 			}
-			return checkMeasured(r, target)
+			return checkMeasured(r, target, data)
 		case '[':
 			r, err := ParseGremlins(data, target)
 			if err != nil {
 				return r, err
 			}
-			return checkMeasured(r, target)
+			return checkMeasured(r, target, data)
 		}
 	}
 	// No `files` at all, or something that is neither. Refusing is the point: a report this code
@@ -55,10 +57,11 @@ func Parse(data []byte, target string) (Report, error) {
 var errNoMutants = errors.New("describes no mutants, so it measured nothing")
 
 // checkMeasured rejects a parsed report that carries no mutants.
-func checkMeasured(r Report, target string) (Report, error) {
+func checkMeasured(r Report, target string, data []byte) (Report, error) {
 	if len(r.Mutants) == 0 {
 		return Report{}, fmt.Errorf("mutation: %s %w", reportName(target), errNoMutants)
 	}
+	r.SHA256 = reportSHA256(data)
 	return r, nil
 }
 
@@ -110,3 +113,9 @@ func sortedKeys[V any](m map[string]V) []string {
 	sort.Strings(out)
 	return out
 }
+
+// reportSHA256 is the report file's digest, which its attestation's reportSha256 must equal (spec §6.1).
+func reportSHA256(data []byte) string {
+	sum := sha256.Sum256(data)
+	return hex.EncodeToString(sum[:])
+}
diff --git a/internal/mutation/report.go b/internal/mutation/report.go
index 143c613..a37baa5 100644
--- a/internal/mutation/report.go
+++ b/internal/mutation/report.go
@@ -50,6 +50,10 @@ type Report struct {
 	Engine  string   `json:"engine"`
 	Target  string   `json:"target,omitempty"`
 	Mutants []Mutant `json:"mutants"`
+	// SHA256 is the report file's sha256 (lowercase hex); Detail is the Stryker data the freshness
+	// gate reads (spec §6.1). Neither reaches JSON, so fingerprints and digests are unchanged.
+	SHA256 string         `json:"-"`
+	Detail *StrykerDetail `json:"-"`
 }
 
 // Score is the honest summary: every class counted, nothing folded away.
diff --git a/internal/mutation/stryker.go b/internal/mutation/stryker.go
index bf426bc..3eeaac9 100644
--- a/internal/mutation/stryker.go
+++ b/internal/mutation/stryker.go
@@ -19,19 +19,54 @@ import (
 type strykerReport struct {
 	SchemaVersion string `json:"schemaVersion"`
 	Files         map[string]struct {
+		Source  string `json:"source"`
 		Mutants []struct {
-			MutatorName string `json:"mutatorName"`
-			Status      string `json:"status"`
+			ID          string   `json:"id"`
+			MutatorName string   `json:"mutatorName"`
+			Status      string   `json:"status"`
+			KilledBy    []string `json:"killedBy"`
+			CoveredBy   []string `json:"coveredBy"`
 			Location    struct {
 				Start struct {
 					Line   int `json:"line"`
 					Column int `json:"column"`
 				} `json:"start"`
+				End struct {
+					Line int `json:"line"`
+				} `json:"end"`
 			} `json:"location"`
 			Description  string `json:"description,omitempty"`
 			StatusReason string `json:"statusReason,omitempty"`
 		} `json:"mutants"`
 	} `json:"files"`
+	TestFiles map[string]struct {
+		Tests []struct {
+			ID string `json:"id"`
+		} `json:"tests"`
+	} `json:"testFiles"`
+}
+
+// StrykerDetail is what the freshness gate reads from a mutation-testing-report-schema report
+// (spec §6.1). Ids are meaningful only within one report.
+type StrykerDetail struct {
+	Files   map[string]StrykerFile
+	TestIDs map[string][]string // test file → its test ids
+}
+
+// StrykerFile is one mutated file: the source Stryker read and its mutants.
+type StrykerFile struct {
+	Source  string
+	Mutants []StrykerMutant
+}
+
+// StrykerMutant carries the ids and span freshness needs.
+type StrykerMutant struct {
+	ID        string
+	Status    Status
+	KilledBy  []string
+	CoveredBy []string
+	StartLine int
+	EndLine   int
 }
 
 // ParseStryker normalises a mutation-testing-report-schema report.
@@ -55,6 +90,22 @@ func ParseStryker(data []byte, target string) (Report, error) {
 			})
 		}
 	}
+	r.Detail = &StrykerDetail{Files: map[string]StrykerFile{}, TestIDs: map[string][]string{}}
+	for path, f := range raw.Files {
+		file := StrykerFile{Source: f.Source}
+		for _, m := range f.Mutants {
+			file.Mutants = append(file.Mutants, StrykerMutant{
+				ID: m.ID, Status: strykerStatus(m.Status), KilledBy: m.KilledBy, CoveredBy: m.CoveredBy,
+				StartLine: m.Location.Start.Line, EndLine: m.Location.End.Line,
+			})
+		}
+		r.Detail.Files[path] = file
+	}
+	for path, tf := range raw.TestFiles {
+		for _, test := range tf.Tests {
+			r.Detail.TestIDs[path] = append(r.Detail.TestIDs[path], test.ID)
+		}
+	}
 	return r, nil
 }
 
diff --git a/internal/mutation/stryker_test.go b/internal/mutation/stryker_test.go
index 8d5c3cf..2007f31 100644
--- a/internal/mutation/stryker_test.go
+++ b/internal/mutation/stryker_test.go
@@ -1,6 +1,9 @@
 package mutation
 
 import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
 	"errors"
 	"os"
 	"path/filepath"
@@ -192,3 +195,28 @@ func TestParseEdgesAndFindingCaps(t *testing.T) {
 		t.Errorf("the example list must be capped and say how many it dropped: %q", got[0].Finding)
 	}
 }
+
+func TestParseKeepsFreshnessDetailOutOfJSON(t *testing.T) {
+	data := []byte(`{"schemaVersion":"1","files":{"src/a.ts":{"source":"x","mutants":[
+		{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1","t2"],"location":{"start":{"line":2,"column":3},"end":{"line":4,"column":1}}}]}},
+		"testFiles":{"tests/a.test.ts":{"tests":[{"id":"t1"},{"id":"t2"}]}}}`)
+	r, err := Parse(data, "r.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	sum := sha256.Sum256(data)
+	if r.SHA256 != hex.EncodeToString(sum[:]) {
+		t.Errorf("SHA256 %q", r.SHA256)
+	}
+	m := r.Detail.Files["src/a.ts"].Mutants[0]
+	if m.ID != "1" || m.Status != Killed || m.StartLine != 2 || m.EndLine != 4 || len(m.KilledBy) != 1 || len(m.CoveredBy) != 2 {
+		t.Errorf("mutant detail %+v", m)
+	}
+	if r.Detail.Files["src/a.ts"].Source != "x" || len(r.Detail.TestIDs["tests/a.test.ts"]) != 2 {
+		t.Errorf("detail %+v", r.Detail)
+	}
+	encoded, _ := json.Marshal(r)
+	if strings.Contains(string(encoded), "coveredBy") || strings.Contains(string(encoded), r.SHA256) {
+		t.Errorf("freshness detail must not reach JSON (fingerprints and digests are unchanged): %s", encoded)
+	}
+}
diff --git a/internal/mutationfresh/attest.go b/internal/mutationfresh/attest.go
new file mode 100644
index 0000000..ce99c2b
--- /dev/null
+++ b/internal/mutationfresh/attest.go
@@ -0,0 +1,75 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"os"
+	"path/filepath"
+)
+
+const harnessTool = "metareview-mutation-incremental"
+
+// Attestation is the part of <stateDir>/attestation.json the gate reads (spec §5.5, §11.2).
+type Attestation struct {
+	SchemaVersion int                  `json:"schemaVersion"`
+	Tool          string               `json:"tool"`
+	Engine        string               `json:"engine"`
+	Report        string               `json:"report"`
+	ReportSha256  string               `json:"reportSha256"`
+	Config        string               `json:"config"`
+	Lists         Lists                `json:"lists"`
+	Exclusions    []string             `json:"exclusions"`
+	Files         map[string]FileEntry `json:"files"`
+	Deferrals     []Deferral           `json:"deferrals"`
+}
+
+// FileEntry is one attested path: its start-of-run digest, category and whether git tracked it.
+type FileEntry struct {
+	Digest   string `json:"digest"`
+	Category string `json:"category"`
+	Tracked  bool   `json:"tracked"`
+}
+
+// Deferral is recorded pending work: kills in files named by paths (or "*") are pending.
+type Deferral struct {
+	Reason    string   `json:"reason"`
+	Paths     []string `json:"paths"`
+	Inherited bool     `json:"inherited"`
+}
+
+// readAttestation reads attestation.json beside the report and validates it against the report
+// (spec §6.1). It returns the attestation, the first failing reason ("" when attested) and the
+// attestation file's sha256 ("" when it cannot be read).
+func readAttestation(reportPath, engine, reportSHA string) (Attestation, string, string) {
+	path := filepath.Join(filepath.Dir(reportPath), "attestation.json")
+	data, err := os.ReadFile(path)
+	if err != nil {
+		return Attestation{}, "missing", ""
+	}
+	var att Attestation
+	if err := json.Unmarshal(data, &att); err != nil || att.Files == nil {
+		return Attestation{}, "unparseable", sha256Hex(data)
+	}
+	return att, attestationReason(att, path, reportPath, engine, reportSHA), sha256Hex(data)
+}
+
+func attestationReason(att Attestation, attPath, reportPath, engine, reportSHA string) string {
+	switch {
+	case att.SchemaVersion != 1:
+		return "version"
+	case att.Tool != harnessTool:
+		return "tool"
+	case att.Engine != engine:
+		return "engine"
+	case !sameFile(filepath.Join(filepath.Dir(attPath), filepath.FromSlash(att.Report)), reportPath):
+		return "report mismatch"
+	case att.ReportSha256 != reportSHA:
+		return "hash mismatch"
+	}
+	return ""
+}
+
+func sameFile(a, b string) bool {
+	ia, errA := os.Stat(a)
+	ib, errB := os.Stat(b)
+	return errA == nil && errB == nil && os.SameFile(ia, ib)
+}
diff --git a/internal/mutationfresh/attest_test.go b/internal/mutationfresh/attest_test.go
new file mode 100644
index 0000000..df21fda
--- /dev/null
+++ b/internal/mutationfresh/attest_test.go
@@ -0,0 +1,68 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"path/filepath"
+	"testing"
+)
+
+func writeAttestation(t *testing.T, dir string, att map[string]any) {
+	t.Helper()
+	data, err := json.Marshal(att)
+	if err != nil {
+		t.Fatal(err)
+	}
+	write(t, dir, "attestation.json", string(data))
+}
+
+func validAttestation(reportSHA string) map[string]any {
+	return map[string]any{
+		"schemaVersion": 1, "tool": "metareview-mutation-incremental", "engine": "stryker",
+		"report": "incremental.json", "reportSha256": reportSHA, "files": map[string]any{},
+	}
+}
+
+// Spec §6.1: attested iff it parses, schemaVersion is 1, the tool and engine match, `report`
+// resolves to the report path and reportSha256 equals the report's bytes; otherwise the first
+// failing reason.
+func TestReadAttestationReasons(t *testing.T) {
+	dir := t.TempDir()
+	write(t, dir, "incremental.json", "{}")
+	report := filepath.Join(dir, "incremental.json")
+	sha := sha256Hex([]byte("{}"))
+	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "missing" || attSHA != "" {
+		t.Errorf("no attestation: %q %q", reason, attSHA)
+	}
+	write(t, dir, "attestation.json", "not json")
+	if _, reason, attSHA := readAttestation(report, "stryker", sha); reason != "unparseable" || attSHA == "" {
+		t.Errorf("not JSON: %q", reason)
+	}
+	write(t, dir, "attestation.json", `{"schemaVersion":1}`)
+	if _, reason, _ := readAttestation(report, "stryker", sha); reason != "unparseable" {
+		t.Errorf("no files: %q", reason)
+	}
+	cases := []struct {
+		name   string
+		mutate func(map[string]any)
+		want   string
+	}{
+		{"valid", func(map[string]any) {}, ""},
+		{"version", func(a map[string]any) { a["schemaVersion"] = 2 }, "version"},
+		{"tool", func(a map[string]any) { a["tool"] = "other" }, "tool"},
+		{"engine", func(a map[string]any) { a["engine"] = "gremlins" }, "engine"},
+		{"report", func(a map[string]any) { a["report"] = "other.json" }, "report mismatch"},
+		{"hash", func(a map[string]any) { a["reportSha256"] = "00" }, "hash mismatch"},
+	}
+	for _, c := range cases {
+		att := validAttestation(sha)
+		c.mutate(att)
+		writeAttestation(t, dir, att)
+		got, reason, _ := readAttestation(report, "stryker", sha)
+		if reason != c.want {
+			t.Errorf("%s: reason %q, want %q", c.name, reason, c.want)
+		}
+		if c.want == "" && got.Report != "incremental.json" {
+			t.Errorf("valid attestation not returned: %+v", got)
+		}
+	}
+}
diff --git a/internal/mutationfresh/build.go b/internal/mutationfresh/build.go
new file mode 100644
index 0000000..b0c07e3
--- /dev/null
+++ b/internal/mutationfresh/build.go
@@ -0,0 +1,176 @@
+package mutationfresh
+
+import (
+	"fmt"
+	"sort"
+	"strings"
+
+	"github.com/dsifry/metareview/internal/findings"
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+const reviewer = "mutation-freshness"
+
+// Result is what a gate needs from the freshness check: the classifications (for pr-ready's
+// digest), the findings, and the review-log section.
+type Result struct {
+	Mode      string
+	Freshness []ReportFreshness
+	Findings  []findings.Input
+	Section   string
+}
+
+// Build classifies every report, then derives the findings (spec §6.5) and the section (§6.6).
+func Build(reports []mutation.Report, content Content, mode string) (Result, error) {
+	res := Result{Mode: mode}
+	for _, r := range reports {
+		f, err := Classify(r, content)
+		if err != nil {
+			return Result{}, err
+		}
+		res.Freshness = append(res.Freshness, f)
+	}
+	res.Findings = freshnessFindings(res.Freshness, mode)
+	res.Section = section(res.Freshness, mode)
+	return res, nil
+}
+
+func gate(mode string) (classification, severity string) {
+	if mode == Enforce {
+		return "blocking", "high" // classForCount counts blocking only at high or critical
+	}
+	return "advisory", "medium"
+}
+
+func freshnessFindings(list []ReportFreshness, mode string) []findings.Input {
+	type staleKey struct{ engine, cause string }
+	type staleSum struct {
+		digest  string
+		kills   int
+		reports []string
+	}
+	sums := map[staleKey]*staleSum{}
+	for _, f := range list {
+		for _, c := range f.Causes {
+			key := staleKey{f.Engine, c.Cause}
+			if sums[key] == nil {
+				sums[key] = &staleSum{digest: c.Digest}
+			}
+			sums[key].kills += c.Kills
+			sums[key].reports = append(sums[key].reports, f.Path)
+		}
+	}
+	keys := make([]staleKey, 0, len(sums))
+	for k := range sums {
+		keys = append(keys, k)
+	}
+	sort.Slice(keys, func(i, j int) bool {
+		return keys[i].engine+"\x00"+keys[i].cause < keys[j].engine+"\x00"+keys[j].cause
+	})
+	classification, severity := gate(mode)
+	var out []findings.Input
+	for _, k := range keys {
+		s := sums[k]
+		out = append(out, findings.Input{
+			Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "implementer",
+			Title:          "Mutation evidence stale: " + k.cause + " changed",
+			Finding:        fmt.Sprintf("%d kill(s) in %s were verified against a different %s, so they are not evidence for the code under review.", s.kills, strings.Join(s.reports, ", "), k.cause),
+			Expected:       "Every kill the gate counts was verified against the content under review.",
+			Found:          fmt.Sprintf("%s is now %s.", k.cause, s.digest),
+			Recommendation: "Re-run the harness (node tools/mutation-incremental/cli.mjs run --mode incremental), commit before running pr-ready, and pass the refreshed <stateDir>/incremental.json.",
+			Evidence:       []findings.Evidence{{Type: "mutant", Path: k.cause}},
+			Fingerprint:    fmt.Sprintf("mutation:stale:%s:%s:%s:%s", mode, k.engine, k.cause, sha256Hex([]byte(k.cause + "=" + s.digest))[:8]),
+		})
+	}
+	for _, f := range list {
+		if f.Pending > 0 {
+			out = append(out, findings.Input{
+				Reviewer: reviewer, Severity: "medium", Classification: "advisory", Owner: "reviewer",
+				Title:          fmt.Sprintf("Mutation evidence pending: %d kills", f.Pending),
+				Finding:        fmt.Sprintf("%d kill(s) in %s are covered by a deferral (%s) and wait for a full run, so they are not counted as evidence.", f.Pending, f.Path, strings.Join(f.Deferrals, "; ")),
+				Expected:       "Deferred work is re-verified by a full run before it counts.",
+				Recommendation: "Let main's full run clear the deferrals, or run the harness with --mode full locally.",
+				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
+				Fingerprint:    fmt.Sprintf("mutation:pending:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
+			})
+		}
+		if !f.Attested && f.Engine == "stryker" {
+			out = append(out, findings.Input{
+				Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "reviewer",
+				Title:          "Mutation report has no attestation",
+				Finding:        fmt.Sprintf("%s has no valid attestation (%s), so the gate cannot tell whether its %d kill(s) still describe the code under review.", f.Path, f.UnattestedReason, f.Unattested),
+				Expected:       "Mutation evidence comes from the mutation-incremental harness, which attests what it verified.",
+				Recommendation: "Produce the report with the harness and pass <stateDir>/incremental.json (its attestation.json sits beside it).",
+				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
+				Fingerprint:    fmt.Sprintf("mutation:unattested:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
+			})
+		}
+	}
+	return out
+}
+
+// section renders "## Mutation Evidence Freshness" (spec §6.6), or "" without reports.
+func section(list []ReportFreshness, mode string) string {
+	if len(list) == 0 {
+		return ""
+	}
+	var attested, verified, stale, pending, unbound, unattested int
+	var reasons, deferrals, unverifiable []string
+	seenDeferral, seenEngine := map[string]bool{}, map[string]bool{}
+	rows := map[[2]string]int{}
+	for _, f := range list {
+		verified += f.Verified
+		stale += f.Stale
+		pending += f.Pending
+		unbound += f.Unbound
+		unattested += f.Unattested
+		if f.Attested {
+			attested++
+		} else {
+			reasons = append(reasons, f.UnattestedReason)
+			if f.Engine != "stryker" && !seenEngine[f.Engine] {
+				seenEngine[f.Engine] = true
+				unverifiable = append(unverifiable, f.Engine)
+			}
+		}
+		for _, d := range f.Deferrals {
+			if !seenDeferral[d] {
+				seenDeferral[d] = true
+				deferrals = append(deferrals, d)
+			}
+		}
+		for _, row := range f.ReRun {
+			rows[[2]string{row.File, row.Cause}] += row.Kills
+		}
+	}
+	var b strings.Builder
+	b.WriteString("## Mutation Evidence Freshness\n\n")
+	fmt.Fprintf(&b, "- Mode: `%s`\n", mode)
+	fmt.Fprintf(&b, "- Reports: %d (%d attested", len(list), attested)
+	if len(reasons) > 0 {
+		fmt.Fprintf(&b, ", %d unattested: %s", len(reasons), strings.Join(reasons, ", "))
+	}
+	b.WriteString(")\n")
+	fmt.Fprintf(&b, "- Kills: %d verified, %d stale, %d pending, %d unbound, %d unattested\n", verified, stale, pending, unbound, unattested)
+	if len(deferrals) > 0 {
+		fmt.Fprintf(&b, "- Deferrals: %s\n", strings.Join(deferrals, "; "))
+	}
+	if len(unverifiable) > 0 {
+		sort.Strings(unverifiable)
+		fmt.Fprintf(&b, "- Freshness not verifiable: %s\n", strings.Join(unverifiable, ", "))
+	}
+	if len(rows) > 0 {
+		keys := make([][2]string, 0, len(rows))
+		for k := range rows {
+			keys = append(keys, k)
+		}
+		sort.Slice(keys, func(i, j int) bool {
+			return keys[i][0]+"\x00"+keys[i][1] < keys[j][0]+"\x00"+keys[j][1]
+		})
+		b.WriteString("\n### Re-run list\n\n| File | Cause | Stale kills |\n| --- | --- | ---: |\n")
+		for _, k := range keys {
+			fmt.Fprintf(&b, "| `%s` | `%s` | %d |\n", k[0], k[1], rows[k])
+		}
+	}
+	return strings.TrimRight(b.String(), "\n")
+}
diff --git a/internal/mutationfresh/build_test.go b/internal/mutationfresh/build_test.go
new file mode 100644
index 0000000..cd3548a
--- /dev/null
+++ b/internal/mutationfresh/build_test.go
@@ -0,0 +1,158 @@
+package mutationfresh
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+func TestModeFromEnv(t *testing.T) {
+	env := func(v string) func(string) string { return func(string) string { return v } }
+	for value, want := range map[string]string{"": Advisory, "advisory": Advisory, "enforce": Enforce} {
+		if got, err := ModeFromEnv(env(value)); err != nil || got != want {
+			t.Errorf("%q: %q %v", value, got, err)
+		}
+	}
+	if _, err := ModeFromEnv(env("strict")); err == nil || !strings.Contains(err.Error(), ModeEnv) {
+		t.Errorf("an unknown mode is a usage error naming %s: %v", ModeEnv, err)
+	}
+	if EffectiveMode("task-done", Enforce) != Advisory || EffectiveMode("pr-ready", Enforce) != Enforce {
+		t.Error("task-done is always advisory; the other gates use the mode")
+	}
+}
+
+func buildAt(t *testing.T, root, mode string, reports ...string) Result {
+	t.Helper()
+	var loaded []mutation.Report
+	for _, p := range reports {
+		r, err := mutation.Load(p)
+		if err != nil {
+			t.Fatal(err)
+		}
+		loaded = append(loaded, r)
+	}
+	res, err := Build(loaded, Worktree(root), mode)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return res
+}
+
+func TestBuildStaleFindingsSumCausesAcrossReportsAndFollowTheMode(t *testing.T) {
+	report, root := realCase(t, "full")
+	// A second copy of the same report and attestation, elsewhere: same cause, summed counts.
+	copyDir := t.TempDir()
+	for _, name := range []string{"incremental.json", "attestation.json"} {
+		data := readFile(t, filepath.Join(filepath.Dir(report), name))
+		write(t, copyDir, name, data)
+	}
+	write(t, root, "src/a.ts", "changed\n")
+	res := buildAt(t, root, Advisory, report, filepath.Join(copyDir, "incremental.json"))
+	if len(res.Findings) != 1 {
+		t.Fatalf("one finding per distinct cause, got %d: %+v", len(res.Findings), res.Findings)
+	}
+	f := res.Findings[0]
+	if f.Title != "Mutation evidence stale: src/a.ts changed" || !strings.HasPrefix(f.Finding, "34 kill(s) in ") ||
+		f.Classification != "advisory" || f.Severity != "medium" || f.Owner != "implementer" || f.Reviewer != "mutation-freshness" {
+		t.Errorf("advisory stale finding %+v", f)
+	}
+	digest := res.Freshness[0].Causes[0].Digest
+	if f.Fingerprint != "mutation:stale:advisory:stryker:src/a.ts:"+sha256Hex([]byte("src/a.ts=" + digest))[:8] || f.Evidence[0].Path != "src/a.ts" {
+		t.Errorf("fingerprint %q evidence %+v", f.Fingerprint, f.Evidence)
+	}
+	enforced := buildAt(t, root, Enforce, report)
+	if e := enforced.Findings[0]; e.Classification != "blocking" || e.Severity != "high" || !strings.HasPrefix(e.Fingerprint, "mutation:stale:enforce:") {
+		t.Errorf("enforce: %+v", e)
+	}
+}
+
+func TestBuildPendingStaysAdvisoryAndUnattestedFollowsTheMode(t *testing.T) {
+	report, root := realCase(t, "lockfile")
+	dir := t.TempDir()
+	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
+	gremlins := t.TempDir()
+	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
+	res := buildAt(t, root, Enforce, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"))
+	byPrefix := map[string]int{}
+	for _, f := range res.Findings {
+		byPrefix[strings.Join(strings.SplitN(f.Fingerprint, ":", 3)[:2], ":")]++
+		switch {
+		case strings.HasPrefix(f.Fingerprint, "mutation:pending:enforce:stryker:"):
+			if f.Classification != "advisory" || f.Title != "Mutation evidence pending: 24 kills" || !strings.Contains(f.Finding, "global input changed: package-lock.json") {
+				t.Errorf("pending stays advisory under enforce: %+v", f)
+			}
+		case strings.HasPrefix(f.Fingerprint, "mutation:unattested:enforce:stryker:"):
+			if f.Classification != "blocking" || f.Severity != "high" || !strings.Contains(f.Finding, "(missing)") {
+				t.Errorf("unattested stryker blocks under enforce: %+v", f)
+			}
+		}
+	}
+	if byPrefix["mutation:pending"] != 1 || byPrefix["mutation:unattested"] != 1 {
+		t.Errorf("one pending, one unattested (gremlins gets none): %v", byPrefix)
+	}
+	advisory := buildAt(t, root, Advisory, filepath.Join(dir, "incremental.json"))
+	if advisory.Findings[0].Classification != "advisory" || advisory.Findings[0].Severity != "medium" {
+		t.Errorf("unattested is advisory by default: %+v", advisory.Findings[0])
+	}
+}
+
+func TestBuildSection(t *testing.T) {
+	report, root := realCase(t, "full")
+	dir := t.TempDir()
+	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
+	gremlins := t.TempDir()
+	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
+	lockReport, _ := realCase(t, "lockfile")
+	write(t, root, "tests/c.test.ts", "// changed\n")
+	res := buildAt(t, root, Advisory, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"), lockReport)
+	want := "## Mutation Evidence Freshness\n\n" +
+		"- Mode: `advisory`\n" +
+		"- Reports: 4 (2 attested, 2 unattested: missing, missing)\n" +
+		"- Kills: 21 verified, 6 stale, 21 pending, 0 unbound, 2 unattested\n" +
+		"- Deferrals: global input changed: package-lock.json\n" +
+		"- Freshness not verifiable: gremlins\n\n" +
+		"### Re-run list\n\n" +
+		"| File | Cause | Stale kills |\n| --- | --- | ---: |\n" +
+		"| `src/c.ts` | `tests/c.test.ts` | 6 |"
+	if res.Section != want {
+		t.Errorf("section:\n%s\n--- want ---\n%s", res.Section, want)
+	}
+	if empty, _ := Build(nil, Worktree(root), Advisory); empty.Section != "" || len(empty.Findings) != 0 {
+		t.Errorf("no reports, no section: %+v", empty)
+	}
+}
+
+// Two causes: one finding each, ordered by cause; the re-run list is ordered by file, then cause.
+func TestBuildOrdersFindingsAndRows(t *testing.T) {
+	report, root := realCase(t, "full")
+	write(t, root, "tests/c.test.ts", "// changed\n")
+	write(t, root, "src/a.ts", "changed\n")
+	res := buildAt(t, root, Advisory, report)
+	if len(res.Findings) != 2 || res.Findings[0].Title != "Mutation evidence stale: src/a.ts changed" ||
+		res.Findings[1].Title != "Mutation evidence stale: tests/c.test.ts changed" {
+		t.Errorf("findings %+v", res.Findings)
+	}
+	if !strings.Contains(res.Section, "| `src/a.ts` | `src/a.ts` | 12 |\n| `src/b.ts` | `src/a.ts` | 5 |\n| `src/c.ts` | `tests/c.test.ts` | 3 |") {
+		t.Errorf("rows:\n%s", res.Section)
+	}
+}
+
+func TestBuildStopsOnContentErrors(t *testing.T) {
+	report, _ := realCase(t, "full")
+	r, _ := mutation.Load(report)
+	if _, err := Build([]mutation.Report{r}, Worktree(t.TempDir()), Advisory); err == nil {
+		t.Error("a content error stops the review")
+	}
+}
+
+func readFile(t *testing.T, path string) string {
+	t.Helper()
+	data, err := os.ReadFile(path)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return string(data)
+}
diff --git a/internal/mutationfresh/classify.go b/internal/mutationfresh/classify.go
new file mode 100644
index 0000000..166b657
--- /dev/null
+++ b/internal/mutationfresh/classify.go
@@ -0,0 +1,231 @@
+package mutationfresh
+
+import (
+	"sort"
+
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+// ReportFreshness is one report's classification (spec §6.3). It is serialized into pr-ready's
+// reviewer-input digest (§6.7), so it holds no machine-dependent path.
+type ReportFreshness struct {
+	Path              string       `json:"-"`
+	Engine            string       `json:"engine"`
+	Attested          bool         `json:"attested"`
+	UnattestedReason  string       `json:"unattestedReason,omitempty"`
+	ReportSha256      string       `json:"reportSha256"`
+	AttestationSha256 string       `json:"attestationSha256,omitempty"`
+	Verified          int          `json:"verified"`
+	Stale             int          `json:"stale"`
+	Pending           int          `json:"pending"`
+	Unbound           int          `json:"unbound"`
+	Unattested        int          `json:"unattested"`
+	Causes            []CauseCount `json:"staleCauses,omitempty"`
+	ReRun             []ReRunRow   `json:"reRun,omitempty"`
+	Deferrals         []string     `json:"deferrals,omitempty"`
+}
+
+// CauseCount is one recorded cause: the changed path, its current digest and the kills it stales.
+type CauseCount struct {
+	Cause  string `json:"cause"`
+	Digest string `json:"digest"`
+	Kills  int    `json:"kills"`
+}
+
+// ReRunRow is one (file, cause) row of the re-run list (§6.6).
+type ReRunRow struct {
+	File  string `json:"file"`
+	Cause string `json:"cause"`
+	Kills int    `json:"kills"`
+}
+
+type change struct{ category, digest string }
+
+// Classify reads the report's attestation and gives every kill exactly one class (spec §6.3):
+// stale (first cause wins), pending, unbound or verified. Kills in an unattested report are
+// unattested. Survivors and every other status are never reclassified.
+func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
+	out := ReportFreshness{Path: r.Target, Engine: r.Engine, ReportSha256: r.SHA256}
+	att, reason, attSHA := readAttestation(r.Target, r.Engine, r.SHA256)
+	out.AttestationSha256 = attSHA
+	if reason == "" && r.Detail == nil {
+		reason = "engine" // only the harness attests, and it attests Stryker reports
+	}
+	if reason != "" {
+		out.UnattestedReason = reason
+		out.Unattested = r.Score().Killed
+		return out, nil
+	}
+	out.Attested = true
+	for _, d := range att.Deferrals {
+		out.Deferrals = append(out.Deferrals, d.Reason)
+	}
+	changed, err := changedPaths(att, content)
+	if err != nil {
+		return ReportFreshness{}, err
+	}
+	causeOf := causeFinder(r.Detail, changed)
+	stale := map[string]int{}
+	rows := map[ReRunRow]int{}
+	for _, file := range sortedKeys(r.Detail.Files) {
+		for _, m := range r.Detail.Files[file].Mutants {
+			if m.Status != mutation.Killed {
+				continue
+			}
+			if cause := causeOf(file, m.KilledBy); cause != "" {
+				out.Stale++
+				stale[cause]++
+				rows[ReRunRow{File: file, Cause: cause}]++
+				continue
+			}
+			_, attested := att.Files[file]
+			switch {
+			case deferred(att.Deferrals, file):
+				out.Pending++
+			case !attested || len(m.KilledBy) == 0:
+				out.Unbound++
+			default:
+				out.Verified++
+			}
+		}
+	}
+	for _, cause := range sortedKeys(stale) {
+		out.Causes = append(out.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: stale[cause]})
+	}
+	for row, kills := range rows {
+		out.ReRun = append(out.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
+	}
+	sort.Slice(out.ReRun, func(i, j int) bool {
+		if out.ReRun[i].File != out.ReRun[j].File {
+			return out.ReRun[i].File < out.ReRun[j].File
+		}
+		return out.ReRun[i].Cause < out.ReRun[j].Cause
+	})
+	return out, nil
+}
+
+// changedPaths: attested paths whose current digest differs (in HEAD mode, an untracked attested
+// path absent from HEAD is skipped), plus present paths that are not attested, not excluded, not
+// ignored, and neither mutate nor test (which cannot invalidate a kill). An unknown category counts
+// as a blanket cause, as global does.
+func changedPaths(att Attestation, content Content) (map[string]change, error) {
+	present, err := content.Paths()
+	if err != nil {
+		return nil, err
+	}
+	var fresh []string
+	for _, p := range present {
+		if _, attested := att.Files[p]; attested || MatchList(p, att.Exclusions) {
+			continue
+		}
+		switch Categorize(p, att.Lists) {
+		case "ignore", "mutate", "test":
+			continue
+		}
+		fresh = append(fresh, p)
+	}
+	paths := append(sortedKeys(att.Files), fresh...)
+	entries, err := content.Read(paths)
+	if err != nil {
+		return nil, err
+	}
+	changed := map[string]change{}
+	for _, p := range paths {
+		digest := Absent
+		if e, ok := entries[p]; ok {
+			digest = digestOf(e)
+			if p == att.Config && !e.Symlink {
+				digest = configDigest(e.Data)
+			}
+		}
+		entry, attested := att.Files[p]
+		if !attested {
+			changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
+			continue
+		}
+		if digest == entry.Digest || (content.Head() && !entry.Tracked && digest == Absent) {
+			continue
+		}
+		changed[p] = change{category: entry.Category, digest: digest}
+	}
+	return changed, nil
+}
+
+// causeFinder returns a kill's recorded cause, in the §6.3 order: its own file; the byte-smallest
+// changed test whose ids it was killed by; the byte-smallest changed mutate file with mutants whose
+// coverage includes a killing test; else the byte-smallest changed support, global, unclassified or
+// zero-mutant mutate path (the gate has no import graph, so these invalidate every kill).
+func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file string, killedBy []string) string {
+	testOf := map[string]string{}
+	type covering struct {
+		path string
+		ids  map[string]bool
+	}
+	var mutated []covering
+	var blanket []string
+	for _, p := range sortedKeys(changed) {
+		switch c := changed[p]; {
+		case c.category == "test":
+			for _, id := range d.TestIDs[p] {
+				if _, seen := testOf[id]; !seen {
+					testOf[id] = p
+				}
+			}
+		case c.category == "mutate" && len(d.Files[p].Mutants) > 0:
+			ids := map[string]bool{}
+			for _, m := range d.Files[p].Mutants {
+				for _, id := range m.CoveredBy {
+					ids[id] = true
+				}
+			}
+			mutated = append(mutated, covering{path: p, ids: ids})
+		default:
+			blanket = append(blanket, p)
+		}
+	}
+	return func(file string, killedBy []string) string {
+		if _, ok := changed[file]; ok {
+			return file
+		}
+		best := ""
+		for _, id := range killedBy {
+			if p, ok := testOf[id]; ok && (best == "" || p < best) {
+				best = p
+			}
+		}
+		if best != "" {
+			return best
+		}
+		for _, y := range mutated {
+			for _, id := range killedBy {
+				if y.ids[id] {
+					return y.path
+				}
+			}
+		}
+		if len(blanket) > 0 {
+			return blanket[0]
+		}
+		return ""
+	}
+}
+
+func deferred(deferrals []Deferral, file string) bool {
+	for _, d := range deferrals {
+		for _, p := range d.Paths {
+			if p == "*" || p == file {
+				return true
+			}
+		}
+	}
+	return false
+}
+
+func sortedKeys[V any](m map[string]V) []string {
+	keys := make([]string, 0, len(m))
+	for k := range m {
+		keys = append(keys, k)
+	}
+	sort.Strings(keys)
+	return keys
+}
diff --git a/internal/mutationfresh/classify_test.go b/internal/mutationfresh/classify_test.go
new file mode 100644
index 0000000..a82cd61
--- /dev/null
+++ b/internal/mutationfresh/classify_test.go
@@ -0,0 +1,279 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+// realCase copies a committed real report (Plan 1c's e2e run) and the e2e fixture tree into a fresh
+// repository. It rebases every attested path outside src/ and tests/ onto that repository, so only
+// the fixture's sources and tests can change, and returns the report path and root.
+func realCase(t *testing.T, row string) (report, root string) {
+	t.Helper()
+	files := map[string]string{}
+	fixture := "../../testdata/mutation-incremental-e2e"
+	err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
+		if err != nil || d.IsDir() {
+			return err
+		}
+		rel, _ := filepath.Rel(fixture, path)
+		data, _ := os.ReadFile(path)
+		files[filepath.ToSlash(rel)] = string(data)
+		return nil
+	})
+	if err != nil {
+		t.Fatal(err)
+	}
+	root = gitRepo(t, files)
+	src := filepath.Join("../../testdata/mutation-incremental/real", row)
+	reportBytes, err := os.ReadFile(filepath.Join(src, "incremental.json"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var att map[string]any
+	attBytes, _ := os.ReadFile(filepath.Join(src, "attestation.json"))
+	if err := json.Unmarshal(attBytes, &att); err != nil {
+		t.Fatal(err)
+	}
+	current, _ := Worktree(root).Read(nil)
+	_ = current
+	entries := att["files"].(map[string]any)
+	for path, raw := range entries {
+		if hasPrefix(path, "src/") || hasPrefix(path, "tests/") {
+			continue
+		}
+		entry := raw.(map[string]any)
+		got, _ := Worktree(root).Read([]string{path})
+		if e, ok := got[path]; ok {
+			entry["digest"] = digestOf(e)
+		} else {
+			entry["digest"] = Absent
+		}
+	}
+	att["config"] = ""
+	dir := t.TempDir()
+	write(t, dir, "incremental.json", string(reportBytes))
+	out, _ := json.Marshal(att)
+	write(t, dir, "attestation.json", string(out))
+	return filepath.Join(dir, "incremental.json"), root
+}
+
+func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
+
+func classifyAt(t *testing.T, report, root string) ReportFreshness {
+	t.Helper()
+	r, err := mutation.Load(report)
+	if err != nil {
+		t.Fatal(err)
+	}
+	f, err := Classify(r, Worktree(root))
+	if err != nil {
+		t.Fatal(err)
+	}
+	return f
+}
+
+func causes(f ReportFreshness) map[string]int {
+	out := map[string]int{}
+	for _, c := range f.Causes {
+		out[c.Cause] = c.Kills
+	}
+	return out
+}
+
+// Spec §6.3 on real reports: 24 kills (a.ts 12, b.ts 5, c.ts 3, e.ts 4).
+func TestClassifyRealReportsUnchangedTreeIsVerified(t *testing.T) {
+	report, root := realCase(t, "full")
+	f := classifyAt(t, report, root)
+	if !f.Attested || f.Verified != 24 || f.Stale+f.Pending+f.Unbound+f.Unattested != 0 {
+		t.Errorf("unchanged tree: %+v", f)
+	}
+}
+
+func TestClassifyRealReportsEditedSourceStalesItsKillsAndTheirCoverageClosure(t *testing.T) {
+	report, root := realCase(t, "full")
+	write(t, root, "src/a.ts", "export function grade(score: number, pass: number, top: number): string {\n  return 'ok';\n}\n")
+	f := classifyAt(t, report, root)
+	// a.ts's own 12 kills, plus b.ts's 5: killed by b.test, whose ids cover a.ts's mutants.
+	if f.Stale != 17 || f.Verified != 7 || causes(f)["src/a.ts"] != 17 {
+		t.Errorf("edited a.ts: %+v", f)
+	}
+	if len(f.ReRun) != 2 || f.ReRun[0].File != "src/a.ts" || f.ReRun[1].File != "src/b.ts" || f.ReRun[1].Cause != "src/a.ts" {
+		t.Errorf("re-run rows %+v", f.ReRun)
+	}
+}
+
+func TestClassifyRealReportsEditedTestStalesWhatItKilled(t *testing.T) {
+	report, root := realCase(t, "full")
+	write(t, root, "tests/c.test.ts", "// rewritten\n")
+	f := classifyAt(t, report, root)
+	if f.Stale != 3 || causes(f)["tests/c.test.ts"] != 3 || f.Verified != 21 {
+		t.Errorf("edited c.test: %+v", f)
+	}
+}
+
+func TestClassifyRealReportsGlobalAndZeroMutantEditsStaleEverything(t *testing.T) {
+	for path, text := range map[string]string{"package-lock.json": "{}\n", "src/limits.ts": "export const LIMIT = 99;\n"} {
+		report, root := realCase(t, "full")
+		write(t, root, path, text)
+		f := classifyAt(t, report, root)
+		if f.Stale != 24 || causes(f)[path] != 24 {
+			t.Errorf("edited %s: %+v", path, f)
+		}
+	}
+}
+
+func TestClassifyRealReportsDeferralsArePending(t *testing.T) {
+	report, root := realCase(t, "lockfile") // "global input changed: package-lock.json" on ["*"]
+	f := classifyAt(t, report, root)
+	if f.Pending != 24 || f.Deferrals[0] != "global input changed: package-lock.json" {
+		t.Errorf("lockfile row: %+v", f)
+	}
+	report, root = realCase(t, "delete-c-test") // "no reachable tests: src/c.ts"
+	f = classifyAt(t, report, root)
+	if f.Pending != 3 || f.Verified != 21 {
+		t.Errorf("delete-c-test row: %+v", f)
+	}
+}
+
+// Rules the real reports do not reach: a changed test file ranks before a changed mutate file,
+// the byte-smallest cause wins a tie, an unattested file or a kill without killedBy is unbound, a
+// new support file present only in the tree is a blanket cause, a new mutate or test file is not,
+// and in HEAD mode an untracked attested path absent from HEAD is not a change.
+func TestClassifySyntheticRules(t *testing.T) {
+	root := gitRepo(t, map[string]string{"src/a.ts": "a", "src/b.ts": "b", "tests/a.test.ts": "t", "tests/b.test.ts": "u"})
+	dir := t.TempDir()
+	report := `{"files":{
+		"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1","t2"],"coveredBy":["t1","t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
+		"src/b.ts":{"source":"b","mutants":[{"id":"2","mutatorName":"M","status":"Killed","killedBy":["t2"],"coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}},
+		                                    {"id":"3","mutatorName":"M","status":"Killed","killedBy":[],"coveredBy":[],"location":{"start":{"line":1},"end":{"line":1}}},
+		                                    {"id":"4","mutatorName":"M","status":"Survived","coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
+		"gen/x.ts":{"source":"g","mutants":[{"id":"5","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},
+		"testFiles":{"tests/a.test.ts":{"tests":[{"id":"t1"}]},"tests/b.test.ts":{"tests":[{"id":"t2"}]}}}`
+	write(t, dir, "incremental.json", report)
+	files := map[string]any{}
+	digests, _ := Worktree(root).Read([]string{"src/a.ts", "src/b.ts", "tests/a.test.ts", "tests/b.test.ts"})
+	for path, category := range map[string]string{"src/a.ts": "mutate", "src/b.ts": "mutate", "tests/a.test.ts": "test", "tests/b.test.ts": "test"} {
+		files[path] = map[string]any{"digest": digestOf(digests[path]), "category": category, "tracked": true}
+	}
+	files["zz-gone.txt"] = map[string]any{"digest": "sha256:0", "category": "unclassified", "tracked": false}
+	att := validAttestation(sha256Hex([]byte(report)))
+	att["files"] = files
+	att["lists"] = map[string]any{"mutate": []string{"src/**"}, "test": []string{"tests/**"}, "support": []string{"helpers/**"}, "global": []string{}, "ignore": []string{"**/*.md"}}
+	att["exclusions"] = []string{".mutation/**"}
+	writeAttestation(t, dir, att)
+	path := filepath.Join(dir, "incremental.json")
+
+	// Worktree: zz-gone.txt is attested but missing, so it is a changed unclassified path, a blanket
+	// cause. Stale is checked before unbound, so the kill without killedBy is stale too: all 4.
+	f := classifyAt(t, path, root)
+	if f.Stale != 4 || causes(f)["zz-gone.txt"] != 4 {
+		t.Errorf("missing attested path: %+v", f)
+	}
+	// HEAD mode: an untracked attested path absent from HEAD is skipped; everything is unchanged.
+	r, _ := mutation.Load(path)
+	head, err := Classify(r, Head(root))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if head.Stale != 0 || head.Verified != 2 || head.Unbound != 2 {
+		t.Errorf("HEAD mode: %+v (a.ts, b.ts#2 verified; b.ts#3 has no killedBy and gen/x.ts is unattested: unbound)", head)
+	}
+	// Both tests changed: a.ts's kill takes the byte-smallest test; b.ts's kill takes b.test.
+	write(t, root, "tests/a.test.ts", "t2")
+	write(t, root, "tests/b.test.ts", "u2")
+	write(t, root, "src/new.ts", "n")        // a new mutate file cannot invalidate a kill
+	write(t, root, "tests/new.test.ts", "n") // nor can a new test
+	write(t, root, "notes.md", "n")          // ignored
+	r, _ = mutation.Load(path)
+	f, _ = Classify(r, Head(root))
+	if f.Stale != 0 {
+		t.Errorf("uncommitted edits are not seen in HEAD mode: %+v", f)
+	}
+	f = classifyAt(t, path, root)
+	if causes(f)["tests/a.test.ts"] != 2 || causes(f)["tests/b.test.ts"] != 1 {
+		t.Errorf("test causes: %+v", f.Causes)
+	}
+	// A changed mutate file (rule 3): b.ts changed only; a.ts's kill is killed by t2, which covers b.ts.
+	write(t, root, "tests/a.test.ts", "t")
+	write(t, root, "tests/b.test.ts", "u")
+	write(t, root, "src/b.ts", "b2")
+	write(t, root, "zz-gone.txt", "g") // present now, but not the attested digest: still a blanket cause
+	f = classifyAt(t, path, root)
+	if causes(f)["src/b.ts"] != 3 || causes(f)["zz-gone.txt"] != 1 {
+		// b.ts#2 and b.ts#3 by their own file; a.ts#1 because t2 covers b.ts (rule 3); gen/x.ts#5
+		// (killed by t1 only) falls through to the blanket cause zz-gone.txt.
+		t.Errorf("rule 3 and own file: %+v", f.Causes)
+	}
+	// A new support file present only in the tree is a blanket cause; it sorts before zz-gone.txt,
+	// so every kill with no other cause records it.
+	write(t, root, "src/b.ts", "b")
+	write(t, root, "helpers/h.ts", "h")
+	f = classifyAt(t, path, root)
+	if causes(f)["helpers/h.ts"] != 4 {
+		t.Errorf("a new support path is a blanket cause: %+v", f.Causes)
+	}
+}
+
+func TestClassifyUnattestedAndGremlins(t *testing.T) {
+	dir := t.TempDir()
+	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
+	r, _ := mutation.Load(filepath.Join(dir, "incremental.json"))
+	f, err := Classify(r, Worktree(t.TempDir()))
+	if err != nil || f.Attested || f.UnattestedReason != "missing" || f.Unattested != 1 {
+		t.Errorf("unattested stryker: %+v %v", f, err)
+	}
+	// A parse without Stryker detail (gremlins) is never attested, even beside a harness attestation.
+	g := mutation.Report{Engine: "stryker", Target: filepath.Join(dir, "incremental.json"), SHA256: r.SHA256,
+		Mutants: []mutation.Mutant{{Status: mutation.Killed}}}
+	writeAttestation(t, dir, validAttestation(r.SHA256))
+	f, _ = Classify(g, Worktree(t.TempDir()))
+	if f.UnattestedReason != "engine" {
+		t.Errorf("no detail: %+v", f)
+	}
+}
+
+func TestClassifyStopsOnContentErrors(t *testing.T) {
+	report, _ := realCase(t, "full")
+	r, _ := mutation.Load(report)
+	if _, err := Classify(r, Worktree(t.TempDir())); err == nil {
+		t.Error("listing paths outside a repository stops the review")
+	}
+	root := gitRepo(t, map[string]string{"src/a.ts": "a"})
+	if err := os.Chmod(filepath.Join(root, "src/a.ts"), 0o000); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := Classify(r, Worktree(root)); err == nil {
+		t.Error("an unreadable attested path stops the review")
+	}
+}
+
+// Review Focus 2 at the gate: the attested config is digested without its views, so a view-map
+// edit is not a change, and any other config edit is a global (blanket) cause.
+func TestClassifyConfigDigestIgnoresViews(t *testing.T) {
+	cfg := `{"schemaVersion":1,"views":{"inline":{"a":["src/**"]}}}`
+	root := gitRepo(t, map[string]string{"src/a.ts": "a", "mi.json": cfg})
+	dir := t.TempDir()
+	report := `{"files":{"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{}}`
+	write(t, dir, "incremental.json", report)
+	att := validAttestation(sha256Hex([]byte(report)))
+	att["config"] = "mi.json"
+	att["files"] = map[string]any{
+		"src/a.ts": map[string]any{"digest": digestOf(Entry{Data: []byte("a")}), "category": "mutate", "tracked": true},
+		"mi.json":  map[string]any{"digest": configDigest([]byte(cfg)), "category": "global", "tracked": true},
+	}
+	writeAttestation(t, dir, att)
+	path := filepath.Join(dir, "incremental.json")
+	write(t, root, "mi.json", `{"views":{"inline":{"b":["src/a.ts"]}},"schemaVersion":1}`)
+	if f := classifyAt(t, path, root); f.Verified != 1 || f.Stale != 0 {
+		t.Errorf("a view-map edit is not a change: %+v", f)
+	}
+	write(t, root, "mi.json", `{"schemaVersion":2}`)
+	if f := classifyAt(t, path, root); f.Stale != 1 || causes(f)["mi.json"] != 1 {
+		t.Errorf("any other config edit is a global cause: %+v", f)
+	}
+}
diff --git a/internal/mutationfresh/content.go b/internal/mutationfresh/content.go
new file mode 100644
index 0000000..5144ee9
--- /dev/null
+++ b/internal/mutationfresh/content.go
@@ -0,0 +1,218 @@
+package mutationfresh
+
+import (
+	"bytes"
+	"errors"
+	"fmt"
+	"io/fs"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"sort"
+	"strings"
+	"sync"
+	"syscall"
+)
+
+// Content is the code under review (spec §6.2): the working tree for task-done and epic-ready, and
+// HEAD for pr-ready unless --include-working-tree.
+type Content interface {
+	// Read returns the content of every path that exists as a regular file or symlink; any other
+	// path is omitted (its digest is Absent). A read error other than absence stops the review.
+	Read(paths []string) (map[string]Entry, error)
+	// Paths lists every present path, excluding gitlinks.
+	Paths() ([]string, error)
+	// Head reports HEAD mode, in which an attested untracked path absent from HEAD is not a change.
+	Head() bool
+}
+
+// runGit runs git in root. A variable so a test can make one subcommand fail.
+var runGit = func(root string, stdin []byte, args ...string) ([]byte, error) {
+	cmd := exec.Command("git", args...)
+	cmd.Dir = root
+	cmd.Stdin = bytes.NewReader(stdin)
+	var stderr bytes.Buffer
+	cmd.Stderr = &stderr
+	out, err := cmd.Output()
+	if err != nil {
+		return nil, fmt.Errorf("mutationfresh: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
+	}
+	return out, nil
+}
+
+func splitZ(out []byte) []string {
+	var parts []string
+	for _, p := range strings.Split(string(out), "\x00") {
+		if p != "" {
+			parts = append(parts, p)
+		}
+	}
+	return parts
+}
+
+type worktree struct{ root string }
+
+// Worktree reads the working tree under root.
+func Worktree(root string) Content { return worktree{root: root} }
+
+func (worktree) Head() bool { return false }
+
+// readEntry reads one path: a symlink's link text (never followed) or a regular file's bytes.
+// Missing paths, paths under a file, and anything else (a directory) are absent.
+func readEntry(full string) (Entry, bool, error) {
+	info, err := os.Lstat(full)
+	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
+		return Entry{}, false, nil
+	}
+	if err != nil {
+		return Entry{}, false, err
+	}
+	if info.Mode()&fs.ModeSymlink != 0 {
+		target, err := os.Readlink(full)
+		return Entry{Data: []byte(target), Symlink: true}, true, err
+	}
+	if !info.Mode().IsRegular() {
+		return Entry{}, false, nil
+	}
+	data, err := os.ReadFile(full)
+	return Entry{Data: data}, true, err
+}
+
+func (w worktree) Read(paths []string) (map[string]Entry, error) {
+	out := map[string]Entry{}
+	for _, p := range paths {
+		e, ok, err := readEntry(filepath.Join(w.root, filepath.FromSlash(p)))
+		if err != nil {
+			return nil, fmt.Errorf("mutationfresh: reading %s: %w", p, err)
+		}
+		if ok {
+			out[p] = e
+		}
+	}
+	return out, nil
+}
+
+// Paths is `git ls-files -c -o --exclude-standard --deduplicate` (spec §6.3). A gitlink or a nested
+// repository checks out as a directory, which is not content.
+func (w worktree) Paths() ([]string, error) {
+	listed, err := runGit(w.root, nil, "ls-files", "-c", "-o", "--exclude-standard", "--deduplicate", "-z")
+	if err != nil {
+		return nil, err
+	}
+	var out []string
+	for _, p := range splitZ(listed) {
+		if info, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p))); err == nil && info.IsDir() {
+			continue
+		}
+		out = append(out, strings.TrimSuffix(p, "/"))
+	}
+	return out, nil
+}
+
+type head struct {
+	root  string
+	modes map[string]string // path → git mode at HEAD
+	cache map[string]Entry  // content already read in this review
+}
+
+// Head reads the committed tree at HEAD through the repository's filters (spec §6.2).
+func Head(root string) Content { return &head{root: root} }
+
+func (*head) Head() bool { return true }
+
+func (h *head) load() error {
+	if h.modes != nil {
+		return nil
+	}
+	out, err := runGit(h.root, nil, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
+	if err != nil {
+		return err
+	}
+	h.modes = map[string]string{}
+	h.cache = map[string]Entry{}
+	for _, line := range splitZ(out) {
+		// "<mode> <type> <object>\t<path>"
+		h.modes[line[strings.IndexByte(line, '\t')+1:]] = line[:strings.IndexByte(line, ' ')]
+	}
+	return nil
+}
+
+// Paths is `git ls-tree -r HEAD` without gitlinks (mode 160000), sorted.
+func (h *head) Paths() ([]string, error) {
+	if err := h.load(); err != nil {
+		return nil, err
+	}
+	var out []string
+	for p, mode := range h.modes {
+		if mode != "160000" {
+			out = append(out, p)
+		}
+	}
+	sort.Strings(out)
+	return out, nil
+}
+
+// headReaders bounds the git processes that read HEAD content at once.
+const headReaders = 16
+
+// Read is `git cat-file --filters HEAD:<path>` per present path (spec §6.2). Batch mode cannot be used:
+// with --filters its header reports the unfiltered size while the content is filtered, so the output
+// cannot be split. Paths are read once per review (Build classifies every report against one Content)
+// and in parallel. A symlink's blob is its link text; gitlinks are absent.
+func (h *head) Read(paths []string) (map[string]Entry, error) {
+	if err := h.load(); err != nil {
+		return nil, err
+	}
+	unread := map[string]bool{}
+	for _, p := range paths {
+		_, cached := h.cache[p]
+		if mode, ok := h.modes[p]; ok && mode != "160000" && !cached {
+			unread[p] = true
+		}
+	}
+	if err := h.fill(unread); err != nil {
+		return nil, err
+	}
+	out := map[string]Entry{}
+	for _, p := range paths {
+		if e, ok := h.cache[p]; ok {
+			out[p] = e
+		}
+	}
+	return out, nil
+}
+
+// fill reads the given paths into the cache through at most headReaders git processes.
+func (h *head) fill(unread map[string]bool) error {
+	type result struct {
+		path string
+		data []byte
+		err  error
+	}
+	jobs := make(chan string)
+	results := make(chan result, len(unread))
+	var wg sync.WaitGroup
+	for w := 0; w < min(headReaders, len(unread)); w++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			for p := range jobs {
+				data, err := runGit(h.root, nil, "cat-file", "--filters", "HEAD:"+p)
+				results <- result{path: p, data: data, err: err}
+			}
+		}()
+	}
+	for p := range unread {
+		jobs <- p
+	}
+	close(jobs)
+	wg.Wait()
+	close(results)
+	for r := range results { // every reader has finished, so returning early leaks nothing
+		if r.err != nil {
+			return r.err
+		}
+		h.cache[r.path] = Entry{Data: r.data, Symlink: h.modes[r.path] == "120000"}
+	}
+	return nil
+}
diff --git a/internal/mutationfresh/content_test.go b/internal/mutationfresh/content_test.go
new file mode 100644
index 0000000..c7a1856
--- /dev/null
+++ b/internal/mutationfresh/content_test.go
@@ -0,0 +1,141 @@
+package mutationfresh
+
+import (
+	"errors"
+	"os"
+	"path/filepath"
+	"slices"
+	"testing"
+)
+
+func TestWorktreeReadsFilesSymlinksAndAbsence(t *testing.T) {
+	root := gitRepo(t, map[string]string{"a.txt": "a", "dir/b.txt": "b"})
+	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
+		t.Fatal(err)
+	}
+	w := Worktree(root)
+	if w.Head() {
+		t.Error("the working tree is not HEAD mode")
+	}
+	got, err := w.Read([]string{"a.txt", "link", "missing", "dir", "a.txt/under-a-file"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if string(got["a.txt"].Data) != "a" || got["a.txt"].Symlink {
+		t.Errorf("a.txt = %+v", got["a.txt"])
+	}
+	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
+		t.Errorf("link = %+v", got["link"])
+	}
+	for _, absent := range []string{"missing", "dir", "a.txt/under-a-file"} {
+		if _, ok := got[absent]; ok {
+			t.Errorf("%s must be absent (missing, a directory, or under a file)", absent)
+		}
+	}
+}
+
+func TestWorktreeReadErrorsStopTheReview(t *testing.T) {
+	root := gitRepo(t, map[string]string{"locked/a.txt": "a", "b.txt": "b"})
+	if err := os.Chmod(filepath.Join(root, "b.txt"), 0o000); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.Chmod(filepath.Join(root, "locked"), 0o000); err != nil {
+		t.Fatal(err)
+	}
+	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked"), 0o755) })
+	if _, err := Worktree(root).Read([]string{"b.txt"}); err == nil {
+		t.Error("an unreadable file stops the review")
+	}
+	if _, err := Worktree(root).Read([]string{"locked/a.txt"}); err == nil {
+		t.Error("an unreadable directory stops the review")
+	}
+}
+
+func TestWorktreePathsListsTrackedAndUntrackedButNotNestedRepositories(t *testing.T) {
+	root := gitRepo(t, map[string]string{"a.txt": "a", ".gitignore": "ignored.txt\n"})
+	write(t, root, "new.txt", "n")
+	write(t, root, "ignored.txt", "i")
+	nested := filepath.Join(root, "sub")
+	if err := os.MkdirAll(nested, 0o755); err != nil {
+		t.Fatal(err)
+	}
+	git(t, nested, "init", "-q")
+	got, err := Worktree(root).Paths()
+	if err != nil {
+		t.Fatal(err)
+	}
+	slices.Sort(got)
+	if !slices.Equal(got, []string{".gitignore", "a.txt", "new.txt"}) {
+		t.Errorf("paths = %v", got)
+	}
+	if _, err := Worktree(t.TempDir()).Paths(); err == nil {
+		t.Error("outside a repository is an error")
+	}
+}
+
+// §6.2 and §7.2: HEAD mode reads the committed content through the repository's filters, sees
+// symlinks as link text, excludes gitlinks, and ignores uncommitted edits.
+func TestHeadContentReadsCommittedContentThroughFilters(t *testing.T) {
+	root := gitRepo(t, map[string]string{".gitattributes": "*.txt eol=crlf\n", "a.txt": "one\ntwo\n", "b.bin": "b"})
+	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
+		t.Fatal(err)
+	}
+	sha := git(t, root, "rev-parse", "HEAD")
+	git(t, root, "update-index", "--add", "--cacheinfo", "160000,"+sha[:40]+",gitlink")
+	git(t, root, "add", "link")
+	git(t, root, "commit", "-qm", "link and gitlink")
+	write(t, root, "b.bin", "uncommitted")
+	h := Head(root)
+	if !h.Head() {
+		t.Error("HEAD mode")
+	}
+	got, err := h.Read([]string{"a.txt", "b.bin", "link", "gitlink", "missing"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if string(got["a.txt"].Data) != "one\r\ntwo\r\n" {
+		t.Errorf("a.txt through eol=crlf = %q", got["a.txt"].Data)
+	}
+	if string(got["b.bin"].Data) != "b" {
+		t.Errorf("b.bin must be the committed content, got %q", got["b.bin"].Data)
+	}
+	if string(got["link"].Data) != "a.txt" || !got["link"].Symlink {
+		t.Errorf("link = %+v", got["link"])
+	}
+	for _, absent := range []string{"gitlink", "missing"} {
+		if _, ok := got[absent]; ok {
+			t.Errorf("%s is absent in HEAD mode", absent)
+		}
+	}
+	paths, err := h.Paths()
+	if err != nil {
+		t.Fatal(err)
+	}
+	if !slices.Equal(paths, []string{".gitattributes", "a.txt", "b.bin", "link"}) {
+		t.Errorf("HEAD paths = %v (gitlinks excluded, sorted)", paths)
+	}
+	if none, err := h.Read([]string{"missing"}); err != nil || len(none) != 0 {
+		t.Errorf("reading only absent paths runs nothing: %v %v", none, err)
+	}
+}
+
+func TestHeadContentErrors(t *testing.T) {
+	if _, err := Head(t.TempDir()).Paths(); err == nil {
+		t.Error("outside a repository is an error")
+	}
+	if _, err := Head(t.TempDir()).Read([]string{"a"}); err == nil {
+		t.Error("outside a repository is an error")
+	}
+	root := gitRepo(t, map[string]string{"a.txt": "a"})
+	saved := runGit
+	t.Cleanup(func() { runGit = saved })
+	runGit = func(dir string, stdin []byte, args ...string) ([]byte, error) {
+		if args[0] == "cat-file" {
+			return nil, errors.New("cat-file failed")
+		}
+		return saved(dir, stdin, args...)
+	}
+	if _, err := Head(root).Read([]string{"a.txt"}); err == nil {
+		t.Error("a cat-file failure stops the review")
+	}
+}
diff --git a/internal/mutationfresh/digest.go b/internal/mutationfresh/digest.go
new file mode 100644
index 0000000..aacddfa
--- /dev/null
+++ b/internal/mutationfresh/digest.go
@@ -0,0 +1,48 @@
+package mutationfresh
+
+import (
+	"bytes"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+)
+
+// Absent is the digest of a path that is missing or is neither a regular file nor a symlink (§6.2).
+const Absent = "absent"
+
+// Entry is one path's content under review: file bytes, or a symlink's link text.
+type Entry struct {
+	Data    []byte
+	Symlink bool
+}
+
+func sha256Hex(data []byte) string {
+	sum := sha256.Sum256(data)
+	return hex.EncodeToString(sum[:])
+}
+
+// digestOf is the §5.4 digest: sha256:<hex> of the bytes, or symlink:<hex> of the link text.
+func digestOf(e Entry) string {
+	if e.Symlink {
+		return "symlink:" + sha256Hex(e.Data)
+	}
+	return "sha256:" + sha256Hex(e.Data)
+}
+
+// configDigest digests the harness config as the harness does (lib/snapshot.mjs configDigest):
+// canonical JSON — keys sorted, two-space indent, a trailing newline — without `views`, so editing
+// the view map is not a change. Text that is not a JSON object is digested raw (the harness would
+// have refused it). The shared vectors pin the two implementations together.
+func configDigest(data []byte) string {
+	var raw map[string]any
+	if err := json.Unmarshal(data, &raw); err != nil {
+		return digestOf(Entry{Data: data})
+	}
+	delete(raw, "views")
+	var buf bytes.Buffer
+	enc := json.NewEncoder(&buf)
+	enc.SetEscapeHTML(false)
+	enc.SetIndent("", "  ")
+	_ = enc.Encode(raw) // a value decoded from JSON always encodes
+	return "sha256:" + sha256Hex(buf.Bytes())
+}
diff --git a/internal/mutationfresh/digest_test.go b/internal/mutationfresh/digest_test.go
new file mode 100644
index 0000000..18ec9dc
--- /dev/null
+++ b/internal/mutationfresh/digest_test.go
@@ -0,0 +1,48 @@
+package mutationfresh
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"os"
+	"testing"
+)
+
+func TestDigestOf(t *testing.T) {
+	sum := sha256.Sum256([]byte("x"))
+	if got := digestOf(Entry{Data: []byte("x")}); got != "sha256:"+hex.EncodeToString(sum[:]) {
+		t.Errorf("file digest %q", got)
+	}
+	if got := digestOf(Entry{Data: []byte("x"), Symlink: true}); got != "symlink:"+hex.EncodeToString(sum[:]) {
+		t.Errorf("symlink digest %q", got)
+	}
+}
+
+// Spec K3.4 and Plan 1a: the harness digests its config file as canonical JSON without `views`.
+// The gate must produce the same digest byte for byte, or the config reads as changed forever.
+func TestConfigDigestVectors(t *testing.T) {
+	data, err := os.ReadFile("../../testdata/mutation-incremental/config-digest-vectors.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	var v struct {
+		Cases []struct{ Name, Text, Digest string } `json:"cases"`
+	}
+	if err := json.Unmarshal(data, &v); err != nil {
+		t.Fatal(err)
+	}
+	if len(v.Cases) < 5 {
+		t.Fatalf("expected the shared vectors, got %d cases", len(v.Cases))
+	}
+	for _, c := range v.Cases {
+		if got := configDigest([]byte(c.Text)); got != c.Digest {
+			t.Errorf("%s: got %s, want %s", c.Name, got, c.Digest)
+		}
+	}
+}
+
+func TestConfigDigestOfUnparseableTextIsTheRawDigest(t *testing.T) {
+	if got, want := configDigest([]byte("not json")), digestOf(Entry{Data: []byte("not json")}); got != want {
+		t.Errorf("got %s, want %s", got, want)
+	}
+}
diff --git a/internal/mutationfresh/glob.go b/internal/mutationfresh/glob.go
new file mode 100644
index 0000000..c674a31
--- /dev/null
+++ b/internal/mutationfresh/glob.go
@@ -0,0 +1,147 @@
+// Package mutationfresh judges whether the kills in a mutation report still describe the code under
+// review (spec §6 of docs/superpowers/specs/2026-09-23-mutation-incremental-design.md). The harness
+// (templates/mutation-incremental/) attests what it verified; this package re-derives, from content
+// digests only, which of those kills a later edit has made stale.
+package mutationfresh
+
+import "strings"
+
+// The glob dialect is shared with the harness (spec §5.3; templates/mutation-incremental/lib/glob.mjs).
+// Both implementations run testdata/mutation-incremental/glob-vectors.json. Paths are repo-relative
+// POSIX paths, case-sensitive. `*` matches a run of characters except `/`; `?` matches one; `**` as a
+// whole segment matches zero or more segments; `{a,b}` is alternation (no nesting). `*`, `?` and `**`
+// never match a segment that starts with `.` unless the pattern segment does (as in Stryker). RE2 has
+// no lookahead, so this is a small matcher rather than a translated regular expression.
+
+type globToken struct {
+	kind  byte // 'l' literal, '*', '?', '{'
+	lit   byte
+	start bool // at the start of a path segment: the dot rule applies
+	alts  [][]globToken
+}
+
+func compileSegment(seg string, atStart bool) []globToken {
+	var out []globToken
+	for i := 0; i < len(seg); i++ {
+		c := seg[i]
+		start := atStart && i == 0
+		switch {
+		case c == '*':
+			out = append(out, globToken{kind: '*', start: start})
+			for i+1 < len(seg) && seg[i+1] == '*' {
+				i++
+			}
+		case c == '?':
+			out = append(out, globToken{kind: '?', start: start})
+		case c == '{' && strings.IndexByte(seg[i:], '}') > 0:
+			end := i + strings.IndexByte(seg[i:], '}')
+			var alts [][]globToken
+			for _, alt := range strings.Split(seg[i+1:end], ",") {
+				alts = append(alts, compileSegment(alt, start))
+			}
+			out = append(out, globToken{kind: '{', alts: alts})
+			i = end
+		default:
+			out = append(out, globToken{kind: 'l', lit: c})
+		}
+	}
+	return out
+}
+
+func matchTokens(tokens []globToken, s string) bool {
+	if len(tokens) == 0 {
+		return s == ""
+	}
+	t, rest := tokens[0], tokens[1:]
+	switch t.kind {
+	case '*':
+		if t.start && strings.HasPrefix(s, ".") {
+			return false
+		}
+		for k := 0; k <= len(s); k++ {
+			if matchTokens(rest, s[k:]) {
+				return true
+			}
+		}
+		return false
+	case '?':
+		if s == "" || (t.start && s[0] == '.') {
+			return false
+		}
+		return matchTokens(rest, s[1:])
+	case '{':
+		for _, alt := range t.alts {
+			if matchTokens(append(append([]globToken(nil), alt...), rest...), s) {
+				return true
+			}
+		}
+		return false
+	default:
+		return s != "" && s[0] == t.lit && matchTokens(rest, s[1:])
+	}
+}
+
+func matchSegments(pattern, path []string) bool {
+	if len(pattern) == 0 {
+		return len(path) == 0
+	}
+	if pattern[0] == "**" {
+		for k := 0; k <= len(path); k++ {
+			if matchSegments(pattern[1:], path[k:]) {
+				return true
+			}
+			if k < len(path) && strings.HasPrefix(path[k], ".") {
+				return false
+			}
+		}
+		return false
+	}
+	return len(path) > 0 && matchTokens(compileSegment(pattern[0], true), path[0]) && matchSegments(pattern[1:], path[1:])
+}
+
+// MatchGlob reports whether path matches one pattern. A leading "./" in the pattern is removed.
+func MatchGlob(pattern, path string) bool {
+	return matchSegments(strings.Split(strings.TrimPrefix(pattern, "./"), "/"), strings.Split(path, "/"))
+}
+
+// MatchList: some non-`!` entry matches and no `!` entry matches.
+func MatchList(path string, list []string) bool {
+	hit := false
+	for _, entry := range list {
+		if strings.HasPrefix(entry, "!") {
+			if MatchGlob(entry[1:], path) {
+				return false
+			}
+		} else if !hit {
+			hit = MatchGlob(entry, path)
+		}
+	}
+	return hit
+}
+
+// Lists are the attested category lists (the implicit global entries included).
+type Lists struct {
+	Mutate  []string `json:"mutate"`
+	Test    []string `json:"test"`
+	Support []string `json:"support"`
+	Global  []string `json:"global"`
+	Ignore  []string `json:"ignore"`
+}
+
+// Categorize returns the first list that matches in the order global, support, test, mutate; else
+// "ignore" for an ignored path, else "unclassified".
+func Categorize(path string, lists Lists) string {
+	ordered := []struct {
+		name string
+		list []string
+	}{{"global", lists.Global}, {"support", lists.Support}, {"test", lists.Test}, {"mutate", lists.Mutate}}
+	for _, l := range ordered {
+		if MatchList(path, l.list) {
+			return l.name
+		}
+	}
+	if MatchList(path, lists.Ignore) {
+		return "ignore"
+	}
+	return "unclassified"
+}
diff --git a/internal/mutationfresh/glob_test.go b/internal/mutationfresh/glob_test.go
new file mode 100644
index 0000000..8aa6a1a
--- /dev/null
+++ b/internal/mutationfresh/glob_test.go
@@ -0,0 +1,75 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"os"
+	"testing"
+)
+
+// The glob dialect is shared with the harness (spec §5.3). Both implementations run the same vectors,
+// so a change to either must change both.
+func TestGlobVectors(t *testing.T) {
+	data, err := os.ReadFile("../../testdata/mutation-incremental/glob-vectors.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	var v struct {
+		Match []struct {
+			Pattern, Path string
+			Match         bool
+		} `json:"match"`
+		List []struct {
+			List  []string `json:"list"`
+			Path  string   `json:"path"`
+			Match bool     `json:"match"`
+		} `json:"list"`
+		Categorize struct {
+			Lists Lists                             `json:"lists"`
+			Cases []struct{ Path, Category string } `json:"cases"`
+		} `json:"categorize"`
+	}
+	if err := json.Unmarshal(data, &v); err != nil {
+		t.Fatal(err)
+	}
+	for _, c := range v.Match {
+		if got := MatchGlob(c.Pattern, c.Path); got != c.Match {
+			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.Pattern, c.Path, got, c.Match)
+		}
+	}
+	for _, c := range v.List {
+		if got := MatchList(c.Path, c.List); got != c.Match {
+			t.Errorf("MatchList(%q, %v) = %v, want %v", c.Path, c.List, got, c.Match)
+		}
+	}
+	for _, c := range v.Categorize.Cases {
+		if got := Categorize(c.Path, v.Categorize.Lists); got != c.Category {
+			t.Errorf("Categorize(%q) = %q, want %q", c.Path, got, c.Category)
+		}
+	}
+}
+
+func TestGlobDotRuleAndAlternation(t *testing.T) {
+	cases := []struct {
+		pattern, path string
+		want          bool
+	}{
+		{"?.ts", ".ts", false},       // ? at the start of a segment does not match a dot
+		{"a?.ts", "a..ts", true},     // ? elsewhere does
+		{"?", "", false},             // ? needs a character
+		{"x*", "x.ts", true},         // * after a literal matches a dot
+		{"{a,b}.ts", "b.ts", true},   // alternation
+		{"{a,*}.ts", ".x.ts", false}, // an alternative at the start keeps the dot rule
+		{"x{a,b", "x{a,b", true},     // an unclosed brace is literal
+		{"src/**", "src/.git/x", false},
+		{"src/.git/*", "src/.git/x", true},
+		{"a**b", "axxb", true}, // consecutive stars inside a segment are one star
+	}
+	for _, c := range cases {
+		if got := MatchGlob(c.pattern, c.path); got != c.want {
+			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
+		}
+	}
+	if !MatchList("src/a.ts", []string{"lib/**", "src/**"}) || MatchList("src/a.d.ts", []string{"src/**", "!src/*.d.ts"}) {
+		t.Error("list rule: some positive entry matches and no negated entry matches")
+	}
+}
diff --git a/internal/mutationfresh/headcache_test.go b/internal/mutationfresh/headcache_test.go
new file mode 100644
index 0000000..7bf6b1d
--- /dev/null
+++ b/internal/mutationfresh/headcache_test.go
@@ -0,0 +1,47 @@
+package mutationfresh
+
+import (
+	"fmt"
+	"sync/atomic"
+	"testing"
+)
+
+// Final review: HEAD content is read once per path per review (Build classifies several reports
+// against one Content), and uncached paths are read in parallel, so a repository with thousands of
+// attested files does not pay one sequential git process per file per report.
+func TestHeadReadCachesAcrossReportsAndReadsInParallel(t *testing.T) {
+	files := map[string]string{}
+	var paths []string
+	for i := 0; i < 40; i++ {
+		p := fmt.Sprintf("f%02d.txt", i)
+		files[p] = fmt.Sprintf("content %d", i)
+		paths = append(paths, p)
+	}
+	root := gitRepo(t, files)
+	var calls atomic.Int64
+	saved := runGit
+	t.Cleanup(func() { runGit = saved })
+	runGit = func(dir string, stdin []byte, args ...string) ([]byte, error) {
+		if args[0] == "cat-file" {
+			calls.Add(1)
+		}
+		return saved(dir, stdin, args...)
+	}
+	h := Head(root)
+	got, err := h.Read(append(paths, paths[0])) // a duplicate is read once
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, p := range paths {
+		if string(got[p].Data) != files[p] {
+			t.Fatalf("%s = %q, want %q", p, got[p].Data, files[p])
+		}
+	}
+	if calls.Load() != 40 {
+		t.Errorf("one read per path, got %d", calls.Load())
+	}
+	again, err := h.Read(paths[:5])
+	if err != nil || len(again) != 5 || calls.Load() != 40 {
+		t.Errorf("a second report re-reads nothing: %d entries, %d calls, %v", len(again), calls.Load(), err)
+	}
+}
diff --git a/internal/mutationfresh/helpers_test.go b/internal/mutationfresh/helpers_test.go
new file mode 100644
index 0000000..f6441f1
--- /dev/null
+++ b/internal/mutationfresh/helpers_test.go
@@ -0,0 +1,45 @@
+package mutationfresh
+
+import (
+	"os"
+	"os/exec"
+	"path/filepath"
+	"testing"
+)
+
+// gitRepo creates a repository with the given files committed, and returns its root.
+func gitRepo(t *testing.T, files map[string]string) string {
+	t.Helper()
+	root := t.TempDir()
+	for path, text := range files {
+		write(t, root, path, text)
+	}
+	git(t, root, "init", "-q")
+	git(t, root, "config", "user.email", "t@example.com")
+	git(t, root, "config", "user.name", "T")
+	git(t, root, "add", "-A")
+	git(t, root, "commit", "-qm", "init")
+	return root
+}
+
+func write(t *testing.T, root, path, text string) {
+	t.Helper()
+	full := filepath.Join(root, filepath.FromSlash(path))
+	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func git(t *testing.T, root string, args ...string) string {
+	t.Helper()
+	cmd := exec.Command("git", args...)
+	cmd.Dir = root
+	out, err := cmd.CombinedOutput()
+	if err != nil {
+		t.Fatalf("git %v: %v: %s", args, err, out)
+	}
+	return string(out)
+}
diff --git a/internal/mutationfresh/mode.go b/internal/mutationfresh/mode.go
new file mode 100644
index 0000000..fb1ceb0
--- /dev/null
+++ b/internal/mutationfresh/mode.go
@@ -0,0 +1,31 @@
+package mutationfresh
+
+import "fmt"
+
+// Modes (spec §6.4).
+const (
+	Advisory = "advisory"
+	Enforce  = "enforce"
+	ModeEnv  = "METAREVIEW_MUTATION_FRESHNESS"
+)
+
+// ModeFromEnv reads METAREVIEW_MUTATION_FRESHNESS: unset, empty or advisory is advisory; enforce is
+// enforce; anything else is a usage error.
+func ModeFromEnv(getenv func(string) string) (string, error) {
+	switch value := getenv(ModeEnv); value {
+	case "", Advisory:
+		return Advisory, nil
+	case Enforce:
+		return Enforce, nil
+	default:
+		return "", fmt.Errorf("%s must be %q or %q, got %q", ModeEnv, Advisory, Enforce, value)
+	}
+}
+
+// EffectiveMode: task-done is always advisory; pr-ready and epic-ready use the configured mode.
+func EffectiveMode(scope, mode string) string {
+	if scope == "task-done" {
+		return Advisory
+	}
+	return mode
+}
diff --git a/internal/prready/freshness_test.go b/internal/prready/freshness_test.go
new file mode 100644
index 0000000..9957081
--- /dev/null
+++ b/internal/prready/freshness_test.go
@@ -0,0 +1,118 @@
+package prready
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"fmt"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/findings"
+	"github.com/dsifry/metareview/internal/reviewers"
+)
+
+// freshnessReport writes, outside the repository, an attested Stryker report whose one kill is in
+// file, attesting file's committed content (spec §6.1).
+func freshnessReport(t *testing.T, root, file string) string {
+	t.Helper()
+	data, err := os.ReadFile(filepath.Join(root, file))
+	if err != nil {
+		t.Fatal(err)
+	}
+	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
+	report := fmt.Sprintf(`{"files":{%q:{"source":"x","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{"t.test.ts":{"tests":[{"id":"t1"}]}}}`, file)
+	att := fmt.Sprintf(`{"schemaVersion":1,"tool":"metareview-mutation-incremental","engine":"stryker","report":"incremental.json","reportSha256":%q,
+		"lists":{"mutate":[],"test":[],"support":[],"global":[],"ignore":["**",".metareview/**","docs/**"]},"exclusions":[],
+		"files":{%q:{"digest":"sha256:%s","category":"mutate","tracked":true}},"deferrals":[]}`, sum([]byte(report)), file, sum(data))
+	dir := t.TempDir()
+	if err := os.WriteFile(filepath.Join(dir, "incremental.json"), []byte(report), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(dir, "attestation.json"), []byte(att), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	return filepath.Join(dir, "incremental.json")
+}
+
+func commitAll(t *testing.T, root, message string) {
+	t.Helper()
+	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", message}} {
+		cmd := exec.Command("git", args...)
+		cmd.Dir = root
+		if out, err := cmd.CombinedOutput(); err != nil {
+			t.Fatalf("git %v: %v %s", args, err, out)
+		}
+	}
+}
+
+func staleRows(t *testing.T, root string) []string {
+	t.Helper()
+	records, err := findings.All(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	var out []string
+	for _, r := range records {
+		if strings.HasPrefix(r.Fingerprint, "mutation:stale:") {
+			out = append(out, r.Status)
+		}
+	}
+	return out
+}
+
+// Spec §6.2–§6.5 at pr-ready: HEAD is under review, enforce blocks on stale evidence, and fresh
+// evidence supersedes the stale row.
+func TestCreateJudgesMutationFreshnessAgainstHEAD(t *testing.T) {
+	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
+	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
+	root := smallPRReadyRepo(t)
+	report := freshnessReport(t, root, "seed.txt")
+	// Uncommitted: pr-ready reads HEAD, so nothing is stale yet.
+	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("edited\n"), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	res, err := Create(root, Options{Base: "main", MutationReportPaths: []string{report}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	log, _ := os.ReadFile(filepath.Join(root, res.ReviewRel))
+	if !strings.Contains(string(log), "- Kills: 1 verified, 0 stale") || !strings.Contains(string(log), "- Mode: `enforce`") {
+		t.Fatalf("an uncommitted edit is not under review in HEAD mode:\n%s", log)
+	}
+	// Committed: the kill is stale, and under enforce it blocks.
+	commitAll(t, root, "edit")
+	res, err = Create(root, Options{Base: "main", MutationReportPaths: []string{report}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	log, _ = os.ReadFile(filepath.Join(root, res.ReviewRel))
+	if res.Verdict != "NEEDS_REVISION" || !strings.Contains(string(log), "Mutation evidence stale: seed.txt changed") {
+		t.Fatalf("committed edit under enforce: verdict %s\n%s", res.Verdict, log)
+	}
+	if got := staleRows(t, root); len(got) != 1 || got[0] != "open" {
+		t.Fatalf("stale rows %v", got)
+	}
+	// Fresh evidence for the committed content supersedes the stale row.
+	fresh := freshnessReport(t, root, "seed.txt")
+	if _, err := Create(root, Options{Base: "main", MutationReportPaths: []string{fresh}}); err != nil {
+		t.Fatal(err)
+	}
+	if got := staleRows(t, root); len(got) != 1 || got[0] != findings.StatusSuperseded {
+		t.Errorf("fresh evidence must supersede the stale row: %v", got)
+	}
+}
+
+// Spec §6.7: a run without reports keeps its reviewer-input digest — the new fields are omitempty.
+func TestNoReportsKeepTheReviewerInputDigestShape(t *testing.T) {
+	encoded, err := json.Marshal(reviewers.MutationContext{})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if string(encoded) != `{"Reports":null}` {
+		t.Errorf("an empty mutation context must serialize as before, got %s", encoded)
+	}
+}
diff --git a/internal/prready/mutationctx_test.go b/internal/prready/mutationctx_test.go
index 25f332d..4b292d6 100644
--- a/internal/prready/mutationctx_test.go
+++ b/internal/prready/mutationctx_test.go
@@ -12,7 +12,7 @@ import (
 // A skipped report is indistinguishable from a package with no survivors, so the failure mode of
 // getting this wrong is a gate that passes because it looked at less.
 func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
-	if _, err := mutationContextFor(nil); err != nil {
+	if _, err := mutationContextFor(t.TempDir(), nil, false); err != nil {
 		t.Errorf("no reports at all is the ordinary case: %v", err)
 	}
 	for name, path := range map[string]string{
@@ -21,7 +21,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 		"empty file":    writeTemp(t, ""),
 		"files is text": writeTemp(t, `{"files":"none"}`),
 	} {
-		ctx, err := mutationContextFor([]string{path})
+		ctx, err := mutationContextFor(t.TempDir(), []string{path}, false)
 		if err == nil {
 			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
 			continue
@@ -35,7 +35,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 // A readable report reaches the reviewers, so the review actually acts on it.
 func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
 	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
-	ctx, err := mutationContextFor([]string{path})
+	ctx, err := mutationContextFor(t.TempDir(), []string{path}, false)
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/internal/prready/review.go b/internal/prready/review.go
index 334aafe..6338310 100644
--- a/internal/prready/review.go
+++ b/internal/prready/review.go
@@ -21,7 +21,6 @@ import (
 	"github.com/dsifry/metareview/internal/githubcontext"
 	"github.com/dsifry/metareview/internal/knowledge"
 	"github.com/dsifry/metareview/internal/markdown"
-	"github.com/dsifry/metareview/internal/mutation"
 	"github.com/dsifry/metareview/internal/repo"
 	"github.com/dsifry/metareview/internal/reviewers"
 	"github.com/dsifry/metareview/internal/reviewlog"
@@ -3



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

No external validation evidence supplied.
