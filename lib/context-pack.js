'use strict';

const fs = require('fs');
const path = require('path');
const childProcess = require('child_process');
const { createRunId } = require('./state');
const { detectRepo } = require('./repo-detect');
const { inlineCode, normalizeMarkdownInline, plainText } = require('./markdown');

function readIfExists(filePath) {
  try {
    const stat = fs.statSync(filePath);
    if (!stat.isFile()) return '';
    fs.accessSync(filePath, fs.constants.R_OK);
    return fs.readFileSync(filePath, 'utf8');
  } catch (_error) {
    return '';
  }
}

function isPathInsideRoot(root, filePath) {
  try {
    const realRootPath = fs.realpathSync(path.resolve(root));
    const realFilePath = fs.realpathSync(filePath);
    return realFilePath === realRootPath || realFilePath.startsWith(`${realRootPath}${path.sep}`);
  } catch (_error) {
    return false;
  }
}

function readRepoFileIfExists(root, filePath) {
  if (!isPathInsideRoot(root, filePath)) return '';
  return readIfExists(filePath);
}

function fencedCodeBlock(language, content) {
  const backtickRuns = String(content).match(/`+/g) || [];
  const longestRun = backtickRuns.reduce((max, run) => Math.max(max, run.length), 0);
  const fence = '`'.repeat(Math.max(3, longestRun + 1));
  return `${fence}${language}\n${content}\n${fence}`;
}

function listKnowledge(root) {
  const dir = path.join(root, '.beads', 'knowledge');
  try {
    if (!fs.statSync(dir).isDirectory()) return [];
  } catch (_error) {
    return [];
  }

  let files;
  try {
    files = fs.readdirSync(dir);
  } catch (_error) {
    return [];
  }

  return files
    .filter(file => file.endsWith('.jsonl'))
    .flatMap(file => {
      const full = path.join(dir, file);
      return readRepoFileIfExists(root, full)
        .split('\n')
        .filter(Boolean)
        .slice(0, 5)
        .map(line => {
          const displayFile = normalizeMarkdownInline(file);
          try {
            const parsed = JSON.parse(line);
            return `- ${displayFile}: ${normalizeMarkdownInline(parsed.fact || parsed.recommendation || parsed.id || line)}`;
          } catch {
            return `- ${displayFile}: ${normalizeMarkdownInline(line)}`;
          }
        });
    });
}

function gitSummary(root) {
  try {
    const head = childProcess.execSync('git rev-parse --short HEAD', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
    const branch = childProcess.execSync('git branch --show-current', { cwd: root, stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
    return { head, branch };
  } catch {
    return { head: 'unavailable', branch: 'unavailable' };
  }
}

function assertTargetInsideRoot(root, target) {
  const rootPath = path.resolve(root);
  const targetPath = path.resolve(rootPath, target);
  if (targetPath !== rootPath && !targetPath.startsWith(`${rootPath}${path.sep}`)) {
    throw new Error(`Target artifact is outside repository root: ${target}`);
  }
  if (!fs.existsSync(targetPath)) {
    throw new Error(`Target artifact not found: ${target}`);
  }
  const stat = fs.statSync(targetPath);
  if (!stat.isFile()) {
    throw new Error(`Target artifact is not a regular file: ${target}`);
  }
  const realRootPath = fs.realpathSync(rootPath);
  const realTargetPath = fs.realpathSync(targetPath);
  if (realTargetPath !== realRootPath && !realTargetPath.startsWith(`${realRootPath}${path.sep}`)) {
    throw new Error(`Target artifact is outside repository root: ${target}`);
  }
  return targetPath;
}

function buildContextPack(root, target, options = {}) {
  const now = options.now || new Date();
  const runId = createRunId('artifact', target, now);
  const targetPath = assertTargetInsideRoot(root, target);
  const titleTarget = plainText(target);

  const report = detectRepo(root);
  const git = gitSummary(root);
  const serviceInventoryPath = report.files.serviceInventory;
  const serviceInventoryDisplayPath = serviceInventoryPath ? normalizeMarkdownInline(serviceInventoryPath) : null;
  const serviceInventory = serviceInventoryPath
    ? readRepoFileIfExists(root, path.join(root, serviceInventoryPath)).slice(0, 2000)
    : '';
  const knowledge = listKnowledge(root);
  const artifact = readIfExists(targetPath).slice(0, 4000);

  const contextRel = `docs/metareview/context/${runId}-context.md`;
  const outputPath = path.join(root, contextRel);
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });

  const content = `# metareview context: ${titleTarget}

Run ID: ${inlineCode(runId)}

## Target

- Path: ${inlineCode(target)}
- Repository mode: ${inlineCode(report.mode)}
- Git branch: ${inlineCode(git.branch)}
- Git head: ${inlineCode(git.head)}

## Artifact Excerpt

${fencedCodeBlock('markdown', artifact)}

## Service Inventory

${serviceInventoryDisplayPath ? `Source: ${inlineCode(serviceInventoryDisplayPath)}\n\n${fencedCodeBlock('markdown', serviceInventory)}` : 'No service inventory found.'}

## Knowledge Facts

${knowledge.length ? knowledge.join('\n') : 'No Beads knowledge facts found.'}

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
`;

  fs.writeFileSync(outputPath, content);
  return { runId, contextPath: contextRel };
}

module.exports = { buildContextPack };
