package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// Seams over the stdlib/embed calls whose error branches below are otherwise unreachable: the
// embedded hook assets always read, filepath.Abs only fails when os.Getwd fails, os.Chmod cannot
// fail on a file just written, and materializeHooks does not "succeed" while leaving the target
// unmaterialized. Each defaults to the real function and is overridden — then restored via
// t.Cleanup — in tests, so production behavior is identical to calling the wrapped function.
var (
	readHookAsset    = hookassets.GitHookAssets.ReadFile
	filepathAbs      = filepath.Abs
	osChmod          = os.Chmod
	applyMaterialize = materializeHooks
	userHomeDir      = os.UserHomeDir
)

// gitHookScripts maps each materialized hook filename to the embedded source it is written from. Only the
// two GIT hooks belong in core.hooksPath; session-start-check.sh is a Claude SessionStart hook, wired
// separately through the plugin / .claude/settings.json.
var gitHookScripts = map[string]string{
	"pre-push":    "hooks/git/pre-push",
	"post-commit": "hooks/git/post-commit",
}

// hooksHome is the user-level directory the gate's hook scripts are materialized under (#173):
// ${XDG_DATA_HOME:-~/.local/share}/metareview/git-hooks. It lives outside every repository on purpose. An absolute
// core.hooksPath into a checkout (or into its .git) goes stale the moment that checkout is moved, and git then
// runs NO hook, silently; a relative one is resolved against each worktree's own root, so it only works in the
// main checkout. A path under the user's data home survives moving or deleting any checkout, in every layout.
func hooksHome() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(base) { // unset, or relative (which the XDG spec says to ignore)
		home, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating the user's data directory for the hook scripts: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "metareview", "git-hooks"), nil
}

// HooksIDKey is the repository-local git config key naming this repository's hook dir under hooksHome (#173). It is
// in the repository's own config, so it moves with the repository; a path never could.
const HooksIDKey = "metareview.hooksId"

var hookIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// hookOwnerFile, inside a hook dir, names the repository (git's common directory) that installed into it. A copied
// checkout (cp -r, rsync, a restored backup) carries .git/config — metareview.hooksId and core.hooksPath included — so
// the id alone cannot tell two repositories apart; the owner can.
const hookOwnerFile = ".metareview-owner"

// commonDir is git's common directory for root: the repository's identity, shared by every worktree.
func commonDir(root string, git GitRunner) (string, error) {
	out, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("locating git's common directory: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

// ownedBy reports whether this repository (common) may treat dir as its own: its owner file names common, or names a
// repository that is no longer there (this one, moved), or is absent. A dir whose owner still exists elsewhere
// belongs to that repository — this checkout is a copy of it — and must be neither reused nor emptied from here.
func ownedBy(dir, common string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, hookOwnerFile)) // #nosec G304 -- a metareview hook dir
	if err != nil {
		return true
	}
	owner := filepath.Clean(strings.TrimSpace(string(raw)))
	if owner == common {
		return true
	}
	_, err = os.Stat(owner)
	return errors.Is(err, fs.ErrNotExist)
}

// maxDerivedIDs bounds the search for a free derived id; each step is a few os.Lstat calls. A repository deleted and
// re-cloned in place without uninstalling leaves its old dir (see released), so the bound is far above any realistic
// churn at one path. A var so a test can exhaust it.
var maxDerivedIDs = 4096

// repoHooksID is this repository's hook-dir id: the recorded metareview.hooksId when its dir is this repository's
// (ownedBy), otherwise a derived one. A derived id is the first of sha256(common dir, n), n = 0, 1, …, whose dir is
// free: absent, or released (see released) — so uninstall + reinstall reuses the dir and a hook the user kept there
// resumes, while a dir some repository may still run from is never adopted. Deterministic for a given filesystem, so
// a read-only plan is stable.
func repoHooksID(root string, git GitRunner, home string) (string, error) {
	common, err := commonDir(root, git)
	if err != nil {
		return "", err
	}
	if out, err := git(root, "config", "--local", "--get", HooksIDKey); err == nil {
		if id := strings.TrimSpace(string(out)); hookIDPattern.MatchString(id) && ownedBy(filepath.Join(home, id), common) {
			return id, nil
		}
	}
	for n := 0; n < maxDerivedIDs; n++ {
		seed := common
		if n > 0 {
			seed = fmt.Sprintf("%s\x00%d", common, n)
		}
		sum := sha256.Sum256([]byte(seed))
		id := hex.EncodeToString(sum[:])[:16]
		if dir := filepath.Join(home, id); !exists(dir) || released(dir) {
			return id, nil
		}
	}
	return "", fmt.Errorf("no free hook dir id for %s under %s after %d tries", common, home, maxDerivedIDs)
}

