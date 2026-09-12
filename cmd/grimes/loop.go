package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
	"github.com/misfitdev/frank-grimes/internal/store"
)

// Exit codes for `loop` follow the stop-hook contract, not this CLI's: 0 allows
// the session to end, 2 asks for another iteration. Errors exit 1 and never 2,
// because an error that asked to continue would pin the loop.
const (
	exitAllowExit = 0
	exitContinue  = 2
)

func cmdLoop(args []string) (int, error) {
	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
	dir := fs.String("dir", ".", "repository root")
	quiet := fs.Bool("quiet", false, "suppress the decision line on stderr")
	if err := fs.Parse(args); err != nil {
		return 0, errUsage
	}

	ctx := context.Background()
	states := store.NewFileStateStore(*dir)
	results := store.NewFileResultStore(*dir)

	state, stateErr := states.Load(ctx)
	if stateErr != nil {
		return unverified(*dir, fmt.Sprintf("loop state rejected: %v", stateErr), *quiet)
	}
	if state == nil {
		return exitAllowExit, nil
	}

	result, resultErr := results.Load(ctx)
	if resultErr != nil {
		return unverified(*dir, fmt.Sprintf("result rejected: %v", resultErr), *quiet)
	}

	var digest []byte
	if result != nil {
		var err error
		if digest, err = contracts.Digest(result); err != nil {
			return unverified(*dir, fmt.Sprintf("result could not be digested: %v", err), *quiet)
		}
	}

	decision := engine.DecideLoop(state, result, digest)
	if !*quiet {
		fmt.Fprintf(os.Stderr, "grimes loop: %s (%s)\n", decision.Outcome, decision.Reason)
	}

	switch decision.Outcome {
	case engine.OutcomeUnverified:
		return unverified(*dir, decision.Reason, *quiet)

	case engine.OutcomeContinue:
		fmt.Print(engine.ContinuationPrompt(
			state.GetTarget().GetDisplay(),
			state.GetIteration(),
			state.GetMaxIterations(),
			decision.Color,
		))
		return exitContinue, nil

	default:
		// Terminal. The record stays on disk; only the loop's claim on the
		// session is released.
		if err := states.Clear(ctx); err != nil {
			return 0, err
		}
		fmt.Printf("Grimes Grind complete: %s. %s\n", decision.Outcome, decision.Reason)
		return exitAllowExit, nil
	}
}

// unverified quarantines the run record and lets the session end without
// recording a pass. It never asks to continue: a loop that cannot read its own
// state cannot tell whether continuing is progress.
func unverified(dir, reason string, quiet bool) (int, error) {
	statePath := store.NewFileStateStore(dir).Path
	if _, err := os.Stat(statePath); err == nil {
		if dest, err := contracts.Quarantine(statePath, "unverified"); err == nil && !quiet {
			fmt.Fprintf(os.Stderr, "grimes loop: quarantined loop state to %s\n", dest)
		}
	}
	fmt.Fprintf(os.Stderr, "grimes loop: this run is NOT a confirmed pass: %s\n", reason)
	fmt.Printf("Grimes Grind: the run record could not be verified, so no pass is recorded. %s\n", reason)
	return exitAllowExit, nil
}
