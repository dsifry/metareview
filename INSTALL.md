# Installation

metareview installs as a local review harness for coding agents. It can run standalone, or as a deeper review layer inside a repository that already uses metaswarm, Superpowers, and Beads.

## Prerequisites

- Git repository checkout.
- Node.js 18+ for the package launcher and manifest tests.
- Go 1.26+ when running from a source checkout.
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

Then use the Codex skills with `$setup`, `$review-task-done`, `$review-epic-ready`, `$review-pr-ready`, `$review-artifact`, `$learn-post-merge`, `$status`, and `$fsm`.

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

Then use `/setup`, `/review-task-done`, `/review-epic-ready`, `/review-pr-ready`, `/review-artifact`, `/learn-post-merge`, `/status`, and `/fsm`.

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

Artifact reviews create a Markdown review scaffold under `docs/metareview/reviews/` with an initial `NOT_REVIEWED` verdict. The default artifact command exits nonzero because the scaffold is not a completed review. Artifact review runs the required lenses as parallel subagents by default; use `in-session-emulated` only when subagents are unavailable or the human requests no delegation, and mark that result as not independently adversarial and weaker evidence. Use `--scaffold-only` only when you explicitly want scaffold creation without passing the gate.

Deterministic lifecycle gates such as `task-done`, `epic-ready`, and `pr-ready` use this result contract: `PASS`/`PASS_ADVISORY` proceed only with zero blockers; `NEEDS_REVISION` repairs via `--previous-run`; `ESCALATED` stops same-target retries; human must narrow, split, or redesign the target. Exit handling: `0` means verify a passing verdict; `1` with a review path means follow that log; nonzero without a path means read stderr. `NOT_REVIEWED` artifact scaffolds are also blocking until completed.

## Agent Workflow

Use the smallest gate that matches the lifecycle point:

```bash
tmp_evidence="$(mktemp)"
metareview evidence run -- go test ./... > "$tmp_evidence"
metareview evidence run -- git diff --check >> "$tmp_evidence"

metareview review artifact <path>
metareview review task-done <task-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
metareview review epic-ready <epic-id-or-path> --base <base-ref> --evidence "$tmp_evidence"
metareview review pr-ready --base <base-ref> --evidence "$tmp_evidence"
metareview learn --post-merge <pr-number> --base <pre-merge-ref>
```

After a GitHub PR exists, append CI receipts with `metareview evidence import --github-checks <pr-number> [--repo <owner/repo>] >> "$tmp_evidence"`.

Task-done and PR-ready parse receipt files as validation evidence; epic-ready reads the supplied evidence text for child-completion signals.

Task-done, epic-ready, and PR-ready context packs include context profiles and generated-artifact filtering. Task-done and PR-ready add a Context Shard Plan and prompt packs when the branch diff exceeds the review context limit, plus a Review Manifest accounting for the shard results ingested; epic-ready renders "not sharded".

Commit durable Markdown artifacts under `docs/metareview/`, including the shard review results under `docs/metareview/shards/`: a later gate reads them back. Keep transient `.metareview/findings.jsonl` and `.metareview/runs.jsonl` local unless a future contract says otherwise, along with the prompt packs under `.metareview/shards/`, which are regenerated per plan and ignore themselves. In ordinary project repositories, prefer exact `.gitignore` entries:

```gitignore
.metareview/findings.jsonl
.metareview/runs.jsonl
```

Do not ignore `docs/metareview/` or the whole `.metareview/` directory. FSM runs (`.metareview/runs/`) ignore themselves; `docs/metareview/fsm/` export bundles are durable. `metareview fsm` needs git ≥ 2.31 (`metareview setup --check` reports it).

## Judge models and providers

The review gates and the `metareview fsm` loop make **judge calls** (adjudicate / verify / symptom
review). The model is chosen per run with `--judge-model` (or `METAREVIEW_JUDGE_MODEL`) and is routed by
name — no code changes needed:

- **Claude** (`claude-*`, `anthropic/*`) → the Anthropic API (`ANTHROPIC_API_KEY`; `ANTHROPIC_BASE_URL`
  overrides the endpoint).
- **Codex/GPT** (`codex/gpt-5.6-sol`, …) → the local **Codex CLI** on its OAuth session under `~/.codex`
  (no API key needed).
- **OpenAI-compatible providers** — GPT (`gpt*`, `openai/*`), **GLM** (`glm*`), **Kimi** (`kimi*`) → a
  `/v1/chat/completions` endpoint. Set `OPENAI_BASE_URL` and `OPENAI_API_KEY`:

  ```bash
  OPENAI_BASE_URL="https://your-openai-compatible-host"  OPENAI_API_KEY="your-api-key" \
    metareview fsm init --workflow sdlc-loop-proved --base <ref> --judge-model glm-5.3-flash --judge-effort medium
  ```

metareview appends `/v1/chat/completions` to `OPENAI_BASE_URL` and gives reasoning models (GLM/Kimi) a
generous token budget automatically. The model that judged a run stays recorded in its snapshot and
`metareview fsm export`. Never pass a secret via `--var`; use the declared env names above.

The `fsm` loop also needs git ≥ 2.31, and runs your project's **real test command** — the first `init`
with a non-default command prints a `cmds_sha256` you consent to with `--allow-custom-cmds <sha>` (a human
decision). Run the loop on throwaway clones; it makes commits. See [USAGE.md](USAGE.md) for the full
workflow.

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
- Review returns `NEEDS_REVISION`: fix the cited blockers and re-run with `--previous-run <run-id>`.
- Review returns `ESCALATED`: stop same-target retries; human must narrow, split, or redesign the target.
