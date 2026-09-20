package engine

import pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"

// Rank orders findings the way a person deciding what to fix first would:
// probability times impact, with ease of fix breaking ties among equals.
//
// The two axes are the ones the contract already carries. Likelihood is the
// probability a defect is real and reachable; blast radius is what it costs
// when it is. Neither alone orders work: a systemic defect nobody can reach
// and a single-user defect anybody can are both mid-rank, and a matrix says so
// where a single severity letter cannot.
//
// Ease of fix is a fraction below one, so it can separate two findings that
// score the same and can never lift one above a finding with a worse product.
//
// Survived refutation raises probability. A claim three independent contexts
// tried and failed to break is likelier to be real than one nobody attacked,
// and it is the only evidence here that gets stronger with effort -- everything
// else is self-reported at the moment of reporting.
func Rank(r *pb.Risk, survived uint32, provenance pb.FindingProvenance) float64 {
	p := probability(r.GetLikelihood(), survived, provenance)
	i := impact(r.GetBlastRadius())
	if p == 0 || i == 0 {
		// Unknown on either axis is not a low score, it is no score. Something
		// is there and nobody could say how bad; ranking it anyway would read
		// as having measured it.
		return 0
	}
	return float64(p*i) + ease(r.GetEaseOfFix())
}

// probability is the likelihood axis, raised by each independent context that
// attacked the claim and failed.
func probability(l pb.Likelihood, survived uint32, provenance pb.FindingProvenance) int {
	base := 0
	switch l {
	case pb.Likelihood_LIKELIHOOD_LIKELY:
		base = 3
	case pb.Likelihood_LIKELIHOOD_PLAUSIBLE:
		base = 2
	case pb.Likelihood_LIKELIHOOD_UNLIKELY:
		base = 1
	default:
		return 0
	}
	// A claim two contexts disagree about has not been attested by either, so
	// the attempts that upheld it do not raise it.
	if provenance == pb.FindingProvenance_FINDING_PROVENANCE_CONTESTED {
		return base
	}
	const cap = 5
	if raised := base + int(min(survived, cap)); raised < cap {
		return raised
	}
	return cap
}

// impact is the blast radius axis.
func impact(b pb.BlastRadius) int {
	switch b {
	case pb.BlastRadius_BLAST_RADIUS_SYSTEMIC:
		return 4
	case pb.BlastRadius_BLAST_RADIUS_SERVICE:
		return 3
	case pb.BlastRadius_BLAST_RADIUS_LOCAL_COMPONENT:
		return 2
	case pb.BlastRadius_BLAST_RADIUS_SINGLE_USER:
		return 1
	}
	return 0
}

// ease is the tie-break, in [0, 1). Unstated sits between the two, so
// declining to estimate neither helps nor hurts.
func ease(e pb.EaseOfFix) float64 {
	switch e {
	case pb.EaseOfFix_EASE_OF_FIX_TRIVIAL:
		return 0.75
	case pb.EaseOfFix_EASE_OF_FIX_MODERATE:
		return 0.5
	case pb.EaseOfFix_EASE_OF_FIX_INVOLVED:
		return 0.25
	}
	return 0.5
}

// survivedRefutations counts the independent contexts that attacked a claim and
// could not break it.
func survivedRefutations(f *pb.Finding) uint32 {
	var n uint32
	for _, a := range f.GetRefutation() {
		if a.GetContextOrigin() != pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED {
			continue
		}
		if _, ok := a.GetOutcome().(*pb.RefutationAttempt_Upheld); ok {
			n++
		}
	}
	return n
}
