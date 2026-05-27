#!/usr/bin/env bash
set -euo pipefail

node - <<'NODE'
const fs = require('fs');
const path = require('path');
const os = require('os');
const { createArtifactReview } = require('./lib/artifact-review');
const { createRunId } = require('./lib/state');

function assert(cond, msg) { if (!cond) throw new Error(msg); }
function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
function write(p, text) { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-'));
mkdirp(path.join(root, '.git'));
write(path.join(root, 'docs', 'plan.md'), '# Plan\n\nBuild the artifact review harness.\n');

const now = new Date('2026-05-26T12:34:56Z');
const expectedRunId = createRunId('artifact', 'docs/plan.md', now);
const result = createArtifactReview(root, 'docs/plan.md', { now });
assert(result.runId === expectedRunId, `run id mismatch: ${result.runId}`);
assert(result.reviewLogPath === `docs/metareview/reviews/${expectedRunId}.md`, result.reviewLogPath);
assert(fs.existsSync(path.join(root, result.reviewLogPath)), 'review log missing');
assert(fs.existsSync(path.join(root, '.metareview', 'runs.jsonl')), 'runs jsonl missing');
assert(fs.existsSync(path.join(root, '.metareview', 'findings.jsonl')), 'findings jsonl missing');
assert(fs.existsSync(path.join(root, 'docs', 'metareview', 'FINDINGS.md')), 'findings index missing');

const log = fs.readFileSync(path.join(root, result.reviewLogPath), 'utf8');
assert(log.includes('Execution mode: `in-session-emulated`'), 'missing execution mode');
assert(log.includes('Previous run: `none`'), 'missing previous run none');
assert(log.includes('## Reviewer Prompts'), 'missing reviewer prompts');
assert(log.includes('## Aggregation Instructions'), 'missing aggregation instructions');

const runs = fs.readFileSync(path.join(root, '.metareview', 'runs.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
assert(runs.length === 1, 'expected one run record');
assert(runs[0].scope === 'artifact', 'run scope mismatch');
assert(runs[0].status === 'open', 'run status mismatch');
assert(runs[0].previousRunId === null, 'first run should not link a previous run');

const retryNow = new Date('2026-05-26T12:35:56Z');
const retryExpectedRunId = createRunId('artifact', 'docs/plan.md', retryNow);
const retry = createArtifactReview(root, 'docs/plan.md', { now: retryNow, previousRunId: result.runId });
assert(retry.runId === retryExpectedRunId, `retry run id mismatch: ${retry.runId}`);
const updatedRuns = fs.readFileSync(path.join(root, '.metareview', 'runs.jsonl'), 'utf8').trim().split('\n').map(JSON.parse);
assert(updatedRuns.length === 2, 'expected two run records after re-review');
assert(updatedRuns[1].previousRunId === result.runId, 're-review did not link previous run');

const preserveRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-'));
mkdirp(path.join(preserveRoot, '.git'));
write(path.join(preserveRoot, 'docs', 'plan.md'), '# Plan\n');
const existingFinding = '{"id":"existing","status":"open"}\n';
write(path.join(preserveRoot, '.metareview', 'findings.jsonl'), existingFinding);
createArtifactReview(preserveRoot, 'docs/plan.md', { now });
const preservedFindings = fs.readFileSync(path.join(preserveRoot, '.metareview', 'findings.jsonl'), 'utf8');
assert(preservedFindings === existingFinding, 'existing findings jsonl content changed');

const collisionRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-collision-'));
mkdirp(path.join(collisionRoot, '.git'));
write(path.join(collisionRoot, 'docs', 'plan.md'), '# Plan\n');
const collision = createArtifactReview(collisionRoot, 'docs/plan.md', { now });
const collisionContextPath = path.join(collisionRoot, collision.contextPath);
fs.writeFileSync(collisionContextPath, 'SENTINEL');
let collisionMessage = '';
try {
  createArtifactReview(collisionRoot, 'docs/plan.md', { now });
} catch (error) {
  collisionMessage = error.message;
}
assert(collisionMessage.includes('Review log already exists'), `unexpected collision error: ${collisionMessage}`);
assert(fs.readFileSync(collisionContextPath, 'utf8') === 'SENTINEL', 'duplicate run rewrote context pack before throwing');

const writeFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-write-fail-'));
mkdirp(path.join(writeFailureRoot, '.git'));
write(path.join(writeFailureRoot, 'docs', 'plan.md'), '# Plan\n');
const reviewDir = path.join(writeFailureRoot, 'docs', 'metareview', 'reviews');
mkdirp(reviewDir);
fs.chmodSync(reviewDir, 0o555);
let writeFailureMessage = '';
try {
  createArtifactReview(writeFailureRoot, 'docs/plan.md', { now });
} catch (error) {
  writeFailureMessage = error.message;
} finally {
  fs.chmodSync(reviewDir, 0o755);
}
assert(writeFailureMessage, 'expected review log write failure');
const failedRunsPath = path.join(writeFailureRoot, '.metareview', 'runs.jsonl');
const failedRuns = fs.existsSync(failedRunsPath) ? fs.readFileSync(failedRunsPath, 'utf8') : '';
assert(failedRuns.trim() === '', 'run record was appended after review log write failed');
const failedContextPath = path.join(writeFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
assert(!fs.existsSync(failedContextPath), 'context pack remained after review log write failed');
assert(!fs.existsSync(path.join(writeFailureRoot, '.metareview', 'findings.jsonl')), 'new findings jsonl remained after review log write failed');
assert(!fs.existsSync(path.join(writeFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'new findings index remained after review log write failed');

const preserveFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-fail-'));
mkdirp(path.join(preserveFailureRoot, '.git'));
write(path.join(preserveFailureRoot, 'docs', 'plan.md'), '# Plan\n');
const preexistingFinding = '{"id":"preexisting","status":"open"}\n';
const preexistingIndex = '# Existing Findings\n';
write(path.join(preserveFailureRoot, '.metareview', 'findings.jsonl'), preexistingFinding);
write(path.join(preserveFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), preexistingIndex);
const preserveFailureReviewDir = path.join(preserveFailureRoot, 'docs', 'metareview', 'reviews');
mkdirp(preserveFailureReviewDir);
fs.chmodSync(preserveFailureReviewDir, 0o555);
try {
  createArtifactReview(preserveFailureRoot, 'docs/plan.md', { now });
} catch (_error) {
} finally {
  fs.chmodSync(preserveFailureReviewDir, 0o755);
}
assert(fs.readFileSync(path.join(preserveFailureRoot, '.metareview', 'findings.jsonl'), 'utf8') === preexistingFinding, 'pre-existing findings jsonl was removed or changed after write failure');
assert(fs.readFileSync(path.join(preserveFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), 'utf8') === preexistingIndex, 'pre-existing findings index was removed or changed after write failure');

const preserveContextFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-context-fail-'));
mkdirp(path.join(preserveContextFailureRoot, '.git'));
write(path.join(preserveContextFailureRoot, 'docs', 'plan.md'), '# Plan\n');
const preexistingContextPath = path.join(preserveContextFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
write(preexistingContextPath, 'PREEXISTING CONTEXT');
const preserveContextReviewDir = path.join(preserveContextFailureRoot, 'docs', 'metareview', 'reviews');
mkdirp(preserveContextReviewDir);
fs.chmodSync(preserveContextReviewDir, 0o555);
try {
  createArtifactReview(preserveContextFailureRoot, 'docs/plan.md', { now });
} catch (_error) {
} finally {
  fs.chmodSync(preserveContextReviewDir, 0o755);
}
assert(fs.readFileSync(preexistingContextPath, 'utf8') === 'PREEXISTING CONTEXT', 'pre-existing context pack was removed or changed after write failure');

const stateInitFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-state-init-fail-'));
mkdirp(path.join(stateInitFailureRoot, '.git'));
write(path.join(stateInitFailureRoot, 'docs', 'plan.md'), '# Plan\n');
write(path.join(stateInitFailureRoot, '.metareview'), 'not a directory\n');
let stateInitFailureMessage = '';
try {
  createArtifactReview(stateInitFailureRoot, 'docs/plan.md', { now });
} catch (error) {
  stateInitFailureMessage = error.message;
}
assert(stateInitFailureMessage, 'expected state initialization failure');
assert(fs.readFileSync(path.join(stateInitFailureRoot, '.metareview'), 'utf8') === 'not a directory\n', 'pre-existing .metareview file was changed');
assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after state initialization failed');
assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`)), 'context pack remained after state initialization failed');
assert(!fs.existsSync(path.join(stateInitFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'findings index remained after state initialization failed');

const appendFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-append-fail-'));
mkdirp(path.join(appendFailureRoot, '.git'));
write(path.join(appendFailureRoot, 'docs', 'plan.md'), '# Plan\n');
mkdirp(path.join(appendFailureRoot, '.metareview', 'runs.jsonl'));
let appendFailureMessage = '';
try {
  createArtifactReview(appendFailureRoot, 'docs/plan.md', { now });
} catch (error) {
  appendFailureMessage = error.message;
}
assert(appendFailureMessage, 'expected run append failure');
assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after run append failed');
assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`)), 'context pack remained after run append failed');
assert(!fs.existsSync(path.join(appendFailureRoot, '.metareview', 'findings.jsonl')), 'new findings jsonl remained after run append failed');
assert(!fs.existsSync(path.join(appendFailureRoot, 'docs', 'metareview', 'FINDINGS.md')), 'new findings index remained after run append failed');

const preserveAppendFailureRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-preserve-append-fail-'));
mkdirp(path.join(preserveAppendFailureRoot, '.git'));
write(path.join(preserveAppendFailureRoot, 'docs', 'plan.md'), '# Plan\n');
write(path.join(preserveAppendFailureRoot, '.metareview', 'findings.jsonl'), preexistingFinding);
write(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), preexistingIndex);
const preexistingAppendContextPath = path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'context', `${expectedRunId}-context.md`);
write(preexistingAppendContextPath, 'PREEXISTING APPEND CONTEXT');
mkdirp(path.join(preserveAppendFailureRoot, '.metareview', 'runs.jsonl'));
try {
  createArtifactReview(preserveAppendFailureRoot, 'docs/plan.md', { now });
} catch (_error) {
}
assert(!fs.existsSync(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'reviews', `${expectedRunId}.md`)), 'review log remained after preserved run append failed');
assert(fs.readFileSync(path.join(preserveAppendFailureRoot, '.metareview', 'findings.jsonl'), 'utf8') === preexistingFinding, 'pre-existing findings jsonl was removed or changed after append failure');
assert(fs.readFileSync(path.join(preserveAppendFailureRoot, 'docs', 'metareview', 'FINDINGS.md'), 'utf8') === preexistingIndex, 'pre-existing findings index was removed or changed after append failure');
assert(fs.readFileSync(preexistingAppendContextPath, 'utf8') === 'PREEXISTING APPEND CONTEXT', 'pre-existing context pack was removed or changed after append failure');

const displayRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-display-'));
mkdirp(path.join(displayRoot, '.git'));
write(path.join(displayRoot, 'docs', 'bad name.md'), '# Plan\n');
const unsafeTarget = 'docs/bad\nname.md';
const unsafePath = path.join(displayRoot, unsafeTarget);
mkdirp(path.dirname(unsafePath));
fs.renameSync(path.join(displayRoot, 'docs', 'bad name.md'), unsafePath);
const unsafeResult = createArtifactReview(displayRoot, unsafeTarget, { now });
const unsafeLog = fs.readFileSync(path.join(displayRoot, unsafeResult.reviewLogPath), 'utf8');
assert(unsafeLog.includes('Target: `docs/bad name.md`'), 'target display was not sanitized');
assert(!unsafeLog.includes('Target: `docs/bad\nname.md`'), 'target display contains newline');

const backtickRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-backtick-'));
mkdirp(path.join(backtickRoot, '.git'));
const injectedTarget = 'docs/bad` **INJECTED** `.md';
write(path.join(backtickRoot, injectedTarget), '# Plan\n');
const backtickResult = createArtifactReview(backtickRoot, injectedTarget, { now });
const backtickLog = fs.readFileSync(path.join(backtickRoot, backtickResult.reviewLogPath), 'utf8');
assert(backtickLog.includes('Target: ``docs/bad` **INJECTED** `.md``'), 'target backticks did not use safe code span delimiters');
assert(!backtickLog.includes('Target: `docs/bad` **INJECTED** `.md`'), 'target contains raw injectable backticks');

const leadingBacktickRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-artifact-leading-backtick-'));
mkdirp(path.join(leadingBacktickRoot, '.git'));
const leadingBacktickTarget = '` **INJECTED** `/plan.md';
write(path.join(leadingBacktickRoot, leadingBacktickTarget), '# Plan\n');
const leadingBacktickResult = createArtifactReview(leadingBacktickRoot, leadingBacktickTarget, { now });
const leadingBacktickLog = fs.readFileSync(path.join(leadingBacktickRoot, leadingBacktickResult.reviewLogPath), 'utf8');
assert(leadingBacktickLog.includes('Target: `` ` **INJECTED** `/plan.md ``'), 'leading backtick target did not use padded safe code span');
assert(!leadingBacktickLog.includes('Target: ``` **INJECTED** `/plan.md``'), 'leading backtick target used ambiguous unpadded code span');
NODE
