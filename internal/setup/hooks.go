package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// GitRunner is the git seam so the install logic is testable without a real repo; nil uses the real binary.
type GitRunner func(root string, args ...string) ([]byte, error)

func realGitRunner(root string, args ...string) ([]byte, error) {
	full := append([]string{"-C", root}, args...)
	return exec.Command("git", full...).Output() // #nosec G204 -- args are literals and a resolved path
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
	target, err := filepath.Abs(filepath.Join(root, "hooks", "git"))
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

	// Ours, set locally → already installed.
	if local != "" && sameHookPath(root, local, target) {
		plan.AlreadyDone = true
		return plan, nil
	}
	// A foreign value in effect — local, global, or system — must not be overridden.
	if plan.Current != "" && !sameHookPath(root, plan.Current, target) {
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
	if plan.AlreadyDone {
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
	if _, err := git(root, "config", "--local", "core.hooksPath", plan.Target); err != nil {
		return fmt.Errorf("setting core.hooksPath: %w", err)
	}
	return nil
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
	target, err := filepath.Abs(filepath.Join(root, "hooks", "git"))
	if err != nil {
		return UninstallStatus{}, err
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return UninstallStatus{}, fmt.Errorf("cannot inspect hooks: %s is not a usable git repository: %w", root, err)
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	return UninstallStatus{Current: current, WouldChange: current != "" && sameHookPath(root, current, target)}, nil
}

// UninstallHookInstall unsets core.hooksPath, but ONLY when it currently points at metareview's hooks/git —
// it never touches a value it did not set. Returns whether it changed anything.
func UninstallHookInstall(root string, git GitRunner) (bool, error) {
	if git == nil {
		git = realGitRunner
	}
	target, err := filepath.Abs(filepath.Join(root, "hooks", "git"))
	if err != nil {
		return false, err
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	if current == "" {
		return false, nil
	}
	if !sameHookPath(root, current, target) {
		return false, fmt.Errorf("core.hooksPath is %s, not metareview's hooks/git — leaving it unchanged", current)
	}
	if _, err := git(root, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		return false, err
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
