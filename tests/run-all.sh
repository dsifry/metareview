#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bash tests/manifest/test-manifests.sh
bash tests/manifest/test-skills.sh

# spec 5 §7 smoke gate: the real-provider judge test must vet and be listable behind its build tag
go vet -tags smoke ./internal/fsm/judge/
go test -tags smoke -list 'TestSmoke' ./internal/fsm/judge/ | grep TestSmoke >/dev/null

if [ -f tests/go/test-cli-baseline.sh ]; then bash tests/go/test-cli-baseline.sh; fi
if [ -f tests/go/test-npm-wrapper-cwd.sh ]; then bash tests/go/test-npm-wrapper-cwd.sh; fi
if [ -f tests/go/test-setup-check.sh ]; then bash tests/go/test-setup-check.sh; fi
if [ -f tests/go/test-evidence.sh ]; then bash tests/go/test-evidence.sh; fi
if [ -f tests/go/test-artifact-review.sh ]; then bash tests/go/test-artifact-review.sh; fi
if [ -f tests/go/test-git-context.sh ]; then bash tests/go/test-git-context.sh; fi
if [ -f tests/go/test-shardpack-coverage.sh ]; then bash tests/go/test-shardpack-coverage.sh; fi
if [ -f tests/go/test-sharded-review.sh ]; then bash tests/go/test-sharded-review.sh; fi
if [ -f tests/go/test-task-source.sh ]; then bash tests/go/test-task-source.sh; fi
if [ -f tests/go/test-knowledge-context.sh ]; then bash tests/go/test-knowledge-context.sh; fi
if [ -f tests/go/test-findings.sh ]; then bash tests/go/test-findings.sh; fi
if [ -f tests/go/test-override.sh ]; then bash tests/go/test-override.sh; fi
if [ -f tests/go/test-override-coverage.sh ]; then bash tests/go/test-override-coverage.sh; fi
if [ -f tests/go/test-taskdone-reviewers.sh ]; then bash tests/go/test-taskdone-reviewers.sh; fi
if [ -f tests/go/test-task-done-review.sh ]; then bash tests/go/test-task-done-review.sh; fi
if [ -f tests/go/test-mutation-report.sh ]; then bash tests/go/test-mutation-report.sh; fi
if [ -f tests/go/test-reviewlog.sh ]; then bash tests/go/test-reviewlog.sh; fi
if [ -f tests/go/test-epic-source.sh ]; then bash tests/go/test-epic-source.sh; fi
if [ -f tests/go/test-epicready-reviewers.sh ]; then bash tests/go/test-epicready-reviewers.sh; fi
if [ -f tests/go/test-epic-ready-review.sh ]; then bash tests/go/test-epic-ready-review.sh; fi
if [ -f tests/go/test-github-context.sh ]; then bash tests/go/test-github-context.sh; fi
if [ -f tests/go/test-metaswarm-integration.sh ]; then bash tests/go/test-metaswarm-integration.sh; fi
if [ -f tests/go/test-pr-evidence.sh ]; then bash tests/go/test-pr-evidence.sh; fi
if [ -f tests/go/test-prready-reviewers.sh ]; then bash tests/go/test-prready-reviewers.sh; fi
if [ -f tests/go/test-pr-ready-review.sh ]; then bash tests/go/test-pr-ready-review.sh; fi
if [ -f tests/go/test-learn-source.sh ]; then bash tests/go/test-learn-source.sh; fi
if [ -f tests/go/test-session-history.sh ]; then bash tests/go/test-session-history.sh; fi
if [ -f tests/go/test-learning-candidates.sh ]; then bash tests/go/test-learning-candidates.sh; fi
if [ -f tests/go/test-learning-git-policy.sh ]; then bash tests/go/test-learning-git-policy.sh; fi
if [ -f tests/go/test-learning-prune.sh ]; then bash tests/go/test-learning-prune.sh; fi
if [ -f tests/go/test-learning-render.sh ]; then bash tests/go/test-learning-render.sh; fi
if [ -f tests/go/test-learning-writers.sh ]; then bash tests/go/test-learning-writers.sh; fi
if [ -f tests/go/test-learn-post-merge.sh ]; then bash tests/go/test-learn-post-merge.sh; fi
if [ -f tests/go/test-fsm.sh ]; then bash tests/go/test-fsm.sh; fi
# The Stop hook's ONLY test. run-all.sh is a hand-maintained list, so a new suite that nobody adds
# here never runs in CI — and this is the suite for the shim the whole enforcement layer rests on.
# It passed by hand and would have merged untested, which is the same silent failure the hook
# exists to prevent, one level up.
# Unguarded, deliberately. Every other suite here is guarded with `[ -f ]`, and that guard is
# what let this one be absent from the list without anything noticing. A required suite that
# silently skips when missing reproduces the exact failure it was added to close.
bash tests/go/test-stop-hook.sh

# CI runs shellcheck over every *.sh (.github/workflows/test.yml) but run-all.sh did not, so a
# shell defect could pass the whole local suite and fail CI - which is exactly what happened on
# 2026-08-29 with SC2043. Run it here when it is available so local and CI agree; skip with a
# notice when it is not, rather than making the suite unrunnable without it.
if command -v shellcheck >/dev/null 2>&1; then
  find . -name '*.sh' -not -path './node_modules/*' -print0 | xargs -0 shellcheck -x
  echo "shellcheck: ok"
else
  echo "shellcheck: SKIPPED (not installed; CI still enforces it)" >&2
fi

# The same gap again, and worth naming as a class rather than a second instance: CI runs checks
# this suite does not, so a defect passes everything locally and fails on the PR. shellcheck was
# added above on 2026-08-29 after SC2043 did exactly that; on 2026-08-30 golangci-lint did it
# again (SA4004, a loop left unconditionally terminated by removing a dead branch). Every check in
# .github/workflows/test.yml belongs here too.
if command -v golangci-lint >/dev/null 2>&1; then
  # GOFLAGS is unset deliberately: coverage.sh runs this suite with `-cover -covermode=atomic` to
  # instrument the binaries the scripts build, and golangci-lint inherits it, fails to typecheck
  # the instrumented tree, and reports a phantom issue. The linter must see the code as written.
  GOFLAGS="" golangci-lint run
  echo "golangci-lint: ok"
else
  echo "golangci-lint: SKIPPED (not installed; CI still enforces it)" >&2
fi
