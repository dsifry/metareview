// Package classify is a deterministic, host-portable classifier for a changed file's review FRAMING.
//
// It answers one question — is a path code, config, or documentation — from its suffix (and a few
// well-known basenames), with a Linguist-seeded table. It never shells out and never reads content, so it
// gives the same answer on every machine; a gate that classified differently on a developer's mac than in
// CI would not be deterministic. `file(1)`/mime is deliberately NOT used here: its output varies by
// platform and version, so it can only ever LABEL an unknown for a human, never decide anything.
//
// The class is only FRAMING — code/config/docs are all claim-bearing and all get reviewed; the class
// merely points the reviewer at the right failure mode (review-as-code vs verify-claims-against-source).
// So a misclassification never causes a file to skip review; the exempting decision (a whitespace-only,
// generated, or pure-rename diff makes no claim to verify) is content-based and lives elsewhere. Because
// of that, the safe default is Code: an unknown or extension-less path is treated as code, the direction
// that over-reviews rather than under-reviews.
package classify

import (
	"path"
	"strings"
)

// Class is a changed file's review framing. The zero value is Code, so an unknown path defaults safe.
type Class int

const (
	// Code is programming/markup source, and every unknown or extension-less path. Frame: review as code.
	Code Class = iota
	// Config is data/behavioral files (yaml, json, toml, lockfiles, …). Frame: review as config.
	Config
	// Docs is prose (markdown, rst, plain text, …). Frame: verify each claim against the source.
	Docs
)

func (c Class) String() string {
	switch c {
	case Config:
		return "config"
	case Docs:
		return "docs"
	default:
		return "code"
	}
}

// docsExt are Linguist `prose` extensions: files whose changes assert things in words. They are still
// reviewed — a false claim in prose is as dangerous as one in a comment — but framed as claim-verification.
var docsExt = map[string]bool{
	".md": true, ".markdown": true, ".mdown": true, ".mkd": true, ".mkdn": true,
	".rst": true, ".txt": true, ".text": true, ".adoc": true, ".asciidoc": true,
	".org": true, ".textile": true, ".rdoc": true, ".pod": true, ".wiki": true,
	".mediawiki": true, ".creole": true,
}

// configExt are Linguist `data` extensions plus common config: structured, behavioral, not prose. A yaml
// or json change alters behaviour (this repo's own sdlc-loop-clean.yaml is a workflow, not a document), so
// these are claim-bearing like code — the class only steers the framing.
var configExt = map[string]bool{
	".yaml": true, ".yml": true, ".json": true, ".json5": true, ".jsonc": true,
	".toml": true, ".ini": true, ".cfg": true, ".conf": true, ".properties": true,
	".env": true, ".xml": true, ".plist": true, ".csv": true, ".tsv": true,
	".lock": true, ".editorconfig": true,
}

// configBase are well-known config files identified by name, not extension.
var configBase = map[string]bool{
	"go.mod": true, "go.sum": true, ".gitignore": true, ".gitattributes": true,
	".dockerignore": true, "Gemfile.lock": true, "yarn.lock": true, "package-lock.json": true,
}

// Classify returns the review framing for a repo-relative path. Suffix and a few basenames only; anything
// unrecognised is Code (the safe, over-reviewing default).
func Classify(p string) Class {
	base := path.Base(p)
	if configBase[base] {
		return Config
	}
	ext := strings.ToLower(path.Ext(base))
	switch {
	case docsExt[ext]:
		return Docs
	case configExt[ext]:
		return Config
	default:
		return Code
	}
}

// Counts is the per-class tally of a changed-file set.
type Counts struct {
	Code, Config, Docs int
}

// Tally classifies each path and counts by class.
func Tally(paths []string) Counts {
	var c Counts
	for _, p := range paths {
		switch Classify(p) {
		case Config:
			c.Config++
		case Docs:
			c.Docs++
		default:
			c.Code++
		}
	}
	return c
}

// HasCodeOrConfig reports whether any path is code or config — i.e. whether the change carries a
// behavioral claim that must be reviewed as code. Docs are claim-bearing too, but this is the predicate a
// caller uses to decide the "review as code" framing specifically.
func (c Counts) HasCodeOrConfig() bool { return c.Code > 0 || c.Config > 0 }
