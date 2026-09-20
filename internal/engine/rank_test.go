package engine

import (
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

func risk(l pb.Likelihood, b pb.BlastRadius, e pb.EaseOfFix) *pb.Risk {
	return &pb.Risk{Likelihood: l, BlastRadius: b, EaseOfFix: e}
}

const (
	likely     = pb.Likelihood_LIKELIHOOD_LIKELY
	plausible  = pb.Likelihood_LIKELIHOOD_PLAUSIBLE
	unlikely   = pb.Likelihood_LIKELIHOOD_UNLIKELY
	unknownL   = pb.Likelihood_LIKELIHOOD_UNKNOWN
	systemic   = pb.BlastRadius_BLAST_RADIUS_SYSTEMIC
	component  = pb.BlastRadius_BLAST_RADIUS_LOCAL_COMPONENT
	oneUser    = pb.BlastRadius_BLAST_RADIUS_SINGLE_USER
	unstatedE  = pb.EaseOfFix_EASE_OF_FIX_UNSPECIFIED
	trivialE   = pb.EaseOfFix_EASE_OF_FIX_TRIVIAL
	involvedE  = pb.EaseOfFix_EASE_OF_FIX_INVOLVED
	unattacked = pb.FindingProvenance_FINDING_PROVENANCE_UNATTACKED
	contestedP = pb.FindingProvenance_FINDING_PROVENANCE_CONTESTED
)

// Both axes decide the rank, so neither alone may.
func TestRankUsesBothAxes(t *testing.T) {
	reachableButSmall := Rank(risk(likely, oneUser, unstatedE), 0, unattacked)
	hugeButUnlikely := Rank(risk(unlikely, systemic, unstatedE), 0, unattacked)
	bothBad := Rank(risk(likely, systemic, unstatedE), 0, unattacked)

	if bothBad <= reachableButSmall || bothBad <= hugeButUnlikely {
		t.Errorf("likely+systemic ranked %v, below one-axis findings (%v, %v)",
			bothBad, reachableButSmall, hugeButUnlikely)
	}
	// 3x1 against 1x4: impact carries the second further, which is the point of
	// a matrix over a single letter.
	if reachableButSmall >= hugeButUnlikely {
		t.Errorf("a single-user certainty (%v) outranked a systemic maybe (%v)",
			reachableButSmall, hugeButUnlikely)
	}
}

// Ease orders equals and never overtakes a worse product.
func TestEaseOnlyBreaksTies(t *testing.T) {
	easy := Rank(risk(plausible, component, trivialE), 0, unattacked)
	hard := Rank(risk(plausible, component, involvedE), 0, unattacked)
	if easy <= hard {
		t.Errorf("the easier repair did not sort first: %v against %v", easy, hard)
	}
	// One step worse on either axis has to beat any ease.
	worseAndHard := Rank(risk(likely, component, involvedE), 0, unattacked)
	if worseAndHard <= easy {
		t.Errorf("ease overtook a worse product: %v against %v", easy, worseAndHard)
	}
	// Unstated sits between, so declining to estimate neither helps nor hurts.
	unstated := Rank(risk(plausible, component, unstatedE), 0, unattacked)
	if unstated >= easy || unstated <= hard {
		t.Errorf("unstated ease ranked %v, outside trivial %v and involved %v",
			unstated, easy, hard)
	}
}

// The one evidence class that gets stronger with effort.
func TestSurvivedRefutationRaisesProbability(t *testing.T) {
	none := Rank(risk(plausible, component, unstatedE), 0, unattacked)
	one := Rank(risk(plausible, component, unstatedE), 1, unattacked)
	three := Rank(risk(plausible, component, unstatedE), 3, unattacked)
	if !(none < one && one < three) {
		t.Errorf("survived attempts did not raise the rank: %v, %v, %v", none, one, three)
	}
	// A claim two contexts disagree about is attested by neither.
	if got := Rank(risk(plausible, component, unstatedE), 3, contestedP); got != none {
		t.Errorf("a contested claim was credited with its upheld attempts: %v against %v", got, none)
	}
}

// Unknown on an axis is no score, not a low one.
func TestAnUnrankableAxisScoresNothing(t *testing.T) {
	if got := Rank(risk(unknownL, systemic, trivialE), 4, unattacked); got != 0 {
		t.Errorf("unknown likelihood ranked %v, want 0", got)
	}
	if got := Rank(risk(likely, pb.BlastRadius_BLAST_RADIUS_UNSPECIFIED, trivialE), 4, unattacked); got != 0 {
		t.Errorf("unstated blast radius ranked %v, want 0", got)
	}
}

// Over the whole input space: ease never decides across different products.
func TestEaseNeverCrossesAProductBoundary(t *testing.T) {
	ls := []pb.Likelihood{likely, plausible, unlikely}
	bs := []pb.BlastRadius{systemic, pb.BlastRadius_BLAST_RADIUS_SERVICE, component, oneUser}
	es := []pb.EaseOfFix{unstatedE, trivialE, pb.EaseOfFix_EASE_OF_FIX_MODERATE, involvedE}
	for _, l1 := range ls {
		for _, b1 := range bs {
			for _, l2 := range ls {
				for _, b2 := range bs {
					p1 := probability(l1, 0, unattacked) * impact(b1)
					p2 := probability(l2, 0, unattacked) * impact(b2)
					if p1 <= p2 {
						continue
					}
					for _, e1 := range es {
						for _, e2 := range es {
							if Rank(risk(l1, b1, e1), 0, unattacked) <= Rank(risk(l2, b2, e2), 0, unattacked) {
								t.Fatalf("ease reordered %d over %d", p1, p2)
							}
						}
					}
				}
			}
		}
	}
}
