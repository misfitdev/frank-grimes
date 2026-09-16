package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"strings"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// repeatedArg collects one flag occurrence per argument, so an argument
// containing a space survives.
type repeatedArg []string

func (r *repeatedArg) String() string { return strings.Join(*r, " ") }

func (r *repeatedArg) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// argTakingFlags are the options whose value is an opaque argument, so a bare
// option name following one of them is almost certainly a mistake.
var argTakingFlags = []string{"provider-arg", "adjudicator-arg", "refuter-arg", "sandbox-arg"}

// rejectSwallowedFlags refuses `--provider-arg --auto-loop`, where the flag
// package would consume --auto-loop as the argument and leave the engine's own
// auto-loop off.
//
// The check lives here because Set cannot see how its value arrived: the flag
// package calls it identically for the space-separated and the = form. Scanning
// the raw arguments is what distinguishes them, so --provider-arg=--auto-loop
// still passes an option name through deliberately.
func rejectSwallowedFlags(fs *flag.FlagSet, args []string) error {
	takesArg := func(tok string) bool {
		for _, name := range argTakingFlags {
			if tok == "-"+name || tok == "--"+name {
				return true
			}
		}
		return false
	}
	for i, tok := range args {
		if !takesArg(tok) || i+1 >= len(args) {
			continue
		}
		next := args[i+1]
		name := strings.TrimLeft(next, "-")
		if name == next || strings.Contains(name, " ") {
			continue
		}
		// An option carries its value in the same token, so compare the name
		// alone: --auto-loop=true is as swallowed as --auto-loop.
		name, _, _ = strings.Cut(name, "=")
		if fs.Lookup(name) != nil {
			return fmt.Errorf("%w: %s %s reads %s as the argument; write %s=%s to pass it through",
				errUsage, tok, next, next, tok, next)
		}
	}
	return nil
}

type config struct {
	Target             string
	Scope              string
	ScopeSet           bool
	Categories         []pb.Category
	Mode               pb.Mode
	VerifyCommand      string
	Commit             bool
	MaxIterations      uint
	AutoLoop           bool
	Research           string
	ProviderCommand    []string
	AdjudicatorCommand []string
	RefuterCommand     []string
	ProviderTimeout    time.Duration
	MaxOutputBytes     int64
	Format             string
	Dir                string
	Kind               pb.TargetKind
	Snapshot           string
	AdjudicatorFresh   bool
	RefuterFresh       bool
	SandboxCommand     []string
	Unsafe             bool
}

// Root is the repository root a code target is taken from. Only a code target
// has one; a document, an argument, or an external source is named in full by
// its own scope and needs no repository to exist.
func (c *config) Root() string {
	if c.Kind == pb.TargetKind_TARGET_KIND_CODE {
		return c.Dir
	}
	return ""
}

var errUsage = errors.New("usage")

