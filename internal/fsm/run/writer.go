package run

import (
	"strconv"
	"strings"

	"github.com/dsifry/metareview/internal/version"
)

// ReaderVersion is the metareview version folding runs: this binary's (#180). A variable so a test can play an
// older reader.
var ReaderVersion = version.Version

// newerWriter reports whether a run written by writer must not be read by reader: its major.minor is newer. A
// patch release never changes the run format, so patch versions are ignored; "" is a writer from before 0.14,
// never newer. A writer version that does not parse is treated as newer — refused, never guessed at.
func newerWriter(writer, reader string) bool {
	if writer == "" {
		return false
	}
	wMajor, wMinor, ok := majorMinor(writer)
	if !ok {
		return true
	}
	rMajor, rMinor, ok := majorMinor(reader)
	if !ok {
		return true
	}
	return wMajor > rMajor || wMajor == rMajor && wMinor > rMinor
}

// majorMinor reads "X.Y" out of "X.Y.Z" (a leading "v" and any pre-release suffix allowed).
func majorMinor(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	return major, minor, errMajor == nil && errMinor == nil
}
