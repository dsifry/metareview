// Package covergate is the pure coverage-gate logic ported from tests/coverage.sh (spec
// docs/specs/2026-09-01-covergate-go-native.md). It is deliberately free of process execution and
// filesystem globbing: it reads a `go tool covdata textfmt` profile and a floor file, and enforces the
// 16 invariants over those bytes. The Makefile owns orchestration (running the tests, merging coverage,
// resolving package lists via `go list`); this package owns parse + enforce + --update-floor, so the
// gate itself is covered Go rather than untested awk.
//
// The identity space is the package path relative to the module (e.g. "internal/jsonl"), the same key
// the floor file uses. Percentages are computed from statement counts, never scraped from
// `covdata percent` (whose output drops newlines for zero-statement packages — a documented trap).
package covergate

import (
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dsifry/metareview/internal/jsonl"
)

// PkgCov is one package's merged statement coverage.
type PkgCov struct {
	Covered int
	Total   int
}

// Pct is covered/total as a percentage. A zero-total package never reaches Pct — it emits no profile
// lines and so never enters the map (spec rule 6); callers key on presence, not on a percentage.
func (p PkgCov) Pct() float64 { return float64(p.Covered) / float64(p.Total) * 100 }

// pct1 rounds a percentage to one decimal place. The floor file stores 1-decimal values, and
// tests/coverage.sh compares the `%.1f`-formatted coverage against them; the gate must compare the same
// rounded value, or a package sitting exactly at its floor (raw 77.777…% vs floor 77.8) spuriously FAILs
// — a real divergence the parity run caught.
func pct1(v float64) float64 { return math.Round(v*10) / 10 }

// Exact reports whether every statement is covered (spec rule 7). Total is always > 0 here (a
// zero-statement package is absent from the map), so this never divides intent by an empty package.
func (p PkgCov) Exact() bool { return p.Total > 0 && p.Covered == p.Total }

// ParseProfile reads a `go tool covdata textfmt` profile and returns per-package coverage keyed by
// package path relative to module (spec rule 6). The first "mode:" line is skipped. Each data line is
// "<file>:<range> <numstmts> <count>"; a statement block counts toward Total, and toward Covered when
// its execution count is > 0.
func ParseProfile(r io.Reader, module string) (map[string]PkgCov, error) {
	prefix := strings.TrimSuffix(module, "/") + "/"
	out := map[string]PkgCov{}
	sc := jsonl.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		if line == 1 && strings.HasPrefix(text, "mode:") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 3 {
			return nil, fmt.Errorf("covergate: malformed profile line %d: %q", line, text)
		}
		filePart, _, ok := strings.Cut(fields[0], ":")
		if !ok {
			return nil, fmt.Errorf("covergate: profile line %d missing ':' in %q", line, fields[0])
		}
		rel := strings.TrimPrefix(filePart, prefix)
		if rel == filePart {
			return nil, fmt.Errorf("covergate: profile line %d file %q is not under module %q", line, filePart, module)
		}
		pkg := path.Dir(rel)
		stmts, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("covergate: profile line %d bad statement count %q: %w", line, fields[1], err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("covergate: profile line %d bad execution count %q: %w", line, fields[2], err)
		}
		c := out[pkg]
		c.Total += stmts
		if count > 0 {
			c.Covered += stmts
		}
		out[pkg] = c
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("covergate: reading profile: %w", err)
	}
	return out, nil
}

// ParseFloor reads tests/coverage-floor.txt: "<pkg> <pct>" lines, blank and #-comment lines ignored.
func ParseFloor(r io.Reader) (map[string]float64, error) {
	out := map[string]float64{}
	sc := jsonl.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("covergate: malformed floor line %d: %q", line, text)
		}
		pct, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return nil, fmt.Errorf("covergate: floor line %d bad percentage %q: %w", line, fields[1], err)
		}
		out[fields[0]] = pct
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("covergate: reading floor: %w", err)
	}
	return out, nil
}

// Row is one line of the report table.
type Row struct {
	Pkg    string
	Pct    string // "100.0", "81.2", or "absent"
	Status string
	Fail   bool
}

// GateInput bundles what Gate enforces over.
type GateInput struct {
	Profile    map[string]PkgCov  // per-package coverage (rule 6)
	Floor      map[string]float64 // pkg -> floor percentage
	Require100 []string           // packages that must be exactly 100% (resolved by the Makefile)
}

