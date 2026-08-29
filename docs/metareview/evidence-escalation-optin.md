{"schemaVersion": 1, "kind": "validation", "command": ["bash", "tests/run-all.sh"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T19:33:16Z", "finishedAt": "2026-08-29T19:33:59Z", "stdoutSha256": "87e42cef88aae8efd1b00a89f9e37707404a514e374bff8538cf41078d8d09c5", "stderrSha256": "4ccf03eead1a46536adf703e5600e7e7fa5a9186395a72e1a0b07ee3504c6f01", "summary": "bash tests/run-all.sh \u2014 full gate suite (go tests, shell gates, coverage floors, manifests); exit 0; source revision b57b7c0e478b650de88b379509f92095f5e4e190 (tree aa747a0eda58358d78e2498b9a5e27b1ccd50b9c)", "sourceRevision": "b57b7c0e478b650de88b379509f92095f5e4e190", "sourceTree": "aa747a0eda58358d78e2498b9a5e27b1ccd50b9c", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["go", "test", "./...", "-count=1"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T19:33:59Z", "finishedAt": "2026-08-29T19:34:15Z", "stdoutSha256": "60f1ab4df121252b66a896867dd8c17ffb3bc984a3914ea92c496cc36106b396", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "go test ./... -count=1 \u2014 unit and integration tests, all packages; exit 0; source revision b57b7c0e478b650de88b379509f92095f5e4e190 (tree aa747a0eda58358d78e2498b9a5e27b1ccd50b9c)", "sourceRevision": "b57b7c0e478b650de88b379509f92095f5e4e190", "sourceTree": "aa747a0eda58358d78e2498b9a5e27b1ccd50b9c", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["golangci-lint", "run", "./..."], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T19:34:15Z", "finishedAt": "2026-08-29T19:34:16Z", "stdoutSha256": "e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "golangci-lint run ./... \u2014 errcheck, ineffassign, unused, and the rest of the repo config; exit 0; source revision b57b7c0e478b650de88b379509f92095f5e4e190 (tree aa747a0eda58358d78e2498b9a5e27b1ccd50b9c)", "sourceRevision": "b57b7c0e478b650de88b379509f92095f5e4e190", "sourceTree": "aa747a0eda58358d78e2498b9a5e27b1ccd50b9c", "covers": ["tests", "lint"]}

# metareview PR evidence — escalation-optin

Structured validation receipts above: exact command, cwd, exit code, timestamps, sha256 of
captured stdout and stderr, and the source revision each command ran against.

**Revision binding.** The receipts name `b57b7c0e478b650de88b379509f92095f5e4e190`, the tree they actually ran on. The commit that
adds this file necessarily comes after them, so the review that reads it records a head one
commit later, and that commit contains only this evidence file. Re-running validation after every
documentation commit to chase its own hash would be theatre; enforcing the match in the gate is
the real fix and is recorded in docs/0.10.0-candidates.md — `evidence.Receipt` has no revision
field, so nothing today compares the two.

## Mutation verification

Every fix in this branch was written test-first and verified by applying the mutation in an
isolated copy at a unique path and re-running the named test. Thirty-one mutations, each
reddening the test named beside it, are listed in the commit messages they belong to.
