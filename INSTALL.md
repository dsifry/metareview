# Installation

metareview installs as a local review harness for coding agents. It can run standalone, or as a deeper review layer inside a repository that already uses metaswarm, Superpowers, and Beads.

## Prerequisites

- Git repository checkout.
- Node.js 18+ for the package launcher and manifest tests.
- Go 1.22+ when running from a source checkout.
- Optional: metaswarm, Superpowers, Beads (`bd`), and GitHub CLI (`gh`) for full lifecycle integration.

## npm Package

```bash
npm install -g metareview
metareview setup --check
```

Packaged releases include a built `bin/metareview` binary. The launcher uses that binary first.

## Source Checkout

```bash
git clone https://github.com/dsifry/metareview.git
cd metareview
npm install
npm run build
./bin/metareview setup --check
```

For development without a built binary:

```bash
go run ./cmd/metareview setup --check
```

## Codex Plugin

Install from the GitHub marketplace manifest:

```bash
codex plugin marketplace add dsifry/metareview
codex plugin add metareview@metareview
```

Restart Codex after installation so the plugin skills are loaded.

For local CLI development, use the source checkout flow above. To validate the Codex marketplace metadata from a local checkout without installing globally:

```bash
codex plugin marketplace add /path/to/metareview
codex plugin list --marketplace metareview
```

Then use the Codex skills with `$setup`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$review-artifact`, `$learn-post-merge`, and `$status`.

## Claude Code Plugin

Install from the GitHub marketplace manifest:

```bash
claude plugin marketplace add dsifry/metareview
claude plugin install metareview@metareview
```

For local development:

```bash
claude plugin marketplace add /path/to/metareview
claude plugin install metareview@metareview
```

Then use `/setup`, `/review-task-done`, `/review-epic-ready`, `/review-pr-ready`, `/review-artifact`, `/learn-post-merge`, and `/status`.

## Standalone Setup

From a repository that does not already use metaswarm:

```bash
metareview setup --check
metareview setup --bootstrap-prereqs --dry-run
```

Review the dry-run output before applying prerequisites. Non-dry-run bootstrap is intentionally explicit because it may introduce Superpowers, Beads, metaswarm-compatible instructions, and a `docs/SERVICE_INVENTORY.md` registry when no equivalent exists.

## Metaswarm Extension Setup

In repositories that already use metaswarm, metareview should extend the existing workflow instead of replacing it:

```bash
metareview setup --check
```

Expected mode is `metaswarm-extension` when metaswarm/Superpowers/Beads signals are present. Keep metaswarm as lifecycle owner. Use metareview for deeper artifact, task-done, epic-ready, pr-ready, and post-merge learning gates.

## Verify Installation

```bash
metareview --version
metareview setup --check
metareview review artifact docs/quickstart.md
```

Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Artifact review runs the required lenses as parallel subagents by default; use `in-session-emulated` only when subagents are unavailable or the human requests no delegation, and mark that result as not independently adversarial and weaker evidence. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate. Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` report `PASS`, `PASS_ADVISORY`, or blocking findings. Treat every blocking finding and every `NOT_REVIEWED` artifact as open work until a re-review clears it.

## Agent Workflow

Use the smallest gate that matches the lifecycle point:

```bash
metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence <file>
metareview review epic-ready <epic-id-or-path>
metareview review pr-ready --base <base-ref>
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

Commit durable Markdown artifacts under `docs/metareview/`. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless a future contract says otherwise. In ordinary project repositories, prefer exact `.gitignore` entries:

```gitignore
.metareview/findings.jsonl
.metareview/runs.jsonl
```

Do not ignore `docs/metareview/` or the whole `.metareview/` directory.

## Update

For package installs:

```bash
npm update -g metareview
metareview --version
metareview setup --check
```

For source checkouts:

```bash
git pull
npm run build
bash tests/run-all.sh
```

## Troubleshooting

- `No packaged metareview binary or Go source checkout found`: run `npm run build` in a source checkout or install a packaged release.
- `setup --check` reports advisory mode: install or configure metaswarm, Superpowers, and Beads if full lifecycle integration is desired.
- Review returns blockers: fix the cited issues and re-run with `--previous-run <run-id>` until the verdict is `PASS` or `PASS_ADVISORY`.