// hookTargetDir is where this repository's hook scripts are MATERIALIZED: <hooksHome>/<metareview.hooksId>. One dir
// per repository, so a hook a user or another tool adds to git's hooks dir (which core.hooksPath now names) stays in
// that repository. Absolute, because a relative core.hooksPath is resolved against each worktree's root.
func hookTargetDir(root string, git GitRunner) (string, error) {
	home, err := hooksHome()
	if err != nil {
		return "", err
	}
	id, err := repoHooksID(root, git, home)
	if err != nil {
		return "", err
	}
	return filepathAbs(filepath.Join(home, id))
}

// isPreviousHookDir reports whether p is a hook dir metareview materialized, so install re-points it rather than
// refusing it as foreign: any <data home>/metareview/git-hooks/<id> (matched by shape, not by the CURRENT data home:
// XDG_DATA_HOME can differ between the shell that installed and the one running now), or the pre-#173 per-checkout
// <checkout>/.metareview/git-hooks. Either is claimed only when it is gone or its pre-push is metareview's gate.
func isPreviousHookDir(p string) bool {
	p = filepath.Clean(p)
	userLevel := hookIDPattern.MatchString(filepath.Base(p)) && filepath.Base(filepath.Dir(p)) == "git-hooks" &&
		filepath.Base(filepath.Dir(filepath.Dir(p))) == "metareview"
	perCheckout := filepath.Base(p) == "git-hooks" && filepath.Base(filepath.Dir(p)) == ".metareview"
	return (userLevel || perCheckout) && legacyHooksAreOurs(p)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// released reports whether dir no longer holds any of metareview's scripts — an uninstall took them out — so it can be
// reused. A dir that still holds the gate is never adopted by a repository with no recorded id, whatever its owner
// file says: from inside a fresh clone, `mv repo repo.bak && git clone … repo` (repo.bak still runs from that dir) and
// `rm -rf repo && git clone … repo` (nothing does) look identical, and adopting a live dir can silently ungate a
// repository, while leaving an orphan behind costs a few KB.
func released(dir string) bool {
	for name := range gitHookScripts {
		if exists(filepath.Join(dir, name)) {
			return false
		}
	}
	return true
}

// ownerIs reports whether dir's owner file names exactly this repository.
func ownerIs(dir, common string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, hookOwnerFile)) // #nosec G304 -- a metareview hook dir
	return err == nil && filepath.Clean(strings.TrimSpace(string(raw))) == common
}

// releaseHookDir takes metareview's scripts out of a pre-#173 per-checkout hook dir (<checkout>/.metareview/git-hooks)
// this repository no longer uses, and the dir once nothing else is left — only when that checkout is one of THIS
// repository's (same git common dir). Before #173 core.hooksPath was an absolute path into the installing checkout,
// so a copy of such an install (cp -r, a restored backup) still points at its ORIGINAL's dir, which must not be
// touched. A user-level dir is never emptied from here: two live repositories can carry the same metareview.hooksId
// (a copy whose original then moved, a backup restored over a moved one's old path) and nothing inside either tells
// them apart. An unused dir is left behind instead (a few KB).
func releaseHookDir(dir, common string, git GitRunner) {
	if filepath.Base(filepath.Dir(dir)) != ".metareview" || !isPreviousHookDir(dir) {
		return
	}
	if owner, err := commonDir(filepath.Dir(filepath.Dir(dir)), git); err != nil || owner != common {
		return
	}
	for name := range gitHookScripts {
		_ = os.Remove(filepath.Join(dir, name))
	}
	_ = os.Remove(dir) // only succeeds when empty
}

