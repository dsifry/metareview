import { expect, test } from 'vitest';
import { inRange } from '../src/b';
import { range } from './helpers/make';

test('inside the range', () => expect(inRange(60, range(50, 90))).toBe(true));
test('outside the range', () => expect(inRange(10, range(50, 90))).toBe(false));
