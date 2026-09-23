import { readFileSync, rmSync, writeFileSync } from 'node:fs';
import { hostname } from 'node:os';
import { join } from 'node:path';
import { LockHeldError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { canonicalPendingFull } from './state.mjs';

export const LOCK_FILE = 'lock';

const readText = (path) => {
  try {
    return readFileSync(path, 'utf8');
  } catch {
    return null;
  }
};

function readLock(path) {
  try {
    return JSON.parse(readText(path));
  } catch {
    return null;
  }
}

// kill(pid, 0) probes without signalling; EPERM means the process exists under another user.
export function pidAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (e) {
    return e.code === 'EPERM';
  }
}

// Spec §5.6 step 1: create <stateDir>/lock with O_EXCL. A lock from this host whose pid is gone is
// a crashed run and is replaced; anything else is held (exit 3). The caller creates stateDir.
export function acquireLock(stateDir, { pid = process.pid, host = hostname(), now = new Date() } = {}) {
  const path = join(stateDir, LOCK_FILE);
  const content = `${JSON.stringify({ pid, hostname: host, startedAt: now.toISOString() })}\n`;
  try {
    writeFileSync(path, content, { flag: 'wx' });
  } catch (e) {
    if (e.code !== 'EEXIST') throw e;
    const held = readLock(path);
    if (held === null || held.hostname !== host || pidAlive(held.pid)) {
      const who = held === null ? 'unreadable lock' : `pid ${held.pid} on ${held.hostname} since ${held.startedAt}`;
      throw new LockHeldError(`lock held: ${path} (${who}); if no run is active, run break-lock`);
    }
    writeFileSync(path, content);
  }
  return () => rmSync(path, { force: true });
}

// Spec §5.1: print and remove <stateDir>/lock (clears a lock left on another machine).
export async function breakLockCommand(io, options) {
  const { config } = loadHarnessConfig(io, options, false);
  const path = join(config.stateDir, LOCK_FILE);
  const text = readText(path);
  if (text === null) io.stdout.write(`break-lock: no lock at ${path}\n`);
  else {
    io.stdout.write(text);
    rmSync(path, { force: true });
    io.stdout.write(`break-lock: removed ${path}\n`);
  }
  io.stderr.write(summaryLine({ command: 'break-lock', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