func parseRun(args []string) (*config, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	scope := fs.String("scope", "recent-changes", "recent-changes or whole-repo")
	categories := fs.String("categories", "", "comma-separated categories to route, e.g. COR,SEC")
	mode := fs.String("mode", "report", "report or fix")
	verify := fs.String("verify-command", "", "gate command run once over a fix batch")
	commit := fs.Bool("commit", false, "authorize one commit; requires --mode fix and a passing gate")
	maxIter := fs.Uint("max-iterations", 5, "iteration ceiling")
	autoLoop := fs.Bool("auto-loop", false, "continue while iterations still change the verdict")
	research := fs.String("research", "offline", "online, offline, or frozen:<path>; advisory until execution boundaries land")
	providerCmd := fs.String("provider-command", "", "command that performs the review; split on whitespace")
	adjudicatorCmd := fs.String("adjudicator-command", "", "command that performs the independent review; split on whitespace")
	adjudicatorFresh := fs.Bool("adjudicator-fresh", false, "assert the adjudicator command begins a new context; without it the review is recorded as unknown-origin and cannot raise confidence")
	refuterCmd := fs.String("refuter-command", "", "command that attacks the surviving findings; split on whitespace")
	refuterFresh := fs.Bool("refuter-fresh", false, "assert the refuter command begins a new context; without it nothing it upholds can raise confidence")
	sandboxCmd := fs.String("sandbox-command", "", "wrapper that confines each role in place of the built-in one; split on whitespace")
	unsafe := fs.Bool("unsafe", false, "run each role unconfined; recorded as an unmet gate and caps confidence")
	var providerArgs, adjudicatorArgs, refuterArgs, sandboxArgs repeatedArg
	fs.Var(&providerArgs, "provider-arg", "one argument for the provider command; repeatable, not split")
	fs.Var(&adjudicatorArgs, "adjudicator-arg", "one argument for the adjudicator command; repeatable, not split")
	fs.Var(&refuterArgs, "refuter-arg", "one argument for the refuter command; repeatable, not split")
	fs.Var(&sandboxArgs, "sandbox-arg", "one argument for the sandbox command; repeatable, not split")
	timeout := fs.Duration("provider-timeout", 10*time.Minute, "per-invocation provider timeout")
	maxBytes := fs.Int64("max-output-bytes", 1<<20, "maximum bytes accepted from a provider")
	format := fs.String("format", "envelope", "envelope or prototext")
	dir := fs.String("dir", ".", "repository root")
	kind := fs.String("kind", "code", "code, document, idea, or external")
	snapshot := fs.String("snapshot", "", "frozen snapshot of an external source; required with --kind=external")

	if err := rejectSwallowedFlags(fs, args); err != nil {
		return nil, err
	}
	if err := fs.Parse(args); err != nil {
		return nil, errUsage
	}
	if fs.NArg() != 1 {
		return nil, fmt.Errorf("%w: run takes exactly one target", errUsage)
	}

	c := &config{
		Target:           fs.Arg(0),
		Scope:            *scope,
		ScopeSet:         wasSet(fs, "scope"),
		VerifyCommand:    *verify,
		Commit:           *commit,
		MaxIterations:    *maxIter,
		AutoLoop:         *autoLoop,
		Research:         *research,
		ProviderTimeout:  *timeout,
		MaxOutputBytes:   *maxBytes,
		Format:           *format,
		Dir:              *dir,
		Snapshot:         *snapshot,
		AdjudicatorFresh: *adjudicatorFresh,
		RefuterFresh:     *refuterFresh,
		Unsafe:           *unsafe,
	}

	var err error
	if c.Mode, err = parseMode(*mode); err != nil {
		return nil, err
	}
	if c.Kind, err = parseKind(*kind); err != nil {
		return nil, err
	}
	if c.Categories, err = parseCategories(*categories); err != nil {
		return nil, err
	}
	c.ProviderCommand = append(strings.Fields(*providerCmd), providerArgs...)
	c.AdjudicatorCommand = append(strings.Fields(*adjudicatorCmd), adjudicatorArgs...)
	if *adjudicatorCmd == "" && len(adjudicatorArgs) > 0 {
		return nil, fmt.Errorf("--adjudicator-arg needs --adjudicator-command")
	}
	if *adjudicatorCmd == "" && *adjudicatorFresh {
		return nil, fmt.Errorf("--adjudicator-fresh needs --adjudicator-command")
	}
	c.RefuterCommand = append(strings.Fields(*refuterCmd), refuterArgs...)
	c.SandboxCommand = append(strings.Fields(*sandboxCmd), sandboxArgs...)
	if *refuterCmd == "" && len(refuterArgs) > 0 {
		return nil, fmt.Errorf("--refuter-arg needs --refuter-command")
	}
	if *refuterCmd == "" && *refuterFresh {
		return nil, fmt.Errorf("--refuter-fresh needs --refuter-command")
	}
	// Both would leave it ambiguous which one the run record should report, and
	// an operator who supplied a wrapper is not asking to skip it.
	if len(c.SandboxCommand) > 0 && c.Unsafe {
		return nil, fmt.Errorf("--unsafe and --sandbox-command are alternatives")
	}
	if *sandboxCmd == "" && len(sandboxArgs) > 0 {
		return nil, fmt.Errorf("--sandbox-arg needs --sandbox-command")
	}

	return c, c.validate()
}

