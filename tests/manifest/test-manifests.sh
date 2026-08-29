#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="$(node -p "require('$ROOT/package.json').version")"

node -e '
const fs = require("fs");
const { execSync } = require("child_process");
const files = [
  "package.json",
  ".codex-plugin/plugin.json",
  ".claude-plugin/plugin.json",
  ".claude-plugin/marketplace.json",
  ".agents/plugins/marketplace.json"
];
for (const file of files) {
  JSON.parse(fs.readFileSync(file, "utf8"));
}
const pkg = JSON.parse(fs.readFileSync("package.json", "utf8"));
const codex = JSON.parse(fs.readFileSync(".codex-plugin/plugin.json", "utf8"));
const claude = JSON.parse(fs.readFileSync(".claude-plugin/plugin.json", "utf8"));
const claudeMarketplace = JSON.parse(fs.readFileSync(".claude-plugin/marketplace.json", "utf8"));
if (pkg.name !== "metareview") throw new Error("package name mismatch");
if (codex.name !== "metareview") throw new Error("codex plugin name mismatch");
if (claude.name !== "metareview") throw new Error("claude plugin name mismatch");
if (pkg.version !== codex.version || pkg.version !== claude.version) {
  throw new Error("version mismatch");
}
if (claudeMarketplace.name !== "metareview") throw new Error("claude marketplace name mismatch");
if (claudeMarketplace.plugins[0].name !== "metareview") throw new Error("claude marketplace plugin name mismatch");
if (claudeMarketplace.plugins[0].version !== pkg.version) throw new Error("claude marketplace version mismatch");
if (codex.skills !== "./skills/") throw new Error("codex skills path mismatch");
if (!JSON.stringify(codex).includes("task-done")) throw new Error("codex plugin does not advertise task-done review");
if (!JSON.stringify(claude).includes("task-done")) throw new Error("claude plugin does not advertise task-done review");
if (!JSON.stringify(codex).includes("epic-ready")) throw new Error("codex plugin does not advertise epic-ready review");
if (!JSON.stringify(codex).includes("pr-ready")) throw new Error("codex plugin does not advertise pr-ready review");
if (!JSON.stringify(codex).includes("post-merge-learning")) throw new Error("codex plugin does not advertise post-merge learning");
if (!JSON.stringify(claude).includes("epic-ready")) throw new Error("claude plugin does not advertise epic-ready review");
if (!JSON.stringify(claude).includes("pr-ready")) throw new Error("claude plugin does not advertise pr-ready review");
if (!JSON.stringify(claude).includes("post-merge-learning")) throw new Error("claude plugin does not advertise post-merge learning");
if (!JSON.stringify(codex).includes("workflow")) throw new Error("codex plugin does not advertise workflow runs");
if (!JSON.stringify(claude).includes("workflow")) throw new Error("claude plugin does not advertise workflow runs");
if (!JSON.stringify(JSON.parse(fs.readFileSync(".agents/plugins/marketplace.json", "utf8"))).includes("task-done")) {
  throw new Error("marketplace does not advertise task-done review");
}
if (!JSON.stringify(JSON.parse(fs.readFileSync(".agents/plugins/marketplace.json", "utf8"))).includes("epic-ready")) {
  throw new Error("marketplace does not advertise epic-ready review");
}
if (!JSON.stringify(JSON.parse(fs.readFileSync(".agents/plugins/marketplace.json", "utf8"))).includes("pr-ready")) {
  throw new Error("marketplace does not advertise pr-ready review");
}
if (!JSON.stringify(JSON.parse(fs.readFileSync(".agents/plugins/marketplace.json", "utf8"))).includes("post-merge learning")) {
  throw new Error("marketplace does not advertise post-merge learning");
}
if (pkg.files.includes("lib/")) throw new Error("package still advertises lib/ as shipped runtime");
if (fs.existsSync("lib")) throw new Error("legacy JS implementation directory must not exist");
if (fs.existsSync("tests/lib")) throw new Error("legacy JS implementation tests must not exist");
for (const required of ["bin/", "cmd/", "internal/", "go.mod"]) {
  if (!pkg.files.includes(required)) throw new Error(`package files missing ${required}`);
}
for (const required of ["docs/quickstart.md", "docs/README.codex.md", "docs/README.claude.md", "docs/index.html", "docs/integrations/", "INSTALL.md", "AGENTS.md", "CLAUDE.md", "LICENSE"]) {
  if (!pkg.files.includes(required)) throw new Error(`package files missing ${required}`);
}
if (!fs.readFileSync("LICENSE", "utf8").startsWith("MIT License")) throw new Error("LICENSE must contain MIT text");
if (!JSON.stringify(pkg).includes("post-merge-learning")) throw new Error("package metadata does not advertise post-merge learning");
// The Go floor is stated in many files and go.mod is the one that enforces it.
// Derived, never enumerated: the first version of this check listed exactly the
// five artifacts whose drift had just been repaired, so it passed by construction
// while .agents/plugins/marketplace.json and cli/metareview.js — both already
// reported — still advertised the old floor. A hardcoded list can only ever check the
// copies someone remembered; every tracked file that states a floor is checked.
const goFloor = (fs.readFileSync("go.mod", "utf8").match(/^go (\d+\.\d+)(\.\d+)?$/m) || [])[1];
if (!goFloor) throw new Error("go.mod does not declare a go directive");
const tracked = execSync("git ls-files -z", { maxBuffer: 64 * 1024 * 1024 })
  .toString("utf8").split("\0").filter(Boolean);
