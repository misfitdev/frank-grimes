package engine

import (
	"bytes"
	"context"
	"fmt"
	"slices"

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
	Refuter     Refuter
	Gate        GateRunner
	Inventory   InventoryStore
	Content     ContentStore
	Claims      ClaimStore
	Ledger      Ledger
	Results     ResultStore
	State       StateStore
	Clock       Clock

	RunID         string
	AutoLoop      bool
	MaxIterations uint32
	Research      string
	Dir           string
	// Confinement names the mechanism each role ran under, for the run record.
	Confinement string
	// Unconfined is set when the operator waived it.
	Unconfined bool
	// Commit authorizes one commit per verified batch, separately from fix
	// mode itself. Without it a fix run edits and stops.
	Commit bool
	// Mode is what this run was asked for, held here because the roles after
	// the first are handed a target a fix run has already changed.
	Mode pb.Mode
}

// Run executes one iteration and returns the result the engine derived.
func (e *Engine) Run(ctx context.Context, spec TargetSpec, mode pb.Mode) (*pb.GrimesResult, error) {
	// Before collection: in fix mode the bytes under review are the worktree's,
	// so the worktree has to exist before anything is fingerprinted.
	e.Mode = mode
	var fix *fixRun
	if mode == pb.Mode_MODE_FIX {
		var err error
		if fix, err = e.prepareFix(ctx, spec.Scope); err != nil {
			return nil, err
		}
		spec.Root = fix.tree.Dir
		spec.KeepBodies = true
	}

	collected, err := e.Collector.Collect(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("collect: %w", err)
	}
	target := collected.Target

	ledger, err := e.Ledger.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}

	iteration, err := e.resume(ctx, target, ledger)
	if err != nil {
		return nil, err
	}
	if err := adoptLedger(ledger, target); err != nil {
		return nil, err
	}

	// Written only once the state and the ledger have agreed this run belongs to
	// this target, and before the target is attacked. Writing it earlier would
	// leave a rejected run's inventory behind, describing a target the surviving
	// state and ledger are not about.
	if e.Inventory != nil {
		if err := e.Inventory.Save(ctx, collected.Inventory); err != nil {
			return nil, fmt.Errorf("inventory: %w", err)
		}
	}
	// A target with no path of its own is written down, so the provider has
	// something to read. Same moment as the inventory and for the same reason:
	// a rejected run must not leave content behind describing a target the
	// surviving state and ledger are not about.
	if len(collected.ContentBytes) > 0 {
		if e.Content == nil {
			return nil, fmt.Errorf("%w: no store for a target with no path of its own", ErrProviderOutput)
		}
		path, err := e.Content.Save(ctx, collected.ContentBytes)
		if err != nil {
			return nil, fmt.Errorf("target content: %w", err)
		}
		collected.ContentPath = path
	}

	inventoryPath := ""
	if e.Inventory != nil {
		inventoryPath = e.Inventory.Path()
	}
	// The worktree is the one place a fixing role may write, and the only role
	// that gets it is this one.
	writeRoot := ""
	if fix != nil {
		writeRoot = fix.tree.Dir
	}
	report, err := e.review(ctx, target, collected.ContentPath, inventoryPath, writeRoot, collected.Categories, mode, iteration)
	if err != nil {
		return nil, err
	}

	surfaced, oscillation, err := e.apply(ctx, ledger, report, collected, iteration)
	if err != nil {
		return nil, err
	}

	cov, err := measure(report, collected.Inventory, collected.Categories)
	if err != nil {
		return nil, err
	}

	// The gate runs where the batch is, which in report mode is the review
	// directory nothing edited.
	gateDir := e.Dir
	if fix != nil {
		gateDir = fix.tree.Dir
	}
	if fix != nil {
		if g, ok := e.Gate.(ExecGate); ok {
			g.Root, g.Writable = e.Dir, []string{fix.tree.Dir}
			e.Gate = g
		}
	}
	verification, err := e.Gate.Run(ctx, gateDir)
	if err != nil {
		return nil, fmt.Errorf("gate: %w", err)
	}
	if fix != nil {
		fix.gate = verification
		if err := e.settle(ctx, fix, ledger, iteration); err != nil {
			return nil, err
		}
	}

	// Before either derivation reads the ledger: both tuples have to be over the
	// same findings, and a claim's standing under attack is part of the finding
	// rather than of the verdict that reads it.
	check, err := e.refute(ctx, ledger, spec, collected, iteration)
	if err != nil {
		return nil, err
	}

	candidates := candidatesOf(ledger)
	blocked := rankingBlocked(ledger)
	falsifierUnavailable := criticalFalsifierUnavailable(ledger)
	primary := Derive(DeriveInput{
		Candidates:                   candidates,
		AdjudicationAvailable:        false,
		RankingBlocked:               blocked,
		CriticalFalsifierUnavailable: falsifierUnavailable,
		AllCategoriesStopped:         cov.CategoriesStopped,
		CriticalInvariantProbed:      cov.Probed,
		CriticalUnknownRemains:       cov.UnknownRemains,
		CoverageIncomplete:           len(cov.Unaccounted) > 0,
		Oscillation:                  oscillation,
		Unconfined:                   e.Unconfined,
	})

	review, err := e.adjudicate(ctx, spec, collected, primary.Verdict)
	if err != nil {
		return nil, err
	}

	final := Derive(DeriveInput{
		Candidates:                   candidates,
		AdjudicationAvailable:        review != nil,
		RankingBlocked:               blocked,
		CriticalFalsifierUnavailable: falsifierUnavailable,
		IndependentDecision:          review.GetVerdict().GetDecision(),
		IndependentContextUnknown: review != nil &&
			review.GetContextOrigin() != pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED,
		AllCategoriesStopped:    cov.CategoriesStopped,
		CriticalInvariantProbed: cov.Probed,
		CriticalUnknownRemains:  cov.UnknownRemains,
		CoverageIncomplete:      len(cov.Unaccounted) > 0,
		Oscillation:             oscillation,
		Unconfined:              e.Unconfined,
	})

	// The target a fix run leaves behind is not the one it reviewed, and that is
	// the one difference between the two modes here.
	if fix != nil {
		if err := e.rebaseline(ctx, spec, collected, ledger); err != nil {
			return nil, err
		}
	} else if err := e.recheck(ctx, spec, collected); err != nil {
		return nil, err
	}

	digest, err := e.Ledger.Save(ctx, ledger)
	if err != nil {
		return nil, fmt.Errorf("ledger: %w", err)
	}

	yield := marginalYield(report, ledger, surfaced)
	result := e.assemble(target, mode, iteration, final, review, verification, ledger, digest, oscillation, yield, check)
	if fix != nil {
		result.FixBatch = fix.record()
	}
	if _, err := contracts.EncodeCanonical(result); err != nil {
		return nil, fmt.Errorf("derived result rejected by the contract: %w", err)
	}
	if err := e.persist(ctx, target, mode, iteration, result, digest); err != nil {
		return nil, err
	}
	return result, nil
}

