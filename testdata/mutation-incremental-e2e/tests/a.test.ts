import assert from 'node:assert';
import { test } from 'vitest';
import { grade } from '../src/a';

test('below pass fails', () => assert.equal(grade(10, 50, 90), 'fail'));
test('at pass is ok', () => assert.equal(grade(50, 50, 90), 'ok'));
test('at top is ok', () => assert.equal(grade(90, 50, 90), 'ok'));
test('above top is over', () => assert.equal(grade(95, 50, 90), 'over'));
