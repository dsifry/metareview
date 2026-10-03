package sourcereview

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GitFunc runs git with optional stdin and returns stdout. A nil GitFunc means realGit.
type GitFunc func(root string, args []string, stdin []byte) ([]byte, error)

// gitBin is the git executable. Tests point it at a missing name to cover a failed start.
var gitBin = "git"

func realGit(root string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.Command(gitBin, append([]string{"-C", root}, args...)...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), bytes.TrimSpace(ee.Stderr))
		}
		return nil, err
	}
	return out, nil
}

func resolveTop(root string, git GitFunc) (string, error) {
	out, err := git(root, []string{"rev-parse", "--show-toplevel"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type treeBlob struct {
	path string
	id   string
}

// parseTree reads `git ls-tree -r -z` records, preserving arbitrary filenames
// and the object IDs used to read this exact snapshot.
func parseTree(out []byte) ([]treeBlob, error) {
	var blobs []treeBlob
	rest := out
	for len(rest) > 0 {
		rec, next, _ := bytes.Cut(rest, []byte{0})
		rest = next
		if len(rec) == 0 {
			continue
		}
		tab := bytes.IndexByte(rec, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("bad ls-tree record")
		}
		fields := strings.Fields(string(rec[:tab]))
		if len(fields) < 3 {
			return nil, fmt.Errorf("bad ls-tree meta")
		}
		if fields[1] != "blob" {
			continue
		}
		p := string(rec[tab+1:])
		if p == "" {
			continue
		}
		blobs = append(blobs, treeBlob{path: p, id: fields[2]})
	}
	return blobs, nil
}

func readBlobs(root string, blobs []treeBlob, git GitFunc) (map[string][]byte, error) {
	if len(blobs) == 0 {
		return map[string][]byte{}, nil
	}
	var stdin bytes.Buffer
	paths := make([]string, 0, len(blobs))
	for _, b := range blobs {
		// Object IDs avoid line-delimited filenames and any later HEAD lookup.
		fmt.Fprintln(&stdin, b.id)
		paths = append(paths, b.path)
	}
	out, err := git(root, []string{"cat-file", "--batch"}, stdin.Bytes())
	if err != nil {
		return nil, err
	}
	return parseBatch(out, paths)
}

func parseBatch(out []byte, paths []string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(paths))
	rest := out
	for _, p := range paths {
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			return nil, fmt.Errorf("short cat-file header for %s", p)
		}
		header := string(rest[:nl])
		rest = rest[nl+1:]
		parts := strings.Split(header, " ")
		if len(parts) < 3 || parts[1] != "blob" {
			return nil, fmt.Errorf("cat-file %s: %s", p, header)
		}
		size, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("cat-file %s: %w", p, err)
		}
		if size < 0 || len(rest) < size+1 {
			return nil, fmt.Errorf("short cat-file blob %s", p)
		}
		if rest[size] != '\n' {
			return nil, fmt.Errorf("cat-file %s: missing trailing newline", p)
		}
		body := make([]byte, size)
		copy(body, rest[:size])
		files[p] = body
		rest = rest[size+1:]
	}
	return files, nil
}
