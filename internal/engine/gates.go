package engine

import pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"

// Gate names reported when a run falls short of a pass.
const (
	GateDecision     = "decision"
	GateResidualRisk = "residual_risk"
	GateConfidence   = "review_confidence"
	GateCompleteness = "review_completeness"
	GateAdjudication = "adjudication"
	GateOscillation  = "oscillation"
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
	if !in.AdjudicationAvailable {
		gates = append(gates, GateAdjudication)
	}
	if in.Oscillation {
		gates = append(gates, GateOscillation)
	}
	return gates
}
