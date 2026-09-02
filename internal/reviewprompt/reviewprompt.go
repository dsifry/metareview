// Package reviewprompt builds the adversarial-review prompt for a change, deterministically, from the
// changed-file set — so the driver hands a reviewer the framing verbatim and cannot mis-scope it. The
// class labels come from classify (suffix-based, host-portable); the "these are CODE changes, review as
// code" directive is emitted whenever any code or config file is present, which is the exact mistake this
// exists to prevent (calling a code-file comment edit "just comments").
package reviewprompt

import (
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/classify"
)

// Build renders the review prompt for base..HEAD over the given repo-relative changed files. kinds maps a
// path to its change type ("added" | "modified" | "deleted" | "renamed"); a path absent from it defaults
// to "modified". Pure and deterministic: same inputs, same bytes.
func Build(base string, changed []string, kinds map[string]string) string {
	files := dedupeSorted(changed)
	c := classify.Tally(files)
	// kindOf whitelists the change type. The value is interpolated into the prompt a reviewer subagent
	// reads, so — like the path, which is control-char sanitised below — it must not be trusted to be one
	// of the known kinds: an unexpected or crafted value (a hand-built kinds map) could otherwise inject a
	// line/heading. Anything not recognised falls back to "modified".
	kindOf := func(p string) string {
		switch kinds[p] {
		case "added", "modified", "deleted", "renamed":
			return kinds[p]
		default:
			return "modified"
		}
	}
	structural := false
	for _, f := range files {
		if k := kindOf(f); k == "deleted" || k == "renamed" {
			structural = true
			break
		}
	}

	var b strings.Builder
	b.WriteString("# Adversarial review — " + sanitizePath(base) + "..HEAD\n\n")
	if len(files) == 0 {
		b.WriteString("No reviewable changed files in this range.\n")
		return b.String()
	}
	b.WriteString(plural(len(files), "changed file") + ": ")
	b.WriteString(itoa(c.Code) + " code, " + itoa(c.Config) + " config, " + itoa(c.Docs) + " docs.\n\n")

	b.WriteString("## Directive\n\n")
	if c.HasCodeOrConfig() {
		b.WriteString("This change contains CODE/CONFIG changes — review them AS CODE: find real defects, " +
			"verify every claim against the source, and mutation-check any load-bearing test assertion. A " +
			"comment-only edit to a code or config file is still a code change.\n")
	}
	if c.Docs > 0 {
		b.WriteString("For DOCS changes, do NOT trust the prose: verify each claim against the source it " +
			"describes — a false claim in documentation is as dangerous as one in a comment.\n")
	}
	b.WriteString("\nEvery changed file below makes a claim until you have verified otherwise.\n\n")

	// A deletion or rename can break code that is NOT in this diff — the impacted sites are elsewhere and
	// unchanged. Say so in general terms, once, when any such change is present. Deliberately
	// language-agnostic: it holds for compiled and interpreted, statically and dynamically typed code.
	if structural {
		b.WriteString("## Impact\n\n")
		b.WriteString("This change DELETES or RENAMES something. That can break code NOT shown in this diff: " +
			"anywhere else that still refers to a removed or renamed name, symbol, function, type, import, " +
			"file, path, or key. For each deleted or renamed item, search the WHOLE codebase for its old " +
			"name/path and confirm nothing is left dangling. This holds in every language — a compiler or " +
			"linter catches some of it, but references made by string, reflection, dynamic import, " +
			"configuration, or path are not seen until they run.\n\n")
	}

	b.WriteString("## Files\n\n")
	for _, f := range files {
		// Sanitise the DISPLAY path (class is computed from the original): the prompt is consumed by a
		// reviewer subagent, so a path carrying a control character must not break the line structure or
		// inject a directive. git normally quotes such paths, but this pure function must not depend on
		// its callers for that.
		k := kindOf(f)
		line := "- [" + classify.Classify(f).String() + ", " + k + "] " + sanitizePath(f)
		switch k {
		case "deleted":
			line += " — a removal: confirm nothing elsewhere still references what this deletes"
		case "renamed":
			line += " — a rename: confirm every reference to the old name/path elsewhere was updated"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n## Report\n\n")
	b.WriteString("Read the " + sanitizePath(base) + "..HEAD diff and report real defects only, each with file:line, the " +
		"concrete inputs that trigger it, and the wrong observable outcome. Verify each finding in the " +
		"source before reporting — no speculation.\n")
	return b.String()
}

// dedupeSorted returns the input sorted with adjacent duplicates removed, without mutating the caller's
// slice. Build must not rely on a caller having deduplicated — a repeated path would otherwise be listed
// twice and inflate the count.
func dedupeSorted(in []string) []string {
	files := append([]string(nil), in...)
	sort.Strings(files)
	out := files[:0]
	for i, f := range files {
		if i == 0 || f != files[i-1] {
			out = append(out, f)
		}
	}
	return out
}

// sanitizePath replaces control characters (including newline, CR, tab) with '?', so a path can neither
// break the one-line-per-file structure nor smuggle a directive into the reviewer's prompt.
func sanitizePath(p string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, p)
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return itoa(n) + " " + unit + "s"
}

// itoa avoids strconv for a tiny non-negative count and keeps the package dependency-light.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
