'use strict';

const fs = require('fs');
const path = require('path');

function exists(root, rel) {
  return fs.existsSync(path.join(root, rel));
}

function readIfExists(root, rel) {
  const full = path.join(root, rel);
  try {
    const stat = fs.statSync(full);
    if (!stat.isFile()) return '';
    fs.accessSync(full, fs.constants.R_OK);
    return fs.readFileSync(full, 'utf8');
  } catch (_error) {
    return '';
  }
}

function hasMetaswarmInstructionMarker(text) {
  const normalized = text.toLowerCase();
  const negatedMarkers = [
    'does not use metaswarm',
    'not using metaswarm',
    'without metaswarm',
    'no metaswarm'
  ];

  if (negatedMarkers.some(marker => normalized.includes(marker))) {
    return false;
  }

  return normalized.includes('uses metaswarm') ||
    normalized.includes('metaswarm workflows') ||
    normalized.includes('metaswarm');
}

function hasMetaswarmMarker(root) {
  const instructionText = [
    readIfExists(root, 'AGENTS.md'),
    readIfExists(root, 'CLAUDE.md'),
    readIfExists(root, 'GEMINI.md')
  ].join('\n');

  return hasMetaswarmInstructionMarker(instructionText) ||
    exists(root, '.claude/plugins/metaswarm') ||
    exists(root, '.codex/plugins/metaswarm') ||
    exists(root, 'docs/metaswarm') ||
    exists(root, '.beads/context/project-context.md');
}

function findServiceInventory(root) {
  const candidates = [
    'docs/SERVICE_INVENTORY.md',
    'SERVICE_INVENTORY.md',
    'docs/service-inventory.md',
    'docs/architecture/SERVICE_INVENTORY.md'
  ];
  return candidates.find(rel => exists(root, rel)) || null;
}

function detectRepo(root) {
  const capabilities = {
    git: exists(root, '.git'),
    beads: exists(root, '.beads') || exists(root, '.beads/issues.jsonl'),
    metaswarm: hasMetaswarmMarker(root),
    serviceInventory: Boolean(findServiceInventory(root)),
    metareviewState: exists(root, '.metareview')
  };

  let mode = 'advisory';
  if (capabilities.metaswarm) {
    mode = 'metaswarm-extension';
  } else if (capabilities.beads) {
    mode = 'standalone-full';
  } else if (capabilities.git) {
    mode = 'standalone-minimal';
  }

  return {
    mode,
    capabilities,
    files: {
      serviceInventory: findServiceInventory(root)
    }
  };
}

module.exports = { detectRepo };
