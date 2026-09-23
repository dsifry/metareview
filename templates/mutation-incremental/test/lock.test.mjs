import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { hostname, tmpdir } from 'node:os';
import { join } from 'node:path';
import { acquireLock, LOCK_FILE, pidAlive } from '../lib/lock.mjs';
import { main } from '../lib/main.mjs';
import { makeRepo } from './helpers.mjs';

const stateDir = () => mkdtempSync(join(tmpdir(), 'mi-lock-'));
const deadPid = () => spawnSync(process.execPath, ['-e', '']).pid;
const now = new Date('2026-09-23T18:00:00.000Z');
const capture = () => ({ text: '', write(s) { this.text += s; } });

test('acquireLock writes pid, hostname and startedAt; release removes it', () => {
  const dir = stateDir();
  const release = acquireLock(dir, { pid: 42, host: 'h', now });
  assert.deepEqual(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')), { pid: 42, hostname: 'h', startedAt: '2026-09-23T18:00:00.000Z' });
  release();
  assert.equal(existsSync(join(dir, LOCK_FILE)), false);
  const own = stateDir();
  const releaseOwn = acquireLock(own);
  assert.deepEqual(Object.keys(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8'))), ['pid', 'hostname', 'startedAt']);
  assert.equal(JSON.parse(readFileSync(join(own, LOCK_FILE), 'utf8')).pid, process.pid);
  releaseOwn();
});

test('a lock held by a live process on this host is exit 3 naming it', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: process.pid, host: hostname(), now });
  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && e.message.includes(`pid ${process.pid}`) && /break-lock/.test(e.message));
});

test('a lock left by a dead process on this host is replaced', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: deadPid(), host: hostname(), now });
  acquireLock(dir, { pid: 7, host: hostname(), now });
  assert.equal(JSON.parse(readFileSync(join(dir, LOCK_FILE), 'utf8')).pid, 7);
});

test('a lock from another machine or an unreadable lock is held', () => {
  const dir = stateDir();
  acquireLock(dir, { pid: deadPid(), host: 'other-machine', now });
  assert.throws(() => acquireLock(dir, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /other-machine/.test(e.message));
  const junk = stateDir();
  writeFileSync(join(junk, LOCK_FILE), '');
  assert.throws(() => acquireLock(junk, { pid: 7, host: hostname(), now }), (e) => e.exitCode === 3 && /unreadable lock/.test(e.message));
});

test('acquireLock rethrows anything but an existing lock', () => {
  assert.throws(() => acquireLock(join(stateDir(), 'missing'), { pid: 7, host: 'h', now }), (e) => e.code === 'ENOENT');
});

test('pidAlive', () => {
  assert.equal(pidAlive(process.pid), true);
  assert.equal(pidAlive(deadPid()), false);
  assert.equal(pidAlive(1), true); // init/launchd: EPERM for a normal user, alive either way
});

test('break-lock prints and removes the lock, or says there is none', async () => {
  const r = makeRepo();
  const run = async () => {
    const stdout = capture();
    const stderr = capture();
    const code = await main(['break-lock'], { stdout, stderr, cwd: r.top });
    return { code, stdout: stdout.text, stderr: stderr.text };
  };
  const none = await run();
  assert.equal(none.code, 0);
  assert.match(none.stdout, /no lock at .*\.mutation\/lock/);
  assert.match(none.stderr, /command=break-lock invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
  mkdirSync(join(r.top, '.mutation'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/lock'), '{"pid":1,"hostname":"ci","startedAt":"x"}\n');
  const held = await run();
  assert.match(held.stdout, /\{"pid":1,"hostname":"ci","startedAt":"x"\}\nbreak-lock: removed /);
  assert.equal(existsSync(join(r.top, '.mutation/lock')), false);
});
