package findings

import (
	"os"
	"path/filepath"
	"testing"
)

// A granted override is an exception to a specific state of the code, not a permanent hole in the
// gate. Before this, `overridden` was terminal - GrantOverride is the only transition into it and
// nothing led out - and worse, an overridden record counted as "active existing", so a later run
// that rediscovered the same fingerprint filed nothing at all. One grant silenced that check on
// that target forever, which makes unattended operation a ratchet: every exception permanently
// removes a check and nothing can re-arm it.
//
// So a grant binds to the content of the files the finding names, and lapses when they change.

func seedFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func grantedOn(t *testing.T, root, fingerprint string) Record {
	t.Helper()
	r := openBlocker("mrvf-1")
	r.Fingerprint = fingerprint
	seedRecord(t, root, r)
	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
		By: "orchestrator", Reason: "stepped outside the workflow deliberately", Now: "2026-08-29T01:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
		By: "dsifry@warmstart.ai", Reason: "reviewed the evidence and accept it", Now: "2026-08-29T02:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	return loadOne(t, root)
}

func reconcileAgain(t *testing.T, root string, head string) Record {
	t.Helper()
	run := Run{ID: "mrv-run-2", Scope: "pr-ready", Target: map[string]string{"type": "branch", "id": "feature"}, GitHead: head}
	if _, err := Reconcile(root, run, nil, Options{}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return loadOne(t, root)
}

func TestGrantBindsToTheFilesTheFindingNames(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "internal/x.go", "package x\n")
	seedFile(t, root, "unrelated.go", "package u\n")
	rec := grantedOn(t, root, "pr:some-lint:internal/x.go")

	if rec.OverrideBoundKind != BoundPaths {
		t.Fatalf("bound kind = %q, want %q: the finding names a real file", rec.OverrideBoundKind, BoundPaths)
	}
	if len(rec.OverrideBoundPaths) != 1 || rec.OverrideBoundPaths[0] != "internal/x.go" {
		t.Fatalf("bound paths = %v, want [internal/x.go]", rec.OverrideBoundPaths)
	}
	if rec.OverrideBoundHash == "" {
		t.Fatal("a binding with no hash cannot detect anything")
	}

	// An unrelated change must not disturb the exception.
	seedFile(t, root, "unrelated.go", "package u\n\nfunc New() {}\n")
	if got := reconcileAgain(t, root, "head-2"); got.Status != StatusOverridden {
		t.Errorf("status = %q after an unrelated edit; the exception should still hold", got.Status)
	}

	// Editing the file the exception was granted over must re-arm the gate.
	seedFile(t, root, "internal/x.go", "package x\n\nfunc Added() {}\n")
	got := reconcileAgain(t, root, "head-3")
	if got.Status != "open" {
		t.Errorf("status = %q after editing the bound file; the override must lapse and block again", got.Status)
	}
	if got.OverrideLapsedAt == "" {
		t.Error("a lapse must be recorded, or the audit cannot explain why the finding came back")
	}
	if got.OverrideGrantedBy == "" {
		t.Error("lapsing must not erase who granted the exception")
	}
}

// Not every finding names a file - "pr:missing-validation-evidence" names none - and a gate must
// not stay silent because it could not work out what to watch. Those bind to the head, so the
// exception lapses on the next commit: noisier, but it fails toward blocking.
func TestAFindingThatNamesNoFileBindsToTheHead(t *testing.T) {
	root := t.TempDir()
	rec := grantedOn(t, root, "pr:missing-validation-evidence")
	if rec.OverrideBoundKind != BoundHead {
		t.Fatalf("bound kind = %q, want %q", rec.OverrideBoundKind, BoundHead)
	}
	if got := reconcileAgain(t, root, rec.GitHead); got.Status != StatusOverridden {
		t.Errorf("status = %q at the same head; nothing changed yet", got.Status)
	}
	if got := reconcileAgain(t, root, "a-different-head"); got.Status != "open" {
		t.Errorf("status = %q at a new head; a head-bound exception must lapse", got.Status)
	}
}

// A path in a fingerprint that does not exist on disk is not something to watch: binding to it
// would produce a hash that never changes, which is a permanent override wearing a binding.
func TestPathsThatDoNotExistAreNotBindable(t *testing.T) {
	root := t.TempDir()
	rec := grantedOn(t, root, "pr:some-lint:does/not/exist.go")
	if rec.OverrideBoundKind != BoundHead {
		t.Errorf("bound kind = %q, want %q: a path that resolves to nothing cannot be watched", rec.OverrideBoundKind, BoundHead)
	}
}

