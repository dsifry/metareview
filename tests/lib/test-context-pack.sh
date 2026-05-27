#!/usr/bin/env bash
set -euo pipefail

node - <<'NODE'
const fs = require('fs');
const path = require('path');
const os = require('os');
const { buildContextPack } = require('./lib/context-pack');
const { createRunId } = require('./lib/state');

function assert(cond, msg) { if (!cond) throw new Error(msg); }
function assertThrows(fn, pattern, msg) {
  try {
    fn();
  } catch (error) {
    assert(pattern.test(String(error.message)), `${msg}: ${error.message}`);
    return;
  }
  throw new Error(msg);
}
function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
function write(p, text) { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-context-'));
mkdirp(path.join(root, '.git'));
write(path.join(root, 'docs', 'SERVICE_INVENTORY.md'), '# Service Inventory\n\nExisting services.\n');
write(path.join(root, '.beads', 'knowledge', 'gotchas.jsonl'), '{"id":"fact-1","fact":"Use constructor injection for services."}\n');
write(path.join(root, '.beads', 'knowledge', 'newlines.jsonl'), '{"id":"fact-2","fact":"first\\n## injected"}\n');
write(path.join(root, '.beads', 'knowledge', 'evil\n## injected.jsonl'), '{"id":"fact-3","fact":"filename fact"}\n');
mkdirp(path.join(root, '.beads', 'knowledge', 'bad.jsonl'));
write(path.join(root, 'docs', 'spec.md'), '# Spec\n\nBuild a thing.\n');
write(path.join(root, 'docs', 'fenced.md'), '# Fenced\n\n```markdown\n## Injected\n```\n');
write(path.join(root, 'docs', 'evil\n## injected.md'), '# Evil target\n');
write(path.join(root, '` **INJECTED** `target.md'), '# Backtick target\n');

const now = new Date('2026-05-26T12:34:56Z');
const expectedRunId = createRunId('artifact', 'docs/spec.md', now);
const result = buildContextPack(root, 'docs/spec.md', { now });
assert(result.contextPath === `docs/metareview/context/${expectedRunId}-context.md`, result.contextPath);

const fullPath = path.join(root, result.contextPath);
assert(fs.existsSync(fullPath), 'context pack was not written');
const content = fs.readFileSync(fullPath, 'utf8');
assert(content.includes('# metareview context:'), 'missing title');
assert(content.includes('docs/spec.md'), 'missing target path');
assert(content.includes('SERVICE_INVENTORY.md'), 'missing service inventory mention');
assert(content.includes('Use constructor injection'), 'missing knowledge fact');
assert(content.includes('first ## injected'), 'knowledge newlines should be normalized');
assert(!content.includes('\n## injected\n'), 'knowledge newline created standalone heading');
assert(content.includes('- evil ## injected.jsonl: filename fact'), 'knowledge filename should be single-line sanitized');
assert(!content.includes('\n## injected.jsonl'), 'knowledge filename created standalone heading');

const evilTarget = 'docs/evil\n## injected.md';
const evilTargetResult = buildContextPack(root, evilTarget, { now });
const evilTargetContent = fs.readFileSync(path.join(root, evilTargetResult.contextPath), 'utf8');
assert(evilTargetContent.includes('# metareview context: docs/evil ## injected.md'), 'target title should be single-line sanitized');
assert(evilTargetContent.includes('- Path: `docs/evil ## injected.md`'), 'target path should be single-line sanitized');
assert(!evilTargetContent.includes('\n## injected.md'), 'target path created standalone heading');

const backtickTarget = '` **INJECTED** `target.md';
const backtickTargetResult = buildContextPack(root, backtickTarget, { now });
const backtickTargetContent = fs.readFileSync(path.join(root, backtickTargetResult.contextPath), 'utf8');
assert(backtickTargetContent.includes('# metareview context: **INJECTED** target.md'), 'target title should strip backticks to avoid heading ambiguity');
assert(backtickTargetContent.includes('- Path: `` ` **INJECTED** `target.md ``'), 'target path should use padded safe code span delimiters');
assert(!backtickTargetContent.includes('- Path: `` ` **INJECTED** `target.md``'), 'target path used ambiguous unpadded code span');

const fencedResult = buildContextPack(root, 'docs/fenced.md', { now });
const fencedContent = fs.readFileSync(path.join(root, fencedResult.contextPath), 'utf8');
assert(/## Artifact Excerpt\n\n````+markdown\n# Fenced/.test(fencedContent), 'artifact excerpt should use a fence longer than artifact backticks');

assertThrows(
  () => buildContextPack(root, 'docs/missing.md', { now }),
  /Target artifact not found: docs\/missing\.md/,
  'missing targets should be rejected'
);
assertThrows(
  () => buildContextPack(root, '../outside.md', { now }),
  /Target artifact is outside repository root: \.\.\/outside\.md/,
  'path traversal should be rejected'
);

const outside = path.join(os.tmpdir(), `metareview-outside-${process.pid}.md`);
fs.writeFileSync(outside, '# Outside\n');
try {
  fs.symlinkSync(outside, path.join(root, 'docs', 'linked.md'));
  assertThrows(
    () => buildContextPack(root, 'docs/linked.md', { now }),
    /outside repository root/,
    'symlink escape should be rejected'
  );
} catch (error) {
  if (!['EACCES', 'EPERM', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) {
    throw error;
  }
} finally {
  fs.rmSync(outside, { force: true });
}

const fileKnowledgeRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-context-file-knowledge-'));
mkdirp(path.join(fileKnowledgeRoot, '.git'));
write(path.join(fileKnowledgeRoot, '.beads', 'knowledge'), 'not a directory\n');
write(path.join(fileKnowledgeRoot, 'docs', 'spec.md'), '# Spec\n');
const fileKnowledgeResult = buildContextPack(fileKnowledgeRoot, 'docs/spec.md', { now });
const fileKnowledgeContent = fs.readFileSync(path.join(fileKnowledgeRoot, fileKnowledgeResult.contextPath), 'utf8');
assert(fileKnowledgeContent.includes('No Beads knowledge facts found.'), 'file-shaped knowledge path should be ignored');

const outsideKnowledge = path.join(os.tmpdir(), `metareview-outside-knowledge-${process.pid}.jsonl`);
fs.writeFileSync(outsideKnowledge, '{"id":"outside","fact":"OUTSIDE_KNOWLEDGE_SENTINEL"}\n');
try {
  fs.symlinkSync(outsideKnowledge, path.join(root, '.beads', 'knowledge', 'linked.jsonl'));
  const linkedKnowledgeResult = buildContextPack(root, 'docs/spec.md', { now });
  const linkedKnowledgeContent = fs.readFileSync(path.join(root, linkedKnowledgeResult.contextPath), 'utf8');
  assert(!linkedKnowledgeContent.includes('OUTSIDE_KNOWLEDGE_SENTINEL'), 'outside symlinked knowledge should be ignored');
} catch (error) {
  if (!['EACCES', 'EPERM', 'ENOTSUP', 'EOPNOTSUPP'].includes(error.code)) {
    throw error;
  }
} finally {
  fs.rmSync(outsideKnowledge, { force: true });
}
NODE
