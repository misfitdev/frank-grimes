package engine

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
)

// ProviderRefuter puts claims to a separate provider context and records what
// that context managed to do to them.
type ProviderRefuter struct {
	Provider  Provider
	RefuterID string
	RunID     string
	Clock     Clock
	// Fresh is the operator asserting that Provider's command begins a new
	// context, exactly as it is for the adjudicator. An attack mounted from the
	// context that formed the claim is the claim agreeing with itself, and the
	// engine cannot see inside an opaque command.
	Fresh bool
}

func (r ProviderRefuter) contextOrigin() pb.ContextOrigin {
	if r.Fresh {
		return pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED
	}
	return pb.ContextOrigin_CONTEXT_ORIGIN_UNKNOWN
}

// Refute runs one pass and returns what it did, keyed by the ref the engine
// issued. Refs the engine did not issue are dropped: a refuter that answers a
// question nobody asked has answered nothing about this review.
func (r ProviderRefuter) Refute(ctx context.Context, target *pb.Target, reads Handoff, claimsPath string) (map[string]*pb.RefutationAttempt, error) {
	if r.Provider == nil {
		return nil, fmt.Errorf("no refuter configured")
	}
	out, err := r.Provider.Review(ctx, Request{
		Role:        RoleRefuter,
		RunID:       r.RunID,
		Target:      target,
		ContentPath: reads.ContentPath,
		Root:        reads.Root,
		ClaimsPath:  claimsPath,
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
	report := &pb.RefutationReport{}
	if err := contracts.UnmarshalCanonical(raw, report); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	if report.GetRunId() != r.RunID {
		return nil, fmt.Errorf("%w: refutation belongs to run %q, this is run %q",
			ErrProviderOutput, report.GetRunId(), r.RunID)
	}
	if !bytes.Equal(report.GetTargetFingerprintSha256(), target.GetFingerprintSha256()) {
		return nil, fmt.Errorf("%w: refutation names a different target", ErrProviderOutput)
	}

	origin := r.contextOrigin()
	at := r.Clock.stamp()
	attempts := map[string]*pb.RefutationAttempt{}
	for _, o := range report.GetOutcomes() {
		attempt := &pb.RefutationAttempt{
			RefuterId:     r.RefuterID,
			ContextOrigin: origin,
			CompletedAt:   at,
		}
		switch outcome := o.GetOutcome().(type) {
		case *pb.ClaimOutcome_Refuted:
			attempt.Outcome = &pb.RefutationAttempt_Refuted{Refuted: outcome.Refuted}
		case *pb.ClaimOutcome_Upheld:
			attempt.Outcome = &pb.RefutationAttempt_Upheld{Upheld: outcome.Upheld}
		case *pb.ClaimOutcome_Unavailable:
			attempt.Outcome = &pb.RefutationAttempt_Unavailable{Unavailable: outcome.Unavailable}
		default:
			continue
		}
		attempts[o.GetRef()] = attempt
	}
	return attempts, nil
}

// refute puts every finding that still drives the verdict to a context that did
// not form it, and records what came back on the finding itself.
//
// Findings that carry no verdict weight are left out. The pass costs a provider
// invocation per run, and an attack on a claim the verdict already ignores buys
// nothing the verdict can read.
//
// Nothing the refuter says is recorded until it has broken the control, so a
// pass that vouches for everything raises no finding above unattacked. The
// control itself never reaches the ledger: it is dropped with every other ref
// the engine did not issue against a real finding.
func (e *Engine) refute(ctx context.Context, ledger *pb.Ledger, spec TargetSpec, collected *Collected, iteration uint32) (pb.RefuterCheck, string, error) {
	none := pb.RefuterCheck_REFUTER_CHECK_UNSPECIFIED
	if e.Refuter == nil || e.Claims == nil {
		return none, "", nil
	}
	claims, refs := claimsOf(ledger, e.RunID, iteration)
	if len(claims) == 0 {
		return none, "", nil
	}
	target := collected.Target

	// Staged only once there is a pass to hand it to: a run with no refuter has
	// no reason to recollect the target or to leave a copy behind.
	reads, err := e.handoff(ctx, spec, collected, RoleRefuter)
	if err != nil {
		return none, "", err
	}

	ctrl, err := controlFor(e.Dir, reads.ContentPath, claims, e.RunID, iteration)
	if err != nil {
		// A pass the engine cannot grade is a pass whose word it has no reason
		// to take, so it is not run at all.
		return pb.RefuterCheck_REFUTER_CHECK_INCONCLUSIVE, "", nil
	}

	path, err := e.Claims.Save(ctx, &pb.RefutationTask{
		SchemaMajor: contracts.SchemaMajor,
		RunId:       e.RunID,
		Target:      target,
		Iteration:   iteration,
		Claims:      insertControl(claims, ctrl),
	})
	if err != nil {
		return none, "", fmt.Errorf("claims: %w", err)
	}
	defer func() { _ = e.Claims.Discard(ctx, path) }()

	attempts, err := e.Refuter.Refute(ctx, target, reads, path)
	if err != nil {
		// A refutation that did not happen is not a finding that survived one.
		// The verdict caps itself on the absence instead, the way it does for a
		// missing second opinion.
		return none, "", nil
	}

	check, reason := checkOf(attempts[ctrl.ref], ctrl)
	if check != pb.RefuterCheck_REFUTER_CHECK_PASSED {
		return check, reason, nil
	}
	for ref, attempt := range attempts {
		id, issued := refs[ref]
		if !issued {
			continue
		}
		f := ledger.GetFindings()[id]
		f.Refutation = append(f.GetRefutation(), attempt)
	}
	return check, reason, nil
}

// claimsOf renders the verdict-driving findings as claims, and returns the map
// from the handle each was issued under back to its finding.
//
// The order is fixed so two runs over the same ledger write the same task
// bytes, and so a refuter cannot read anything from the order it was given.
func claimsOf(ledger *pb.Ledger, runID string, iteration uint32) ([]*pb.ClaimUnderTest, map[string]string) {
	ids := make([]string, 0, len(ledger.GetFindings()))
	for id := range ledger.GetFindings() {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	refs := map[string]string{}
	var claims []*pb.ClaimUnderTest
	for _, id := range ids {
		f := ledger.GetFindings()[id]
		if !Open(f.GetStatus()) || !Weighted(tagsFor(f)) {
			continue
		}
		ref := contracts.ClaimRef(runID, iteration, id)
		refs[ref] = id
		claims = append(claims, &pb.ClaimUnderTest{
			Ref:      ref,
			Category: f.GetCategory(),
			Anchor:   f.GetLocation().GetAnchor(),
			Claim:    f.GetEvidence().GetClaim(),
		})
	}
	return claims, refs
}

// provenanceOf reports how much a finding's standing was earned.
//
// A refutation that broke the claim outranks one that failed to: a single
// context that got through is the observation, and the ones that did not are
// the absence of one. Only an attack from a context the engine knows did not
// form the claim can raise the finding above unattacked; anything else is the
// claim vouching for itself, which is what the pass exists to stop.
//
// Both at once is neither. A claim one independent context broke and another
// could not is a disagreement, and the record says so rather than resolving it.
// Both halves have to be independent: contested is the only provenance that
// removes a finding from the blocking counts, and a context the engine cannot
// vouch for must not be what removes it.
func provenanceOf(f *pb.Finding) pb.FindingProvenance {
	refuted, brokeIndependently, upheld := false, false, false
	for _, a := range f.GetRefutation() {
		independent := a.GetContextOrigin() == pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED
		switch a.GetOutcome().(type) {
		case *pb.RefutationAttempt_Refuted:
			refuted = true
			brokeIndependently = brokeIndependently || independent
		case *pb.RefutationAttempt_Upheld:
			upheld = upheld || independent
		}
	}
	switch {
	// Both sides independent. Contested is the only provenance that takes a
	// finding out of the blocking counts, so an attempt from a context the
	// engine cannot vouch for must not be half of the disagreement that does
	// it: that would let an unknown context lift a P0 off a block.
	case brokeIndependently && upheld:
		// Two contexts that did not form the claim reached opposite answers.
		// Reading whichever arrived first as the outcome would let the order
		// of a repeated field decide what the review found.
		return pb.FindingProvenance_FINDING_PROVENANCE_CONTESTED
	case refuted:
		return pb.FindingProvenance_FINDING_PROVENANCE_REFUTED
	case upheld:
		return pb.FindingProvenance_FINDING_PROVENANCE_UPHELD
	}
	return pb.FindingProvenance_FINDING_PROVENANCE_UNATTACKED
}
