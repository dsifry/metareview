#!/usr/bin/env bash
# Regenerates oracle.jsonl and oracle.sha256 from oracle.template.jsonl using an external sha256
# implementation (never Go). Each line's "prev" is the sha256 of the previous emitted line's exact
# bytes (without the newline); seq 1 has an empty prev. Run manually; commit the outputs.
set -euo pipefail
cd "$(dirname "$0")"
if command -v sha256sum >/dev/null 2>&1; then SHA="sha256sum"; else SHA="shasum -a 256"; fi
prev=""
: > oracle.jsonl
: > oracle.sha256
while IFS= read -r line; do
  emitted="${line/@PREV@/$prev}"
  printf '%s\n' "$emitted" >> oracle.jsonl
  prev="$(printf '%s' "$emitted" | $SHA | awk '{print $1}')"
  echo "$prev" >> oracle.sha256
done < oracle.template.jsonl
