package gitcontext

import "testing"

func TestMaxDiffBytesAccommodatesMediumDeletionReviews(t *testing.T) {
	if maxDiffBytes < 100_000 {
		t.Fatalf("maxDiffBytes = %d, want at least 100000 for medium deletion reviews", maxDiffBytes)
	}
}
