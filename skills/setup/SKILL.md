---
name: setup
description: Detect repository mode and prepare metareview review artifacts without overwriting metaswarm or Beads-owned files.
---

# metareview setup

Use when the user asks to set up metareview, check installation status, or prepare a repository for internal review gates.

## Workflow

1. Run `metareview setup --check`.
2. Report detected mode: `metaswarm-extension`, `standalone-full`, `standalone-minimal`, or `advisory`.
3. In metaswarm extension mode, do not patch metaswarm core files automatically.
4. In standalone modes, run `metareview setup --bootstrap-prereqs --dry-run` before proposing any Beads, Superpowers, metaswarm, or `docs/SERVICE_INVENTORY.md` bootstrap.
5. Use `templates/SERVICE-INVENTORY.md` only when no equivalent registry exists and setup is approved.

## Safety

- Append or create only.
- Do not overwrite `AGENTS.md`, `CLAUDE.md`, `.beads`, or metaswarm files without explicit user approval.
- Keep setup report factual and include exact missing prerequisites.
- Non-dry-run prerequisite bootstrap requires explicit confirmation.
