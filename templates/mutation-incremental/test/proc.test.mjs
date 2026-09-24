import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createInterrupt, installSignalHandlers, KILL_GRACE_MS, signalGroup, spawnGroup } from '../lib/proc.mjs';

const node = (script) => [process.execPath, '-e', script];
const opts = (extra) => ({ cwd: tmpdir(), env: process.env, timeoutMs: null, graceMs: 200, interrupt: createInterrupt(), onOutput: () => {}, ...extra });

test('spawnGroup reports the exit code and forwards both output streams', async () => {
  let out = '';
  const r = await spawnGroup(node("process.stdout.write('o'); process.stderr.write('e'); process.exit(3)"), opts({ onOutput: (c) => { out += c; } }));
  assert.deepEqual([r.code, r.timedOut, r.interrupted, r.error], [3, false, false, null]);
  assert.deepEqual([...out].sort(), ['e', 'o']);
  assert.equal(KILL_GRACE_MS, 30_000);
});

test('spawnGroup passes the given environment', async () => {
  const r = await spawnGroup(node("process.exit(process.env.MI_X === 'y' ? 0 : 5)"), opts({ env: { ...process.env, MI_X: 'y' } }));
  assert.equal(r.code, 0);
});

test('a timeout terminates the whole process group', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
  const late = join(dir, 'late');
  const grandchild = `setTimeout(() => require("fs").writeFileSync(${JSON.stringify(late)}, "x"), 1000)`;
  const script = `require('child_process').spawn(process.execPath, ['-e', ${JSON.stringify(grandchild)}], { stdio: 'ignore' }); setTimeout(() => {}, 20000);`;
  const r = await spawnGroup(node(script), opts({ cwd: dir, timeoutMs: 400 }));
  assert.equal(r.timedOut, true);
  await new Promise((res) => setTimeout(res, 1300));
  assert.equal(existsSync(late), false);
});

test('a child that ignores SIGTERM is killed after the grace period', async () => {
  const r = await spawnGroup(node("process.on('SIGTERM', () => {}); setTimeout(() => {}, 20000);"), opts({ timeoutMs: 400, graceMs: 200 }));
  assert.deepEqual([r.timedOut, r.signal], [true, 'SIGKILL']);
});

test('an interrupt stops the child; after an interrupt nothing new is spawned', async () => {
  const interrupt = createInterrupt();
  setTimeout(() => interrupt.trigger(), 300);
  const r = await spawnGroup(node('setTimeout(() => {}, 20000)'), opts({ interrupt }));
  assert.equal(r.interrupted, true);
  const dir = mkdtempSync(join(tmpdir(), 'mi-proc-'));
  const again = await spawnGroup(node("require('fs').writeFileSync('ran', 'x')"), opts({ interrupt, cwd: dir }));
  assert.equal(again.interrupted, true);
  assert.equal(existsSync(join(dir, 'ran')), false);
});

test('a command that cannot be spawned reports the error', async () => {
  const r = await spawnGroup(['/nonexistent/mi-command'], opts({}));
  assert.equal(r.code, null);
  assert.match(r.error.message, /ENOENT/);
});

test('signalGroup is false for a group that does not exist', () => {
  const { pid } = spawnSync(process.execPath, ['-e', '']);
  assert.equal(signalGroup(pid, 'SIGTERM'), false);
});

test('installSignalHandlers routes signals to the interrupt until removed', () => {
  const interrupt = createInterrupt();
  const before = process.listenerCount('SIGHUP');
  const uninstall = installSignalHandlers(interrupt);
  process.emit('SIGHUP');
  assert.equal(interrupt.interrupted, true);
  uninstall();
  assert.equal(process.listenerCount('SIGHUP'), before);
});
