import { expect, test } from 'vitest';
import { twice } from '@app/c';
import { fmt } from './helpers/fmt';

test('doubles small numbers', () => expect(fmt(twice(3))).toBe('6'));
test('caps at LIMIT', () => expect(fmt(twice(80))).toBe('100'));
