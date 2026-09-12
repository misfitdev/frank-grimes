package adjudicate

import (
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

var decisions = []pb.Decision{
	pb.Decision_DECISION_UNSPECIFIED,
	pb.Decision_DECISION_BLOCK,
	pb.Decision_DECISION_CONDITIONAL,
	pb.Decision_DECISION_PASS,
	pb.Decision_DECISION_INFORMATIONAL,
}

func TestResolveTable(t *testing.T) {
	pass := pb.Decision_DECISION_PASS
	cond := pb.Decision_DECISION_CONDITIONAL
	block := pb.Decision_DECISION_BLOCK

	cases := []struct {
		primary, independent, want pb.Decision
	}{
		{pass, pass, pass},
		{pass, cond, cond},
		{pass, block, block},
		{cond, pass, cond},
		{cond, cond, cond},
		{cond, block, cond},
		{block, pass, block},
		{block, cond, block},
		{block, block, block},
	}
	for _, c := range cases {
		if got := Resolve(c.primary, c.independent); got != c.want {
			t.Errorf("Resolve(%v, %v) = %v, want %v", c.primary, c.independent, got, c.want)
		}
	}
}

// An independent pass must never lift a primary that did not pass, for any
// pairing including the enum values the table does not name.
func TestResolveNeverUpgrades(t *testing.T) {
	rank := map[pb.Decision]int{
		pb.Decision_DECISION_BLOCK:       0,
		pb.Decision_DECISION_CONDITIONAL: 1,
		pb.Decision_DECISION_PASS:        2,
	}
	for _, primary := range decisions {
		for _, independent := range decisions {
			got := Resolve(primary, independent)
			pr, ok := rank[primary]
			if !ok {
				continue
			}
			gr, ok := rank[got]
			if !ok {
				t.Errorf("Resolve(%v, %v) = %v, which is not a verdict decision", primary, independent, got)
				continue
			}
			if gr > pr {
				t.Errorf("Resolve(%v, %v) = %v upgraded the primary decision", primary, independent, got)
			}
		}
	}
}

// A primary that did not pass is returned untouched whatever the second opinion.
func TestResolveNonPassPrimaryStands(t *testing.T) {
	for _, primary := range []pb.Decision{pb.Decision_DECISION_BLOCK, pb.Decision_DECISION_CONDITIONAL} {
		for _, independent := range decisions {
			if got := Resolve(primary, independent); got != primary {
				t.Errorf("Resolve(%v, %v) = %v, want %v", primary, independent, got, primary)
			}
		}
	}
}

// A pass primary meeting an unusable second opinion cannot stay a pass.
func TestResolvePassWithUnusableIndependent(t *testing.T) {
	for _, independent := range []pb.Decision{pb.Decision_DECISION_UNSPECIFIED, pb.Decision_DECISION_INFORMATIONAL} {
		got := Resolve(pb.Decision_DECISION_PASS, independent)
		if got == pb.Decision_DECISION_PASS {
			t.Errorf("Resolve(pass, %v) = pass, want a downgrade", independent)
		}
	}
}
