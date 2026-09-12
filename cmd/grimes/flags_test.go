package main

import (
	"errors"
	"strings"
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/engine"
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

// Fix mode is a separately authorized privilege that this engine does not
// implement; accepting the flag silently would imply it does.
func TestParseRunRejectsFixMode(t *testing.T) {
	_, err := parseRun(baseArgs("--mode", "fix"))
	if !errors.Is(err, engine.ErrFixModeUnsupported) {
		t.Errorf("error = %v, want ErrFixModeUnsupported", err)
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
		{"scope", baseArgs("--scope", "everything")},
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
