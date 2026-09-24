import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { configDigest } from '../lib/snapshot.mjs';

// Shared with metareview's Go gate (internal/mutationfresh): both must digest the config file
// identically, or every kill reads stale whenever the attested config is compared.
const VECTORS = new URL('../../../testdata/mutation-incremental/config-digest-vectors.json', import.meta.url);

test('config digest vectors (shared with the Go gate)', () => {
  const { cases } = JSON.parse(readFileSync(VECTORS, 'utf8'));
  assert.ok(cases.length >= 5);
  const dir = mkdtempSync(join(tmpdir(), 'mi-cfgdigest-'));
  for (const c of cases) {
    const path = join(dir, 'config.json');
    writeFileSync(path, c.text);
    assert.equal(configDigest(path), c.digest, c.name);
  }
});

test('the view map does not change the digest; every other key does', () => {
  const { cases } = JSON.parse(readFileSync(VECTORS, 'utf8'));
  const byName = Object.fromEntries(cases.map((c) => [c.name, c.digest]));
  assert.equal(byName['example with inline views'], byName.example);
  assert.equal(byName['example reformatted'], byName.example);
  assert.notEqual(byName['example with another budget'], byName.example);
});
