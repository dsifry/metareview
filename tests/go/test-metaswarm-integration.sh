#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go test ./internal/integration
test -f docs/integrations/metaswarm.md
test -f docs/integrations/metaswarm.integration.json
grep -q 'metareview learn --post-merge <pr-number> --base <pre-merge-ref>' docs/integrations/metaswarm.md
grep -q '"postMergeLearning"' docs/integrations/metaswarm.integration.json
grep -q '"strictByDefault": false' docs/integrations/metaswarm.integration.json
grep -q 'Automatic hook installation is out of scope' docs/integrations/metaswarm.md
