package mutationfresh

import (
	"sort"

	"github.com/dsifry/metareview/internal/mutation"
)

// ReportFreshness is one report's classification (spec §6.3). It is serialized into pr-ready's
// reviewer-input digest (§6.7), so it holds no machine-dependent path.
type ReportFreshness struct {
	Path              string `json:"-"`
	Engine            string `json:"engine"`
	Attested          bool   `json:"attested"`
	UnattestedReason  string `json:"unattestedReason,omitempty"`
	ReportSha256      string `json:"reportSha256"`
	AttestationSha256 string `json:"attestationSha256,omitempty"`
	Tally
	Deferrals []string `json:"deferrals,omitempty"`
	// Views (spec §11.3) is one classification per requested view, in request order; nil when the
	// run requested none. ViewNames are the names in the report's own map (nil without one).
	Views     []ViewFreshness `json:"views,omitempty"`
	ViewNames []string        `json:"-"`
}

// Tally is the kill classes of one report or one view of it. Pending counts every pending kill;
// PendingInherited is the part covered only by inherited deferrals (§11.2).
type Tally struct {
	Verified         int          `json:"verified"`
	Stale            int          `json:"stale"`
	Pending          int          `json:"pending"`
	PendingInherited int          `json:"pendingInherited,omitempty"`
	Unbound          int          `json:"unbound"`
	Unattested       int          `json:"unattested"`
	Causes           []CauseCount `json:"staleCauses,omitempty"`
	ReRun            []ReRunRow   `json:"reRun,omitempty"`
}

// ViewFreshness is one view's classification. Scoped is false when the report has no view map (an
// unattested report, or one attested before views), in which case every kill counts.
type ViewFreshness struct {
	View   string `json:"view"`
	Scoped bool   `json:"scoped"`
	Tally
}

// CauseCount is one recorded cause: the changed path, its current digest and the kills it stales.
type CauseCount struct {
	Cause  string `json:"cause"`
	Digest string `json:"digest"`
	Kills  int    `json:"kills"`
}

// ReRunRow is one (file, cause) row of the re-run list (§6.6).
type ReRunRow struct {
	File  string `json:"file"`
	Cause string `json:"cause"`
	Kills int    `json:"kills"`
}

type change struct {
	category, digest string
	data             []byte // current content; nil when absent
}

// tallier accumulates one Tally, with its stale causes and re-run rows.
type tallier struct {
	t     Tally
	stale map[string]int
	rows  map[ReRunRow]int
}

func newTallier() *tallier {
	return &tallier{stale: map[string]int{}, rows: map[ReRunRow]int{}}
}

const (
	classVerified = iota
	classStale
	classPending
	classPendingInherited
	classUnbound
)

func (a *tallier) add(file string, class int, cause string) {
	switch class {
	case classStale:
		a.t.Stale++
		a.stale[cause]++
		a.rows[ReRunRow{File: file, Cause: cause}]++
	case classPendingInherited:
		a.t.PendingInherited++
		a.t.Pending++
	case classPending:
		a.t.Pending++
	case classUnbound:
		a.t.Unbound++
	default:
		a.t.Verified++
	}
}

func (a *tallier) finish(changed map[string]change) Tally {
	for _, cause := range sortedKeys(a.stale) {
		a.t.Causes = append(a.t.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: a.stale[cause]})
	}
	for row, kills := range a.rows {
		a.t.ReRun = append(a.t.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
	}
	sort.Slice(a.t.ReRun, func(i, j int) bool {
		return a.t.ReRun[i].File+"\x00"+a.t.ReRun[i].Cause < a.t.ReRun[j].File+"\x00"+a.t.ReRun[j].Cause
	})
	return a.t
}

// Classify reads the report's attestation and gives every kill exactly one class (spec §6.3):
// stale (first cause wins), pending, unbound or verified. Kills in an unattested report are
// unattested. Survivors and every other status are never reclassified. Each requested view tallies
// the kills in the files its patterns match (§11.3), or every kill when the report has no map.
func Classify(r mutation.Report, content Content, views []string) (ReportFreshness, error) {
	out := ReportFreshness{Path: r.Target, Engine: r.Engine, ReportSha256: r.SHA256}
	att, reason, attSHA := readAttestation(r.Target, r.Engine, r.SHA256)
	out.AttestationSha256 = attSHA
	if reason == "" && r.Detail == nil {
		reason = "engine" // only the harness attests, and it attests Stryker reports
	}
	if reason != "" {
		out.UnattestedReason = reason
		out.Unattested = r.Score().Killed
		for _, v := range views {
			out.Views = append(out.Views, ViewFreshness{View: v, Tally: Tally{Unattested: out.Unattested}})
		}
		return out, nil
	}
	out.Attested = true
	if att.Views != nil {
		out.ViewNames = sortedKeys(att.Views)
	}
	for _, d := range att.Deferrals {
		out.Deferrals = append(out.Deferrals, d.Reason)
	}
	changed, err := changedPaths(att, content)
	if err != nil {
		return ReportFreshness{}, err
	}
	causeOf := causeFinder(r.Detail, changed)
	total := newTallier()
	perView := make([]*tallier, len(views))
	for i := range views {
		perView[i] = newTallier()
	}
	for _, file := range sortedKeys(r.Detail.Files) {
		for _, m := range r.Detail.Files[file].Mutants {
			if m.Status != mutation.Killed {
				continue
			}
			class, cause := classVerified, causeOf(file, m.KilledBy)
			_, attested := att.Files[file]
			switch kind := deferred(att.Deferrals, file); {
			case cause != "":
				class = classStale
			case kind != classVerified:
				class = kind
			case !attested || len(m.KilledBy) == 0:
				class = classUnbound
			}
			total.add(file, class, cause)
			for i, v := range views {
				if att.Views == nil || MatchList(file, att.Views[v]) {
					perView[i].add(file, class, cause)
				}
			}
		}
	}
	out.Tally = total.finish(changed)
	for i, v := range views {
		out.Views = append(out.Views, ViewFreshness{View: v, Scoped: att.Views != nil, Tally: perView[i].finish(changed)})
	}
	return out, nil
}