// handoff returns the path a later role reads the target at.
//
// The primary provider runs with write access to the review directory, so the
// file it was pointed at is not the file a role after it should read. A target
// with no path of its own is written out again from the bytes collection
// fingerprinted, once per role. One with a path of its own cannot be copied
// that way, so it is recollected and the run refused if its fingerprint moved.
//
// In fix mode the target has changed by now, on purpose, and what a later role
// reads is the batch rather than the bytes the primary reviewed. That can only
// make the verdict stricter, since the engine takes the stricter of the two
// decisions either way, and the fingerprint the role answers against is still
// the reviewed one. Handing it the reviewed bytes instead is fg-dfs.
func (e *Engine) handoff(ctx context.Context, spec TargetSpec, collected *Collected, role Role) (string, error) {
	if len(collected.ContentBytes) == 0 {
		if e.Mode != pb.Mode_MODE_FIX {
			if err := e.recheck(ctx, spec, collected); err != nil {
				return "", err
			}
		}
		return collected.ContentPath, nil
	}
	if e.Content == nil {
		return "", fmt.Errorf("%w: no store for a target with no path of its own", ErrProviderOutput)
	}
	return e.Content.Stage(ctx, role, collected.ContentBytes)
}

// recheck confirms the target still hashes to what collection recorded.
//
// Evidence is checked against the digests collection took, so a line a provider
// planted cannot be quoted. An edit nobody quotes reaches no check at all: the
// run would persist a ledger and a verdict bound to a fingerprint over bytes
// that are no longer there.
func (e *Engine) recheck(ctx context.Context, spec TargetSpec, collected *Collected) error {
	want := collected.Target.GetFingerprintSha256()
	if len(collected.ContentBytes) > 0 {
		// A pasted argument has no source to collect from twice, and the engine
		// holds it rather than reading it back: the staged file is a copy handed
		// to a provider, not the target. Nothing downstream reads it.
		return nil
	}
	again, err := e.Collector.Collect(ctx, spec)
	if err != nil {
		return fmt.Errorf("rechecking the target: %w", err)
	}
	if !bytes.Equal(again.Target.GetFingerprintSha256(), want) {
		return fmt.Errorf("%w: %q changed during the review", ErrTargetChanged, collected.Target.GetScope())
	}
	return nil
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
func (e *Engine) resume(ctx context.Context, target *pb.Target, ledger *pb.Ledger) (uint32, error) {
	state, err := e.State.Load(ctx)
	if err != nil {
		return 0, fmt.Errorf("state: %w", err)
	}
	if state == nil {
		return 1, nil
	}
	// A fix run leaves state describing the bytes it reviewed and a ledger
	// describing the bytes it produced, so the iteration after it collects
	// neither of the two things a report-mode run would compare. What carries
	// it across is the ledger's own record of the change: this target, from
	// that one. What refuses a move nobody recorded is adoptLedger, below, and
	// it is unchanged.
	carried := descends(ledger, state.GetTarget().GetFingerprintSha256()) &&
		bytes.Equal(ledger.GetTarget().GetFingerprintSha256(), target.GetFingerprintSha256())
	if !proto.Equal(state.GetTarget(), target) && !carried {
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
func (e *Engine) review(ctx context.Context, target *pb.Target, contentPath, inventoryPath, writeRoot string, categories []pb.Category, mode pb.Mode, iteration uint32) (*pb.ProviderReport, error) {
	out, err := e.Provider.Review(ctx, Request{
		Role:          RolePrimary,
		RunID:         e.RunID,
		Target:        target,
		ContentPath:   contentPath,
		InventoryPath: inventoryPath,
		WriteRoot:     writeRoot,
		Mode:          mode,
		Iteration:     iteration,
		Categories:    categories,
		Research:      e.Research,
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
	if why := reportBinding(report, e.RunID, target, mode, iteration); why != "" {
		return nil, fmt.Errorf("%w: %s", ErrProviderOutput, why)
	}
	return report, nil
}

// reportBinding names why a report is not the answer to this request, or
// returns "" when it is.
//
// The fields are validated as present but bound to nothing, so a report left on
// disk by an earlier iteration reads as a fresh one. The adapter's documented
// `cat .grimes/report.envelope` makes that the ordinary case rather than the
// adversarial one: a provider that fails to rewrite the file hands back the
// previous answer, whose candidates would be admitted and whose examined count
// would feed the stopping rule.
func reportBinding(report *pb.ProviderReport, runID string, target *pb.Target, mode pb.Mode, iteration uint32) string {
	if report.GetSchemaMajor() != contracts.SchemaMajor {
		return fmt.Sprintf("report declares contract major %d, expected %d",
			report.GetSchemaMajor(), contracts.SchemaMajor)
	}
	if report.GetRunId() != runID {
		return fmt.Sprintf("report belongs to run %q, this is run %q", report.GetRunId(), runID)
	}
	if !bytes.Equal(report.GetTarget().GetFingerprintSha256(), target.GetFingerprintSha256()) {
		return "report reviewed a different target than this run"
	}
	if report.GetMode() != mode {
		return fmt.Sprintf("report was produced in %v, this run is %v", report.GetMode(), mode)
	}
	if report.GetIteration() != iteration {
		return fmt.Sprintf("report was taken at iteration %d, this run is at %d",
			report.GetIteration(), iteration)
	}
	return ""
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
func (e *Engine) apply(ctx context.Context, ledger *pb.Ledger, report *pb.ProviderReport, against *Collected, iteration uint32) (surfaced []string, oscillation bool, err error) {
	if ledger.Findings == nil {
		ledger.Findings = map[string]*pb.Finding{}
	}
	// Admitted before anything is recorded: whether a finding is new or a
	// reworded restatement of one already held depends on what else this
	// iteration reported, which is not known partway through the list.
	admitted := make([]*pb.Finding, 0, len(report.GetCandidates()))
	reported := map[string]bool{}
	for _, candidate := range report.GetCandidates() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		finding, err := e.admit(ctx, candidate, against, iteration)
		if err != nil {
			return nil, false, err
		}
		admitted = append(admitted, finding)
		reported[finding.GetId()] = true
	}

	named := map[string]bool{}
	for _, finding := range admitted {
		id := finding.GetId()
		known, present := ledger.GetFindings()[id]
		switch {
		case !present:
			replaced, err := supersede(ledger, finding, reported, iteration)
			if err != nil {
				return nil, false, fmt.Errorf("ledger: %w", err)
			}
			ledger.Findings[id] = finding
			if replaced != pb.FindingStatus_FINDING_STATUS_UNSPECIFIED {
				// Restating a defect the ledger had recorded as gone is the same
				// news as that defect coming back under its own identity, and a
				// successor is not a fresh discovery either way.
				oscillation = oscillation || replaced == pb.FindingStatus_FINDING_STATUS_FIXED ||
					replaced == pb.FindingStatus_FINDING_STATUS_VERIFIED
				id = ""
			}
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

// supersede links a finding to the one it restates, and reports what status
// that one held. It reports UNSPECIFIED when this finding replaces nothing.
//
// A claim is prose, and prose is reworded far more readily than code is moved.
// The same defect described in different words derives a different identity, so
// without this the ledger holds two findings where there is one: the first
// never closes and the second reads as news.
//
// What makes the two the same is structural rather than textual, because
// nothing here can judge whether two sentences mean the same thing: one record
// at this anchor, in this category, that this iteration did not report. Two
// such records are two defects the engine cannot tell apart, and it links
// neither rather than guessing which was replaced.
// supersedable reports whether a record may stand aside for a restatement.
//
// An accepted risk and a dismissed one are human decisions with a name against
// them. Retiring either on a rewording would let a provider undo a person's
// call by restating the claim.
func supersedable(s pb.FindingStatus) bool {
	switch s {
	case pb.FindingStatus_FINDING_STATUS_OPEN,
		pb.FindingStatus_FINDING_STATUS_FIXED,
		pb.FindingStatus_FINDING_STATUS_VERIFIED,
		pb.FindingStatus_FINDING_STATUS_REGRESSED:
		return true
	default:
		return false
	}
}

func supersede(ledger *pb.Ledger, finding *pb.Finding, reported map[string]bool, iteration uint32) (pb.FindingStatus, error) {
	kind, parts := contracts.AnchorKey(finding.GetLocation().GetAnchor())
	var replaced *pb.Finding
	for id, held := range ledger.GetFindings() {
		if reported[id] || held.GetCategory() != finding.GetCategory() {
			continue
		}
		if !supersedable(held.GetStatus()) {
			continue
		}
		heldKind, heldParts := contracts.AnchorKey(held.GetLocation().GetAnchor())
		if heldKind != kind || !slices.Equal(heldParts, parts) {
			continue
		}
		if replaced != nil {
			return pb.FindingStatus_FINDING_STATUS_UNSPECIFIED, nil
		}
		replaced = held
	}
	if replaced == nil {
		return pb.FindingStatus_FINDING_STATUS_UNSPECIFIED, nil
	}

	was := replaced.GetStatus()
	if _, err := contracts.Transition(ledger, replaced.GetId(), pb.FindingStatus_FINDING_STATUS_SUPERSEDED,
		contracts.TransitionOpts{Iteration: iteration, Actor: actorName}); err != nil {
		return pb.FindingStatus_FINDING_STATUS_UNSPECIFIED, err
	}
	finding.Supersedes = replaced.GetId()
	return was, nil
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
func (e *Engine) admit(ctx context.Context, candidate *pb.CandidateFinding, against *Collected, iteration uint32) (*pb.Finding, error) {
	vetted, err := e.Broker.Admit(ctx, candidate, against)
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

func (e *Engine) adjudicate(ctx context.Context, spec TargetSpec, collected *Collected, claimed *pb.Verdict) (*pb.IndependentReview, error) {
	if e.Adjudicator == nil {
		return nil, nil
	}
	// Staged only once there is a second opinion to hand it to, for the same
	// reason refutation stages only when a pass will run.
	contentPath, err := e.handoff(ctx, spec, collected, RoleAdjudicator)
	if err != nil {
		return nil, err
	}
	review, err := e.Adjudicator.Adjudicate(ctx, collected.Target, contentPath, claimed)
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
		// A superseded record is history its successor points at. Weighing both
		// would count one defect twice, in the totals and in the residual risk.
		if f.GetStatus() == pb.FindingStatus_FINDING_STATUS_SUPERSEDED {
			continue
		}
		out = append(out, weigh(f))
	}
	return out
}
