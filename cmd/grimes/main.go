// Command grimes runs a review and derives its result.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/confine"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/engine"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"github.com/misfitdev/frank-grimes/internal/provider"
	"github.com/misfitdev/frank-grimes/internal/store"
	"google.golang.org/protobuf/encoding/prototext"
)

const usage = `grimes - run a Grimes review and derive its result

Usage:
  grimes run <target> [flags]
      Run one review iteration. The provider proposes; the engine decides.
      --kind selects what the target is: code (default), document, idea, or
      external. A code target is a path under --dir; a document or an idea is a
      file, or "-" to read an idea from stdin; an external source is a URI and
      requires --snapshot, since nothing here fetches it.

  grimes loop [--dir=<repo>]
      Decide whether a review may end, from a verified run record.
      Exit 0 allows the session to end; exit 2 asks for another iteration.

  grimes state [--show|--clear]
      Inspect or discard loop state.

Exit codes for run:
  0  pass
  3  conditional
  4  block
  1  operational failure
  2  usage

loop follows the stop-hook contract instead: 0 allow exit, 2 continue.
`

// Exit codes carry the verdict so a caller can gate on it without parsing.
const (
	exitPass        = 0
	exitFailure     = 1
	exitUsage       = 2
	exitConditional = 3
	exitBlock       = 4
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(exitUsage)
	}

	var code int
	var err error
	switch os.Args[1] {
	case "run":
		code, err = cmdRun(os.Args[2:])
	case "loop":
		code, err = cmdLoop(os.Args[2:])
	case "state":
		code, err = cmdState(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(exitUsage)
	}

	if err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(os.Stderr, "error: %v\n\n%s", err, usage)
			os.Exit(exitUsage)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitFailure)
	}
	os.Exit(code)
}

