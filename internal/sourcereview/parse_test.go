package sourcereview

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/lensoutput"
)

func keptFiles() map[string][]byte {
	return map[string][]byte{
		"pkg/app.go": []byte("package pkg\nfunc App() int { return 1 }\n"),
		"main.go":    []byte("package main\n"),
	}
}

func validEntry() string {
	return `{"tag":"bug","file":"pkg/app.go","start_line":1,"end_line":2,"issue":"off by one","consequence":"wrong answer","confidence":80,"severity":"P1"}`
}

func TestParseAccepts(t *testing.T) {
	files := keptFiles()
	got, _, err := Parse(`{"findings":[]}`, files)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	got, _, err = Parse(" \n"+`{"findings":[`+validEntry()+"]}\n", files)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	if err := got[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if got[0].File != "pkg/app.go" || got[0].StartLine != 1 || got[0].EndLine != 2 || got[0].Confidence != 80 {
		t.Fatalf("%+v", got[0])
	}
	fenced := "```json\n{\"findings\":[" + validEntry() + "]}\n```"
	if _, _, err := Parse(fenced, files); err != nil {
		t.Fatal(err)
	}
	bare := "```\n{\"findings\":[]}\n```"
	if _, _, err := Parse(bare, files); err != nil {
		t.Fatal(err)
	}
	crlf := "```json\r\n{\"findings\":[]}\r\n```"
	if _, _, err := Parse(crlf, files); err != nil {
		t.Fatal(err)
	}
	adv := strings.Replace(validEntry(), `"tag":"bug"`, `"tag":"advisory"`, 1)
	adv = strings.Replace(adv, `"confidence":80`, `"confidence":0`, 1)
	adv = strings.Replace(adv, `"severity":"P1"`, `"severity":"P0"`, 1)
	if _, _, err := Parse(`{"findings":[`+adv+`]}`, files); err != nil {
		t.Fatal(err)
	}
}

func TestParseRejects(t *testing.T) {
	files := keptFiles()
	bad := []string{
		"",
		"   ",
		`{"findings":null}`,
		`{}`,
		"```\n```",
		"```json\nnot json\n```",
		`not json`,
		`{not json`,
	}
	for _, text := range bad {
		if _, _, err := Parse(text, files); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	for _, text := range []string{
		"```json5\n{\"findings\":[]}\n```",
		"```json\n{\"findings\":[]}",
		`{"findings":[]} trailing`,
		`{"nope":1} {"findings":[]}`,
	} {
		if _, _, err := Parse(text, files); err != nil {
			t.Fatalf("object inside fence: %v %q", err, text)
		}
	}
	prose := "The review follows.\n" + `{"findings":[]}` + "\nThanks."
	if got, _, err := Parse(prose, files); err != nil || len(got) != 0 {
		t.Fatalf("prose around object: %v %v", got, err)
	}
}

func TestParseSkipsBadFindings(t *testing.T) {
	files := keptFiles()
	good := validEntry()
	bad := strings.Replace(validEntry(), `"pkg/app.go"`, `"missing.go"`, 1)
	got, skipped, err := Parse(`{"findings":[`+good+`,`+bad+`]}`, files)
	if err != nil || len(got) != 1 || got[0].File != "pkg/app.go" || len(skipped) != 1 {
		t.Fatalf("got %d skipped %v err %v", len(got), skipped, err)
	}
	over := strings.Replace(validEntry(), `"confidence":80`, `"confidence":101`, 1)
	got, skipped, err = Parse(`{"findings":[`+over+`]}`, files)
	if err != nil || len(got) != 0 || len(skipped) != 1 {
		t.Fatalf("confidence 101: got %d skipped %v err %v", len(got), skipped, err)
	}
	full := strings.Replace(validEntry(), `"confidence":80`, `"confidence":100`, 1)
	got, _, err = Parse(`{"findings":[`+full+`]}`, files)
	if err != nil || got[0].Confidence != 100 || got[0].Tag != lensoutput.TagBug {
		t.Fatal(err)
	}
	line := strings.Replace(validEntry(), `"end_line":2`, `"end_line":9`, 1)
	if _, skipped, err = Parse(`{"findings":[`+line+`]}`, files); err != nil || len(skipped) != 1 {
		t.Fatal(err, skipped)
	}
	if _, skipped, err = Parse(`{"findings":[1]}`, files); err != nil || len(skipped) != 1 {
		t.Fatal(err, skipped)
	}
	if _, skipped, err = Parse(`{"findings":[{"tag":"bug"}]}`, files); err != nil || len(skipped) != 1 {
		t.Fatal(err, skipped)
	}
}

func TestResolveShortPath(t *testing.T) {
	files := map[string][]byte{
		"web/vite.config.js": []byte("export default {}\n"),
		"pkg/app.go":         []byte("package pkg\n"),
		"cmd/app.go":         []byte("package cmd\n"),
	}
	text := `{"findings":[{"tag":"bug","file":"vite.config.js","start_line":1,"end_line":1,"issue":"i","consequence":"c","confidence":80,"severity":"P1"}]}`
	got, _, err := Parse(text, files)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].File != "web/vite.config.js" {
		t.Fatalf("file %s", got[0].File)
	}
	dotted := strings.Replace(text, `"vite.config.js"`, `"./web/vite.config.js"`, 1)
	got, _, err = Parse(dotted, files)
	if err != nil || got[0].File != "web/vite.config.js" {
		t.Fatalf("%v %+v", err, got)
	}
	slash := strings.Replace(text, `"vite.config.js"`, `"web\\\\vite.config.js"`, 1)
	got, _, err = Parse(slash, files)
	if err != nil || got[0].File != "web/vite.config.js" {
		t.Fatalf("backslash %v %+v", err, got)
	}
	short := strings.Replace(text, `"vite.config.js"`, `"app.go"`, 1)
	if _, skipped, err := Parse(short, files); err != nil || len(skipped) != 1 || !strings.Contains(skipped[0], "more than one") {
		t.Fatal(err, skipped)
	}
	missing := strings.Replace(text, `"vite.config.js"`, `"nope.js"`, 1)
	if _, skipped, err := Parse(missing, files); err != nil || len(skipped) != 1 || !strings.Contains(skipped[0], "not kept") {
		t.Fatal(err, skipped)
	}
	escape := strings.Replace(text, `"vite.config.js"`, `"../vite.config.js"`, 1)
	if _, skipped, err := Parse(escape, files); err != nil || len(skipped) != 1 {
		t.Fatal(err, skipped)
	}
	if _, err := resolveKept("  ", files); err == nil {
		t.Fatal("blank")
	}
	if _, err := resolveKept("./", files); err == nil {
		t.Fatal("dot")
	}
}

func TestResolveKeptLiteralDoubleDots(t *testing.T) {
	files := map[string][]byte{
		"web/.. -t=silently/coverage/index.html": []byte("source\n"),
		"pkg/file..name.go":                      []byte("source\n"),
	}
	for p := range files {
		got, err := resolveKept(p, files)
		if err != nil || got != p {
			t.Fatalf("valid kept path %q was rejected: %q %v", p, got, err)
		}
	}
	for _, p := range []string{"..", "../pkg/file..name.go", "other/../pkg/file..name.go", "pkg/..", `other\..\pkg\file..name.go`} {
		if _, err := resolveKept(p, files); err == nil {
			t.Fatalf("parent traversal accepted: %q", p)
		}
	}
}

func TestLineCountAndSplit(t *testing.T) {
	if lineCount(nil) != 0 || splitLines(nil) != nil {
		t.Fatal("empty")
	}
	if lineCount([]byte("a\n")) != 1 || len(splitLines([]byte("a\n"))) != 1 {
		t.Fatal("one line")
	}
	if lineCount([]byte("a\nb")) != 2 {
		t.Fatal("no trailing newline")
	}
	if lineCount([]byte("\n")) != 1 || splitLines([]byte("\n"))[0] != "" {
		t.Fatal("blank line")
	}
	if lineCount([]byte("\n\n")) != 2 || len(splitLines([]byte("\n\n"))) != 2 {
		t.Fatal("two blanks")
	}
	if excerpt([]byte("a\nb\n"), 1, 2) != "a\nb" {
		t.Fatal(excerpt([]byte("a\nb\n"), 1, 2))
	}
	if excerpt([]byte("a\n"), 0, 1) != "" || excerpt([]byte("a\n"), 1, 3) != "" || excerpt([]byte("a\n"), 2, 1) != "" {
		t.Fatal("excerpt guards")
	}
}

func TestObjectTextFenceWord(t *testing.T) {
	if _, err := objectText("```JSON\n{\"findings\":[]}\n```"); err != nil {
		t.Fatal(err)
	}
	if lettersOnly("json") != true || lettersOnly("json5") != false || lettersOnly("") != true {
		t.Fatal("lettersOnly")
	}
}
