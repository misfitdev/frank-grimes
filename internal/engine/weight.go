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
//
// A fix that no gate has passed over is the same kind of claim: someone edited
// something and said it was enough. Only verified leaves nothing behind.
func carriesRisk(s pb.FindingStatus) bool {
	return Open(s) ||
		s == pb.FindingStatus_FINDING_STATUS_ACCEPTED ||
		s == pb.FindingStatus_FINDING_STATUS_FIXED
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
	// A verdict over part of a target is not a verdict over the target.
	if criticalUnknown(in) {
		return pb.Decision_DECISION_CONDITIONAL
	}
	// A severe finding nobody tested carries no verdict weight, so it cannot
	// block. It must not buy a pass either: skipping the disproof would then
	// earn a softer decision than performing one that fails.
	if carriesUntestedSevere(in.Candidates) {
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

// carriesUntestedSevere reports whether an open P0 or P1 was never put to a
// disproof, which is the one way a severe finding leaves verdict weight without
// anyone having shown it wrong.
func carriesUntestedSevere(cands []Candidate) bool {
	for _, c := range cands {
		if Weighted(c.Tags) || !Open(c.Status) {
			continue
		}
		switch c.Severity {
		case pb.Severity_SEVERITY_P0, pb.Severity_SEVERITY_P1:
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

// confidence reports how much the review's own conclusions were tested.
//
// Every other input here is something the reporting context said about its own
// finding: its tier, whether it probed, whether it saw a conflict. Provenance
// is the one input it could not write, so it is the one that decides whether
// confidence can reach the level a pass needs. A claim an independent context
// broke ranks with a material evidence conflict, because that is what it is:
// two contexts over one artifact reaching opposite results.
func confidence(in DeriveInput) pb.ReviewConfidence {
	if in.CriticalFalsifierUnavailable || !in.AdjudicationAvailable {
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW
	}
	driving := drivingFindings(in.Candidates)
	for _, c := range driving {
		if c.EvidenceConflict || c.Provenance == pb.FindingProvenance_FINDING_PROVENANCE_REFUTED {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW
		}
	}
	// A second opinion from a context that may have seen the first is not a
	// second opinion. It still counts as adjudication, so it can still block;
	// it cannot raise confidence to the level a pass requires.
	if in.IndependentContextUnknown {
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
	}
	// Nothing stopped a role from editing the target between the pass that found
	// something and the pass that checked it, so no conclusion here rests on
	// bytes the engine can say were the ones reviewed.
	if in.Unconfined {
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
	}
	for _, c := range driving {
		if !c.ProbeAttempted {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
		}
		if c.Tier == pb.EvidenceTier_EVIDENCE_TIER_E3 {
			return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM
		}
		// Nobody but the context that raised this has looked at it.
		if c.Provenance != pb.FindingProvenance_FINDING_PROVENANCE_UPHELD {
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

// unrefuted reports whether a finding the verdict rests on has yet to survive
// an independent attack. A review holding no such finding has nothing left to
// attack, so it is not short of one.
func unrefuted(cands []Candidate) bool {
	for _, c := range drivingFindings(cands) {
		if c.Provenance != pb.FindingProvenance_FINDING_PROVENANCE_UPHELD {
			return true
		}
	}
	return false
}

// criticalUnknown reports whether part of the target may hold a defect nobody
// looked for: a unit left unaccounted for, a skip the provider called material,
// or a category that stopped for want of evidence.
//
// The decision and completeness read the same three together. Ranking the two
// confessed ones below the silent one would pay for staying quiet, and a review
// that never named a unit has not accounted for the target either way.
func criticalUnknown(in DeriveInput) bool {
	return in.CoverageIncomplete || in.CriticalUnknownRemains
}

func completeness(in DeriveInput) pb.ReviewCompleteness {
	if !in.CriticalInvariantProbed {
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_INCONCLUSIVE
	}
	if in.AllCategoriesStopped && !criticalUnknown(in) {
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
