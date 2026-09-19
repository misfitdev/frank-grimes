package engine

import "errors"

// actorName is who the ledger records for engine-made transitions.
const actorName = "grimes"

// Every path that cannot prove a result is sound returns one of these. A run
// that cannot be completed is never downgraded to a lenient verdict.
var (
	ErrProviderFailed  = errors.New("provider failed")
	ErrProviderOutput  = errors.New("provider output rejected")
	ErrForgedFindingID = errors.New("finding id does not match its own fingerprint")
	ErrStaleState      = errors.New("loop state belongs to a different target")
	ErrLedgerTarget    = errors.New("ledger belongs to a different target")
	ErrTargetChanged   = errors.New("the target changed during the review")
)
