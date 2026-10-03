package sourcereview

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPackWholeFilesSmallest(t *testing.T) {
	files := []File{
		{Path: "a.go", Body: bytes.Repeat([]byte("x"), 100000)},
		{Path: "b.go", Body: bytes.Repeat([]byte("y"), 50000)},
		{Path: "c.go", Body: bytes.Repeat([]byte("z"), 60000)},
	}
	prompts, err := Pack(files, MaxPromptBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts = %d, want 2", len(prompts))
	}
	seen := map[string]int{}
	for _, p := range prompts {
		if len(p) > MaxPromptBytes {
			t.Fatalf("prompt is %d bytes", len(p))
		}
		if !bytes.Contains(p, []byte(instruction)) {
			t.Fatal("missing instruction")
		}
		if bytes.Contains(p, []byte("byte slice of")) {
			t.Fatal("whole file prompt has a slice note")
		}
		for _, f := range files {
			needle := append([]byte(f.Path+"\n"), numberLines(f.Body, 1)...)
			if bytes.Contains(p, needle) {
				seen[f.Path]++
			}
		}
	}
	for _, f := range files {
		if seen[f.Path] != 1 {
			t.Fatalf("%s seen %d times", f.Path, seen[f.Path])
		}
	}
}

func TestPackSlicesPartitionUTF8(t *testing.T) {
	body := bytes.Repeat([]byte("世"), 70000)
	f := File{Path: "pkg/wide.go", Body: body}
	prompts, err := Pack([]File{f}, MaxPromptBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) < 2 {
		t.Fatalf("prompts = %d", len(prompts))
	}
	off := 0
	files := map[string][]byte{f.Path: body}
	for _, p := range prompts {
		if len(p) > MaxPromptBytes {
			t.Fatalf("prompt is %d bytes", len(p))
		}
		header := sliceHeader(f.Path, lineAt(body, off))
		prefix := header + f.Path + "\n"
		if !strings.HasPrefix(string(p), prefix) {
			t.Fatalf("prompt missing slice header at offset %d", off)
		}
		// The body has no newlines, so each slice is one numbered line: the
		// original line that holds the slice's first byte.
		numbered := p[len(prefix):]
		mark := strconv.Itoa(lineAt(body, off)) + "| "
		if !bytes.HasPrefix(numbered, []byte(mark)) {
			t.Fatalf("slice at offset %d is not numbered %q", off, mark)
		}
		part := numbered[len(mark):]
		if len(part) == 0 {
			t.Fatal("empty slice")
		}
		if !utf8.Valid(part) {
			t.Fatal("slice is not valid UTF-8")
		}
		if !bytes.HasPrefix(body[off:], part) {
			t.Fatal("slice is not the next bytes of the file")
		}
		line := lineAt(body, off)
		text := `{"findings":[{"tag":"bug","file":"pkg/wide.go","start_line":` + strconv.Itoa(line) + `,"end_line":` + strconv.Itoa(line) + `,"issue":"i","consequence":"c","confidence":80,"severity":"P1"}]}`
		got, _, err := Parse(text, files)
		if err != nil {
			t.Fatalf("finding at original line %d: %v", line, err)
		}
		if got[0].File != f.Path || got[0].StartLine != line {
			t.Fatalf("citation %+v", got[0])
		}
		off += len(part)
	}
	if off != len(body) {
		t.Fatalf("partition covered %d of %d", off, len(body))
	}
}

func TestPackMixedAndSeparators(t *testing.T) {
	small := File{Path: "small.go", Body: []byte("package small\n")}
	empty := File{Path: "empty.go", Body: nil}
	nonl := File{Path: "nonl.go", Body: []byte("package nonl")}
	big := File{Path: "big.go", Body: bytes.Repeat([]byte("世"), 70000)}
	prompts, err := Pack([]File{small, empty, nonl, big}, MaxPromptBytes)
	if err != nil {
		t.Fatal(err)
	}
	var whole, slices int
	for _, p := range prompts {
		if bytes.Contains(p, []byte("byte slice of")) {
			slices++
			if !bytes.Contains(p, []byte("big.go\n")) {
				t.Fatal("slice prompt missing path")
			}
			continue
		}
		whole++
		for _, f := range []File{small, empty, nonl} {
			if !bytes.Contains(p, []byte(f.Path+"\n")) {
				t.Fatalf("whole prompt missing %s", f.Path)
			}
		}
		if bytes.Contains(p, []byte("big.go\n")) {
			t.Fatal("big file packed whole")
		}
	}
	if whole != 1 || slices < 2 {
		t.Fatalf("whole=%d slices=%d", whole, slices)
	}
}

