# metareview Architecture & Patterns

The map a new contributor (human or agent) should read first, so we stop reinventing wheels. It is a
**reference, not a tutorial** — it points at the deeper docs rather than duplicating them. Keep it current
when the shapes below change.

- Getting started / install: [INSTALL.md](../INSTALL.md), [quickstart](quickstart.md)
- Driving the FSM: [docs/fsm/driving-a-workflow.md](fsm/driving-a-workflow.md)
- Per-harness setup: [README.claude.md](README.claude.md) · [README.codex.md](README.codex.md) · [README.pi.md](README.pi.md)
- Review lenses: `rubrics/*.md`

---

## 1. What metareview is

A **review harness and quality-gate engine for coding agents.** It runs adversarial reviews over artifacts,
code diffs, PR readiness, and post-merge learning; records durable Markdown review logs + JSONL state; and
enforces **review-before-push** through a git-native gate (fail-closed;
`git push --no-verify` is the deliberate escape hatch; a pushed ref that is not the checked-out branch is
blocked rather than silently waved through — see §5). It runs standalone or as a deeper review layer
inside a metaswarm/Beads/Superpowers repo. Distributed as an npm package and as Claude Code / Codex plugins.

**Division of labor (remember this).** Deterministic machinery runs *outside* the model — anywhere, including
git hooks and CI — and emits exit codes + messages. The **agent** reads those, runs the actual adversarial
review (as subagent lenses), records findings/coverage, and lets metareview adjudicate. A hook or the CLI
gate never tries to run the agentic review itself.

## 2. The review types — nested surface areas

Each is a distinct scope, smaller-to-larger, **not** four ways to do the same thing. The **CLI** column is
abbreviated — task-done/epic-ready/pr-ready also take `--evidence <file>` (a validation receipt from
`evidence run`) and other flags; run `metareview review <type> --help` for the full form.

| Type | CLI | Reviews | Engine today |
|------|-----|---------|--------------|
| **artifact** | `review artifact <path>` | a spec/plan/design/doc (no code yet) | **agent lenses** (scaffold → subagents fill rows; `NOT_REVIEWED` until done) |
| **task-done** | `review task-done <id> --base <ref>` | one task's diff (small) | deterministic-local gate ⚠️ |
| **pr-ready** | `review pr-ready --base <ref>` | the whole branch diff (sharded over 120 KB) | deterministic-local gate ⚠️ |
| **epic-ready** | `review epic-ready <id>` | the epic's **integration diff** (base..HEAD, the union of the children's changes), **with the roll-up as context** — child evidence present? contradictions? intent drift? registry coverage? | deterministic heuristics (roll-up freshness) **+ a required adjudicated review** over the integration diff (base..HEAD) via the `epic-review-loop` workflow — same require-lenses gate as pr-ready/task-done |
| **learn** | `learn --post-merge <pr>` | what the merged PR + bot findings teach us | learning extraction |

`epic-ready` runs *after* every child task is task-done-reviewed. It reviews the epic's **integration diff**
(base..HEAD, the union of the children's changes) **with the roll-up — child review logs, evidence, parent
intent — as context**; the roll-up's own freshness is guarded by the deterministic pre-checks (re-read every
run). See `internal/reviewers/epicready.go`, `rubrics/epic-ready-review-rubric.md`.

## 3. Two review engines — and the gap between them

There are **two** engines that produce a review, joined by a **review-evidence marker**:

1. **Deterministic gate** (`review pr-ready` / `task-done`) — `ExecutionMode: deterministic-local`. Its
   "reviewers" are **structural checks**: validation evidence present, no unresolved prior blockers, clean
   working tree, mutation report clean, PR-evidence section readable. **No model call.** Fast, reproducible,
   runs in a git hook / CI. On its own it reports PASS for *structurally clean*, which is **not the same as
   *adversarially reviewed*** — so it no longer stands alone (see the require-lenses gate below).

2. **FSM review-lenses** (`metareview fsm --workflow …`) — the `review-lenses` node runs the adversarial
   lenses as **subagents with a real `$REVIEWER` model**, then `match-then-adjudicate` judges them. This is
   the genuine AI adversarial review. Its findings live in the **FSM run store**
   (`<git-common-dir>/metareview/runs/<id>/audit.jsonl`, carried as `FoldState.Findings`) — **not** in the
   `.metareview/findings.jsonl` that the deterministic gate reconciles and reads. Lens output emitted in
   the typed 0.12 contract (`internal/lensoutput`: tag/file/lines/issue/consequence/confidence/severity)
   is validated deterministically before it can become a candidate: malformed entries are rejected and
   counted, and the **anchor-in-diff gate** (±10 context lines) rejects findings citing files or lines the
   diff never touched — fabricated findings by contract definition. An output with nothing kept and anything
   rejected for its shape, values or anchor fails the node at apply (`lens_all_rejected`, GATE_FAILED → fork and
   re-record) instead of routing the loop to a clean ending, so a malformed review never reads as a clean one — nor
   backs a `record-lenses --from-run` marker (mr-0vk); entries only suppressed below the confidence floor still
   decode. The judge transports themselves carry
   an output-cap retry (a gateway `400` output-limit answer is retried once at 4× the cap — transport
   headroom; prompts and calibration are frozen).

