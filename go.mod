module github.com/dsifry/metareview

go 1.26

// Pinned so that local runs and CI measure the same thing. encoding/json
// changed its treatment of U+2028/U+2029 and of invalid UTF-8 between 1.26 and
// 1.27, and coverage percentages differ by a few tenths between them, so an
// unpinned toolchain meant the coverage floors and the canonical-encoding tests
// could pass locally and fail on the runner.
toolchain go1.26.7

require gopkg.in/yaml.v3 v3.0.1
