package setup

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	hookassets "github.com/dsifry/metareview"
	"github.com/dsifry/metareview/internal/gitpolicy"
)

// GitRunner is the git seam so the install logic is testable without a real repo; nil uses the real binary.
type GitRunner func(root string, args ...string) ([]byte, error)

func realGitRunner(root string, args ...string) ([]byte, error) {
	full := append([]string{"-C", root}, args...)
	return exec.Command("git", full...).Output() // #nosec G204 -- args are literals and a resolved path
}

// gitHookScripts maps each materialized hook filename to the embedded source it is written from. Only the
// two GIT hooks belong in core.hooksPath; session-start-check.sh is a Claude SessionStart hook, wired
// separately through the plugin / .claude/settings.json.
var gitHookScripts = map[string]string{
	"pre-push":    "hooks/git/pre-push",
	"post-commit": "hooks/git/post-commit",
}

// hookTargetDir is where the gate's hook scripts are MATERIALIZED: a metareview-owned dir inside .metareview
// (already git-ignored, so the per-clone install artifacts are never committed). Absolute, because a relative
// core.hooksPath is resolved inconsistently by git.
func hookTargetDir(root string) (string, error) {
	return filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))
}

// legacyHookTargetDir is the pre-0.11 install target: the committed hooks/git of metareview's OWN checkout.
// It is recognised as "ours" so an existing metareview clone upgrades cleanly instead of seeing a conflict.
func legacyHookTargetDir(root string) (string, error) {
	return filepath.Abs(filepath.Join(root, "hooks", "git"))
}

// materializeHooks writes the embedded hook scripts into dir, each executable. This is what lets the gate
// reach a CONSUMER repo: the scripts are compiled into the binary, not assumed to already exist on disk.
func materializeHooks(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, src := range gitHookScripts {
		body, err := hookassets.GitHookAssets.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading embedded hook %s: %w", src, err)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o755); err != nil { //nolint:gosec // a git hook must be executable
			return err
		}
		if err := os.Chmod(p, 0o755); err != nil { // WriteFile perms are umask-masked; force the exec bit
			return err
		}
	}
	return nil
}

// (The .gitignore block that keeps metareview's ephemeral state out of a consumer's commits lives in
// internal/gitpolicy — one source shared with the learning post-merge writer. ApplyHookInstall calls
// gitpolicy.Ensure directly.)

// hooksMaterialized reports whether dir holds an executable copy of EVERY gate hook — the check the CLI runs
// to confirm the gate is genuinely active before it says so (git silently ignores a missing/empty hooksPath).
func hooksMaterialized(dir string) bool {
	for name := range gitHookScripts {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode()&0o100 == 0 {
			return false
		}
	}
	return true
}

// hooksCurrent is stricter than hooksMaterialized: every gate hook must be present, executable, AND
// byte-identical to the CURRENT embed. The materialized scripts live in git-ignored .metareview/git-hooks, so
// an UPGRADED binary carries a newer hook body while the on-disk script predates the upgrade — present-but-
// stale. Treating that as "installed" (hooksMaterialized alone) leaves the consumer running the old hook
// forever, so a hook fix (e.g. the #82 per-ref gate) never reaches an already-installed repo. Content drift ⇒
// not current ⇒ PlanHookInstall does NOT report AlreadyDone ⇒ re-install rematerializes.
func hooksCurrent(dir string) bool {
	for name, src := range gitHookScripts {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil || info.Mode()&0o100 == 0 {
			return false
		}
		want, err := hookassets.GitHookAssets.ReadFile(src)
		if err != nil {
			return false
		}
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
	}
	return true
}

