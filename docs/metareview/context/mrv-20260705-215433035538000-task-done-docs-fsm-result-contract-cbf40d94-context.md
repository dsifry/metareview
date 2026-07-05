# metareview task-done context

Run ID: `mrv-20260705-215433035538000-task-done-docs-fsm-result-contract-cbf40d94`

## Task

Advisory task target: docs-fsm-result-contract

## Git

- Base: `f625e62ef8fec74627a2895cc8284f4e708d3bb8`
- Head: `b50914fb0babcb2b1d1d2be9d096f091f09dc834`
- Branch: `codex/docs-0-6-release-notes`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `204347`
- Filtered diff bytes: `49839`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260705-063635212376000-artifact-changelog-ab09011f-context.md, docs/metareview/context/mrv-20260705-065156498851000-task-done-docs-0-6-a2ac175a-context.md, docs/metareview/context/mrv-20260705-161047045358000-task-done-docs-0-6-a2ac175a-context.md, docs/metareview/context/mrv-20260705-161102669835000-pr-ready-branch-10d735e5-context.md, docs/metareview/context/mrv-20260705-161214438540000-pr-ready-branch-10d735e5-context.md, docs/metareview/context/mrv-20260705-161259663343000-pr-ready-branch-10d735e5-context.md, docs/metareview/reviews/mrv-20260705-063635212376000-artifact-changelog-ab09011f.md, docs/metareview/reviews/mrv-20260705-065156498851000-task-done-docs-0-6-a2ac175a.md, docs/metareview/reviews/mrv-20260705-161047045358000-task-done-docs-0-6-a2ac175a.md, docs/metareview/reviews/mrv-20260705-161102669835000-pr-ready-branch-10d735e5.md, docs/metareview/reviews/mrv-20260705-161214438540000-pr-ready-branch-10d735e5.md, docs/metareview/reviews/mrv-20260705-161259663343000-pr-ready-branch-10d735e5.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `ba9a47e6ab4a7f34`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- AGENTS.md
- CHANGELOG.md
- CLAUDE.md
- INSTALL.md
- README.md
- commands/review-epic-ready.md
- commands/review-pr-ready.md
- commands/review-task-done.md
- docs/README.claude.md
- docs/README.codex.md
- docs/index.html
- docs/integrations/metaswarm.integration.json
- docs/integrations/metaswarm.md
- docs/quickstart.md
- skills/review-epic-ready/SKILL.md
- skills/review-pr-ready/SKILL.md
- skills/review-task-done/SKILL.md

### Path Dispositions
- docs/metareview/context/mrv-20260705-063635212376000-artifact-changelog-ab09011f-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-065156498851000-task-done-docs-0-6-a2ac175a-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-161047045358000-task-done-docs-0-6-a2ac175a-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-161102669835000-pr-ready-branch-10d735e5-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-161214438540000-pr-ready-branch-10d735e5-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260705-161259663343000-pr-ready-branch-10d735e5-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-063635212376000-artifact-changelog-ab09011f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-065156498851000-task-done-docs-0-6-a2ac175a.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-161047045358000-task-done-docs-0-6-a2ac175a.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-161102669835000-pr-ready-branch-10d735e5.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-161214438540000-pr-ready-branch-10d735e5.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260705-161259663343000-pr-ready-branch-10d735e5.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: AGENTS.md, CHANGELOG.md, CLAUDE.md, INSTALL.md, README.md, commands/review-epic-ready.md, commands/review-pr-ready.md, commands/review-task-done.md, docs/README.claude.md, docs/README.codex.md, docs/index.html, docs/integrations/metaswarm.integration.json, docs/integrations/metaswarm.md, docs/quickstart.md, skills/review-epic-ready/SKILL.md, skills/review-pr-ready/SKILL.md, skills/review-task-done/SKILL.md

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- CHANGELOG.md
- INSTALL.md
- README.md
- docs/README.claude.md
- docs/README.codex.md
- docs/index.html
- docs/integrations/metaswarm.integration.json
- docs/integrations/metaswarm.md
- docs/quickstart.md
- AGENTS.md
- CLAUDE.md
- commands/review-epic-ready.md
- commands/review-pr-ready.md
- commands/review-task-done.md
- skills/review-epic-ready/SKILL.md
- skills/review-pr-ready/SKILL.md
- skills/review-task-done/SKILL.md

## Diff

