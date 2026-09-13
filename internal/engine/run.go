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
	Inventory   InventoryStore
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

	target, inventory, categories, err := e.Collector.Collect(ctx, spec)
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

	// Written only once the state and the ledger have agreed this run belongs to
	// this target, and before the target is attacked. Writing it earlier would
	// leave a rejected run's inventory behind, describing a target the surviving
	// state and ledger are not about.
	if e.Inventory != nil {
		if err := e.Inventory.Save(ctx, inventory); err != nil {
			return nil, fmt.Errorf("inventory: %w", err)
		}
	}

	report, err := e.review(ctx, target, categories, mode, iteration)
	if err != nil {
		return nil, err
	}

	surfaced, oscillation, err := e.apply(ctx, ledger, report, iteration)
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

	yield := marginalYield(report, ledger, surfaced)
	result := e.assemble(target, mode, iteration, final, review, verification, ledger, digest, oscillation, yield)
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
func (e *Engine) review(ctx context.Context, target *pb.Target, categories []pb.Category, mode pb.Mode, iteration uint32) (*pb.ProviderReport, error) {
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
	if !envelope.ContainsReport(string(raw)) {
		return nil, fmt.Errorf("%w: no report envelope", ErrProviderOutput)
	}
	raw, err = envelope.ExtractReport(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}

	report := &pb.ProviderReport{}
	if err := contracts.UnmarshalCanonical(raw, report); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderOutput, err)
	}
	return report, nil
}

// apply admits each reported candidate into the ledger.
//
// A candidate carries no identity, so the engine derives it. Two reviews that
// find the same defect in the same place with the same evidence therefore
// produce the same finding, and a reappearance is a regression rather than a
// duplicate.
//
// Surfaced names the findings whose severity is new information this iteration:
// the ones just admitted and the ones a re-report escalated. The marginal yield
// counts both, so an iteration that only learns an existing finding is critical
// is not mistaken for one that learned nothing. Each is named once however many
// candidates reached it, since the yield counts findings rather than mentions.
func (e *Engine) apply(ctx context.Context, ledger *pb.Ledger, report *pb.ProviderReport, iteration uint32) (surfaced []string, oscillation bool, err error) {
	if ledger.Findings == nil {
		ledger.Findings = map[string]*pb.Finding{}
	}
	named := map[string]bool{}
	for _, candidate := range report.GetCandidates() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		finding, err := e.admit(ctx, candidate, iteration)
		if err != nil {
			return nil, false, err
		}
		id := finding.GetId()
		known, present := ledger.GetFindings()[id]
		switch {
		case !present:
			ledger.Findings[id] = finding
		case finding.GetRisk().GetSeverity() < known.GetRisk().GetSeverity():
			escalate(known, finding, iteration)
		default:
			id = ""
		}
		if id != "" && !named[id] {
			named[id] = true
			surfaced = append(surfaced, id)
		}
		osc, err := contracts.Observe(ledger, finding.GetId(), iteration, actorName)
		if err != nil {
			return nil, false, fmt.Errorf("ledger: %w", err)
		}
		oscillation = oscillation || osc
	}
	return surfaced, oscillation, nil
}

// escalate adopts a re-report's stricter risk and the evidence that earned it.
//
// The finding ID is derived over category, anchor, and claim, so the same claim
// reported at a worse severity lands on the existing record. Severity only
// ratchets: a re-report at a milder severity is left alone, since a provider
// must not be able to defuse a finding by restating it. Retiring one goes
// through a status transition, which is evidenced and legal-transition checked.
func escalate(known, reported *pb.Finding, iteration uint32) {
	known.Risk = reported.GetRisk()
	known.Evidence = reported.GetEvidence()
	known.EvidenceSha256 = reported.GetEvidenceSha256()
	known.History = append(known.GetHistory(), &pb.FindingEvent{
		Iteration:      iteration,
		From:           known.GetStatus(),
		To:             known.GetStatus(),
		EvidenceSha256: reported.GetEvidenceSha256(),
		At:             reported.GetLastSeen(),
		Actor:          actorName,
	})
}

// admit turns a candidate into a ledger finding, assigning everything the
// provider is not permitted to state.
func (e *Engine) admit(ctx context.Context, candidate *pb.CandidateFinding, iteration uint32) (*pb.Finding, error) {
	vetted, err := e.Broker.Admit(ctx, candidate)
	if err != nil {
		return nil, err
	}

	anchor := vetted.GetLocation().GetAnchor()
	claim := vetted.GetEvidence().GetClaim()
	fingerprint := contracts.Fingerprint(vetted.GetCategory(), anchor, claim)
	evidenceSum, err := contracts.Digest(vetted.GetEvidence())
	if err != nil {
		return nil, err
	}
	now := e.Clock.stamp()

	return &pb.Finding{
		Id:                contracts.FindingID(vetted.GetCategory(), fingerprint, false),
		FingerprintSha256: fingerprint,
		Category:          vetted.GetCategory(),
		Location:          vetted.GetLocation(),
		Risk:              vetted.GetRisk(),
		Evidence:          vetted.GetEvidence(),
		EvidenceSha256:    evidenceSum,
		Status:            pb.FindingStatus_FINDING_STATUS_OPEN,
		FirstSeen:         now,
		LastSeen:          now,
		History: []*pb.FindingEvent{{
			Iteration:      iteration,
			To:             pb.FindingStatus_FINDING_STATUS_OPEN,
			EvidenceSha256: evidenceSum,
			At:             now,
			Actor:          actorName,
		}},
	}, nil
}

// verifyIdentity recomputes a finding's ID from its own anchor and evidence.
// The provider never supplies one, so this guards what the engine itself wrote.
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

// marginalYield reports what this iteration added that the ledger did not
// already hold.
//
// The new-finding counts are derived rather than reported: they decide when the
// loop stops, so a provider stating them would choose how long its own review
// ran. The examined and disproved counts are the provider's declared self-grind
// arithmetic and drive nothing.
func marginalYield(report *pb.ProviderReport, ledger *pb.Ledger, surfaced []string) *pb.MarginalYield {
	y := &pb.MarginalYield{
		CandidatesExamined:  report.GetCandidatesExamined(),
		CandidatesDisproved: report.GetCandidatesDisproved(),
	}
	for _, id := range surfaced {
		switch ledger.GetFindings()[id].GetRisk().GetSeverity() {
		case pb.Severity_SEVERITY_P0, pb.Severity_SEVERITY_P1:
			y.NewP0P1++
		case pb.Severity_SEVERITY_P2, pb.Severity_SEVERITY_P3:
			y.NewP2P3++
		}
	}
	return y
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
