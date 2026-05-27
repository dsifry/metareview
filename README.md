# metareview

Local-first review gates and learning for specs, plans, code, epics, PRs, and post-merge follow-up. Metareview is Go-backed, Markdown-friendly, and designed to run standalone or as a deeper review engine inside metaswarm/Superpowers/Beads workflows.

Start with [docs/quickstart.md](docs/quickstart.md). Installation details are in [INSTALL.md](INSTALL.md), Codex-specific usage is in [docs/README.codex.md](docs/README.codex.md), and Claude Code usage is in [docs/README.claude.md](docs/README.claude.md). The static GitHub Pages entrypoint is [docs/index.html](docs/index.html).

## Install Shape

Packaged releases should provide `bin/metareview`; source checkout mode requires Go 1.22+ and falls back to:

```bash
go run ./cmd/metareview
```

Check the repository mode and prerequisites:

```bash
metareview setup --check
metareview setup --bootstrap-prereqs --dry-run
```

See [INSTALL.md](INSTALL.md) for package, source checkout, Codex plugin, Claude plugin, standalone, and metaswarm-extension installation paths.

## Core Commands

```bash
metareview review artifact <path>
metareview review task-done <task-id-or-path>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

Review artifacts are written under `docs/metareview/`. Local transient state stays under `.metareview/`.

## Agent Contract

Coding agents should run the smallest applicable gate before claiming completion:

- `artifact` for specs, plans, designs, decompositions, and docs.
- `task-done` after each local code chunk.
- `epic-ready` when child work is complete.
- `pr-ready` before push, PR creation, or merge.
- `learn --post-merge` after a confirmed PR merge.

Any blocking finding must be fixed and re-reviewed. `PASS_ADVISORY` is acceptable only when the review reports zero blockers.