// HookInstallPlan is what installing the git-native review gate WOULD do, computed read-only. A non-empty
// Conflicts blocks the install unless the caller forces it — metareview never silently overrides a user's
// git config or disables hooks they already have.
type HookInstallPlan struct {
	// Target is the absolute path core.hooksPath should point at: this clone's committed hooks/git. Absolute,
	// not relative — a relative core.hooksPath is resolved inconsistently by git (a known footgun), and
	// core.hooksPath is per-clone local config anyway, so an absolute per-clone value is correct.
	Target string
	// Current is this repo's LOCAL core.hooksPath, empty when unset (git then uses the default .git/hooks).
	Current string
	// AlreadyDone is true when Current already points at Target — install is a no-op.
	AlreadyDone bool
	// Conflicts are human-readable reasons an install would disturb the user's setup. Non-empty ⇒ refuse
	// without force.
	Conflicts []string
}

// PlanHookInstall inspects the repo READ-ONLY and returns what installing the gate would do. Two conflicts
// are detected before anything is touched: (1) core.hooksPath is already set to a DIFFERENT path — we will
// not override a user's choice; (2) core.hooksPath is unset but there are active (non-sample) hooks in
// .git/hooks — setting core.hooksPath makes git IGNORE .git/hooks, silently disabling those hooks.
func PlanHookInstall(root string, git GitRunner) (HookInstallPlan, error) {
	if git == nil {
		git = realGitRunner
	}
	target, err := hookTargetDir(root)
	if err != nil {
		return HookInstallPlan{}, err
	}
	// Confirm this is a usable git repo BEFORE reading config. `git config --get` exits 1 (empty) when a key
	// is merely unset, which is not an error — but it also fails on a broken/absent repo, and treating THAT as
	// "unset" would produce a conflict-free plan and let an install shadow hooks it never inspected. Gating on
	// rev-parse first means a later config-read error can be read as "key unset", not masked repo breakage.
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return HookInstallPlan{}, fmt.Errorf("cannot inspect hooks: %s is not a usable git repository: %w", root, err)
	}
	plan := HookInstallPlan{Target: target}
	// Two reads: the LOCAL value (what we would own) and the EFFECTIVE value (local > global > system, what
	// git actually uses). A foreign value set GLOBALLY is in effect for this repo and must be treated as a
	// conflict too — reading only --local silently overrode a user's global hooks. git config exits 1 (empty)
	// when unset; in a repo we have confirmed usable, that is the only benign failure, so it is treated as unset.
	localOut, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	local := strings.TrimSpace(string(localOut))
	effOut, _ := git(root, "config", "--get", "core.hooksPath")
	plan.Current = strings.TrimSpace(string(effOut))

	// Ours, set locally → already installed ONLY if the scripts are present AND byte-current with the embed AND
	// the ephemeral-state .gitignore block is in place. .metareview/git-hooks is git-ignored, so the scripts can
	// be deleted (missing) or predate a binary upgrade (present-but-stale) while core.hooksPath still points at
	// them; treating either as "done" leaves the gate inert or running an old hook. And an EARLIER install (before
	// the gitignore block existed) has current hooks but no block — short-circuiting there would never write it,
	// so a reinstall could not restore Gap B for an already-installed repo. All three must hold, or we fall
	// through and ApplyHookInstall re-materializes the scripts and (re)writes the gitignore block.
	if local != "" && sameHookPath(root, local, target) && hooksCurrent(target) && gitpolicy.Present(root) {
		plan.AlreadyDone = true
		return plan, nil
	}
	// A foreign value in effect — local, global, or system — must not be overridden. The legacy metareview
	// target (committed hooks/git of an older clone) is NOT foreign: it is ours, so an upgrade replaces it
	// rather than refusing (AlreadyDone stayed false above, so install proceeds to the new target).
	if plan.Current != "" && !isOurHookPath(root, plan.Current, target) {
		scope := "core.hooksPath"
		if local == "" {
			scope = "a global or system core.hooksPath"
		}
		plan.Conflicts = append(plan.Conflicts,
			scope+" is already set to "+plan.Current+" — metareview will not override it")
		return plan, nil
	}
	// Effective value is unset → git uses the default .git/hooks. Redirecting away from it would bypass
	// anything active there. (When it already resolves to our target via a global value, there is nothing to
	// bypass, so this check is skipped.)
	if plan.Current == "" {
		active, err := activeGitHooks(root, git)
		if err != nil {
			// Fail closed: an unreadable .git/hooks might hold active hooks a core.hooksPath redirect would
			// silently disable. Not being able to tell is not permission to install over them.
			return HookInstallPlan{}, fmt.Errorf("inspecting .git/hooks for active hooks: %w", err)
		}
		if len(active) > 0 {
			plan.Conflicts = append(plan.Conflicts,
				"active hooks in .git/hooks would be bypassed by core.hooksPath: "+strings.Join(active, ", "))
		}
	}
	return plan, nil
}

