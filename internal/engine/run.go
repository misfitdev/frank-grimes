package engine

import (
	"bytes"
	"context"
	"fmt"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"google.golang.org/protobuf/proto"
)

// Engine runs reviews. Every field is a seam; none carries methodology.
type Engine struct {
	Collector   Collector
	Provider    Provider
	Broker      EvidenceBroker
	Adjudicator Adjudicator
	Gate        GateRunner
	Ledger      Ledger
	Results     ResultStore
	State       StateStore
	Clock       Clock

	RunID         string
	AutoLoop      bool
	MaxIterations uint32
	Research      string
	Dir           string
}

// Run executes one iteration and returns the result the engine derived.
func (e *Engine) Run(ctx context.Context, spec TargetSpec, mode pb.Mode) (*pb.GrimesResult, error) {
	if mode == pb.Mode_MODE_FIX {
		return nil, ErrFixModeUnsupported
	}

	target, categories, err := e.Collector.Collect(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("collect: %w", err)
	}

	iteration, err := e.resume(ctx, target)
	if err != nil {
		return nil, err
	}

	ledger, err := e.Ledger.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}
	if err := adoptLedger(ledger, target); err != nil {
		return nil, err
	}

	proposal, err := e.review(ctx, target, categories, mode, iteration)
	if err != nil {
		return nil, err
	}

	oscillation, err := e.apply(ctx, ledger, proposal, iteration)
	if err != nil {
		return nil, err
	}

	verification, err := e.Gate.Run(ctx, e.Dir)
	if err != nil {
		return nil, fmt.Errorf("gate: %w", err)
	}

	candidates := candidatesOf(ledger)
	primary := Derive(DeriveInput{
		Candidates:              candidates,
		AdjudicationAvailable:   false,
		AllCategoriesStopped:    false,
		CriticalInvariantProbed: len(candidates) > 0,
		Oscillation:             oscillation,
	})

	review, err := e.adjudicate(ctx, target, primary.Verdict)
	if err != nil {
		return nil, err
	}

	final := Derive(DeriveInput{
		Candidates:              candidates,
		AdjudicationAvailable:   review != nil,
		IndependentDecision:     review.GetVerdict().GetDecision(),
		AllCategoriesStopped:    false,
		CriticalInvariantProbed: len(candidates) > 0,
		Oscillation:             oscillation,
	})

	digest, err := e.Ledger.Save(ctx, ledger)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}

	result := e.assemble(target, mode, iteration, final, review, verification, ledger, digest, oscillation)
	if _, err := contracts.EncodeCanonical(result); err != nil {
		return nil, fmt.Errorf("derived result rejected by the contract: %w", err)
	}
	if err := e.persist(ctx, target, mode, iteration, result, digest); err != nil {
		return nil, err
	}
	return result, nil
}

// adoptLedger binds an empty ledger to this target and refuses one raised
// against a different artifact.
//
// The contract pins the ledger to a single path, so a directory holds one
// ledger. Reviewing a second target in that directory would otherwise count the
// first target's findings toward the second target's verdict, and the result
// would be wrong rather than absent.
func adoptLedger(ledger *pb.Ledger, target *pb.Target) error {
	if ledger.GetTarget() == nil {
		ledger.Target = target
		return nil
	}
	if !bytes.Equal(ledger.GetTarget().GetFingerprintSha256(), target.GetFingerprintSha256()) {
		return fmt.Errorf("%w: %s holds findings for %q, this run reviews %q; move it aside to start a new ledger",
			ErrLedgerTarget, contracts.LedgerPath,
			ledger.GetTarget().GetScope(), target.GetScope())
	}
	return nil
}

// resume returns the iteration this run is on, refusing state raised against a
// different target.
func (e *Engine) resume(ctx context.Context, target *pb.Target) (uint32, error) {
	state, err := e.State.Load(ctx)
	if err != nil {
		return 0, fmt.Errorf("state: %w", err)
	}
	if state == nil {
		return 1, nil
	}
	if !proto.Equal(state.GetTarget(), target) {
		return 0, fmt.Errorf("%w: state holds %q, this run is %q",
			ErrStaleState, state.GetTarget().GetScope(), target.GetScope())
	}
	next := state.GetIteration() + 1
	if next > e.MaxIterations {
		return 0, fmt.Errorf("iteration %d exceeds the limit of %d", next, e.MaxIterations)
	}
	return next, nil
}