````diff
diff --git a/CHANGELOG.md b/CHANGELOG.md
new file mode 100644
index 0000000..5b8e781
--- /dev/null
+++ b/CHANGELOG.md
@@ -0,0 +1,41 @@
+# Changelog
+
+## 0.6.0 - 2026-07-05
+
+0.6.0 is the release that turned metareview from a basic local review gate into a more evidence-backed, stateful, and shard-aware review harness. There was no public 0.5.0 tag; these notes cover the work between `v0.4.0` and `v0.6.0`.
+
+### Added
+
+- Structured validation receipts with `metareview evidence run -- <command>`. Receipts preserve command, working directory, exit code, timestamps, output hashes, summary, and coverage labels so reviewers can distinguish real validation from prose.
+- GitHub check import with `metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]`.
+- Context profiles in task-done, epic-ready, and PR-ready context packs, including raw diff bytes, filtered diff bytes, generated review-artifact exclusions, untracked-file omissions, truncation signals, and deterministic context-risk reasons.
+- Context shard planning for large or risky diffs. The shard plan records source diff hashes, shard IDs, shard paths, byte counts, prompt-pack paths, and reviewer instructions for shard-local and cross-shard findings.
+- Review Manifest sections in task-done and PR-ready context packs. Manifests account for reviewed source paths, generated path dispositions, shard assignments, source-manifest hashes, manifest blockers, and static runtime-assessment status.
+- Review Manifest aggregation validation for stale shard hashes, missing or duplicate shard results, unknown shard IDs, incomplete cross-shard coverage, invalid evidence references, and extra or unassigned covered paths.
+- PR-ready review-state projection so previous blockers are reconciled by target and run chain before a branch is blocked by older review state.
+- Post-merge learning artifacts for the 0.6.0 work, including accepted learning and discarded low-value candidates.
+
+### Changed
+
+- `task-done` and `pr-ready` now parse structured receipts as validation evidence while still accepting freeform evidence as a fallback. `epic-ready` accepts evidence files and uses their text for child-completion evidence.
+- `task-done`, `epic-ready`, and `pr-ready` fail closed when context risk is detected instead of silently treating truncated, omitted, or oversized context as a normal review surface.
+- Generated `docs/metareview/**` review artifacts are filtered out of source review context and represented explicitly as generated dispositions in the Review Manifest.
+- Plugin metadata and package metadata now agree on `0.6.0` across npm, Codex, Claude Code, and Go source checkout version reporting.
+- Review skill assets and integration docs now prefer structured receipts and document the receipt workflow.
+
+### Fixed
+
+- PR-ready no longer keeps unrelated or superseded blockers alive after follow-up runs clear the relevant target.
+- The release-blocking manifest version mismatch was fixed before `v0.6.0`.
+- Shard and manifest validation now reports stale or incomplete review evidence explicitly so missing coverage is visible in the Review Manifest.
+
+### Validation
+
+The release was validated with:
+
+- `go test ./...`
+- `bash tests/run-all.sh`
+- `npm pack --dry-run`
+- `git diff --check`
+
+The `metareview@0.6.0` npm package was then published manually.
diff --git a/INSTALL.md b/INSTALL.md
index eb690cf..af0c6a7 100644
--- a/INSTALL.md
+++ b/INSTALL.md
@@ -108,13 +108,23 @@ Artifact reviews create a Markdown review scaffold under `docs/metareview/review
 Use the smallest gate that matches the lifecycle point:

 ```bash
+tmp_evidence="$(mktemp)"
+metareview evidence run -- go test ./... > "$tmp_evidence"
+metareview evidence run -- git diff --check >> "$tmp_evidence"
+
 metareview review artifact <path>
-metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
-metareview review epic-ready <epic-id-or-path>
-metareview review pr-ready --base <base-ref>
+metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
+metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
+metareview review pr-ready --base <base-ref> --evidence "$tmp_evidence"
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```

+After a GitHub PR exists, append CI receipts with `metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]`.
+
+Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals.
+
+Task-done, epic-ready, and PR-ready context packs include context profiles, generated-artifact filtering, and shard plans for risky diffs. Task-done and PR-ready also include Review Manifest coverage accounting.
+
 Commit durable Markdown artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless a future contract says otherwise. In ordinary project repositories, prefer exact `.gitignore` entries:

 ```gitignore
diff --git a/README.md b/README.md
index 596700f..a48d2ff 100644
--- a/README.md
+++ b/README.md
@@ -50,6 +50,19 @@ metareview is built around review patterns that work well when humans and coding
 - **Review artifact accountability:** write durable Markdown context and review logs so future humans and agents can inspect what was reviewed, what blocked, and why it passed.
 - **Post-merge reflection:** after a PR lands, extract accepted learnings, discarded candidates, and reviewer calibration so the next review starts smarter.

+## What Changed From 0.4.0 To 0.6.0
+
+0.6.0 made metareview more useful for real agent work by adding concrete coverage accounting around the review surface:
+
+- **Structured evidence receipts:** `metareview evidence run -- <command>` records validation commands as JSON receipts with exit codes, timestamps, summaries, and output hashes. `metareview evidence import --github-checks <pr-number>` imports GitHub check results into the same receipt format. Task-done and PR-ready parse those receipts as validation evidence; epic-ready accepts the same evidence file as child-completion context.
+- **Context preflight:** task-done, epic-ready, and PR-ready reviews now include a Context Profile that records raw and filtered diff size, generated review-artifact exclusions, omitted or truncated untracked files, and context-risk reasons.
+- **Shard planning:** large or risky diffs get deterministic Context Shard Plans so agents can split review work by source paths while preserving a shared source diff hash.
+- **Review Manifest aggregation:** task-done and PR-ready context packs now account for source paths, generated path dispositions, shard assignments, manifest hashes, static runtime status, and manifest blockers.
+- **Stateful PR-ready projection:** PR-ready reconciles prior findings by target and run chain, so resolved or unrelated blockers do not keep blocking a later branch review.
+- **0.6.0 metadata alignment:** npm, Codex plugin, Claude Code plugin, and Go source checkout version reporting now agree on `0.6.0`.
+
+See [CHANGELOG.md](CHANGELOG.md) for the full release notes.
+
 ## Install

 ### npm Package
@@ -144,9 +157,9 @@ flowchart TD
     moreChildren{More child units?}
     parentIntent{Parent intent preserved?}
     parentRevise[Reconcile drift against original intent]
