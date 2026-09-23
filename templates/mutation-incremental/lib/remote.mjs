import { execFileSync, spawnSync } from 'node:child_process';
import { mkdirSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadHarnessConfig, summaryLine } from './inputs.mjs';
import { ATTESTATION_FILE, REPORT_FILE, canonicalPendingFull, primaryCandidate } from './state.mjs';

export const KINDS = ['inc', 'full'];
const AUTHOR = { GIT_AUTHOR_NAME: 'mutation-incremental', GIT_AUTHOR_EMAIL: 'noreply@localhost', GIT_COMMITTER_NAME: 'mutation-incremental', GIT_COMMITTER_EMAIL: 'noreply@localhost' };
const branchRef = (kind) => `refs/heads/mutation-state/${kind}`;

// Spec §5.7: the token travels as an http extraheader through GIT_CONFIG_* (never argv), appended
// after entries already in the environment. An empty or unset token means git's own credentials.
export function authEnv(env, url, token) {
  if (!token) return { env, mask: null };
  const mask = Buffer.from(`x-access-token:${token}`).toString('base64');
  const n = Number(env.GIT_CONFIG_COUNT ?? 0);
  return {
    env: { ...env, GIT_CONFIG_COUNT: String(n + 1), [`GIT_CONFIG_KEY_${n}`]: `http.${url}.extraheader`, [`GIT_CONFIG_VALUE_${n}`]: `AUTHORIZATION: basic ${mask}` },
    mask,
  };
}

function remoteEnv(io, top, remote) {
  const url = spawnSync('git', ['remote', 'get-url', remote], { cwd: top, encoding: 'utf8' });
  if (url.status !== 0) throw new UsageError(`unknown git remote ${remote}`);
  const { env, mask } = authEnv(process.env, url.stdout.trim(), process.env.MUTATION_STATE_TOKEN);
  if (mask !== null && process.env.GITHUB_ACTIONS) io.stdout.write(`::add-mask::${mask}\n`);
  return env;
}

// A failed ls-remote, fetch, show or push is exit 2 (spec §5.7: network and auth failures are visible).
function must(result, what, command) {
  if (result.status !== 0) throw new UsageError(`${command}: ${what} failed: ${String(result.stderr).trim()}`);
  return result.stdout;
}

// Spec §5.7: list the state branches (a missing one is skipped), fetch each present one without
// --depth, and write its two files into <stateDir>/remote/<kind>/. A kind's old copy is removed
// first, so a branch that is gone leaves nothing to adopt.
export async function fetchStateCommand(io, options) {
  const { top, config } = loadHarnessConfig(io, options, false);
  const remote = options.remote ?? 'origin';
  const env = remoteEnv(io, top, remote);
  const listing = must(spawnSync('git', ['ls-remote', remote, 'refs/heads/mutation-state/*'], { cwd: top, env, encoding: 'utf8' }), `git ls-remote ${remote}`, 'fetch-state');
  const present = new Set(listing.split('\n').filter(Boolean).map((line) => line.split('\t')[1]));
  for (const kind of KINDS) {
    const dir = join(config.stateDir, 'remote', kind);
    rmSync(dir, { recursive: true, force: true });
    if (!present.has(branchRef(kind))) {
      io.stderr.write(`fetch-state: ${branchRef(kind)} is not on ${remote}; skipped\n`);
      continue;
    }
    must(spawnSync('git', ['fetch', '--no-tags', remote, branchRef(kind)], { cwd: top, env, encoding: 'utf8' }), `git fetch ${branchRef(kind)}`, 'fetch-state');
    mkdirSync(dir, { recursive: true });
    for (const name of [ATTESTATION_FILE, REPORT_FILE]) {
      // Reports embed every mutated file's source: far beyond spawnSync's 1 MiB default buffer.
      const bytes = must(spawnSync('git', ['show', `FETCH_HEAD:${name}`], { cwd: top, env, maxBuffer: 1 << 30 }), `reading ${name} from ${branchRef(kind)}`, 'fetch-state');
      writeFileSync(join(dir, `${name}.tmp`), bytes);
      renameSync(join(dir, `${name}.tmp`), join(dir, name));
    }
  }
  io.stderr.write(summaryLine({ command: 'fetch-state', pendingFull: canonicalPendingFull(config) }));
  return 0;
}

// Spec §5.7: a parentless commit holding the canonical pair, built with plumbing and force-pushed
// to mutation-state/<kind>. Only a usable state is published; otherwise it says why and exits 0.
export async function publishStateCommand(io, options) {
  if (!KINDS.includes(options.kind)) throw new UsageError('publish-state needs --kind inc|full');
  const { top, config } = loadHarnessConfig(io, options, false);
  const remote = options.remote ?? 'origin';
  const state = primaryCandidate(config);
  if (!state.usable) {
    io.stdout.write(`publish-state: canonical state is not usable (${state.reason}); nothing published\n`);
  } else {
    const env = remoteEnv(io, top, remote);
    const plumb = (args, input, extra) => execFileSync('git', args, { cwd: top, env: { ...env, ...extra }, input, encoding: 'utf8' }).trim();
    const blob = (name) => plumb(['hash-object', '-w', join(config.stateDir, name)], undefined, {});
    const tree = plumb(['mktree'], `100644 blob ${blob(ATTESTATION_FILE)}\t${ATTESTATION_FILE}\n100644 blob ${blob(REPORT_FILE)}\t${REPORT_FILE}\n`, {});
    const commit = plumb(['commit-tree', tree, '-m', `mutation-state ${options.kind}`], undefined, AUTHOR);
    must(spawnSync('git', ['push', '--force', remote, `${commit}:${branchRef(options.kind)}`], { cwd: top, env, encoding: 'utf8' }), `push to ${remote}`, 'publish-state');
    io.stdout.write(`publish-state: pushed ${commit} to ${branchRef(options.kind)}\n`);
  }
  io.stderr.write(summaryLine({ command: 'publish-state', pendingFull: canonicalPendingFull(config) }));
  return 0;
}
