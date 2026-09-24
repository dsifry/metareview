package mutationfresh

import "strings"

// Content-based line diff (spec §11.4.1), a port of templates/mutation-incremental/lib/diff.mjs.
// Both implementations run testdata/mutation-incremental/diff-vectors.json, so a change here needs
// the same change there. Above maxEditDistance lineDiff gives up and the caller treats the file as
// changed everywhere.
const maxEditDistance = 2000

// hunk is one changed region in old and new line numbers (1-based, inclusive). A pure insertion
// has OldEnd == OldStart-1; a pure deletion has NewEnd == NewStart-1.
type hunk struct {
	OldStart int `json:"oldStart"`
	OldEnd   int `json:"oldEnd"`
	NewStart int `json:"newStart"`
	NewEnd   int `json:"newEnd"`
}

func splitLines(text string) []string {
	text = strings.TrimPrefix(text, "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// editOps is Myers' O(ND) shortest edit script with the harness's tie rule: the insertion ("down")
// move when k == -d or v[k-1] < v[k+1], otherwise the deletion ("right") move. trace[d] keeps v for
// diagonals -d-1 .. d+1 as it was before step d.
func editOps(a, b []string) ([]byte, bool) {
	n, m := len(a), len(b)
	off := n + m + 1
	v := make([]int, 2*(n+m)+3)
	var trace [][]int
	for d := 0; d <= maxEditDistance; d++ {
		trace = append(trace, append([]int(nil), v[off-d-1:off+d+2]...))
		for k := -d; k <= d; k += 2 {
			x := v[off+k-1] + 1
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m), true
			}
		}
	}
	return nil, false
}

func backtrack(trace [][]int, n, m int) []byte {
	var ops []byte
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		w := trace[d]
		at := func(k int) int { return w[k+d+1] }
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			prevK = k + 1
		}
		prevX := at(prevK)
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			ops = append(ops, '=')
			x--
			y--
		}
		if d > 0 {
			if x == prevX {
				ops = append(ops, '+')
			} else {
				ops = append(ops, '-')
			}
		}
		x, y = prevX, prevY
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

// lineDiff returns the changed hunks between two texts after BOM and line-ending normalisation, or
// false when the edit distance exceeds maxEditDistance.
func lineDiff(oldText, newText string) ([]hunk, bool) {
	ops, ok := editOps(splitLines(oldText), splitLines(newText))
	if !ok {
		return nil, false
	}
	var hunks []hunk
	i, j := 0, 0
	open := false
	for _, op := range ops {
		if op == '=' {
			i++
			j++
			open = false
			continue
		}
		if !open {
			hunks = append(hunks, hunk{OldStart: i + 1, OldEnd: i, NewStart: j + 1, NewEnd: j})
			open = true
		}
		cur := &hunks[len(hunks)-1]
		if op == '-' {
			i++
			cur.OldEnd = i
		} else {
			j++
			cur.NewEnd = j
		}
	}
	return hunks, true
}

// hunkIntersects (spec §11.4.3): a mutant [start, end] (old lines) intersects a changed hunk when
// the ranges overlap, and a pure insertion when it contains the insertion point strictly inside.
func hunkIntersects(start, end int, h hunk) bool {
	if h.OldEnd >= h.OldStart {
		return start <= h.OldEnd && end >= h.OldStart
	}
	after := h.OldStart - 1
	return start <= after && end >= after+1
}
