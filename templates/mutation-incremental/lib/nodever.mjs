import { readFileSync } from 'node:fs';
import { join } from 'node:path';

// Spec §5.1: the first of .nvmrc and .node-version that exists, trimmed; a numeric version whose
// dot-separated components are not a prefix of the running version's warns. Aliases are skipped.
export function nodeVersionWarning(top, running = process.version) {
  for (const file of ['.nvmrc', '.node-version']) {
    let text;
    try {
      text = readFileSync(join(top, file), 'utf8');
    } catch {
      continue;
    }
    const want = text.trim().replace(/^v/, '');
    if (!/^\d+(\.\d+)*$/.test(want)) return null;
    const have = running.replace(/^v/, '').split('.');
    const matches = want.split('.').every((part, i) => part === have[i]);
    return matches ? null : `mutation-incremental: warning: ${file} pins Node ${want} but this is ${running}`;
  }
  return null;
}
