#!/usr/bin/env bash
# The typed lens-output conformance corpus, run explicitly (0.12). The corpus cases run in
# the ordinary `go test ./...` sweep too; this script exists so a conformance failure is
# legible as a CONTRACT failure — a named case pinning the typed schema, the anchor-in-diff
# gate, or the judge output-cap retry ladder — instead of one line lost among unit tests.
# The lab-side provenance and the benchmark evidence live in dsifry/metareview#159.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# Every corpus case, verbose, named: a failure prints the case that regressed.
go test ./internal/lensoutput/ -run 'TestConformanceCorpus' -count=1 -v
# Every bucket has at least one pinned case (a bucket with no case can silently change).
go test ./internal/lensoutput/ -run 'TestConformanceCorpusExhaustiveBuckets' -count=1
# The judge cap-retry ladder corpus (output-cap 400 -> one 4x retry, then terminal).
go test ./internal/fsm/judge/ -run 'TestCapRetryLadder|TestIsOutputCapBody|TestRaiseOutputCap|TestCapRetryComposesWithTransportRetry' -count=1 -v

echo "test-lens-conformance.sh: OK"
