package run

import (
	"crypto/sha1" // #nosec G505 -- identity digest, not security
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding-identity (T0.1). The stable id of a finding is derived from (file, normalized-text) so the
// same fault survives whitespace/case (T1), line-number drift (T2) and a location prefix (T3) across
// rounds, while two different faults never collapse (precision). Genuine lexical rewording (T4) is
// NOT canonicalized by any lexical normalization — Similarity exists to measure how far a similarity
// match can carry T4 recall, since an exact key over normalized text cannot (spec §9.8 R6).

var shardPrefix = regexp.MustCompile(`^\[(shard-[0-9a-z-]+|cross-shard)\]\s*`)
var digitRun = regexp.MustCompile(`\d+`)
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
var wsRun = regexp.MustCompile(`\s+`)

// NormalizeText is the N2+N3 normalization (spike §4): strip a leading [shard-XX]/[cross-shard]
// prefix (T3), lowercase, collapse digit runs to a single '#' so "line 42" ≡ "line 51" (T2), drop
// punctuation, and collapse whitespace (T1). It is pure and deterministic — no locale, no embedding.
func NormalizeText(s string) string {
	s = shardPrefix.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = digitRun.ReplaceAllString(s, "#")
	s = nonWord.ReplaceAllString(s, " ")
	s = wsRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// normalizePath is the file half of the key. A cross-package cycle blocks reusing judge.NormalizePath
// (judge imports run), so the minimal, equivalent normalization lives here: slashes, no leading "./",
// lowercased, trimmed. An empty path is its own bucket (kept as "").
func normalizePath(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "./")
	return strings.ToLower(p)
}

// FindingKey is the file-aware finding identity: hex(sha1(normPath(file) \x00 normalizeText(text)))[:12].
// Same (file, normalized-text) → same key; a different file or a distinct fault → a different key.
// This supersedes the text-only BugID once T0.1's algorithm is frozen (spike §5.2 migration).
func FindingKey(file, text string) string {
	h := sha1.Sum([]byte(normalizePath(file) + "\x00" + NormalizeText(text))) // #nosec G401
	return hex.EncodeToString(h[:])[:12]
}

// SimTokens is the set of normalized tokens of a finding's text — the unit Similarity compares.
func simTokens(text string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, t := range strings.Fields(NormalizeText(text)) {
		set[t] = struct{}{}
	}
	return set
}

// Similarity is the deterministic Jaccard overlap of two findings' normalized token sets, in [0,1].
// It is the lexical, reproducible stand-in for "same fault" that §9.8 R6's Recall@10 evidence points
// to — used to measure whether a similarity-match continuity layer can reach the T4 recall floor an
// exact key cannot. Two empty texts are defined as identical (1.0).
func Similarity(a, b string) float64 {
	A, B := simTokens(a), simTokens(b)
	if len(A) == 0 && len(B) == 0 {
		return 1
	}
	inter := 0
	for t := range A {
		if _, ok := B[t]; ok {
			inter++
		}
	}
	union := len(A) + len(B) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
