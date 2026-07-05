package contextprofile

import "testing"

func TestPlanShardsSplitsLargeProfilesDeterministically(t *testing.T) {
	profile := Profile{
		Files: []FileProfile{
			{Path: "internal/a/a.go", DiffBytes: 70},
			{Path: "internal/b/b.go", DiffBytes: 60},
			{Path: "cmd/metareview/main.go", DiffBytes: 40},
		},
		FilteredDiffBytes: 170,
	}

	first, err := PlanShards(profile, ShardOptions{MaxBytesPerShard: 100, GroupBy: "path"})
	if err != nil {
		t.Fatalf("plan shards: %v", err)
	}
	second, err := PlanShards(profile, ShardOptions{MaxBytesPerShard: 100, GroupBy: "path"})
	if err != nil {
		t.Fatalf("plan shards second time: %v", err)
	}

	if len(first.Shards) != 2 {
		t.Fatalf("len(Shards) = %d, want 2: %+v", len(first.Shards), first.Shards)
	}
	if first.SourceDiffHash == "" {
		t.Fatalf("SourceDiffHash should be populated")
	}
	if first.SourceDiffHash != second.SourceDiffHash || first.Shards[0].PromptPackPath != second.Shards[0].PromptPackPath {
		t.Fatalf("shard plan is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	for _, shard := range first.Shards {
		if shard.ByteCount > 100 {
			t.Fatalf("shard %s ByteCount = %d, want <= 100", shard.ID, shard.ByteCount)
		}
		if shard.PromptPackPath == "" {
			t.Fatalf("shard %s missing prompt pack path", shard.ID)
		}
		if shard.SourceDiffHash != first.SourceDiffHash {
			t.Fatalf("shard %s SourceDiffHash = %q, want plan hash %q", shard.ID, shard.SourceDiffHash, first.SourceDiffHash)
		}
		if !containsAll(shard.Prompt, []string{"file:line evidence", "acceptance coverage", "severity", "disposition", "shard-local", "cross-shard"}) {
			t.Fatalf("shard %s prompt lacks reviewer instructions:\n%s", shard.ID, shard.Prompt)
		}
	}
}

func TestPlanShardsHonorsDomainGrouping(t *testing.T) {
	profile := Profile{
		Files: []FileProfile{
			{Path: "internal/a.go", DiffBytes: 40},
			{Path: "docs/readme.md", DiffBytes: 40},
			{Path: "internal/b.go", DiffBytes: 40},
		},
		FilteredDiffBytes: 120,
	}

	plan, err := PlanShards(profile, ShardOptions{MaxBytesPerShard: 100, GroupBy: "domain"})
	if err != nil {
		t.Fatalf("plan shards: %v", err)
	}
	if len(plan.Shards) != 2 {
		t.Fatalf("len(Shards) = %d, want one docs shard and one internal shard: %+v", len(plan.Shards), plan.Shards)
	}
	for _, shard := range plan.Shards {
		hasDocs := containsString(stringsJoin(shard.Paths), "docs/")
		hasInternal := containsString(stringsJoin(shard.Paths), "internal/")
		if hasDocs && hasInternal {
			t.Fatalf("domain shard mixed docs and internal paths: %+v", shard)
		}
	}
}

func stringsJoin(values []string) string {
	out := ""
	for _, value := range values {
		out += value + "\n"
	}
	return out
}

func containsAll(text string, wants []string) bool {
	for _, want := range wants {
		if !containsString(text, want) {
			return false
		}
	}
	return true
}

func containsString(text, want string) bool {
	for i := 0; i+len(want) <= len(text); i++ {
		if text[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
