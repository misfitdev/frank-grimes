package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeProvider struct {
	out   []byte
	err   error
	calls int
}

func (f *fakeProvider) Review(ctx context.Context, _ Request) (*ProviderOutput, error) {
	f.calls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return &ProviderOutput{Raw: f.out}, nil
}

type memLedger struct {
	ledger *pb.Ledger
	saves  int
}

func (m *memLedger) Load(context.Context) (*pb.Ledger, error) {
	if m.ledger == nil {
		return &pb.Ledger{SchemaMajor: 1}, nil
	}
	return m.ledger, nil
}

func (m *memLedger) Save(_ context.Context, l *pb.Ledger) ([]byte, error) {
	m.saves++
	m.ledger = l
	return contracts.Digest(l)
}

type memResults struct {
	result *pb.GrimesResult
	saves  int
}

func (m *memResults) Load(context.Context) (*pb.GrimesResult, error) { return m.result, nil }
func (m *memResults) Save(_ context.Context, r *pb.GrimesResult) ([]byte, error) {
	m.saves++
	m.result = r
	return contracts.Digest(r)
}

type memState struct {
	state  *pb.LoopState
	saves  int
	clears int
}

func (m *memState) Load(context.Context) (*pb.LoopState, error) { return m.state, nil }
func (m *memState) Save(_ context.Context, s *pb.LoopState) error {
	m.saves++
	m.state = s
	return nil
}
func (m *memState) Clear(context.Context) error { m.clears++; m.state = nil; return nil }

type fixedAdjudicator struct {
	decision pb.Decision
	err      error
}

func (f fixedAdjudicator) Adjudicate(_ context.Context, target *pb.Target, _ *pb.Verdict) (*pb.IndependentReview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pb.IndependentReview{
		RunId:                   "run-adj",
		ReviewerId:              "fake",
		ZeroKnowledge:           true,
		TargetFingerprintSha256: target.GetFingerprintSha256(),
		Verdict: &pb.Verdict{
			Decision:           f.decision,
			ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_LOW,
			ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
			ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT,
		},
		CompletedAt: timestamppb.New(testTime()),
	}, nil
}

func testTime() time.Time { return time.Unix(1780000000, 0).UTC() }

const seedPath, seedClaim = "bad.sh", "caller-controlled deletion"

// p0ID is derived the same way the engine derives it, so the fixture cannot
// drift away from the fingerprint rule it is meant to exercise.
func p0ID() string {
	cat := pb.Category_CATEGORY_SEC
	return contracts.FindingID(cat, contracts.Fingerprint(cat, seedPath, seedClaim), false)
}

// seededLedger holds one open P0 whose id matches its own fingerprint.
func seededLedger(t *testing.T, status pb.FindingStatus) *pb.Ledger {
	t.Helper()
	path, claim := seedPath, seedClaim
	cat := pb.Category_CATEGORY_SEC
	id := p0ID()
	ts := timestamppb.New(testTime())
	return &pb.Ledger{
		SchemaMajor: 1,
		Target:      testTarget(t),
		Findings: map[string]*pb.Finding{
			id: {
				Id:                id,
				FingerprintSha256: contracts.Fingerprint(cat, path, claim),
				Category:          cat,
				Location:          &pb.Location{Path: &pb.RepoPath{Value: path}},
				Risk: &pb.Risk{
					Severity:    pb.Severity_SEVERITY_P0,
					Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
					BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
				},
				Evidence: &pb.Evidence{
					Tier:  pb.EvidenceTier_EVIDENCE_TIER_E2,
					Claim: claim,
					Detail: &pb.Evidence_Citation{Citation: &pb.Citation{
						Path: &pb.RepoPath{Value: path}, Line: 22, Quote: "rm -rf \"$1\"/*",
					}},
				},
				EvidenceSha256: make([]byte, 32),
				Status:         status,
				FirstSeen:      ts,
				LastSeen:       ts,
				History: []*pb.FindingEvent{{
					Iteration: 1, To: pb.FindingStatus_FINDING_STATUS_OPEN,
					EvidenceSha256: make([]byte, 32), At: ts, Actor: "grimes",
				}},
			},
		},
	}
}

