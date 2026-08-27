#!/usr/bin/env bash
# Combined coverage gate: unit tests + the behavioral shell suite, merged via `go tool covdata`.
#
#   bash tests/coverage.sh                        # run the full suite under coverage and enforce the gate
#   bash tests/coverage.sh --update-floor         # ... then raise tests/coverage-floor.txt (never lowers a line)
#   bash tests/coverage.sh --update-floor --allow-floor-decrease   # explicitly accept a lower floor
#
# Gate:
#   - every package in `go list ./internal/fsm/... ./workflows` must be present in the merged profile
#     and report 100.0% of statements
#   - every other package must not drop below its line in tests/coverage-floor.txt
#     (packages absent from the floor file are reported but not enforced)
#
# Per-package percentages are computed from the textfmt profile (statement counts), not parsed from
# `go tool covdata percent`, whose output drops newlines for zero-statement packages.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

UPDATE_FLOOR=false
ALLOW_DECREASE=false
for arg in "$@"; do
  case "$arg" in
    --update-floor) UPDATE_FLOOR=true ;;
    --allow-floor-decrease) ALLOW_DECREASE=true ;;
    *) echo "Unknown option: $arg" >&2; exit 2 ;;
  esac
done

FLOOR_FILE="$ROOT/tests/coverage-floor.txt"
COVDIR="$(mktemp -d)"
PROFILE="$COVDIR/profile.txt"
# The shell suite runs `npm run build`, which under GOFLAGS=-cover would leave an instrumented
# bin/metareview behind; rebuild it plainly on exit.
cleanup() { rm -rf "$COVDIR"; go build -o bin/metareview ./cmd/metareview; }
trap cleanup EXIT

# 1. Unit tests, instrumented, writing per-package counters into COVDIR.
# Output is captured rather than discarded: a failure here used to surface in CI as
# a bare "exit 1" with nothing to diagnose.
if ! go test -cover -covermode=atomic ./... -args -test.gocoverdir="$COVDIR" > "$COVDIR/unit.log" 2>&1; then
  echo "coverage gate: unit tests failed" >&2
  cat "$COVDIR/unit.log" >&2
  exit 1
fi

# 2. Behavioral shell suite. GOFLAGS=-cover instruments every `go run` / `go build` the scripts
#    perform; GOCOVERDIR makes the resulting binaries emit counters into the same directory.
if ! GOFLAGS="-cover -covermode=atomic" GOCOVERDIR="$COVDIR" bash tests/run-all.sh > "$COVDIR/suite.log" 2>&1; then
  echo "coverage gate: behavioral suite failed" >&2
  tail -80 "$COVDIR/suite.log" >&2
  exit 1
fi

# 3. Merge into a textfmt profile and compute per-package statement coverage.
go tool covdata textfmt -i="$COVDIR" -o "$PROFILE"
MODULE="$(go list -m)"

# profile lines: <file>:<start>,<end> <numstmts> <count>   (first line is "mode: ...")
# package = directory of <file>, relative to the module path.
# Columns: <pkg> <pct(%.1f)> <exact: 1 if covered == total else 0>
PCTS="$(awk -v mod="$MODULE/" 'NR > 1 {
  split($1, a, ":"); f = a[1]; if (index(f, mod) == 1) f = substr(f, length(mod) + 1); sub(/\/[^\/]*$/, "", f)
  total[f] += $2; if ($3 > 0) covered[f] += $2
} END {
  for (p in total) printf "%s %.1f %d\n", p, (covered[p] / total[p]) * 100, (covered[p] == total[p])
}' "$PROFILE" | sort)"

failures=0
table=""
check_pkg() {
  local rel="$1" pct="$2" exact="$3" status="ok"
  case "$rel" in
    internal/fsm/*|workflows)
      if [ "$exact" != "1" ]; then status="FAIL (requires every statement covered)"; failures=$((failures + 1)); fi
      ;;
    *)
      local floor
      floor="$(awk -v p="$rel" '$1 == p {print $2}' "$FLOOR_FILE" 2>/dev/null || true)"
      if [ -n "$floor" ] && awk -v a="$pct" -v b="$floor" 'BEGIN{exit !(a < b)}'; then
        status="FAIL (floor $floor)"; failures=$((failures + 1))
      elif [ -z "$floor" ]; then
        status="no floor"
      fi
      ;;
  esac
  table+="$(printf '%-48s %7s%%  %s\n' "$rel" "$pct" "$status")"$'\n'
}

while read -r rel pct exact; do
  [ -n "$rel" ] || continue
  check_pkg "$rel" "$pct" "$exact"
done <<< "$PCTS"

# Every required package must be present in the profile (a package with no executed statements,
# or with no statements at all, would otherwise be silently absent). The required set itself must
# be non-empty: a moved tree or a `go list` failure must not turn the gate into a no-op.
REQUIRED="$(go list ./internal/fsm/... ./workflows 2>/dev/null || true)"
if [ -z "$REQUIRED" ]; then
  if [ -d internal/fsm ] || [ -d workflows ]; then
    echo "coverage gate: required packages could not be listed (go list failed)" >&2; exit 1
  fi
  echo "coverage gate: no FSM packages yet; enforcing legacy floor only" >&2
fi
for pkg in $REQUIRED; do
  rel="${pkg#"$MODULE"/}"
  if ! printf '%s\n' "$PCTS" | awk -v p="$rel" '$1 == p {found=1} END {exit !found}'; then
    table+="$(printf '%-48s %8s  %s\n' "$rel" 'absent' 'FAIL (required package not in profile)')"$'\n'
    failures=$((failures + 1))
  fi
done

# Every legacy package with a floor line must still be present in the profile (a package that
# vanishes from the profile must not silently escape its floor).
if [ -f "$FLOOR_FILE" ]; then
  while read -r fpkg fpct; do
    case "$fpkg" in ''|\#*) continue ;; esac
    if ! printf '%s\n' "$PCTS" | awk -v p="$fpkg" '$1 == p {found=1} END {exit !found}'; then
      table+="$(printf '%-48s %8s  %s\n' "$fpkg" 'absent' "FAIL (floor $fpct but package not in profile)")"$'\n'
      failures=$((failures + 1))
    fi
  done < "$FLOOR_FILE"
fi

printf '%s' "$table"

if [ "$UPDATE_FLOOR" = true ]; then
  new_floor="$(printf '%s\n' "$PCTS" | awk '$1 !~ /^internal\/fsm\// && $1 != "workflows" {print $1, $2}')"
  if [ "$ALLOW_DECREASE" = false ] && [ -f "$FLOOR_FILE" ]; then
    lowered="$(printf '%s\n' "$new_floor" | awk -v floor="$FLOOR_FILE" '
      BEGIN { while ((getline line < floor) > 0) { split(line, f, " "); if (f[1] !~ /^#/) old[f[1]] = f[2] } }
      ($1 in old) && ($2 < old[$1]) { printf "%s %s -> %s\n", $1, old[$1], $2 }')"
    if [ -n "$lowered" ]; then
      echo "refusing to lower the floor (pass --allow-floor-decrease to accept):" >&2
      printf '%s\n' "$lowered" >&2
      exit 1
    fi
  fi
  {
    echo "# Per-package combined coverage floor (unit + shell suite). Regenerate: bash tests/coverage.sh --update-floor"
    printf '%s\n' "$new_floor"
  } > "$FLOOR_FILE"
  echo "floor updated: $FLOOR_FILE"
fi

if [ "$failures" -gt 0 ]; then
  echo "coverage gate FAILED ($failures package(s))" >&2
  exit 1
fi
echo "coverage gate passed"
