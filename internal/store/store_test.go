package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func digest32() []byte { return make([]byte, 32) }

func target() *pb.Target {
	return &pb.Target{Root: "/repo", Scope: "src", FingerprintSha256: digest32()}
}

func sampleLedger() *pb.Ledger {
	ts := timestamppb.New(timeZero())
	return &pb.Ledger{
		SchemaMajor: 1,
		Target:      target(),
		Findings: map[string]*pb.Finding{
			"FG-SEC-9a34db214c59": {
				Id:                "FG-SEC-9a34db214c59",
				FingerprintSha256: digest32(),
				Category:          pb.Category_CATEGORY_SEC,
				Location:          &pb.Location{Path: &pb.RepoPath{Value: "bad.sh"}},
				Risk: &pb.Risk{
					Severity:    pb.Severity_SEVERITY_P0,
					Likelihood:  pb.Likelihood_LIKELIHOOD_LIKELY,
					BlastRadius: pb.BlastRadius_BLAST_RADIUS_SYSTEMIC,
				},
				Evidence: &pb.Evidence{
					Tier:  pb.EvidenceTier_EVIDENCE_TIER_E2,
					Claim: "caller-controlled deletion",
					Detail: &pb.Evidence_Citation{Citation: &pb.Citation{
						Path: &pb.RepoPath{Value: "bad.sh"}, Line: 22, Quote: "rm -rf \"$1\"/*",
					}},
				},
				EvidenceSha256: digest32(),
				Status:         pb.FindingStatus_FINDING_STATUS_OPEN,
				FirstSeen:      ts,
				LastSeen:       ts,
				History: []*pb.FindingEvent{{
					Iteration:      1,
					To:             pb.FindingStatus_FINDING_STATUS_OPEN,
					EvidenceSha256: digest32(),
					At:             ts,
					Actor:          "grimes",
				}},
			},
		},
	}
}

func sampleState() *pb.LoopState {
	return &pb.LoopState{
		SchemaMajor:        2,
		RunId:              "run-001",
		Target:             target(),
		Mode:               pb.Mode_MODE_REPORT,
		Iteration:          1,
		MaxIterations:      5,
		LedgerDigestSha256: digest32(),
		LastResultSha256:   digest32(),
	}
}

func TestFileLedgerLoadMissingReturnsEmpty(t *testing.T) {
	l := NewFileLedger(t.TempDir())
	got, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.GetFindings()) != 0 {
		t.Errorf("findings = %d, want 0", len(got.GetFindings()))
	}
}

func TestFileLedgerRoundTrip(t *testing.T) {
	l := NewFileLedger(t.TempDir())
	want := sampleLedger()
	sum, err := l.Save(context.Background(), want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(sum) != 32 {
		t.Errorf("digest length = %d, want 32", len(sum))
	}
	got, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Error("round-tripped ledger differs from what was saved")
	}
}

func TestFileLedgerSaveIsCanonical(t *testing.T) {
	l := NewFileLedger(t.TempDir())
	if _, err := l.Save(context.Background(), sampleLedger()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(l.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := contracts.UnmarshalCanonical(data, &pb.Ledger{}); err != nil {
		t.Errorf("saved bytes are not canonical: %v", err)
	}
}

func TestFileLedgerDigestMatchesContents(t *testing.T) {
	l := NewFileLedger(t.TempDir())
	ledger := sampleLedger()
	sum, err := l.Save(context.Background(), ledger)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want, err := contracts.Digest(ledger)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if string(sum) != string(want) {
		t.Error("returned digest does not match the saved ledger")
	}
}

func TestFileLedgerCorruptQuarantinesAndFails(t *testing.T) {
	dir := t.TempDir()
	l := NewFileLedger(dir)
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.Path, []byte("not a protobuf"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := l.Load(context.Background())
	if err == nil {
		t.Fatal("want an error for a corrupt ledger")
	}
	if !strings.Contains(err.Error(), "quarantined") {
		t.Errorf("error = %v, want it to report quarantining", err)
	}
	if _, statErr := os.Stat(l.Path); statErr == nil {
		t.Error("corrupt ledger was left in place")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".grimes", "quarantine", "*"))
	if len(matches) == 0 {
		t.Error("corrupt ledger was not preserved in quarantine")
	}
}

func TestFileLedgerRejectsInvalidOnSave(t *testing.T) {
	l := NewFileLedger(t.TempDir())
	// schema_major is const 1; 7 cannot be persisted.
	if _, err := l.Save(context.Background(), &pb.Ledger{SchemaMajor: 7, Target: target()}); err == nil {
		t.Fatal("want an error saving a contract-invalid ledger")
	}
	if _, err := os.Stat(l.Path); err == nil {
		t.Error("an invalid ledger was written to disk")
	}
}

func TestFileStateStoreLoadMissingReturnsNil(t *testing.T) {
	s := NewFileStateStore(t.TempDir())
	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("state = %v, want nil", got)
	}
}

func TestFileStateStoreRoundTrip(t *testing.T) {
	s := NewFileStateStore(t.TempDir())
	want := sampleState()
	if err := s.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Error("round-tripped state differs from what was saved")
	}
}

