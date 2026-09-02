# metareview build + coverage gate (spec docs/specs/2026-09-01-covergate-go-native.md).
#
# `make cover` is the Go-native replacement for `bash tests/coverage.sh`: it runs the unit tests and the
# behavioral shell suite under coverage, merges them, and hands the profile to cmd/covergate (the tested
# Go gate). The whole cover recipe is ONE backslash-continued shell line so COVDIR and the EXIT trap
# share a single shell without relying on .ONESHELL (macOS ships GNU Make 3.81, which ignores it).
#
# During the transition both this and tests/coverage.sh run in CI and must agree; once proven equivalent
# tests/coverage.sh is deleted (spec §3).

# Build with the go.mod toolchain pins, not whatever the developer happens to have — coverage percentages
# differ by a few tenths between Go releases (tests/coverage.sh rule 1).
export GOTOOLCHAIN := $(shell awk '$$1=="toolchain"{print $$2}' go.mod)
MODULE := $(shell go list -m)

.PHONY: build cover cover-update-floor test

build:
	go build -o bin/metareview ./cmd/metareview

cover:
	@set -e; \
	COVDIR=$$(mktemp -d); \
	trap 'rm -rf "$$COVDIR"; go build -o bin/metareview ./cmd/metareview || echo "cover: post-run rebuild of bin/metareview FAILED" >&2' EXIT; \
	go test -cover -covermode=atomic ./... -args -test.gocoverdir="$$COVDIR"; \
	GOFLAGS="-cover -covermode=atomic" GOCOVERDIR="$$COVDIR" bash tests/run-all.sh; \
	go tool covdata textfmt -i="$$COVDIR" -o "$$COVDIR/profile.txt"; \
	if req=$$(go list ./internal/fsm/... ./workflows 2>&1); then :; \
	elif [ -d internal/fsm ] || [ -d workflows ]; then echo "cover: go list of the required set failed:" >&2; echo "$$req" >&2; exit 1; \
	else req=""; fi; \
	printf '%s\n' $$req | sort -u > "$$COVDIR/require100.txt"; \
	go run ./cmd/covergate --profile "$$COVDIR/profile.txt" --floor tests/coverage-floor.txt \
	  --module "$(MODULE)" --require-100 "$$COVDIR/require100.txt"

# Regenerate the floor from measured packages (never lowers without --allow-floor-decrease). The
# require-100 set here is exactly `internal/fsm/... ./workflows` — the same packages coverage.sh excludes
# from the regenerated floor — so cmd/covergate and internal/covergate are KEPT in the floor (at their
# measured 100.0) rather than dropped, matching `coverage.sh --update-floor`. They stay pinned to 100 by
# their 100.0 floor line plus covergate's refuse-to-lower rule.
cover-update-floor:
	@set -e; \
	COVDIR=$$(mktemp -d); \
	trap 'rm -rf "$$COVDIR"; go build -o bin/metareview ./cmd/metareview || echo "cover-update-floor: post-run rebuild of bin/metareview FAILED" >&2' EXIT; \
	go test -cover -covermode=atomic ./... -args -test.gocoverdir="$$COVDIR"; \
	GOFLAGS="-cover -covermode=atomic" GOCOVERDIR="$$COVDIR" bash tests/run-all.sh; \
	go tool covdata textfmt -i="$$COVDIR" -o "$$COVDIR/profile.txt"; \
	go list ./internal/fsm/... ./workflows 2>/dev/null | sort -u > "$$COVDIR/require100.txt"; \
	go run ./cmd/covergate --profile "$$COVDIR/profile.txt" --floor tests/coverage-floor.txt \
	  --module "$(MODULE)" --require-100 "$$COVDIR/require100.txt" --update-floor $(ARGS)

test: cover