-    epicReady[Epic-ready review<br/>metareview review epic-ready target]
+    epicReady[Epic-ready review<br/>metareview review epic-ready target --base ref --evidence file]
     epicPass{Epic review passes?}
-    prReady[PR-ready review<br/>metareview review pr-ready --base ref]
+    prReady[PR-ready review<br/>metareview review pr-ready --base ref --evidence file]
     prPass{PR review passes?}
     merge[Push, PR, merge]
     learn[Post-merge learning<br/>metareview learn --post-merge pr --base pre-merge-ref]
@@ -178,10 +191,14 @@ Every review produces Markdown artifacts under `docs/metareview/` and local tran
 Humans use metareview to make review timing explicit:

 ```bash
+tmp_evidence="$(mktemp)"
+metareview evidence run -- go test ./... > "$tmp_evidence"
+metareview evidence run -- git diff --check >> "$tmp_evidence"
+
 metareview review artifact docs/spec.md
-metareview review task-done docs/tasks/task-001.md --base main --evidence /tmp/evidence.txt
-metareview review epic-ready docs/epics/epic-001.md
-metareview review pr-ready --base main
+metareview review task-done docs/tasks/task-001.md --base main --evidence "$tmp_evidence"
+metareview review epic-ready docs/epics/epic-001.md --base main --evidence "$tmp_evidence"
+metareview review pr-ready --base main --evidence "$tmp_evidence"
 metareview learn --post-merge 42 --base pre-merge-sha
 ```

@@ -211,10 +228,12 @@ When configuring `.gitignore` in ordinary project repositories, ignore those tra
 ```bash
 metareview setup --check
 metareview setup --bootstrap-prereqs --dry-run
+metareview evidence run -- <command> [args...]
+metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]
 metareview review artifact <path>
 metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
-metareview review epic-ready <epic-id-or-path>
-metareview review pr-ready --base <base-ref>
+metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence <file>
+metareview review pr-ready --base <base-ref> --evidence <file>
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 metareview status
 ```
@@ -234,6 +253,7 @@ metareview follows a few practical rules:
 ## More Docs

 - [INSTALL.md](INSTALL.md) - installation paths and troubleshooting
+- [CHANGELOG.md](CHANGELOG.md) - release notes
 - [docs/quickstart.md](docs/quickstart.md) - short operator guide
 - [docs/README.codex.md](docs/README.codex.md) - Codex plugin usage
 - [docs/README.claude.md](docs/README.claude.md) - Claude Code plugin usage
diff --git a/docs/README.claude.md b/docs/README.claude.md
index 14f31f6..cfc0e24 100644
--- a/docs/README.claude.md
+++ b/docs/README.claude.md
@@ -27,10 +27,11 @@ For local development, install the plugin from the current checkout using the lo

 ```bash
 metareview setup --check
+metareview evidence run -- go test ./... > /tmp/metareview-evidence.jsonl
 metareview review artifact <path>
-metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
-metareview review epic-ready <epic-id-or-path>
-metareview review pr-ready --base <base-ref>
+metareview review task-done <task-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
+metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
+metareview review pr-ready --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```

@@ -44,6 +45,8 @@ go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --e

 Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Re-run the review with `--previous-run <run-id>` after fixes. `PASS_ADVISORY` is acceptable only when the report has zero blocking findings.

+Prefer structured evidence receipts from `metareview evidence run -- <command>` and, after a PR exists, `metareview evidence import --github-checks <pr-number>`. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals. Task-done, epic-ready, and PR-ready reviews include context profiles, generated-artifact filtering, and shard plans for risky diffs. Task-done and PR-ready also include Review Manifest coverage accounting.
+
 Commit durable review and context Markdown under `docs/metareview/`. Leave transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.

 ## Metaswarm Repositories
diff --git a/docs/README.codex.md b/docs/README.codex.md
index a937ec3..1d0b044 100644
--- a/docs/README.codex.md
+++ b/docs/README.codex.md
@@ -38,10 +38,11 @@ Use direct commands when a skill is unavailable:

 ```bash
 metareview setup --check
+metareview evidence run -- go test ./... > /tmp/metareview-evidence.jsonl
 metareview review artifact <path>
-metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
-metareview review epic-ready <epic-id-or-path>
-metareview review pr-ready --base <base-ref>
+metareview review task-done <task-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
+metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
+metareview review pr-ready --base <base-ref> --evidence /tmp/metareview-evidence.jsonl
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```