const floorPattern = /Go (\d+\.\d+)\+/g;
let statedAnywhere = 0;
for (const file of tracked) {
  if (file.startsWith("docs/metareview/")) continue; // generated review artifacts
  let text;
  try {
    text = fs.readFileSync(file, "utf8");
  } catch {
    continue; // binary or unreadable: carries no prose floor
  }
  for (const m of text.matchAll(floorPattern)) {
    statedAnywhere++;
    if (m[1] !== goFloor) {
      throw new Error(`${file} advertises Go ${m[1]}+ but go.mod requires ${goFloor}`);
    }
  }
}
if (statedAnywhere === 0) throw new Error("no tracked file states a Go floor; the check would pass vacuously");
if (pkg.scripts.build !== "go build -o bin/metareview ./cmd/metareview") throw new Error("package build script must create bin/metareview");
if (pkg.scripts.prepack !== "npm run build") throw new Error("package prepack must build the packaged binary");
'

for doc in \
  "$ROOT/INSTALL.md" \
  "$ROOT/AGENTS.md" \
  "$ROOT/CLAUDE.md" \
  "$ROOT/docs/README.codex.md" \
  "$ROOT/docs/README.claude.md" \
  "$ROOT/docs/index.html" \
  "$ROOT/docs/quickstart.md"
do
  test -f "$doc"
done

for term in task-done epic-ready pr-ready post-merge PASS_ADVISORY "blocking finding"; do
  grep -R -Fq "$term" "$ROOT/INSTALL.md" "$ROOT/AGENTS.md" "$ROOT/CLAUDE.md" "$ROOT/docs/README.codex.md" "$ROOT/docs/README.claude.md" "$ROOT/docs/index.html" "$ROOT/docs/quickstart.md"
done

npm run build >/tmp/metareview-build-test.out
test -x "$ROOT/bin/metareview"
"$ROOT/bin/metareview" --version | grep -q "^${VERSION}$"

pack_json="$(cd "$ROOT" && npm pack --dry-run --json)"
PACK_JSON="$pack_json" node - <<'NODE'
const payload = JSON.parse(process.env.PACK_JSON);
const files = payload[0].files.map(file => file.path);
if (!files.includes("bin/metareview")) {
  throw new Error("npm pack output does not include bin/metareview");
}
NODE

"$ROOT/cli/metareview.js" --version | grep -q "^${VERSION}$"
"$ROOT/cli/metareview.js" --help | grep -q 'metareview review task-done'
"$ROOT/cli/metareview.js" --help | grep -q 'metareview review epic-ready'
"$ROOT/cli/metareview.js" --help | grep -q 'metareview review pr-ready'
"$ROOT/cli/metareview.js" --help | grep -q 'metareview learn --post-merge'
grep -q 'packagedBinary' "$ROOT/cli/metareview.js"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT
mkdir -p "$TMPDIR/cli" "$TMPDIR/bin"
cp "$ROOT/cli/metareview.js" "$TMPDIR/cli/metareview.js"
cat > "$TMPDIR/bin/metareview" <<'SH'
#!/usr/bin/env bash
printf 'PACKAGED:%s\n' "$*"
SH
chmod +x "$TMPDIR/bin/metareview"
packaged_output="$(cd "$TMPDIR" && node cli/metareview.js --version)"
test "$packaged_output" = "PACKAGED:--version"

rm -rf "${TMPDIR:?}/bin"
mkdir -p "$TMPDIR/cli"
cp "$ROOT/cli/metareview.js" "$TMPDIR/cli/metareview.js"
output="$(cd "$TMPDIR" && node cli/metareview.js --version 2>&1)" && status=0 || status=$?
if [ "$status" -eq 0 ]; then
  echo "Expected copied launcher without Go source to fail" >&2
  exit 1
fi
printf '%s\n' "$output" | grep -Fq "No packaged metareview binary or Go source checkout found"