func cmdRun(args []string) (int, error) {
	cfg, err := parseRun(args)
	if err != nil {
		return 0, err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// One identity for the whole run. Derived twice, it can differ across a
	// second boundary, and the refuter's report is bound to the run it answers.
	run := runID()

	mech, err := mechanismFor(ctx, cfg)
	if err != nil {
		return 0, err
	}

	e := &engine.Engine{
		Collector: engine.TargetCollector{Stdin: os.Stdin},
		Provider: &provider.Exec{
			Command: cfg.ProviderCommand, Dir: cfg.Dir, Confine: mech,
			MaxOutputBytes: cfg.MaxOutputBytes, Timeout: cfg.ProviderTimeout,
		},
		Broker:        engine.StrictBroker{},
		Adjudicator:   adjudicatorFor(cfg, run, mech),
		Refuter:       refuterFor(cfg, run, mech),
		Gate:          gateFor(cfg),
		Inventory:     store.NewFileInventoryStore(cfg.Dir),
		Content:       store.NewFileContentStore(cfg.Dir),
		Claims:        store.NewFileClaimStore(cfg.Dir),
		Ledger:        store.NewFileLedger(cfg.Dir),
		Results:       store.NewFileResultStore(cfg.Dir),
		State:         store.NewFileStateStore(cfg.Dir),
		Clock:         engine.SystemClock,
		RunID:         run,
		AutoLoop:      cfg.AutoLoop,
		MaxIterations: uint32(cfg.MaxIterations),
		Research:      cfg.Research,
		Confinement:   mech.Name(),
		Unconfined:    cfg.Unsafe,
		Commit:        cfg.Commit,
		Dir:           cfg.Dir,
	}

	result, err := e.Run(ctx, engine.TargetSpec{
		Root: cfg.Root(), Scope: cfg.Target, Kind: cfg.Kind,
		Snapshot: cfg.Snapshot, Categories: cfg.Categories,
	}, cfg.Mode)
	if err != nil {
		return 0, err
	}

	if err := emit(result, cfg.Format); err != nil {
		return 0, err
	}
	return codeFor(result.GetVerdict().GetDecision()), nil
}

// gateFor picks the verification a fix batch will be held to.
//
// Report mode edits nothing, so there is nothing to verify and the record says
// so rather than leaving the field to be read as a gate that failed.
func gateFor(cfg *config) engine.GateRunner {
	if cfg.Mode != pb.Mode_MODE_FIX {
		return engine.NotApplicableGate{}
	}
	return engine.SelectGate(cfg.VerifyCommand, cfg.Dir, cfg.RepositoryCheck)
}

// mechanismFor resolves how each role will be confined, and proves it before
// any role runs.
//
// The probe is not a formality for the operator-supplied case alone. A policy
// that was applied and one that was silently ignored produce the same
// successful run, so neither the built-in mechanism nor a wrapper is taken on
// faith. The run fails here rather than reporting findings gathered under a
// boundary that was not there.
func mechanismFor(ctx context.Context, cfg *config) (confine.Mechanism, error) {
	mech, err := resolveMechanism(cfg)
	if err != nil {
		return nil, err
	}
	if err := confine.Verify(ctx, mech, cfg.Dir); err != nil {
		return nil, err
	}
	// A fixing role runs under a second policy, and a mechanism that applies one
	// correctly has not thereby been shown to apply the other.
	if cfg.Mode == pb.Mode_MODE_FIX {
		if err := confine.VerifyFixing(ctx, mech, cfg.Dir); err != nil {
			return nil, err
		}
	}
	return mech, nil
}

func resolveMechanism(cfg *config) (confine.Mechanism, error) {
	switch {
	case cfg.Unsafe:
		return confine.Unsafe{}, nil
	case len(cfg.SandboxCommand) > 0:
		return confine.External{Command: cfg.SandboxCommand}, nil
	}
	mech, err := confine.Default()
	if err != nil {
		// Refused rather than downgraded: a run that quietly stopped confining
		// would report the same findings as one that did not.
		return nil, fmt.Errorf("%w; supply --sandbox-command, or --unsafe to accept an unconfined run", err)
	}
	return mech, nil
}

func adjudicatorFor(cfg *config, run string, mech confine.Mechanism) engine.Adjudicator {
	if len(cfg.AdjudicatorCommand) == 0 {
		return nil
	}
	return engine.ProviderAdjudicator{
		Fresh: cfg.AdjudicatorFresh,
		Provider: &provider.Exec{
			Command: cfg.AdjudicatorCommand, Dir: cfg.Dir, Confine: mech,
			MaxOutputBytes: cfg.MaxOutputBytes, Timeout: cfg.ProviderTimeout,
		},
		ReviewerID: cfg.AdjudicatorCommand[0],
		RunID:      run,
		Clock:      engine.SystemClock,
	}
}

func refuterFor(cfg *config, run string, mech confine.Mechanism) engine.Refuter {
	if len(cfg.RefuterCommand) == 0 {
		return nil
	}
	return engine.ProviderRefuter{
		Fresh: cfg.RefuterFresh,
		Provider: &provider.Exec{
			Command: cfg.RefuterCommand, Dir: cfg.Dir, Confine: mech,
			MaxOutputBytes: cfg.MaxOutputBytes, Timeout: cfg.ProviderTimeout,
		},
		RefuterID: cfg.RefuterCommand[0],
		RunID:     run,
		Clock:     engine.SystemClock,
	}
}

func emit(result *pb.GrimesResult, format string) error {
	if format == "prototext" {
		out, err := prototext.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(result)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
		return nil
	}
	encoded, err := contracts.EncodeCanonical(result)
	if err != nil {
		return err
	}
	fmt.Print(envelope.Wrap(encoded))
	return nil
}

func codeFor(d pb.Decision) int {
	switch d {
	case pb.Decision_DECISION_PASS:
		return exitPass
	case pb.Decision_DECISION_BLOCK:
		return exitBlock
	default:
		return exitConditional
	}
}

func cmdState(args []string) (int, error) {
	fs := flag.NewFlagSet("state", flag.ContinueOnError)
	show := fs.Bool("show", false, "print current loop state")
	clear := fs.Bool("clear", false, "discard loop state")
	dir := fs.String("dir", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return 0, errUsage
	}

	s := store.NewFileStateStore(*dir)
	ctx := context.Background()

	if *clear {
		return exitPass, s.Clear(ctx)
	}
	if !*show {
		return 0, fmt.Errorf("%w: state needs --show or --clear", errUsage)
	}

	state, err := s.Load(ctx)
	if err != nil {
		return 0, err
	}
	if state == nil {
		fmt.Println("no run in progress")
		return exitPass, nil
	}
	out, err := prototext.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(state)
	if err != nil {
		return 0, err
	}
	fmt.Print(string(out))
	return exitPass, nil
}

// runID identifies this run in the ledger and loop state.
func runID() string {
	return fmt.Sprintf("run-%d-%d", time.Now().UTC().Unix(), os.Getpid())
}
