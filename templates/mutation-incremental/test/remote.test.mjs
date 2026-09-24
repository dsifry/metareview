import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { authEnv } from '../lib/remote.mjs';
import { readCandidate } from '../lib/state.mjs';
import { cli, reportFor, runRepo, warm } from './fake.mjs';

function bareRemote(r) {
  const bare = mkdtempSync(join(tmpdir(), 'mi-remote-'));
  execFileSync('git', ['init', '-q', '--bare', bare]);
  r.git('remote', 'add', 'origin', bare);
  return bare;
}
const usable = (dir) => readCandidate(dir, { label: 'x', primary: false }).usable;

test('authEnv passes the token as an extraheader after existing GIT_CONFIG entries', () => {
  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', ''), { env: { A: '1' }, mask: null });
  assert.deepEqual(authEnv({ A: '1' }, 'https://h/r.git', undefined), { env: { A: '1' }, mask: null });
  const b64 = Buffer.from('x-access-token:tok').toString('base64');
  assert.deepEqual(authEnv({}, 'https://h/r.git', 'tok'), {
    env: { GIT_CONFIG_COUNT: '1', GIT_CONFIG_KEY_0: 'http.https://h/r.git.extraheader', GIT_CONFIG_VALUE_0: `AUTHORIZATION: basic ${b64}` },
    mask: b64,
  });
  const more = authEnv({ GIT_CONFIG_COUNT: '2' }, 'u', 'tok').env;
  assert.deepEqual([more.GIT_CONFIG_COUNT, more.GIT_CONFIG_KEY_2], ['3', 'http.u.extraheader']);
});

test('publish-state pushes a parentless commit that fetch-state reads back in another checkout', async () => {
  const r = runRepo();
  await warm(r);
  const bare = bareRemote(r);
  const pub = await cli(r, ['publish-state', '--kind', 'full']);
  assert.equal(pub.code, 0);
  assert.match(pub.stdout, /^publish-state: pushed [0-9a-f]{40} to refs\/heads\/mutation-state\/full\n$/);
  assert.match(pub.stderr, /command=publish-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=false\n$/);
  const head = execFileSync('git', ['log', '-1', '--format=%P|%an <%ae>|%cn', 'refs/heads/mutation-state/full'], { cwd: bare, encoding: 'utf8' }).trim();
  assert.equal(head, '|mutation-incremental <noreply@localhost>|mutation-incremental');

  const other = runRepo();
  other.git('remote', 'add', 'origin', bare);
  const got = await cli(other, ['fetch-state']);
  assert.equal(got.code, 0);
  assert.match(got.stderr, /refs\/heads\/mutation-state\/inc is not on origin; skipped/);
  assert.equal(readFileSync(join(other.top, '.mutation/remote/full/incremental.json'), 'utf8'), readFileSync(join(r.top, '.mutation/incremental.json'), 'utf8'));
  assert.equal(usable(join(other.top, '.mutation/remote/full')), true);
  // The other checkout adopts it; its files are identical, so there is nothing to run.
  const run = await cli(other, ['run', '--mode', 'incremental', '--also-state', '.mutation/remote/full']);
  assert.equal(run.code, 0);
  assert.deepEqual(other.calls(), []);
  assert.equal(usable(join(other.top, '.mutation')), true);
});

test('fetch-state reads a state larger than 1 MiB (reports embed every source file)', async () => {
  const r = runRepo({ steps: [{ report: { ...reportFor(), padding: 'x'.repeat(1_500_000) } }] });
  assert.equal((await cli(r, ['run', '--mode', 'full'])).code, 0);
  const bare = bareRemote(r);
  assert.equal((await cli(r, ['publish-state', '--kind', 'full'])).code, 0);
  const other = runRepo();
  other.git('remote', 'add', 'origin', bare);
  const got = await cli(other, ['fetch-state']);
  assert.equal(got.code, 0, got.stderr);
  assert.equal(readFileSync(join(other.top, '.mutation/remote/full/incremental.json'), 'utf8'), readFileSync(join(r.top, '.mutation/incremental.json'), 'utf8'));
});

