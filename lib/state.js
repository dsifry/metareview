'use strict';

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function appendJsonl(filePath, record) {
  ensureDir(path.dirname(filePath));
  fs.appendFileSync(filePath, `${JSON.stringify(record)}\n`);
}

function readJsonl(filePath) {
  if (!fs.existsSync(filePath)) return [];
  return fs.readFileSync(filePath, 'utf8')
    .split('\n')
    .filter(line => line.trim().length > 0)
    .map(line => JSON.parse(line));
}

function slugify(value) {
  return String(value)
    .replace(/\.[a-z0-9]+$/i, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48) || 'target';
}

function formatDate(date) {
  const pad = n => String(n).padStart(2, '0');
  const ms = String(date.getUTCMilliseconds()).padStart(3, '0');
  return `${date.getUTCFullYear()}${pad(date.getUTCMonth() + 1)}${pad(date.getUTCDate())}-${pad(date.getUTCHours())}${pad(date.getUTCMinutes())}${pad(date.getUTCSeconds())}${ms}`;
}

function targetHash(target) {
  return crypto.createHash('sha1').update(String(target)).digest('hex').slice(0, 8);
}

function createRunId(scope, target, date = new Date()) {
  const base = path.basename(String(target));
  return `mrv-${formatDate(date)}-${slugify(scope)}-${slugify(base)}-${targetHash(target)}`;
}

function createFindingId(runId, index) {
  return `${runId.replace(/^mrv-/, 'mrvf-')}-${String(index).padStart(3, '0')}`;
}

module.exports = {
  appendJsonl,
  readJsonl,
  createRunId,
  createFindingId,
  slugify
};