// foreignHooksIn lists the files in a metareview hook dir that are not its own scripts: hooks a user or another tool
// put there. Re-pointing core.hooksPath away from that dir would silently stop them running.
func foreignHooksIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var extra []string
	for _, e := range entries {
		if _, ours := gitHookScripts[e.Name()]; !ours && e.Name() != hookOwnerFile && !strings.Contains(e.Name(), ".tmp-") {
			extra = append(extra, e.Name())
		}
	}
	return extra
}

// legacyHookTargetDir is the pre-0.11 install target: the committed hooks/git of metareview's OWN checkout.
// It is recognised as "ours" so an existing metareview clone upgrades cleanly instead of seeing a conflict.
func legacyHookTargetDir(root string) (string, error) {
	return filepath.Abs(filepath.Join(root, "hooks", "git"))
}

// materializeHooks writes the embedded hook scripts into dir, each executable. This is what lets the gate
// reach a CONSUMER repo: the scripts are compiled into the binary, not assumed to already exist on disk. An upgrade
// rewrites them in place, so each script is written to a temporary name and renamed into place: a hook git starts
// while an install is writing sees the old script or the new one, never half of one. Other files are left alone.
func materializeHooks(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, src := range gitHookScripts {
		body, err := readHookAsset(src)
		if err != nil {
			return fmt.Errorf("reading embedded hook %s: %w", src, err)
		}
		p := filepath.Join(dir, name)
		tmp := fmt.Sprintf("%s.tmp-%d", p, os.Getpid())
		if err := os.WriteFile(tmp, body, 0o755); err != nil { //nolint:gosec // a git hook must be executable
			return err
		}
		if err := osChmod(tmp, 0o755); err != nil { // WriteFile perms are umask-masked; force the exec bit
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, p); err != nil {
			_ = os.Remove(tmp)
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
// byte-identical to the CURRENT embed. The materialized scripts live outside the repository, so
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
		want, err := readHookAsset(src)
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
	// Target is the absolute path core.hooksPath should point at: this repository's user-level hook dir (#173).
	// Absolute, not relative — a relative core.hooksPath is resolved against each worktree's own root.
	Target string
	// Current is this repo's LOCAL core.hooksPath, empty when unset (git then uses the default .git/hooks).
	Current string
	// AlreadyDone is true when Current already points at Target — install is a no-op.
	AlreadyDone bool
	// HooksCurrent is true when core.hooksPath is OURS and the materialized scripts are byte-current with the
	// embed — i.e. the pre-push gate is ACTIVE — regardless of the .gitignore block. AlreadyDone is stricter
	// (it also requires the gitpolicy block, so a reinstall restores it); a report of whether `git push` is
	// gated must not be dragged to false by a missing ignore line, so it reads HooksCurrent, not AlreadyDone.
	HooksCurrent bool
	// Conflicts are human-readable reasons an install would disturb the user's setup. Non-empty ⇒ refuse
	// without force.
	Conflicts []string
}

// StopGateKey is the repository-local git config key that opts a repository into the Stop-hook gate (#194). The
// plugin registers hooks/pre-finish.sh in every session on the machine; the hook gates only where this is "true",
// and `setup --install-hooks` is what sets it.
const StopGateKey = "metareview.stopGate"

// EnableStopGate records the Stop-gate opt-in and nothing else, so a repository whose own hook manager owns
// core.hooksPath (husky, lefthook, beads) — where `setup --install-hooks` refuses rather than override it — can
// still opt in.
func EnableStopGate(root string, git GitRunner) error {
	if git == nil {
		git = realGitRunner
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("cannot opt in: %s is not a usable git repository: %w", root, err)
	}
	if _, err := git(root, "config", "--local", StopGateKey, "true"); err != nil {
		return fmt.Errorf("recording the Stop-gate opt-in: %w", err)
	}
	return nil
}

// DisableStopGate removes the Stop-gate opt-in, reporting whether one was recorded.
func DisableStopGate(root string, git GitRunner) (bool, error) {
	if git == nil {
		git = realGitRunner
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return false, fmt.Errorf("cannot opt out: %s is not a usable git repository: %w", root, err)
	}
	if !stopGateOptedIn(root, git) {
		return false, nil
	}
	_, err := git(root, "config", "--local", "--unset-all", StopGateKey)
	return err == nil, err
}

// PlanHookInstall inspects the repo READ-ONLY and returns what installing the gate would do. Two conflicts
// are detected before anything is touched: (1) core.hooksPath is already set to a DIFFERENT path — we will
// not override a user's choice; (2) core.hooksPath is unset but there are active (non-sample) hooks in
// .git/hooks — setting core.hooksPath makes git IGNORE .git/hooks, silently disabling those hooks.
func PlanHookInstall(root string, git GitRunner) (HookInstallPlan, error) {
	if git == nil {
		git = realGitRunner
	}
	// Confirm this is a usable git repo BEFORE reading config. `git config --get` exits 1 (empty) when a key
	// is merely unset, which is not an error — but it also fails on a broken/absent repo, and treating THAT as
	// "unset" would produce a conflict-free plan and let an install shadow hooks it never inspected. Gating on
	// rev-parse first means a later config-read error can be read as "key unset", not masked repo breakage.
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return HookInstallPlan{}, fmt.Errorf("cannot inspect hooks: %s is not a usable git repository: %w", root, err)
	}
	target, err := hookTargetDir(root, git)
	if err != nil {
		return HookInstallPlan{}, err
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
	// the ephemeral-state .gitignore block is in place. The scripts live outside the repository, so they can
	// be deleted (missing) or predate a binary upgrade (present-but-stale) while core.hooksPath still points at
	// them; treating either as "done" leaves the gate inert or running an old hook. And an EARLIER install (before
	// the gitignore block existed) has current hooks but no block — short-circuiting there would never write it,
	// so a reinstall could not restore Gap B for an already-installed repo. All three must hold, or we fall
	// through and ApplyHookInstall re-materializes the scripts and (re)writes the gitignore block.
	plan.HooksCurrent = local != "" && sameHookPath(root, local, target) && hooksCurrent(target)
	// An install from before the Stop-gate opt-in (#194) has current hooks but no opt-in: not done, so a
	// re-install records it and the Stop gate keeps working after the upgrade.
	optIn, _ := git(root, "config", "--local", "--get", StopGateKey)
	// ...and one whose hook dir still names another location as its owner (this repository was moved) is not done
	// either: a re-install records the new owner, so a later repository at the old path cannot claim the dir.
	common, _ := commonDir(root, git)
	if plan.HooksCurrent && gitpolicy.Present(root) && strings.TrimSpace(string(optIn)) == "true" && ownerIs(target, common) {
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
	// Moving off an earlier metareview location stops every hook in it running, and only metareview's own are
	// rewritten at the target: anything else there (a hand-added commit-msg) would be dropped silently.
	if prev := resolveHookPath(root, plan.Current); plan.Current != "" && !sameHookPath(root, prev, target) {
		if extra := foreignHooksIn(prev); len(extra) > 0 {
			plan.Conflicts = append(plan.Conflicts,
				"hooks in "+prev+" that are not metareview's would stop running when core.hooksPath moves to "+target+": "+strings.Join(extra, ", ")+" — move them to "+target+" first")
			return plan, nil
		}
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
	if err := applyMaterialize(plan.Target); err != nil {
		return fmt.Errorf("writing hook scripts to %s: %w", plan.Target, err)
	}
	common, err := commonDir(root, git)
	if err != nil {
		return err
	}
	// Best-effort: keep metareview's ephemeral per-clone state out of the consumer's commits (the shared
	// gitpolicy block — one source with the learning post-merge writer). Never fatal.
	_ = gitpolicy.Ensure(root)
	// Record the id first: once core.hooksPath names <hooksHome>/<id>, the id must no longer be derived from where
	// the repository lives.
	if _, err := git(root, "config", "--local", HooksIDKey, filepath.Base(plan.Target)); err != nil {
		return fmt.Errorf("recording the hook dir id (%s): %w", HooksIDKey, err)
	}
	if _, err := git(root, "config", "--local", "core.hooksPath", plan.Target); err != nil {
		return fmt.Errorf("setting core.hooksPath: %w", err)
	}
	// Migrated from an earlier location: its scripts are no longer used. Remove metareview's own (the plan refused
	// if anything else was there), and the dir once it is empty.
	if prev := resolveHookPath(root, plan.Current); plan.Current != "" && !sameHookPath(root, prev, plan.Target) {
		releaseHookDir(prev, common, git)
	}
	if _, err := git(root, "config", "--local", StopGateKey, "true"); err != nil {
		return fmt.Errorf("recording the Stop-gate opt-in: %w", err)
	}
	// Verify the gate is genuinely in place before the caller says so.
	if !hooksMaterialized(plan.Target) {
		return fmt.Errorf("hook scripts were not written to %s; the gate is NOT active", plan.Target)
	}
	// Record whose dir this is, once the gate is in place: a copied checkout carrying this config must get its own.
	if err := os.WriteFile(filepath.Join(plan.Target, hookOwnerFile), []byte(common+"\n"), 0o644); err != nil { //nolint:gosec // not secret
		return fmt.Errorf("recording the hook dir's owner: %w", err)
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
	return isPreviousHookDir(resolveHookPath(root, current))
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
	// WouldChange is true iff Current points at a metareview hook location — the only case uninstall unsets it.
	WouldChange bool
}

// UninstallPreview reports what UninstallHookInstall would do without changing anything.
func UninstallPreview(root string, git GitRunner) (UninstallStatus, error) {
	if git == nil {
		git = realGitRunner
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return UninstallStatus{}, fmt.Errorf("cannot inspect hooks: %s is not a usable git repository: %w", root, err)
	}
	target, err := hookTargetDir(root, git)
	if err != nil {
		return UninstallStatus{}, err
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	return UninstallStatus{Current: current, WouldChange: current != "" && isOurHookPath(root, current, target)}, nil
}

// UninstallHookInstall unsets core.hooksPath, but ONLY when it currently points at a metareview hook location —
// it never touches a value it did not set. Returns whether it changed anything.
func UninstallHookInstall(root string, git GitRunner) (bool, error) {
	if git == nil {
		git = realGitRunner
	}
	out, _ := git(root, "config", "--local", "--get", "core.hooksPath")
	current := strings.TrimSpace(string(out))
	if current == "" {
		return false, nil
	}
	target, err := hookTargetDir(root, git)
	if err != nil {
		return false, err
	}
	if !isOurHookPath(root, current, target) {
		return false, fmt.Errorf("core.hooksPath is %s, not metareview's — leaving it unchanged", current)
	}
	// The opt-in goes FIRST, with the gate. Unset exits 5 when the key is already absent (an install from before
	// #194), which is not a failure; anything else stops here with nothing taken apart, so the same command can
	// finish the job once the failure clears. (Removing core.hooksPath first left a retry that found nothing to
	// uninstall while metareview.stopGate kept pre-finish.sh gating the repository.)
	if _, err := git(root, "config", "--local", "--unset-all", StopGateKey); err != nil && !isExitCode(err, 5) {
		return false, fmt.Errorf("removing the Stop-gate opt-in (%s): %w", StopGateKey, err)
	}
	if _, err := git(root, "config", "--local", "--unset", "core.hooksPath"); err != nil {
		return false, err
	}
	// A pre-#173 per-checkout dir goes. A user-level dir stays, and so does metareview.hooksId: another live repository
	// may run from the same dir (see releaseHookDir), and a reinstall here reuses it — a hook the user keeps there
	// resumes.
	if common, err := commonDir(root, git); err == nil {
		releaseHookDir(resolveHookPath(root, current), common, git)
	}
	return true, nil
}

// isExitCode reports whether err is a git process that exited with code.
func isExitCode(err error, code int) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == code
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
