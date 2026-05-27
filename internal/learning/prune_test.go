package learning

import (
	"testing"

	"github.com/dsifry/metareview/internal/knowledge"
)

func TestPruneAcceptsOnlyBehaviorChangingCandidates(t *testing.T) {
	result := PruneCandidates(PruneInput{
		Candidates: []Candidate{
			{
				Kind:           "knowledge-candidate",
				Text:           "Before adding a new service path, reviewers should check SERVICE_INVENTORY and require reuse or an explicit split rationale.",
				Scope:          "repository-review",
				Provenance:     "review finding",
				SourceRefs:     []SourceRef{{Type: "finding", ID: "mrvf-1"}},
				Confidence:     "high",
				ProposedTarget: "repository-knowledge",
			},
			{
				Kind:           "knowledge-candidate",
				Text:           "Run tests before finishing.",
				Scope:          "repository-review",
				Provenance:     "review finding",
				SourceRefs:     []SourceRef{{Type: "finding", ID: "mrvf-2"}},
				Confidence:     "high",
				ProposedTarget: "repository-knowledge",
			},
		},
	})

	if len(result.Accepted) != 1 {
		t.Fatalf("expected one accepted candidate: %+v", result)
	}
	if len(result.Discarded) != 1 || result.Discarded[0].Reason != DiscardSelfEvident {
		t.Fatalf("expected self-evident discard: %+v", result.Discarded)
	}
}

func TestPruneDiscardsCoveredRegistryFollowupUnverifiedSpecificAndObsoleteCandidates(t *testing.T) {
	candidates := []Candidate{
		candidate("already", "Before adding a new service path, reviewers should check SERVICE_INVENTORY and require reuse.", "high"),
		candidate("registry", "Update SERVICE_INVENTORY.md to list internal/new_service.go.", "high"),
		candidate("follow", "Create a follow-up task to add missing auth tests.", "high"),
		candidate("unverified", "Reviewer correction from unverified session history.", "low"),
		candidate("specific", "In PR 42 for customer AcmeCorp, require deleting the temp FooBarBaz shim.", "high"),
		candidate("obsolete", "Obsolete reviewer rule for the deprecated legacy importer.", "high"),
	}

	result := PruneCandidates(PruneInput{
		Candidates: candidates,
		Knowledge: knowledge.Context{Facts: []knowledge.Fact{{
			Source: ".beads/knowledge/metareview.jsonl",
			Text:   "Before adding a new service path, reviewers should check SERVICE_INVENTORY and require reuse.",
		}}},
	})

	if len(result.Accepted) != 0 {
		t.Fatalf("expected all candidates discarded: %+v", result.Accepted)
	}
	assertDiscard(t, result.Discarded, "already", DiscardAlreadyCovered)
	assertDiscard(t, result.Discarded, "registry", DiscardRegistryUpdateNotKnowledge)
	assertDiscard(t, result.Discarded, "follow", DiscardFollowUpNotKnowledge)
	assertDiscard(t, result.Discarded, "unverified", DiscardUnverified)
	assertDiscard(t, result.Discarded, "specific", DiscardTooSpecific)
	assertDiscard(t, result.Discarded, "obsolete", DiscardObsolete)
	for _, discard := range result.Discarded {
		if !IsApprovedDiscardReason(discard.Reason) {
			t.Fatalf("unapproved discard reason: %+v", discard)
		}
	}
}

func candidate(kind, text, confidence string) Candidate {
	return Candidate{
		Kind:           kind,
		Text:           text,
		Scope:          "repository-review",
		Provenance:     "test",
		SourceRefs:     []SourceRef{{Type: "finding", ID: kind}},
		Confidence:     confidence,
		ProposedTarget: "repository-knowledge",
	}
}

func assertDiscard(t *testing.T, discarded []DiscardedCandidate, kind, reason string) {
	t.Helper()
	for _, discard := range discarded {
		if discard.Candidate.Kind == kind && discard.Reason == reason {
			return
		}
	}
	t.Fatalf("missing discard kind=%s reason=%s in %+v", kind, reason, discarded)
}
