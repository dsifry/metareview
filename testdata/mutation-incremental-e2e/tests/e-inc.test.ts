import { expect, test } from 'vitest';
import { inc } from '../src/e';

test('inc', () => expect(inc(1)).toBe(2));
