#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

repo="$TMP/repo"
mkdir -p "$repo/lib" "$repo/tests"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
printf "'use strict';\nmodule.exports = () => 'old';\n" > lib/example.js
printf "'use strict';\nmodule.exports = () => 'tracked';\n" > lib/unstaged.js
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/example.js
git add lib/example.js
git commit -qm "change example"
printf "'use strict';\nmodule.exports = () => 'staged';\n" > lib/staged.js
git add lib/staged.js
printf "'use strict';\nmodule.exports = () => 'unstaged';\n" > lib/unstaged.js
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/untracked.js

"$TMP/metareview" context diff --base "$base" > "$TMP/context.json"
node - <<'NODE' "$TMP/context.json" "$base"
const assert = require('assert');
const fs = require('fs');
const context = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const base = process.argv[3];
assert.strictEqual(context.baseSha, base, 'base sha mismatch');
assert(context.headSha.match(/^[0-9a-f]{40}$/), 'head sha should be full sha');
assert(context.changedFiles.includes('lib/example.js'), 'committed file missing');
assert(context.stagedFiles.includes('lib/staged.js'), 'staged file missing');
assert(context.unstagedFiles.includes('lib/unstaged.js'), 'unstaged file missing');
assert(context.workingTreeFiles.includes('lib/unstaged.js'), 'working tree file missing');
assert(context.untrackedFiles.includes('lib/untracked.js'), 'untracked file missing');
assert(context.diff.includes('eval(input)'), 'committed diff missing');
assert(context.stagedDiff.includes('staged'), 'staged diff missing');
assert(context.workingTreeDiff.includes('unstaged'), 'unstaged diff missing');
assert(context.untrackedExcerpts.includes('+module.exports = input => eval(input);'), 'untracked excerpt should be added-line style');
assert.strictEqual(context.stagedDiffTruncated, false, 'staged truncation flag mismatch');
assert(context.rawDiffBytes > 0, 'raw diff bytes should be recorded');
assert.strictEqual(context.filteredDiffBytes, context.rawDiffBytes, 'unfiltered context should have equal raw and filtered bytes');
assert.deepStrictEqual(context.generatedExcludedFiles, [], 'no generated files should be excluded without pathspec filters');
assert.strictEqual(context.untrackedOmittedCount, 0, 'untracked omission count mismatch');
NODE

set +e
"$TMP/metareview" context diff --base ../outside > "$TMP/bad.out" 2>"$TMP/bad.err"
code=$?
set -e
test "$code" -eq 1
grep -q 'Invalid git base' "$TMP/bad.err"