@@ -51,6 +52,8 @@ In a source checkout without a packaged binary, prefix commands with `go run ./c

 Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Fix blockers, re-run with `--previous-run <run-id>`, and proceed only after `PASS` or `PASS_ADVISORY` with zero blockers.

+Prefer structured evidence receipts from `metareview evidence run -- <command>` and, after a PR exists, `metareview evidence import --github-checks <pr-number>`. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals. Task-done, epic-ready, and PR-ready reviews include context profiles, generated-artifact filtering, and shard plans for risky diffs. Task-done and PR-ready also include Review Manifest coverage accounting.
+
 Commit durable review artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.

 ## Metaswarm Repositories
diff --git a/docs/index.html b/docs/index.html
index 23943e1..9578f0a 100644
--- a/docs/index.html
+++ b/docs/index.html
@@ -453,18 +453,35 @@
 $ metareview setup --check
 mode: metaswarm-extension

+$ metareview evidence run -- go test ./... \
+  > /tmp/metareview-evidence.jsonl
+
 $ metareview review task-done task-42 \
   --base main \
-  --evidence /tmp/evidence.txt
+  --evidence /tmp/metareview-evidence.jsonl

 verdict: BLOCKED
-finding: acceptance evidence missing
+finding: context-risk requires shard review
 next: fix, re-run with --previous-run</code></pre>
       </div>
     </div>
   </header>

   <main>
+    <section>
+      <div class="wrap split">
+        <div>
+          <h2>0.6.0 adds evidence receipts, context profiles, and review manifests.</h2>
+          <p class="section-intro">The 0.4.0 to 0.6.0 work focused on making review gates more actionable for real agents working in large or messy repos.</p>
+        </div>
+        <div class="panel">
+          <p><strong>Receipts:</strong> validation commands and GitHub checks can be captured as structured evidence with exit codes, timestamps, summaries, and output hashes.</p>
+          <p><strong>Profiles and shards:</strong> task-done, epic-ready, and PR-ready reviews now record diff size, generated artifact exclusions, omitted or truncated context, and deterministic shard plans when the review surface is too large or risky.</p>
+          <p><strong>Manifests:</strong> Task-done and PR-ready Review Manifest sections account for source paths, generated dispositions, shard coverage, source-manifest hashes, and aggregate blockers.</p>
+        </div>
+      </div>
+    </section>
+
     <section>
       <div class="wrap split">
         <div>
diff --git a/docs/integrations/metaswarm.integration.json b/docs/integrations/metaswarm.integration.json
index a5f2d88..217db90 100644
--- a/docs/integrations/metaswarm.integration.json
+++ b/docs/integrations/metaswarm.integration.json
@@ -58,17 +58,17 @@
       },
       {
         "stage": "task-done",
-        "metareview": "metareview review task-done <task-id-or-path>",
+        "metareview": "metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>",
         "effect": "Run when a work unit claims done; unresolved blocking findings block local task closure."
       },
       {
         "stage": "epic-ready",
-        "metareview": "metareview review epic-ready <epic-id-or-path>",
+        "metareview": "metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence <file>",
         "effect": "Run after all child tasks pass; integration, acceptance, and intent-drift blockers prevent epic landing."
       },
       {
         "stage": "pr-ready",
-        "metareview": "metareview review pr-ready --base <base-ref>",
+        "metareview": "metareview review pr-ready --base <base-ref> --evidence <file>",
         "effect": "Run before PR push or merge readiness to catch branch-level blockers."
       },
       {
diff --git a/docs/integrations/metaswarm.md b/docs/integrations/metaswarm.md
index 91986e3..47e1b89 100644
--- a/docs/integrations/metaswarm.md
+++ b/docs/integrations/metaswarm.md
@@ -20,9 +20,9 @@ The machine-readable descriptor is `docs/integrations/metaswarm.integration.json
 | Metaswarm stage | Metareview command | Gate behavior |
 | --- | --- | --- |
 | Artifact ready for implementation | `metareview review artifact <path>` | Creates a fail-closed scaffold; remains blocking while verdict is `NOT_REVIEWED`, reviewer rows are incomplete, or blockers remain. |
-| Work unit claims done | `metareview review task-done <task-id-or-path>` | Blocks task closure on unresolved blocking findings. |
-| Epic locally complete | `metareview review epic-ready <epic-id-or-path>` | Blocks epic landing on integration, acceptance, or intent-drift findings. |
-| PR ready to push or merge | `metareview review pr-ready --base <base-ref>` | Blocks PR readiness on branch-level blockers. |
+| Work unit claims done | `metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>` | Blocks task closure on unresolved blocking findings. |
+| Epic locally complete | `metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence <file>` | Blocks epic landing on integration, acceptance, or intent-drift findings. |
+| PR ready to push or merge | `metareview review pr-ready --base <base-ref> --evidence <file>` | Blocks PR readiness on branch-level blockers. |
 | Confirmed PR merge | `metareview learn --post-merge <pr-number> --base <pre-merge-ref>` | Curates accepted/discarded learning and reviewer calibration. |

 Post-merge learning is advisory by default. In normal mode, a learning failure should be recorded and release cleanup may continue. In strict mode, the caller treats a nonzero learning exit as blocking release cleanup until the learning run succeeds or is explicitly waived.
diff --git a/docs/quickstart.md b/docs/quickstart.md
index 6f0239a..5e9e999 100644
--- a/docs/quickstart.md
+++ b/docs/quickstart.md
@@ -18,15 +18,33 @@ metareview setup --bootstrap-prereqs --dry-run

 The dry run does not install Superpowers, Beads, or metaswarm. Non-dry-run bootstrap requires explicit confirmation.

-## 2. Run Reviews At The Right Gate
+## 2. Capture Validation Evidence
+
+Prefer structured receipts over prose evidence:
+
+```bash
+tmp_evidence="$(mktemp)"
+metareview evidence run -- go test ./... > "$tmp_evidence"
+metareview evidence run -- git diff --check >> "$tmp_evidence"
+```
+
+After a GitHub PR exists, CI checks can be imported into the same evidence file:
+
+```bash
+metareview evidence import --github-checks <pr-number> --repo <owner/repo> >> "$tmp_evidence"
+```
+
+Freeform evidence files still work as a fallback, but receipts preserve command, working directory, exit code, timestamps, summary, and output hashes. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals.
+
+## 3. Run Reviews At The Right Gate

 Use the smallest gate that matches the work:

 ```bash
 metareview review artifact <path>
-metareview review task-done <task-id-or-path>
-metareview review epic-ready <epic-id-or-path>
-metareview review pr-ready --base <base-ref>
+metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
+metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
+metareview review pr-ready --base <base-ref> --evidence "$tmp_evidence"
 metareview learn --post-merge <pr-number> --base <pre-merge-ref>
 ```

