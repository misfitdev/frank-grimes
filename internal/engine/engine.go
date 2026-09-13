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
	pb.Category_CATEGORY_HUM,
}

// StrictBroker admits a finding only when the contract accepts it whole.
//
// It cannot yet establish that a probe was attempted, so confidence stays below
// high until evidence falsification is machine-verifiable.
type StrictBroker struct{}

func (StrictBroker) Admit(ctx context.Context, c *pb.CandidateFinding) (*pb.CandidateFinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := contracts.Validate(c); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
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
}

func (a ProviderAdjudicator) Adjudicate(ctx context.Context, target *pb.Target, contentPath string, claimed *pb.Verdict) (*pb.IndependentReview, error) {
	if a.Provider == nil {
		return nil, fmt.Errorf("no adjudicator configured")
	}
	out, err := a.Provider.Review(ctx, Request{
		Role:        RoleAdjudicator,
		Target:      target,
		ContentPath: contentPath,
		Claimed:     claimed,
	})
	if err != nil {
		return nil, err
	}
	raw, err := envelope.ExtractReport(string(out.Raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	second := &pb.AdjudicationReport{}
	if err := contracts.UnmarshalCanonical(raw, second); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	// The adjudicator names the target it judged; a tuple over a different
	// artifact is not a second opinion on this one.
	if !bytes.Equal(second.GetTargetFingerprintSha256(), target.GetFingerprintSha256()) {
		return nil, fmt.Errorf("%w: adjudication names a different target", ErrProviderOutput)
	}
	return &pb.IndependentReview{
		RunId:                   a.RunID,
		ReviewerId:              a.ReviewerID,
		ZeroKnowledge:           true,
		TargetFingerprintSha256: target.GetFingerprintSha256(),
		Verdict:                 second.GetVerdict(),
		CompletedAt:             a.Clock.stamp(),
	}, nil
}
