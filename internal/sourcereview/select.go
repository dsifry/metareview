package sourcereview

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/classify"
)

// File is one kept source file. Body is the committed blob, untrimmed.
type File struct {
	Path string
	Body []byte
}

// generatedLine is the Go generated-file marker. A line that only says
// "DO NOT EDIT" does not match.
var generatedLine = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// Select lists HEAD blobs and splits them into kept first-party source and
// everything this command drops. Classify is unchanged: config and docs are
// still Code/Config/Docs for the diff review; this command drops Config and Docs.
func Select(root string, git GitFunc) (kept []File, dropped []string, commit string, err error) {
	return selectPaths(root, git, nil)
}

func selectPaths(root string, git GitFunc, filters []string) (kept []File, dropped []string, commit string, err error) {
	filters, err = cleanPaths(filters)
	if err != nil {
		return nil, nil, "", err
	}
	if git == nil {
		git = realGit
	}
	top, err := resolveTop(root, git)
	if err != nil {
		return nil, nil, "", err
	}
	head, err := git(top, []string{"rev-parse", "HEAD"}, nil)
	if err != nil {
		return nil, nil, "", err
	}
	commit = strings.TrimSpace(string(head))
	tree, err := git(top, []string{"ls-tree", "-r", "-z", commit}, nil)
	if err != nil {
		return nil, nil, "", err
	}
	entries, err := parseTree(tree)
	if err != nil {
		return nil, nil, "", err
	}
	var selected []treeBlob
	for _, entry := range entries {
		if !eligiblePath(entry.path) {
			dropped = append(dropped, entry.path)
		} else if pathSelected(entry.path, filters) {
			selected = append(selected, entry)
		}
	}
	blobs, err := readBlobs(top, selected, git)
	if err != nil {
		return nil, nil, "", err
	}
	for _, entry := range selected {
		p := entry.path
		body := blobs[p]
		if keepPath(p, body) {
			kept = append(kept, File{Path: p, Body: body})
		} else {
			dropped = append(dropped, p)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Path < kept[j].Path })
	sort.Strings(dropped)
	kept, err = onlyPaths(kept, filters)
	if err != nil {
		return nil, nil, "", err
	}
	return kept, dropped, commit, nil
}

// keepPath is the whole-repo cut. An unrecognised path is still Code to Classify
// and is kept when the other cuts do not apply.
func keepPath(p string, body []byte) bool {
	if !eligiblePath(p) {
		return false
	}
	if !utf8.Valid(body) {
		return false
	}
	if isGenerated(body) {
		return false
	}
	return true
}

func eligiblePath(p string) bool {
	return classify.Classify(p) == classify.Code && !claimcheck.IsTestPath(p) && !vendored(p)
}

func vendored(p string) bool {
	for _, s := range strings.Split(p, "/") {
		switch s {
		case "vendor", "node_modules", "third_party":
			return true
		}
	}
	return false
}

// isGenerated reports whether a line matches the generated marker and every
// line before that match is blank or a comment.
func isGenerated(body []byte) bool {
	inBlock := false
	allComment := true
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if allComment && generatedLine.MatchString(line) {
			return true
		}
		comment, next := classifyLine(line, inBlock)
		inBlock = next
		if !comment {
			allComment = false
		}
	}
	return false
}

// classifyLine reports whether line has no code outside a comment, and whether
// a block comment remains open. A blank line is a comment line. A line whose
// first non-whitespace characters are // is a comment line. /* opens a block
// and */ closes it. Code before /* or after */ means the line is not a comment,
// and the block still runs from /* to */.
func classifyLine(line string, inBlock bool) (comment bool, still bool) {
	code := false
	i := 0
	for i < len(line) {
		if inBlock {
			j := strings.Index(line[i:], "*/")
			if j < 0 {
				return !code, true
			}
			i += j + 2
			inBlock = false
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if strings.HasPrefix(line[i:], "//") {
			return !code, false
		}
		if strings.HasPrefix(line[i:], "/*") {
			inBlock = true
			i += 2
			continue
		}
		code = true
		i += size
	}
	return !code, inBlock
}