@@ -34,13 +52,15 @@ metareview learn --post-merge <pr-number> --base <pre-merge-ref>

 If a review reports any blocking finding or remains `NOT_REVIEWED`, fix it and re-run with `--previous-run <run-id>` until the result is `PASS` or `PASS_ADVISORY` with zero blockers.

-## 3. Metaswarm Fit
+Task-done, epic-ready, and PR-ready context packs now include a Context Profile and Context Shard Plan when risk requires sharding. Task-done and PR-ready also include a Review Manifest that accounts for source paths, generated path dispositions, shard assignments, manifest hashes, and manifest blockers.
+
+## 4. Metaswarm Fit

 When metaswarm, Superpowers, and Beads are present, metaswarm remains the lifecycle owner. Metareview supplies deeper review commands and durable artifacts. The integration contract is in `docs/integrations/metaswarm.md`.

 In standalone mode, metareview still runs advisory reviews and can use `.metareview/knowledge/metareview.jsonl` until Beads knowledge is available.

-## 4. What To Commit
+## 5. What To Commit

 Commit:

@@ -67,7 +87,7 @@ For ordinary project repositories, use exact file entries for transient state. D

 The repository `.gitignore` keeps transient state local while allowing fallback learning knowledge and calibration to sync through git.

-## 5. Agent Syntax
+## 6. Agent Syntax

 Codex users invoke metareview through `$setup`, `$review-artifact`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$learn-post-merge`, and `$status`.

diff --git a/AGENTS.md b/AGENTS.md
index 83a60e9..b4fa7ce 100644
--- a/AGENTS.md
+++ b/AGENTS.md
@@ -16,9 +16,18 @@ Use `go run ./cmd/metareview ...` when running from a source checkout without a

 ## Blocker Policy

-Do not claim completion while any blocking finding remains open. Fix blockers, re-run the same gate with `--previous-run <run-id>`, and continue until the verdict is `PASS` or `PASS_ADVISORY`.
+Do not claim completion while any blocking finding remains open.

-`PASS_ADVISORY` is acceptable only when the review explicitly reports zero blocking findings. Advisory notes can be recorded for later, but blockers are current work.
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
+
+Lifecycle gate verdicts have this contract:
+
+- `PASS`: proceed.
+- `PASS_ADVISORY`: proceed only when the review explicitly reports zero blocking findings.
+- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
+- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
+
+Advisory notes can be recorded for later, but blockers are current work.

 ## Evidence Policy

diff --git a/CLAUDE.md b/CLAUDE.md
index 77870b6..9fea063 100644
--- a/CLAUDE.md
+++ b/CLAUDE.md
@@ -21,7 +21,13 @@ go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --e

 ## Completion Rule

-Before saying work is done, run the appropriate metareview gate. A `PASS_ADVISORY` result is acceptable only when there is no blocking finding. Any blocker must be fixed and reviewed again with `--previous-run <run-id>`.
+Before saying work is done, run the appropriate metareview gate.
+
+- `PASS`/`PASS_ADVISORY` proceed only with zero blockers.
+- `NEEDS_REVISION` repairs via `--previous-run <run-id>`.
+- `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target.
+
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.

 ## Lifecycle Placement

diff --git a/INSTALL.md b/INSTALL.md
index af0c6a7..de5667b 100644
--- a/INSTALL.md
+++ b/INSTALL.md
@@ -101,7 +101,9 @@ metareview setup --check
 metareview review artifact docs/quickstart.md
 ```

-Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Artifact review runs the required lenses as parallel subagents by default; use `in-session-emulated` only when subagents are unavailable or the human requests no delegation, and mark that result as not independently adversarial and weaker evidence. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate. Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` report `PASS`, `PASS_ADVISORY`, or blocking findings. Treat every blocking finding and every `NOT_REVIEWED` artifact as open work until a re-review clears it.
+Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Artifact review runs the required lenses as parallel subagents by default; use `in-session-emulated` only when subagents are unavailable or the human requests no delegation, and mark that result as not independently adversarial and weaker evidence. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate.
+
+Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` use this result contract: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs via `--previous-run`; `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target. Exit handling: `0` means verify a passing verdict; `1` with a review path means follow that log; nonzero without a path means read stderr. `NOT_REVIEWED` artifact scaffolds are also blocking until completed.

 ## Agent Workflow

@@ -156,4 +158,5 @@ bash tests/run-all.sh

 - `No packaged metareview binary or Go source checkout found`: run `npm run build` in a source checkout or install a packaged release.
 - `setup --check` reports advisory mode: install or configure metaswarm, Superpowers, and Beads if full lifecycle integration is desired.
