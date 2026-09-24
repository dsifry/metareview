package findings

import "testing"

func viewedStale(view string) Input {
	in := staleInput()
	in.Fingerprint = "mutation:stale:enforce:stryker:" + view + ":src/a.ts:0123abcd"
	in.View = view
	return in
}

func openFingerprints(res Result) map[string]bool {
	out := map[string]bool{}
	for _, r := range res.OpenFindings {
		out[r.Fingerprint] = true
	}
	return out
}

// Spec §11.3: a run with views counts as open, and supersedes, only rows of its views.
func TestViewedRunCountsOnlyItsViews(t *testing.T) {
	root := t.TempDir()
	core, edge := viewedStale("core"), viewedStale("edge")
	opts := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}
	if _, err := Reconcile(root, run("r1"), []Input{core, edge}, opts); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, edge.Fingerprint); len(got) != 1 || got[0] != "open/" {
		t.Fatalf("the view is stored on the row: %v", got)
	}
	res, err := Reconcile(root, run("r2"), nil, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, core.Fingerprint); got[0] != StatusSuperseded+"/" {
		t.Errorf("core: %v", got)
	}
	if got := statusOf(t, root, edge.Fingerprint); got[0] != "open/" {
		t.Errorf("edge stays open: %v", got)
	}
	if res.OpenBlockingCount != 0 || openFingerprints(res)[edge.Fingerprint] {
		t.Errorf("edge is not this run's: %d open blocking", res.OpenBlockingCount)
	}
	records, _ := All(root)
	for _, r := range records {
		if r.Fingerprint == edge.Fingerprint && r.View != "edge" {
			t.Errorf("row view %q", r.View)
		}
	}
}

// A run without views counts and supersedes every freshness row, viewed or not.
func TestUnviewedRunSupersedesViewedRows(t *testing.T) {
	root := t.TempDir()
	core, edge := viewedStale("core"), viewedStale("edge")
	if _, err := Reconcile(root, run("r1"), []Input{core, edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}); err != nil {
		t.Fatal(err)
	}
	if res, err := Reconcile(root, run("r2"), nil, Options{}); err != nil || res.OpenBlockingCount != 2 {
		t.Fatalf("a run without reports counts every row: %d %v", res.OpenBlockingCount, err)
	}
	if _, err := Reconcile(root, run("r3"), nil, Options{MutationEngines: []string{"stryker"}}); err != nil {
		t.Fatal(err)
	}
	for _, fp := range []string{core.Fingerprint, edge.Fingerprint} {
		if got := statusOf(t, root, fp); got[0] != StatusSuperseded+"/" {
			t.Errorf("%s: %v", fp, got)
		}
	}
}

// Spec §11.3: a view absent from every view map of the run's reports of that engine has its rows
// superseded; runs whose reports carry no map, unviewed rows and overridden rows are untouched.
func TestRenamedViewSupersedesItsRows(t *testing.T) {
	root := t.TempDir()
	edge, plain := viewedStale("edge"), staleInput()
	if _, err := Reconcile(root, run("r1"), []Input{plain}, Options{MutationEngines: []string{"stryker"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(root, run("r2"), []Input{edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"edge"}}); err != nil {
		t.Fatal(err)
	}
	coreOnly := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}}
	if _, err := Reconcile(root, run("r3"), nil, coreOnly); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, edge.Fingerprint); got[0] != "open/" {
		t.Fatalf("no report with a view map: %v", got)
	}
	coreOnly.MutationViewMaps = map[string][]string{"stryker": {"core", "periphery"}}
	if _, err := Reconcile(root, run("r4"), nil, coreOnly); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, edge.Fingerprint); got[0] != StatusSuperseded+"/" {
		t.Errorf("renamed view: %v", got)
	}
	if got := statusOf(t, root, plain.Fingerprint); got[0] != "open/" {
		t.Errorf("an unviewed row is outside the sweep: %v", got)
	}
}

func TestRenameSweepLeavesOverriddenRows(t *testing.T) {
	root := t.TempDir()
	edge := viewedStale("edge")
	res, err := Reconcile(root, run("r1"), []Input{edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"edge"}})
	if err != nil {
		t.Fatal(err)
	}
	id := res.NewFindings[0].ID
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "edge is being retired", Now: "2026-09-24T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted for the rename", Now: "2026-09-24T00:00:01Z"}); err != nil {
		t.Fatal(err)
	}
	sweep := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}, MutationViewMaps: map[string][]string{"stryker": {"core"}}}
	if _, err := Reconcile(root, run("r2"), nil, sweep); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, root, edge.Fingerprint); got[0] != StatusOverridden+"/" {
		t.Errorf("overridden: %v", got)
	}
}

// Per-view fingerprints differ, so an override on one view never covers the same cause in another.
func TestOverrideOnOneViewDoesNotCoverAnother(t *testing.T) {
	root := t.TempDir()
	opts := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}
	res, err := Reconcile(root, run("r1"), []Input{viewedStale("core"), viewedStale("edge")}, opts)
	if err != nil {
		t.Fatal(err)
	}
	id := res.NewFindings[0].ID
	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "core accepted for now", Now: "2026-09-24T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted by the maintainer", Now: "2026-09-24T00:00:01Z"}); err != nil {
		t.Fatal(err)
	}
	res, err = Reconcile(root, run("r2"), []Input{viewedStale("core"), viewedStale("edge")}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenBlockingCount != 1 || !openFingerprints(res)[viewedStale("edge").Fingerprint] {
		t.Errorf("edge still blocks: %d %v", res.OpenBlockingCount, openFingerprints(res))
	}
}
