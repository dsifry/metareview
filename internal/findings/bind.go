package findings

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Override bindings. A granted override is an exception to a particular state of the code, not a
// permanent hole in the gate: it is bound to what justified it, and lapses when that changes.
//
// This is what makes unattended operation safe. Before it, `overridden` was terminal - the only
// transition was into it - and an overridden record counted as "active existing" during
// reconciliation, so a later run that rediscovered the same fingerprint filed nothing at all. One
// grant silenced that check on that target permanently, and every exception an autonomous loop
// took was a check it could never get back.
const (
	// BoundPaths: the finding named files that exist, and the exception lapses when their
	// contents change. This is the precise case - unrelated commits leave the exception alone.
	BoundPaths = "paths"
	// BoundHead: the finding named nothing watchable, so the exception is bound to the commit it
	// was granted at and lapses on the next one. Noisier, and deliberately so: a gate that cannot
	// work out what to watch must fail toward blocking rather than stay silent.
	BoundHead = "head"
)

// hashFile is a seam so the failure path is testable without depending on filesystem permissions.
var hashFile = func(path string) (string, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- path is repo-relative and checked by bindablePaths
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// pathish matches a path-shaped token: at least one slash, and an extension. Findings in this
// repository carry their file information in the fingerprint far more often than in Evidence.Path,
// which frequently holds a work-marker regex or a state marker ("diffTruncated") rather than a
// path. (The regex is not quoted here on purpose: the deterministic work-marker lint reads this
// file, and a comment describing the lint should not trip it.)
var pathish = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-]+)+\.[A-Za-z0-9]+`)

// bindablePaths returns the repo-relative files this finding names that actually exist. A path
// that resolves to nothing is not watchable: binding to it would produce a hash that never
// changes, which is a permanent override wearing the costume of a bound one.
func bindablePaths(root string, record Record) []string {
	seen := map[string]bool{}
	var out []string
	consider := func(raw string) {
		for _, m := range pathish.FindAllString(raw, -1) {
			clean := filepath.ToSlash(filepath.Clean(m))
			if clean == "" || seen[clean] || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
				continue
			}
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(clean))); err != nil || info.IsDir() {
				continue
			}
			seen[clean] = true
			out = append(out, clean)
		}
	}
	consider(record.Fingerprint)
	for _, e := range record.Evidence {
		consider(e.Path)
	}
	sort.Strings(out)
	return out
}

// bindingFor computes what an exception on this record is an exception to.
func bindingFor(root string, record Record, head string) (kind string, paths []string, hash string) {
	paths = bindablePaths(root, record)
	if len(paths) == 0 {
		return BoundHead, nil, strings.TrimSpace(head)
	}
	h := sha256.New()
	for _, p := range paths {
		sum, err := hashFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			// Unreadable now, whatever it was at grant time: treat as a change, which lapses the
			// exception rather than preserving it over a file nobody can inspect.
			sum = "unreadable"
		}
		_, _ = h.Write([]byte(p + "\x00" + sum + "\n"))
	}
	return BoundPaths, paths, hex.EncodeToString(h.Sum(nil))
}

// lapseStaleOverrides returns granted overrides to `open` when what they were granted over has
// changed. Grant provenance is kept: the audit has to be able to say that an exception existed,
// who granted it, and why it stopped applying.
func lapseStaleOverrides(root string, records []Record, run Run, now string) []Record {
	for i := range records {
		record := records[i]
		if record.Status != StatusOverridden || !sameRunTarget(record, run) {
			continue
		}
		head := record.GitHead
		if record.OverrideBoundKind == BoundHead {
			head = run.GitHead
		}
		kind, paths, hash := bindingFor(root, record, head)
		if kind == record.OverrideBoundKind && hash == record.OverrideBoundHash {
			continue
		}
		records[i].Status = "open"
		records[i].OverrideLapsedAt = now
		records[i].UpdatedAt = now
		records[i].OverrideBoundKind = kind
		records[i].OverrideBoundPaths = paths
		records[i].OverrideBoundHash = hash
	}
	return records
}