func TestCutEndAndSliceErrors(t *testing.T) {
	if _, err := cutEnd([]byte("世"), 0, 0); err == nil {
		t.Fatal("room 0")
	}
	if _, err := cutEnd([]byte("世"), 0, 1); err == nil {
		t.Fatal("rune does not fit")
	}
	end, err := cutEnd([]byte("世世"), 0, 4)
	if err != nil || end != 3 {
		t.Fatalf("end=%d err=%v", end, err)
	}
	end, err = cutEnd([]byte("ab"), 0, 10)
	if err != nil || end != 2 {
		t.Fatalf("short end=%d err=%v", end, err)
	}
	// Room exactly equal to the bytes left takes the rest without reading past the end.
	end, err = cutEnd([]byte("abc"), 1, 2)
	if err != nil || end != 3 {
		t.Fatalf("exact end=%d err=%v", end, err)
	}
	header := sliceHeader("p.go", 1)
	if _, err := sliceFile(File{Path: "p.go", Body: []byte("世")}, len(header)+len("p.go")+1+1); err == nil {
		t.Fatal("sliceFile should reject a rune that does not fit")
	}
	if _, err := sliceFile(File{Path: "p.go", Body: []byte("hello")}, 10); err == nil {
		t.Fatal("sliceFile should reject a tiny budget")
	}
	if _, err := Pack([]File{{Path: "a.go", Body: []byte("package a\n")}}, 0); err == nil {
		t.Fatal("budget 0")
	}
	if _, err := Pack([]File{{Path: "a.go", Body: bytes.Repeat([]byte("x"), 50)}}, 10); err == nil {
		t.Fatal("pack should surface the slice error")
	}
	if prompts, err := Pack(nil, MaxPromptBytes); err != nil || len(prompts) != 0 {
		t.Fatalf("empty pack %v %v", prompts, err)
	}
	two, err := Pack([]File{
		{Path: "z.go", Body: bytes.Repeat([]byte("世"), 70000)},
		{Path: "a.go", Body: bytes.Repeat([]byte("世"), 70000)},
	}, MaxPromptBytes)
	if err != nil || len(two) < 2 {
		t.Fatalf("two big files: %v %d", err, len(two))
	}
}

func TestPromptNamesPathBeforeBody(t *testing.T) {
	body := []byte("package pkg\nfunc A() {}\n")
	p := promptWith(instruction, []File{{Path: "pkg/app.go", Body: body}})
	if !bytes.Contains(p, []byte("pkg/app.go\n1| package pkg\n2| func A() {}\n")) {
		t.Fatalf("path is not immediately before the numbered body:\n%s", p)
	}
	if !bytes.Contains(p, []byte(`{"findings":[...]}`)) || !bytes.Contains(p, []byte("Cite those numbers.")) {
		t.Fatal("instruction missing findings object or line-number note")
	}
}

func TestNumberLines(t *testing.T) {
	cases := []struct {
		body  string
		first int
		want  string
	}{
		{"", 1, ""},
		{"a\n", 1, "1| a\n"},
		{"a\nb", 1, "1| a\n2| b"},
		{"a\n\nb\n", 7, "7| a\n8| \n9| b\n"},
	}
	for _, c := range cases {
		if got := string(numberLines([]byte(c.body), c.first)); got != c.want {
			t.Fatalf("numberLines(%q, %d) = %q, want %q", c.body, c.first, got, c.want)
		}
	}
}

func TestSliceNumbersFromOriginalLine(t *testing.T) {
	// 30,000 two-byte lines do not fit one prompt, and numbering more than
	// doubles their size, so each cut has to shrink to fit the numbered text.
	var body bytes.Buffer
	for i := 1; i <= 30000; i++ {
		body.WriteString("x\n")
	}
	f := File{Path: "short.go", Body: body.Bytes()}
	const budget = 20000
	prompts, err := sliceFile(f, budget)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) < 2 {
		t.Fatalf("prompts = %d", len(prompts))
	}
	number := regexp.MustCompile(`(?m)^(\d+)\| `)
	off := 0
	for _, p := range prompts {
		if len(p) > budget {
			t.Fatalf("prompt is %d bytes", len(p))
		}
		numbered := p[bytes.Index(p, []byte("short.go\n"))+len("short.go\n"):]
		first := number.FindSubmatch(numbered)
		if first == nil || string(first[1]) != strconv.Itoa(lineAt(f.Body, off)) {
			t.Fatalf("slice at offset %d starts at %q, want line %d", off, first, lineAt(f.Body, off))
		}
		part := number.ReplaceAll(numbered, nil)
		if !bytes.HasPrefix(f.Body[off:], part) {
			t.Fatalf("slice at offset %d is not the next bytes of the file", off)
		}
		off += len(part)
	}
	if off != len(f.Body) {
		t.Fatalf("partition covered %d of %d", off, len(f.Body))
	}
}

func TestCutNumbered(t *testing.T) {
	// Numbered, "a\nb\nc" is 14 bytes for a room of 5; the cut shrinks until
	// the numbered text fits.
	end, err := cutNumbered([]byte("a\nb\nc\n"), 0, 5, 1)
	if err != nil || end != 1 {
		t.Fatalf("end=%d err=%v", end, err)
	}
	end, err = cutNumbered([]byte("abcdef"), 0, 5, 1)
	if err != nil || end != 2 {
		t.Fatalf("end=%d err=%v", end, err)
	}
	if _, err := cutNumbered([]byte("abc"), 0, 3, 100000); err == nil {
		t.Fatal("a number wider than the room cannot fit")
	}
}
