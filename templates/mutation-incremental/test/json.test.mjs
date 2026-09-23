import { test } from 'node:test';
import assert from 'node:assert/strict';
import { canonicalJSON } from '../lib/json.mjs';
import { UsageError, LockHeldError, EngineError } from '../lib/errors.mjs';

test('canonicalJSON sorts keys recursively and ends with a newline', () => {
  const out = canonicalJSON({ b: 1, a: { d: [{ z: 1, y: 2 }], c: null } });
  assert.equal(out, '{\n  "a": {\n    "c": null,\n    "d": [\n      {\n        "y": 2,\n        "z": 1\n      }\n    ]\n  },\n  "b": 1\n}\n');
});

test('errors carry exit codes', () => {
  assert.equal(new UsageError('x').exitCode, 2);
  assert.equal(new LockHeldError('x').exitCode, 3);
  assert.equal(new EngineError('x').exitCode, 4);
});

test('cli placeholder loads', async () => {
  await import('../cli.mjs');
});
