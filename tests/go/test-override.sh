#!/usr/bin/env bash
# Process overrides: an agent may record an out-of-workflow escalation, only an
# authority outside the workflow may acknowledge it, and CI stays red until then.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)

REPO="$TMP/repo"
mkdir -p "$REPO/.metareview"
cd "$REPO"
git init -q -b main
git config user.email tester@example.com
git config user.name "Test User"

cat > .metareview/findings.jsonl <<'EOF'
{"schemaVersion":1,"id":"mrvf-1","runId":"mrv-1","scope":"pr-ready","reviewer":"architecture-reviewer","severity":"high","classification":"blocking","status":"open","title":"Review context risk","finding":"","expected":"","found":"","evidence":[],"recommendation":"","owner":"","knowledgeCandidate":false,"beadsFollowupId":null,"fingerprint":"pr:architecture:context-risk","target":{"type":"branch","id":"feature"},"createdAt":"2026-08-27T00:00:00Z","updatedAt":"2026-08-27T00:00:00Z"}
EOF

fail() { echo "FAIL: $1" >&2; exit 1; }

# A reason that explains nothing is refused, and the record is untouched.
if "$TMP/metareview" override request mrvf-1 --by orchestrator --reason "nope" >/dev/null 2>&1; then
  fail "a trivial reason must be refused"
fi
grep -q '"status":"open"' .metareview/findings.jsonl || fail "a refused override changed the record"

# The orchestrator may request; the finding still blocks.
"$TMP/metareview" override request mrvf-1 --by orchestrator \
  --reason "chain exhausted overnight; three lens passes were sufficient evidence" \
  --escalation "artifact review chain exhausted (attempt 3 of 3)" >/dev/null
grep -q '"status":"override-pending"' .metareview/findings.jsonl || fail "request did not mark the finding pending"
grep -q '"overrideEscalation"' .metareview/findings.jsonl || fail "the escalation context was not recorded"

# CI hook: pending overrides keep the pipeline red.
if "$TMP/metareview" override list --pending >/dev/null 2>&1; then
  fail "override list --pending must exit nonzero while an override is unacknowledged"
fi

# Acknowledgement from outside the workflow clears it, with attribution.
"$TMP/metareview" override grant mrvf-1 --reason "reviewed the evidence and accept the exception" >/dev/null
grep -q '"status":"overridden"' .metareview/findings.jsonl || fail "grant did not mark the finding overridden"
grep -q '"overrideGrantedBy":"tester@example.com"' .metareview/findings.jsonl || fail "the granting actor was not recorded"
grep -q '"fixedInRunId"' .metareview/findings.jsonl && fail "an override must never read as a fix"

"$TMP/metareview" override list --pending >/dev/null || fail "a granted override must not stay pending"

# The exception is visible, with both halves of its provenance.
grep -q "## Process Overrides" docs/metareview/FINDINGS.md || fail "FINDINGS.md does not surface overrides"
grep -q "granted by tester@example.com" docs/metareview/FINDINGS.md || fail "FINDINGS.md omits the granting actor"
grep -q "requested by orchestrator" docs/metareview/FINDINGS.md || fail "FINDINGS.md omits the requesting actor"

# An already-overridden finding cannot be re-overridden.
if "$TMP/metareview" override request mrvf-1 --by orchestrator --reason "trying to override this a second time" >/dev/null 2>&1; then
  fail "an overridden finding must not accept a new request"
fi

echo "test-override: ok"