-- Review returns blockers: fix the cited issues and re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
+- Review returns `NEEDS_REVISION`: fix the cited blockers and re-run with `--previous-run <run-id>`.
+- Review returns `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
diff --git a/README.md b/README.md
index a48d2ff..b6522df 100644
--- a/README.md
+++ b/README.md
@@ -40,7 +40,7 @@ The goal is not another loose "please review this" prompt. The goal is a review
 metareview is built around review patterns that work well when humans and coding agents are collaborating:

 - **Adversarial multi-agent reviews:** run independent reviewer lenses such as architecture, code quality, security, test adequacy, product/user impact, and acceptance completeness against the same artifact or diff.
-- **Iterations with hard gates:** treat critical, high, and spec-contract findings as blockers; revise the work and re-run review with `--previous-run <run-id>` until blockers are cleared.
+- **Iterations with hard gates:** treat critical, high, and spec-contract findings as blockers; retry only while the gate reports `NEEDS_REVISION`, and stop autonomous retries on `ESCALATED`.
 - **Fractal review loops:** decompose large work into epics, tasks, and child plans, then review each level before implementation proceeds.
 - **Cross-level intent checks:** after multiple revision loops, compare the accepted child work back to the parent plan and original user request.
 - **Evidence-backed reviews:** attach test output, validation commands, acceptance notes, and PR context so reviewers judge the real work product, not a summary.
@@ -170,21 +170,34 @@ flowchart TD
     child --> childReview --> childApproved
     childApproved -- no --> childRevise --> childReview
     childApproved -- yes --> implement --> taskDone --> taskPass
-    taskPass -- no --> fix --> taskDone
-    taskPass -- yes --> moreChildren
+    taskPass -- NEEDS_REVISION --> fix --> taskDone
+    taskPass -- ESCALATED --> escalate
+    taskPass -- PASS/PASS_ADVISORY --> moreChildren
     moreChildren -- yes --> child
     moreChildren -- no --> parentIntent
     parentIntent -- no --> parentRevise --> childReview
     parentIntent -- yes --> epicReady --> epicPass
-    epicPass -- no --> childReview
-    epicPass -- yes --> prReady --> prPass
-    prPass -- no --> fix --> prReady
-    prPass -- yes --> merge --> learn
+    epicPass -- NEEDS_REVISION --> childReview
+    epicPass -- ESCALATED --> escalate
+    epicPass -- PASS/PASS_ADVISORY --> prReady --> prPass
+    prPass -- NEEDS_REVISION --> fix --> prReady
+    prPass -- ESCALATED --> escalate
+    prPass -- PASS/PASS_ADVISORY --> merge --> learn
+    escalate[Human narrows, splits, or redesigns target]
 ```

 The decomposition loop is intentionally fractal: a parent plan can be decomposed into child epics, each child can be decomposed again, and each level gets reviewed before implementation continues. After the iteration converges, metareview checks back against the original parent intent so accumulated local fixes do not quietly drift away from the user request.

-Every review produces Markdown artifacts under `docs/metareview/` and local transient state under `.metareview/`. A blocking finding is current work. A `NOT_REVIEWED` artifact scaffold is also current work, not a pass. Artifact review runs the five required lenses as parallel subagents by default; `in-session-emulated` fallback is weaker evidence and must say the review is not independently adversarial. Fix blockers, re-run with `--previous-run <run-id>`, and do not claim completion until the review reports `PASS` or `PASS_ADVISORY` with zero blockers.
+Every review produces Markdown artifacts under `docs/metareview/` and local transient state under `.metareview/`. A blocking finding is current work. A `NOT_REVIEWED` artifact scaffold is also current work, not a pass. Artifact review runs the five required lenses as parallel subagents by default; `in-session-emulated` fallback is weaker evidence and must say the review is not independently adversarial.
+
+Lifecycle gate results have a small operating contract:
+
+- `PASS`: proceed.
+- `PASS_ADVISORY`: proceed only when the review reports zero blocking findings.
+- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
+- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
+
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.

 ## How Humans Use It

@@ -214,7 +227,7 @@ Coding agents should treat metareview as a completion gate, not an optional comm
 - Before push, PR creation, or merge, run `pr-ready`.
 - After merge, run `learn --post-merge` so repository knowledge improves.

-Agents must not say work is done while a blocking finding remains unresolved. They should commit durable review/context artifacts when the repository's artifact policy says to do so, and keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.
+Agents must not say work is done while a blocking finding remains unresolved or while a gate is `NEEDS_REVISION` or `ESCALATED`. They should commit durable review/context artifacts when the repository's artifact policy says to do so, and keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local.

 When configuring `.gitignore` in ordinary project repositories, ignore those transient files with exact file entries. Do not ignore `docs/metareview/` or the whole `.metareview/` directory, because durable learning, calibration, and fallback knowledge can live there:

diff --git a/commands/review-epic-ready.md b/commands/review-epic-ready.md
index e0921f0..664538b 100644
--- a/commands/review-epic-ready.md
+++ b/commands/review-epic-ready.md
@@ -3,7 +3,7 @@
 Run the local epic-ready review gate:

 ```bash
-metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
+metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
 ```

-Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`.
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
diff --git a/commands/review-pr-ready.md b/commands/review-pr-ready.md
index 1035cc8..185891a 100644
--- a/commands/review-pr-ready.md
+++ b/commands/review-pr-ready.md
@@ -3,7 +3,7 @@
 Run the local PR-ready review gate:

 ```bash
-metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--evidence <path>] [--github-pr <number>]
+metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree]
 ```

-Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`. Use the generated `metareview PR Evidence` section when preparing the PR.
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. Use the generated `metareview PR Evidence` section after a passing verdict.
diff --git a/commands/review-task-done.md b/commands/review-task-done.md
index 24bf1a9..c3b596e 100644
--- a/commands/review-task-done.md
+++ b/commands/review-task-done.md
@@ -3,7 +3,7 @@
 Run the local task-done review gate:

 ```bash
-metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
+metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
 ```

