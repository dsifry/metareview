#!/bin/sh
# Local real-Stryker proof of the mutation-incremental harness (spec §7.1). Needs network; not in CI.
exec node "$(dirname "$0")/e2e-mutation-incremental.mjs" "$@"
