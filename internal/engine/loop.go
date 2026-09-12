package engine

import (
	"bytes"
	"fmt"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// LoopOutcome is what the loop decided to do.
type LoopOutcome int

const (
	// OutcomeNoRun means no review is in progress.
	OutcomeNoRun LoopOutcome = iota
	// OutcomeContinue means another iteration is owed.
	OutcomeContinue
	// OutcomeConfirmedPass is the only successful terminal outcome.
	OutcomeConfirmedPass
	// OutcomeIterationLimit means the bound was reached without a pass.
	OutcomeIterationLimit
	// OutcomeYieldExhausted means an iteration surfaced nothing new.
	OutcomeYieldExhausted
	// OutcomeUnverified means the run record could not be trusted.
	OutcomeUnverified
)

func (o LoopOutcome) String() string {
	switch o {
	case OutcomeNoRun:
		return "no_run"
	case OutcomeContinue:
		return "continue"
	case OutcomeConfirmedPass:
		return "confirmed_pass"
	case OutcomeIterationLimit:
		return "iteration_limit"
	case OutcomeYieldExhausted:
		return "yield_exhausted"
	default:
		return "unverified"
	}
}

// Terminal reports whether the loop ends here.
func (o LoopOutcome) Terminal() bool { return o != OutcomeContinue && o != OutcomeNoRun }

// LoopDecision is the outcome, why, and the colour it was reached at.
type LoopDecision struct {
	Outcome LoopOutcome
	Reason  string
	Color   string
}

// DecideLoop decides whether a review may end, from a run record it verifies
// rather than a verdict it is told.
//
// The binding checks are the point: a result is only this run's result if its
// digest is the one the state recorded, its run identity matches, and its
// target fingerprint is the artifact the state was raised against. Any break in
// that chain is unverified, which is never a pass.
func DecideLoop(state *pb.LoopState, result *pb.GrimesResult, resultDigest []byte) LoopDecision {
	if state == nil {
		return LoopDecision{Outcome: OutcomeNoRun, Reason: "no review in progress"}
	}
	if result == nil {
		return LoopDecision{Outcome: OutcomeUnverified, Reason: "loop state references a result that is not on disk"}
	}
	if reason := bindingFailure(state, result, resultDigest); reason != "" {
		return LoopDecision{Outcome: OutcomeUnverified, Reason: reason}
	}

	color := colorName(result)
	if result.GetLegacyColor() == pb.LegacyColor_LEGACY_COLOR_GREEN {
		return LoopDecision{Outcome: OutcomeConfirmedPass, Reason: "independently confirmed pass", Color: color}
	}
	if state.GetIteration() >= state.GetMaxIterations() {
		return LoopDecision{
			Outcome: OutcomeIterationLimit,
			Reason:  fmt.Sprintf("iteration limit of %d reached at %s", state.GetMaxIterations(), color),
			Color:   color,
		}
	}
	// Re-grinding a target that produced nothing new restates the same findings.
	if state.GetIteration() > 1 && result.GetMarginalYield().GetNewP0P1() == 0 {
		return LoopDecision{
			Outcome: OutcomeYieldExhausted,
			Reason:  fmt.Sprintf("iteration %d surfaced no new P0/P1", state.GetIteration()),
			Color:   color,
		}
	}
	return LoopDecision{
		Outcome: OutcomeContinue,
		Reason:  fmt.Sprintf("%s at iteration %d of %d", color, state.GetIteration(), state.GetMaxIterations()),
		Color:   color,
	}
}

func bindingFailure(state *pb.LoopState, result *pb.GrimesResult, resultDigest []byte) string {
	if !bytes.Equal(resultDigest, state.GetLastResultSha256()) {
		return "result digest does not match the one loop state recorded"
	}
	if result.GetRunId() != state.GetRunId() {
		return fmt.Sprintf("result belongs to run %q, state to run %q", result.GetRunId(), state.GetRunId())
	}
	if !bytes.Equal(result.GetTarget().GetFingerprintSha256(), state.GetTarget().GetFingerprintSha256()) {
		return "result reviewed a different target than the state was raised against"
	}
	if result.GetSchemaMajor() != contracts.SchemaMajor {
		return fmt.Sprintf("result declares contract major %d, expected %d",
			result.GetSchemaMajor(), contracts.SchemaMajor)
	}
	// The digest proves which result this is; these prove the state describes
	// the same run of it. A state claiming iteration 5 over a result taken at
	// iteration 1 would otherwise read as having reached the bound.
	if result.GetIteration() != state.GetIteration() {
		return fmt.Sprintf("result was taken at iteration %d, state claims %d",
			result.GetIteration(), state.GetIteration())
	}
	if result.GetMaxIterations() != state.GetMaxIterations() {
		return fmt.Sprintf("result was bounded at %d iterations, state claims %d",
			result.GetMaxIterations(), state.GetMaxIterations())
	}
	if result.GetMode() != state.GetMode() {
		return fmt.Sprintf("result was produced in %v, state claims %v",
			result.GetMode(), state.GetMode())
	}
	if !bytes.Equal(result.GetLedger().GetDigestSha256(), state.GetLedgerDigestSha256()) {
		return "result references a different ledger than the state recorded"
	}
	if result.GetProducerRole() != pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR {
		return "result was not produced by the orchestrator"
	}
	return ""
}

func colorName(result *pb.GrimesResult) string {
	switch result.GetLegacyColor() {
	case pb.LegacyColor_LEGACY_COLOR_GREEN:
		return "GREEN"
	case pb.LegacyColor_LEGACY_COLOR_RED:
		return "RED"
	case pb.LegacyColor_LEGACY_COLOR_YELLOW:
		return "YELLOW"
	default:
		return "no verdict"
	}
}
