#!/usr/bin/env bash
set -euo pipefail

for file in \
  skills/setup/SKILL.md \
  skills/review-artifact/SKILL.md \
  skills/review-task-done/SKILL.md \
  skills/review-epic-ready/SKILL.md \
  skills/review-pr-ready/SKILL.md \
  skills/learn-post-merge/SKILL.md \
  skills/status/SKILL.md \
  skills/fsm/SKILL.md
do
  test -f "$file"
  grep -q '^---$' "$file"
  grep -q '^name:' "$file"
  grep -q '^description:' "$file"
done

for file in README.md docs/quickstart.md commands/setup.md commands/review-artifact.md commands/review-task-done.md commands/review-epic-ready.md commands/review-pr-ready.md commands/learn-post-merge.md commands/status.md commands/fsm.md docs/fsm/driving-a-workflow.md docs/fsm/sdlc-loop-example.md workflows/sdlc-loop.yaml workflows/review-loop.yaml testdata/fsm/agent-prompt.golden tests/go/agent-prompt-anchors.txt rubrics/task-done-review-rubric.md rubrics/epic-ready-review-rubric.md rubrics/pr-ready-review-rubric.md rubrics/learning-review-rubric.md templates/SERVICE-INVENTORY.md
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
grep -q 'Do not ignore `docs/metareview/`' docs/quickstart.md
grep -q 'exact file entries' docs/quickstart.md
grep -q 'metaswarm remains the lifecycle owner' docs/quickstart.md
grep -q 'docs/quickstart.md' README.md
grep -q 'Do not ignore `docs/metareview/`' README.md
grep -q 'whole `.metareview/` directory' README.md
grep -q 'Do not ignore `docs/metareview/`' INSTALL.md
grep -q 'whole `.metareview/` directory' INSTALL.md
grep -q '^## Use Cases$' README.md
grep -q 'Spec review' README.md
grep -q 'Plan review' README.md
grep -q 'Architecture review' README.md
grep -q 'Feasibility review' README.md
grep -q 'Decomposition review' README.md
grep -q 'Fractal child-plan review' README.md
grep -q 'Code review' README.md
grep -q 'Test and acceptance review' README.md
grep -q 'PR readiness review' README.md
grep -q 'Intent-drift review' README.md
grep -q 'Post-merge learning' README.md
grep -q 'Repository knowledge review' README.md
grep -q '^## What Is This?' README.md
grep -q 'initial repository analysis' README.md
grep -q 'docs/SERVICE_INVENTORY.md' README.md
grep -q 'CodeRabbit' README.md
grep -q 'Greptile' README.md
grep -q 'nonproprietary' README.md
grep -q 'user-readable' README.md
grep -q 'Markdown/JSONL-friendly' README.md
grep -q 'pruning stale' README.md
grep -q '^## Agentic Review Patterns$' README.md
grep -q 'Adversarial multi-agent reviews' README.md
grep -q 'Iterations with hard gates' README.md
grep -q 'Fractal review loops' README.md
grep -q 'Cross-level intent checks' README.md
grep -q 'Evidence-backed reviews' README.md
grep -q 'Deterministic local reviewers' README.md
grep -q 'Specialist optional reviewers' README.md
grep -q 'Repository-knowledge priming' README.md
grep -q 'Review artifact accountability' README.md
grep -q 'Post-merge reflection' README.md
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
grep -q -- '--scaffold-only' skills/review-artifact/SKILL.md
grep -q 'parallel subagents by default' skills/review-artifact/SKILL.md
grep -q 'explicit authorization' skills/review-artifact/SKILL.md
grep -q 'not independently adversarial' skills/review-artifact/SKILL.md
# All NINE lens names, not a shorter prefix: a prefix matched whether the document listed five
# or the full set, so the assertion could not fail and the docs once drifted to "five" while
# reviewlog.artifactReviewComplete required more. Eight passing artifact reviews were left
# permanently unresolvable as a result. Mechanical-precision (0.9.0) is the ninth.
# and against the ENUMERATION LINE, not the page. Every lens name also appears in prose on the
# same page ("The Security lens uses rubrics/security-review-rubric.md"), so a whole-file grep
# still passes when a name is deleted from the list agents are actually told to run: removing
# ", Security," from the enumeration left this script at exit 0.
# The enumeration and that prose share one markdown line, so anchoring to the line is not enough
# either: the list itself has to be cut out of it, from "by default:" to the sentence's period.
lens_line="$(grep -m1 'Run the required lenses' skills/review-artifact/SKILL.md || true)"
test -n "$lens_line" || { echo "FAIL: skills/review-artifact/SKILL.md has no 'Run the required lenses' enumeration"; exit 1; }
lens_list="$(printf '%s' "$lens_line" | sed -n 's/.*by default: \([^.]*\)\..*/\1/p')"
test -n "$lens_list" || { echo "FAIL: could not read the lens enumeration out of skills/review-artifact/SKILL.md"; exit 1; }
for lens in 'Feasibility' 'Completeness' 'Scope and alignment' 'Architecture' \
            'Intent preservation' 'Security' 'Testing-quality' 'Data-migration' \
            'Mechanical-precision'; do
  case "$lens_list" in
    *"$lens"*) ;;
    *) echo "FAIL: the required-lens enumeration in skills/review-artifact/SKILL.md omits $lens"; exit 1 ;;
  esac
done

# and no user-facing document may claim a different count than the gate enforces
for doc in README.md docs/quickstart.md docs/README.claude.md docs/README.codex.md commands/review-artifact.md; do
  # Any claim of a lens count other than nine, not just the exact phrase "five required": none
  # of these documents contains the word "five" at all today, so matching one phrase asserted
  # nothing. "Run the five lenses." could be appended to any of them and this stayed at exit 0.
  # Read the count and compare it, rather than trying to enumerate every wrong spelling: the
  # first attempt at this matched [0-9]+ and so rejected a document that correctly said
  # "8 lenses", while allowing "eight" only because that word had been left out of the list.
  while read -r count; do
    case "$(printf '%s' "$count" | tr '[:upper:]' '[:lower:]')" in
      nine|9) ;;
      *) echo "FAIL: $doc claims $count lenses; artifactReviewComplete enforces nine"; exit 1 ;;
    esac
  done < <(grep -Eoi '\b(one|two|three|four|five|six|seven|eight|nine|ten|[0-9]+) (required )?lenses\b' "$doc" | awk '{print $1}')
done
grep -q 'return the actual artifact-review verdict' skills/review-artifact/SKILL.md
grep -q 'parallel subagents by default' docs/quickstart.md
grep -q 'in-session-emulated' docs/quickstart.md
grep -q 'weaker evidence' docs/quickstart.md
grep -q 'not independently adversarial' docs/README.codex.md
grep -q 'weaker evidence' docs/README.codex.md
grep -q 'not independently adversarial' docs/README.claude.md
grep -q 'weaker evidence' docs/README.claude.md
if [ -d lib ] || [ -d tests/lib ]; then
  echo "legacy JS implementation and tests must not exist" >&2
  exit 1
fi
grep -q -- '--scaffold-only' docs/quickstart.md
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

grep -q 'metareview fsm --agent-prompt' skills/fsm/SKILL.md
grep -q 'Fork first, then commit' skills/fsm/SKILL.md
grep -q 'never satisfies a gate' skills/fsm/SKILL.md
grep -q 'docs/fsm/driving-a-workflow.md' README.md
grep -q 'metareview fsm' CLAUDE.md
grep -q 'metareview fsm' AGENTS.md
