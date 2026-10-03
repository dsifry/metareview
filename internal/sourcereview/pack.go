package sourcereview

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxPromptBytes is the prompt budget. AGENTS.md uses 120000 as the size that
// cannot be held in one review context.
const MaxPromptBytes = 120000

// instruction is the same text for every model. Only the CLI and the model id change.
const instruction = `Review the first-party source below and return only {"findings":[...]}.
Each finding's tag is bug or advisory. Each finding's severity is P0, P1, P2, or P3.
Every field is present: tag, file, start_line, end_line, issue, consequence, confidence, severity.
confidence is an integer from 0 to 100.
file is one of the paths given below. start_line and end_line are line numbers in that file.
Return {"findings":[]} when there is nothing to report.
Do not write anything outside that JSON object.
Each source line below starts with its line number and "| ". That prefix is not part of the file. Cite those numbers.`

// numberLines prefixes each line of body with its line number and "| ",
// counting from first. Given the numbers, a model does not have to count lines
// to cite them: on one hh file this cut Opus from 47 s to 29 s and Grok from
// 634 s to 540 s per prompt, with no fewer findings.
func numberLines(body []byte, first int) []byte {
	var b bytes.Buffer
	n := first
	for _, line := range bytes.SplitAfter(body, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%d| ", n)
		b.Write(line)
		n++
	}
	return b.Bytes()
}

func sliceNote(path string, line int) string {
	return fmt.Sprintf("This prompt contains a byte slice of %s starting at original line %d. Citations use the original path and the original line numbers.", path, line)
}

func sliceHeader(path string, line int) string {
	return instruction + "\n" + sliceNote(path, line) + "\n"
}

// promptWith writes the instruction header and each path on its own line
// immediately before that file's numbered lines. A following path stays on its
// own line when the previous body does not end in a newline.
func promptWith(header string, files []File) []byte {
	return promptFrom(header, files, 1)
}

// promptFrom is promptWith with the first file numbered from first: a slice of
// a large file is numbered from the original line that holds its first byte.
func promptFrom(header string, files []File, first int) []byte {
	var b bytes.Buffer
	b.WriteString(header)
	if !strings.HasSuffix(header, "\n") {
		b.WriteByte('\n')
	}
	for i, f := range files {
		start := 1
		if i == 0 {
			start = first
		}
		if i > 0 && (len(files[i-1].Body) == 0 || files[i-1].Body[len(files[i-1].Body)-1] != '\n') {
			b.WriteByte('\n')
		}
		b.WriteString(f.Path)
		b.WriteByte('\n')
		b.Write(numberLines(f.Body, start))
	}
	return b.Bytes()
}

// Pack packs whole kept files into the smallest number of prompts that stay
// within budget (first-fit decreasing). A file that cannot fit in one prompt
// is cut into consecutive non-empty UTF-8 slices that partition it.
func Pack(files []File, budget int) ([][]byte, error) {
	if budget <= 0 {
		return nil, fmt.Errorf("prompt budget must be positive")
	}
	var whole []sizedFile
	var big []File
	for _, f := range files {
		size := len(f.Path) + 1 + numberedSize(f.Body)
		if len(instruction)+1+size <= budget {
			whole = append(whole, sizedFile{File: f, size: size})
		} else {
			big = append(big, f)
		}
	}
	prompts := packWhole(whole, budget)
	sort.Slice(big, func(i, j int) bool { return big[i].Path < big[j].Path })
	for _, f := range big {
		parts, err := sliceFile(f, budget)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, parts...)
	}
	return prompts, nil
}

type sizedFile struct {
	File
	size int
}

// numberedSize measures the rendered body without building a prompt.
func numberedSize(body []byte) int {
	size := len(body)
	for line := 1; len(body) > 0; line++ {
		size += len(strconv.Itoa(line)) + 2
		_, body, _ = bytes.Cut(body, []byte("\n"))
	}
	return size
}

func packWhole(files []sizedFile, budget int) [][]byte {
	sorted := append([]sizedFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i].Body) == len(sorted[j].Body) {
			return sorted[i].Path < sorted[j].Path
		}
		return len(sorted[i].Body) > len(sorted[j].Body)
	})
	type bin struct {
		files     []File
		size      int
		separator bool
	}
	var bins []bin
	for _, f := range sorted {
		placed := false
		for i := range bins {
			size := bins[i].size + f.size
			if bins[i].separator {
				size++
			}
			if size <= budget {
				bins[i].files = append(bins[i].files, f.File)
				bins[i].size = size
				bins[i].separator = len(f.Body) == 0 || f.Body[len(f.Body)-1] != '\n'
				placed = true
				break
			}
		}
		if !placed {
			bins = append(bins, bin{files: []File{f.File}, size: len(instruction) + 1 + f.size,
				separator: len(f.Body) == 0 || f.Body[len(f.Body)-1] != '\n'})
		}
	}
	out := make([][]byte, 0, len(bins))
	for _, bin := range bins {
		out = append(out, promptWith(instruction, bin.files))
	}
	return out
}

func sliceFile(f File, budget int) ([][]byte, error) {
	var out [][]byte
	start := 0
	for start < len(f.Body) {
		line := lineAt(f.Body, start)
		header := sliceHeader(f.Path, line)
		room := budget - len(header) - len(f.Path) - 1
		end, err := cutNumbered(f.Body, start, room, line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		out = append(out, promptFrom(header, []File{{Path: f.Path, Body: f.Body[start:end]}}, line))
		start = end
	}
	return out, nil
}

// cutNumbered is cutEnd for a slice whose lines are numbered from line: the
// numbered slice, not the raw bytes, must fit in room. While it does not, the
// raw cut shrinks in proportion to the overshoot (numbering can multiply the
// size of short lines). size > room, so each pass cuts strictly fewer bytes.
func cutNumbered(content []byte, start, room, line int) (int, error) {
	raw := room
	for {
		end, err := cutEnd(content, start, raw)
		if err != nil {
			return 0, err
		}
		size := len(numberLines(content[start:end], line))
		if size <= room {
			return end, nil
		}
		raw = (end - start) * room / size
	}
}

func lineAt(content []byte, offset int) int {
	return bytes.Count(content[:offset], []byte("\n")) + 1
}

// cutEnd returns the byte end of a non-empty slice that starts at start, is at
// most room bytes, and ends on a UTF-8 character boundary.
func cutEnd(content []byte, start, room int) (int, error) {
	if room < 1 {
		return 0, fmt.Errorf("prompt overhead exceeds the budget")
	}
	end := start + room
	// end == len(content) takes the rest of the file; content[end] would be out of range.
	if end >= len(content) {
		end = len(content)
	} else {
		for end > start && !utf8.RuneStart(content[end]) {
			end--
		}
	}
	if end == start {
		return 0, fmt.Errorf("a character does not fit in one prompt")
	}
	return end, nil
}
