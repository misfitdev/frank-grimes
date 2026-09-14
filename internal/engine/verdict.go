package engine

import (
	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// Tags a provider may attach to a finding that remove it from verdict weight.
const (
	TagAssumptionDependent = "assumption-dependent"
	TagUnverified          = "unverified"
)

// Weighted reports whether a finding counts toward the verdict. A candidate
// resting on an unconfirmed assumption, or whose falsifier was never attempted,
// is carried in the report but cannot drive the decision.
func Weighted(tags []string) bool {
	for _, t := range tags {
		if t == TagAssumptionDependent || t == TagUnverified {
			return false
		}
	}
	return true
}

// Open reports whether a finding's status leaves risk outstanding.
func Open(s pb.FindingStatus) bool {
	switch s {
	case pb.FindingStatus_FINDING_STATUS_OPEN, pb.FindingStatus_FINDING_STATUS_REGRESSED:
		return true
	default:
		return false
	}
}

// Candidate is one finding as the verdict sees it.
type Candidate struct {
	Severity         pb.Severity
	Status           pb.FindingStatus
	Tier             pb.EvidenceTier
	Tags             []string
	ProbeAttempted   bool
	EvidenceConflict bool
}

// DeriveInput is every fact the verdict depends on.
type DeriveInput struct {
	Candidates []Candidate

	// Adjudication
	AdjudicationAvailable bool
	IndependentDecision   pb.Decision
	// IndependentContextUnknown is set when a second opinion arrived from a
	// context whose origin could not be established. It caps confidence and
	// leaves the decision alone: such a reviewer may make a verdict worse,
	// never better.
	IndependentContextUnknown bool

	// Coverage, measured against the inventory the engine resolved. Routed
	// categories that did not reach a stop cap completeness.
	AllCategoriesStopped    bool
	CriticalInvariantProbed bool
	CriticalUnknownRemains  bool
	// CoverageIncomplete is set when a unit of the target was neither examined
	// nor explicitly skipped. A review that did not look at part of what it was
	// given cannot pass on the part it did look at.
	CoverageIncomplete bool

	// TargetKind records what was reviewed. It selects nothing in the verdict
	// rules; it is carried so the result can say what it judged.
	TargetKind pb.TargetKind

	// Run facts
	Oscillation                  bool
	RankingBlocked               bool
	CriticalFalsifierUnavailable bool
}

// Derived is everything the engine computes rather than accepts.
type Derived struct {
	Verdict    *pb.Verdict
	Color      pb.LegacyColor
	Counts     *pb.FindingCounts
	UnmetGates []string
}

// Derive maps findings and run facts onto a verdict tuple and its colour.
//
// This is the only place in the repository that decides a verdict or a colour.
// scripts/validate.sh enforces that.
func Derive(in DeriveInput) Derived {
	counts := count(in.Candidates)

	decision := decide(in, counts)
	if in.AdjudicationAvailable && decision == pb.Decision_DECISION_PASS {
		decision = resolveWith(in.IndependentDecision)
	}

	v := &pb.Verdict{
		Decision:           decision,
		ResidualRisk:       residualRisk(in),
		ReviewConfidence:   confidence(in),
		ReviewCompleteness: completeness(in),
	}
	color := colorOf(v, in.Oscillation)
	return Derived{Verdict: v, Color: color, Counts: counts, UnmetGates: unmetGates(v, in)}
}
