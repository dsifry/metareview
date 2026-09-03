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
# Since 0.10.0 metareview DOES ship the git-native review gate (setup --install-hooks); the doc must reflect
# that (the old "hook installation is out of scope" claim was stale). Metaswarm still owns the lifecycle.
grep -q 'git-native review gate' docs/integrations/metaswarm.md
grep -q 'Metaswarm remains the lifecycle owner' docs/integrations/metaswarm.md
