// Content-based line diff (spec §11.4.1), shared with the Go gate. Both implementations run
// testdata/mutation-incremental/diff-vectors.json, so a change here needs the same change there.
// Above MAX_EDIT_DISTANCE lineDiff returns null and the planner forces the file whole.
export const MAX_EDIT_DISTANCE = 2000;

export function normalizeText(text) {
  const noBom = text.startsWith('\ufeff') ? text.slice(1) : text;
  return noBom.replace(/\r\n?/g, '\n');
}

export function splitLines(text) {
  const t = normalizeText(text);
  if (t === '') return [];
  const lines = t.split('\n');
  if (lines[lines.length - 1] === '') lines.pop();
  return lines;
}

// Myers' O(ND) shortest edit script. Ties take the insertion ("down") move when k === -d or
// v[k-1] < v[k+1], otherwise the deletion ("right") move, so the result is deterministic.
// trace[d] keeps v for diagonals -d-1 .. d+1 as it was before step d.
function editOps(a, b) {
  const n = a.length;
  const m = b.length;
  const off = n + m + 1;
  const v = new Int32Array(2 * (n + m) + 3);
  const trace = [];
  for (let d = 0; d <= MAX_EDIT_DISTANCE; d++) {
    trace.push(v.slice(off - d - 1, off + d + 2));
    for (let k = -d; k <= d; k += 2) {
      let x = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]) ? v[off + k + 1] : v[off + k - 1] + 1;
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) {
        x++;
        y++;
      }
      v[off + k] = x;
      if (x >= n && y >= m) return backtrack(trace, n, m);
    }
  }
  return null;
}

function backtrack(trace, n, m) {
  const ops = [];
  let x = n;
  let y = m;
  for (let d = trace.length - 1; d >= 0; d--) {
    const w = trace[d];
    const at = (k) => w[k + d + 1];
    const k = x - y;
    const prevK = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
    const prevX = at(prevK);
    const prevY = prevX - prevK;
    while (x > prevX && y > prevY) {
      ops.push('=');
      x--;
      y--;
    }
    if (d > 0) ops.push(x === prevX ? '+' : '-');
    x = prevX;
    y = prevY;
  }
  return ops.reverse();
}

export function lineDiff(oldText, newText) {
  const a = splitLines(oldText);
  const b = splitLines(newText);
  const ops = editOps(a, b);
  if (ops === null) return null;
  const hunks = [];
  const oldToNew = [null];
  let i = 0;
  let j = 0;
  let cur = null;
  for (const op of ops) {
    if (op === '=') {
      i++;
      j++;
      oldToNew[i] = j;
      cur = null;
      continue;
    }
    if (cur === null) {
      cur = { oldStart: i + 1, oldEnd: i, newStart: j + 1, newEnd: j };
      hunks.push(cur);
    }
    if (op === '-') {
      i++;
      cur.oldEnd = i;
      oldToNew[i] = null;
    } else {
      j++;
      cur.newEnd = j;
    }
  }
  return { hunks, oldToNew };
}

// Spec §11.4.3: a mutant [start, end] (old lines) intersects a changed hunk when the ranges overlap,
// and a pure insertion when it contains the insertion point (strictly inside the mutant's span).
export function hunkIntersects(start, end, h) {
  if (h.oldEnd >= h.oldStart) return start <= h.oldEnd && end >= h.oldStart;
  const after = h.oldStart - 1;
  return start <= after && end >= after + 1;
}