func testTarget(t *testing.T) *pb.Target {
	t.Helper()
	target, _, err := PathCollector{}.Collect(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func testSpec() TargetSpec { return TargetSpec{Root: "/repo", Scope: "bad.sh"} }

// proposal renders provider output naming the seeded finding.
func proposal(t *testing.T, ids ...string) []byte {
	t.Helper()
	snaps := make([]*pb.FindingSnapshot, 0, len(ids))
	for _, id := range ids {
		snaps = append(snaps, &pb.FindingSnapshot{
			Id:     id,
			Status: pb.FindingStatus_FINDING_STATUS_OPEN,
			Risk: &pb.Risk{
				Severity:    pb.Severity_SEVERITY_P0,
				Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
				BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
			},
			EvidenceTier:   pb.EvidenceTier_EVIDENCE_TIER_E2,
			EvidenceSha256: make([]byte, 32),
		})
	}
	r := &pb.GrimesResult{
		SchemaMajor:     2,
		RunId:           "run-provider",
		ProducerRole:    pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:          testTarget(t),
		Mode:            pb.Mode_MODE_REPORT,
		Iteration:       1,
		MaxIterations:   5,
		CompletionState: pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE,
		Verdict: &pb.Verdict{
			Decision:           pb.Decision_DECISION_BLOCK,
			ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_CRITICAL,
			ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
			ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT,
		},
		LegacyColor:   pb.LegacyColor_LEGACY_COLOR_RED,
		MarginalYield: &pb.MarginalYield{},
		Counts:        &pb.FindingCounts{Total: uint32(len(ids)), OpenP0: uint32(len(ids))},
		Findings:      snaps,
		Verification:  &pb.Verification{Status: pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE},
		Ledger:        &pb.LedgerRef{Path: contracts.LedgerPath, DigestSha256: make([]byte, 32)},
		UnmetGates:    []string{"decision"},
		Summary:       "provider proposal",
	}
	encoded, err := contracts.EncodeCanonical(r)
	if err != nil {
		t.Fatal(err)
	}
	return []byte("prose before\n" + envelope.Wrap(encoded))
}

func newEngine(p Provider, l Ledger, s StateStore, a Adjudicator) *Engine {
	return &Engine{
		Collector: PathCollector{}, Provider: p, Broker: StrictBroker{},
		Adjudicator: a, Gate: NotApplicableGate{}, Ledger: l,
		Results: &memResults{}, State: s,
		Clock: func() time.Time { return testTime() },
		RunID: "run-001", MaxIterations: 5,
	}
}

func TestRunEndToEndReportMode(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, s, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetProducerRole() != pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR {
		t.Errorf("producer role = %v, want orchestrator", result.GetProducerRole())
	}
	if result.GetCompletionState() != pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE {
		t.Errorf("completion state = %v, want review complete", result.GetCompletionState())
	}
	if result.GetVerification().GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE {
		t.Errorf("verification = %v, want not applicable", result.GetVerification().GetStatus())
	}
	if result.GetLegacyColor() != pb.LegacyColor_LEGACY_COLOR_RED {
		t.Errorf("color = %v, want red for an open P0", result.GetLegacyColor())
	}
	if _, err := contracts.EncodeCanonical(result); err != nil {
		t.Errorf("result fails the contract: %v", err)
	}
	if l.saves != 1 {
		t.Errorf("ledger saves = %d, want 1", l.saves)
	}
	if s.saves != 1 {
		t.Errorf("state saves = %d, want 1", s.saves)
	}
}

// The hook verifies the result against the digest state recorded, so a state
// naming a result that was never written would be unverifiable.
func TestRunPersistsResultBeforeState(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	results := &memResults{}
	e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, s, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})
	e.Results = results

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results.saves != 1 {
		t.Fatalf("result saves = %d, want 1", results.saves)
	}
	want, err := contracts.Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(s.state.GetLastResultSha256()) != string(want) {
		t.Error("state records a digest that is not the persisted result")
	}
	if !proto.Equal(results.result, result) {
		t.Error("persisted result differs from the one returned")
	}
}

