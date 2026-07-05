#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bash tests/manifest/test-manifests.sh
bash tests/manifest/test-skills.sh

if [ -f tests/go/test-cli-baseline.sh ]; then bash tests/go/test-cli-baseline.sh; fi
if [ -f tests/go/test-npm-wrapper-cwd.sh ]; then bash tests/go/test-npm-wrapper-cwd.sh; fi
if [ -f tests/go/test-setup-check.sh ]; then bash tests/go/test-setup-check.sh; fi
if [ -f tests/go/test-evidence.sh ]; then bash tests/go/test-evidence.sh; fi
if [ -f tests/go/test-artifact-review.sh ]; then bash tests/go/test-artifact-review.sh; fi
if [ -f tests/go/test-git-context.sh ]; then bash tests/go/test-git-context.sh; fi
if [ -f tests/go/test-task-source.sh ]; then bash tests/go/test-task-source.sh; fi
if [ -f tests/go/test-knowledge-context.sh ]; then bash tests/go/test-knowledge-context.sh; fi
if [ -f tests/go/test-findings.sh ]; then bash tests/go/test-findings.sh; fi
if [ -f tests/go/test-taskdone-reviewers.sh ]; then bash tests/go/test-taskdone-reviewers.sh; fi
if [ -f tests/go/test-task-done-review.sh ]; then bash tests/go/test-task-done-review.sh; fi
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
