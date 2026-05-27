package integration

import (
	"os"
	"path/filepath"
)

type MetaswarmDescriptor struct {
	SchemaVersion int                `json:"schemaVersion"`
	Name          string             `json:"name"`
	Present       bool               `json:"present"`
	Path          string             `json:"path,omitempty"`
	Surfaces      []MetaswarmSurface `json:"surfaces"`
	Contract      MetaswarmContract  `json:"contract"`
}

type MetaswarmSurface struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

type MetaswarmContract struct {
	Flow              []FlowHook             `json:"flow"`
	PostMergeLearning PostMergeLearningHook  `json:"postMergeLearning"`
	HookInstallation  HookInstallationPolicy `json:"hookInstallation"`
}

type FlowHook struct {
	Stage      string `json:"stage"`
	Metareview string `json:"metareview"`
	Effect     string `json:"effect"`
}

type PostMergeLearningHook struct {
	Command         string `json:"command"`
	StrictByDefault bool   `json:"strictByDefault"`
	FailureBehavior string `json:"failureBehavior"`
}

type HookInstallationPolicy struct {
	InScope bool   `json:"inScope"`
	Reason  string `json:"reason"`
}

func InspectMetaswarm(root string) MetaswarmDescriptor {
	metaswarmPath := filepath.Clean(filepath.Join(root, "..", "metaswarm"))
	descriptor := MetaswarmDescriptor{
		SchemaVersion: 1,
		Name:          "metaswarm",
		Contract:      defaultMetaswarmContract(),
	}
	if !isDirectory(metaswarmPath) {
		return descriptor
	}

	descriptor.Present = true
	descriptor.Path = "../metaswarm"
	descriptor.Surfaces = discoverMetaswarmSurfaces(metaswarmPath)
	return descriptor
}

func defaultMetaswarmContract() MetaswarmContract {
	return MetaswarmContract{
		Flow: []FlowHook{
			{
				Stage:      "task-done",
				Metareview: "metareview review task-done <task-id-or-path>",
				Effect:     "Blocks local task closure when unresolved blocking findings remain.",
			},
			{
				Stage:      "epic-ready",
				Metareview: "metareview review epic-ready <epic-id-or-path>",
				Effect:     "Blocks epic landing when integration, acceptance, or intent-drift findings remain.",
			},
			{
				Stage:      "pr-ready",
				Metareview: "metareview review pr-ready --base <base-ref>",
				Effect:     "Runs before PR push or merge readiness to catch branch-level review blockers.",
			},
			{
				Stage:      "post-merge-learning",
				Metareview: "metareview learn --post-merge <pr-number> --base <pre-merge-ref>",
				Effect:     "Curates accepted/discarded learning and reviewer calibration after confirmed merge.",
			},
		},
		PostMergeLearning: PostMergeLearningHook{
			Command:         "metareview learn --post-merge <pr-number> --base <pre-merge-ref>",
			StrictByDefault: false,
			FailureBehavior: "Learning failures are advisory by default; callers block release cleanup only when they explicitly opt into strict mode.",
		},
		HookInstallation: HookInstallationPolicy{
			InScope: false,
			Reason:  "Automatic hook installation is out of scope for this slice; callers invoke the documented command contract explicitly.",
		},
	}
}

func discoverMetaswarmSurfaces(root string) []MetaswarmSurface {
	candidates := []MetaswarmSurface{
		{Kind: "instruction", Path: "AGENTS.md", Purpose: "Codex-facing workflow rules, quality gates, Beads usage, and completion requirements."},
		{Kind: "instruction", Path: "CLAUDE.md", Purpose: "Claude-facing metaswarm command list, quality gates, and lifecycle enforcement."},
		{Kind: "command", Path: ".claude/commands/start-task.md", Purpose: "Starts the metaswarm task pipeline."},
		{Kind: "command", Path: ".claude/commands/pr-shepherd.md", Purpose: "Monitors PRs through merge readiness."},
		{Kind: "command", Path: ".claude/commands/self-reflect.md", Purpose: "Captures learnings before branch finish or after merge."},
		{Kind: "command", Path: "commands/pr-shepherd.md", Purpose: "Portable command surface for PR shepherding."},
		{Kind: "skill", Path: "skills/orchestrated-execution/SKILL.md", Purpose: "Four-phase work-unit loop with validation and adversarial review."},
		{Kind: "skill", Path: "skills/pr-shepherd/SKILL.md", Purpose: "PR monitoring, CI handling, review comment handling, and merge readiness."},
		{Kind: "skill", Path: "skills/plan-review-gate/SKILL.md", Purpose: "Adversarial plan review gate before implementation."},
		{Kind: "rubric", Path: "rubrics/adversarial-review-rubric.md", Purpose: "Fresh reviewer rubric for work-unit adversarial review."},
	}

	surfaces := []MetaswarmSurface{}
	for _, candidate := range candidates {
		if fileExists(filepath.Join(root, filepath.FromSlash(candidate.Path))) {
			surfaces = append(surfaces, candidate)
		}
	}
	return surfaces
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
