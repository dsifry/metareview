import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';

// Test repositories live under os.tmpdir(), which loadConfig refuses for real state; allow it here
// so a bare `node --test` works without the variable (the Go wrapper also sets it).
process.env.MUTATION_ALLOW_TMP_STATE ??= '1';

export const baseConfig = () => ({
  schemaVersion: 1,
  stateDir: '.mutation',
  stryker: { command: [process.execPath, 'fake-stryker.mjs'], configFile: 'stryker.config.json', extraArgs: [] },
  mutate: ['src/**/*.ts'],
  test: ['tests/**/*.test.ts'],
  support: ['tests/helpers/**'],
  global: ['package.json'],
  ignore: ['**/*.md'],
  aliases: {},
  runtime: { commands: [], env: [] },
  budget: { maxForcedShare: 1, maxForcedMutants: null, maxMinutesPerInvocation: 5 },
  residual: { mode: 'bounded' },
});

// A throwaway git repo with a valid harness config, a JSON Stryker config and a fake
// @stryker-mutator/core install, so loadConfig passes without real Stryker.
export function makeRepo({ config = {}, stryker = {}, files = {}, base = tmpdir() } = {}) {
  const top = mkdtempSync(join(base, 'mi-'));
  const git = (...args) => execFileSync('git', args, { cwd: top, encoding: 'utf8' });
  const write = (rel, text) => {
    mkdirSync(dirname(join(top, rel)), { recursive: true });
    writeFileSync(join(top, rel), text);
  };
  git('init', '-q');
  git('config', 'user.email', 't@example.com');
  git('config', 'user.name', 'T');
  const cfg = { ...baseConfig(), ...config };
  write('mutation-incremental.json', JSON.stringify(cfg));
  write('stryker.config.json', JSON.stringify({ testRunner: 'vitest', mutate: cfg.mutate, ignorePatterns: [cfg.stateDir], disableBail: true, ...stryker }));
  write('node_modules/@stryker-mutator/core/package.json', JSON.stringify({ version: '10.0.0' }));
  write('.gitignore', 'node_modules/\n.mutation/\n');
  for (const [rel, text] of Object.entries(files)) write(rel, text);
  git('add', '-A');
  git('commit', '-q', '-m', 'init');
  return { top, git, write };
}

// Writes a usable state pair into dir: the report bytes and a minimal attestation that
// readCandidate accepts. Attestation writing proper is Plan 1b's job.
export function writeState(dir, { report, attestation = {} }) {
  mkdirSync(dir, { recursive: true });
  const text = JSON.stringify(report);
  writeFileSync(join(dir, 'incremental.json'), text);
  const att = {
    schemaVersion: 1,
    tool: 'metareview-mutation-incremental',
    stateVersion: 1,
    reportSha256: createHash('sha256').update(text).digest('hex'),
    files: {},
    runtime: {},
    deferrals: [],
    ...attestation,
  };
  writeFileSync(join(dir, 'attestation.json'), JSON.stringify(att));
  return att;
}
