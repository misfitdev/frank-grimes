package engine

import (
	"sort"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// assemble builds the result from derived values only. Nothing a provider sent
// reaches any field here.
func (e *Engine) assemble(
	target *pb.Target,
	mode pb.Mode,
	iteration uint32,
	d Derived,
	review *pb.IndependentReview,
	verification *pb.Verification,
	ledger *pb.Ledger,
	digest []byte,
	oscillation bool,
	yield *pb.MarginalYield,
	check pb.RefuterCheck,
	checkReason string,
) *pb.GrimesResult {
	// The same stopping rule the stop hook applies, so the record cannot claim a
	// review ended while the loop is still owed an iteration.
	outcome := stopRule(Progress{
		Green:         d.Color == pb.LegacyColor_LEGACY_COLOR_GREEN,
		Iteration:     iteration,
		MaxIterations: e.MaxIterations,
		NewP0P1:       yield.GetNewP0P1(),
		Exhausted:     Exhausted(d.UnmetGates),
	})
	completion := outcome.CompletionState()
	if mode == pb.Mode_MODE_FIX {
		completion = fixCompletion(outcome, verification)
	}
	return &pb.GrimesResult{
		SchemaMajor:        contracts.SchemaMajor,
		RunId:              e.RunID,
		ProducerRole:       pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:             target,
		Mode:               mode,
		Iteration:          iteration,
		MaxIterations:      e.MaxIterations,
		CompletionState:    completion,
		Verdict:            d.Verdict,
		LegacyColor:        d.Color,
		RefuterCheck:       check,
		RefuterCheckReason: checkReason,
		Confinement:        e.Confinement,
		MarginalYield:      yield,
		Counts:             d.Counts,
		Findings:           snapshots(ledger),
		Verification:       verification,
		IndependentReview:  review,
		Ledger: &pb.LedgerRef{
			Path:                contracts.LedgerPath,
			DigestSha256:        digest,
			OscillationDetected: oscillation,
		},
		UnmetGates: d.UnmetGates,
		Summary:    summarize(d),
	}
}

// snapshots renders the ledger in a stable order so two runs over the same
// ledger produce identical result bytes.
func snapshots(ledger *pb.Ledger) []*pb.FindingSnapshot {
	out := make([]*pb.FindingSnapshot, 0, len(ledger.GetFindings()))
	for _, f := range ledger.GetFindings() {
		provenance := provenanceOf(f)
		survived := survivedRefutations(f)
		out = append(out, &pb.FindingSnapshot{
			Id:             f.GetId(),
			Status:         f.GetStatus(),
			Risk:           f.GetRisk(),
			EvidenceTier:   f.GetEvidence().GetTier(),
			EvidenceSha256: f.GetEvidenceSha256(),
			Provenance:     provenance,
			// Both positions, not just the label saying they differ.
			Refutation:          f.GetRefutation(),
			SurvivedRefutations: survived,
			Rank:                Rank(f.GetRisk(), survived, provenance),
		})
	}
	// Worst first, so the order is the order to work in. Ties fall back to the
	// id, which is content-derived, so two runs over the same ledger write the
	// same bytes.
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetRank() != out[j].GetRank() {
			return out[i].GetRank() > out[j].GetRank()
		}
		return out[i].GetId() < out[j].GetId()
	})
	return out
}

func summarize(d Derived) string {
	switch d.Color {
	case pb.LegacyColor_LEGACY_COLOR_RED:
		return "Open P0 findings block this target."
	case pb.LegacyColor_LEGACY_COLOR_GREEN:
		return "No verdict-weighted findings remain and an independent review agreed."
	default:
		return "Unmet gates remain; see unmet_gates."
	}
}
