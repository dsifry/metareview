#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

node - <<'NODE'
const fs = require('fs');
const path = require('path');
const os = require('os');
const { detectRepo } = require('./lib/repo-detect');

function mkdirp(p) { fs.mkdirSync(p, { recursive: true }); }
function write(p, text = '') { mkdirp(path.dirname(p)); fs.writeFileSync(p, text); }
function assert(cond, msg) { if (!cond) throw new Error(msg); }

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'metareview-detect-'));

const advisory = path.join(root, 'advisory');
mkdirp(advisory);
let report = detectRepo(advisory);
assert(report.mode === 'advisory', `expected advisory, got ${report.mode}`);
assert(report.capabilities.git === false, 'advisory fixture should not have git');

const standalone = path.join(root, 'standalone');
mkdirp(path.join(standalone, '.git'));
report = detectRepo(standalone);
assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
assert(report.capabilities.git === true, 'standalone fixture should have git');

const negatedMetaswarm = path.join(root, 'negated-metaswarm');
mkdirp(path.join(negatedMetaswarm, '.git'));
write(path.join(negatedMetaswarm, 'AGENTS.md'), 'This repository does not use metaswarm.');
report = detectRepo(negatedMetaswarm);
assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
assert(report.capabilities.metaswarm === false, 'negated metaswarm fixture should not detect metaswarm');

const directoryAgents = path.join(root, 'directory-agents');
mkdirp(path.join(directoryAgents, '.git'));
mkdirp(path.join(directoryAgents, 'AGENTS.md'));
report = detectRepo(directoryAgents);
assert(report.mode === 'standalone-minimal', `expected standalone-minimal, got ${report.mode}`);
assert(report.capabilities.metaswarm === false, 'directory AGENTS.md fixture should not detect metaswarm');

const beads = path.join(root, 'beads');
mkdirp(path.join(beads, '.git'));
mkdirp(path.join(beads, '.beads', 'knowledge'));
write(path.join(beads, '.beads', 'issues.jsonl'), '');
report = detectRepo(beads);
assert(report.mode === 'standalone-full', `expected standalone-full, got ${report.mode}`);
assert(report.capabilities.beads === true, 'beads fixture should detect beads');

const metaswarm = path.join(root, 'metaswarm');
mkdirp(path.join(metaswarm, '.git'));
mkdirp(path.join(metaswarm, '.beads', 'knowledge'));
write(path.join(metaswarm, 'AGENTS.md'), 'This repo uses metaswarm workflows.');
write(path.join(metaswarm, 'docs', 'SERVICE_INVENTORY.md'), '# Service Inventory\n');
report = detectRepo(metaswarm);
assert(report.mode === 'metaswarm-extension', `expected metaswarm-extension, got ${report.mode}`);
assert(report.capabilities.metaswarm === true, 'metaswarm fixture should detect metaswarm');
assert(report.files.serviceInventory === 'docs/SERVICE_INVENTORY.md', 'service inventory path mismatch');
NODE
