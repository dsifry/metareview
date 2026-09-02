// Command covergate is the coverage gate: it reads a `go tool covdata textfmt` profile and the floor
// file and enforces metareview's coverage invariants (spec docs/specs/2026-09-01-covergate-go-native.md).
// All logic lives in internal/covergate (unit-tested to 100%); this main is a one-line delegate so there
// is nothing here to get wrong. The Makefile invokes it after merging coverage.
package main

import (
	"os"

	"github.com/dsifry/metareview/internal/covergate"
)

func main() {
	os.Exit(covergate.Run(os.Args[1:], os.Stdout, os.Stderr))
}
