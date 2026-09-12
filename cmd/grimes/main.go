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

  grimes state [--show|--clear]
      Inspect or discard loop state.

Exit codes:
  0  pass
  3  conditional
  4  block
  1  operational failure
  2  usage
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

	e := &engine.Engine{
		Collector: engine.PathCollector{},
		Provider: &provider.Exec{
			Command: cfg.ProviderCommand, Dir: cfg.Dir,
			MaxOutputBytes: cfg.MaxOutputBytes, Timeout: cfg.ProviderTimeout,
		},
		Broker:        engine.StrictBroker{},
		Adjudicator:   adjudicatorFor(cfg),
		Gate:          engine.NotApplicableGate{},
		Ledger:        store.NewFileLedger(cfg.Dir),
		State:         store.NewFileStateStore(cfg.Dir),
		Clock:         engine.SystemClock,
		RunID:         runID(),
		MaxIterations: uint32(cfg.MaxIterations),
		Research:      cfg.Research,
		Dir:           cfg.Dir,
	}

	result, err := e.Run(ctx, engine.TargetSpec{
		Root: cfg.Dir, Scope: cfg.Target, Categories: cfg.Categories,
	}, cfg.Mode)
	if err != nil {
		return 0, err
	}

	if err := emit(result, cfg.Format); err != nil {
		return 0, err
	}
	return codeFor(result.GetVerdict().GetDecision()), nil
}

func adjudicatorFor(cfg *config) engine.Adjudicator {
	if len(cfg.AdjudicatorCommand) == 0 {
		return nil
	}
	return engine.ProviderAdjudicator{
		Provider: &provider.Exec{
			Command: cfg.AdjudicatorCommand, Dir: cfg.Dir,
			MaxOutputBytes: cfg.MaxOutputBytes, Timeout: cfg.ProviderTimeout,
		},
		ReviewerID: cfg.AdjudicatorCommand[0],
		RunID:      runID(),
		Clock:      engine.SystemClock,
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