// The provider proposed a full GREEN tuple. The engine must ignore it and
// derive RED from the open P0 the ledger actually holds.
func TestRunProviderCannotSelfCertify(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	green := greenProposal(t)
	e := newEngine(&fakeProvider{out: green}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetLegacyColor() == pb.LegacyColor_LEGACY_COLOR_GREEN {
		t.Fatal("engine adopted the provider's GREEN")
	}
	if result.GetVerdict().GetDecision() != pb.Decision_DECISION_BLOCK {
		t.Errorf("decision = %v, want block", result.GetVerdict().GetDecision())
	}
	if result.GetSummary() == "provider proposal" {
		t.Error("engine adopted the provider's summary")
	}
	if result.GetRunId() != "run-001" {
		t.Errorf("run id = %q, want the engine's own", result.GetRunId())
	}
}

func greenProposal(t *testing.T) []byte {
	t.Helper()
	r := &pb.GrimesResult{
		SchemaMajor:     2,
		RunId:           "run-provider",
		ProducerRole:    pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:          testTarget(t),
		Mode:            pb.Mode_MODE_REPORT,
		Iteration:       1,
		MaxIterations:   5,
		CompletionState: pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE,
		Verdict: &pb.Verdict{
			Decision:           pb.Decision_DECISION_PASS,
			ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_LOW,
			ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
			ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT,
		},
		LegacyColor:   pb.LegacyColor_LEGACY_COLOR_GREEN,
		MarginalYield: &pb.MarginalYield{},
		Counts:        &pb.FindingCounts{},
		Findings: []*pb.FindingSnapshot{{
			Id:     p0ID(),
			Status: pb.FindingStatus_FINDING_STATUS_OPEN,
			Risk: &pb.Risk{
				Severity:    pb.Severity_SEVERITY_P0,
				Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
				BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
			},
			EvidenceTier:   pb.EvidenceTier_EVIDENCE_TIER_E2,
			EvidenceSha256: make([]byte, 32),
		}},
		Verification: &pb.Verification{Status: pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE},
		IndependentReview: &pb.IndependentReview{
			RunId: "self", ReviewerId: "self", ZeroKnowledge: true,
			TargetFingerprintSha256: testTarget(t).GetFingerprintSha256(),
			Verdict: &pb.Verdict{
				Decision:           pb.Decision_DECISION_PASS,
				ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_LOW,
				ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
				ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT,
			},
			CompletedAt: timestamppb.New(testTime()),
		},
		Ledger:  &pb.LedgerRef{Path: contracts.LedgerPath, DigestSha256: make([]byte, 32)},
		Summary: "provider proposal",
	}
	encoded, err := contracts.EncodeCanonical(r)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(envelope.Wrap(encoded))
}

func TestRunProviderErrorFailsClosed(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	e := newEngine(&fakeProvider{err: errors.New("boom")}, l, s, nil)

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err == nil {
		t.Fatal("want an error when the provider fails")
	}
	if l.saves != 0 {
		t.Errorf("ledger written despite a provider failure (saves = %d)", l.saves)
	}
	if s.saves != 0 {
		t.Errorf("state written despite a provider failure (saves = %d)", s.saves)
	}
}

func TestRunRejectsBadProviderOutput(t *testing.T) {
	for _, c := range []struct {
		name string
		out  []byte
	}{
		{"no envelope", []byte("I reviewed it and it looks fine")},
		{"garbage in envelope", []byte(envelope.Wrap([]byte("not a protobuf")))},
		{"empty", nil},
	} {
		l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
		e := newEngine(&fakeProvider{out: c.out}, l, &memState{}, nil)
		_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
		if err == nil {
			t.Errorf("%s: want an error", c.name)
		}
		if l.saves != 0 {
			t.Errorf("%s: ledger written for rejected output", c.name)
		}
	}
}

// A provider naming a finding the ledger has never seen cannot conjure one:
// the engine has no evidence record to back it.
func TestRunRejectsUnknownFinding(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	e := newEngine(&fakeProvider{out: proposal(t, "FG-SEC-ffffffffffff")}, l, &memState{}, nil)
	_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if !errors.Is(err, ErrProviderOutput) {
		t.Errorf("error = %v, want ErrProviderOutput", err)
	}
}

func TestRunStaleStateFailsClosed(t *testing.T) {
	other, _, err := PathCollector{}.Collect(context.Background(), TargetSpec{Root: "/repo", Scope: "somewhere-else"})
	if err != nil {
		t.Fatal(err)
	}
	s := &memState{state: &pb.LoopState{
		SchemaMajor: 2, RunId: "run-000", Target: other,
		Mode: pb.Mode_MODE_REPORT, Iteration: 1, MaxIterations: 5,
		LedgerDigestSha256: make([]byte, 32), LastResultSha256: make([]byte, 32),
	}}
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, s, nil)

	_, err = e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if !errors.Is(err, ErrStaleState) {
		t.Errorf("error = %v, want ErrStaleState", err)
	}
	if l.saves != 0 {
		t.Error("ledger written against stale state")
	}
}