// ApplyHookInstall sets core.hooksPath to the plan's Target. It is a no-op when already installed, and
// REFUSES (returns an error, changes nothing) when the plan has conflicts unless force is true.
func ApplyHookInstall(root string, plan HookInstallPlan, force bool, git GitRunner) error {
	if git == nil {
		git = realGitRunner
	}
	// --force ALWAYS re-materializes: it is the explicit "rewrite the scripts now" override, for a
	// hand-tampered or partially-updated hook that a read-only plan happened to call AlreadyDone.
	if plan.AlreadyDone && !force {
		return nil
	}
	if len(plan.Conflicts) > 0 && !force {
		return fmt.Errorf("refusing to install over a conflict (use --force to override): %s",
			strings.Join(plan.Conflicts, "; "))
	}
	// The plan may be stale: core.hooksPath can change between PlanHookInstall and here — e.g. across an
	// interactive [y/N] prompt. Re-inspect immediately before the write and refuse a state that has become a
	// conflict (or is now AlreadyDone), so an unforced install never silently shadows a value set meanwhile.
	if !force {
		fresh, err := PlanHookInstall(root, git)
		if err != nil {
			return fmt.Errorf("revalidating hook state before install: %w", err)
		}
		if fresh.AlreadyDone {
			return nil
		}
		if len(fresh.Conflicts) > 0 {
			return fmt.Errorf("core.hooksPath changed since planning; refusing to install over a conflict (use --force to override): %s",
				strings.Join(fresh.Conflicts, "; "))
		}
	}
	// Materialize the embedded hook scripts into the target dir BEFORE pointing git at it, so core.hooksPath
	// never references an empty/absent directory — git silently ignores that, which is exactly how the gate
	// was inert in a consumer repo while the CLI still reported it "active".
	if err := materializeHooks(plan.Target); err != nil {
		return fmt.Errorf("writing hook scripts to %s: %w", plan.Target, err)
	}
	// Best-effort: keep metareview's ephemeral per-clone state out of the consumer's commits (the shared
	// gitpolicy block — one source with the learning post-merge writer). Never fatal.
	_ = gitpolicy.Ensure(root)
	if _, err := git(root, "config", "--local", "core.hooksPath", plan.Target); err != nil {
		return fmt.Errorf("setting core.hooksPath: %w", err)
	}
	// Verify the gate is genuinely in place before the caller says so.
	if !hooksMaterialized(plan.Target) {
		return fmt.Errorf("hook scripts were not written to %s; the gate is NOT active", plan.Target)
	}
	return nil
}

// isOurHookPath reports whether a core.hooksPath value is one metareview manages: the current materialized
// target, or the legacy committed hooks/git of an older metareview clone. The legacy path is claimed ONLY
// when its pre-push is genuinely metareview's (content marker) — a consumer repo may legitimately keep its
// OWN hooks in <root>/hooks/git, and install must never silently replace, nor uninstall unset, those.
func isOurHookPath(root, current, target string) bool {
	if sameHookPath(root, current, target) {
		return true
	}
	if legacy, err := legacyHookTargetDir(root); err == nil && sameHookPath(root, current, legacy) {
		return legacyHooksAreOurs(legacy)
	}
	return false
}

