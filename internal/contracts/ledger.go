package contracts

import (
	"bytes"
	"fmt"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// allowedTransitions is the whole lifecycle. Anything absent is rejected;
// direct file mutation is not a supported path to any of these states.
var allowedTransitions = map[pb.FindingStatus][]pb.FindingStatus{
	pb.FindingStatus_FINDING_STATUS_OPEN: {
		pb.FindingStatus_FINDING_STATUS_FIXED,
		pb.FindingStatus_FINDING_STATUS_ACCEPTED,
		pb.FindingStatus_FINDING_STATUS_FALSE_POSITIVE,
	},
	pb.FindingStatus_FINDING_STATUS_FIXED: {
		pb.FindingStatus_FINDING_STATUS_VERIFIED,
		pb.FindingStatus_FINDING_STATUS_REGRESSED,
	},
	pb.FindingStatus_FINDING_STATUS_VERIFIED: {
		pb.FindingStatus_FINDING_STATUS_REGRESSED,
	},
	pb.FindingStatus_FINDING_STATUS_REGRESSED: {
		pb.FindingStatus_FINDING_STATUS_FIXED,
		pb.FindingStatus_FINDING_STATUS_ACCEPTED,
	},
	// Reopening a closed finding requires the evidence to have changed;
	// TransitionOpts enforces that, since the state pair alone cannot.
	pb.FindingStatus_FINDING_STATUS_ACCEPTED: {
		pb.FindingStatus_FINDING_STATUS_OPEN,
	},
	pb.FindingStatus_FINDING_STATUS_FALSE_POSITIVE: {
		pb.FindingStatus_FINDING_STATUS_OPEN,
	},
}

// TransitionOpts carries what a state change needs beyond the target state.
type TransitionOpts struct {
	Iteration      uint32
	Actor          string
	NewEvidenceSum []byte
	Owner          *pb.HumanOwner
}

// Transition applies one lifecycle change to one finding.
//
// Reappearance is the case that matters: a fingerprint that was fixed or
// verified showing up again is a regression, not a new finding, and it sets the
// run-level oscillation flag that makes GREEN unreachable. A loop that keeps
// re-breaking what it just fixed should not be able to declare success on the
// iteration where the damage happens to be invisible.
func Transition(l *pb.Ledger, id string, to pb.FindingStatus, opts TransitionOpts) (oscillation bool, err error) {
	f, ok := l.GetFindings()[id]
	if !ok {
		return false, fmt.Errorf("finding %s not in ledger", id)
	}
	from := f.GetStatus()

	if to == pb.FindingStatus_FINDING_STATUS_ACCEPTED && opts.Owner == nil && f.GetOwner() == nil {
		return false, fmt.Errorf("accepting %s requires a human owner with rationale and review deadline", id)
	}

	if (from == pb.FindingStatus_FINDING_STATUS_ACCEPTED || from == pb.FindingStatus_FINDING_STATUS_FALSE_POSITIVE) &&
		to == pb.FindingStatus_FINDING_STATUS_OPEN {
		if len(opts.NewEvidenceSum) == 0 || bytes.Equal(opts.NewEvidenceSum, f.GetEvidenceSha256()) {
			return false, fmt.Errorf("reopening %s requires changed evidence", id)
		}
	}

	permitted := false
	for _, cand := range allowedTransitions[from] {
		if cand == to {
			permitted = true
			break
		}
	}
	if !permitted {
		return false, fmt.Errorf("illegal transition for %s: %s -> %s", id, shortStatus(from), shortStatus(to))
	}

	if to == pb.FindingStatus_FINDING_STATUS_REGRESSED &&
		(from == pb.FindingStatus_FINDING_STATUS_FIXED || from == pb.FindingStatus_FINDING_STATUS_VERIFIED) {
		oscillation = true
	}

	if opts.Owner != nil {
		f.Owner = opts.Owner
	}
	if len(opts.NewEvidenceSum) > 0 {
		f.EvidenceSha256 = opts.NewEvidenceSum
	}
	f.Status = to
	f.History = append(f.History, &pb.FindingEvent{
		Iteration:      opts.Iteration,
		From:           from,
		To:             to,
		EvidenceSha256: f.GetEvidenceSha256(),
		At:             nowTimestamp(),
		Actor:          opts.Actor,
	})

	if err := Validate(l); err != nil {
		return oscillation, fmt.Errorf("ledger invalid after transition: %w", err)
	}
	return oscillation, nil
}

// Observe records a fingerprint seen this iteration. A fingerprint already
// marked fixed or verified comes back as regressed.
func Observe(l *pb.Ledger, id string, iteration uint32, actor string) (oscillation bool, err error) {
	f, ok := l.GetFindings()[id]
	if !ok {
		return false, fmt.Errorf("finding %s not in ledger", id)
	}
	switch f.GetStatus() {
	case pb.FindingStatus_FINDING_STATUS_FIXED, pb.FindingStatus_FINDING_STATUS_VERIFIED:
		return Transition(l, id, pb.FindingStatus_FINDING_STATUS_REGRESSED, TransitionOpts{
			Iteration: iteration,
			Actor:     actor,
		})
	default:
		f.LastSeen = nowTimestamp()
		return false, nil
	}
}

func shortStatus(s pb.FindingStatus) string {
	return trimPrefix(s.String(), "FINDING_STATUS_")
}

func trimPrefix(s, p string) string {
	if len(s) > len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return s
}
