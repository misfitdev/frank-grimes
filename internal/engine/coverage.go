package engine

import (
	"fmt"
	"sort"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// Coverage is what a review accounted for, measured against the inventory the
// engine resolved rather than against anything the provider supplied.
//
// The denominator is the engine's; the numerator is the provider's claim. This
// makes a review accountable to a fixed set of units. It does not make the
// claim true: that is what refutation is for.
type Coverage struct {
	// Unaccounted names inventory units the report neither examined nor
	// skipped, sorted, so a run that looked at part of a target says which
	// part it did not.
	Unaccounted []string
	// CategoriesStopped is true when every routed category recorded a stop and
	// none of them was blocked for want of evidence.
	CategoriesStopped bool
	// Probed is true when some category attempted a probe. Finding a defect is
	// not evidence that an invariant was tested, which is what the count of
	// candidates used to stand in for.
	Probed bool
	// UnknownRemains is true when a material unit was skipped or a category
	// stopped because the evidence to continue was unavailable.
	UnknownRemains bool
}

// measure compares what the report accounted for against the inventory.
//
// A unit the inventory does not contain is an error rather than a finding: the
// provider is describing a target this run did not resolve, and nothing
// downstream could tell whether it examined the right artifact.
func measure(report *pb.ProviderReport, inventory *pb.TargetInventory) (Coverage, error) {
	inScope := map[string]bool{}
	for _, u := range inventory.GetUnits() {
		inScope[u.GetId()] = true
	}

	accounted := map[string]bool{}
	for _, id := range report.GetCoverage().GetExamined() {
		if !inScope[id] {
			return Coverage{}, fmt.Errorf("%w: examined %q, which is not in the target", ErrProviderOutput, id)
		}
		accounted[id] = true
	}

	var cov Coverage
	for _, s := range report.GetCoverage().GetSkipped() {
		if !inScope[s.GetUnitId()] {
			return Coverage{}, fmt.Errorf("%w: skipped %q, which is not in the target", ErrProviderOutput, s.GetUnitId())
		}
		accounted[s.GetUnitId()] = true
		if s.GetMaterial() {
			cov.UnknownRemains = true
		}
	}

	for id := range inScope {
		if !accounted[id] {
			cov.Unaccounted = append(cov.Unaccounted, id)
		}
	}
	sort.Strings(cov.Unaccounted)

	stopped := map[pb.Category]*pb.CategoryStop{}
	for _, s := range report.GetCategoryStops() {
		stopped[s.GetCategory()] = s
		if s.GetCondition() == pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE {
			cov.UnknownRemains = true
		}
		if s.GetProbesAttempted() > 0 {
			cov.Probed = true
		}
	}

	cov.CategoriesStopped = len(report.GetRoutedCategories()) > 0
	for _, c := range report.GetRoutedCategories() {
		s, ok := stopped[c]
		if !ok || s.GetCondition() == pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE {
			cov.CategoriesStopped = false
			break
		}
	}
	return cov, nil
}
