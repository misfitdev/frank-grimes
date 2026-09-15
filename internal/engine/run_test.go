package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
		return &pb.Ledger{SchemaMajor: 2}, nil
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

func (f fixedAdjudicator) Adjudicate(_ context.Context, target *pb.Target, _ string, _ *pb.Verdict) (*pb.IndependentReview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pb.IndependentReview{
		RunId:                   "run-adj",
		ReviewerId:              "fake",
		ContextOrigin:           pb.ContextOrigin_CONTEXT_ORIGIN_ENGINE_SPAWNED,
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

// seedQuote is in the target the stub collector writes, because the broker now
// checks a citation against the artifact rather than taking its word for it.
const seedQuote = "rm -rf \"$1\"/*"

// seedUnits are the files seedRoot holds, and the inventory the stub collector
// reports. The engine reads the target to check evidence against it, so a
// fabricated path would exercise the failure to read rather than the behaviour
// under test.
var seedUnits = []string{seedPath, "slow.sh"}

var seedRoot = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "grimes-seed")
	if err != nil {
		panic(err)
	}
	body := "#!/bin/sh\nclean() {\n  " + seedQuote + "\n}\n"
	for _, name := range seedUnits {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			panic(err)
		}
	}
	return dir
})

// p0ID is derived the same way the engine derives it, so the fixture cannot
// drift away from the fingerprint rule it is meant to exercise.
func p0ID() string {
	cat := pb.Category_CATEGORY_SEC
	return contracts.FindingID(cat, contracts.Fingerprint(cat, contracts.RepoAnchor(seedPath), seedClaim), false)
}

