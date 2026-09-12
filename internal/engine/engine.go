package engine

import (
	"context"
	"crypto/sha256"
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

// PathCollector fingerprints a target by its root and scope.
//
// It does not enumerate or hash the target's contents, so it cannot report that
// a routed category reached its stop; completeness therefore stays below
// sufficient until real collection lands.
type PathCollector struct{}

func (PathCollector) Collect(ctx context.Context, spec TargetSpec) (*pb.Target, []pb.Category, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if spec.Root == "" || spec.Scope == "" {
		return nil, nil, fmt.Errorf("target needs both a root and a scope")
	}
	sum := sha256.Sum256([]byte(spec.Root + "\x00" + spec.Scope))
	target := &pb.Target{
		Root:              spec.Root,
		Scope:             spec.Scope,
		FingerprintSha256: sum[:],
		Display:           spec.Scope,
	}
	categories := spec.Categories
	if len(categories) == 0 {
		categories = AllCategories
	}
	return target, categories, nil
}

// StrictBroker admits a finding only when the contract accepts it whole.
//
// It cannot yet establish that a probe was attempted, so confidence stays below
// high until evidence falsification is machine-verifiable.
type StrictBroker struct{}

func (StrictBroker) Admit(ctx context.Context, f *pb.Finding) (*pb.Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := contracts.Validate(f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	return f, nil
}

// NotApplicableGate is the gate for report mode, which edits nothing.
type NotApplicableGate struct{}

func (NotApplicableGate) Run(ctx context.Context, _ string) (*pb.Verification, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pb.Verification{Status: pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE}, nil
}

// ProviderAdjudicator obtains a second verdict from a separate provider
// context that receives only the target's identity and the claimed tuple.
type ProviderAdjudicator struct {
	Provider   Provider
	ReviewerID string
	RunID      string
	Clock      Clock
}

func (a ProviderAdjudicator) Adjudicate(ctx context.Context, target *pb.Target, claimed *pb.Verdict) (*pb.IndependentReview, error) {
	if a.Provider == nil {
		return nil, fmt.Errorf("no adjudicator configured")
	}
	out, err := a.Provider.Review(ctx, Request{
		Role:    RoleAdjudicator,
		Target:  target,
		Claimed: claimed,
	})
	if err != nil {
		return nil, err
	}
	raw, err := envelope.Extract(string(out.Raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	second := &pb.GrimesResult{}
	if err := contracts.UnmarshalCanonical(raw, second); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
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
