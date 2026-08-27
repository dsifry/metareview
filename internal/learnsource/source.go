package learnsource

import (
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewlog"
)

type Options struct {
	Base     string
	GitHubPR string
}

type Context struct {
	ReviewLogs         []reviewlog.Summary   `json:"reviewLogs"`
	UnresolvedFindings []string              `json:"unresolvedFindings"`
	Git                gitcontext.Context    `json:"git"`
	GitHub             githubcontext.Context `json:"github"`
	GitHubMarkdown     string                `json:"githubMarkdown"`
	Knowledge          knowledge.Context     `json:"knowledge"`
}

func Collect(root string, options Options) (Context, error) {
	logs, err := reviewlog.Discover(root)
	if err != nil {
		return Context{}, err
	}
	// Committed shard results are audit records of the review, not source under
	// review: post-merge learning never reads them back.
	git, err := gitcontext.CollectWithExcludes(root, options.Base, []string{"docs/metareview/shards", "docs/metareview/shards/**"})
	if err != nil {
		return Context{}, err
	}
	gh, err := githubcontext.Collect(root, options.GitHubPR)
	if err != nil {
		return Context{}, err
	}
	knowledgeContext, err := knowledge.Collect(root)
	if err != nil {
		return Context{}, err
	}
	return Context{
		ReviewLogs:         logs,
		UnresolvedFindings: unresolvedFindings(logs),
		Git:                git,
		GitHub:             gh,
		GitHubMarkdown:     githubcontext.RenderMarkdown(gh),
		Knowledge:          knowledgeContext,
	}, nil
}

func unresolvedFindings(logs []reviewlog.Summary) []string {
	var ids []string
	for _, log := range logs {
		if !log.HasUnresolvedBlockers {
			continue
		}
		for _, id := range log.FindingIDs {
			ids = appendUnique(ids, id)
		}
	}
	return ids
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