-Resolve blocking findings, then re-run with `--previous-run` until the generated review log reports `PASS` or `PASS_ADVISORY`.
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
diff --git a/docs/README.claude.md b/docs/README.claude.md
index cfc0e24..62fc0d3 100644
--- a/docs/README.claude.md
+++ b/docs/README.claude.md
@@ -43,7 +43,9 @@ go run ./cmd/metareview review task-done <task-id-or-path> --base <base-ref> --e

 ## Agent Contract

-Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Re-run the review with `--previous-run <run-id>` after fixes. `PASS_ADVISORY` is acceptable only when the report has zero blocking findings.
+Claude Code agents must resolve every blocking finding before claiming completion. A `NOT_REVIEWED` artifact scaffold is also blocking; complete the required reviewer rows and final verdict before treating the artifact as reviewed. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence.
+
+Lifecycle gate results are actionable: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs via `--previous-run`; `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target. Exit handling: `0` means verify a passing verdict; `1` with a review path means follow that log; nonzero without a path means read stderr.

 Prefer structured evidence receipts from `metareview evidence run -- <command>` and, after a PR exists, `metareview evidence import --github-checks <pr-number>`. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals. Task-done, epic-ready, and PR-ready reviews include context profiles, generated-artifact filtering, and shard plans for risky diffs. Task-done and PR-ready also include Review Manifest coverage accounting.

diff --git a/docs/README.codex.md b/docs/README.codex.md
index 1d0b044..a7b839a 100644
--- a/docs/README.codex.md
+++ b/docs/README.codex.md
@@ -50,7 +50,9 @@ In a source checkout without a packaged binary, prefix commands with `go run ./c

 ## Agent Contract

-Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence. Fix blockers, re-run with `--previous-run <run-id>`, and proceed only after `PASS` or `PASS_ADVISORY` with zero blockers.
+Codex agents must not claim work complete while a blocking finding remains open or while an artifact review remains `NOT_REVIEWED`. The default artifact command exits nonzero after scaffold creation until agents complete the required reviewer rows and final verdict. Artifact review authorizes the five required lenses to run as parallel subagents by default. If subagents are unavailable or the human requests no delegation, record `in-session-emulated` and state that the review is not independently adversarial and is weaker evidence.
+
+Lifecycle gate results are actionable: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs via `--previous-run`; `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target. Exit handling: `0` means verify a passing verdict; `1` with a review path means follow that log; nonzero without a path means read stderr.

 Prefer structured evidence receipts from `metareview evidence run -- <command>` and, after a PR exists, `metareview evidence import --github-checks <pr-number>`. Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals. Task-done, epic-ready, and PR-ready reviews include context profiles, generated-artifact filtering, and shard plans for risky diffs. Task-done and PR-ready also include Review Manifest coverage accounting.

diff --git a/docs/integrations/metaswarm.md b/docs/integrations/metaswarm.md
index 47e1b89..43b0dc1 100644
--- a/docs/integrations/metaswarm.md
+++ b/docs/integrations/metaswarm.md
@@ -25,6 +25,8 @@ The machine-readable descriptor is `docs/integrations/metaswarm.integration.json
 | PR ready to push or merge | `metareview review pr-ready --base <base-ref> --evidence <file>` | Blocks PR readiness on branch-level blockers. |
 | Confirmed PR merge | `metareview learn --post-merge <pr-number> --base <pre-merge-ref>` | Curates accepted/discarded learning and reviewer calibration. |

+For lifecycle gates, `NEEDS_REVISION` means metaswarm should repair and re-run the same gate with `--previous-run <run-id>`. `ESCALATED` means the same-target autonomous loop is exhausted; human must narrow, split, or redesign the target.
+
 Post-merge learning is advisory by default. In normal mode, a learning failure should be recorded and release cleanup may continue. In strict mode, the caller treats a nonzero learning exit as blocking release cleanup until the learning run succeeds or is explicitly waived.

 Automatic hook installation is out of scope for this slice. Metaswarm remains the lifecycle owner; metareview supplies commands, review artifacts, and knowledge updates that metaswarm can invoke explicitly.
diff --git a/docs/quickstart.md b/docs/quickstart.md
index 5e9e999..b9dba90 100644
--- a/docs/quickstart.md
+++ b/docs/quickstart.md
@@ -50,7 +50,14 @@ metareview learn --post-merge <pr-number> --base <pre-merge-ref>

 `artifact` creates an incomplete review scaffold for specs, plans, and docs. The command exits nonzero while the scaffold is still `NOT_REVIEWED`; complete every required reviewer row and update the verdict before treating the artifact as reviewed. Artifact review runs the five required lenses as parallel subagents by default. Use `in-session-emulated` only when subagents are unavailable or the human explicitly requests no delegation, and state that the review is not independently adversarial and is weaker evidence. Use `--scaffold-only` only when scaffold creation itself is the intended action. `task-done` runs after a local task or chunk claims done. `epic-ready` runs when child tasks are complete. `pr-ready` runs before push or merge readiness. `learn --post-merge` runs after confirmed PR merge.