test('fetch-state removes a copy whose branch is gone', async () => {
  const r = runRepo();
  bareRemote(r);
  mkdirSync(join(r.top, '.mutation/remote/inc'), { recursive: true });
  writeFileSync(join(r.top, '.mutation/remote/inc/attestation.json'), '{}');
  const res = await cli(r, ['fetch-state']);
  assert.equal(res.code, 0);
  assert.equal(existsSync(join(r.top, '.mutation/remote/inc')), false);
  assert.match(res.stderr, /command=fetch-state invocations=0 scope=0 forced=0 deferrals=0 pending_full=true\n$/);
});

test('an unreachable or unknown remote is exit 2', async () => {
  const r = runRepo();
  r.git('remote', 'add', 'origin', join(tmpdir(), 'mi-no-such-remote.git'));
  const unreachable = await cli(r, ['fetch-state']);
  assert.equal(unreachable.code, 2);
  assert.match(unreachable.stderr, /fetch-state: git ls-remote origin failed/);
  const unknown = await cli(r, ['fetch-state', '--remote', 'nope']);
  assert.equal(unknown.code, 2);
  assert.match(unknown.stderr, /unknown git remote nope/);
});

test('a state branch without both files is exit 2', async () => {
  const r = runRepo();
  bareRemote(r);
  const git = (args, input) => execFileSync('git', args, { cwd: r.top, input, encoding: 'utf8' }).trim();
  const blob = git(['hash-object', '-w', '--stdin'], '{}');
  const tree = git(['mktree'], `100644 blob ${blob}\tattestation.json\n`);
  const commit = git(['commit-tree', tree, '-m', 'x']);
  r.git('push', '-q', 'origin', `${commit}:refs/heads/mutation-state/inc`);
  const res = await cli(r, ['fetch-state']);
  assert.equal(res.code, 2);
  assert.match(res.stderr, /fetch-state: reading incremental\.json from refs\/heads\/mutation-state\/inc failed/);
});

test('publish-state without usable state publishes nothing; a rejected push or a missing kind is exit 2', async () => {
  const r = runRepo();
  const bare = bareRemote(r);
  const none = await cli(r, ['publish-state', '--kind', 'inc']);
  assert.equal(none.code, 0);
  assert.match(none.stdout, /canonical state is not usable \(missing attestation\); nothing published/);
  await warm(r);
  writeFileSync(join(bare, 'hooks/pre-receive'), '#!/bin/sh\necho "refs are protected" >&2\nexit 1\n');
  chmodSync(join(bare, 'hooks/pre-receive'), 0o755);
  const rejected = await cli(r, ['publish-state', '--kind', 'inc']);
  assert.equal(rejected.code, 2);
  assert.match(rejected.stderr, /publish-state: push to origin failed: [\s\S]*refs are protected/);
  const noKind = await cli(r, ['publish-state']);
  assert.equal(noKind.code, 2);
  assert.match(noKind.stderr, /publish-state needs --kind inc\|full/);
});

test('with MUTATION_STATE_TOKEN under GitHub Actions the header value is masked first', async () => {
  const r = runRepo();
  bareRemote(r);
  const saved = { token: process.env.MUTATION_STATE_TOKEN, actions: process.env.GITHUB_ACTIONS };
  process.env.MUTATION_STATE_TOKEN = 'tok';
  process.env.GITHUB_ACTIONS = 'true';
  try {
    const res = await cli(r, ['fetch-state']);
    assert.equal(res.code, 0);
    assert.equal(res.stdout, `::add-mask::${Buffer.from('x-access-token:tok').toString('base64')}\n`);
  } finally {
    for (const [name, value] of [['MUTATION_STATE_TOKEN', saved.token], ['GITHUB_ACTIONS', saved.actions]]) {
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
  }
});
