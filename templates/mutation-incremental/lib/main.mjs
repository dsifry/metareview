import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { loadConfig } from './config.mjs';
import { takeSnapshot } from './snapshot.mjs';
import { buildGraph } from './graph.mjs';
import { readCandidate, adopt } from './state.mjs';
import { computePlan, publicPlan } from './plan.mjs';
import { resolveViews } from './views.mjs';
import { nodeVersionWarning } from './nodever.mjs';

const USAGE = 'usage: cli.mjs plan [--config <path>] [--also-state <dir>]...';

export function parseArgs(args) {
  const out = { command: args[0], config: undefined, alsoState: [], rest: [] };
  for (let i = 1; i < args.length; i++) {
    const a = args[i];
    const value = () => {
      if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
      return args[++i];
    };
    if (a === '--config') out.config = value();
    else if (a === '--also-state') out.alsoState.push(value());
    else throw new UsageError(`unknown option ${a}`);
  }
  return out;
}

export const summaryLine = ({ command, invocations = 0, scope = 0, forced = 0, deferrals = 0, pendingFull = false }) =>
  `mutation-incremental: command=${command} invocations=${invocations} scope=${scope} forced=${forced} deferrals=${deferrals} pending_full=${pendingFull}\n`;

function topLevel(cwd) {
  try {
    return execFileSync('git', ['rev-parse', '--show-toplevel'], { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    throw new UsageError('not inside a git repository');
  }
}

// Shared with Plan 1b's run command: everything planning needs, with adoption in memory only.
export function loadPlanInputs(io, options) {
  const top = topLevel(io.cwd);
  const config = loadConfig(top, options.config);
  const warning = nodeVersionWarning(top);
  if (warning) io.stderr.write(`${warning}\n`);
  const snapshot = takeSnapshot(config);
  const graph = buildGraph(snapshot.files, config);
  const candidates = [
    readCandidate(config.stateDir, { label: config.stateDirRaw, primary: true }),
    ...options.alsoState.map((dir) => readCandidate(resolve(top, dir), { label: dir, primary: false })),
  ];
  return { top, config, snapshot, graph, candidates, chosen: adopt(candidates, snapshot) };
}

function planCommand(io, options) {
  const { config, snapshot, graph, chosen } = loadPlanInputs(io, options);
  const plan = computePlan({ config, snapshot, graph, chosen });
  resolveViews(config, snapshot);
  io.stdout.write(canonicalJSON(publicPlan(plan)));
  io.stderr.write(summaryLine({
    command: 'plan',
    invocations: Number(plan.invocation1.length > 0) + Number(plan.invocation2.length > 0),
    scope: plan.scope.length,
    forced: plan.forced.length,
    deferrals: plan.deferrals.length,
    pendingFull: plan.pendingFull,
  }));
  return 0;
}

const COMMANDS = { plan: planCommand };

export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
  try {
    const options = parseArgs(args);
    const command = COMMANDS[options.command];
    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}; ${USAGE}`);
    return await command(io, options);
  } catch (e) {
    // Exit 1 means "ok, state committed" for run (Plan 1b), so an internal crash is exit 4.
    const usage = e instanceof UsageError;
    io.stderr.write(`mutation-incremental: ${usage ? 'error' : 'internal error'}: ${e.message}\n`);
    io.stderr.write(summaryLine({ command: args[0] ?? 'none' }));
    return usage ? e.exitCode : 4;
  }
}
