package engine

import (
	"github.com/misfitdev/frank-grimes/internal/adjudicate"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

func resolveWith(independent pb.Decision) pb.Decision {
	return adjudicate.Resolve(pb.Decision_DECISION_PASS, independent)
}

// count tallies only weighted findings.
//
// The contract's open_p0_blocks rule reads counts.open_p0 and requires a block
// whenever it is non-zero, so an unweighted open P0 reported here would demand
// a decision that decide() will not produce and the result would fail
// validation.
func count(cands []Candidate) *pb.FindingCounts {
	c := &pb.FindingCounts{}
	for _, cand := range cands {
		c.Total++
		if cand.Status == pb.FindingStatus_FINDING_STATUS_VERIFIED {
			c.Verified++
		}
		if cand.Status == pb.FindingStatus_FINDING_STATUS_ACCEPTED {
			c.Accepted++
		}
		if !Weighted(cand.Tags) || !Open(cand.Status) {
			continue
		}
		switch cand.Severity {
		case pb.Severity_SEVERITY_P0:
			c.OpenP0++
		case pb.Severity_SEVERITY_P1:
			c.OpenP1++
		}
	}
	return c
}

// carriesRisk reports whether a finding still leaves risk on the target.
//
// Accepting a risk is a decision to carry it, not evidence it is gone, so an
// accepted finding ranks in residual risk even though it is not open. Counting
// it as open instead would demand a block the decision rules do not produce.
func carriesRisk(s pb.FindingStatus) bool {
	return Open(s) || s == pb.FindingStatus_FINDING_STATUS_ACCEPTED
}

func decide(in DeriveInput, counts *pb.FindingCounts) pb.Decision {
	if counts.GetOpenP0() > 0 {
		return pb.Decision_DECISION_BLOCK
	}
	if counts.GetOpenP1() > 0 {
		return pb.Decision_DECISION_CONDITIONAL
	}
	if !in.AdjudicationAvailable {
		return pb.Decision_DECISION_CONDITIONAL
	}
	// Accepting a P0 is a decision to carry the risk, not evidence it is gone.
	if carriesAcceptedP0(in.Candidates) {
		return pb.Decision_DECISION_CONDITIONAL
	}
	return pb.Decision_DECISION_PASS
}

// carriesAcceptedP0 reports whether a weighted P0 is being carried rather than
// fixed. Weighting is checked here as it is in count, residualRisk, and
// drivingFindings: a finding the other three ignore must not be the one that
// downgrades the decision.
func carriesAcceptedP0(cands []Candidate) bool {
	for _, c := range cands {
		if !Weighted(c.Tags) {
			continue
		}
		if c.Severity == pb.Severity_SEVERITY_P0 && c.Status == pb.FindingStatus_FINDING_STATUS_ACCEPTED {
			return true
		}
	}
	return false
}

func residualRisk(in DeriveInput) pb.ResidualRisk {
	if in.RankingBlocked {
		return pb.ResidualRisk_RESIDUAL_RISK_UNKNOWN
	}
	highest := pb.Severity_SEVERITY_UNSPECIFIED
	for _, c := range in.Candidates {
		if !Weighted(c.Tags) || !carriesRisk(c.Status) {
			continue
		}
		if highest == pb.Severity_SEVERITY_UNSPECIFIED || c.Severity < highest {
			highest = c.Severity
		}
	}
	switch highest {
	case pb.Severity_SEVERITY_P0:
		return pb.ResidualRisk_RESIDUAL_RISK_CRITICAL
	case pb.Severity_SEVERITY_P1:
		return pb.ResidualRisk_RESIDUAL_RISK_HIGH
	case pb.Severity_SEVERITY_P2:
		return pb.ResidualRisk_RESIDUAL_RISK_MODERATE
	default:
		return pb.ResidualRisk_RESIDUAL_RISK_LOW
	}
}

func confidence(in DeriveInput) pb.ReviewConfidence {
	if in.CriticalFalsifierUnavailable || !in.AdjudicationAvailable {
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW
	}
	driving := drivingFindings(in.Candidates)
	for _, c := range driving {
		if c.EvidenceConflict {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW
		}
	}
	for _, c := range driving {
		if !c.ProbeAttempted {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
		}
		if c.Tier == pb.EvidenceTier_EVIDENCE_TIER_E3 {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
		}
	}
	return pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH
}

func drivingFindings(cands []Candidate) []Candidate {
	var out []Candidate
	for _, c := range cands {
		if Weighted(c.Tags) && Open(c.Status) {
			out = append(out, c)
		}
	}
	return out
}

func completeness(in DeriveInput) pb.ReviewCompleteness {
	if !in.CriticalInvariantProbed {
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_INCONCLUSIVE
	}
	if in.AllCategoriesStopped && !in.CriticalUnknownRemains {
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT
	}
	return pb.ReviewCompleteness_REVIEW_COMPLETENESS_LIMITED
}

func colorOf(v *pb.Verdict, oscillation bool) pb.LegacyColor {
	if v.GetDecision() == pb.Decision_DECISION_BLOCK {
		return pb.LegacyColor_LEGACY_COLOR_RED
	}
	if !oscillation &&
		v.GetDecision() == pb.Decision_DECISION_PASS &&
		v.GetResidualRisk() == pb.ResidualRisk_RESIDUAL_RISK_LOW &&
		v.GetReviewConfidence() == pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH &&
		v.GetReviewCompleteness() == pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT {
		return pb.LegacyColor_LEGACY_COLOR_GREEN
	}
	return pb.LegacyColor_LEGACY_COLOR_YELLOW
}
