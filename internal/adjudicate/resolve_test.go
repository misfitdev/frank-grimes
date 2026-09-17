package adjudicate

import (
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// Strictest is a fold over every decision a panel can hold, which is more
// combinations than a CLI test can put in front of it.

var every = []pb.Decision{
	pb.Decision_DECISION_UNSPECIFIED,
	pb.Decision_DECISION_BLOCK,
	pb.Decision_DECISION_CONDITIONAL,
	pb.Decision_DECISION_PASS,
	pb.Decision_DECISION_INFORMATIONAL,
}

// A panel of one is that reviewer.
func TestAPanelOfOneIsThatReviewer(t *testing.T) {
	for _, d := range every {
		if got := Strictest(d); got != d {
			t.Errorf("Strictest(%v) = %v", d, got)
		}
	}
}

// Order is not an argument. Two reviewers reaching the same pair of decisions
// resolve the same way whichever order the engine ran them in.
func TestAPanelDoesNotDependOnTheOrderItAnsweredIn(t *testing.T) {
	for _, a := range every {
		for _, b := range every {
			if Strictest(a, b) != Strictest(b, a) {
				t.Errorf("Strictest(%v, %v) = %v but reversed = %v",
					a, b, Strictest(a, b), Strictest(b, a))
			}
		}
	}
}

// Adding a reviewer can only take confidence away.
func TestAnAddedReviewerNeverRelaxesAPanel(t *testing.T) {
	for _, a := range every {
		for _, b := range every {
			panel := Strictest(a, b)
			if rank(panel) < rank(a) {
				t.Errorf("Strictest(%v, %v) = %v, which is less strict than %v", a, b, panel, a)
			}
		}
	}
}

// A block anywhere in a panel is the panel's decision.
func TestOneBlockDecidesTheWholePanel(t *testing.T) {
	for _, other := range every {
		got := Strictest(pb.Decision_DECISION_PASS, other, pb.Decision_DECISION_BLOCK)
		if got != pb.Decision_DECISION_BLOCK {
			t.Errorf("a panel holding a block resolved to %v", got)
		}
	}
}

// An empty panel decided nothing, which is not agreement.
func TestAnEmptyPanelDecidesNothing(t *testing.T) {
	if got := Strictest(); got != pb.Decision_DECISION_UNSPECIFIED {
		t.Errorf("Strictest() = %v, want unspecified", got)
	}
}

// Resolve is what folds the panel into the primary, and it must keep treating
// anything it cannot read as withholding rather than granting.
func TestAPanelThatSaysNothingDoesNotConfirmAPass(t *testing.T) {
	got := Resolve(pb.Decision_DECISION_PASS, Strictest())
	if got == pb.Decision_DECISION_PASS {
		t.Error("a pass survived a panel that decided nothing")
	}
}
