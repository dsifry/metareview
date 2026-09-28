package findings

import "github.com/dsifry/metareview/internal/scope"

// loadScope is the seam over reading the branch in hand (scope.Load with the real git runner).
var loadScope = func(root string) scope.Scope { return scope.Load(root, nil) }

// ScopedBlocking splits the unresolved blockers in the checkout's ledger into this branch's and the rest (#178). The
// ledger is per checkout, not per branch, so without this a finding raised on branch A blocks branch B after a
// `git switch`. Each finding is placed by the one rule the abandoned-run scan uses (internal/scope): in scope when it
// was recorded on this branch or one of its former names, or its head lies in this branch's range; a row with no
// branch (before #178, or recorded on a detached HEAD) is in scope unless git shows its head belongs nowhere here. An
// unreadable scope keeps every blocker in scope (fail closed). elsewhere holds the blockers of other branches and of
// none, which never block here. With no blockers at all, git is not asked.
func ScopedBlocking(root string) (inScope, elsewhere []Record, err error) {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return nil, nil, err
	}
	blockers := unresolvedBlockingFrom(records)
	if len(blockers) == 0 {
		return blockers, nil, nil
	}
	sc := loadScope(root)
	inScope = make([]Record, 0, len(blockers))
	for _, record := range blockers {
		if sc.Classify(record.Branch, record.GitHead) == scope.InScope {
			inScope = append(inScope, record)
		} else {
			elsewhere = append(elsewhere, record)
		}
	}
	return inScope, elsewhere, nil
}

// UnresolvedBlockingAllBranches is every unresolved blocker in the checkout's ledger, whichever branch raised it: for a
// caller that selects blockers by an explicit target rather than by the branch in hand. Epic-ready surfaces its child
// tasks' blockers this way, since a child is usually reviewed on a branch of its own that is later squash- or
// rebase-merged into the epic's, which the branch scope would place elsewhere.
func UnresolvedBlockingAllBranches(root string) ([]Record, error) {
	records, err := readJSONL(findingsPath(root))
	if err != nil {
		return nil, err
	}
	return unresolvedBlockingFrom(records), nil
}
