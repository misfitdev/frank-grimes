package main

import (
	"errors"
	"strings"
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

func baseArgs(extra ...string) []string {
	return append([]string{"--provider-command", "/bin/true"}, append(extra, "src")...)
}

func TestParseRunDefaults(t *testing.T) {
	cfg, err := parseRun(baseArgs())
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if cfg.Target != "src" {
		t.Errorf("target = %q, want src", cfg.Target)
	}
	if cfg.Mode != pb.Mode_MODE_REPORT {
		t.Errorf("mode = %v, want report", cfg.Mode)
	}
	if cfg.MaxIterations != 5 {
		t.Errorf("max iterations = %d, want 5", cfg.MaxIterations)
	}
	if len(cfg.Categories) != 0 {
		t.Errorf("categories = %v, want none by default", cfg.Categories)
	}
}

func TestParseRunRequiresOneTarget(t *testing.T) {
	for _, args := range [][]string{
		{"--provider-command", "/bin/true"},
		{"--provider-command", "/bin/true", "a", "b"},
	} {
		if _, err := parseRun(args); !errors.Is(err, errUsage) {
			t.Errorf("args %v: error = %v, want errUsage", args, err)
		}
	}
}

func TestParseRunRequiresProviderCommand(t *testing.T) {
	if _, err := parseRun([]string{"src"}); err == nil {
		t.Fatal("want an error without --provider-command")
	}
}

// A fix run edits a checkout and commits from one. Every other kind of target
// is bytes with no history to put a worktree on.
func TestParseRunRejectsFixModeForTargetsWithoutARepository(t *testing.T) {
	_, err := parseRun(baseArgs("--mode", "fix", "--kind=document"))
	if err == nil {
		t.Fatal("want an error for a document fix run")
	}
	if !strings.Contains(err.Error(), "--kind code") {
		t.Errorf("error = %v, want it to name --kind code", err)
	}
}

func TestParseRunAcceptsFixModeForCode(t *testing.T) {
	if _, err := parseRun(baseArgs("--mode", "fix")); err != nil {
		t.Errorf("parseRun(--mode fix) = %v", err)
	}
}

// The flag package sees a list of values with no memory of what they followed,
// so the grouping is recovered from the raw arguments. What it has to preserve
// is that an argument belongs to the reviewer it was written after.
func TestEachReviewerKeepsTheArgumentsWrittenAfterIt(t *testing.T) {
	cfg, err := parseRun(baseArgs(
		"--adjudicator-command", "claude",
		"--adjudicator-arg", "-p",
		"--adjudicator-arg", "first prompt",
		"--adjudicator-command", "codex",
		"--adjudicator-arg", "exec",
		"--adjudicator-arg", "second prompt",
	))
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}

	want := [][]string{
		{"claude", "-p", "first prompt"},
		{"codex", "exec", "second prompt"},
	}
	if len(cfg.AdjudicatorCommands) != len(want) {
		t.Fatalf("got %d reviewers, want %d: %q", len(cfg.AdjudicatorCommands), len(want), cfg.AdjudicatorCommands)
	}
	for i, reviewer := range want {
		if strings.Join(cfg.AdjudicatorCommands[i], "\x00") != strings.Join(reviewer, "\x00") {
			t.Errorf("reviewer %d = %q, want %q", i, cfg.AdjudicatorCommands[i], reviewer)
		}
	}
}

// One reviewer with its arguments is what every adapter passes today, and it
// has to keep parsing to exactly what it did before a panel was possible.
func TestOneReviewerWithArgumentsIsUnchanged(t *testing.T) {
	cfg, err := parseRun(baseArgs(
		"--adjudicator-command=claude", "--adjudicator-arg=-p", "--adjudicator-arg=the prompt",
	))
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}

	got := strings.Join(cfg.AdjudicatorCommands[0], "\x00")
	if len(cfg.AdjudicatorCommands) != 1 || got != strings.Join([]string{"claude", "-p", "the prompt"}, "\x00") {
		t.Errorf("got %q", cfg.AdjudicatorCommands)
	}
}

