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
