// Package adjudicate folds an independent verdict into a primary one.
package adjudicate

import pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"

// Resolve returns the decision that survives comparing the two reviews.
//
// Adjudication can only remove confidence, never manufacture it: an
// independent pass leaves a primary block or conditional exactly where it was,
// and a primary pass falls to whatever the independent review decided.
//
// A block is the exception in the other direction. A reviewer that blocks has
// found something, and what it found does not stop counting because the primary
// was already withholding for a reason of its own — a run that asked for a
// second opinion and got a block must not report less than a block.
func Resolve(primary, independent pb.Decision) pb.Decision {
	if independent == pb.Decision_DECISION_BLOCK {
		return pb.Decision_DECISION_BLOCK
	}
	if primary != pb.Decision_DECISION_PASS {
		return primary
	}
	switch independent {
	case pb.Decision_DECISION_PASS, pb.Decision_DECISION_CONDITIONAL:
		return independent
	default:
		return pb.Decision_DECISION_CONDITIONAL
	}
}

// Strictest returns the decision a panel resolves to.
//
// One objection is an objection whoever raised it, so the panel speaks with the
// least permissive voice in it. An empty panel decided nothing and says so,
// which is not the same as agreeing.
func Strictest(decisions ...pb.Decision) pb.Decision {
	worst := pb.Decision_DECISION_UNSPECIFIED
	for _, d := range decisions {
		if rank(d) > rank(worst) {
			worst = d
		}
	}
	return worst
}

// rank orders decisions by how much they withhold. An unrecognised decision
// ranks between conditional and block: it is not a pass, and reading it as one
// would let a reviewer approve by saying something the engine cannot read.
func rank(d pb.Decision) int {
	switch d {
	case pb.Decision_DECISION_PASS:
		return 1
	case pb.Decision_DECISION_INFORMATIONAL:
		return 2
	case pb.Decision_DECISION_CONDITIONAL:
		return 3
	case pb.Decision_DECISION_BLOCK:
		return 5
	case pb.Decision_DECISION_UNSPECIFIED:
		return 0
	default:
		return 4
	}
}