// Gate enforces rules 7–12 and returns the sorted report rows plus the failure count. It does not
// execute anything; the caller decides the exit code from failures. Require100 must be non-empty (a
// defensive form of rule 10: the Makefile only writes the require list when `go list` succeeded, and
// always appends cmd/covergate) — an empty list is a caller error signalled by a single failing row.
func Gate(in GateInput) (rows []Row, failures int) {
	req := map[string]bool{}
	for _, p := range in.Require100 {
		req[p] = true
	}

	// rule 7 / 8 / 9: every profile package (one with statements) is checked.
	for pkg, cov := range in.Profile {
		pctStr := strconv.FormatFloat(cov.Pct(), 'f', 1, 64)
		switch {
		case req[pkg]:
			if cov.Exact() {
				rows = append(rows, Row{pkg, pctStr, "ok", false})
			} else {
				rows = append(rows, Row{pkg, pctStr, "FAIL (requires every statement covered)", true})
				failures++
			}
		default:
			floor, has := in.Floor[pkg]
			// An if/else-if chain rather than a tagless `switch {}` so the guards are coverage-
			// instrumentable (Go's cover tool emits no counter for a tagless-switch case expression);
			// behaviour is identical.
			if !has {
				rows = append(rows, Row{pkg, pctStr, "FAIL (no floor: add a line to tests/coverage-floor.txt, or run --update-floor)", true})
				failures++
			} else if pct1(cov.Pct()) < floor {
				rows = append(rows, Row{pkg, pctStr, fmt.Sprintf("FAIL (floor %s)", strconv.FormatFloat(floor, 'f', -1, 64)), true})
				failures++
			} else {
				rows = append(rows, Row{pkg, pctStr, "ok", false})
			}
		}
	}

	// rule 11: every required package must be present in the profile.
	for _, pkg := range in.Require100 {
		if _, ok := in.Profile[pkg]; !ok {
			rows = append(rows, Row{pkg, "absent", "FAIL (required package not in profile)", true})
			failures++
		}
	}
	// rule 10 (defensive): the require set must never be empty.
	if len(in.Require100) == 0 {
		rows = append(rows, Row{"<require-100>", "absent", "FAIL (empty required set — gate would be a no-op)", true})
		failures++
	}

	// rule 12: every floored package must be present in the profile.
	for pkg, floor := range in.Floor {
		if _, ok := in.Profile[pkg]; !ok {
			rows = append(rows, Row{pkg, "absent", fmt.Sprintf("FAIL (floor %s but package not in profile)", strconv.FormatFloat(floor, 'f', -1, 64)), true})
			failures++
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Pkg < rows[j].Pkg })
	return rows, failures
}

// UpdateFloor implements rules 13–16: it regenerates the floor from measured packages (every package in
// the profile that is not in require100), refusing to write when that would lower an existing floor
// (unless allowDecrease) or drop a floored package that vanished from the profile. On success it returns
// the new floor map; on refusal it returns a non-nil error naming the offending packages.
func UpdateFloor(profile map[string]PkgCov, oldFloor map[string]float64, require100 []string, allowDecrease bool) (map[string]float64, error) {
	req := map[string]bool{}
	for _, p := range require100 {
		req[p] = true
	}

	// rule 16: a floored package absent from the profile must not be silently dropped.
	var vanished []string
	for pkg := range oldFloor {
		if _, ok := profile[pkg]; !ok {
			vanished = append(vanished, pkg)
		}
	}
	if len(vanished) > 0 {
		sort.Strings(vanished)
		return nil, fmt.Errorf("refusing to update the floor: floored package(s) absent from the profile: %s", strings.Join(vanished, ", "))
	}

	// rule 13: new floor = measured, non-required packages, at 1-decimal precision (the file's and the
	// gate's comparison precision — see pct1).
	newFloor := map[string]float64{}
	for pkg, cov := range profile {
		if req[pkg] {
			continue
		}
		newFloor[pkg] = pct1(cov.Pct())
	}

	// rule 14: refuse to lower an existing floor unless allowed.
	if !allowDecrease {
		var lowered []string
		for pkg, old := range oldFloor {
			if nf, ok := newFloor[pkg]; ok && nf < old {
				lowered = append(lowered, fmt.Sprintf("%s %s -> %s", pkg,
					strconv.FormatFloat(old, 'f', -1, 64), strconv.FormatFloat(nf, 'f', 1, 64)))
			}
		}
		if len(lowered) > 0 {
			sort.Strings(lowered)
			return nil, fmt.Errorf("refusing to lower the floor (pass --allow-floor-decrease to accept):\n%s", strings.Join(lowered, "\n"))
		}
	}
	return newFloor, nil
}

// FormatFloor renders a floor map as the file body (header + sorted "<pkg> <pct>" lines), the inverse of
// ParseFloor for the packages it manages.
func FormatFloor(floor map[string]float64) string {
	pkgs := make([]string, 0, len(floor))
	for p := range floor {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	var b strings.Builder
	b.WriteString("# Per-package combined coverage floor (unit + shell suite). Regenerate: make cover-update-floor\n")
	for _, p := range pkgs {
		fmt.Fprintf(&b, "%s %s\n", p, strconv.FormatFloat(floor[p], 'f', 1, 64))
	}
	return b.String()
}
