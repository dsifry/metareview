import { copyFileSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { spawnGroup } from './proc.mjs';
import { WORK_DIR } from './attest.mjs';

// Stryker prints this on stdout and stderr when the dry run finds no tests (spec F4).
export const NO_TESTS = 'No tests were executed';

// Spec §5.6 step 4: argv, no shell.
export function strykerArgv(config, incrementalFile, { force, mutate }) {
  return [
    ...config.stryker.command, 'run', config.stryker.configFile,
    '--incremental', '--incrementalFile', incrementalFile,
    ...(force ? ['--force'] : []),
    ...(mutate === null ? [] : ['--mutate', mutate.join(',')]),
    ...config.stryker.extraArgs,
  ];
}

const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

// A mutation-testing-report with a `files` object (selected files absent from it have zero
// mutants, spec F9), or null.
export function parseReport(bytes) {
  let report;
  try {
    report = JSON.parse(bytes.toString('utf8'));
  } catch {
    return null;
  }
  return isObject(report) && isObject(report.files) ? report : null;
}

// mtime (ns), size and inode: "written during the invocation" (spec §5.6 step 5).
function stamp(path) {
  try {
    const s = statSync(path, { bigint: true });
    return `${s.mtimeNs}:${s.size}:${s.ino}`;
  } catch {
    return null;
  }
}

// One invocation (spec §5.6 steps 4–5). `input` is copied to work/inv-<index>.json first; null in
// full mode, where Stryker starts fresh. The caller creates work/.
export async function invoke(config, { index, input, force, mutate, timeoutMs, interrupt, graceMs, onOutput }) {
  const name = `inv-${index}.json`;
  const file = join(config.stateDir, WORK_DIR, name);
  if (input !== null) copyFileSync(input, file);
  const before = stamp(file);
  let tail = '';
  let noTests = false;
  const r = await spawnGroup(strykerArgv(config, `${config.stateRel}/${WORK_DIR}/${name}`, { force, mutate }), {
    cwd: config.top,
    env: process.env,
    timeoutMs,
    graceMs,
    interrupt,
    onOutput: (chunk) => {
      onOutput(chunk);
      const text = tail + chunk.toString('utf8');
      if (text.includes(NO_TESTS)) noTests = true;
      tail = text.slice(-NO_TESTS.length);
    },
  });
  if (r.interrupted) return { outcome: 'interrupted' };
  if (r.timedOut) return { outcome: 'timeout' };
  const after = stamp(file);
  const written = after !== null && after !== before;
  if (written && (r.code === 0 || r.code === 1)) {
    const bytes = readFileSync(file);
    const report = parseReport(bytes);
    if (report !== null) return { outcome: 'success', path: file, bytes, report };
  }
  if (!written && r.code === 1 && noTests) return { outcome: 'no-tests' };
  return { outcome: 'failed', detail: r.error ? r.error.message : `exit ${r.code}` };
}
