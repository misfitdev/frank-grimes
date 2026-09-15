package engine

import (
	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// weigh turns a ledger finding into the candidate the verdict reasons about.
//
// Everything here is derived from the finding's own record. A provider states
// none of it: a tag removes a finding from verdict weight, and a provider that
// could write one could defuse its own accusation by relabelling it, which is a
// severity change under another name. Severity itself only ratchets for the
// same reason. Provenance is the exception in the other direction — the engine
// writes it from what a separate context did to the claim.
func weigh(f *pb.Finding) Candidate {
	d := f.GetEvidence().GetDisproof()
	return Candidate{
		Severity:         f.GetRisk().GetSeverity(),
		Status:           f.GetStatus(),
		Tier:             f.GetEvidence().GetTier(),
		Tags:             tagsFor(f),
		ProbeAttempted:   attempted(d),
		EvidenceConflict: d.GetContradictsClaim(),
		Provenance:       provenanceOf(f),
	}
}

// attempted reports whether the disproof was actually carried out. A reason why
// it could not be is a record of something that did not happen.
func attempted(d *pb.DisproofAttempt) bool {
	_, ok := d.GetOutcome().(*pb.DisproofAttempt_Performed)
	return ok
}

// unavailable reports whether the disproof was possible but could not be made.
func unavailable(d *pb.DisproofAttempt) bool {
	_, ok := d.GetOutcome().(*pb.DisproofAttempt_Unavailable)
	return ok
}

// tagsFor names why a finding cannot carry verdict weight.
//
// Phase 6 sets both: a probe that could not be attempted marks the candidate
// unverified, and a candidate still resting on an unconfirmed assumption is
// assumption-dependent. An assumption is unconfirmed exactly when the
// observation that would have settled it was never made.
func tagsFor(f *pb.Finding) []string {
	var tags []string
	d := f.GetEvidence().GetDisproof()
	if attempted(d) {
		return nil
	}
	if f.GetEvidence().GetTier() == pb.EvidenceTier_EVIDENCE_TIER_E3 {
		tags = append(tags, TagAssumptionDependent)
	}
	return append(tags, TagUnverified)
}

// rankingBlocked reports whether the review holds an open finding it could not
// size.
//
// Such a finding is excluded from verdict weight, so the ones that remain rank
// cleanly and the rank would read as the whole picture. It is not: something is
// there and nobody could say how bad. Declining to rank is not the same as
// manufacturing risk weight, which is what the skill excludes it from.
func rankingBlocked(ledger *pb.Ledger) bool {
	for _, f := range ledger.GetFindings() {
		if Open(f.GetStatus()) && !Weighted(tagsFor(f)) {
			return true
		}
	}
	return false
}

// criticalFalsifierUnavailable reports whether a finding that drives the
// verdict rests on a disproof that could not be attempted.
func criticalFalsifierUnavailable(ledger *pb.Ledger) bool {
	for _, f := range ledger.GetFindings() {
		if !Open(f.GetStatus()) {
			continue
		}
		switch f.GetRisk().GetSeverity() {
		case pb.Severity_SEVERITY_P0, pb.Severity_SEVERITY_P1:
			if unavailable(f.GetEvidence().GetDisproof()) {
				return true
			}
		}
	}
	return false
}
