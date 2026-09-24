package mutationfresh

import "fmt"

// Modes (spec §6.4).
const (
	Advisory = "advisory"
	Enforce  = "enforce"
	ModeEnv  = "METAREVIEW_MUTATION_FRESHNESS"
)

// ModeFromEnv reads METAREVIEW_MUTATION_FRESHNESS: unset, empty or advisory is advisory; enforce is
// enforce; anything else is a usage error.
func ModeFromEnv(getenv func(string) string) (string, error) {
	switch value := getenv(ModeEnv); value {
	case "", Advisory:
		return Advisory, nil
	case Enforce:
		return Enforce, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q, got %q", ModeEnv, Advisory, Enforce, value)
	}
}

// EffectiveMode: task-done is always advisory; pr-ready and epic-ready use the configured mode.
func EffectiveMode(scope, mode string) string {
	if scope == "task-done" {
		return Advisory
	}
	return mode
}