-If a review reports any blocking finding or remains `NOT_REVIEWED`, fix it and re-run with `--previous-run <run-id>` until the result is `PASS` or `PASS_ADVISORY` with zero blockers.
+Lifecycle gate results use this contract:
+
+- `PASS`: proceed.
+- `PASS_ADVISORY`: proceed only when the review reports zero blocking findings.
+- `NEEDS_REVISION`: fix blockers, then re-run the same gate with `--previous-run <run-id>`.
+- `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
+
+Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr. `NOT_REVIEWED` artifact scaffolds are also blocking until completed.

 Task-done, epic-ready, and PR-ready context packs now include a Context Profile and Context Shard Plan when risk requires sharding. Task-done and PR-ready also include a Review Manifest that accounts for source paths, generated path dispositions, shard assignments, manifest hashes, and manifest blockers.

diff --git a/skills/review-epic-ready/SKILL.md b/skills/review-epic-ready/SKILL.md
index 6d8d15d..f0a6774 100644
--- a/skills/review-epic-ready/SKILL.md
+++ b/skills/review-epic-ready/SKILL.md
@@ -10,16 +10,17 @@ Run this before declaring an epic ready to land.
 ## Command

 ```bash
-metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
+metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
 ```

-Use `--base` for the reviewed diff, `--previous-run` after fixing blockers, and `--evidence` for validation or acceptance notes.
+Use `--base` for the reviewed diff, `--previous-run` after fixes, and `--evidence` for validation or acceptance notes. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1.

 ## Workflow

 1. Run the command from the repository root.
-2. If it exits `1`, open the generated review log and fix every blocking finding.
-3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
-4. Re-check that the final result still satisfies the original epic intent.
+2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
+3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
+4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
+5. After a passing verdict, re-check that the final result still satisfies the original epic intent.

 The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.
diff --git a/skills/review-pr-ready/SKILL.md b/skills/review-pr-ready/SKILL.md
index e5b8d40..f79a44c 100644
--- a/skills/review-pr-ready/SKILL.md
+++ b/skills/review-pr-ready/SKILL.md
@@ -10,10 +10,10 @@ Run this before pushing a PR branch or asking external reviewers to spend time.
 ## Command

 ```bash
-metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--evidence <path>] [--github-pr <number>]
+metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>] [--include-working-tree]
 ```

-Use `--base` for the reviewed branch diff, `--previous-run` after fixing blockers, `--evidence` for structured receipts or test output, and `--github-pr` to include available GitHub PR context.
+Use `--base` for the reviewed branch diff, `--previous-run` after fixes, and `--evidence` for validation output. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1. Use `--github-pr` to include available GitHub PR context. By default, PR-ready reviews the committed branch diff and blocks on non-generated working-tree changes; use `--include-working-tree` only when those changes intentionally belong to the review.

 Prefer structured evidence receipts:

@@ -27,8 +27,9 @@ Freeform evidence remains accepted as a fallback, but receipts preserve command,
 ## Workflow

 1. Run the command from the repository root.
-2. If it exits `1`, open the generated review log and fix every blocking finding.
-3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
-4. Use the generated `metareview PR Evidence` section in the PR description or handoff.
+2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
+3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
+4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
+5. After a passing verdict, use the generated `metareview PR Evidence` section in the PR description or handoff.

 GitHub context is optional in local mode. Missing `gh`, auth, remote, or PR number is recorded as unavailable context rather than a blocker.
diff --git a/skills/review-task-done/SKILL.md b/skills/review-task-done/SKILL.md
index 1c148d4..b64c671 100644
--- a/skills/review-task-done/SKILL.md
+++ b/skills/review-task-done/SKILL.md
@@ -10,10 +10,10 @@ Run this before saying a coding task is done.
 ## Command

 ```bash
-metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--evidence <path>]
+metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
 ```

-Use `--base` to define the reviewed diff. Use `--previous-run` when re-reviewing after fixes. Use `--evidence` for validation output such as structured receipts or test logs.
+Use `--base` for the reviewed diff, `--previous-run` after fixes, and `--evidence` for validation output. Use `--max-attempts` only on the first run; it sets the chain budget (default 3), with the first blocker run as attempt 1.

 Prefer structured evidence receipts:

@@ -26,8 +26,8 @@ Freeform evidence remains accepted as a fallback, but receipts preserve command,
 ## Workflow

 1. Run the command from the repository root.
-2. If it exits `1`, open the generated review log and fix every blocking finding.
-3. Re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
-4. Do not claim task completion while unresolved blocking findings remain.
+2. Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
+3. `NEEDS_REVISION`: fix blockers and re-run with `--previous-run <run-id>`.
+4. `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.

 The review updates `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, `docs/metareview/FINDINGS.md`, and Markdown review/context artifacts.

````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["git","diff","--check"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/docs-0-6-release-notes","exitCode":0,"startedAt":"2026-07-05T21:54:05.948288Z","finishedAt":"2026-07-05T21:54:05.970736Z","stdoutSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"git diff --check exited 0"}
{"schemaVersion":1,"kind":"validation","command":["bash","tests/run-all.sh"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/docs-0-6-release-notes","exitCode":0,"startedAt":"2026-07-05T21:54:06.032678Z","finishedAt":"2026-07-05T21:54:28.467494Z","stdoutSha256":"d936305ae705f975d49104fdc460082fa29afb51327e07d77416f73f08894674","stderrSha256":"2bdaf97dbb5c8ec8319c8b5e0a4fc379528af40b511befe365922dbf0e376f6b","summary":"bash tests/run-all.sh exited 0"}
