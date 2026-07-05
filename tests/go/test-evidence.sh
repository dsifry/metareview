#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./internal/evidence

receipt="$(go run ./cmd/metareview evidence run -- go test ./internal/evidence -run TestParseJSONLReceipts -count=1)"
RECEIPT="$receipt" node - <<'NODE'
const receipt = JSON.parse(process.env.RECEIPT);
if (receipt.schemaVersion !== 1) throw new Error("schema version mismatch");
if (receipt.kind !== "validation") throw new Error("kind mismatch");
if (receipt.exitCode !== 0) throw new Error("exit code mismatch");
if (!receipt.summary.includes("go test ./internal/evidence")) throw new Error("summary missing command");
if (!receipt.stdoutSha256 || !receipt.stderrSha256) throw new Error("missing output hashes");
NODE

set +e
missing_output="$(go run ./cmd/metareview evidence run -- __metareview_missing_command__ 2>/tmp/metareview-evidence-missing.err)"
missing_status=$?
set -e
if [ "$missing_status" -eq 0 ]; then
  echo "missing command evidence run should exit nonzero"
  exit 1
fi
RECEIPT="$missing_output" node - <<'NODE'
const receipt = JSON.parse(process.env.RECEIPT);
if (receipt.schemaVersion !== 1) throw new Error("schema version mismatch");
if (receipt.kind !== "validation") throw new Error("kind mismatch");
if (receipt.exitCode === 0) throw new Error("missing command should have nonzero exit code");
if (!receipt.summary.includes("__metareview_missing_command__")) throw new Error("summary missing command");
NODE

fakebin="$(mktemp -d)"
trap 'rm -rf "$fakebin"' EXIT
cat > "$fakebin/gh" <<'SH'
#!/usr/bin/env bash
printf '[{"name":"lint","bucket":"fail","state":"FAILURE"}]\n'
exit 1
SH
chmod +x "$fakebin/gh"

set +e
import_output="$(PATH="$fakebin:$PATH" go run ./cmd/metareview evidence import --github-checks 4 2>/tmp/metareview-evidence-import.err)"
import_status=$?
set -e
if [ "$import_status" -eq 0 ]; then
  echo "failed CI import should exit nonzero"
  exit 1
fi
RECEIPT="$import_output" node - <<'NODE'
const lines = process.env.RECEIPT.trim().split(/\n+/);
if (lines.length !== 1) throw new Error("expected one imported receipt");
const receipt = JSON.parse(lines[0]);
if (receipt.kind !== "ci-check") throw new Error("kind mismatch");
if (receipt.exitCode === 0) throw new Error("failed check should have nonzero exit code");
NODE