// changedPaths: attested paths whose current digest differs (in HEAD mode, an untracked attested
// path absent from HEAD is skipped), plus present paths that are not attested, not excluded, not
// ignored, and neither mutate nor test (which cannot invalidate a kill). An unknown category counts
// as a blanket cause, as global does.
func changedPaths(att Attestation, content Content) (map[string]change, error) {
	present, err := content.Paths()
	if err != nil {
		return nil, err
	}
	var fresh []string
	for _, p := range present {
		if _, attested := att.Files[p]; attested || MatchList(p, att.Exclusions) {
			continue
		}
		switch Categorize(p, att.Lists) {
		case "ignore", "mutate", "test":
			continue
		}
		fresh = append(fresh, p)
	}
	paths := append(sortedKeys(att.Files), fresh...)
	entries, err := content.Read(paths)
	if err != nil {
		return nil, err
	}
	changed := map[string]change{}
	for _, p := range paths {
		digest := Absent
		if e, ok := entries[p]; ok {
			digest = digestOf(e)
			if p == att.Config && !e.Symlink {
				digest = configDigest(e.Data)
			}
		}
		entry, attested := att.Files[p]
		if !attested {
			if digest != Absent { // a listed path removed from disk is not present (§6.3)
				changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
			}
			continue
		}
		if digest == entry.Digest || (content.Head() && !entry.Tracked && digest == Absent) {
			continue
		}
		changed[p] = change{category: entry.Category, digest: digest, data: entries[p].Data}
	}
	return changed, nil
}

// causeFinder returns a kill's recorded cause, in the §6.3 order: its own file; the byte-smallest
// changed test whose ids it was killed by; the byte-smallest changed mutate file with mutants whose
// coverage includes a killing test; else the byte-smallest changed support, global, unclassified or
// mutate path with no mutants or with an edit outside every mutant (§11.5): the gate has no import
// graph, so these invalidate every kill.
func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file string, killedBy []string) string {
	testOf := map[string]string{}
	type covering struct {
		path string
		ids  map[string]bool
	}
	var mutated []covering
	var blanket []string
	for _, p := range sortedKeys(changed) {
		switch c := changed[p]; {
		case c.category == "test":
			for _, id := range d.TestIDs[p] {
				if _, seen := testOf[id]; !seen {
					testOf[id] = p
				}
			}
		case c.category == "mutate" && len(d.Files[p].Mutants) > 0 && !outOfMutant(d.Files[p], c):
			ids := map[string]bool{}
			for _, m := range d.Files[p].Mutants {
				for _, id := range m.CoveredBy {
					ids[id] = true
				}
			}
			mutated = append(mutated, covering{path: p, ids: ids})
		default:
			blanket = append(blanket, p)
		}
	}
	return func(file string, killedBy []string) string {
		if _, ok := changed[file]; ok {
			return file
		}
		best := ""
		for _, id := range killedBy {
			if p, ok := testOf[id]; ok && (best == "" || p < best) {
				best = p
			}
		}
		if best != "" {
			return best
		}
		for _, y := range mutated {
			for _, id := range killedBy {
				if y.ids[id] {
					return y.path
				}
			}
		}
		if len(blanket) > 0 {
			return blanket[0]
		}
		return ""
	}
}

// outOfMutant: the file's edit has a hunk that no mutant of any status intersects, or cannot be
// diffed at all. Mutant spans are lines of the source Stryker read, the diff's old side. A deleted
// file is changed everywhere, which its mutants intersect.
func outOfMutant(file mutation.StrykerFile, c change) bool {
	if c.digest == Absent {
		return false
	}
	hunks, ok := lineDiff(file.Source, string(c.data))
	if !ok {
		return true
	}
	for _, h := range hunks {
		hit := false
		for _, m := range file.Mutants {
			hit = hit || hunkIntersects(m.StartLine, m.EndLine, h)
		}
		if !hit {
			return true
		}
	}
	return false
}

// deferred is the pending class of a kill in file: classPending when any counted deferral covers
// it, else classPendingInherited when an inherited one does (§11.2), else classVerified (none).
func deferred(deferrals []Deferral, file string) int {
	kind := classVerified
	for _, d := range deferrals {
		for _, p := range d.Paths {
			if p != "*" && p != file {
				continue
			}
			if !d.Inherited {
				return classPending
			}
			kind = classPendingInherited
		}
	}
	return kind
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
