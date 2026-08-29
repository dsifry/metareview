# Turning escalation back on

Escalation is opt-in (`--escalate`) as of the commit that added this file. It shipped **on** in
0.9.0, and a nine-reviewer pass over that release's own additions found nine high-severity defects
in it — of the twelve highs in the whole release, nine were here. The intent is to return it to
default-on; this file is the bar for doing that, so the decision is a checklist rather than a
judgment call made months later by someone reading a comment.

The asymmetry that motivates the feature is unchanged and still correct: unattended, a false
reject is silent — the finding is simply gone — while a false confirm only costs a human a look.
That is *why* the bar is high. A guardrail whose failure mode is "loses findings quietly" has to
be more trustworthy than the thing it guards, not less.

## The bar

Every item needs a **failing test written first** and a **recorded mutation kill** — reverting the
fix must redden a named test. Several of the defects below are ones where a test already existed
and passed for the wrong reason, so "a test covers it" is not sufficient evidence; the mutation is.

### A. The evidence must be the code

1. **Materialized files are whitespace-trimmed.** `escalation.go:99-108` builds the `ShowFunc` from
   `c.git`, which is `strings.TrimSpace(...)` (`wiring.go:38-41`) — a helper written for SHAs and
   ref names, never for file bodies. Every file loses its trailing newline; a file with leading
   blank lines loses those too, moving `func Added` from line 5 to line 3 while the finding's
   `Line` still points at 5. The judge is aimed at the wrong line of a file that matches neither
   revision. **Required:** file bytes reach the tree untouched, proved by a fixture whose leading
   and trailing whitespace is significant.

2. **`diff.noprefix=true` defeats the diff hardening** (`gitcontext`, no `--src-prefix`/
   `--dst-prefix`). An ordinary documented gitconfig setting turns the header into
   `diff --git f.go f.go`; every consumer keys on a `b/` prefix, so `ChangedPaths` returns empty and every
   candidate becomes `unverified_no_evidence`. **Required:** prefixes pinned explicitly, with a
   test that sets `diff.noprefix` in the fixture repo.

3. **Per-candidate selection is applied at three call sites and pinned at one** (`kind.go:529`
   pinned; `:588` escalation and `:715` still-present unpinned). Both mutants leave the whole suite
   green while the judge receives another file's hunks. **Required:** each call site independently
   pinned.

### B. A failure must never read as a verdict

4. **`changedPaths` swallows a nonzero git exit.** `gate.RealExec` reports failures as
   `(code, nil)`, so `escalation.go:86-87` returns `(nil, nil)`, which `kind.go:583` reads as
   "escalation deliberately unavailable: the rejection stands" and records as `hallucination`. A
   rebased or gc'd base SHA exits 128 and silently deletes a real finding. The existing test
   asserts this is impossible but passes for the wrong reason: both fakes return `os.ErrPermission`,
   a channel git failures never use. **Required:** the code-nonzero path surfaces an error, proved
   by a fake in the production shape.

5. **A nonzero `git show` is indistinguishable from "absent at that revision"**
   (`escalation.go:106-108`). With every show failing, `Materialize` writes nothing and returns a
   well-formed `TreeHash` over zero files; the judge is asked to settle a claim inside an empty
   directory while the audit records `evidence=sandbox`. **Required:** absent and unreadable are
   distinct, and an empty tree cannot be presented as evidence.

6. **A duplicate destination is batch-fatal and then run-fatal.** `Materialize`'s own `0o444` makes
   the second `os.WriteFile` return EACCES; dedup is by raw string, so `./internal/x.go` from judge
   prose and `internal/x.go` from `changedPaths` are two keys and one destination. `resolve` caches
   the error under `sync.Once`, so **every** cross-file rejection in the run becomes
   `checked_but_unverified`. **Required:** destination-level dedup, and a run-scope test that one
   bad candidate cannot disable escalation for the rest.

### C. The second opinion must be a second opinion

7. **The escalated judge is never told the tree exists.** Measured: a 2081-byte escalated prompt
   containing none of `head/`, `base/`, `sandbox`, `working directory`, or the path itself. Same
   model, same effort, same `ContextForClaim` excerpt — the only difference is `cmd.Dir`. This makes
   the feature inert, and makes `escalation.go:21-23`'s justification for skipping HTTP judges
   ("would re-ask the same question with the same evidence at twice the cost") a description of
   what the codex path actually does. **Required:** the prompt names the tree and instructs the
   judge to read it, pinned by asserting on the captured prompt.

8. **The containment claim is a comment, not a mechanism** — duplicated in `deps.go:39,107`,
   `judge.go:201`, `codex.go:25`. `cmd.Dir` is a relative-path root; `--sandbox read-only` bounds
   writes and network, not reads. Verified: `codex sandbox -- /bin/ls <repo>/.git` succeeds from
   outside the repo. So `TreeHash`/`BaseSHA`/`HeadSHA` describe what was *offered*, not what the
   verdict rests on, while `run/event.go:115-117` claims they "content-address what the judge COULD
   have read". **Required:** either a real read boundary, or all four comments and the audit
   schema's wording corrected to claim only what is enforced. Do not re-enable while shipped text
   asserts a containment property that does not exist.

### D. Hygiene

9. **The tree is never deleted.** 1015 `mrv-evidence-*` directories (37MB) accumulated on one
   machine in a day. **Required:** removed on run completion, including the failure paths.

10. **Untrusted finding text selects which repository files are copied in.** Judge prose feeds
    `AllReferencedPaths`, which feeds materialization. Bound it to the changed set before
    re-enabling.

## The flip itself

One more item belongs to the flip rather than to the gates above, raised on PR #21: `--escalate` is
read per invocation (`run.go:49`) and is not carried in the run's durable state, so `init
--escalate` followed by a bare `advance --run <id>` silently runs without it. That was masked while
the default was on. Persist it through `InitData`/`Snapshot` and restore it on resume, with a test
covering a real judge transition on a resumed run.

When A–D are done, invert the default in `ctxDeps.escalation` (`wiring.go`), update
`TestEscalationIsOffUnlessRequested`, the agent prompt, `skills/fsm/SKILL.md`,
`docs/fsm/driving-a-workflow.md` and the CHANGELOG together — `TestEscalateFlagIsPinned` pins the
prompt half, and the flip should keep `--escalate` accepted as a no-op rather than retired, so
drivers written during the opt-in period keep working.

Re-enabling should also come with the measurement the SKILL currently asserts without evidence
("roughly 2x the judge tokens on a run where a fifth of the rejections are cross-file"), and with
a run on a real branch showing escalation changing at least one verdict for a reason a human agrees
with. Turning it back on because the checklist is ticked, without ever having seen it work, would
repeat how it shipped the first time.
