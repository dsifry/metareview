package mutationfresh

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dsifry/metareview/internal/mutation"
)

var viewName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// CheckViews validates --mutation-view names against the supplied reports (spec §11.3): every name
// matches [A-Za-z0-9._-]+, and appears in the view map of every attested report that has one. An
// unattested report, or one attested before views, is not scoped and accepts any name. An error is
// a usage error (exit 2).
func CheckViews(reports []mutation.Report, views []string) error {
	for _, v := range views {
		if !viewName.MatchString(v) {
			return fmt.Errorf("invalid mutation view name %q", v)
		}
	}
	for _, r := range reports {
		if r.Detail == nil {
			continue
		}
		att, reason, _ := readAttestation(r.Target, r.Engine, r.SHA256)
		if reason != "" || att.Views == nil {
			continue
		}
		names := sortedKeys(att.Views)
		for _, v := range views {
			if !slices.Contains(names, v) {
				return fmt.Errorf("mutation view %q is not in the views of %s (%s)", v, r.Target, strings.Join(names, ", "))
			}
		}
	}
	return nil
}
