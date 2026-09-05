# metareview build + coverage gate (spec docs/specs/2026-09-01-covergate-go-native.md).
#
# `make cover` is the coverage gate: it runs the unit tests and the behavioral shell suite under coverage,
# merges them, and hands the profile to cmd/covergate (the tested Go gate). It requires EVERY package
# `go list ./...` reports to be at exactly 100.0% statement coverage, except the packages named in
# tests/coverage-exclude.txt. The whole cover recipe is ONE backslash-continued shell line so COVDIR and
# the EXIT trap share a single shell without relying on .ONESHELL (macOS ships GNU Make 3.81, which
# ignores it).

# Build with the go.mod toolchain pins, not whatever the developer happens to have — coverage percentages
# differ by a few tenths between Go releases.
export GOTOOLCHAIN := $(shell awk '$$1=="toolchain"{print $$2}' go.mod)
MODULE := $(shell go list -m)

.PHONY: build cover test

build:
	go build -o bin/metareview ./cmd/metareview

cover:
	@set -e; \
	COVDIR=$$(mktemp -d); \
	trap 'rm -rf "$$COVDIR"; go build -o bin/metareview ./cmd/metareview || echo "cover: post-run rebuild of bin/metareview FAILED" >&2' EXIT; \
	go test -cover -covermode=atomic ./... -args -test.gocoverdir="$$COVDIR"; \
	GOFLAGS="-cover -covermode=atomic" GOCOVERDIR="$$COVDIR" bash tests/run-all.sh; \
	go tool covdata textfmt -i="$$COVDIR" -o "$$COVDIR/profile.txt"; \
	if pkgs=$$(go list ./... 2>"$$COVDIR/golist.err"); then :; else echo "cover: go list ./... failed:" >&2; cat "$$COVDIR/golist.err" >&2; exit 1; fi; \
	grep -vE '^[[:space:]]*(#|$$)' tests/coverage-exclude.txt > "$$COVDIR/exclude.txt" || true; \
	printf '%s\n' "$$pkgs" | grep -vxF -f "$$COVDIR/exclude.txt" | sort -u > "$$COVDIR/require100.txt"; \
	go run ./cmd/covergate --profile "$$COVDIR/profile.txt" \
	  --module "$(MODULE)" --require-100 "$$COVDIR/require100.txt"

test: cover