// The point of the whole mechanism: a lapsed override must actually re-block. Two things had to
// be true and only one was - the record has to return to `open`, AND it has to stop suppressing
// rediscovery. Reconcile builds `activeExisting` from every record that is not `fixed`, so while
// the record sat at `overridden` a current finding with the same fingerprint was skipped entirely
// and never filed. An unattended loop would have seen the check simply disappear.
func TestALapsedOverrideBlocksAgainEndToEnd(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "internal/x.go", "package x\n")
	fingerprint := "pr:some-lint:internal/x.go"
	grantedOn(t, root, fingerprint)

	run := Run{ID: "mrv-run-2", Scope: "pr-ready", Target: map[string]string{"type": "branch", "id": "feature"}, GitHead: "head-2"}
	current := []Input{{
		Reviewer: "architecture-reviewer", Severity: "high", Classification: "blocking",
		Title: "Review context risk", Fingerprint: fingerprint,
	}}

	// While the exception holds, the gate stays quiet even though the finding is rediscovered.
	res, err := Reconcile(root, run, current, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenBlockingCount != 0 {
		t.Fatalf("a live exception must not block, got %d", res.OpenBlockingCount)
	}

	// Edit the file the exception was granted over: the gate must come back on its own.
	seedFile(t, root, "internal/x.go", "package x\n\nfunc Added() {}\n")
	res, err = Reconcile(root, run, current, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenBlockingCount == 0 {
		t.Error("after the bound file changed, the finding must block again without a human touching anything")
	}
	blocked, err := UnresolvedBlocking(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) == 0 {
		t.Error("UnresolvedBlocking must report the re-armed finding, since that is what gates read")
	}
}

// The parts of binding that only fire on awkward input. Each is a decision, so each gets stated.
func TestBindingEdges(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "internal/x.go", "package x\n")

	t.Run("evidence paths are considered, not only the fingerprint", func(t *testing.T) {
		rec := openBlocker("mrvf-e")
		rec.Fingerprint = "pr:names-no-file"
		rec.Evidence = []Evidence{{Type: "diff", Path: "internal/x.go"}}
		if got := bindablePaths(root, rec); len(got) != 1 || got[0] != "internal/x.go" {
			t.Errorf("bindablePaths = %v, want [internal/x.go]", got)
		}
	})

	// A real fingerprint names several files: "pr:unresolved-review-blockers:CHANGELOG.md|docs/x.md".
	// Binding to only the first would leave the exception alive when any of the others changed -
	// found by mutation: FindAllString(raw, -1) -> (raw, 1) survived the whole suite, because every
	// fixture named at most one path.
	t.Run("every file the finding names is watched, not just the first", func(t *testing.T) {
		seedFile(t, root, "docs/second.md", "second\n")
		rec := openBlocker("mrvf-m")
		rec.Fingerprint = "pr:unresolved-review-blockers:internal/x.go|docs/second.md"
		got := bindablePaths(root, rec)
		if len(got) != 2 {
			t.Fatalf("bindablePaths = %v, want both files", got)
		}
		// And the binding must actually change when the SECOND one does.
		_, _, before := bindingFor(root, rec, "head-1")
		seedFile(t, root, "docs/second.md", "second, edited\n")
		if _, _, after := bindingFor(root, rec, "head-1"); after == before {
			t.Error("editing the second named file did not change the binding, so the override would not lapse")
		}
	})

	t.Run("a traversing or absolute path is never watched", func(t *testing.T) {
		rec := openBlocker("mrvf-t")
		rec.Fingerprint = "pr:x:../outside/y.go|/etc/passwd.go"
		if got := bindablePaths(root, rec); len(got) != 0 {
			t.Errorf("bindablePaths = %v, want none: those escape the repository", got)
		}
	})

	t.Run("a file that cannot be read counts as changed", func(t *testing.T) {
		rec := openBlocker("mrvf-u")
		rec.Fingerprint = "pr:x:internal/x.go"
		_, _, before := bindingFor(root, rec, "head-1")
		restore := hashFile
		t.Cleanup(func() { hashFile = restore })
		hashFile = func(string) (string, error) { return "", os.ErrPermission }
		if _, _, after := bindingFor(root, rec, "head-1"); after == before {
			t.Error("an unreadable bound file must not hash the same as a readable one; the exception has to lapse")
		}
	})

	t.Run("hashFile surfaces a read error", func(t *testing.T) {
		if _, err := hashFile(filepath.Join(root, "no-such-file.go")); err == nil {
			t.Error("reading a missing file must be an error")
		}
	})
}
