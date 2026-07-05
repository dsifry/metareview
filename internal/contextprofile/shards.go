package contextprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const DefaultMaxBytesPerShard = 60000

type ShardOptions struct {
	MaxBytesPerShard int
	GroupBy          string
}

type ShardPlan struct {
	SourceDiffHash string
	Shards         []Shard
}

type Shard struct {
	ID             string
	Paths          []string
	ByteCount      int
	SourceDiffHash string
	Reason         string
	PromptPackPath string
	Prompt         string
}

func PlanShards(profile Profile, options ShardOptions) (ShardPlan, error) {
	maxBytes := options.MaxBytesPerShard
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytesPerShard
	}
	groupBy := strings.TrimSpace(options.GroupBy)
	if groupBy == "" {
		groupBy = "path"
	}
	switch groupBy {
	case "path", "domain", "workunit":
	default:
		return ShardPlan{}, fmt.Errorf("unsupported shard grouping: %s", options.GroupBy)
	}
	files := append([]FileProfile{}, profile.Files...)
	sort.Slice(files, func(i, j int) bool {
		if files[i].DiffBytes == files[j].DiffBytes {
			return files[i].Path < files[j].Path
		}
		return files[i].DiffBytes > files[j].DiffBytes
	})
	sourceHash := sourceDiffHash(profile)
	var shards []Shard
	for _, group := range groupedFiles(files, groupBy) {
		for _, file := range group.Files {
			if strings.TrimSpace(file.Path) == "" {
				continue
			}
			index := shardForFile(shards, file, maxBytes, group.Key)
			if index < 0 {
				shards = append(shards, Shard{
					Paths:          []string{},
					SourceDiffHash: sourceHash,
					Reason:         shardReason(profile, groupBy, group.Key),
				})
				index = len(shards) - 1
			}
			shards[index].Paths = append(shards[index].Paths, file.Path)
			shards[index].ByteCount += file.DiffBytes
		}
	}
	for i := range shards {
		sort.Strings(shards[i].Paths)
		shards[i].ID = fmt.Sprintf("shard-%02d", i+1)
		shards[i].PromptPackPath = fmt.Sprintf("docs/metareview/shards/%s-%s.md", sourceHash, shards[i].ID)
		shards[i].Prompt = shardPrompt(shards[i])
	}
	return ShardPlan{SourceDiffHash: sourceHash, Shards: shards}, nil
}

type fileGroup struct {
	Key   string
	Files []FileProfile
}

func groupedFiles(files []FileProfile, groupBy string) []fileGroup {
	if groupBy == "path" {
		return []fileGroup{{Key: "path", Files: files}}
	}
	byKey := map[string][]FileProfile{}
	for _, file := range files {
		key := shardGroupKey(file.Path)
		byKey[key] = append(byKey[key], file)
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]fileGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, fileGroup{Key: key, Files: byKey[key]})
	}
	return groups
}

func shardGroupKey(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return "root"
	}
	if slash := strings.Index(path, "/"); slash > 0 {
		return path[:slash]
	}
	return "root"
}

func shardForFile(shards []Shard, file FileProfile, maxBytes int, groupKey string) int {
	if file.DiffBytes > maxBytes {
		return -1
	}
	for i, shard := range shards {
		if shard.Reason != "" && !strings.HasSuffix(shard.Reason, ":"+groupKey) && groupKey != "path" {
			continue
		}
		if shard.ByteCount+file.DiffBytes <= maxBytes {
			return i
		}
	}
	return -1
}

func shardReason(profile Profile, groupBy, groupKey string) string {
	if profile.RiskLevel == RiskContextRisk {
		if groupBy != "path" {
			return "context-risk;group-by-" + groupBy + ":" + groupKey
		}
		return "context-risk"
	}
	if groupBy != "path" {
		return "group-by-" + groupBy + ":" + groupKey
	}
	return "large-diff-shard"
}

func sourceDiffHash(profile Profile) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("raw=%d\nfiltered=%d\n", profile.RawDiffBytes, profile.FilteredDiffBytes))
	files := append([]FileProfile{}, profile.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		builder.WriteString(fmt.Sprintf("%s=%d\n", file.Path, file.DiffBytes))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:16]
}

func shardPrompt(shard Shard) string {
	return "Review this metareview shard.\n\n" +
		"Paths:\n- " + strings.Join(shard.Paths, "\n- ") + "\n\n" +
		"Report findings with file:line evidence, acceptance coverage, severity, disposition, and whether each finding is shard-local or cross-shard."
}
