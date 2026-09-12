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
) *pb.GrimesResult {
	return &pb.GrimesResult{
		SchemaMajor:       contracts.SchemaMajor,
		RunId:             e.RunID,
		ProducerRole:      pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:            target,
		Mode:              mode,
		Iteration:         iteration,
		MaxIterations:     e.MaxIterations,
		CompletionState:   pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE,
		Verdict:           d.Verdict,
		LegacyColor:       d.Color,
		MarginalYield:     &pb.MarginalYield{},
		Counts:            d.Counts,
		Findings:          snapshots(ledger),
		Verification:      verification,
		IndependentReview: review,
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
		out = append(out, &pb.FindingSnapshot{
			Id:             f.GetId(),
			Status:         f.GetStatus(),
			Risk:           f.GetRisk(),
			EvidenceTier:   f.GetEvidence().GetTier(),
			EvidenceSha256: f.GetEvidenceSha256(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetId() < out[j].GetId() })
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
