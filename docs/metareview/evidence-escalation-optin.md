{"schemaVersion": 1, "kind": "validation", "command": ["bash", "tests/run-all.sh"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:00:22Z", "finishedAt": "2026-08-29T16:01:02Z", "stdoutSha256": "d0ac3b2709ead37792ec430e3cf52a49094a4a5a59e5ca8119dba786c1d92211", "stderrSha256": "4ccf03eead1a46536adf703e5600e7e7fa5a9186395a72e1a0b07ee3504c6f01", "summary": "bash tests/run-all.sh \u2014 full gate suite (go tests, shell gates, coverage floors, manifests); exit 0", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["go", "test", "./...", "-count=1"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:01:02Z", "finishedAt": "2026-08-29T16:01:15Z", "stdoutSha256": "69f0431570fe7aa25f5d75e28272fcbc957b7ee433715ab479858cc3776d00f1", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "go test ./... -count=1 \u2014 unit and integration tests, all packages; exit 0", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["golangci-lint", "run", "./..."], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:01:15Z", "finishedAt": "2026-08-29T16:01:16Z", "stdoutSha256": "e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "golangci-lint run ./... \u2014 errcheck, ineffassign, unused, and the rest of the repo config; exit 0", "covers": ["tests", "lint"]}

# metareview PR evidence — escalation-optin

Structured validation receipts above: exact command, cwd, exit code, timestamps, and
sha256 of captured stdout/stderr for each run, executed in this workspace at 8ff5216.

## Mutation verification

Each mutation was applied in an isolated copy at a unique path and the named test re-run:

    --escalate dropped from boolFlags           -> TestEscalateFlagIsPinned                     reddens
    escalation default reverted to ON           -> TestEscalationIsOffUnlessRequested           reddens
    --no-escalate left in the agent prompt      -> TestEscalateFlagIsPinned                     reddens
    lens cutoff moved to 2099                   -> TestArtifactLensSetIsGrandfathered           reddens
    unknown provenance treated as legacy        -> TestArtifactMissingRequiredReviewerRows...   reddens
    declared lens set ignored                   -> TestArtifactLensSetIsGrandfathered           reddens
    scaffold emits an empty lens marker         -> TestScaffoldDeclaresItsLensSet               reddens
    scaffold declares a truncated lens set      -> TestScaffoldDeclaresItsLensSet               reddens
    scope-lens alias fold removed               -> TestCompletedScaffoldReadsAsCompleteInRev... reddens
    test-fsm.sh stale-binary reuse reintroduced -> test-manifests.sh guard                      reddens
    test-fsm.sh unconditional build removed     -> test-manifests.sh guard                      reddens
