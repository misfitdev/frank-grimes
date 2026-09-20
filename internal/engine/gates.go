package engine

import pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"

// Gate names reported when a run falls short of a pass.
const (
	GateDecision           = "decision"
	GateResidualRisk       = "residual_risk"
	GateConfidence         = "review_confidence"
	GateCompleteness       = "review_completeness"
	GateAdjudication       = "adjudication"
	GateIndependentContext = "independent_context"
	GateRefutation         = "refutation"
	GateOscillation        = "oscillation"
	GateConfinement        = "confinement"
	GateCoverage           = "coverage"
	GateVerificationScope  = "verification_scope"
	GateContested          = "contested"
	GateCoordinator        = "coordinator_separation"
)

// unmetGates names every gate standing between this run and a pass, in a fixed
// order so two runs of the same shape produce the same result bytes.
func unmetGates(v *pb.Verdict, in DeriveInput) []string {
	var gates []string
	if v.GetDecision() != pb.Decision_DECISION_PASS {
		gates = append(gates, GateDecision)
	}
	if v.GetResidualRisk() != pb.ResidualRisk_RESIDUAL_RISK_LOW {
		gates = append(gates, GateResidualRisk)
	}
	if v.GetReviewConfidence() != pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH {
		gates = append(gates, GateConfidence)
	}
	if v.GetReviewCompleteness() != pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT {
		gates = append(gates, GateCompleteness)
	}
	if !in.AdjudicationAvailable || in.AdjudicationCompleted < in.AdjudicationRequested {
		gates = append(gates, GateAdjudication)
	}
	// Named separately from confidence so the record says why it was capped.
	if in.IndependentContextUnknown {
		gates = append(gates, GateIndependentContext)
	}
	// Likewise: a finding nobody attacked and a finding somebody broke both cap
	// confidence, and the record should not read as though the review simply
	// lacked evidence.
	if unrefuted(in.Candidates) {
		gates = append(gates, GateRefutation)
	}
	// Named separately from refutation: a claim nobody attacked and a claim two
	// contexts answered differently are different states of knowledge, and the
	// record should not read as though the second were the first.
	if contested(in.Candidates) {
		gates = append(gates, GateContested)
	}
	// A unit nobody named and a category that stopped attacking before it ran
	// out of probes are the same shortfall measured two ways, so they answer to
	// one gate.
	// A gate that skipped a check has not verified the batch, whatever it
	// returned for the rest. Named rather than folded into coverage, which is
	// about the units a review accounted for.
	if len(in.VerificationExcluded) > 0 {
		gates = append(gates, GateVerificationScope)
	}
	if in.CoverageIncomplete || !in.AllCategoriesStopped {
		gates = append(gates, GateCoverage)
	}
	if in.Oscillation {
		gates = append(gates, GateOscillation)
	}
	// A role that could have rewritten the target it was reviewing, or read the
	// ledger it was meant to be blind to, was not held to the boundary the rest
	// of these gates assume.
	if in.Unconfined {
		gates = append(gates, GateConfinement)
	}
	// A coordinator holds context across the whole review. One that also
	// authors findings has rebuilt self-ratification above every fresh context
	// beneath it.
	if in.CoordinatorAuthored {
		gates = append(gates, GateCoordinator)
	}
	return gates
}
