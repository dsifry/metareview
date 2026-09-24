package reviewers

import (
	"reflect"
	"testing"

	"github.com/dsifry/metareview/internal/mutationfresh"
)

// The rename sweep (spec §11.3) needs, per engine, the view names of the attested reports that
// carry a map; unattested reports and reports without a map contribute nothing.
func TestViewMaps(t *testing.T) {
	if got := (MutationContext{}).ViewMaps(); got != nil {
		t.Errorf("no reports: %v", got)
	}
	ctx := MutationContext{Freshness: []mutationfresh.ReportFreshness{
		{Engine: "stryker", Attested: true, ViewNames: []string{"edge", "core"}},
		{Engine: "stryker", Attested: true, ViewNames: []string{"core", "periphery"}},
		{Engine: "stryker", Attested: true},
		{Engine: "gremlins", Attested: false, ViewNames: []string{"x"}},
	}}
	want := map[string][]string{"stryker": {"core", "edge", "periphery"}}
	if got := ctx.ViewMaps(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
