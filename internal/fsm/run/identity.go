package run

import (
	"crypto/sha1" // #nosec G505 -- identity digest, not security
	"encoding/hex"
	"path"
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
// prefix (T3), lowercase, drop punctuation, then collapse each digit run to a single "#" token so
// "line 42" ≡ "line 51" (T2) while a text that mentions no number stays distinct, and collapse
// whitespace (T1). Punctuation is stripped BEFORE the digit collapse so the "#" placeholder is not
// itself removed as punctuation. Pure and deterministic — no locale, no embedding.
func NormalizeText(s string) string {
	s = shardPrefix.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = nonWord.ReplaceAllString(s, " ")
	s = digitRun.ReplaceAllString(s, " # ")
	s = wsRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// normalizePath is the file half of the key. A cross-package import cycle blocks reusing
// judge.NormalizePath (judge imports run), so its exact definition of "same file" is replicated here:
// trim, drop a leading "./", strip git "a/"/"b/" diff prefixes, and path.Clean. An empty path is its
// own bucket ("") so a fileless finding never matches another by similarity (SameFault guards on it).
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "./")
	for _, prefix := range []string{"a/", "b/"} {
		p = strings.TrimPrefix(p, prefix)
	}
	return path.Clean(p)
}

// FindingKey is the file-aware finding identity: hex(sha1(normPath(file) \x00 normalizeText(text)))[:12].
// Same (file, normalized-text) → same key; a different file or a distinct fault → a different key.
// This supersedes the text-only BugID once T0.1's algorithm is frozen (spike §5.2 migration).
func FindingKey(file, text string) string {
	h := sha1.Sum([]byte(normalizePath(file) + "\x00" + NormalizeText(text))) // #nosec G401
	return hex.EncodeToString(h[:])[:12]
}

// FindingScheme is the identity-derivation version (spike §5.2). Bump it whenever NormalizeText,
// FindingKey, or the continuity rule below changes, so ids/overrides/Unproven gaps keyed under an
// old scheme are MIGRATED forward from the retained source text — never silently orphaned.
const FindingScheme = 1

// ContinuityThreshold (τ) is the frozen Jaccard bar at/above which two SAME-FILE findings are the
// same fault for continuity (Unproven clear, class carry). Frozen 2026-09-01 against the pre-locked
// ground truth: T4 recall 92% (≥90% floor) and precision 100% on 274 real same-file distinct-fault
// pairs (whose max was 0.30, a 0.05 margin). Do not retune without re-running the §7 floors.
const ContinuityThreshold = 0.35

// SameFault reports whether two findings are the same fault for CONTINUITY. It is true when their
// exact identity matches — the whitespace/case/line/prefix rewordings NormalizeText canonicalizes
// (T1–T3) — OR when they are in the same (non-empty) file and their normalized-token Jaccard is at
// least τ — a genuine lexical rewording (T4) that no exact key can canonicalize. Deterministic: the
// frozen realization of §9.8 R6's lexical Recall@10, never an embedding. Identity (FindingKey) stays
// an exact hash; this is the separate continuity relation the exact key cannot express.
func SameFault(aFile, aText, bFile, bText string) bool {
	if FindingKey(aFile, aText) == FindingKey(bFile, bText) {
		return true
	}
	af, bf := normalizePath(aFile), normalizePath(bFile)
	return af != "" && af == bf && Similarity(aText, bText) >= ContinuityThreshold
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
	// Not both empty (guarded above), so the union has at least one element.
	union := len(A) + len(B) - inter
	return float64(inter) / float64(union)
}
