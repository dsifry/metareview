package learning

import "github.com/dsifry/metareview/internal/gitpolicy"

// EnsureLearningGitPolicy ensures the shared metareview .gitignore block (ignore ephemeral state, keep durable
// learning state committable). The block is owned by internal/gitpolicy so this and setup's hook-install write
// ONE source and never orphan each other's marker. Kept as a named wrapper for its existing callers.
func EnsureLearningGitPolicy(root string) error {
	return gitpolicy.Ensure(root)
}
