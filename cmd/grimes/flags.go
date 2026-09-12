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
	"github.com/misfitdev/frank-grimes/internal/engine"
)

// repeatedArg collects one flag occurrence per argument, so an argument
// containing a space survives.
type repeatedArg []string

func (r *repeatedArg) String() string { return strings.Join(*r, " ") }

func (r *repeatedArg) Set(v string) error {
	*r = append(*r, v)
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
	ProviderTimeout    time.Duration
	MaxOutputBytes     int64
	Format             string
	Dir                string
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
	var providerArgs, adjudicatorArgs repeatedArg
	fs.Var(&providerArgs, "provider-arg", "one argument for the provider command; repeatable, not split")
	fs.Var(&adjudicatorArgs, "adjudicator-arg", "one argument for the adjudicator command; repeatable, not split")
	timeout := fs.Duration("provider-timeout", 10*time.Minute, "per-invocation provider timeout")
	maxBytes := fs.Int64("max-output-bytes", 1<<20, "maximum bytes accepted from a provider")
	format := fs.String("format", "envelope", "envelope or prototext")
	dir := fs.String("dir", ".", "repository root")

	if err := fs.Parse(args); err != nil {
		return nil, errUsage
	}
	if fs.NArg() != 1 {
		return nil, fmt.Errorf("%w: run takes exactly one target", errUsage)
	}

	c := &config{
		Target:          fs.Arg(0),
		Scope:           *scope,
		ScopeSet:        wasSet(fs, "scope"),
		VerifyCommand:   *verify,
		Commit:          *commit,
		MaxIterations:   *maxIter,
		AutoLoop:        *autoLoop,
		Research:        *research,
		ProviderTimeout: *timeout,
		MaxOutputBytes:  *maxBytes,
		Format:          *format,
		Dir:             *dir,
	}

	var err error
	if c.Mode, err = parseMode(*mode); err != nil {
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
	if c.Mode == pb.Mode_MODE_FIX {
		return engine.ErrFixModeUnsupported
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