// legacyHooksAreOurs reports whether dir/pre-push is metareview's own gate script, by a distinctive marker
// its body always contains. This is the ownership check that keeps the legacy-upgrade path from claiming a
// consumer's unrelated hooks/git.
func legacyHooksAreOurs(dir string) bool {
	// The ENTIRE legacy dir is absent → the original-bug case: core.hooksPath pointed at a hooks/git that never
	// existed. Nothing of the consumer's is there, so reclaim it (install rematerializes and recovers the
	// broken clone without --force).
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return true
	}
	// The dir EXISTS. It may hold the consumer's own hooks (commit-msg, etc.) even without a pre-push, so claim
	// it ONLY when its pre-push carries metareview's marker — never on a missing pre-push alone.
	body, err := os.ReadFile(filepath.Join(dir, "pre-push"))
	if err != nil {
		return false // no readable metareview pre-push in an existing dir → not ours; leave it alone
	}
	return strings.Contains(string(body), "metareview") && strings.Contains(string(body), "review gate --push")
}

// UninstallStatus is what uninstalling WOULD do, computed read-only, so the CLI can honour --dry-run and the
// same confirmation guard install uses before touching anything.
type UninstallStatus struct {
	// Current is this repo's LOCAL core.hooksPath, empty when unset.
	Current string
	// WouldChange is true iff Current points at metareview's hooks/git — the only case uninstall unsets it.
	WouldChange bool
}

// UninstallPreview reports what UninstallHookInstall would do without changing anything.
func UninstallPreview(root string, git GitRunner) (UninstallStatus, error) {
	if git == nil {
		git = realGitRunner
	}
	target, err := hookTargetDir(root)
	if err != nil {
		return UninstallStatus{}, err
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return UninstallStatus{}, fmt.Errorf("cannot inspect hooks: %s is not a usable git repository: %w", root, err)
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	return UninstallStatus{Current: current, WouldChange: current != "" && isOurHookPath(root, current, target)}, nil
}

// UninstallHookInstall unsets core.hooksPath, but ONLY when it currently points at metareview's hooks/git —
// it never touches a value it did not set. Returns whether it changed anything.
func UninstallHookInstall(root string, git GitRunner) (bool, error) {
	if git == nil {
		git = realGitRunner
	}
	target, err := hookTargetDir(root)
	if err != nil {
		return false, err
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	if current == "" {
		return false, nil
	}
	if !isOurHookPath(root, current, target) {
		return false, fmt.Errorf("core.hooksPath is %s, not metareview's — leaving it unchanged", current)
	}
	if _, err := git(root, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		return false, err
	}
	// Remove the materialized hook dir we own, so uninstall leaves no dangling scripts. A non-existent or
	// legacy (committed) dir is left alone: RemoveAll on the materialized target only.
	if mine, e := hookTargetDir(root); e == nil {
		_ = os.RemoveAll(mine)
	}
	return true, nil
}

// activeGitHooks lists the non-sample hook files in the repo's COMMON .git/hooks — the hooks a core.hooksPath
// redirect would silently stop running. It asks git for the hooks path (`rev-parse --git-path hooks`) rather
// than joining "hooks" onto --absolute-git-dir: in a linked worktree the latter is the per-worktree gitdir
// (whose hooks/ does not exist), so active hooks in the shared common dir were missed. --git-path returns the
// common .git/hooks; when core.hooksPath is unset (the only case this runs) it is the default location.
func activeGitHooks(root string, git GitRunner) ([]string, error) {
	out, err := git(root, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return nil, err
	}
	dir := strings.TrimSpace(string(out))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // no hooks directory → nothing active to bypass
	}
	if err != nil {
		return nil, err
	}
	var active []string
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".sample") {
			continue
		}
		active = append(active, e.Name())
	}
	sort.Strings(active)
	return active, nil
}

// sameHookPath reports whether two core.hooksPath values name the same directory. A relative value is
// resolved against the repo root (how git treats it from the worktree top), so "hooks/git" and the absolute
// form compare equal.
func sameHookPath(root, a, b string) bool {
	return resolveHookPath(root, a) == resolveHookPath(root, b)
}

func resolveHookPath(root, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return filepath.Clean(p)
}
