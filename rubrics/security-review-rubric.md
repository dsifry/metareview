# Security Review Rubric

Use this rubric for the **Security** lens of an artifact/code review. The Security lens hunts
for security vulnerabilities the change introduces or fails to prevent — the OWASP classes a
diff-review can see (no runtime/audit-log checks). This is a *diff-scoped* lens: judge whether
the change preserves or weakens security, not whether the whole codebase is hardened.

Mined from metaswarm's `rubrics/security-review-rubric.md` (OWASP Top 10, 2021 edition),
scoped to the classes a diff review can verify: A01-A05, A07, A08, A10, plus XSS/escaping.
A06 (vulnerable/outdated components) and A09 (logging & monitoring failures) are deliberately
excluded as non-diff-reviewable (A06 needs a dependency-manifest audit + external CVE DB; A09 is
runtime/config posture, rarely visible in a diff).

Per docs/METAREVIEW_IMPROVEMENTS.md H1: metareview's original 5 lenses were all artifact-shape
checks (Feasibility, Completeness, Scope, Architecture, Intent); none prompted a reviewer to
look for vulnerabilities, so metareview under-recalled on security goldens vs vanilla. This 6th
lens closes that gap at ~zero marginal cost (lenses are Haiku subagents — ~0.04% of cost; the
orchestrator dominates — see SPEC §6.3.1).

## Verdicts

- PASS: no blocking security findings.
- NEEDS_REVISION: one or more CRITICAL/HIGH security findings.
- ESCALATE: a finding's exploitability depends on runtime/context the diff can't settle.
- NOT_APPLICABLE: the diff touches no security-relevant surface (no auth, input, secrets,
  network, crypto, or data-persistence changes). State the surface you checked and found absent.

## OWASP Classes To Check (diff-scoped)

For each class, check whether the diff introduces or fails to prevent it. Report each distinct
issue with file:line and the vulnerable code. Only flag issues you are confident are real
vulnerabilities in THIS diff, not generic hardening advice.

### A01 — Broken Access Control
- DB queries/lookups using a user-supplied id without an ownership/org/tenant scope check (IDOR).
- Routes without auth middleware; role checks missing or bypassable.
- CORS overly permissive.
- Block on unscoped user-supplied-id lookups; bypassable role checks.

### A02 — Cryptographic Failures / Secrets
- Hardcoded keys/tokens/passwords/API credentials in the diff.
- Secrets or PII written to logs.
- Weak hashing (MD5/SHA1) for credentials; insecure random (`Math.random`) for security tokens.
- Block on hardcoded secrets in committed code; credentials logged.

### A03 — Injection
- SQL/NoSQL/command/LDAP/XPath injection: user input concatenated or interpolated into a query,
  shell command, or expression. Flag string-template SQL (`...${...}...` in a query), `exec`/
  `spawn` with user input, unvalidated parse into a query.
- Block on user input reaching a query/command/eval without parameterization or validation.
  (Note: metareview's deterministic `eval(` gate already covers bare `eval(` injection — do not
  double-report that; flag injection the gate does not catch, e.g. SQL string interpolation.)

### A04 — Insecure Design
- A security control implemented at the wrong layer (client-side authz, trust-the-client checks).
- Missing abuse-case handling for a feature that handles untrusted input.
- Block on trust-the-client authorization; missing rate-limiting on an unauthenticated endpoint.

### A05 — Security Misconfiguration
- Debug mode enabled in prod paths; default credentials; exposed error details/stack traces.
- Missing security headers where a frame-protection or CSP golden exists.
- Block on debug/default-credential exposure in shipped code.

### A07 — Auth/Session Failures
- Session fixation; token generation without sufficient entropy; session timeout removed.
- JWT/cookie handling changes that weaken integrity or expiry.
- Block on weakened token integrity/entropy; removed session expiry.

### A08 — Software/Data Integrity
- Unvalidated deserialization of untrusted input; unsigned updates/code paths.
- Block on unvalidated parse of untrusted bytes into executable/evaluable structures.

### A10 — SSRF
- User-supplied URLs fetched server-side without validation; internal-network/localhost access;
  protocol bypass (`file://`, `gopher://`).
- Block on server-side fetch of unvalidated user URLs reaching internal/localhost targets.

### XSS / Output Encoding (diff-scoped)
- Unescaped user input rendered to HTML/JS; missing output encoding; unescaped header values
  reflected.
- Block on unescaped user input reaching HTML/JS output or reflected headers.

## Evidence Rules

Every blocking finding must cite the vulnerable code (file:line + the verbatim line that makes
the finding true — the "quote-the-line" gate) and state the failure mode (what breaks / what an
attacker gains), not just "this is bad". Do not double-report issues the deterministic gates
already catch (the `eval(` gate covers bare `eval(` injection). If the PR's stated intent is to
change auth/security behavior, judge whether the change preserves or weakens security.
