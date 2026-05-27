#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

node -e '
const fs = require("fs");
const files = [
  "package.json",
  ".codex-plugin/plugin.json",
  ".claude-plugin/plugin.json",
  ".agents/plugins/marketplace.json"
];
for (const file of files) {
  JSON.parse(fs.readFileSync(file, "utf8"));
}
const pkg = JSON.parse(fs.readFileSync("package.json", "utf8"));
const codex = JSON.parse(fs.readFileSync(".codex-plugin/plugin.json", "utf8"));
const claude = JSON.parse(fs.readFileSync(".claude-plugin/plugin.json", "utf8"));
if (pkg.name !== "metareview") throw new Error("package name mismatch");
if (codex.name !== "metareview") throw new Error("codex plugin name mismatch");
if (claude.name !== "metareview") throw new Error("claude plugin name mismatch");
if (pkg.version !== codex.version || pkg.version !== claude.version) {
  throw new Error("version mismatch");
}
if (codex.skills !== "./skills/") throw new Error("codex skills path mismatch");
if (!JSON.stringify(codex).includes("task-done")) throw new Error("codex plugin does not advertise task-done review");
if (!JSON.stringify(claude).includes("task-done")) throw new Error("claude plugin does not advertise task-done review");
if (!JSON.stringify(codex).includes("epic-ready")) throw new Error("codex plugin does not advertise epic-ready review");
if (!JSON.stringify(codex).includes("pr-ready")) throw new Error("codex plugin does not advertise pr-ready review");
if (!JSON.stringify(codex).includes("post-merge-learning")) throw new Error("codex plugin does not advertise post-merge learning");
if (!JSON.stringify(claude).includes("epic-ready")) throw new Error("claude plugin does not advertise epic-ready review");
if (!JSON.stringify(claude).includes("pr-ready")) throw new Error("claude plugin does not advertise pr-ready review");
if (!JSON.stringify(claude).includes("post-merge-learning")) throw new Error("claude plugin does not advertise post-merge learning");
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
for (const required of ["bin/", "cmd/", "internal/", "go.mod"]) {
  if (!pkg.files.includes(required)) throw new Error(`package files missing ${required}`);
}
for (const required of ["docs/quickstart.md", "docs/README.codex.md", "docs/README.claude.md", "docs/index.html", "docs/integrations/", "INSTALL.md", "AGENTS.md", "CLAUDE.md", "LICENSE"]) {
  if (!pkg.files.includes(required)) throw new Error(`package files missing ${required}`);
}
if (!fs.readFileSync("LICENSE", "utf8").startsWith("MIT License")) throw new Error("LICENSE must contain MIT text");
if (!JSON.stringify(pkg).includes("post-merge-learning")) throw new Error("package metadata does not advertise post-merge learning");
if (!JSON.stringify(pkg).includes("Go 1.22")) throw new Error("package metadata does not describe Go runtime expectation");
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
"$ROOT/bin/metareview" --version | grep -q '^0\.1\.0$'

pack_json="$(cd "$ROOT" && npm pack --dry-run --json)"
PACK_JSON="$pack_json" node - <<'NODE'
const payload = JSON.parse(process.env.PACK_JSON);
const files = payload[0].files.map(file => file.path);
if (!files.includes("bin/metareview")) {
  throw new Error("npm pack output does not include bin/metareview");
}
NODE

"$ROOT/cli/metareview.js" --version | grep -q '^0\.1\.0$'
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

rm -rf "$TMPDIR/bin"
mkdir -p "$TMPDIR/cli"
cp "$ROOT/cli/metareview.js" "$TMPDIR/cli/metareview.js"
output="$(cd "$TMPDIR" && node cli/metareview.js --version 2>&1)" && status=0 || status=$?
if [ "$status" -eq 0 ]; then
  echo "Expected copied launcher without Go source to fail" >&2
  exit 1
fi
printf '%s\n' "$output" | grep -Fq "No packaged metareview binary or Go source checkout found"
