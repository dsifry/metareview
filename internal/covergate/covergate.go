// Package covergate is the pure coverage-gate logic (spec docs/specs/2026-09-01-covergate-go-native.md).
// It is deliberately free of process execution and filesystem globbing: it reads a `go tool covdata
// textfmt` profile and a resolved required-package list, and enforces that every required package is at
// exactly 100% statement coverage. The Makefile owns orchestration (running the tests, merging coverage,
// resolving the package list via `go list ./...` minus the explicit exclude list); this package owns
// parse + enforce, so the gate itself is covered Go rather than untested awk.
//
// The identity space is the package path relative to the module (e.g. "internal/jsonl"). Percentages are
// computed from statement counts, never scraped from `covdata percent` (whose output drops newlines for
// zero-statement packages — a documented trap).
//
// History: this gate was floor-based (a per-package ratcheting floor in tests/coverage-floor.txt) until
// the repo-wide 100% campaign brought every package to the bar; the floor mechanism and the sibling bash
// gate (tests/coverage.sh) were then removed in favour of require-100-for-all-minus-an-exclude-list.
package covergate

import (
	"fmt"
	"io"
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
// lines and so never enters the map; callers key on presence, not on a percentage.
func (p PkgCov) Pct() float64 { return float64(p.Covered) / float64(p.Total) * 100 }

// Exact reports whether every statement is covered. Total is always > 0 here (a zero-statement package is
// absent from the map), so this never divides intent by an empty package.
func (p PkgCov) Exact() bool { return p.Total > 0 && p.Covered == p.Total }

// ParseProfile reads a `go tool covdata textfmt` profile and returns per-package coverage keyed by
// package path relative to module. The first "mode:" line is skipped. Each data line is
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

// Row is one line of the report table.
type Row struct {
	Pkg    string
	Pct    string // "100.0", "81.2", or "absent"
	Status string
	Fail   bool
}

// GateInput bundles what Gate enforces over.
type GateInput struct {
	Profile  map[string]PkgCov // per-package coverage
	Required []string          // every package that must be exactly 100% (go list ./... minus the exclude list, resolved by the Makefile)
}

// Gate enforces the require-100 gate and returns the sorted report rows plus the failure count. It does
// not execute anything; the caller decides the exit code from failures.
//
//   - Every required package must be present in the profile at exactly 100% (else FAIL). A required
//     package absent from the profile is an untested/unmeasured package — the fail-closed check that once
//     caught internal/mutation shipping an entire subsystem ungated. Because the required set is `go list
//     ./...` minus the explicit exclude list, a new package is required by default and must be covered.
//   - A profile package that is NOT required is an explicitly-excluded package (the Makefile subtracts the
//     exclude list before writing the required set); it is reported for transparency but not gated.
//   - Required must be non-empty: a `go list` failure must not turn the gate into a no-op.
func Gate(in GateInput) (rows []Row, failures int) {
	req := map[string]bool{}
	for _, p := range in.Required {
		req[p] = true
	}

	// Every package with statements in the profile: required ones must be exactly 100%; the rest are
	// explicitly excluded and reported without gating.
	for pkg, cov := range in.Profile {
		pctStr := strconv.FormatFloat(cov.Pct(), 'f', 1, 64)
		if !req[pkg] {
			rows = append(rows, Row{pkg, pctStr, "ok (excluded)", false})
		} else if cov.Exact() {
			rows = append(rows, Row{pkg, pctStr, "ok", false})
		} else {
			rows = append(rows, Row{pkg, pctStr, "FAIL (requires every statement covered)", true})
			failures++
		}
	}

	// Every required package must be present in the profile (the fail-closed ungated-package check).
	for _, pkg := range in.Required {
		if _, ok := in.Profile[pkg]; !ok {
			rows = append(rows, Row{pkg, "absent", "FAIL (required package not in profile)", true})
			failures++
		}
	}
	// The required set must never be empty.
	if len(in.Required) == 0 {
		rows = append(rows, Row{"<require-100>", "absent", "FAIL (empty required set — gate would be a no-op)", true})
		failures++
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Pkg < rows[j].Pkg })
	return rows, failures
}