// seededLedger holds one open P0 whose id matches its own fingerprint.
func seededLedger(t *testing.T, status pb.FindingStatus) *pb.Ledger {
	t.Helper()
	path, claim := seedPath, seedClaim
	cat := pb.Category_CATEGORY_SEC
	id := p0ID()
	ts := timestamppb.New(testTime())
	return &pb.Ledger{
		SchemaMajor: 2,
		Target:      testTarget(t),
		Findings: map[string]*pb.Finding{
			id: {
				Id:                id,
				FingerprintSha256: contracts.Fingerprint(cat, contracts.RepoAnchor(path), claim),
				Category:          cat,
				Location:          &pb.Location{Anchor: contracts.RepoAnchor(path)},
				Risk: &pb.Risk{
					Severity:    pb.Severity_SEVERITY_P0,
					Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
					BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
				},
				Evidence: &pb.Evidence{
					Tier:  pb.EvidenceTier_EVIDENCE_TIER_E2,
					Claim: claim,
					Detail: &pb.Evidence_Citation{Citation: &pb.Citation{
						Anchor: contracts.RepoAnchor(path), Quote: seedQuote,
					}},
					Disproof: heldUp(),
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

// stubCollector resolves a target without touching a filesystem. These tests
// exercise the run, not collection; tests/test-collector.sh drives the real
// collector against real artifacts.
type stubCollector struct{}

func (stubCollector) Collect(ctx context.Context, spec TargetSpec) (*Collected, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(spec.Root + "\x00" + spec.Scope))
	target := &pb.Target{
		Root: spec.Root, Scope: spec.Scope, FingerprintSha256: sum[:],
		Display: spec.Scope, Kind: pb.TargetKind_TARGET_KIND_CODE,
	}
	categories := spec.Categories
	if len(categories) == 0 {
		categories = AllCategories
	}
	return &Collected{
		Target: target,
		Inventory: &pb.TargetInventory{
			SchemaMajor:             contracts.SchemaMajor,
			TargetFingerprintSha256: target.GetFingerprintSha256(),
			Units:                   stubUnits(spec),
		},
		Categories:  categories,
		ContentPath: filepath.Join(spec.Root, spec.Scope),
		UnitDigests: stubDigests(spec),
	}, nil
}

// stubDigests hashes what seedRoot holds, the way a real collector does while
// fingerprinting. Evidence is checked against these, so a collector that
// omitted them would be refused rather than trusted.
func stubDigests(spec TargetSpec) map[string][]byte {
	out := map[string][]byte{}
	for _, u := range stubUnits(spec) {
		body, err := os.ReadFile(filepath.Join(spec.Root, u.GetId()))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(body)
		out[u.GetId()] = sum[:]
	}
	return out
}

// stubUnits reports what seedRoot holds when the spec names it, and the scope
// itself otherwise, so a target pointed somewhere else still has an inventory.
func stubUnits(spec TargetSpec) []*pb.TargetUnit {
	if spec.Root != seedRoot() {
		return []*pb.TargetUnit{{Id: spec.Scope, Label: spec.Scope}}
	}
	out := make([]*pb.TargetUnit, 0, len(seedUnits))
	for _, name := range seedUnits {
		out = append(out, &pb.TargetUnit{Id: name, Label: name})
	}
	return out
}

func testTarget(t *testing.T) *pb.Target {
	t.Helper()
	out, err := stubCollector{}.Collect(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	return out.Target
}

func testSpec() TargetSpec { return TargetSpec{Root: seedRoot(), Scope: "."} }

// report renders provider output carrying the given candidates, answering the
// first iteration.
func report(t *testing.T, candidates ...*pb.CandidateFinding) []byte {
	t.Helper()
	return reportAt(t, 1, candidates...)
}

// reportAt renders provider output for a named iteration. A report answers one
// request, so the iteration it declares is part of what binds it to that one.
func reportAt(t *testing.T, iteration uint32, candidates ...*pb.CandidateFinding) []byte {
	t.Helper()
	r := &pb.ProviderReport{
		SchemaMajor:      contracts.SchemaMajor,
		RunId:            "run-001",
		Target:           testTarget(t),
		Mode:             pb.Mode_MODE_REPORT,
		Iteration:        iteration,
		Candidates:       candidates,
		RoutedCategories: []pb.Category{pb.Category_CATEGORY_SEC, pb.Category_CATEGORY_COR},
		CategoryStops: []*pb.CategoryStop{
			{Category: pb.Category_CATEGORY_SEC, Condition: pb.StopCondition_STOP_CONDITION_MARGINAL_YIELD, ProbesAttempted: 3},
			{Category: pb.Category_CATEGORY_COR, Condition: pb.StopCondition_STOP_CONDITION_MARGINAL_YIELD, ProbesAttempted: 2},
		},
		// stubCollector resolves one unit named after the scope.
		Coverage:            &pb.UnitCoverage{Examined: seedUnits},
		CandidatesExamined:  uint32(len(candidates)) + 3,
		CandidatesDisproved: 3,
		Summary:             "provider report",
	}
	encoded, err := contracts.EncodeCanonical(r)
	if err != nil {
		t.Fatal(err)
	}
	return []byte("prose before\n" + envelope.WrapReport(encoded))
}

// candidate builds a cited finding at the given severity and path.
// heldUp is a disproof that was performed and failed to disprove the finding.
func heldUp() *pb.DisproofAttempt {
	return &pb.DisproofAttempt{Outcome: &pb.DisproofAttempt_Performed{
		Performed: &pb.Reproduction{
			CompletedAt: timestamppb.New(testTime()),
			Exhibit: &pb.Reproduction_ExecutedCommand{ExecutedCommand: &pb.ExecutedCommand{
				Action:   "tried to show the path is unreachable",
				Cwd:      &pb.RepoPath{Value: "."},
				ExitCode: 1,
				Output:   &pb.ExecutedCommand_OutputExcerpt{OutputExcerpt: "still reachable"},
			}},
		},
	}}
}

func candidate(path, claim string, sev pb.Severity) *pb.CandidateFinding {
	return &pb.CandidateFinding{
		Category: pb.Category_CATEGORY_SEC,
		Location: &pb.Location{Anchor: contracts.RepoAnchor(path)},
		Risk: &pb.Risk{
			Severity:    sev,
			Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
			BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
		},
		Evidence: &pb.Evidence{
			Tier:  pb.EvidenceTier_EVIDENCE_TIER_E2,
			Claim: claim,
			Detail: &pb.Evidence_Citation{Citation: &pb.Citation{
				Anchor: contracts.RepoAnchor(path), Quote: seedQuote,
			}},
			// A finding nobody attacked carries no verdict weight, so a fixture
			// about severity has to record the attack that earned it.
			Disproof: heldUp(),
		},
	}
}

// seedCandidate matches the finding seededLedger holds.
func seedCandidate() *pb.CandidateFinding {
	return candidate(seedPath, seedClaim, pb.Severity_SEVERITY_P0)
}

func newEngine(p Provider, l Ledger, s StateStore, a Adjudicator) *Engine {
	return &Engine{
		Collector: stubCollector{}, Provider: p, Broker: StrictBroker{},
		Adjudicator: a, Gate: NotApplicableGate{}, Ledger: l,
		Results: &memResults{}, State: s,
		Clock: func() time.Time { return testTime() },
		RunID: "run-001", AutoLoop: true, MaxIterations: 5,
	}
}

func TestRunEndToEndReportMode(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, s, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetProducerRole() != pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR {
		t.Errorf("producer role = %v, want orchestrator", result.GetProducerRole())
	}
	if result.GetCompletionState() != pb.CompletionState_COMPLETION_STATE_CONTINUE {
		t.Errorf("completion state = %v, want continue at iteration 1 of 5", result.GetCompletionState())
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
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, s, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})
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

// A report has no verdict, colour, counts, or summary field, so the engine
// cannot adopt one. What it can still do is derive the wrong answer, so assert
// the derivation over what the ledger holds.
func TestRunDerivesVerdictFromLedgerNotReport(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetVerdict().GetDecision() != pb.Decision_DECISION_BLOCK {
		t.Errorf("decision = %v, want block for a reported P0", result.GetVerdict().GetDecision())
	}
	if result.GetLegacyColor() != pb.LegacyColor_LEGACY_COLOR_RED {
		t.Errorf("color = %v, want red", result.GetLegacyColor())
	}
	if result.GetRunId() != "run-001" {
		t.Errorf("run id = %q, want the engine's own, not the report's", result.GetRunId())
	}
	if result.GetSummary() == "provider report" {
		t.Error("the engine adopted the report's summary")
	}
	if result.GetProducerRole() != pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR {
		t.Errorf("producer role = %v, want orchestrator", result.GetProducerRole())
	}
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

// The case that was impossible before: a first review against an empty ledger.
func TestRunAdmitsFirstFindings(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, nil)

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("a first review was rejected: %v", err)
	}
	if len(l.ledger.GetFindings()) != 1 {
		t.Fatalf("ledger holds %d findings, want 1", len(l.ledger.GetFindings()))
	}
	if result.GetCounts().GetOpenP0() != 1 {
		t.Errorf("open_p0 = %d, want 1", result.GetCounts().GetOpenP0())
	}

	// Identity, timestamps, and history are the engine's, derived from evidence.
	for id, f := range l.ledger.GetFindings() {
		if id != p0ID() {
			t.Errorf("finding id = %q, want the derived %q", id, p0ID())
		}
		if err := verifyIdentity(f); err != nil {
			t.Errorf("admitted finding fails its own identity check: %v", err)
		}
		if f.GetFirstSeen().AsTime() != testTime() {
			t.Errorf("first_seen = %v, want the engine clock %v", f.GetFirstSeen().AsTime(), testTime())
		}
		if len(f.GetHistory()) != 1 || f.GetHistory()[0].GetTo() != pb.FindingStatus_FINDING_STATUS_OPEN {
			t.Errorf("history = %v, want one opening event", f.GetHistory())
		}
		if len(f.GetEvidenceSha256()) != 32 {
			t.Error("evidence digest was not computed")
		}
	}
}

// The same defect reported twice is one finding, not two.
func TestRunReportingTwiceDoesNotDuplicate(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate(), seedCandidate())}, l, &memState{}, nil)

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(l.ledger.GetFindings()); got != 1 {
		t.Errorf("ledger holds %d findings, want 1", got)
	}
}

// A candidate the contract refuses never reaches the ledger.
func TestRunRejectsInadmissibleCandidate(t *testing.T) {
	bad := seedCandidate()
	// An inferred P0: refused by the tier cap on CandidateFinding.
	bad.Evidence = &pb.Evidence{
		Tier:  pb.EvidenceTier_EVIDENCE_TIER_E3,
		Claim: "probably unvalidated",
		Detail: &pb.Evidence_Inference{Inference: &pb.Inference{
			Assumption: "callers pass raw paths", Reasoning: "none seen", Falsifier: "a validating caller",
		}},
	}
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: unvalidatedReport(t, bad)}, l, &memState{}, nil)

	_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if !errors.Is(err, ErrProviderOutput) {
		t.Fatalf("error = %v, want ErrProviderOutput", err)
	}
	if l.saves != 0 {
		t.Error("an inadmissible candidate reached a saved ledger")
	}
}

