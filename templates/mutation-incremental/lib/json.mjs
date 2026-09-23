// Deterministic JSON so attestations and plans byte-compare across runs and machines.
function sortKeys(value) {
  if (Array.isArray(value)) return value.map(sortKeys);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, sortKeys(value[key])]));
  }
  return value;
}

export function canonicalJSON(value) {
  return `${JSON.stringify(sortKeys(value), null, 2)}\n`;
}

// JSON.parse keeps the last of two duplicate keys silently; the spec treats a duplicate view name
// (and any duplicate config key) as an error. Call only on text that JSON.parse already accepted.
export function duplicateKey(text) {
  const stack = [];
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    if (c === '"') {
      let j = i + 1;
      while (text[j] !== '"') j += text[j] === '\\' ? 2 : 1;
      const top = stack[stack.length - 1];
      if (top !== undefined && top.object && top.expectKey) {
        const key = JSON.parse(text.slice(i, j + 1));
        if (top.keys.has(key)) return key;
        top.keys.add(key);
        top.expectKey = false;
      }
      i = j + 1;
      continue;
    }
    if (c === '{') stack.push({ object: true, keys: new Set(), expectKey: true });
    else if (c === '[') stack.push({ object: false });
    else if (c === '}' || c === ']') stack.pop();
    else if (c === ',') stack[stack.length - 1].expectKey = true;
    i++;
  }
  return null;
}
