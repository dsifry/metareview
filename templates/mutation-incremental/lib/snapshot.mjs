import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { lstatSync, readFileSync, readlinkSync } from 'node:fs';
import { join } from 'node:path';
import { categorize, matchList } from './glob.mjs';
import { UsageError } from './errors.mjs';

export const sha256 = (data) => createHash('sha256').update(data).digest('hex');

export function git(top, args) {
  return execFileSync('git', args, {
    cwd: top,
    encoding: 'utf8',
    maxBuffer: 1 << 30,
    env: { ...process.env, GIT_LITERAL_PATHSPECS: '1' },
  });
}

const splitZ = (s) => s.split('\0').filter(Boolean);

// Spec §5.4: regular files by content, symlinks by link text (never followed), anything else skipped.
export function digestOf(absPath) {
  let st;
  try {
    st = lstatSync(absPath);
  } catch (e) {
    if (e.code === 'ENOENT') return null;
    throw e;
  }
  if (st.isSymbolicLink()) return `symlink:${sha256(readlinkSync(absPath))}`;
  if (st.isFile()) return `sha256:${sha256(readFileSync(absPath))}`;
  return null;
}

function listRepoPaths(top) {
  const all = splitZ(git(top, ['ls-files', '-c', '-o', '--exclude-standard', '--deduplicate', '-z']));
  const tracked = new Set();
  const gitlinks = new Set();
  for (const entry of splitZ(git(top, ['ls-files', '-c', '-s', '-z']))) {
    const path = entry.slice(entry.indexOf('\t') + 1);
    tracked.add(path);
    if (entry.startsWith('160000 ')) gitlinks.add(path);
  }
  return { paths: all.filter((p) => !gitlinks.has(p)), tracked };
}

function defaultRunCommand(argv, cwd) {
  try {
    return execFileSync(argv[0], argv.slice(1), { cwd });
  } catch (e) {
    throw new UsageError(`runtime command ${argv.join(' ')} failed: ${e.message}`);
  }
}

export function takeSnapshot(config, { runCommand = defaultRunCommand } = {}) {
  const { paths, tracked } = listRepoPaths(config.top);
  const files = {};
  for (const path of paths) {
    if (matchList(path, config.exclusions)) continue;
    const category = categorize(path, config.lists);
    if (category === 'ignore') continue;
    const digest = digestOf(join(config.top, path));
    if (digest === null) continue;
    files[path] = { digest, category, tracked: tracked.has(path) };
  }
  const runtime = {};
  for (const argv of config.runtime.commands) runtime[`cmd:${argv.join('\u001f')}`] = sha256(runCommand(argv, config.top));
  for (const name of config.runtime.env) {
    runtime[`env:${name}`] = process.env[name] === undefined ? 'unset' : sha256(process.env[name]);
  }
  return { files, runtime };
}