// A scanner that consumed one token per option would read the reviewer as the
// value of the boolean flag before it, and the flag package would parse the
// same arguments without complaint: the panel would simply be gone.
func TestAReviewerAfterABooleanFlagIsStillARequestedReviewer(t *testing.T) {
	cfg, err := parseRun([]string{
		"--provider-command", "/bin/true",
		"--auto-loop", "--adjudicator-command=claude", "src",
	})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}

	if len(cfg.AdjudicatorCommands) != 1 {
		t.Fatalf("reviewers = %q, want one", cfg.AdjudicatorCommands)
	}
	if !cfg.AutoLoop {
		t.Error("the boolean flag before it lost its own meaning")
	}
}

// Two reviewers running the same command are one reviewer asked twice. The
// count would say two, and what makes a second opinion second is that it came
// from somewhere else.
func TestTheSameReviewerTwiceIsRefused(t *testing.T) {
	_, err := parseRun(baseArgs(
		"--adjudicator-command", "claude",
		"--adjudicator-command", "claude",
	))

	if err == nil {
		t.Fatal("want an error for a repeated reviewer command")
	}
	if !strings.Contains(err.Error(), "asked twice") {
		t.Errorf("error = %v, want it to say the reviewer was asked twice", err)
	}
}

func TestParseRunCommitRequiresFixMode(t *testing.T) {
	_, err := parseRun(baseArgs("--commit"))
	if err == nil {
		t.Fatal("want an error for --commit in report mode")
	}
	if !strings.Contains(err.Error(), "--mode fix") {
		t.Errorf("error = %v, want it to name --mode fix", err)
	}
}

func TestParseRunCommitRequiresVerifyCommand(t *testing.T) {
	_, err := parseRun(baseArgs("--commit", "--mode", "fix"))
	if err == nil {
		t.Fatal("want an error for --commit without a gate")
	}
	if !strings.Contains(err.Error(), "--verify-command") {
		t.Errorf("error = %v, want it to name --verify-command", err)
	}
}

func TestParseRunRejectsZeroIterations(t *testing.T) {
	if _, err := parseRun(baseArgs("--max-iterations", "0")); err == nil {
		t.Fatal("want an error for a zero iteration ceiling")
	}
}

func TestParseRunCategories(t *testing.T) {
	cfg, err := parseRun(baseArgs("--categories", "COR,sec"))
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	want := []pb.Category{pb.Category_CATEGORY_COR, pb.Category_CATEGORY_SEC}
	if len(cfg.Categories) != len(want) {
		t.Fatalf("categories = %v, want %v", cfg.Categories, want)
	}
	for i := range want {
		if cfg.Categories[i] != want[i] {
			t.Errorf("category %d = %v, want %v", i, cfg.Categories[i], want[i])
		}
	}
}

func TestParseRunRejectsUnknownValues(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
	}{
		{"category", baseArgs("--categories", "NOPE")},
		{"mode", baseArgs("--mode", "sideways")},
		{"research", baseArgs("--research", "vibes")},
		{"format", baseArgs("--format", "yaml")},
	} {
		if _, err := parseRun(c.args); err == nil {
			t.Errorf("%s: want an error for an unknown value", c.name)
		}
	}
}

func TestParseRunResearchModes(t *testing.T) {
	for _, mode := range []string{"online", "offline", "frozen:/tmp/bundle"} {
		if _, err := parseRun(baseArgs("--research", mode)); err != nil {
			t.Errorf("research %q rejected: %v", mode, err)
		}
	}
	if _, err := parseRun(baseArgs("--research", "frozen:")); err == nil {
		t.Error("want an error for a frozen bundle with no path")
	}
}

// strings.Fields cannot express an argument containing a space, so arguments
// that need one are passed individually.
func TestParseRunRepeatableProviderArgs(t *testing.T) {
	cfg, err := parseRun([]string{
		"--provider-command", "/bin/echo",
		"--provider-arg", "/tmp/config file.json",
		"--provider-arg", "--flag=a b",
		"src",
	})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	want := []string{"/bin/echo", "/tmp/config file.json", "--flag=a b"}
	if len(cfg.ProviderCommand) != len(want) {
		t.Fatalf("argv = %q, want %q", cfg.ProviderCommand, want)
	}
	for i := range want {
		if cfg.ProviderCommand[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, cfg.ProviderCommand[i], want[i])
		}
	}
}

