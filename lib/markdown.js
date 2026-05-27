'use strict';

function normalizeMarkdownInline(value) {
  return String(value).replace(/[\r\n]+/g, ' ').replace(/\s+/g, ' ').trim();
}

function inlineCode(value) {
  const normalized = normalizeMarkdownInline(value);
  const backtickRuns = normalized.match(/`+/g) || [];
  const longestRun = backtickRuns.reduce((max, run) => Math.max(max, run.length), 0);
  const delimiter = '`'.repeat(Math.max(1, longestRun + 1));
  const needsPadding = normalized.startsWith('`') || normalized.endsWith('`');
  const content = needsPadding ? ` ${normalized} ` : normalized;
  return `${delimiter}${content}${delimiter}`;
}

function plainText(value) {
  return normalizeMarkdownInline(value).replace(/`+/g, '').trim();
}

module.exports = {
  inlineCode,
  normalizeMarkdownInline,
  plainText
};
