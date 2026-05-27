# PR-Ready Review Rubric

Use for `metareview review pr-ready`.

Critical and high findings block PR readiness.

Block when:

- Task, epic, or findings state still has unresolved blockers.
- Validation evidence is missing or does not show a passing result.
- Branch diff review finds critical or high deterministic issues.
- The generated PR evidence section is missing or unreadable.
- Available GitHub review context contains unresolved requested changes or blocker comments.

GitHub context is optional in local mode. Its absence should be recorded as unavailable evidence, not as a blocker.
