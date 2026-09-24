import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { loadConfig } from './config.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { buildGraph } from './graph.mjs';
import { adopt, primaryCandidate, readCandidate } from './state.mjs';
import { nodeVersionWarning } from './nodever.mjs';

// Spec §5.1: every command prints exactly one summary line on stderr.
export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;

function topLevel(cwd) {
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    throw new UsageError('not inside a git repository');
  }
}

// Every command: the git top-level and the validated config (spec §5.1). plan and run also print
// the Node-version warning.
export function loadHarnessConfig(io, options, warn) {
  const top = topLevel(io.cwd);
  const config = loadConfig(top, options.config);
  const warning = warn ? nodeVersionWarning(top) : null;
  if (warning) io.stderr.write(`${warning}\n`);
  return { top, config };
}

// Everything planning needs, with adoption in memory only (spec §5.4).
export function planInputs(top, config, alsoState) {
  const snapshot = takeSnapshot(config);
  const graph = buildGraph(snapshot.files, config);
  const candidates = [primaryCandidate(config), ...alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false }))];
  return { snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
}
