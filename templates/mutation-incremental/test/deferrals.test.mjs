import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reasonInfo, classifyDeferrals, pendingCause, deferralKey } from '../lib/deferrals.mjs';

test('reasonInfo: every canonical reason and its named key', () => {
  const cases = [
    ['global input changed: package-lock.json', 'global', 'package-lock.json'],
    ['global input changed: env:DB', 'global', 'env:DB'],
    ['support t/h.ts has no resolvable importer', 'global', 't/h.ts'],
    ['support t/h.ts deleted', 'global', 't/h.ts'],
    ['unclassified s/x.sh has no resolvable importer', 'global', 's/x.sh'],
    ['no usable state', 'global', null],
    ['seeded from r.json', 'global', null],
    ['no reachable tests: src/c.ts', 'unreachable', 'src/c.ts'],
    ['residual off: src/a.ts', 'other', 'src/a.ts'],
    ['time budget exceeded', 'timeout', null],
    ['blocked by deferred scope', 'timeout', null],
    ['forced set 4 exceeds budget 1', 'timeout', null],
  ];
  for (const [reason, cause, named] of cases) assert.deepEqual(reasonInfo(reason), { cause, named }, reason);
  assert.throws(() => reasonInfo('mystery'), (e) => e.exitCode === 2 && /unknown deferral reason/.test(e.message));
});

const snapshot = { files: { 'package-lock.json': { digest: 'sha256:new' }, 'src/c.ts': { digest: 'sha256:c' } }, runtime: { 'env:DB': 'd1' } };
const main = (deferrals, files = { 'package-lock.json': { digest: 'sha256:new' }, 'src/c.ts': { digest: 'sha256:c' } }, runtime = { 'env:DB': 'd1' }) => ({ deferrals, files, runtime });
const lock = { reason: 'global input changed: package-lock.json', paths: ['*'] };

test('classify: identical deferral with unchanged inputs is inherited', () => {
  const [d] = classifyDeferrals({ deferrals: [lock], alsoAttestations: [main([lock])], snapshot });
  assert.equal(d.inherited, true);
});

test('classify: identical deferral with a changed digest is counted', () => {
  const [d] = classifyDeferrals({ deferrals: [lock], alsoAttestations: [main([lock], { 'package-lock.json': { digest: 'sha256:old' } })], snapshot });
  assert.equal(d.inherited, false);
});

test('classify: runtime keys, absent on both sides, nameless reasons', () => {
  const env = { reason: 'global input changed: env:DB', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [main([env])], snapshot })[0].inherited, true);
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [main([env], undefined, { 'env:DB': 'd0' })], snapshot })[0].inherited, false);
  const gone = { reason: 'support t/gone.ts deleted', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [gone], alsoAttestations: [main([gone])], snapshot })[0].inherited, true);
  const nus = { reason: 'no usable state', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [nus], alsoAttestations: [main([nus], {}, {})], snapshot })[0].inherited, true);
  const perFile = { reason: 'no reachable tests: src/c.ts', paths: ['src/c.ts'] };
  assert.equal(classifyDeferrals({ deferrals: [perFile], alsoAttestations: [main([perFile], { 'src/c.ts': { digest: 'sha256:other' } })], snapshot })[0].inherited, false);
  // An unusable main attestation may lack files or runtime: its digests read as absent.
  assert.equal(classifyDeferrals({ deferrals: [perFile], alsoAttestations: [{ deferrals: [perFile] }], snapshot })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [env], alsoAttestations: [{ deferrals: [env] }], snapshot })[0].inherited, false);
});

test('classify: cold no-usable-state and deferrals produced by this run are always counted', () => {
  const nus = { reason: 'no usable state', paths: ['*'] };
  assert.equal(classifyDeferrals({ deferrals: [nus], alsoAttestations: [main([nus])], snapshot, cold: true })[0].inherited, false);
  const tb = { reason: 'time budget exceeded', paths: ['src/c.ts'] };
  const produced = new Set([deferralKey(tb)]);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [main([tb])], snapshot, producedKeys: produced })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [main([tb])], snapshot })[0].inherited, true);
  assert.equal(classifyDeferrals({ deferrals: [tb], snapshot })[0].inherited, false);
  assert.equal(classifyDeferrals({ deferrals: [tb], alsoAttestations: [{}], snapshot })[0].inherited, false);
});

test('pendingCause precedence: global > unreachable > other > timeout > none', () => {
  const c = (reason, paths, inherited = false) => ({ reason, paths, inherited });
  const cause = (...ds) => pendingCause(ds).cause;
  assert.equal(cause(), 'none');
  assert.equal(cause(c('global input changed: x', ['*'], true)), 'none');
  assert.equal(cause(c('time budget exceeded', ['a']), c('no usable state', ['*'])), 'global');
  assert.equal(cause(c('time budget exceeded', ['a']), c('no reachable tests: b', ['b'])), 'unreachable');
  assert.equal(cause(c('time budget exceeded', ['a']), c('residual off: b', ['c'])), 'other');
  assert.equal(cause(c('forced set 4 exceeds budget 1', ['a'])), 'timeout');
  assert.deepEqual(pendingCause([c('blocked by deferred scope', ['a']), c('x', ['*'], true)]).counted, [c('blocked by deferred scope', ['a'])]);
  assert.throws(() => pendingCause([c('mystery', ['a'])]), (e) => e.exitCode === 2);
});
