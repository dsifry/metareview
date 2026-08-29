package kind

import (
	"encoding/json"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
)

func errNoJudge() error {
	return errs.E(machine.CodeExecutorFailed, "this registry has no judge", "reason", "no_judge")
}

// Decision extracts a stored judge verdict's decision (spec 3 §1): Raw is the kind's decision field (match / is_real /
// still_present), nil when the verdict is null, the field is absent or null, or the kind is unknown; Effective is the
// kind's per-call rule — is_real ∧ confidence ≥ AdjudicateThreshold for adjudicate, still_present for still-present,
// and Raw for match (the match rule is a relative argmax across candidates, so a single verdict has no other rule).
func Decision(kind string, verdict json.RawMessage) machine.Decision {
	var v struct {
		Match        *bool    `json:"match"`
		IsReal       *bool    `json:"is_real"`
		StillPresent *bool    `json:"still_present"`
		Confidence   *float64 `json:"confidence"`
	}
	if len(verdict) == 0 || json.Unmarshal(verdict, &v) != nil {
		return machine.Decision{}
	}
	var raw *bool
	switch kind {
	case judge.KindMatch:
		raw = v.Match
	case judge.KindAdjudicate:
		raw = v.IsReal
	case judge.KindStillPresent:
		raw = v.StillPresent
	}
	if raw == nil {
		return machine.Decision{}
	}
	r := *raw
	eff := r
	if kind == judge.KindAdjudicate {
		conf := 0.0
		if v.Confidence != nil {
			conf = *v.Confidence
		}
		eff = r && conf >= AdjudicateThreshold
	}
	return machine.Decision{Raw: &r, Effective: &eff}
}
