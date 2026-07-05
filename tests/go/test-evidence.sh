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
