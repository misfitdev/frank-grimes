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

// Finding a defect is not evidence that an invariant was tested, which is what
// the count of candidates used to stand in for.
func TestMeasureProbedComesFromProbesNotFindings(t *testing.T) {
	r := &pb.ProviderReport{
		RoutedCategories: []pb.Category{pb.Category_CATEGORY_SEC},
		CategoryStops:    []*pb.CategoryStop{stop(pb.Category_CATEGORY_SEC, yield, 0)},
		Coverage:         &pb.UnitCoverage{Examined: []string{"a"}},
		Candidates:       []*pb.CandidateFinding{{}, {}},
	}
	cov, err := measure(r, inventoryOf("a"), r.GetRoutedCategories())
	if err != nil {
		t.Fatal(err)
	}
	if cov.Probed {
		t.Error("candidates without an attempted probe counted as having probed")
	}
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
