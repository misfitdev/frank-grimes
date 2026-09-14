package engine

import (
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// measure is the engine's own check, and it overlaps the contract on purpose:
// report.every_routed_category_stopped rejects a report with a missing stop
// before a run ever sees one. These shapes therefore only exist here, which is
// the point — if the CEL rule were dropped, the engine would still catch it.

func inventoryOf(ids ...string) *pb.TargetInventory {
	units := make([]*pb.TargetUnit, 0, len(ids))
	for _, id := range ids {
		units = append(units, &pb.TargetUnit{Id: id, Label: id})
	}
	return &pb.TargetInventory{Units: units}
}

func stop(c pb.Category, cond pb.StopCondition, probes uint32) *pb.CategoryStop {
	return &pb.CategoryStop{Category: c, Condition: cond, ProbesAttempted: probes}
}

const yield = pb.StopCondition_STOP_CONDITION_MARGINAL_YIELD

func TestMeasureNamesUnaccountedUnits(t *testing.T) {
	r := &pb.ProviderReport{
		Coverage: &pb.UnitCoverage{Examined: []string{"a"}},
	}
	cov, err := measure(r, inventoryOf("a", "b", "c"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cov.Unaccounted) != 2 || cov.Unaccounted[0] != "b" || cov.Unaccounted[1] != "c" {
		t.Errorf("unaccounted = %v, want [b c] sorted", cov.Unaccounted)
	}
}

// A report describing units this run did not resolve is an error, not a
// finding: nothing downstream could tell whether it reviewed the right thing.
func TestMeasureRefusesUnitsOutsideTheTarget(t *testing.T) {
	for _, c := range []struct {
		name string
		cov  *pb.UnitCoverage
	}{
		{"examined", &pb.UnitCoverage{Examined: []string{"elsewhere"}}},
		{"skipped", &pb.UnitCoverage{Skipped: []*pb.SkippedUnit{{UnitId: "elsewhere", Reason: "r"}}}},
	} {
		if _, err := measure(&pb.ProviderReport{Coverage: c.cov}, inventoryOf("a"), nil); err == nil {
			t.Errorf("%s: a unit outside the target was accepted", c.name)
		}
	}
}

// A routed category with no stop is a grind abandoned rather than finished.
// The contract rejects this shape, so the engine only meets it if that rule
// is ever removed.
func TestMeasureRequiresEveryRoutedCategoryToStop(t *testing.T) {
	r := &pb.ProviderReport{
		RoutedCategories: []pb.Category{pb.Category_CATEGORY_SEC, pb.Category_CATEGORY_COR},
		CategoryStops:    []*pb.CategoryStop{stop(pb.Category_CATEGORY_SEC, yield, 2)},
		Coverage:         &pb.UnitCoverage{Examined: []string{"a"}},
	}
	cov, err := measure(r, inventoryOf("a"), r.GetRoutedCategories())
	if err != nil {
		t.Fatal(err)
	}
	if cov.CategoriesStopped {
		t.Error("a category that never stopped counted as stopped")
	}
}

func TestMeasureDistinguishesWhyACategoryStopped(t *testing.T) {
	routed := []pb.Category{pb.Category_CATEGORY_SEC}
	for _, c := range []struct {
		name           string
		cond           pb.StopCondition
		wantStopped    bool
		wantUnknown    bool
		wantProbedWith uint32
	}{
		{"marginal yield", yield, true, false, 2},
		{"probes exhausted", pb.StopCondition_STOP_CONDITION_PROBES_EXHAUSTED, true, false, 1},
		{"evidence unavailable", pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE, false, true, 0},
	} {
		r := &pb.ProviderReport{
			RoutedCategories: routed,
			CategoryStops:    []*pb.CategoryStop{stop(pb.Category_CATEGORY_SEC, c.cond, c.wantProbedWith)},
			Coverage:         &pb.UnitCoverage{Examined: []string{"a"}},
		}
		cov, err := measure(r, inventoryOf("a"), routed)
		if err != nil {
			t.Fatal(err)
		}
		if cov.CategoriesStopped != c.wantStopped {
			t.Errorf("%s: stopped = %v, want %v", c.name, cov.CategoriesStopped, c.wantStopped)
		}
		if cov.UnknownRemains != c.wantUnknown {
			t.Errorf("%s: unknown = %v, want %v", c.name, cov.UnknownRemains, c.wantUnknown)
		}
	}
}

// Probed means this review ran something known to be capable of failing. The
// whole space, because a count of probes the provider typed used to stand in
// for it and every combination here would have read the same.
func TestMeasureProbedRequiresAProbeShownCapableOfFailing(t *testing.T) {
	routed := []pb.Category{pb.Category_CATEGORY_SEC}
	for _, c := range []struct {
		name       string
		candidates []*pb.CandidateFinding
		acquittals []*pb.Acquittal
		want       bool
	}{
		{"nothing but a probe count", nil, nil, false},
		{"E1 finding", []*pb.CandidateFinding{e1Candidate(pb.Category_CATEGORY_SEC)}, nil, true},
		{"E1 finding in an unrouted category", []*pb.CandidateFinding{e1Candidate(pb.Category_CATEGORY_COR)}, nil, false},
		{"E2 finding", []*pb.CandidateFinding{citedCandidate(pb.Category_CATEGORY_SEC)}, nil, false},
		{"controlled acquittal", nil, []*pb.Acquittal{acquittal(pb.Category_CATEGORY_SEC, true)}, true},
		{"uncontrolled acquittal", nil, []*pb.Acquittal{acquittal(pb.Category_CATEGORY_SEC, false)}, false},
		// The contract refuses this shape outright, so it cannot arrive through
		// a real run. Asserted here so the engine is independently right about
		// it rather than right by way of a rule somewhere else.
		{"acquittal whose control passed", nil,
			[]*pb.Acquittal{survivedControl(pb.Category_CATEGORY_SEC)}, false},
		{"controlled acquittal in an unrouted category", nil,
			[]*pb.Acquittal{acquittal(pb.Category_CATEGORY_COR, true)}, false},
	} {
		r := &pb.ProviderReport{
			RoutedCategories: routed,
			// A non-zero count, so a derivation that still read it would
			// answer true for every case here.
			CategoryStops: []*pb.CategoryStop{stop(pb.Category_CATEGORY_SEC, yield, 4)},
			Coverage:      &pb.UnitCoverage{Examined: []string{"a"}},
			Candidates:    c.candidates,
			Acquittals:    c.acquittals,
		}
		cov, err := measure(r, inventoryOf("a"), routed)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if cov.Probed != c.want {
			t.Errorf("%s: probed = %v, want %v", c.name, cov.Probed, c.want)
		}
	}
}

func e1Candidate(c pb.Category) *pb.CandidateFinding {
	return &pb.CandidateFinding{Category: c, Evidence: &pb.Evidence{Tier: pb.EvidenceTier_EVIDENCE_TIER_E1}}
}

func citedCandidate(c pb.Category) *pb.CandidateFinding {
	return &pb.CandidateFinding{Category: c, Evidence: &pb.Evidence{Tier: pb.EvidenceTier_EVIDENCE_TIER_E2}}
}

func acquittal(c pb.Category, controlled bool) *pb.Acquittal {
	a := &pb.Acquittal{Category: c, Claim: "it holds", Scope: "the request path"}
	if controlled {
		a.Control = &pb.NegativeControl{Mutation: "break it", Result: ran(1)}
	}
	return a
}

// survivedControl is a probe that passed even against the mutated target, so it
// is blind to the defect it claims to rule out.
func survivedControl(c pb.Category) *pb.Acquittal {
	a := acquittal(c, true)
	a.Control.Result = ran(0)
	return a
}

func ran(exit int32) *pb.Reproduction {
	return &pb.Reproduction{Exhibit: &pb.Reproduction_ExecutedCommand{
		ExecutedCommand: &pb.ExecutedCommand{Action: "run the suite", ExitCode: exit},
	}}
}

// The routed set is the engine's, not the report's. A provider that names a
// shorter list would otherwise satisfy every check on the remainder, and a stop
// for a category nobody routed would supply the probe count on its behalf.
func TestMeasureRefusesCategoriesTheRunDidNotRoute(t *testing.T) {
	routed := []pb.Category{pb.Category_CATEGORY_SEC}
	for _, c := range []struct {
		name   string
		report *pb.ProviderReport
	}{
		{"routed", &pb.ProviderReport{
			RoutedCategories: []pb.Category{pb.Category_CATEGORY_SEC, pb.Category_CATEGORY_COR},
			CategoryStops: []*pb.CategoryStop{
				stop(pb.Category_CATEGORY_SEC, yield, 1),
				stop(pb.Category_CATEGORY_COR, yield, 1),
			},
			Coverage: &pb.UnitCoverage{Examined: []string{"a"}},
		}},
		{"stopped", &pb.ProviderReport{
			RoutedCategories: routed,
			CategoryStops: []*pb.CategoryStop{
				stop(pb.Category_CATEGORY_SEC, yield, 1),
				stop(pb.Category_CATEGORY_COR, yield, 1),
			},
			Coverage: &pb.UnitCoverage{Examined: []string{"a"}},
		}},
	} {
		if _, err := measure(c.report, inventoryOf("a"), routed); err == nil {
			t.Errorf("%s: a category this run did not route was accepted", c.name)
		}
	}
}

// A category the engine routed and the report omits is not stopped, however
// complete the report looks from the inside.
func TestMeasureCountsCategoriesTheReportOmits(t *testing.T) {
	r := &pb.ProviderReport{
		RoutedCategories: []pb.Category{pb.Category_CATEGORY_SEC},
		CategoryStops:    []*pb.CategoryStop{stop(pb.Category_CATEGORY_SEC, yield, 2)},
		Coverage:         &pb.UnitCoverage{Examined: []string{"a"}},
	}
	routed := []pb.Category{pb.Category_CATEGORY_SEC, pb.Category_CATEGORY_COR}
	cov, err := measure(r, inventoryOf("a"), routed)
	if err != nil {
		t.Fatal(err)
	}
	if cov.CategoriesStopped {
		t.Error("a routed category the report never mentioned counted as stopped")
	}
}

// Two accounts of how one category's grind ended, and nothing can say which
// happened. The contract rejects the shape; this holds if that rule is removed.
func TestMeasureRefusesTwoStopsForOneCategory(t *testing.T) {
	routed := []pb.Category{pb.Category_CATEGORY_SEC}
	r := &pb.ProviderReport{
		RoutedCategories: routed,
		CategoryStops: []*pb.CategoryStop{
			stop(pb.Category_CATEGORY_SEC, pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE, 0),
			stop(pb.Category_CATEGORY_SEC, yield, 3),
		},
		Coverage: &pb.UnitCoverage{Examined: []string{"a"}},
	}
	if _, err := measure(r, inventoryOf("a"), routed); err == nil {
		t.Error("two stops for one category were accepted")
	}
}

func TestMeasureMaterialSkipLeavesUnknown(t *testing.T) {
	for _, material := range []bool{false, true} {
		r := &pb.ProviderReport{
			Coverage: &pb.UnitCoverage{
				Skipped: []*pb.SkippedUnit{{UnitId: "a", Reason: "generated", Material: material}},
			},
		}
		cov, err := measure(r, inventoryOf("a"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if cov.UnknownRemains != material {
			t.Errorf("material=%v: unknown = %v", material, cov.UnknownRemains)
		}
		if len(cov.Unaccounted) != 0 {
			t.Errorf("material=%v: an explicit skip was counted as unaccounted", material)
		}
	}
}
