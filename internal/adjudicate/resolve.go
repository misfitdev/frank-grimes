// Package adjudicate folds an independent verdict into a primary one.
package adjudicate

import pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"

// Resolve returns the decision that survives comparing the two reviews.
//
// Adjudication can only remove confidence, never manufacture it: an
// independent pass leaves a primary block or conditional exactly where it was,
// and a primary pass falls to whatever the independent review decided.
func Resolve(primary, independent pb.Decision) pb.Decision {
	if primary != pb.Decision_DECISION_PASS {
		return primary
	}
	switch independent {
	case pb.Decision_DECISION_PASS, pb.Decision_DECISION_CONDITIONAL, pb.Decision_DECISION_BLOCK:
		return independent
	default:
		return pb.Decision_DECISION_CONDITIONAL
	}
}