// unvalidatedReport marshals without validating, which is the only way to
// produce what a hostile or broken provider would actually send. report() goes
// through EncodeCanonical and so cannot build one.
func unvalidatedReport(t *testing.T, candidates ...*pb.CandidateFinding) []byte {
	t.Helper()
	r := &pb.ProviderReport{
		SchemaMajor: contracts.SchemaMajor,
		RunId:       "run-001",
		Target:      testTarget(t),
		Mode:        pb.Mode_MODE_REPORT,
		Iteration:   1,
		Candidates:  candidates,
		Summary:     "provider report",
	}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(envelope.WrapReport(encoded))
}

func TestRunStaleStateFailsClosed(t *testing.T) {
	otherCollected, err := stubCollector{}.Collect(context.Background(), TargetSpec{Root: "/repo", Scope: "somewhere-else"})
	if err != nil {
		t.Fatal(err)
	}
	other := otherCollected.Target
	s := &memState{state: &pb.LoopState{
		SchemaMajor: 2, RunId: "run-000", Target: other,
		Mode: pb.Mode_MODE_REPORT, Iteration: 1, MaxIterations: 5,
		LedgerDigestSha256: make([]byte, 32), LastResultSha256: make([]byte, 32),
	}}
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, s, nil)

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
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, s, nil)

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err == nil {
		t.Fatal("want an error past the iteration limit")
	}
}

func TestRunCancelledContextWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	p := &fakeProvider{out: report(t, seedCandidate())}
	e := newEngine(p, l, s, nil)

	if _, err := e.Run(ctx, testSpec(), pb.Mode_MODE_REPORT); err == nil {
		t.Fatal("want an error for a cancelled run")
	}
	if l.saves != 0 || s.saves != 0 {
		t.Errorf("cancelled run persisted state (ledger %d, state %d)", l.saves, s.saves)
	}
}

// A run that was not asked to loop must leave no loop state, or a stop hook
// reads it and asks for an iteration the caller never requested.
func TestRunWithoutAutoLoopLeavesNoState(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	results := &memResults{}
	// Seeded with state from an earlier run: starting from nil would let an
	// implementation that never clears anything satisfy the assertions below.
	s.state = &pb.LoopState{
		SchemaMajor: contracts.SchemaMajor, RunId: "run-000", Target: testTarget(t),
		Mode: pb.Mode_MODE_REPORT, Iteration: 1, MaxIterations: 5,
		LedgerDigestSha256: make([]byte, 32), LastResultSha256: make([]byte, 32),
	}
	// The seeded state puts this run at iteration 2, and a report answers the
	// iteration it was asked for.
	e := newEngine(&fakeProvider{out: reportAt(t, 2, seedCandidate())}, l, s, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})
	e.Results = results
	e.AutoLoop = false

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.saves != 0 {
		t.Errorf("state saves = %d, want 0 without auto-loop", s.saves)
	}
	if s.clears != 1 {
		t.Errorf("state clears = %d, want 1; stale state has to be discarded, not merely not written", s.clears)
	}
	if s.state != nil {
		t.Error("a one-shot run left loop state behind")
	}
	// The result is still recorded; only the loop's claim on the session is not.
	if results.saves != 1 {
		t.Errorf("result saves = %d, want 1", results.saves)
	}
}

