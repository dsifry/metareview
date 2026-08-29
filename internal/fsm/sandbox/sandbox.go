// Package sandbox materializes the evidence an agentic judge is allowed to read.
//
// The judge process runs with the filesystem access its CLI grants it. Left to inherit
// metareview's own working directory it can read the entire repository - .git, local
// configuration, anything else on disk - which is far wider than judging a diff requires.
// A sandbox narrows that to an enumerable set: the files the change actually touched, at
// both revisions, and nothing else.
//
// It is also what makes the audit honest. When the prompt no longer contains the evidence,
// the recorded input stops determining the verdict; content-addressing the tree restores it.
package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dirs are the two subdirectories a judge is told about.
const (
	Head = "head"
	Base = "base"
)

// ShowFunc returns one path's contents at one revision. A missing path is not an error:
// a file added on the branch has no base revision, and the judge is told so by its absence.
type ShowFunc func(rev, path string) ([]byte, bool, error)

// Tree is a materialized sandbox.
type Tree struct {
	Root     string
	BaseSHA  string
	HeadSHA  string
	TreeHash string // content address of everything written, so a verdict is replayable
	Files    int
}

// Materialize writes head/<path> and base/<path> under root for every path given.
//
// Paths are contained: anything that escapes root once cleaned is refused rather than
// written, because the path list ultimately derives from a diff and a diff is data.
func Materialize(root, baseSHA, headSHA string, paths []string, show ShowFunc) (*Tree, error) {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	h := sha256.New()
	fmt.Fprintf(h, "base %s\nhead %s\n", baseSHA, headSHA)
	t := &Tree{Root: root, BaseSHA: baseSHA, HeadSHA: headSHA}
	for _, rel := range sorted {
		clean := filepath.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return nil, fmt.Errorf("sandbox: path escapes the tree: %q", rel)
		}
		for _, side := range [...]struct{ dir, rev string }{{Head, headSHA}, {Base, baseSHA}} {
			body, ok, err := show(side.rev, rel)
			if err != nil {
				return nil, fmt.Errorf("sandbox: %s %s: %w", side.dir, rel, err)
			}
			if !ok {
				continue // absent at that revision
			}
			dest := filepath.Join(root, side.dir, clean)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dest, body, 0o444); err != nil { // read-only on disk too
				return nil, err
			}
			sum := sha256.Sum256(body)
			fmt.Fprintf(h, "%s %s %s\n", side.dir, clean, hex.EncodeToString(sum[:]))
			t.Files++
		}
	}
	t.TreeHash = hex.EncodeToString(h.Sum(nil))
	return t, nil
}