**The bridge (require-lenses gate).** `pr-ready`/`task-done`/`epic-ready` now **require** an adjudicated lens
review by default. After a real review the agent records a **review-evidence marker** —
`metareview review record-lenses --scope <pr-ready|task-done|epic-ready> --lenses <names> [--from-run <fsm-run-id>]` — a record in
`.metareview/runs.jsonl` (`scope="review-evidence"`, `Kind="review-evidence"`) carrying the adjudicated
verdict, confirmed finding IDs, lens set, execution mode, and the **base..HEAD SHAs it reviewed**. The gate
(`internal/reviewers/adversarial.go`) looks up the latest marker for the scope over the **exact
base..HEAD diff** via `reviewstate.CurrentReviewEvidence`; a marker for a stale HEAD *or a different base*
does not count (a review of a narrow `HEAD~1..HEAD` must not be credited for a wider `main..HEAD`) — except that a
marker at an ancestor head still counts when every commit since it adds only gate artifacts
(`reviewstate.IsGateArtifact`: `.md`/`.json`/`.jsonl` files under `docs/metareview/{reviews,context,shards,fsm,learning}/`,
each FSM bundle's own `docs/metareview/fsm/<run>/workflow.yaml`, and `docs/metareview/FINDINGS.md`), so committing a passing gate's own output does not strand the review (#161). Any
other change — code, tests, another doc, a `.go` file placed in those folders — still invalidates it, and a git
failure never counts the marker. It blocks
with `adversarial-review-reviewer` when no current marker is present, blocks when the adjudicated verdict is
not `PASS`/`PASS_ADVISORY`, and emits an **advisory** finding (not a block) when the marker is
`in-session-emulated` rather than `subagent-adjudicated`. Of several markers over one base..head, the
**last-recorded** wins (append order = record order), so a re-review's newer verdict supersedes the older.

The FSM stays scope-agnostic: the **agent** bridges its run into a marker with `--from-run`, rather than the
FSM emitting scope-specific markers. Because a CLI seam cannot witness that independent subagents actually
ran, `record-lenses --mode subagent-adjudicated` is admitted **only** when `--from-run` names an FSM run that
reviewed the same `base..head` — its init, or the head at which its final `clean`/`reviewed` transition passed when its
last review-lenses node reviewed that very head and nothing moved it since (a transition's head is only git's HEAD when
it fired, so a commit made after the last review is never credited), so a fix loop whose last review was clean backs
the commit it made clean (mr-1ad; a `fixed` ending was not
re-reviewed and counts only for its init) — AND reached a passing terminal transition (`clean|reviewed|fixed`);
an empty, wrong-diff, incomplete, failed, or **mock** run is rejected (a run initialised with `--mock-ai`, or one
carrying a mock-stamped event, is test infrastructure and never evidence — #185), and a self-attested review has no such run and
must record the labeled, advisory `in-session-emulated` mode. This keeps a hand-typed one-liner from
laundering a fake review as full-strength independent evidence. A marker attests the committed `base..HEAD`
only, so a `--include-working-tree` run over a dirty tree blocks on a `working-tree-unattested` reviewer
despite a valid marker.

**Escape hatch.** `METAREVIEW_ALLOW_MECHANICAL_PASS=1` opts a single run out of the requirement, restoring
the old deterministic-only pass (`reviewstate.RequireAdjudicatedReview()`). `artifact` review is unchanged —
it still scaffolds `NOT_REVIEWED` and blocks until the agent fills real reviewer rows.

## 4. The FSM — a scope-agnostic review/fix loop

`metareview fsm` drives workflows built from **node kinds**, not review-type states (`internal/fsm/`,
`workflows/*.yaml`, [docs/fsm/](fsm/driving-a-workflow.md)):

- `review-lenses` (`exec: subagent, model: $REVIEWER`) — run the adversarial lenses over a diff/target.
- `match-then-adjudicate` (`exec: fork, model: $JUDGE`) — the judge adjudicates candidate findings.
- `agent-edit` — the fix.
- `still-present` — verify a finding is gone.
- `prove` — mutation-verify a fix (sdlc-loop-proved).

Workflows: `review-loop` (discover → adjudicate → done: a one-shot review, no fix loop), `sdlc-loop`
(discover → adjudicate → fix → verify), `sdlc-loop-clean` (adds `recheck` — **re-review the fix**, loop until
a fresh review is clean; the loop MUST target a review node), `sdlc-loop-proved` (adds `prove`). The loop is
defined by the *graph*, not flags; a loop reset clears findings.

epic-ready is **not** a fix-loop: it reviews child *readiness*, which has no "find-bug-fix-in-loop" shape. Its
adjudicated review is a **one-shot** `epic-review-loop` (a `review-loop` variant) the agent drives over the
epic's **integration diff** (base..HEAD, the union of the children's changes); its `discover` node carries a
`rubric: rubrics/epic-ready-review-rubric.md` param so the epic lenses (integration, acceptance-vs-intent,
cross-child regression, architecture coherence) are configured independently of the pr-ready/task-done lens
set — the `review-lenses` node's rubric is a per-workflow param defaulting to the task-done rubric.
**Division of labor:** the marker attests the integration-diff review (base..HEAD currency); the roll-up's
own freshness (child logs/evidence/intent, which live outside the diff) is guarded by the deterministic
pre-checks in `RunEpicReady`, which re-read current state on every gate run.

**Where a run lives (§6).** A run's audit, sidecars and terminal ledger row live in the shared store in git's
common directory (`<git-common-dir>/metareview/`, #173), whichever worktree ran `fsm init` — so every worktree sees
one store and run ids stay unique; its `RepoRoot` anchor is the main worktree. The run *reviews* its **work dir** (default: the checkout `init` ran in; `--work-dir` overrides),
and a default `fsm export` bundle is written under the checkout that ran `export`
(`export.DefaultOut`), so it is committed on that branch.

Exit contract (`metareview fsm`): `3` = the FSM needs the host to do a node's work; `1`+`GATE_FAILED` = run
`resume_hint` (forks a child = new run id); `1`+`ERR_*` = read `code`; `2` = nothing recorded (fix input and
retry unless it's a consent/escalation code); `STOPPED`/`DONE` terminal. Escalation is per fork lineage.

## 5. The git-native gate

Enforces review-before-push **in git**, not in a command-string parser (which is fundamentally bypassable —
`git commit;git push`, subshells, aliases, eval). See [INSTALL.md](../INSTALL.md) and `hooks/git/`,
`internal/setup/hooks.go`, `internal/githooktest/`.

- **pre-push** = the HARD gate: runs `review gate --push` (deterministic), BLOCKS an unreviewed/unresolved
  branch, **fails closed**; `git push --no-verify` is the escape hatch. It forwards git's pushed-ref stdin to
  the gate (`--pre-push-stdin`) and gates the **pushed refs**: a ref whose local sha equals the checked-out
  HEAD is gated as that branch (the common `git push` / `git push origin HEAD`); a **non-checked-out** ref
  (`git push origin other:main`, `HEAD~3:main`) is **BLOCKED** — the gate measures the checkout and cannot
  verify a different ref's content, so it fails closed with a check-it-out-or-`--no-verify` remedy (issue #82,
  Option B). A pure ref *deletion* is skipped. (Reviewing a non-checked-out ref's *own* content — so a reviewed
  cross-ref push passes without `--no-verify` — is the Option A follow-up.)
- **post-commit** = NEVER blocks (a commit saves work). Names the files it wrote + a review-owed nudge.
- **Commit-always, enforce-at-push:** saving work must never be held hostage to the reviewer being down;
  the enforcement lives at push, where not-pushing loses nothing.
- **Install materializes** (`setup --install-hooks`): the scripts are `go:embed`ded (root `githookassets.go`) and
  written into `${XDG_DATA_HOME:-~/.local/share}/metareview/git-hooks/<repo-id>/` (executable, user-level, one dir per
  repository, named by `metareview.hooksId` in the repository's own config, which moves with it), with
  `core.hooksPath` pointed there (absolute) and verified before "active" is reported. This is what makes the gate work
  in **any** repo, not just metareview's own checkout. It lives outside every repository on purpose (#173): an
  absolute `core.hooksPath` into a checkout or its `.git` goes stale when that checkout moves, and git then runs
  **no** hook, silently; a relative one resolves against each worktree's own root. One dir per repository, because
  `core.hooksPath` makes it git's hooks dir: a hook a user or another tool adds there must not run in other
  repositories. An upgrade rewrites the scripts in place (atomically per file); uninstall only unsets `core.hooksPath`
  and keeps the dir and the id, so a reinstall reuses them. **Ownership.** The dir's `.metareview-owner` names the
  repository (git's common dir) that installed it. A copied checkout (`cp -r`, rsync, a restored backup) carries
  `metareview.hooksId`, so a recorded id is reused only when its owner is this repository or no longer exists (this
  one, moved) — a copy gets its own dir. A repository with no recorded id takes the first `sha256(common dir, n)`
  whose dir is absent or holds none of metareview's scripts: a dir that still holds the gate is never adopted, because
  from a fresh clone `mv repo repo.bak && git clone … repo` (repo.bak still runs from it) and `rm -rf repo && git
  clone … repo` (nothing does) look identical. And nothing ever empties a user-level dir: two live repositories can
  carry one id (a copy whose original then moved, a backup restored over a moved one's old path) and nothing inside
  either tells them apart. The cost is an unused dir left behind (a few KB) after a re-clone in place, an uninstall or
  a migration; `mr-ap2` tracks a prune command. Earlier locations — any `<data home>/metareview/git-hooks/<id>`
  (matched by shape, since `XDG_DATA_HOME` can differ between shells), a pre-#173 `<checkout>/.metareview/git-hooks`
  (removed on migration: it lives inside the checkout), legacy `hooks/git` — are reclaimed only when gone or when
  their `pre-push` carries metareview's content marker; `setup --check` reports one as `stale`, and `--install-hooks`
  migrates it (refusing if it holds other hooks).
- **The Stop gate is opt-in per repository (#194).** The plugin's `hooks/hooks.json` registers
  `hooks/pre-finish.sh` in every host session on the machine, so the script is inert — exit 0, no output,
  before it even looks for the binary — unless the repository it stands in has `metareview.stopGate=true` in its
  *local* git config. `setup --install-hooks` records that opt-in (and `--uninstall-hooks` removes it); an install
  from before #194 is not `AlreadyDone` until the opt-in is recorded; `setup --enable-stop-gate` /
  `--disable-stop-gate` toggle it alone (for a repository whose hook manager owns `core.hooksPath`). A repository
  with metareview's git gate installed but no opt-in is never *silently* ungated: the Stop hook says so on stderr
  and the SessionStart notice names `--enable-stop-gate`. `setup --check` reports `optedIn`, and `active` is false
  for a registered hook that does not gate the repository. Unrelated projects, non-repositories and the FSM
  judge's `codex exec` sessions are never gated (a gate there corrupted judge reasoning, #193).

## 6. State, evidence & storage

- **Durable, committed** under `docs/metareview/`: review logs (`reviews/`), context packs (`context/`),
  shard results (`shards/`), FSM export bundles (`fsm/`), findings render (`FINDINGS.md`). ⚠️ Context packs
  can leak an absolute `cwd` (issue #80) — do not commit a leaking context artifact; the review `.md` is
  clean.
- **`FINDINGS.md` merges as a union (#181).** Two branches that each regenerate it (a new blocker, a new
  override) conflict on a plain merge — both append to the end of the same lists — and a hand-resolved conflict
  in a generated file can silently drop a line. `.gitattributes` marks it `merge=union` (git's built-in driver,
  no per-clone config) for a local `git merge`/`rebase`/`cherry-pick`: both sides' lines are kept, no conflict
  marker. The trade is deliberate: a union never drops a line, but it can keep one a side removed — a blocker
  fixed on one branch next to a line the other branch added comes back — and it keeps both versions of a line
  two branches edited (`[pending]` beside `[granted]`, or a hand-maintained section's line). `FINDINGS.md` is
  display only — no gate reads it; they read the ledger and the review logs — so the residue is a stale line
  shown, never a live one lost, and the next render in a checkout whose ledger knows the finding retires it
  (`TestFindingsIndexUnionResurrectionRetiresAtTheNextInformedRender`); a line no ledger knows stays until it is
  edited out by hand, the carry-over's standing limit. Pinned by
  `TestFindingsIndexMergesWithoutConflictUnderTheRepositoryAttributes` (and the plain-merge conflict by
  `TestFindingsIndexConflictsOnAPlainMerge`). GitHub's web merge is not documented to honor merge drivers: if a
  PR shows a `FINDINGS.md` conflict there, merge `main` into the branch locally, where the union applies. An
  adopting repository gets the same by adding the line to its own `.gitattributes`.
- **Transient, local (git-ignored)** under `.metareview/`: `findings.jsonl`, `runs.jsonl` (review records),
  `shards/` (and `git-hooks/`, from before #173). A `mock: true` FSM run never satisfies a gate.
- **The shared store is in git's common directory (#173).** `repo.StoreDir` = `<git rev-parse --git-common-dir>/
  metareview/`: FSM runs (`runs/<id>/`), their terminal ledger (`runs.jsonl`; run ids are store-unique,
  `record.Exists` checks it; `FSMRunDir` is relative to the common dir), the migration lock, and the session
  bindings (`sessions/`, #166). One store for the main checkout and every linked worktree; it works in a bare
  repository too — a command run from a linked worktree of a bare repository anchors on that worktree (#174) —
  and it survives `git clean -fdX`, a moved or deleted main checkout, and every git maintenance command
  (AC-2.8 pins it); clones do not copy it. **Migration:** the first `fsm` command after upgrading moves a 0.13.x
  store (`<main>/.metareview/runs/<id>/`) in under an exclusive lock — byte-identical, idempotent, never
  overwriting (an id in both places is a `STORE_COLLISION` warning, both copies kept) — and copies the legacy
  `fsm-*` ledger rows (read leniently — it is the main checkout's live review ledger — and skipped once migrated via
  a size stamp, `legacy-ledger.size`). For one release `record-lenses --from-run` and the `status` abandoned-run scan
  also read that single legacy location (never another worktree's), and `status` warns while it holds runs.
- **Branch scope (#177, `internal/scope`).** The abandoned-run scan classifies each run against the branch in hand,
  one rule in one package: a run recorded on branch N at head H is **in scope** when N is the current branch or one
  of its former names — the `git branch -m` / `-c` entries its reflog carries, while no live branch holds that name —
  (the name leg: survives rebase, amend and rename; mid-rebase the branch being rebased is current; a rebase begun detached is no branch), or when H lies in
  `merge-base(HEAD, main|master)..HEAD`, less anything a remote's default branch already has — the branch its `refs/remotes/<remote>/HEAD` names, and
  its `main` and `master` (the range leg:
  detached snapshots, stacked branches; the exclusion keeps a stale local main from pulling merged branches' runs
  into a branch cut from a fresh origin/main). Otherwise it is
  **other-branch** while N exists, else **orphaned**; neither blocks, and `status --all` lists both, each with the
  run directory to delete, without changing the exit code. Branch names are compared as full refnames, so a
  same-named tag cannot unmatch them, and spelled as git lists them (`scope.Canonical`: on a case-insensitive
  filesystem `git checkout Feat` lands on `feat` with HEAD spelled `Feat`; init records, and status compares, `feat` —
  folded only when git resolves that spelling, so on a case-sensitive one an unborn `Feat` stays its own branch). `fsm init` records `branch` in its init event: the checked-out branch; on a
  detached HEAD `--for-branch` is required and must name a local branch exactly as git lists it; on an attached one
  it may only restate it. **Fail closed:** any git call `Load` makes that fails (other than git's own "detached")
  leaves the scope unknown, and an unknown scope puts everything in scope. **Legacy runs** (before #177, no branch)
  are in scope unless git shows their head belongs nowhere here: out of the range, not one of the current branch's
  past reflog heads, and unreachable from HEAD (`merge-base --is-ancestor` exit 1) or pruned — so an upgrade never
  silently clears one, but a pre-#177 run abandoned on main now blocks every branch forked after it until its
  directory is deleted or it is closed (#179, below). `scope.Load` makes a fixed number of git calls however many runs there are (AC-4.9); only
  legacy runs ask more, up to two calls per distinct head. **Known trade-offs:** a deleted branch name recreated for
  unrelated work inherits the old name's runs (the name leg), and after a rename, recreating the old name hands the
  runs recorded under it to the new branch, where they still block; `git checkout -b new` after a rewrite leaves the run
  with the old branch, where it still blocks (and it is counted here); a repository that keeps no branch reflogs (a
  bare one's default), or whose reflogs were expired, has no former names, so a rewrite then a rename orphans a run
  there; from other checkouts a renamed branch's run reads as orphaned (only the renamed branch reads its own
  reflog); a stack rebased as a whole keeps its base branch's runs on the base branch only; the range leg needs a
  local `main` or `master`. a squash-merged (then deleted) lower branch of a stack keeps blocking the upper branch through the range leg until
  it is rebased `--onto` main (the blocker names the lower branch); a detached HEAD other than a rebase (bisect, an
  inspection checkout) has no name leg; "orphaned" means the recorded branch is gone, even when a live stacked branch
  still holds its commits — so `git switch -c copy`, `git branch -D orig`, then a rewrite orphans orig's items on the
  copy (git state cannot tell that copy from a stacked branch whose merged parent was deleted, which must not block;
  use `git branch -m`, which the name leg follows). Clearing a stale run is the closing operation's job. **Findings (#178)** go through the same
  rule: the per-checkout ledger (`.metareview/findings.jsonl`) records each finding's `branch` (`scope.Load`'s current
  branch, stamped by `findings.Reconcile`) beside its `gitHead`, one row per branch. `Reconcile` refreshes only the rows
  the branch owns by name (`scope.Owns`: its name, or a former one after a rename) and never re-stamps a branchless row
  (legacy, or a detached review — no run moves one: its head is what ties it to the branches that contain it; only where the
  scope is unreadable and no branch is checked out — outside a repository, or a detached HEAD after a git failure — are
  rows refreshed as before #178, never re-stamped). A named run
  deduplicates against a branchless row only when that row's head is one of its own reflog heads (`scope.PastHead`),
  else it records its own row; a granted override that gates the branch absorbs a re-raise (only with a readable scope,
  where "gates" means something). A finding raised
  again on a stacked or throwaway branch is that branch's own row and never takes the first branch's. Its verdict (the
  open findings task-done and epic-ready count) holds only rows that gate this branch (`Classify`), and a
  `--previous-run` chain closes any row it names, whichever branch recorded it (as before #178: the chain is the
  explicit repair path, so a fix, stacked or epic branch can close what it inherits, and a deleted branch's row is
  never stranded) — and `findings.ScopedBlocking` / `UnresolvedBlocking` classify every unresolved blocker, so after `git
  switch` branch B's pr-ready no longer blocks on branch A's open findings — unless B is stacked on A and still carries
  the commits they were raised on (the range leg) — and lists them as "Open on other branches: N" under Repository
  Health Advisory. Fresh mutation evidence supersedes a stale freshness row of this branch or of none (orphaned: its
  branch merged and deleted), never another live branch's. Epic-ready names its child tasks explicitly and reads their blockers across
  branches (`UnresolvedBlockingAllBranches`): a child reviewed on its own branch and squash-merged into the epic's would
  otherwise be dropped. Rows from before #178 carry no branch and take the legacy rule; so does a row an older binary
  rewrote (it drops the field it does not know — version skew is #180). A NEEDS_REVISION review *log* merged into main is committed
  evidence, not the ledger, and still blocks a later branch whose diff overlaps it — the stale committed-log blocker
  an override clears (#188).
- **Store vs anchor vs work (#169, #172, #173).** Every `.metareview`/`docs` path in `internal/fsm` names which it
  means. **Common dir** = the shared store above. **Store root (anchor)** = the main worktree (`git worktree list
  --porcelain`, `repo.MainWorktreeFromPorcelain`), or with a bare main the linked worktree the command runs in (#174):
  a run's `RepoRoot` — mock scenarios, escalation evidence and
  export paths resolve against a real checkout — and the 0.13.x store location. **Work root** = the checkout the
  command runs in (`rev-parse --show-toplevel`): a run's default work dir, the diff base..head, and work output
  meant to be committed on that branch — a default `fsm export` bundle lands in the *requesting* worktree's
  `docs/metareview/fsm/`. In a single checkout anchor and work root coincide. Tripwires, not proofs (they match
  literal path forms only): `TestFSMRootsAreDeclared` (a `root: store|work` declaration at each such site in
  `internal/fsm`; a common-dir site must say `root: store (git's common directory)`, and a site that is no store
  path at all may say `root: none`) and `TestRunStoreReadersAreDeclared` (run-store readers outside the FSM, declared
  by `RunStoreRoot`, `StoreDir` or a `run-store:` comment).
- **One base resolver (#175):** `internal/baseref` decides what an explicit `--base` means for every gate,
  `record-lenses`, `context diff` (via `gitcontext.resolveBase`) and `fsm init` (via `gate.Git.ResolveBase`). A
  branch name (`main`, `origin/main`, `refs/heads/…`, `refs/remotes/…`) is `merge-base(HEAD, <branch>)` — the fork
  point, so an advancing base never folds its new commits into the reviewed diff; a SHA, tag or revision
  expression (`HEAD~2`) is that exact commit. A short name that is both a branch and a SHA prefix is the branch
  (git's precedence), a branch that shares a tag's name is the branch, and a full 40/64-hex string is always the
  commit. With no merge-base in a shallow clone the error says to fetch full history. No `--base` keeps each
  command's default.
  Records store the SHA (`baseSha`/`base_sha`, which every match compares) and the base as typed
  (`requestedBase`/`requested_base`, never matched on). A pre-#175 marker keeps matching whenever its recorded
  SHA equals what the base resolves to now — a SHA `--base`, or a branch that has not moved past the fork point.
  One recorded with `--base main` after main advanced holds main's tip, a different diff, so it no longer matches
  and the review is re-recorded.
- **Incremental review, `--base last-reviewed` (#176):** a reserved token for task-done, epic-ready, pr-ready and
  `record-lenses`; `review checkpoint --scope <s>` prints what it resolves to. `reviewstate.Checkpoint` derives it
  from the review-evidence markers in this checkout's `runs.jsonl` (no new state): the **nearest** head that is a
  *strict* ancestor of HEAD, whose **latest** marker of the scope passed (a later NEEDS_REVISION at the same head
  withdraws an earlier PASS, as the gate's last-recorded-wins rule does), and whose marker's base reaches back to
  the fork point (`gitcontext.ForkPoint`: the merge-base with a local main/master — never HEAD itself or the HEAD~1 fallback; with
  no fork point the token is refused) — directly, or through a chain of qualifying heads in this history. So a checkpoint always vouches
  for fork..checkpoint, and a marker recorded over a narrow base, before a rebase, or with no base (pre-#175) never
  qualifies. A marker at HEAD is skipped (after recording C3..C5 the token still resolves to C3, so the gate finds
  that marker); a marker whose head no longer exists here (pruned after a rebase) is skipped. None → exit 2 before
  anything is recorded. An incremental **pr-ready** reviews only checkpoint..HEAD but scopes its **blockers** to the
  whole branch (`prready.Options.Incremental`), so an open finding on a file changed before the checkpoint still
  blocks. Every incremental run and marker records `requestedBase: last-reviewed` beside the checkpoint SHA, and the
  context pack says so. `fsm init` takes no scope: pass `--base $(metareview review checkpoint --scope pr-ready)`.
  Known limits: after merging main into the branch, checkpoint..HEAD includes the merged upstream changes, so the
  increment is wider — never narrower — than the branch's own work (tracked by #182). A later NEEDS_REVISION at a
  *descendant* head does not withdraw an earlier checkpoint (only a later marker at the same head does), and a
  checkpoint resting on an in-session-emulated marker is not surfaced as the whole-branch gate's emulated advisory.
- **Stale task reviews (#187):** pr-ready and `status` automatically retire only task-done and epic-ready reviews
  whose target is exactly `--help` or `-h` (artifacts of the bug #164 fixed, never reviews of work), through one
  shared predicate, `reviewstate.FlagTargetRunIDs`. The task-done and epic-ready CLIs now refuse any target starting
  with `-`, so no new such logs can be written. pr-ready does *not* infer from heads or branches that a review
  covered someone else's landed work: task-done also reviews uncommitted changes and records only HEAD, so any such
  inference fails open under rebases, renames, detached checkouts or a moving base. Any other stale blocker is
  meant to be cleared by a human-granted process override, which reaches blockers that exist only in committed
  review logs (#188, below).
- **Run lineage:** a NEEDS_REVISION parent is retired when a clean same-target+same-kind child links via
  `previousRunId` (supersede). Repair via `--previous-run <run-id>`; never `git add -A` failed-run artifacts. A chained
  pr-ready run's lineage also holds every earlier pr-ready run (with an authenticated run record) of the same target over the same base..head as a run in
  its chain (mr-mrf): a standalone re-run at that diff was the same review, so the repair chain can close its findings.
  A run with no `--previous-run` adopts nothing — a fresh look at an unchanged diff is never a fix.
- **Evidence receipts:** `evidence run -- <cmd>` records a validation receipt (kind + exitCode + hashes);
  `evidence import --github-checks <pr>` pulls CI. task-done/pr-ready require a passing validation receipt.
  Freeform evidence text (no receipts) passes only with a success signal and no failure signal, and failure reading
  fails closed (`internal/evidence` failurePatterns, run after ANSI escapes are stripped): any "failed", upper-case
  `FAIL`/`FAILURE`/`FAILURES`, a "fail" verdict ("Result: Fail", `"status":"fail"`, `# fail 1`), `Failures:`/
  `Errors:` with a nonzero count, `N failing`, `N errors` ending a clause or followed by in/during/generated/found/
  and, pytest `ERROR` lines, `Traceback`, `TypeError:`-style exception lines, `panicked at`, `Segmentation fault`,
  `Killed`, TAP `not ok` (indented too), make `Error N`, `error TS…`/`error CS…`/`error[E…]`, `npm ERR!`, go
  `file.go:L:C:` diagnostics, golangci-lint `N issues:`, `error:`, and a nonzero or negative exit in any common shape.
  "fail" in any case counts as the base reader's `(?i)\bFAIL\b` did ("Status: Fail because timeout", "lint: fail (3
  warnings)", "3 tests fail"), except a path segment, file name or compound (`TestX/fail`, `fail.test.ts`, Fail-safe).
  Exempt (mr-r3y), and nothing else:
  (1) a lower-case "fail" in prose — after a hypothetical or negated modal ("should fail (3 ms)", "doesn't fail",
  "expected to fail") or after a subject word and before against/without ("the new tests fail against origin/main");
  never upper or title case, never "did/does/continues to fail", never after ":"/"=", never when counted ("2 tests
  fail", though a modal "1 should fail" stays prose);
  (2) a clause-initial zero report ("…, 0 failed", "no tests failed", bun "0 fail", ctest "0 tests failed out of 5");
  (3) a zero label that ends there ("Failed: 0, Passed: 5", "# fail 0", "failed=0 skipped=0");
  (4) unittest's `expected failures=N` ("OK (skipped=1, expected failures=1)").
  ANSI colour codes (";"- or ":"-separated) are stripped first.
  A zero that does not start a clause ("shard 0 failed", "Passed: 0 Failed: 3") is a failure, and so is prose such
  as "TestX failed before the fix" — prefer receipts. Tool shapes neither reader recognizes are tracked in mr-b08.
- **Sharded review** (exclude-filtered diff > 120 KB): the gate writes prompt packs under
  `.metareview/shards/…/plan.json`; review one subagent per shard + a cross-shard pack, write results, re-run
  with `--previous-run`. Editing a file invalidates only its own shard.
- **Overrides** (`override request` / `grant`): requesting does NOT clear the gate; granting must come from
  **outside** the workflow (a human/authority) — the requester cannot grant. `--by` is audit metadata, not
  authentication. An override is never a fix (`fixedInRunId` stays empty). **Closing an abandoned FSM run (#179):**
  `override request|grant <run-id>` on a run of the store left in a non-terminal state files a closure row
  (`findings.AbandonedRunRecord`: fingerprint `fsm:abandoned-run:<id>`, the run's own branch and init head, advisory —
  bookkeeping no review gate counts). A request alone leaves the run blocking `status` (it names who asked); once
  granted by another actor, `status` drops it for every branch and `--all` lists it with Scope `closed`, actor and
  reason, and the closure renders under Process Overrides (carried over by the run's `mrv-` ID). The row records when the
  run last moved (`runUpdated`: its last event other than an `fsm record` note): a grant closes the run only as it
  stood, so a run resumed since blocks again and its stale closure can be requested and granted afresh, while a
  `stopped` note never reopens it. A request made before the run moved no longer describes it: status stops naming it,
  and a grant then is a direct decision on the run as it stands. `fsm record stopped` stays an annotation that never removes
  a run. The ledger is per checkout, so a closure granted in one worktree does not close the run in another. An ID with
  no ledger row is looked up in the
  committed review logs (#188): every log listing it under `## Blocking Findings` supplies it **and all its other
  blockers** (pr-ready clears a log once every ID the ledger knows is resolved, so importing one alone would let its
  grant retire the rest). The scan runs on every override, not only for an unknown ID — a finding imported as one log's
  sibling may be listed by another log whose own blockers are still unknown — and never takes pr-ready's derived
  "Unresolved review blockers" summary as a sibling (every pr-ready run re-derives it). Each is imported as this branch's open row at HEAD — its run is the one its ID names, taken
  from that run's own log where committed, else from a log that carries it forward; header fields are read above the
  first `## ` only — with fingerprint `imported-review-log:<id>` (never a live finding's) and the log as evidence. A
  `--previous-run` chain naming its run closes it as fixed. An ID found nowhere exits 1. Unscoped `status` still
  lists a committed log with no local run record (#147: the ledger never clears an unauthenticated log).

## 7. Cross-agent integration

The portable core is the **Agent Skills standard** (`skills/<name>/SKILL.md` with `name`/`description`
frontmatter) — metareview already ships it, and it serves **Claude Code**, **Codex**, and **Pi** with the
same files. MCP is the *second* surface for the MCP-speaking long tail (OpenCode, Cursor, Cline, Zed) but
**cannot reach Pi**, which is CLI+skill by design. So: Skills reach Pi & co.; an MCP server (not built yet)
reaches the GUI/TUI agents; thin per-agent command wrappers on top.

- **Claude Code / Codex:** `.claude-plugin/` + `.codex-plugin/` manifests; marketplace is the metareview
  repo itself (`.claude-plugin/marketplace.json`, which Codex accepts as legacy-compatible). Skills → `/name`
  (Claude) or `$name` (Codex).
- **Pi** (`github.com/earendil-works/pi`): point it at the skills under `.agents/skills/` or
  `~/.agents/skills/`; invoke `/skill:name`. No manifest needed.
- **metaswarm** is a *separate* installable orchestration
  (**[github.com/dsifry/metaswarm](https://github.com/dsifry/metaswarm)**), not metareview. `setup --check`
  also detects a local sibling checkout at `../metaswarm` for development convenience, but the canonical
  source is the repo URL. In a metaswarm repo, metareview is the deeper review gate; do not replace Beads
  task state or metaswarm PR shepherding.

## 8. Package map

Grouped by role (mostly `internal/`; not exhaustive — `run ls internal/` for the full set, and the `fsm/`
list below is illustrative, omitting e.g. `judge`, `gate`, `converge`, `export`):

- **Entry / dispatch:** `cmd/metareview` (CLI), `internal/repo` (root detection), `internal/version`.
- **Review types:** `artifactreview`, `taskdone`, `prready`, `epicready`, `reviewers` (the deterministic
  reviewer lenses), `lens` (the lens set), `lensoutput` (the typed lens-output contract: `TypedFinding` +
  `Validate` + the anchor-in-diff gate ±10 — the deterministic, pre-LLM validation layer with lab-mirrored
  rejection buckets; see `internal/lensoutput`'s package doc and the conformance corpus in
  `tests/go/test-lens-conformance.sh`).
- **Review state & logs:** `reviewlog` (parse/discover `.md` logs), `reviewstate`, `reviewmanifest`,
  `findings`, `runchain` (lineage), `state`/`jsonl` (append/scan), `reviewprompt`.
- **Gate & install:** `setup` (mode/prereqs + hook install), `status` (branch scope, `CommitGate`/`PushGate`,
  `BuildForBranch`, coverage/unreviewed), `session` (binds a host session to the worktree its work is in, so
  the Stop hook `hooks/pre-finish.sh` evaluates that worktree rather than the checkout the host launched in —
  hosts such as Codex report only the launch checkout), `githooktest` (black-box hook tests), `covergate`
  (floor gate).
- **Context & evidence:** `gitcontext` (exclude-filtered diff), `githubcontext`, `contextpack`,
  `contextprofile`, `shardpack` (shard packs), `evidence`, `mutation` (Stryker/gremlins report), `knowledge`,
  `markdown`, `classify` (file class), `testconv` (test-file convention).
- **FSM:** `fsm/{cli,machine,workflow,run,record,sandbox,kind}`.
- **Sources & learning:** `tasksource`, `epicsource` (Beads etc.), `learning`, `learnsource`,
  `sessionhistory`, `integration` (metaswarm).

## 9. Build process & conventions

- **TDD, dependency injection, mock-AI, enforced 100% coverage.** `make cover` (logic in
  `internal/covergate`) merges unit + a behavioral shell suite via `go tool covdata`, then requires **every**
  package `go list ./...` reports to be at exactly 100% of statements, **except** the packages named in
  `tests/coverage-exclude.txt` (the embed-only root package, `internal/version`, `cmd/covergate`, and the
  black-box `internal/githooktest` — each genuinely statement-free or a value-less delegate). A new package is
  required at 100% by default (fail-closed: an untested new package fails the gate — the way `internal/mutation`
  once shipped an entire subsystem ungated); excluding one is a deliberate, commented line. A package with **no
  statements** stays out of the profile (e.g. the embed-only root package) — exclude it rather than trying to
  cover it. (History: the gate was a ratcheting per-package floor in `tests/coverage-floor.txt` plus a sibling
  bash gate `tests/coverage.sh`; both were removed once the repo-wide 100% campaign brought every package to
  the bar.)
- **The command-seam DI pattern:** git access goes through an injectable `RunGit`/`GitRunner` func so logic is
  hermetically testable without a real repo; `nil` uses the real binary. Mirror it for any external command.
  For the pinned-rev evidence seams (grep at a rev, blob reads — the covering-test search and the escalation
  sandbox), use the shared constructors in `internal/fsm/judge` (`GrepSeam`/`ShowSeam`) rather than re-implementing
  the contract per call site; the rules (exit 1 is no-matches, `-z` NUL parsing, ls-tree absence vs failure,
  byte-exact blob reads) live and are tested there once.
- **Embed + materialize:** ship scripts/templates via `go:embed`, write them into the target on demand,
  verify before claiming success (see the hook install). Don't assume files exist on disk in a consumer repo.
- **Fail closed:** when the gate can't tell (unresolvable scope, unreadable state, a git error listing what a
  commit writes), block — never wave through.
- **Mutation testing (gremlins):** the reliable, complete-verdict config is `--workers 1 --timeout-coefficient
  30`; `--workers 8 --timeout-coefficient 120` is a faster config with a few flaky timeouts. Timeouts are
  recompile contention, not real survivors. 100% line coverage still leaves killable mutants — construct the
  distinguishing test before calling a survivor equivalent.
- **Mutation evidence freshness (StrykerJS):** the change-driven harness is a zero-dependency Node template in
  `templates/mutation-incremental/` that projects copy to `tools/mutation-incremental/`. The repository owns
  what runs; metareview owns the attestation contract (`<stateDir>/attestation.json`, spec §5.5) and the
  glob dialect (vectors in `testdata/mutation-incremental/`). CI enforces the project's own bar (threshold
  and/or verifier); the gate judges only freshness. The template's Node suite runs under `go test` at 100%
  coverage (`mutationtemplate_test.go`), and its workflow has a static test (`mutationworkflow_test.go`). The
  real-Stryker proof is local-only (`tests/e2e-mutation-incremental.mjs`). Guide: `docs/mutation-harness.md`.
- **Review-first, then bots:** run metareview's adversarial review BEFORE opening the PR, so CodeRabbit/Cursor
  measure only the *residual* we missed — the recall yardstick. Reviewing after the PR opens confounds it.

## 10. Gotchas that have bitten us

- "The repository root" is two roots once linked worktrees exist (§6). A path built from the wrong one works in
  a single checkout and silently breaks in a worktree (#169: runs written to main, read from the linked tree).
  Declare the root at the site; the tripwire tests fail otherwise.
- A single-package test fixture hides multi-package `go test ./...` classification bugs — key on the target's
  own `-v` markers.
- A mock judge ignores model/effort — a node that calls the judge needs `model` in the YAML or it's DOA in
  prod.
- Differential proof binds against `base..head`, not the fix diff — bind against `FixEntryHead..head`.
- task-done scans untracked files: a finding's own TODO text can self-reference; clear via `--previous-run`
  to the opening run, and an untracked file over 4,000 bytes raises `UNTRACKED_TRUNCATED`.
- The module-root package is embed-only (no statements) so it stays out of the coverage profile — keep it
  that way and leave it in `tests/coverage-exclude.txt`; its embed integrity is guarded by
  `githookassets_test.go` instead of by coverage.
