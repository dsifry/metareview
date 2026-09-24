import { appendFileSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { EngineError, InterruptedError, UsageError } from './errors.mjs';
import { loadHarnessConfig, planInputs, summaryLine } from './inputs.mjs';
import { computePlan } from './plan.mjs';
import { classifyDeferrals, dedupeDeferrals, deferralKey, pendingCause, routeFor, sortDeferrals } from './deferrals.mjs';
import { resolveViews, viewSummaries } from './views.mjs';
import { acquireLock } from './lock.mjs';
import { buildAttestation, commitState, installAdopted, markChangedDuringRun, WORK_DIR } from './attest.mjs';
import { invoke, NO_TESTS, parseReport } from './engine.mjs';
import { runVerify } from './verify.mjs';
import { createInterrupt, installSignalHandlers, KILL_GRACE_MS } from './proc.mjs';
import { canonicalPendingFull, REPORT_FILE } from './state.mjs';

export const MAX_CAUSES = 50;

// Forced entries are `<file>` or `<file>:<start>-<end>`; paths never contain ':' (the planner exits 2).
const fileOf = (entry) => entry.split(':')[0];

function checkOptions(options) {
  if (options.mode !== 'incremental' && options.mode !== 'full') throw new UsageError('run needs --mode incremental|full');
  if (options.pr && options.mode === 'full') throw new UsageError('--pr marks an incremental PR run; the sweep (--mode full) is never --pr');
  if (options.maxMinutes === undefined) return null;
  if (!options.pr) throw new UsageError('--max-minutes applies only to --pr runs');
  const minutes = Number(options.maxMinutes);
  if (!(minutes > 0)) throw new UsageError('--max-minutes must be a positive number');
  return minutes;
}

// Spec §5.1: appended on every exit path when GITHUB_OUTPUT is set.
function writeOutputs(entries) {
  const file = process.env.GITHUB_OUTPUT;
  if (!file) return;
  appendFileSync(file, entries.map(([key, value]) => `${key}=${value}\n`).join(''));
}

// Spec §11.2: the counted {reason, paths} as single-line JSON, at most MAX_CAUSES entries followed
// by {"more": n}. JSON.stringify escapes control characters; paths never contain a newline.
export function causesJSON(counted) {
  const list = counted.map((d) => ({ reason: d.reason, paths: d.paths }));
  const shown = list.length > MAX_CAUSES ? [...list.slice(0, MAX_CAUSES), { more: list.length - MAX_CAUSES }] : list;
  return JSON.stringify(shown);
}

function readOutput(path) {
  if (!existsSync(path)) return null;
  const bytes = readFileSync(path);
  const report = parseReport(bytes);
  return report === null ? null : { path, bytes, report };
}

// Spec §5.6 step 5: selected files with kills lose their killing tests; files without kills (type-only
// files, a new module before its first test) record nothing.
function noReachableDeferrals(files, report) {
  return files
    .filter((f) => (report.files[f]?.mutants ?? []).some((m) => m.status === 'Killed'))
    .map((f) => ({ reason: `no reachable tests: ${f}`, paths: [f] }));
}

async function executeIncremental(ctx) {
  const { config, inputs, interrupt } = ctx;
  const { snapshot, graph, candidates, chosen } = inputs;
  // Spec §11.2 (i): main's fetched states count whether or not they are usable.
  const alsoAttestations = candidates.filter((c) => !c.primary && c.attestation !== null).map((c) => c.attestation);
  if (chosen !== null && !chosen.primary) installAdopted(config.stateDir, chosen.dir);
  const plan = computePlan({ config, snapshot, graph, chosen, unbudgeted: ctx.routed, disableResidual: process.env.MUTATION_TEST_DISABLE_RESIDUAL === '1' });
  // Counted/inherited at plan time decides the shortcut (spec §11.2; review advisory).
  const planned = classifyDeferrals({ deferrals: plan.deferrals, alsoAttestations, snapshot, cold: plan.cold });
  const shortcut = ctx.routed && pendingCause(planned).cause === 'global';
  let latest = readOutput(join(config.stateDir, REPORT_FILE));
  const produced = [];
  let invocations = 0;
  if (!plan.cold && !shortcut) {
    const timeoutMs = (ctx.maxMinutes ?? config.budget.maxMinutesPerInvocation) * 60_000;
    let blocked = false;
    const steps = [{ list: plan.invocation1, force: false }, { list: plan.invocation2, force: true }];
    for (const [i, { list, force }] of steps.entries()) {
      if (list.length === 0) continue;
      const files = [...new Set(list.map(fileOf))].sort();
      if (blocked) {
        produced.push({ reason: 'blocked by deferred scope', paths: files });
        continue;
      }
      const r = await invoke(config, { index: i + 1, input: latest.path, force, mutate: list, timeoutMs, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
      invocations++;
      if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
      if (r.outcome === 'failed') throw new EngineError(`stryker invocation ${i + 1} failed (${r.detail}); nothing committed`);
      if (r.outcome === 'success') latest = r;
      else if (r.outcome === 'timeout') {
        produced.push({ reason: 'time budget exceeded', paths: files });
        blocked = true;
      } else produced.push(...noReachableDeferrals(files, latest.report));
    }
  }
  const deferrals = sortDeferrals(dedupeDeferrals(plan.deferrals, produced));
  const producedKeys = new Set(produced.map(deferralKey));
  return {
    plan,
    latest,
    invocations,
    mode: 'incremental',
    lastFullAt: chosen === null ? null : chosen.attestation.lastFullAt,
    classified: classifyDeferrals({ deferrals, alsoAttestations, snapshot, producedKeys, cold: plan.cold }),
  };
}

async function executeFull(ctx) {
  const r = await invoke(ctx.config, { index: 1, input: null, force: false, mutate: null, timeoutMs: null, interrupt: ctx.interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput });
  if (r.outcome === 'interrupted') throw new InterruptedError('interrupted; nothing committed');
  // A full run that finds no tests (e.g. a dependency bump broke every import) is never attested.
  if (r.outcome === 'no-tests') throw new EngineError(`the full run found no tests (${NO_TESTS}); nothing committed`);
  if (r.outcome === 'failed') throw new EngineError(`the full run failed (${r.detail}); nothing committed`);
  return { plan: null, latest: r, invocations: 1, mode: 'full', lastFullAt: null, classified: [] };
}

// Spec §5.6 steps 6–7. A cold run with no incremental.json commits nothing.
function commit(ctx, result) {
  const { config, inputs, views } = ctx;
  if (result.latest === null) return null;
  const completedAt = new Date().toISOString();
  const attestation = buildAttestation({
    config,
    files: markChangedDuringRun(config, inputs.snapshot.files),
    runtime: inputs.snapshot.runtime,
    reportBytes: result.latest.bytes,
    report: result.latest.report,
    mode: result.mode,
    completedAt,
    lastFullAt: result.mode === 'full' ? completedAt : result.lastFullAt,
    deferrals: result.classified,
    views,
    summaries: viewSummaries(result.latest.report, views, result.classified),
  });
  commitState(config.stateDir, result.latest.path, attestation);
  return attestation;
}

// Spec §5.1 `run`, §5.6, §11.1, §11.2.
export async function runCommand(io, options) {
  const interrupt = createInterrupt();
  const uninstall = installSignalHandlers(interrupt);
  let config = null;
  let release = null;
  let exitCode = 0;
  try {
    const maxMinutes = checkOptions(options);
    ({ config } = loadHarnessConfig(io, options, true));
    mkdirSync(config.stateDir, { recursive: true });
    release = acquireLock(config.stateDir);
    const work = join(config.stateDir, WORK_DIR);
    rmSync(work, { recursive: true, force: true });
    mkdirSync(work);
    const inputs = planInputs(config.top, config, options.alsoState);
    // Spec §11.3: view completeness is checked at plan time, before any invocation.
    const views = resolveViews(config, inputs.snapshot);
    // Spec §11.2: --pr and --max-minutes take effect only when pendingOnPr routes PRs; under allow a
    // PR run is an ordinary run (budgeted, MUTATION_RUN_KIND=other).
    const routed = options.pr && config.pendingOnPr !== 'allow';
    const ctx = { config, inputs, views, options, interrupt, routed, maxMinutes: routed ? maxMinutes : null, graceMs: io.killGraceMs ?? KILL_GRACE_MS, onOutput: (chunk) => io.stderr.write(chunk) };
    const result = options.mode === 'full' ? await executeFull(ctx) : await executeIncremental(ctx);
    const attestation = commit(ctx, result);
    const failures = [];
    if (attestation !== null) {
      if (attestation.thresholdBreak) failures.push(`score ${attestation.score} is below thresholds.break ${config.thresholdBreak}`);
      failures.push(...(await runVerify(config, { views, snapshot: inputs.snapshot, pr: routed, interrupt, graceMs: ctx.graceMs, onOutput: ctx.onOutput })));
    }
    if (interrupt.interrupted) throw new InterruptedError('interrupted; the state is committed');
    for (const f of failures) io.stderr.write(`mutation-incremental: ${f}\n`);
    const code = failures.length > 0 ? 1 : 0;
    const pendingFull = attestation === null || result.classified.length > 0;
    const { cause, counted } = pendingCause(result.classified);
    writeOutputs([['pending_full', pendingFull], ['exit_code', code], ['pending_cause', cause], ['pending_causes', causesJSON(counted)], ['route', routeFor(config.pendingOnPr, cause, options.pr)]]);
    io.stderr.write(summaryLine({
      command: 'run',
      invocations: result.invocations,
      scope: result.plan?.scope.length ?? 0,
      forced: result.plan?.forced.length ?? 0,
      deferrals: result.classified.length,
      pendingFull,
    }));
    exitCode = code;
  } catch (e) {
    writeOutputs([['pending_full', config === null ? false : canonicalPendingFull(config)], ['exit_code', Number.isInteger(e.exitCode) ? e.exitCode : 4]]);
    throw e;
  } finally {
    // Only the lock holder may touch work/: on exit 3 it belongs to the other run.
    if (release !== null) {
      rmSync(join(config.stateDir, WORK_DIR), { recursive: true, force: true });
      release();
    }
    uninstall();
  }
  return exitCode;
}
