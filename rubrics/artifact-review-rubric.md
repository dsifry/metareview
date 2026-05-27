# Artifact Review Rubric

Use this rubric for specs, plans, designs, decompositions, architecture docs, pre-mortems, runbooks, and acceptance reports.

## Verdicts

- PASS: no blocking findings.
- NEEDS_REVISION: one or more blocking findings.
- ESCALATE: human decision required.
- NOT_APPLICABLE: the reviewer lens does not apply.

## Required Lenses

### Feasibility

- Verify paths, commands, dependencies, and stated prerequisites against repository reality.
- Block on fabricated paths, impossible ordering, missing tools, or invalid commands.

### Completeness

- Map user/spec requirements to artifact sections.
- Block on missing acceptance criteria, missing verification, or unhandled obvious edge cases.

### Scope And Alignment

- Check whether the artifact solves the stated intent without unrelated expansion.
- Block on scope drift, under-scoping, or implementation work not traceable to requirements.

### Architecture

- Check boundaries, ownership, duplication risk, registry impact, and integration shape.
- Block on parallel service paths or contradictions with existing architecture.

### Intent Preservation

- Compare final artifact direction against original intent and accepted constraints.
- Block when review iterations changed the objective without explicit human acceptance.

## Evidence Rules

Every blocking finding must cite at least one concrete source:

- file path and line
- command output
- artifact section
- git SHA
- Beads task ID
- review log section

Session-derived facts are hints unless the finding is about intent or process history.
