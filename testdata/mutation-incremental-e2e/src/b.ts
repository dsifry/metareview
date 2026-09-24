import { grade } from './a';
import type { Range } from './types';

export const inRange = (n: number, r: Range) => grade(n, r.lo, r.hi) === 'ok';
