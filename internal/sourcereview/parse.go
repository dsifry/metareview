package sourcereview

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/dsifry/metareview/internal/lensoutput"
)

// wireEntry matches internal/lensoutput.wireEntry: a missing field is a nil
// pointer. This command fails the run on that pointer; it does not call
// ValidatePayload.
type wireEntry struct {
	Tag         *string `json:"tag"`
	File        *string `json:"file"`
	StartLine   *int    `json:"start_line"`
	EndLine     *int    `json:"end_line"`
	Issue       *string `json:"issue"`
	Consequence *string `json:"consequence"`
	Confidence  *int    `json:"confidence"`
	Severity    *string `json:"severity"`
}

// Parse reads model text as {"findings":[...]}. files maps a kept path to its
// committed bytes so line numbers can be checked against that file. A finding
// that does not resolve to one kept file is skipped. The skipped messages are
// the second result. The whole text fails only when it has no findings object.
func Parse(text string, files map[string][]byte) ([]lensoutput.TypedFinding, []string, error) {
	body, err := extractFindings(text)
	if err != nil {
		return nil, nil, err
	}
	var top struct {
		Findings []json.RawMessage `json:"findings"`
	}
	_ = json.Unmarshal([]byte(body), &top)
	out := make([]lensoutput.TypedFinding, 0, len(top.Findings))
	var skipped []string
	for _, raw := range top.Findings {
		f, err := decodeEntry(raw)
		if err != nil {
			skipped = append(skipped, err.Error())
			continue
		}
		if err := f.Validate(); err != nil {
			skipped = append(skipped, err.Error())
			continue
		}
		kept, err := resolveKept(f.File, files)
		if err != nil {
			skipped = append(skipped, err.Error())
			continue
		}
		f.File = kept
		if f.EndLine > lineCount(files[kept]) {
			skipped = append(skipped, fmt.Sprintf("line numbers do not exist in %s", f.File))
			continue
		}
		out = append(out, f)
	}
	return out, skipped, nil
}

// resolveKept accepts the path the model wrote. An exact kept path wins.
// Otherwise a path matches when exactly one kept path ends with "/" plus the
// citation, so "vite.config.js" means "web/vite.config.js" when that is the
// only such file. Zero matches or two matches fail the run.
func resolveKept(cited string, files map[string][]byte) (string, error) {
	if _, ok := files[cited]; ok {
		return cited, nil
	}
	shown := cited
	cited = strings.ReplaceAll(strings.TrimSpace(cited), "\\", "/")
	// Only a complete parent-directory component is traversal. Literal dots
	// in a kept file or directory name must not discard a valid citation.
	if cited == "" || strings.Contains("/"+cited+"/", "/../") {
		return "", fmt.Errorf("file is not kept: %s", shown)
	}
	cited = strings.TrimPrefix(cited, "./")
	cited = strings.TrimPrefix(cited, "/")
	cited = path.Clean(cited)
	if cited == "." {
		return "", fmt.Errorf("file is not kept: %s", shown)
	}
	if _, ok := files[cited]; ok {
		return cited, nil
	}
	match := ""
	for p := range files {
		if strings.HasSuffix(p, "/"+cited) {
			if match != "" {
				return "", fmt.Errorf("file matches more than one kept path: %s", cited)
			}
			match = p
		}
	}
	if match == "" {
		return "", fmt.Errorf("file is not kept: %s", cited)
	}
	return match, nil
}

func decodeEntry(raw json.RawMessage) (lensoutput.TypedFinding, error) {
	var w wireEntry
	if err := json.Unmarshal(raw, &w); err != nil {
		return lensoutput.TypedFinding{}, fmt.Errorf("finding: %w", err)
	}
	if w.Tag == nil || w.File == nil || w.StartLine == nil || w.EndLine == nil ||
		w.Issue == nil || w.Consequence == nil || w.Confidence == nil || w.Severity == nil {
		return lensoutput.TypedFinding{}, errors.New("missing field")
	}
	return lensoutput.TypedFinding{
		Tag:         lensoutput.Tag(*w.Tag),
		File:        *w.File,
		StartLine:   *w.StartLine,
		EndLine:     *w.EndLine,
		Issue:       *w.Issue,
		Consequence: *w.Consequence,
		Confidence:  *w.Confidence,
		Severity:    *w.Severity,
	}, nil
}

// extractFindings returns the findings object. A clean object or one fence is
// preferred. If the model wrote prose around the object, the first JSON value
// that contains a findings array is used.
func extractFindings(text string) (string, error) {
	if body, err := objectText(text); err == nil && findingsObject(body) {
		return body, nil
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(text[i:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			continue
		}
		if findingsObject(string(raw)) {
			return string(raw), nil
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("missing model text")
	}
	return "", errors.New("missing findings object")
}

func findingsObject(s string) bool {
	dec := json.NewDecoder(strings.NewReader(s))
	var top struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if err := dec.Decode(&top); err != nil || top.Findings == nil {
		return false
	}
	var extra struct{}
	return errors.Is(dec.Decode(&extra), io.EOF)
}

// objectText trims surrounding whitespace and, when the text is one fence whose
// body is the object, returns that body. Any other extra text fails.
func objectText(text string) (string, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return "", errors.New("missing model text")
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if strings.HasPrefix(lines[0], "```") {
		word := strings.TrimSpace(lines[0][3:])
		if word != "" && !lettersOnly(word) {
			return "", errors.New("malformed fence")
		}
		if len(lines) < 2 || strings.TrimSpace(lines[len(lines)-1]) != "```" {
			return "", errors.New("malformed fence")
		}
		body := strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		if body == "" {
			return "", errors.New("missing findings object")
		}
		return body, nil
	}
	return s, nil
}

func lettersOnly(s string) bool {
	for _, r := range s {
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return true
}

func lineCount(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func splitLines(body []byte) []string {
	n := lineCount(body)
	if n == 0 {
		return nil
	}
	lines := make([]string, 0, n)
	rest := body
	for len(lines) < n {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			lines = append(lines, string(rest))
			break
		}
		lines = append(lines, string(rest[:i]))
		rest = rest[i+1:]
	}
	return lines
}
