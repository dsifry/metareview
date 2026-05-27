#!/usr/bin/env bash
set -euo pipefail

for file in \
  skills/setup/SKILL.md \
  skills/review-artifact/SKILL.md \
  skills/review-task-done/SKILL.md \
  skills/review-epic-ready/SKILL.md \
  skills/review-pr-ready/SKILL.md \
  skills/learn-post-merge/SKILL.md \
  skills/status/SKILL.md
do
  test -f "$file"
  grep -q '^---$' "$file"
  grep -q '^name:' "$file"
  grep -q '^description:' "$file"
done

for file in README.md docs/quickstart.md commands/setup.md commands/review-artifact.md commands/review-task-done.md commands/review-epic-ready.md commands/review-pr-ready.md commands/learn-post-merge.md commands/status.md rubrics/task-done-review-rubric.md rubrics/epic-ready-review-rubric.md rubrics/pr-ready-review-rubric.md rubrics/learning-review-rubric.md templates/SERVICE-INVENTORY.md
do
  test -f "$file"
done

grep -q 'metareview review artifact <path>' docs/quickstart.md
grep -q 'metareview review task-done <task-id-or-path>' docs/quickstart.md
grep -q 'metareview review epic-ready <epic-id-or-path>' docs/quickstart.md
grep -q 'metareview review pr-ready' docs/quickstart.md
grep -q 'metareview learn --post-merge <pr-number> --base <pre-merge-ref>' docs/quickstart.md
grep -q '.metareview/findings.jsonl' docs/quickstart.md
grep -q '.metareview/knowledge/metareview.jsonl' docs/quickstart.md
grep -q 'docs/metareview/reviews/' docs/quickstart.md
grep -q 'metaswarm remains the lifecycle owner' docs/quickstart.md
grep -q 'docs/quickstart.md' README.md
grep -q '^## What Is This?' README.md
grep -q '^## Install$' README.md
grep -q 'codex plugin marketplace add dsifry/metareview-marketplace' README.md
grep -q 'claude plugin marketplace add dsifry/metareview-marketplace' README.md
grep -q '^## Works even better with metaswarm!$' README.md
grep -q 'https://github.com/dsifry/metaswarm' README.md
grep -q 'multi-agent orchestration framework' README.md
grep -q '^## How The Workflow Works$' README.md
grep -q '^```mermaid$' README.md
grep -q 'Fractal decomposition review' README.md
grep -q 'Child unit decomposition' README.md
grep -q 'Parent intent preserved?' README.md
grep -q '^## How Humans Use It$' README.md
grep -q '^## How Coding Agents Use It$' README.md
grep -q '^## Philosophy$' README.md
grep -q 'metareview review task-done' skills/review-task-done/SKILL.md
grep -q 'metareview review task-done' commands/review-task-done.md
grep -q 'metareview review epic-ready' skills/review-epic-ready/SKILL.md
grep -q 'metareview review epic-ready' commands/review-epic-ready.md
grep -q 'metareview review pr-ready' skills/review-pr-ready/SKILL.md
grep -q 'metareview review pr-ready' commands/review-pr-ready.md
grep -q 'metareview learn --post-merge' skills/learn-post-merge/SKILL.md
grep -q 'metareview learn --post-merge' commands/learn-post-merge.md
grep -q 'metareview learn --post-merge <pr-number> --base <pre-merge-ref>' skills/learn-post-merge/SKILL.md
grep -q 'strict mode' commands/learn-post-merge.md
grep -q 'setup --bootstrap-prereqs --dry-run' commands/setup.md
grep -q 'setup --bootstrap-prereqs --dry-run' skills/setup/SKILL.md
grep -q 'commit' commands/status.md
grep -q 'commit' skills/status/SKILL.md
grep -q 'Critical, high, and spec-contract findings block' rubrics/task-done-review-rubric.md
grep -q 'Critical and high findings block epic readiness' rubrics/epic-ready-review-rubric.md
grep -q 'Critical and high findings block PR readiness' rubrics/pr-ready-review-rubric.md
grep -q 'changes future reviewer behavior' rubrics/learning-review-rubric.md