func TestRunIterationLimit(t *testing.T) {
	target := testTarget(t)
	s := &memState{state: &pb.LoopState{
		SchemaMajor: 2, RunId: "run-001", Target: target,
		Mode: pb.Mode_MODE_REPORT, Iteration: 5, MaxIterations: 5,
		LedgerDigestSha256: make([]byte, 32), LastResultSha256: make([]byte, 32),
	}}
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, s, nil)

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err == nil {
		t.Fatal("want an error past the iteration limit")
	}
}

func TestRunCancelledContextWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	p := &fakeProvider{out: proposal(t, p0ID())}
	e := newEngine(p, l, s, nil)

	if _, err := e.Run(ctx, testSpec(), pb.Mode_MODE_REPORT); err == nil {
		t.Fatal("want an error for a cancelled run")
	}
	if l.saves != 0 || s.saves != 0 {
		t.Errorf("cancelled run persisted state (ledger %d, state %d)", l.saves, s.saves)
	}
}

func TestRunFixModeRejected(t *testing.T) {
	e := newEngine(&fakeProvider{}, &memLedger{}, &memState{}, nil)
	_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_FIX)
	if !errors.Is(err, ErrFixModeUnsupported) {
		t.Errorf("error = %v, want ErrFixModeUnsupported", err)
	}
}

// Without a second opinion the run cannot reach a pass, whatever the ledger says.
func TestRunWithoutAdjudicatorCapsAtConditional(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 1, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: proposal(t)}, l, &memState{}, nil)

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetVerdict().GetDecision() == pb.Decision_DECISION_PASS {
		t.Error("reached a pass with no adjudication")
	}
	if result.GetLegacyColor() == pb.LegacyColor_LEGACY_COLOR_GREEN {
		t.Error("reached GREEN with no adjudication")
	}
	if !hasGate(result.GetUnmetGates(), GateAdjudication) {
		t.Errorf("unmet gates = %v, want %q", result.GetUnmetGates(), GateAdjudication)
	}
}

// An adjudicator that fails is unavailable, not agreement.
func TestRunAdjudicatorFailureIsNotAgreement(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 1, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: proposal(t)}, l, &memState{}, fixedAdjudicator{err: errors.New("unreachable")})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetVerdict().GetDecision() == pb.Decision_DECISION_PASS {
		t.Error("a failed adjudication was treated as agreement")
	}
}

func TestRunIndependentBlockOverridesClean(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 1, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: proposal(t)}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_BLOCK})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetVerdict().GetDecision() != pb.Decision_DECISION_BLOCK {
		t.Errorf("decision = %v, want block from the independent review", result.GetVerdict().GetDecision())
	}
	if result.GetLegacyColor() != pb.LegacyColor_LEGACY_COLOR_RED {
		t.Errorf("color = %v, want red", result.GetLegacyColor())
	}
}

// A fingerprint already fixed reappearing is a regression, which makes a pass
// unreachable for the run.
func TestRunRegressionSetsOscillation(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_FIXED)}
	e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.GetLedger().GetOscillationDetected() {
		t.Error("a fixed finding reappearing did not set oscillation")
	}
	if result.GetLegacyColor() == pb.LegacyColor_LEGACY_COLOR_GREEN {
		t.Error("GREEN survived an oscillating ledger")
	}
}

// Two runs over the same ledger at the same instant must produce identical
// bytes, so a digest identifies a result rather than the moment it was taken.
func TestRunResultIsDeterministic(t *testing.T) {
	restore := contracts.Now
	contracts.Now = testTime
	t.Cleanup(func() { contracts.Now = restore })

	encode := func() []byte {
		l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
		e := newEngine(&fakeProvider{out: proposal(t, p0ID())}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})
		result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := contracts.EncodeCanonical(result)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	if string(encode()) != string(encode()) {
		t.Error("two identical runs produced different result bytes")
	}
}
