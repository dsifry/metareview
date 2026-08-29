package run

import "fmt"

// FoldError codes (§2.4).
const (
	CodeAuditEmpty   = "ERR_AUDIT_EMPTY"
	CodeAuditVersion = "ERR_AUDIT_VERSION"
	CodeAuditInvalid = "ERR_AUDIT_INVALID"
)

// FoldError reasons (§2.4). Every reason maps to exactly one code via CodeFor.
const (
	ReasonEmpty              = "empty"
	ReasonVersion            = "version"
	ReasonFirstNotInit       = "first_not_init"
	ReasonSecondInit         = "second_init"
	ReasonSeqGap             = "seq_gap"
	ReasonUnknownType        = "unknown_type"
	ReasonBadPayload         = "bad_payload"
	ReasonOversize           = "oversize"
	ReasonOutputAfterDelta   = "output_after_delta"
	ReasonDeltaWithoutOutput = "delta_without_output"
	ReasonSecondDelta        = "second_delta"
	ReasonOutputHash         = "output_hash"
	ReasonStatusNotSubset    = "status_not_subset"
	ReasonStatusIncomplete   = "status_incomplete"
	ReasonStatusDuplicate    = "status_duplicate"
	ReasonPostTerminal       = "post_terminal"
	ReasonProvenance         = "provenance"
	ReasonStamp              = "stamp"
	ReasonInitStamp          = "init_stamp"
	ReasonMockStamp          = "mock_stamp"
	ReasonNodeScope          = "node_scope"
	ReasonFixBaselineHead    = "fix_baseline_head"
	ReasonFixBaselineKind    = "fix_baseline_kind"
	ReasonFixBaselineOrder   = "fix_baseline_order"
	ReasonUnsanctionedCmd    = "unsanctioned_cmd"
	ReasonBadOutcome         = "bad_outcome"
	ReasonTokensNegative     = "tokens_negative"
	ReasonTokensTooLarge     = "tokens_too_large"
)

// CodeFor returns the FoldError code for a reason (§2.4 table).
func CodeFor(reason string) string {
	switch reason {
	case ReasonEmpty:
		return CodeAuditEmpty
	case ReasonVersion:
		return CodeAuditVersion
	default:
		return CodeAuditInvalid
	}
}

// StoreError codes (§2.4).
const (
	CodeStorePath        = "ERR_STORE_PATH"
	CodeRunExists        = "ERR_RUN_EXISTS"
	CodeRunNotFound      = "ERR_RUN_NOT_FOUND"
	CodeRunLocked        = "ERR_RUN_LOCKED"
	CodeAuditChain       = "ERR_AUDIT_CHAIN"
	CodeAuditCAS         = "ERR_AUDIT_CAS"
	CodeAuditTorn        = "ERR_AUDIT_TORN"
	CodeAuditTailChanged = "ERR_AUDIT_TAIL_CHANGED"
	CodeAuditNotTorn     = "ERR_AUDIT_NOT_TORN"
	CodeEventTooLarge    = "ERR_EVENT_TOO_LARGE"
	CodeAuditFull        = "ERR_AUDIT_FULL"
	CodeAppendRejected   = "ERR_APPEND_REJECTED"
)

// FoldError is the typed, fail-closed error returned by Apply/Fold/FoldFull.
type FoldError struct {
	Code   string
	Reason string
	Seq    int64
	Type   string
}

func (e *FoldError) Error() string {
	return fmt.Sprintf("%s (%s) at seq %d type %s", e.Code, e.Reason, e.Seq, e.Type)
}

func foldErr(reason string, ev Event) *FoldError {
	return &FoldError{Code: CodeFor(reason), Reason: reason, Seq: ev.Seq, Type: ev.Type}
}

// StoreError is the typed error returned by RunStore methods.
type StoreError struct {
	Code   string
	Seq    int64
	Detail string
	Cause  error
}

func (e *StoreError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

// Unwrap exposes the cause (a *FoldError for ERR_APPEND_REJECTED).
func (e *StoreError) Unwrap() error { return e.Cause }
