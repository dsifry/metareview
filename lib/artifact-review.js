'use strict';

const fs = require('fs');
const path = require('path');
const childProcess = require('child_process');
const { buildContextPack } = require('./context-pack');
const { appendJsonl, createRunId } = require('./state');
const { inlineCode } = require('./markdown');

function gitHead(root) {
  try {
    return childProcess.execSync('git rev-parse HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
  } catch {
    return 'unavailable';
  }
}

function ensureEmptyJsonl(filePath) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  if (!fs.existsSync(filePath)) fs.writeFileSync(filePath, '');
}

function ensureFindingsIndex(root) {
  const findingsPath = path.join(root, 'docs', 'metareview', 'FINDINGS.md');
  fs.mkdirSync(path.dirname(findingsPath), { recursive: true });
  if (!fs.existsSync(findingsPath)) {
    fs.writeFileSync(findingsPath, `# metareview Findings

No unresolved findings recorded yet.
`);
  }
}

function removeIfCreated(filePath, existedBefore) {
  if (!existedBefore && fs.existsSync(filePath)) {
    fs.rmSync(filePath, { force: true });
  }
}

function restoreOrRemove(filePath, existedBefore, previousContent) {
  if (existedBefore) {
    fs.writeFileSync(filePath, previousContent);
    return;
  }
  removeIfCreated(filePath, false);
}

function cleanupArtifacts(paths) {
  fs.rmSync(paths.reviewPath, { force: true });
  restoreOrRemove(paths.contextPath, paths.contextExisted, paths.previousContext);
  removeIfCreated(paths.findingsJsonlPath, paths.findingsJsonlExisted);
  removeIfCreated(paths.findingsIndexPath, paths.findingsIndexExisted);
}

function createArtifactReview(root, target, options = {}) {
  const now = options.now || new Date();
  const runId = createRunId('artifact', target, now);
  const reviewRel = `docs/metareview/reviews/${runId}.md`;
  const reviewPath = path.join(root, reviewRel);
  if (fs.existsSync(reviewPath)) {
    throw new Error(`Review log already exists for run ${runId}: ${reviewRel}`);
  }

  const contextRel = `docs/metareview/context/${runId}-context.md`;
  const contextPath = path.join(root, contextRel);
  const contextExisted = fs.existsSync(contextPath);
  const previousContext = contextExisted ? fs.readFileSync(contextPath, 'utf8') : null;
  const findingsJsonlPath = path.join(root, '.metareview', 'findings.jsonl');
  const findingsIndexPath = path.join(root, 'docs', 'metareview', 'FINDINGS.md');
  const findingsJsonlExisted = fs.existsSync(findingsJsonlPath);
  const findingsIndexExisted = fs.existsSync(findingsIndexPath);

  try {
    const context = buildContextPack(root, target, { now });
    if (context.runId !== runId) {
      throw new Error(`Context pack run ID mismatch: expected ${runId}, got ${context.runId}`);
    }
    fs.mkdirSync(path.dirname(reviewPath), { recursive: true });

    const head = gitHead(root);
    const nowIso = now.toISOString();
    const runRecord = {
      schemaVersion: 1,
      id: context.runId,
      scope: 'artifact',
      target: { type: 'path', path: target },
      status: 'open',
      verdict: 'NOT_REVIEWED',
      executionMode: 'in-session-emulated',
      previousRunId: options.previousRunId || null,
      baseSha: head,
      headSha: head,
      contextPackPath: context.contextPath,
      reviewLogPath: reviewRel,
      reviewers: ['feasibility', 'completeness', 'scope-alignment', 'architecture', 'intent-preservation'],
      findingIds: [],
      sourceRefs: [{ type: 'path', path: target }],
      createdAt: nowIso,
      updatedAt: nowIso,
      repoRoot: root,
      gitHead: head
    };

    ensureEmptyJsonl(findingsJsonlPath);
    ensureFindingsIndex(root);

    const content = `# metareview: artifact review

Run ID: ${inlineCode(context.runId)}

Target: ${inlineCode(target)}

Context pack: ${inlineCode(context.contextPath)}

Execution mode: \`in-session-emulated\`

Previous run: ${inlineCode(runRecord.previousRunId || 'none')}

## Verdict

NOT_REVIEWED

## Reviewer Prompts

Use \`rubrics/artifact-review-rubric.md\` and the context pack above. Run these lenses independently before aggregation:

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation

For each lens, produce:

- verdict
- blocking findings with evidence
- warnings with evidence
- knowledge candidates

## Aggregation Instructions

After all lenses are complete:

1. Deduplicate findings.
2. Assign stable finding IDs.
3. Separate blockers from warnings.
4. Update this review log verdict.
5. Append machine-readable findings to \`.metareview/findings.jsonl\`.
6. Update \`docs/metareview/FINDINGS.md\`.

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |

## Findings

No reviewer findings recorded yet.
`;

    fs.writeFileSync(reviewPath, content);
    appendJsonl(path.join(root, '.metareview', 'runs.jsonl'), runRecord);
    return { runId: context.runId, reviewLogPath: reviewRel, contextPath: context.contextPath };
  } catch (error) {
    cleanupArtifacts({
      reviewPath,
      contextPath,
      contextExisted,
      previousContext,
      findingsJsonlPath,
      findingsJsonlExisted,
      findingsIndexPath,
      findingsIndexExisted
    });
    throw error;
  }
}

module.exports = { createArtifactReview };
