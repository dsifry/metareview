import { expect, test } from 'vitest';
import { grade } from '../src/index';

test('barrel re-exports grade', () => expect(grade(70, 50, 90)).toBe('ok'));
