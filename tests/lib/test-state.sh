#!/usr/bin/env bash
set -euo pipefail

node - <<'NODE'
const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');
const { appendJsonl, readJsonl, createRunId, createFindingId } = require('./lib/state');

function assert(cond, msg) { if (!cond) throw new Error(msg); }
function targetHash(target) {
  return crypto.createHash('sha1').update(target).digest('hex').slice(0, 8);
}

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-state-'));
const file = path.join(root, '.metareview', 'runs.jsonl');

appendJsonl(file, { id: 'one', value: 1 });
appendJsonl(file, { id: 'two', value: 2 });
const rows = readJsonl(file);
assert(rows.length === 2, `expected 2 rows, got ${rows.length}`);
assert(rows[0].id === 'one', 'first row mismatch');
assert(rows[1].value === 2, 'second row mismatch');

const target = 'docs/specs/example plan.md';
const runId = createRunId('artifact', target, new Date('2026-05-26T12:34:56Z'));
assert(runId === `mrv-20260526-123456000-artifact-example-plan-${targetHash(target)}`, `run id mismatch: ${runId}`);

const findingId = createFindingId(runId, 7);
assert(findingId === `mrvf-20260526-123456000-artifact-example-plan-${targetHash(target)}-007`, `finding id mismatch: ${findingId}`);

const firstRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:01Z'));
const secondRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:02Z'));
assert(firstRunId !== secondRunId, 'run ids should differ for same-minute runs with different seconds');

const firstMsRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:56.001Z'));
const secondMsRunId = createRunId('artifact', target, new Date('2026-05-26T12:34:56.002Z'));
assert(firstMsRunId !== secondMsRunId, 'run ids should differ for same-second runs with different milliseconds');

const specApiRunId = createRunId('artifact', 'docs/specs/api/spec.md', new Date('2026-05-26T12:34:56Z'));
const specDesignRunId = createRunId('artifact', 'docs/design/spec.md', new Date('2026-05-26T12:34:56Z'));
assert(specApiRunId !== specDesignRunId, 'run ids should differ for same-timestamp targets with same basename');
NODE