// A ledger is pinned to one path by the contract, so a second target in the
// same directory must not inherit the first target's findings.
func TestRunRejectsLedgerForAnotherTarget(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	s := &memState{}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, s, nil)

	other := TargetSpec{Root: "/repo", Scope: "somewhere-else"}
	_, err := e.Run(context.Background(), other, pb.Mode_MODE_REPORT)
	if !errors.Is(err, ErrLedgerTarget) {
		t.Fatalf("error = %v, want ErrLedgerTarget", err)
	}
	if l.saves != 0 {
		t.Error("the other target's ledger was written")
	}
	if s.saves != 0 {
		t.Error("state was written for a rejected run")
	}
}

// An empty ledger adopts whatever target it is first used for.
func TestRunAdoptsEmptyLedger(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 2}}
	e := newEngine(&fakeProvider{out: report(t)}, l, &memState{}, nil)

	if _, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if l.ledger.GetTarget().GetScope() != testSpec().Scope {
		t.Errorf("ledger target = %q, want %q", l.ledger.GetTarget().GetScope(), testSpec().Scope)
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
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 2, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t)}, l, &memState{}, nil)

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
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 2, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t)}, l, &memState{}, fixedAdjudicator{err: errors.New("unreachable")})

	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetVerdict().GetDecision() == pb.Decision_DECISION_PASS {
		t.Error("a failed adjudication was treated as agreement")
	}
}

func TestRunIndependentBlockOverridesClean(t *testing.T) {
	l := &memLedger{ledger: &pb.Ledger{SchemaMajor: 2, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t)}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_BLOCK})

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

// A report has no oscillation field, so a provider cannot claim one. Oscillation
// comes from what the ledger records happening.
func TestRunOscillationComesFromTheLedger(t *testing.T) {
	clean := &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, clean, &memState{}, nil)
	result, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.GetLedger().GetOscillationDetected() {
		t.Error("a first sighting was recorded as oscillation")
	}
}

// A fingerprint already fixed reappearing is a regression, which makes a pass
// unreachable for the run.
func TestRunRegressionSetsOscillation(t *testing.T) {
	l := &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_FIXED)}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})

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
		e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, fixedAdjudicator{decision: pb.Decision_DECISION_PASS})
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

// Marginal yield decides when the loop stops, so it is derived from what the
// ledger did not already hold rather than taken from the report.
func TestRunDerivesMarginalYield(t *testing.T) {
	empty := func() *memLedger {
		return &memLedger{ledger: &pb.Ledger{SchemaMajor: contracts.SchemaMajor, Target: testTarget(t)}}
	}

	// A new P0 counts.
	l := empty()
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, nil)
	r, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := r.GetMarginalYield().GetNewP0P1(); got != 1 {
		t.Errorf("new_p0_p1 = %d, want 1 for a newly admitted P0", got)
	}

	// A P2 counts separately, and not toward the loop's stopping rule.
	l = empty()
	e = newEngine(&fakeProvider{out: report(t, candidate("slow.sh", "quadratic scan", pb.Severity_SEVERITY_P2))}, l, &memState{}, nil)
	r, err = e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := r.GetMarginalYield().GetNewP0P1(); got != 0 {
		t.Errorf("new_p0_p1 = %d, want 0 for a P2", got)
	}
	if got := r.GetMarginalYield().GetNewP2P3(); got != 1 {
		t.Errorf("new_p2_p3 = %d, want 1", got)
	}

	// A finding the ledger already holds is not new, however it is reported.
	l = &memLedger{ledger: seededLedger(t, pb.FindingStatus_FINDING_STATUS_OPEN)}
	e = newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, nil)
	r, err = e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := r.GetMarginalYield().GetNewP0P1(); got != 0 {
		t.Errorf("new_p0_p1 = %d, want 0 for a re-reported finding", got)
	}

	// The provider's self-grind arithmetic passes through untouched.
	if got := r.GetMarginalYield().GetCandidatesExamined(); got != 4 {
		t.Errorf("candidates_examined = %d, want the reported 4", got)
	}
}

