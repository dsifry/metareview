package mutationfresh

import (
	"sort"

	"github.com/dsifry/metareview/internal/mutation"
)

// ReportFreshness is one report's classification (spec §6.3). It is serialized into pr-ready's
// reviewer-input digest (§6.7), so it holds no machine-dependent path.
type ReportFreshness struct {
	Path              string       `json:"-"`
	Engine            string       `json:"engine"`
	Attested          bool         `json:"attested"`
	UnattestedReason  string       `json:"unattestedReason,omitempty"`
	ReportSha256      string       `json:"reportSha256"`
	AttestationSha256 string       `json:"attestationSha256,omitempty"`
	Verified          int          `json:"verified"`
	Stale             int          `json:"stale"`
	Pending           int          `json:"pending"`
	Unbound           int          `json:"unbound"`
	Unattested        int          `json:"unattested"`
	Causes            []CauseCount `json:"staleCauses,omitempty"`
	ReRun             []ReRunRow   `json:"reRun,omitempty"`
	Deferrals         []string     `json:"deferrals,omitempty"`
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

type change struct{ category, digest string }

// Classify reads the report's attestation and gives every kill exactly one class (spec §6.3):
// stale (first cause wins), pending, unbound or verified. Kills in an unattested report are
// unattested. Survivors and every other status are never reclassified.
func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
	out := ReportFreshness{Path: r.Target, Engine: r.Engine, ReportSha256: r.SHA256}
	att, reason, attSHA := readAttestation(r.Target, r.Engine, r.SHA256)
	out.AttestationSha256 = attSHA
	if reason == "" && r.Detail == nil {
		reason = "engine" // only the harness attests, and it attests Stryker reports
	}
	if reason != "" {
		out.UnattestedReason = reason
		out.Unattested = r.Score().Killed
		return out, nil
	}
	out.Attested = true
	for _, d := range att.Deferrals {
		out.Deferrals = append(out.Deferrals, d.Reason)
	}
	changed, err := changedPaths(att, content)
	if err != nil {
		return ReportFreshness{}, err
	}
	causeOf := causeFinder(r.Detail, changed)
	stale := map[string]int{}
	rows := map[ReRunRow]int{}
	for _, file := range sortedKeys(r.Detail.Files) {
		for _, m := range r.Detail.Files[file].Mutants {
			if m.Status != mutation.Killed {
				continue
			}
			if cause := causeOf(file, m.KilledBy); cause != "" {
				out.Stale++
				stale[cause]++
				rows[ReRunRow{File: file, Cause: cause}]++
				continue
			}
			_, attested := att.Files[file]
			switch {
			case deferred(att.Deferrals, file):
				out.Pending++
			case !attested || len(m.KilledBy) == 0:
				out.Unbound++
			default:
				out.Verified++
			}
		}
	}
	for _, cause := range sortedKeys(stale) {
		out.Causes = append(out.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: stale[cause]})
	}
	for row, kills := range rows {
		out.ReRun = append(out.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
	}
	sort.Slice(out.ReRun, func(i, j int) bool {
		if out.ReRun[i].File != out.ReRun[j].File {
			return out.ReRun[i].File < out.ReRun[j].File
		}
		return out.ReRun[i].Cause < out.ReRun[j].Cause
	})
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
			changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
			continue
		}
		if digest == entry.Digest || (content.Head() && !entry.Tracked && digest == Absent) {
			continue
		}
		changed[p] = change{category: entry.Category, digest: digest}
	}
	return changed, nil
}

// causeFinder returns a kill's recorded cause, in the §6.3 order: its own file; the byte-smallest
// changed test whose ids it was killed by; the byte-smallest changed mutate file with mutants whose
// coverage includes a killing test; else the byte-smallest changed support, global, unclassified or
// zero-mutant mutate path (the gate has no import graph, so these invalidate every kill).
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
		case c.category == "mutate" && len(d.Files[p].Mutants) > 0:
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

func deferred(deferrals []Deferral, file string) bool {
	for _, d := range deferrals {
		for _, p := range d.Paths {
			if p == "*" || p == file {
				return true
			}
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