func TestFileStateStoreCorruptQuarantines(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStateStore(dir)
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(context.Background()); err == nil {
		t.Fatal("want an error for corrupt loop state")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".grimes", "quarantine", "*"))
	if len(matches) == 0 {
		t.Error("corrupt state was not preserved in quarantine")
	}
}

func TestFileStateStoreClearIsIdempotent(t *testing.T) {
	s := NewFileStateStore(t.TempDir())
	ctx := context.Background()
	if err := s.Clear(ctx); err != nil {
		t.Errorf("Clear on absent state: %v", err)
	}
	if err := s.Save(ctx, sampleState()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Clear(ctx); err != nil {
		t.Errorf("Clear: %v", err)
	}
	if err := s.Clear(ctx); err != nil {
		t.Errorf("second Clear: %v", err)
	}
	got, err := s.Load(ctx)
	if err != nil || got != nil {
		t.Errorf("Load after Clear = %v, %v; want nil, nil", got, err)
	}
}

// The engine's state must not collide with the file hooks/stop.sh owns.
func TestFileStateStoreDoesNotUseHookStateFile(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStateStore(dir)
	if err := s.Save(context.Background(), sampleState()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".grimes-state.json")); err == nil {
		t.Error("engine wrote the hook's state file")
	}
}

func TestStoreRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if _, err := NewFileLedger(dir).Load(ctx); err == nil {
		t.Error("ledger Load ignored a cancelled context")
	}
	if _, err := NewFileLedger(dir).Save(ctx, sampleLedger()); err == nil {
		t.Error("ledger Save ignored a cancelled context")
	}
	if _, err := NewFileStateStore(dir).Load(ctx); err == nil {
		t.Error("state Load ignored a cancelled context")
	}
	if err := NewFileStateStore(dir).Save(ctx, sampleState()); err == nil {
		t.Error("state Save ignored a cancelled context")
	}
}

func timeZero() time.Time { return time.Unix(1780000000, 0).UTC() }

func sampleResult() *pb.GrimesResult {
	digest := digest32()
	return &pb.GrimesResult{
		SchemaMajor:     2,
		RunId:           "run-001",
		ProducerRole:    pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:          target(),
		Mode:            pb.Mode_MODE_REPORT,
		Iteration:       1,
		MaxIterations:   5,
		CompletionState: pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE,
		Verdict: &pb.Verdict{
			Decision:           pb.Decision_DECISION_BLOCK,
			ResidualRisk:       pb.ResidualRisk_RESIDUAL_RISK_CRITICAL,
			ReviewConfidence:   pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH,
			ReviewCompleteness: pb.ReviewCompleteness_REVIEW_COMPLETENESS_LIMITED,
		},
		LegacyColor:   pb.LegacyColor_LEGACY_COLOR_RED,
		MarginalYield: &pb.MarginalYield{CandidatesExamined: 4, NewP0P1: 2},
		Counts:        &pb.FindingCounts{Total: 1, OpenP0: 1},
		Verification:  &pb.Verification{Status: pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE},
		Ledger:        &pb.LedgerRef{Path: contracts.LedgerPath, DigestSha256: digest},
		UnmetGates:    []string{"decision"},
		Summary:       "An open P0 blocks this target.",
	}
}

func TestFileResultStoreLoadMissingReturnsNil(t *testing.T) {
	got, err := NewFileResultStore(t.TempDir()).Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("result = %v, want nil", got)
	}
}

func TestFileResultStoreRoundTrip(t *testing.T) {
	r := NewFileResultStore(t.TempDir())
	want := sampleResult()
	sum, err := r.Save(context.Background(), want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(sum) != 32 {
		t.Errorf("digest length = %d, want 32", len(sum))
	}
	got, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Error("round-tripped result differs from what was saved")
	}
}

func TestFileResultStoreDigestMatchesContents(t *testing.T) {
	r := NewFileResultStore(t.TempDir())
	result := sampleResult()
	sum, err := r.Save(context.Background(), result)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want, err := contracts.Digest(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(sum) != string(want) {
		t.Error("returned digest does not match the saved result")
	}
}

func TestFileResultStoreCorruptQuarantines(t *testing.T) {
	dir := t.TempDir()
	r := NewFileResultStore(dir)
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Path, []byte("not a protobuf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Load(context.Background()); err == nil {
		t.Fatal("want an error for a corrupt result")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".grimes", "quarantine", "*"))
	if len(matches) == 0 {
		t.Error("corrupt result was not preserved in quarantine")
	}
}

func TestFileResultStoreRejectsInvalidOnSave(t *testing.T) {
	r := NewFileResultStore(t.TempDir())
	bad := sampleResult()
	// Only the orchestrator may emit a result.
	bad.ProducerRole = pb.ProducerRole_PRODUCER_ROLE_GRINDER
	if _, err := r.Save(context.Background(), bad); err == nil {
		t.Fatal("want an error saving a contract-invalid result")
	}
	if _, err := os.Stat(r.Path); err == nil {
		t.Error("an invalid result was written to disk")
	}
}
