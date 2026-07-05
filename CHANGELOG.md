# Changelog

## 0.6.0 - 2026-07-05

0.6.0 is the release that turned metareview from a basic local review gate into a more evidence-backed, stateful, and shard-aware review harness. There was no public 0.5.0 tag; these notes cover the work between `v0.4.0` and `v0.6.0`.

### Added

- Structured validation receipts with `metareview evidence run -- <command>`. Receipts preserve command, working directory, exit code, timestamps, output hashes, summary, and coverage labels so reviewers can distinguish real validation from prose.
- GitHub check import with `metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]`.
- Context profiles in task-done, epic-ready, and PR-ready context packs, including raw diff bytes, filtered diff bytes, generated review-artifact exclusions, untracked-file omissions, truncation signals, and deterministic context-risk reasons.
- Context shard planning for large or risky diffs. The shard plan records source diff hashes, shard IDs, shard paths, byte counts, prompt-pack paths, and reviewer instructions for shard-local and cross-shard findings.
- Review Manifest sections in task-done and PR-ready context packs. Manifests account for reviewed source paths, generated path dispositions, shard assignments, source-manifest hashes, manifest blockers, and static runtime-assessment status.
- Review Manifest aggregation validation for stale shard hashes, missing or duplicate shard results, unknown shard IDs, incomplete cross-shard coverage, invalid evidence references, and extra or unassigned covered paths.
- PR-ready review-state projection so previous blockers are reconciled by target and run chain before a branch is blocked by older review state.
- Post-merge learning artifacts for the 0.6.0 work, including accepted learning and discarded low-value candidates.

### Changed

- `task-done` and `pr-ready` now parse structured receipts as validation evidence while still accepting freeform evidence as a fallback. `epic-ready` accepts evidence files and uses their text for child-completion evidence.
- `task-done`, `epic-ready`, and `pr-ready` fail closed when context risk is detected instead of silently treating truncated, omitted, or oversized context as a normal review surface.
- Generated `docs/metareview/**` review artifacts are filtered out of source review context and represented explicitly as generated dispositions in the Review Manifest.
- Plugin metadata and package metadata now agree on `0.6.0` across npm, Codex, Claude Code, and Go source checkout version reporting.
- Review skill assets and integration docs now prefer structured receipts and document the receipt workflow.

### Fixed

- PR-ready no longer keeps unrelated or superseded blockers alive after follow-up runs clear the relevant target.
- The release-blocking manifest version mismatch was fixed before `v0.6.0`.
- Shard and manifest validation now reports stale or incomplete review evidence explicitly so missing coverage is visible in the Review Manifest.

### Validation

The release was validated with:

- `go test ./...`
- `bash tests/run-all.sh`
- `npm pack --dry-run`
- `git diff --check`

The `metareview@0.6.0` npm package was then published manually.