// flag would consume a following option name as the argument, handing it to the
// provider while leaving the engine's own option unset.
func TestParseRunRejectsSwallowedOption(t *testing.T) {
	for _, args := range [][]string{
		{"--provider-command", "/bin/echo", "--provider-arg", "--auto-loop", "src"},
		{"--provider-command", "/bin/echo", "--adjudicator-command", "/bin/echo", "--adjudicator-arg", "--format", "src"},
		{"--provider-command", "/bin/echo", "--provider-arg", "--max-iterations", "src"},
		// An option carrying its value in the same token is swallowed the same way.
		{"--provider-command", "/bin/echo", "--provider-arg", "--auto-loop=true", "src"},
		{"--provider-command", "/bin/echo", "--provider-arg", "--max-iterations=99", "src"},
		{"--provider-command", "/bin/echo", "--provider-arg", "--dir=/tmp", "src"},
	} {
		if _, err := parseRun(args); !errors.Is(err, errUsage) {
			t.Errorf("args %v: error = %v, want errUsage", args, err)
		}
	}
}

// The = form is unambiguous, so an option name may be passed through on purpose.
func TestParseRunAllowsExplicitOptionAsArgument(t *testing.T) {
	cfg, err := parseRun([]string{"--provider-command", "/bin/echo", "--provider-arg=--auto-loop", "src"})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if cfg.AutoLoop {
		t.Error("--provider-arg=--auto-loop set the engine's own auto-loop")
	}
	want := []string{"/bin/echo", "--auto-loop"}
	if len(cfg.ProviderCommand) != len(want) || cfg.ProviderCommand[1] != want[1] {
		t.Errorf("argv = %q, want %q", cfg.ProviderCommand, want)
	}
}

// A value that merely begins with a dash is what the flag exists to carry.
func TestParseRunKeepsDashedArgumentValues(t *testing.T) {
	cfg, err := parseRun([]string{
		"--provider-command", "/bin/echo",
		"--provider-arg", "--flag=a b",
		"--provider-arg", "--not-a-registered-option",
		"src",
	})
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if len(cfg.ProviderCommand) != 3 {
		t.Fatalf("argv = %q, want three elements", cfg.ProviderCommand)
	}
}

func TestParseRunAdjudicatorArgNeedsCommand(t *testing.T) {
	if _, err := parseRun(baseArgs("--adjudicator-arg", "x")); err == nil {
		t.Fatal("want an error for --adjudicator-arg without a command")
	}
}

// The contract carries the bound as a uint32, so a larger value must be refused
// rather than wrapped into a smaller limit.
func TestParseRunRejectsIterationOverflow(t *testing.T) {
	if _, err := parseRun(baseArgs("--max-iterations", "4294967296")); err == nil {
		t.Fatal("want an error for an iteration bound above uint32")
	}
}

func TestCodeForDecision(t *testing.T) {
	for _, c := range []struct {
		decision pb.Decision
		want     int
	}{
		{pb.Decision_DECISION_PASS, exitPass},
		{pb.Decision_DECISION_BLOCK, exitBlock},
		{pb.Decision_DECISION_CONDITIONAL, exitConditional},
		{pb.Decision_DECISION_UNSPECIFIED, exitConditional},
	} {
		if got := codeFor(c.decision); got != c.want {
			t.Errorf("codeFor(%v) = %d, want %d", c.decision, got, c.want)
		}
	}
}

// The flag package stops reading options at a bare --, so a target spelled
// after one is a path. A raw scanner that kept going would turn that path into
// a command the engine runs.
func TestScannersStopAtDashDash(t *testing.T) {
	args := []string{"--dir=.", "--provider-command=/bin/echo", "--", "--adjudicator-command=./evil.sh"}

	panel, err := adjudicators(args)
	if err != nil {
		t.Fatalf("adjudicators: %v", err)
	}
	if len(panel) != 0 {
		t.Errorf("a positional target introduced reviewer commands: %q", panel)
	}

	// And the whole parse keeps the token as the target it is.
	c, err := parseRun(args)
	if err != nil {
		t.Fatalf("parseRun: %v", err)
	}
	if c.Target != "--adjudicator-command=./evil.sh" {
		t.Errorf("target = %q, want the token after --", c.Target)
	}
	if len(c.AdjudicatorCommands) != 0 {
		t.Errorf("reviewers = %q, want none", c.AdjudicatorCommands)
	}
}
