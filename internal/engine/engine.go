package engine

import (
	"bytes"
	"context"
	"fmt"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
)

// AllCategories is the canonical routing set.
var AllCategories = []pb.Category{
	pb.Category_CATEGORY_COR, pb.Category_CATEGORY_INT, pb.Category_CATEGORY_SEC,
	pb.Category_CATEGORY_REL, pb.Category_CATEGORY_OPS, pb.Category_CATEGORY_PER,
	pb.Category_CATEGORY_VER, pb.Category_CATEGORY_MNT, pb.Category_CATEGORY_DEP,
	pb.Category_CATEGORY_HUM, pb.Category_CATEGORY_NEC,
}

// StrictBroker admits a finding only when the contract accepts it whole and its
// evidence resolves against the target under review.
//
// The contract sees the shape of a record; only the engine holds the artifact.
// A path that is not in the target, or a quote that is nowhere in the section
// it cites, has the right shape and establishes nothing.
type StrictBroker struct{}

func (StrictBroker) Admit(ctx context.Context, c *pb.CandidateFinding, against *Collected) (*pb.CandidateFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := contracts.Validate(c); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	if err := resolve(c, against); err != nil {
		return nil, err
	}
	return c, nil
}

// NotApplicableGate is the gate for report mode, which edits nothing.
type NotApplicableGate struct{}

func (NotApplicableGate) Run(ctx context.Context, _ string) (*pb.Verification, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pb.Verification{
		Status:     pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE,
		SelectedBy: pb.GateSelection_GATE_SELECTION_UNAVAILABLE,
	}, nil
}

// ProviderAdjudicator obtains a second verdict from a separate provider
// context that receives only the target's identity and the claimed tuple.
type ProviderAdjudicator struct {
	Provider   Provider
	ReviewerID string
	RunID      string
	Clock      Clock
	// Fresh is the operator asserting that Provider's command begins a new
	// context. The engine cannot see inside an opaque command, so this is the
	// one input it takes on trust — from the person running the review, not
	// from either model.
	Fresh bool
}

// contextOrigin records how this reviewer's context came to exist.
//
// ENGINE_SPAWNED needs all three: the operator said the command starts fresh,
// the engine started the process itself, and the request carried nothing from
// the review — for an adjudicator that is target identity and the content path,
// with the claimed verdict deliberately withheld.
//
// Not yet covered: the review directory is still reachable from where the
// provider runs, so a determined command could read the ledger off disk.
// Confining that is fg-64y.49, and fg-64y.40 for the staged target.
func (a ProviderAdjudicator) contextOrigin() pb.ContextOrigin {
	if a.Fresh {
		return pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED
	}
	return pb.ContextOrigin_CONTEXT_ORIGIN_UNKNOWN
}

func (a ProviderAdjudicator) Adjudicate(ctx context.Context, target *pb.Target, at Handoff, claimed *pb.Verdict) (*pb.IndependentReview, error) {
	if a.Provider == nil {
		return nil, fmt.Errorf("no adjudicator configured")
	}
	out, err := a.Provider.Review(ctx, Request{
		Role:        RoleAdjudicator,
		RunID:       a.RunID,
		Target:      target,
		ContentPath: at.ContentPath,
		Root:        at.Root,
		Claimed:     claimed,
	})
	if err != nil {
		return nil, err
	}
	body, ok := delivered(out)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderOutput, missingEnvelope(out))
	}
	raw, err := envelope.ExtractReport(string(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	second := &pb.AdjudicationReport{}
	if err := contracts.UnmarshalCanonical(raw, second); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	// The adjudicator names the run it answered and the target it judged. A
	// tuple over a different artifact is not a second opinion on this one, and
	// one reached for an earlier run is not a second opinion reached now:
	// adjudication is what GREEN turns on, and the engine stamps its own run
	// onto the review it returns, so an answer from elsewhere would leave
	// nothing in the record to show it had come from there.
	if second.GetRunId() != a.RunID {
		return nil, fmt.Errorf("%w: adjudication belongs to run %q, this is run %q",
			ErrProviderOutput, second.GetRunId(), a.RunID)
	}
	if !bytes.Equal(second.GetTargetFingerprintSha256(), target.GetFingerprintSha256()) {
		return nil, fmt.Errorf("%w: adjudication names a different target", ErrProviderOutput)
	}
	return &pb.IndependentReview{
		RunId:                   a.RunID,
		ReviewerId:              a.ReviewerID,
		ContextOrigin:           a.contextOrigin(),
		TargetFingerprintSha256: target.GetFingerprintSha256(),
		Verdict:                 second.GetVerdict(),
		CompletedAt:             a.Clock.stamp(),
	}, nil
}