func (c *config) validate() error {
	if c.MaxIterations == 0 {
		return fmt.Errorf("--max-iterations must be at least 1")
	}
	// The contract carries the bound as a uint32; a larger value would wrap into
	// a smaller limit rather than be refused.
	if c.MaxIterations > math.MaxUint32 {
		return fmt.Errorf("--max-iterations must not exceed %d", uint32(math.MaxUint32))
	}
	if c.Scope != "recent-changes" && c.Scope != "whole-repo" {
		return fmt.Errorf("unknown scope %q", c.Scope)
	}
	// The collector resolves a target from the positional argument alone, so a
	// caller asking for a narrower scope would be silently reviewed on a wider
	// one. Refused until collection can honour it.
	if c.ScopeSet {
		return fmt.Errorf("--scope is not implemented; the target argument selects the scope")
	}
	// A snapshot is the whole of an external target, and it is meaningless for
	// the kinds that are read from disk directly.
	if c.Kind == pb.TargetKind_TARGET_KIND_EXTERNAL && c.Snapshot == "" {
		return fmt.Errorf("--kind external requires --snapshot; nothing fetches the source")
	}
	if c.Kind != pb.TargetKind_TARGET_KIND_EXTERNAL && c.Snapshot != "" {
		return fmt.Errorf("--snapshot applies only to --kind external")
	}
	if c.Format != "envelope" && c.Format != "prototext" {
		return fmt.Errorf("unknown format %q", c.Format)
	}
	if err := validateResearch(c.Research); err != nil {
		return err
	}
	if c.Commit && c.Mode != pb.Mode_MODE_FIX {
		return fmt.Errorf("--commit requires --mode fix")
	}
	if c.Commit && c.VerifyCommand == "" {
		return fmt.Errorf("--commit requires --verify-command")
	}
	// A fix run edits a checkout and commits from one. Every other kind of
	// target is bytes with no history to put a worktree on.
	if c.Mode == pb.Mode_MODE_FIX && c.Kind != pb.TargetKind_TARGET_KIND_CODE {
		return fmt.Errorf("--mode fix applies only to --kind code")
	}
	if len(c.ProviderCommand) == 0 {
		return fmt.Errorf("--provider-command is required")
	}
	return nil
}

// wasSet reports whether a flag was given explicitly, which a default value
// cannot distinguish on its own.
func wasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func parseMode(s string) (pb.Mode, error) {
	switch s {
	case "report":
		return pb.Mode_MODE_REPORT, nil
	case "fix":
		return pb.Mode_MODE_FIX, nil
	default:
		return pb.Mode_MODE_UNSPECIFIED, fmt.Errorf("unknown mode %q", s)
	}
}

func parseKind(s string) (pb.TargetKind, error) {
	switch s {
	case "code":
		return pb.TargetKind_TARGET_KIND_CODE, nil
	case "document":
		return pb.TargetKind_TARGET_KIND_DOCUMENT, nil
	case "idea":
		return pb.TargetKind_TARGET_KIND_IDEA, nil
	case "external":
		return pb.TargetKind_TARGET_KIND_EXTERNAL, nil
	default:
		return pb.TargetKind_TARGET_KIND_UNSPECIFIED, fmt.Errorf("unknown kind %q", s)
	}
}

func parseCategories(s string) ([]pb.Category, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []pb.Category
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cat, err := contracts.ParseCategory(strings.ToUpper(name))
		if err != nil {
			return nil, err
		}
		out = append(out, cat)
	}
	return out, nil
}

func validateResearch(s string) error {
	switch {
	case s == "online", s == "offline":
		return nil
	case strings.HasPrefix(s, "frozen:") && len(s) > len("frozen:"):
		return nil
	default:
		return fmt.Errorf("unknown research mode %q", s)
	}
}
