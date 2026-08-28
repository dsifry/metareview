package state

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dsifry/metareview/internal/jsonl"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxJSONLLineBytes is this package's name for the shared JSONL line cap.
// bufio rejects a token equal to the buffer maximum, and ScanLines needs the
// line terminator to fit alongside the token, so callers size the buffer two
// bytes larger to admit a line of exactly this length ending in CRLF.
const maxJSONLLineBytes = jsonl.MaxLineBytes

// closeFile is a seam so the Close-failure branch below can be tested. A
// permission- or descriptor-based test would be the alternative, and those
// stop failing when the suite runs as root, which is the normal case in CI.
var closeFile = func(f *os.File) error { return f.Close() }

func AppendJSONL(path string, record any) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		// A write can fail at Close rather than at Write — a delayed ENOSPC, a
		// write-back error on a network filesystem. This is the append that
		// records every finding and every run, so swallowing that error would
		// let metareview report a row as recorded when it never landed.
		if cerr := closeFile(file); cerr != nil && err == nil {
			err = cerr
		}
	}()
	bytes, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = file.Write(append(bytes, '\n'))
	return err
}

func ReadJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Read-only path: there is nothing a Close error could tell the caller.
	defer func() { _ = file.Close() }()
	var records []T
	scanner := jsonl.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(value string) string {
	base := strings.ToLower(value)
	if dot := strings.LastIndex(base, "."); dot > 0 {
		base = base[:dot]
	}
	base = slugPattern.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > 48 {
		base = base[:48]
		base = strings.Trim(base, "-")
	}
	if base == "" {
		return "target"
	}
	return base
}

func RunID(scope, target string, at time.Time) string {
	hash := sha1.Sum([]byte(target))
	utc := at.UTC()
	stamp := fmt.Sprintf("%s%09d", utc.Format("20060102-150405"), utc.Nanosecond())
	return "mrv-" + stamp + "-" + Slugify(scope) + "-" + Slugify(filepath.Base(target)) + "-" + hex.EncodeToString(hash[:])[:8]
}

func FindingID(runID string, index int) string {
	id := strings.TrimPrefix(runID, "mrv-")
	return "mrvf-" + id + "-" + leftPad(index, 3)
}

func leftPad(value, width int) string {
	text := strconv.Itoa(value)
	for len(text) < width {
		text = "0" + text
	}
	return text
}
