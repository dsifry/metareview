import { test } from 'node:test';
import assert from 'node:assert/strict';
import { canonicalJSON } from '../lib/json.mjs';
import { UsageError, LockHeldError, EngineError } from '../lib/errors.mjs';
import { duplicateKey } from '../lib/json.mjs';

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


test('duplicateKey finds a repeated key at any depth and ignores keys in arrays and strings', () => {
  assert.equal(duplicateKey('{"a":1,"b":{"c":1,"c":2}}'), 'c');
  assert.equal(duplicateKey('{"a":1,"a":2}'), 'a');
  assert.equal(duplicateKey('{"a":["a","a"],"b":"a\\"a","c":{"a":1}}'), null);
  assert.equal(duplicateKey('"just a string"'), null);
  assert.equal(duplicateKey('{"x\\u0041":1,"xA":2}'), 'xA');
});