// review obtains and decodes provider output. Nothing here is trusted beyond
// its findings; every authoritative field on the proposal is discarded.
func (e *Engine) review(ctx context.Context, target *pb.Target, categories []pb.Category, mode pb.Mode, iteration uint32) (*pb.GrimesResult, error) {
	out, err := e.Provider.Review(ctx, Request{
		Role:       RolePrimary,
		Target:     target,
		Mode:       mode,
		Iteration:  iteration,
		Categories: categories,
		Research:   e.Research,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderFailed, err)
	}

	raw := out.Raw
	if envelope.Contains(string(raw)) {
		raw, err = envelope.Extract(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
		}
	} else {
		return nil, fmt.Errorf("%w: no result envelope", ErrProviderOutput)
	}

	proposal := &pb.GrimesResult{}
	if err := contracts.UnmarshalCanonical(raw, proposal); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	return proposal, nil
}

// apply admits each proposed finding and records it against the ledger.
func (e *Engine) apply(ctx context.Context, ledger *pb.Ledger, proposal *pb.GrimesResult, iteration uint32) (bool, error) {
	oscillation := proposal.GetLedger().GetOscillationDetected()
	for _, snap := range proposal.GetFindings() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		existing, ok := ledger.GetFindings()[snap.GetId()]
		if !ok {
			// A snapshot names a finding the ledger has never seen. A snapshot
			// carries no location and no evidence detail, so the engine cannot
			// build the record the contract requires and refuses rather than
			// inventing one. Broker.Admit is unreachable until a provider can
			// send a whole Finding.
			return false, fmt.Errorf("%w: %s is not in the ledger", ErrProviderOutput, snap.GetId())
		}
		if err := verifyIdentity(existing); err != nil {
			return false, err
		}
		osc, err := contracts.Observe(ledger, snap.GetId(), iteration, "grimes")
		if err != nil {
			return false, fmt.Errorf("ledger: %w", err)
		}
		oscillation = oscillation || osc
	}
	return oscillation, nil
}

// verifyIdentity recomputes a finding's ID from its own anchor and evidence. A
// provider cannot rename a finding to escape its history.
func verifyIdentity(f *pb.Finding) error {
	anchor := f.GetLocation().GetAnchor()
	claim := f.GetEvidence().GetClaim()
	want := contracts.FindingID(f.GetCategory(), contracts.Fingerprint(f.GetCategory(), anchor, claim), false)
	if f.GetId() != want {
		return fmt.Errorf("%w: %s should be %s", ErrForgedFindingID, f.GetId(), want)
	}
	return nil
}

func (e *Engine) adjudicate(ctx context.Context, target *pb.Target, claimed *pb.Verdict) (*pb.IndependentReview, error) {
	if e.Adjudicator == nil {
		return nil, nil
	}
	review, err := e.Adjudicator.Adjudicate(ctx, target, claimed)
	if err != nil {
		// Absence of a second opinion is not agreement, but it is also not a
		// run failure: the verdict caps itself instead.
		return nil, nil
	}
	return review, nil
}

// persist writes the result before the state that references it, so state
// never names a result digest that is not on disk.
//
// Loop state is what a stop hook reads to ask for another iteration, so a run
// that was not asked to loop leaves none. Otherwise omitting --auto-loop would
// still produce a hook request for a second pass.
func (e *Engine) persist(ctx context.Context, target *pb.Target, mode pb.Mode, iteration uint32, result *pb.GrimesResult, ledgerDigest []byte) error {
	resultDigest, err := e.Results.Save(ctx, result)
	if err != nil {
		return err
	}
	if !e.AutoLoop {
		return e.State.Clear(ctx)
	}
	return e.State.Save(ctx, &pb.LoopState{
		SchemaMajor:        contracts.SchemaMajor,
		RunId:              e.RunID,
		Target:             target,
		Mode:               mode,
		Iteration:          iteration,
		MaxIterations:      e.MaxIterations,
		LedgerDigestSha256: ledgerDigest,
		LastResultSha256:   resultDigest,
	})
}

func candidatesOf(ledger *pb.Ledger) []Candidate {
	out := make([]Candidate, 0, len(ledger.GetFindings()))
	for _, f := range ledger.GetFindings() {
		out = append(out, Candidate{
			Severity: f.GetRisk().GetSeverity(),
			Status:   f.GetStatus(),
			Tier:     f.GetEvidence().GetTier(),
		})
	}
	return out
}
