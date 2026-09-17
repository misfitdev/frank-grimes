package contracts

import (
	"bytes"
	"strings"
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Transition's gate rule is asserted here rather than through the CLI because
// the property is over the whole status space: every state a finding can be in,
// against every state it can be moved to. The CLI asserts the cases an operator
// can reach.

func ledgerWith(status pb.FindingStatus) (*pb.Ledger, string) {
	const id = "FG-SEC-0123456789ab"
	now := timestamppb.Now()
	digest := bytes.Repeat([]byte{7}, 32)
	return &pb.Ledger{
		SchemaMajor: 2,
		Target: &pb.Target{
			Root: "/repo", Scope: "app.sh", Kind: pb.TargetKind_TARGET_KIND_CODE,
			FingerprintSha256: bytes.Repeat([]byte{1}, 32),
		},
		Findings: map[string]*pb.Finding{
			id: {
				Id:                id,
				FingerprintSha256: digest,
				Category:          pb.Category_CATEGORY_SEC,
				Location: &pb.Location{Anchor: &pb.Anchor{At: &pb.Anchor_RepoLine{
					RepoLine: &pb.RepoLine{Path: &pb.RepoPath{Value: "app.sh"}},
				}}},
				Risk: &pb.Risk{
					Severity: pb.Severity_SEVERITY_P2, Likelihood: pb.Likelihood_LIKELIHOOD_UNLIKELY,
					BlastRadius: pb.BlastRadius_BLAST_RADIUS_LOCAL_COMPONENT,
				},
				Evidence: &pb.Evidence{
					Tier: pb.EvidenceTier_EVIDENCE_TIER_E2, Claim: "a deletion path",
					Detail: &pb.Evidence_Citation{Citation: &pb.Citation{
						Quote: "rm -rf", Anchor: &pb.Anchor{At: &pb.Anchor_RepoLine{
							RepoLine: &pb.RepoLine{Path: &pb.RepoPath{Value: "app.sh"}},
						}},
					}},
				},
				EvidenceSha256: digest,
				Status:         status,
				FirstSeen:      now,
				LastSeen:       now,
				History: []*pb.FindingEvent{{
					Iteration: 1, To: status, EvidenceSha256: digest, At: now, Actor: "test",
				}},
			},
		},
	}, id
}

// Every route into verified needs a gate; no route out of anywhere else may
// carry one.
func TestOnlyAGateCanVerifyAFinding(t *testing.T) {
	every := []pb.FindingStatus{
		pb.FindingStatus_FINDING_STATUS_OPEN,
		pb.FindingStatus_FINDING_STATUS_FIXED,
		pb.FindingStatus_FINDING_STATUS_VERIFIED,
		pb.FindingStatus_FINDING_STATUS_ACCEPTED,
		pb.FindingStatus_FINDING_STATUS_FALSE_POSITIVE,
		pb.FindingStatus_FINDING_STATUS_REGRESSED,
		pb.FindingStatus_FINDING_STATUS_SUPERSEDED,
	}
	gate := bytes.Repeat([]byte{9}, 32)

	for _, from := range every {
		for _, to := range every {
			if !permitted(from, to) {
				continue
			}
			l, id := ledgerWith(from)
			_, withoutGate := Transition(l, id, to, TransitionOpts{Iteration: 2, Actor: "test"})

			l, id = ledgerWith(from)
			_, withGate := Transition(l, id, to, TransitionOpts{
				Iteration: 2, Actor: "test", VerifiedBySum: gate,
			})

			if to == pb.FindingStatus_FINDING_STATUS_VERIFIED {
				if withoutGate == nil {
					t.Errorf("%v -> verified was allowed with no gate", from)
				} else if !strings.Contains(withoutGate.Error(), "gate that passed") {
					t.Errorf("%v -> verified refused for the wrong reason: %v", from, withoutGate)
				}
				if withGate != nil {
					t.Errorf("%v -> verified refused a passing gate: %v", from, withGate)
				}
				continue
			}
			if withGate == nil {
				t.Errorf("%v -> %v carried a gate digest and was allowed", from, to)
			}
		}
	}
}

// Accepting needs an owner, which the table above cannot supply, so it is
// asserted on its own rather than special-cased there.
func TestAGateDigestIsRefusedOnAnAcceptance(t *testing.T) {
	l, id := ledgerWith(pb.FindingStatus_FINDING_STATUS_OPEN)
	now := timestamppb.Now()

	_, err := Transition(l, id, pb.FindingStatus_FINDING_STATUS_ACCEPTED, TransitionOpts{
		Iteration: 2, Actor: "test",
		Owner:         &pb.HumanOwner{Id: "tucker", Rationale: "known", RecordedAt: now, ReviewBy: now},
		VerifiedBySum: bytes.Repeat([]byte{9}, 32),
	})

	if err == nil {
		t.Fatal("an acceptance carried a gate digest")
	}
}

func TestAVerifiedFindingRecordsTheGateThatPassedOverIt(t *testing.T) {
	l, id := ledgerWith(pb.FindingStatus_FINDING_STATUS_FIXED)
	gate := bytes.Repeat([]byte{9}, 32)

	if _, err := Transition(l, id, pb.FindingStatus_FINDING_STATUS_VERIFIED, TransitionOpts{
		Iteration: 2, Actor: "test", VerifiedBySum: gate,
	}); err != nil {
		t.Fatal(err)
	}

	history := l.GetFindings()[id].GetHistory()
	last := history[len(history)-1]
	if !bytes.Equal(last.GetVerifiedBySha256(), gate) {
		t.Errorf("the event does not carry the gate: %x", last.GetVerifiedBySha256())
	}
}

// A digest of the wrong size is not a digest. Without this, a caller could pass
// anything non-empty and satisfy the requirement.
func TestAGateDigestOfTheWrongSizeIsRefused(t *testing.T) {
	l, id := ledgerWith(pb.FindingStatus_FINDING_STATUS_FIXED)

	_, err := Transition(l, id, pb.FindingStatus_FINDING_STATUS_VERIFIED, TransitionOpts{
		Iteration: 2, Actor: "test", VerifiedBySum: []byte("short"),
	})

	if err == nil {
		t.Fatal("a five-byte gate digest verified a finding")
	}
}

// A refused transition has to leave the finding alone. Validation runs over the
// ledger this already changed, so a caller that retries with a gate digest
// would otherwise be refused for the state the first attempt left behind.
func TestARefusedTransitionChangesNothing(t *testing.T) {
	l, id := ledgerWith(pb.FindingStatus_FINDING_STATUS_FIXED)
	before := l.GetFindings()[id]
	events := len(before.GetHistory())

	if _, err := Transition(l, id, pb.FindingStatus_FINDING_STATUS_VERIFIED,
		TransitionOpts{Iteration: 2, Actor: "test"}); err == nil {
		t.Fatal("a verification with no gate was allowed")
	}

	after := l.GetFindings()[id]
	if after.GetStatus() != pb.FindingStatus_FINDING_STATUS_FIXED {
		t.Errorf("status = %v after a refusal, want fixed", after.GetStatus())
	}
	if len(after.GetHistory()) != events {
		t.Errorf("history grew by %d on a refused transition", len(after.GetHistory())-events)
	}

	// The retry is the point: it has to be refused or allowed on its own merits.
	if _, err := Transition(l, id, pb.FindingStatus_FINDING_STATUS_VERIFIED, TransitionOpts{
		Iteration: 2, Actor: "test", VerifiedBySum: bytes.Repeat([]byte{9}, 32),
	}); err != nil {
		t.Errorf("the retry with a gate was refused: %v", err)
	}
}

func permitted(from, to pb.FindingStatus) bool {
	for _, cand := range allowedTransitions[from] {
		if cand == to {
			return true
		}
	}
	return false
}