// A report answers one request. These are the fields that say which, each
// defeated on its own, because the adapter's documented `cat` of a file on disk
// makes a stale answer the ordinary case rather than the adversarial one.
func TestRunRejectsAReportForAnotherRequest(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*pb.ProviderReport)
	}{
		{"another run", func(r *pb.ProviderReport) { r.RunId = "run-999" }},
		{"another target", func(r *pb.ProviderReport) {
			r.Target = &pb.Target{
				Root: "/repo", Scope: "elsewhere",
				FingerprintSha256: bytesOf(0xbb), Kind: pb.TargetKind_TARGET_KIND_CODE,
			}
		}},
		{"another iteration", func(r *pb.ProviderReport) { r.Iteration = 4 }},
		{"another contract major", func(r *pb.ProviderReport) { r.SchemaMajor = 3 }},
	} {
		raw := report(t, seedCandidate())
		decoded := &pb.ProviderReport{}
		body, err := envelope.ExtractReport(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := contracts.UnmarshalCanonical(body, decoded); err != nil {
			t.Fatal(err)
		}
		c.mutate(decoded)
		if why := reportBinding(decoded, "run-001", testTarget(t), pb.Mode_MODE_REPORT, 1); why == "" {
			t.Errorf("%s: a report for another request was accepted", c.name)
		}
	}

	// The control: an unmutated report is the answer to this request.
	raw := report(t, seedCandidate())
	body, err := envelope.ExtractReport(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	decoded := &pb.ProviderReport{}
	if err := contracts.UnmarshalCanonical(body, decoded); err != nil {
		t.Fatal(err)
	}
	if why := reportBinding(decoded, "run-001", testTarget(t), pb.Mode_MODE_REPORT, 1); why != "" {
		t.Errorf("a matching report was rejected: %s", why)
	}
}

// The binding has to be applied, not merely available: a stale report reaching
// the ledger is the failure fg-64y.26 describes.
func TestRunRejectsAStaleReport(t *testing.T) {
	l := &memLedger{}
	// A report for iteration 4 handed to a run at iteration 1.
	e := newEngine(&fakeProvider{out: reportAt(t, 4, seedCandidate())}, l, &memState{}, nil)
	_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if err == nil {
		t.Fatal("a report for another iteration was accepted")
	}
	if !errors.Is(err, ErrProviderOutput) {
		t.Errorf("err = %v, want a provider-output rejection", err)
	}
	if l.saves != 0 {
		t.Errorf("a rejected report still wrote the ledger %d times", l.saves)
	}
}

// The broker decides whether a candidate's evidence earns the severity it
// claims. It was dead code until a provider could report findings at all, so
// this pins that it is consulted: a candidate it refuses must not reach the
// ledger, whatever the report says.
type refusingBroker struct{ called bool }

func (b *refusingBroker) Admit(context.Context, *pb.CandidateFinding, *Collected) (*pb.CandidateFinding, error) {
	b.called = true
	return nil, fmt.Errorf("%w: refused by the broker", ErrProviderOutput)
}

func TestRunConsultsTheEvidenceBroker(t *testing.T) {
	l := &memLedger{}
	b := &refusingBroker{}
	e := newEngine(&fakeProvider{out: report(t, seedCandidate())}, l, &memState{}, nil)
	e.Broker = b

	_, err := e.Run(context.Background(), testSpec(), pb.Mode_MODE_REPORT)
	if !b.called {
		t.Fatal("the broker was never consulted")
	}
	if err == nil {
		t.Fatal("a refused candidate was admitted anyway")
	}
	if l.saves != 0 {
		t.Errorf("a refused candidate still wrote the ledger %d times", l.saves)
	}
}
