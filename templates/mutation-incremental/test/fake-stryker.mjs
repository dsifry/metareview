// Test double for `stryker run` (spec F4–F6), driven by .fake/steps.json in the cwd: one step per
// invocation (the last step repeats). A step may print `output`, write `touch` files, sleep
// `sleepMs` (ignoring SIGTERM with `ignoreTerm`), write `report` (an object, or "input" to rewrite
// the incremental file's own bytes) and exit with `exit` (default 0). Each call's argv is appended
// to .fake/calls.jsonl.
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';

const argv = process.argv.slice(2);
const log = '.fake/calls.jsonl';
const n = existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).length : 0;
appendFileSync(log, `${JSON.stringify(argv)}\n`);
const steps = JSON.parse(readFileSync('.fake/steps.json', 'utf8'));
const step = steps[Math.min(n, steps.length - 1)];
const file = argv[argv.indexOf('--incrementalFile') + 1];
if (step.ignoreTerm) process.on('SIGTERM', () => {});
if (step.output) process.stdout.write(step.output);
for (const [path, text] of Object.entries(step.touch ?? {})) writeFileSync(path, text);
await new Promise((resolve) => setTimeout(resolve, step.sleepMs ?? 0));
if (step.report === 'input') writeFileSync(file, readFileSync(file));
else if (step.report) writeFileSync(file, JSON.stringify(step.report));
process.exit(step.exit ?? 0);
