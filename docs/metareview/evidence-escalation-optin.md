{"schemaVersion": 1, "kind": "validation", "command": ["bash", "tests/run-all.sh"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:23:09Z", "finishedAt": "2026-08-29T16:23:50Z", "stdoutSha256": "87e42cef88aae8efd1b00a89f9e37707404a514e374bff8538cf41078d8d09c5", "stderrSha256": "4ccf03eead1a46536adf703e5600e7e7fa5a9186395a72e1a0b07ee3504c6f01", "summary": "bash tests/run-all.sh \u2014 full gate suite (go tests, shell gates, coverage floors, manifests); exit 0; source revision 994e84efaf037ef29f56063824a819fc6010e71f (tree 4ceb672b51f247fb6cebbf4496e9e355384d7f2c)", "sourceRevision": "994e84efaf037ef29f56063824a819fc6010e71f", "sourceTree": "4ceb672b51f247fb6cebbf4496e9e355384d7f2c", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["go", "test", "./...", "-count=1"], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:23:50Z", "finishedAt": "2026-08-29T16:24:03Z", "stdoutSha256": "dbacd4745ce74babc228b5c43e96297be70c41d6c82920900c39970307582658", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "go test ./... -count=1 \u2014 unit and integration tests, all packages; exit 0; source revision 994e84efaf037ef29f56063824a819fc6010e71f (tree 4ceb672b51f247fb6cebbf4496e9e355384d7f2c)", "sourceRevision": "994e84efaf037ef29f56063824a819fc6010e71f", "sourceTree": "4ceb672b51f247fb6cebbf4496e9e355384d7f2c", "covers": ["tests", "lint"]}
{"schemaVersion": 1, "kind": "validation", "command": ["golangci-lint", "run", "./..."], "cwd": "/Users/dsifry/Developer/metareview", "exitCode": 0, "startedAt": "2026-08-29T16:24:03Z", "finishedAt": "2026-08-29T16:24:03Z", "stdoutSha256": "e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47", "stderrSha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "summary": "golangci-lint run ./... \u2014 errcheck, ineffassign, unused, and the rest of the repo config; exit 0; source revision 994e84efaf037ef29f56063824a819fc6010e71f (tree 4ceb672b51f247fb6cebbf4496e9e355384d7f2c)", "sourceRevision": "994e84efaf037ef29f56063824a819fc6010e71f", "sourceTree": "4ceb672b51f247fb6cebbf4496e9e355384d7f2c", "covers": ["tests", "lint"]}

# metareview PR evidence — escalation-optin

Structured validation receipts above. Each records the exact command, cwd, exit code,
start/finish timestamps, sha256 of captured stdout and stderr, and the **source revision**
the commands ran against (994e84efaf037ef29f56063824a819fc6010e71f).

Binding the revision matters: without it a receipt proves a command succeeded somewhere, not
that it succeeded on the tree under review, and evidence generated at one commit can be
presented for another. Raised by CodeRabbit on PR #21. `Receipt` has no revision field in the
schema, so it is carried in `summary` plus `sourceRevision`/`sourceTree` keys; making it a
first-class field the gate verifies against the context head is recorded for 0.10.0.

## Mutation verification

Each mutation was applied in an isolated copy at a unique path and the named test re-run:

    --escalate dropped from boolFlags            -> TestEscalateFlagIsPinned                     reddens
    escalation default reverted to ON            -> TestEscalationIsOffUnlessRequested           reddens
    --no-escalate left in the agent prompt       -> TestEscalateFlagIsPinned                     reddens
    declared lens set ignored                    -> TestArtifactLensSetIsGrandfathered           reddens
    scaffold emits an empty lens marker          -> TestScaffoldDeclaresItsLensSet               reddens
    scaffold declares a truncated lens set       -> TestScaffoldDeclaresItsLensSet               reddens
    scope-lens alias fold removed                -> TestCompletedScaffoldReadsAsCompleteInRev... reddens
    declaration replaces the era floor           -> post-cutoff cannot declare legacy rubric     reddens
    era lookup ignores the run date              -> TestLensErasAreKeyedByDate                   reddens
    test-fsm.sh stale-binary reuse reintroduced  -> test-manifests.sh guard                      reddens
    test-fsm.sh unconditional build removed      -> test-manifests.sh guard                      reddens
