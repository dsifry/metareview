// Glob dialect shared with metareview's Go gate (spec §5.3). Both implementations run
// testdata/mutation-incremental/glob-vectors.json, so a change here needs the same change there.
const SEG = '(?!\\.)[^/]+';
const compiled = new Map();

function segment(seg, atStart) {
  let out = '';
  for (let i = 0; i < seg.length; i++) {
    const c = seg[i];
    const start = atStart && i === 0;
    if (c === '*') {
      out += `${start ? '(?!\\.)' : ''}[^/]*`;
      while (seg[i + 1] === '*') i++;
    } else if (c === '?') {
      out += `${start ? '(?!\\.)' : ''}[^/]`;
    } else if (c === '{' && seg.indexOf('}', i) > i) {
      const end = seg.indexOf('}', i);
      const alternatives = seg.slice(i + 1, end).split(',');
      out += `(?:${alternatives.map((alt) => segment(alt, start)).join('|')})`;
      i = end;
    } else {
      out += c.replace(/[.+^$()|[\]\\{}]/g, '\\$&');
    }
  }
  return out;
}

export function compileGlob(pattern) {
  const p = pattern.startsWith('./') ? pattern.slice(2) : pattern;
  const cached = compiled.get(p);
  if (cached) return cached;
  const segs = p.split('/');
  let src = '';
  let needSep = false;
  segs.forEach((seg, i) => {
    const last = i === segs.length - 1;
    if (seg === '**') {
      if (last) src += i === 0 ? `(?:${SEG}(?:/${SEG})*)?` : `(?:/${SEG})*`;
      else src += `${needSep ? '/' : ''}(?:${SEG}/)*`;
      needSep = false;
    } else {
      src += `${needSep ? '/' : ''}${segment(seg, true)}`;
      needSep = true;
    }
  });
  const re = new RegExp(`^${src}$`);
  compiled.set(p, re);
  return re;
}

export function matchList(path, list) {
  let hit = false;
  for (const entry of list) {
    if (entry.startsWith('!')) {
      if (compileGlob(entry.slice(1)).test(path)) return false;
    } else if (!hit) {
      hit = compileGlob(entry).test(path);
    }
  }
  return hit;
}

export const CATEGORY_ORDER = ['global', 'support', 'test', 'mutate'];

export function categorize(path, lists) {
  for (const name of CATEGORY_ORDER) {
    if (matchList(path, lists[name])) return name;
  }
  return matchList(path, lists.ignore) ? 'ignore' : 'unclassified';
}
