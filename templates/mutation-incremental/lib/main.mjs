import { UsageError } from './errors.mjs';
import { canonicalJSON } from './json.mjs';
import { computePlan, publicPlan } from './plan.mjs';
import { resolveViews } from './views.mjs';
import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
import { breakLockCommand } from './lock.mjs';
import { runCommand } from './run.mjs';
import { seedCommand } from './seed.mjs';
import { fetchStateCommand, publishStateCommand } from './remote.mjs';

const USAGE = `usage: cli.mjs <command> [--config <path>]
  plan [--also-state <dir>]...
  run --mode incremental|full [--also-state <dir>]... [--pr [--max-minutes <m>]]
  seed --from <report>... [--replace]
  fetch-state [--remote <name>]
  publish-state --kind inc|full [--remote <name>]
  break-lock`;

const VALUE_OPTIONS = { '--config': 'config', '--mode': 'mode', '--max-minutes': 'maxMinutes', '--remote': 'remote', '--kind': 'kind' };
const LIST_OPTIONS = { '--also-state': 'alsoState', '--from': 'from' };
const FLAG_OPTIONS = { '--pr': 'pr', '--replace': 'replace' };

// One option grammar for every command; each command reads the options it uses (spec §5.1).
export function parseArgs(args) {
  const out = { command: args[0], config: undefined, mode: undefined, maxMinutes: undefined, remote: undefined, kind: undefined, alsoState: [], from: [], pr: false, replace: false };
  for (let i = 1; i < args.length; i++) {
    const a = args[i];
    if (Object.hasOwn(FLAG_OPTIONS, a)) {
      out[FLAG_OPTIONS[a]] = true;
      continue;
    }
    const single = Object.hasOwn(VALUE_OPTIONS, a);
    if (!single && !Object.hasOwn(LIST_OPTIONS, a)) throw new UsageError(`unknown option ${a}`);
    if (i + 1 >= args.length) throw new UsageError(`${a} needs a value`);
    const value = args[++i];
    if (single) out[VALUE_OPTIONS[a]] = value;
    else out[LIST_OPTIONS[a]].push(value);
  }
  return out;
}

function planCommand(io, options) {
  const { top, config } = loadHarnessConfig(io, options, true);
  const { snapshot, graph, chosen } = planInputs(top, config, options.alsoState);
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

const COMMANDS = {
  plan: planCommand,
  run: runCommand,
  seed: seedCommand,
  'fetch-state': fetchStateCommand,
  'publish-state': publishStateCommand,
  'break-lock': breakLockCommand,
};

export async function main(args, io = { stdout: process.stdout, stderr: process.stderr, cwd: process.cwd() }) {
  try {
    const options = parseArgs(args);
    const command = COMMANDS[options.command];
    if (!command) throw new UsageError(options.command === undefined ? USAGE : `unknown command ${options.command}\n${USAGE}`);
    return await command(io, options);
  } catch (e) {
    // Known failures carry their exit code (2, 3, 4, 130). Anything else is an internal error and
    // exits 4: for run, exit 1 means "ok, state committed", so a crash must never produce it.
    const known = Number.isInteger(e.exitCode);
    io.stderr.write(`mutation-incremental: ${known ? 'error' : 'internal error'}: ${e.message}\n`);
    io.stderr.write(summaryLine({ command: args[0] ?? 'none' }));
    return known ? e.exitCode : 4;
  }
}
