import { existsSync, readFileSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { isAbsolute, join, relative, resolve } from 'node:path';
import { UsageError } from './errors.mjs';
import { matchList } from './glob.mjs';
import { duplicateKey } from './json.mjs';

const REQUIRED = ['aliases', 'budget', 'global', 'ignore', 'mutate', 'residual', 'runtime', 'schemaVersion', 'stateDir', 'stryker', 'support', 'test'];
const OPTIONAL = ['allowBail', 'editedFiles', 'pendingOnPr', 'verify', 'views'];
const LIST_KEYS = ['mutate', 'test', 'support', 'global', 'ignore'];
export const TOOL_DIR_GLOB = 'tools/mutation-incremental/**';

const fail = (message) => {
  throw new UsageError(`config: ${message}`);
};
const toPosix = (p) => p.split('\\').join('/');
const inside = (root, p) => {
  const rel = relative(root, p);
  return rel === '' || (!rel.startsWith('..') && !isAbsolute(rel));
};
const isArgv = (a) => Array.isArray(a) && a.length > 0 && a.every((s) => typeof s === 'string');
const positive = (n) => typeof n === 'number' && n > 0;
const isObject = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

function readJSON(path, what) {
  let text;
  let value;
  try {
    text = readFileSync(path, 'utf8');
    value = JSON.parse(text);
  } catch (e) {
    return fail(`${what}: ${e.message}`);
  }
  const dup = duplicateKey(text);
  if (dup !== null) fail(`${what}: duplicate key ${JSON.stringify(dup)}`);
  return value;
}

function validateOptional(raw) {
  const editedFiles = raw.editedFiles ?? 'residual';
  if (!['residual', 'whole'].includes(editedFiles)) fail('editedFiles must be residual or whole');
  const pendingOnPr = raw.pendingOnPr ?? 'allow';
  if (!['allow', 'full', 'full-on-global'].includes(pendingOnPr)) fail('pendingOnPr must be allow, full or full-on-global');
  const allowBail = raw.allowBail ?? false;
  if (typeof allowBail !== 'boolean') fail('allowBail must be a boolean');
  const verify = raw.verify ?? null;
  if (verify !== null) {
    const ok = isObject(verify)
      && Object.keys(verify).every((k) => k === 'command' || k === 'timeoutMinutes')
      && isArgv(verify.command)
      && (verify.timeoutMinutes === undefined || verify.timeoutMinutes === null || positive(verify.timeoutMinutes));
    if (!ok) fail('verify must be {"command": [argv], "timeoutMinutes": <positive number or null>}');
    // Spec §11.2: a job-level timeout must never pre-empt the harness, so the verifier is bounded.
    if (pendingOnPr !== 'allow' && !positive(verify.timeoutMinutes)) fail('verify.timeoutMinutes is required when pendingOnPr is not allow');
  }
  const views = raw.views ?? null;
  if (views !== null) {
    const keys = isObject(views) ? Object.keys(views) : [];
    const ok = keys.length === 1 && ((keys[0] === 'command' && isArgv(views.command)) || (keys[0] === 'inline' && isObject(views.inline)));
    if (!ok) fail('views must be {"command": [argv]} or {"inline": {name: [patterns]}}');
  }
  return { editedFiles, pendingOnPr, allowBail, verify: verify === null ? null : { command: verify.command, timeoutMinutes: verify.timeoutMinutes ?? null }, views };
}

export function loadConfig(top, configArg) {
  const configPath = resolve(top, configArg ?? 'mutation-incremental.json');
  if (!existsSync(configPath)) fail(`${configPath} not found`);
  const raw = readJSON(configPath, `cannot parse ${configPath}`);
  if (!isObject(raw)) fail('must be a JSON object');
  for (const key of REQUIRED) if (!(key in raw)) fail(`missing key ${key}`);
  for (const key of Object.keys(raw)) if (!REQUIRED.includes(key) && !OPTIONAL.includes(key)) fail(`unknown key ${key}`);
  if (raw.schemaVersion !== 1) fail('schemaVersion must be 1');
  for (const key of LIST_KEYS) {
    if (!Array.isArray(raw[key]) || !raw[key].every((s) => typeof s === 'string')) fail(`${key} must be an array of strings`);
  }
  const aliases = raw.aliases;
  if (!isObject(aliases) || !Object.values(aliases).every((v) => typeof v === 'string')) fail('aliases must map strings to strings');
  const mode = raw.residual?.mode;
  if (!['strict', 'bounded', 'off'].includes(mode)) fail('residual.mode must be strict, bounded or off');
  const budget = raw.budget ?? {};
  if (!(typeof budget.maxForcedShare === 'number' && budget.maxForcedShare > 0 && budget.maxForcedShare <= 1)) fail('budget.maxForcedShare must be in (0, 1]');
  if (!(budget.maxForcedMutants === null || (Number.isInteger(budget.maxForcedMutants) && budget.maxForcedMutants > 0))) {
    fail('budget.maxForcedMutants must be a positive integer or null');
  }
  if (!positive(budget.maxMinutesPerInvocation)) fail('budget.maxMinutesPerInvocation must be positive');
  const stryker = raw.stryker ?? {};
  if (!isArgv(stryker.command) || typeof stryker.configFile !== 'string' || !Array.isArray(stryker.extraArgs)) {
    fail('stryker needs command[], configFile and extraArgs[]');
  }
  const runtime = raw.runtime ?? {};
  if (!Array.isArray(runtime.commands) || !runtime.commands.every(isArgv) || !Array.isArray(runtime.env) || !runtime.env.every((n) => typeof n === 'string')) {
    fail('runtime needs commands[] (argv arrays) and env[] (variable names)');
  }
  const optional = validateOptional(raw);
  if (mode === 'off' && optional.pendingOnPr === 'full-on-global') fail('residual.mode off cannot be combined with pendingOnPr full-on-global');

  const stateDir = resolve(top, raw.stateDir);
  if (!inside(top, stateDir) || stateDir === resolve(top)) fail('stateDir must be inside the repository');
  if (!process.env.MUTATION_ALLOW_TMP_STATE) {
    for (const t of [tmpdir(), '/tmp']) {
      if (inside(realpathSync(t), realpathSync(top))) fail('stateDir must not be under the OS temp directory (it is purged)');
    }
  }
  const stateRel = toPosix(relative(top, stateDir));
  for (const probe of [stateRel, `${stateRel}/attestation.json`, `${stateRel}/incremental.json`]) {
    for (const key of ['mutate', 'test', 'support']) {
      if (matchList(probe, raw[key])) fail(`stateDir ${stateRel} must not match a ${key} pattern`);
    }
  }

  const sc = readJSON(resolve(top, stryker.configFile), 'stryker.configFile must be readable JSON');
  if (sc.testRunner !== 'vitest') fail('stryker config must use testRunner "vitest"');
  if (sc.inPlace === true) fail('stryker config must not set inPlace');
  if (JSON.stringify(sc.mutate) !== JSON.stringify(raw.mutate)) fail('stryker config mutate must equal config mutate');
  if (!Array.isArray(sc.ignorePatterns) || !sc.ignorePatterns.some((p) => p === stateRel || p === `${stateRel}/**`)) {
    fail(`stryker config ignorePatterns must contain ${stateRel}`);
  }
  const disableBail = sc.disableBail === true;
  if (!disableBail && !optional.allowBail) fail('stryker config must set disableBail: true (or set allowBail: true)');
  if (optional.allowBail && optional.editedFiles === 'residual') fail('allowBail is refused with editedFiles "residual" (its closure reads killedBy)');
  const corePkg = join(top, 'node_modules/@stryker-mutator/core/package.json');
  if (!existsSync(corePkg)) fail('@stryker-mutator/core is not installed');
  const engineVersion = readJSON(corePkg, 'cannot read @stryker-mutator/core').version;
  if (!String(engineVersion).startsWith('10.')) fail(`StrykerJS 10.x required, found ${engineVersion}`);

  const configRel = toPosix(relative(top, configPath));
  return {
    top,
    configPath,
    stateDir,
    stateDirRaw: raw.stateDir,
    stateRel,
    lists: { mutate: raw.mutate, test: raw.test, support: raw.support, global: [...raw.global, configRel, TOOL_DIR_GLOB], ignore: raw.ignore },
    exclusions: [
      `${stateRel}/**`,
      `${sc.tempDirName ?? '.stryker-tmp'}/**`,
      sc.jsonReporter?.fileName ?? 'reports/mutation/mutation.json',
      sc.htmlReporter?.fileName ?? 'reports/mutation/mutation.html',
    ],
    aliases,
    stryker,
    engineVersion,
    budget,
    residualMode: mode,
    runtime,
    ...optional,
    thresholdBreak: typeof sc.thresholds?.break === 'number' ? sc.thresholds.break : null,
    disableBail,
  };
}
