package mutationfresh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Absent is the digest of a path that is missing or is neither a regular file nor a symlink (§6.2).
const Absent = "absent"

// Entry is one path's content under review: file bytes, or a symlink's link text.
type Entry struct {
	Data    []byte
	Symlink bool
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// digestOf is the §5.4 digest: sha256:<hex> of the bytes, or symlink:<hex> of the link text.
func digestOf(e Entry) string {
	if e.Symlink {
		return "symlink:" + sha256Hex(e.Data)
	}
	return "sha256:" + sha256Hex(e.Data)
}

// configDigest digests the harness config as the harness does (lib/snapshot.mjs configDigest):
// canonical JSON — keys sorted, two-space indent, a trailing newline — without `views`, so editing
// the view map is not a change. Text that is not a JSON object is digested raw (the harness would
// have refused it). The shared vectors pin the two implementations together.
func configDigest(data []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return digestOf(Entry{Data: data})
	}
	delete(raw, "views")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(raw) // a value decoded from JSON always encodes
	return "sha256:" + sha256Hex(buf.Bytes())
}
