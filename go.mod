module github.com/dsifry/metareview

go 1.26

// Two directives, two jobs. `go 1.26` is what a consumer needs: the npm package
// ships cmd/ and internal/ and falls back to `go run ./cmd/metareview`, so this
// line is the oldest Go someone can install with. `toolchain` is what we build
// and measure with, and it is deliberately newer.
//
// The pin matters because Go releases do not agree with each other. encoding/json
// writes U+2028/U+2029 and invalid UTF-8 differently between 1.26 and 1.27 — which
// broke the canonical audit encoding — and coverage percentages differ by a few
// tenths, so an unpinned gate produces floors that pass locally and fail on CI.
toolchain go1.27.0

require gopkg.in/yaml.v3 v3.0.1
