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

// measure compares what the report accounted for against the inventory and the
// categories the engine routed.
//
// Both denominators are the engine's. A unit the inventory does not contain is
// an error rather than a finding: the provider is describing a target this run
// did not resolve, and nothing downstream could tell whether it examined the
// right artifact. A category the engine routed but the report does not carry is
// the same error in the other dimension — taking the routed set from the report
// would let a provider shorten its own denominator and pass every check on the
// remainder.
func measure(report *pb.ProviderReport, inventory *pb.TargetInventory, routed []pb.Category) (Coverage, error) {
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

	wasRouted := map[pb.Category]bool{}
	for _, c := range routed {
		wasRouted[c] = true
	}
	for _, c := range report.GetRoutedCategories() {
		if !wasRouted[c] {
			return Coverage{}, fmt.Errorf("%w: reported routing %v, which this run did not route", ErrProviderOutput, c)
		}
	}

	// Keyed by category, so a second stop for one category cannot raise the
	// probe count of a category that was never reached. The contract rejects
	// the duplicate outright; this stays correct if it ever cannot.
	stopped := map[pb.Category]*pb.CategoryStop{}
	for _, s := range report.GetCategoryStops() {
		if !wasRouted[s.GetCategory()] {
			return Coverage{}, fmt.Errorf("%w: stopped %v, which this run did not route", ErrProviderOutput, s.GetCategory())
		}
		if _, dup := stopped[s.GetCategory()]; dup {
			return Coverage{}, fmt.Errorf("%w: %v recorded two stops", ErrProviderOutput, s.GetCategory())
		}
		stopped[s.GetCategory()] = s
	}

	cov.CategoriesStopped = len(routed) > 0
	for _, c := range routed {
		s, ok := stopped[c]
		if !ok || s.GetCondition() == pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE {
			cov.CategoriesStopped = false
		}
		if !ok {
			continue
		}
		if s.GetCondition() == pb.StopCondition_STOP_CONDITION_EVIDENCE_UNAVAILABLE {
			cov.UnknownRemains = true
		}
		if s.GetProbesAttempted() > 0 {
			cov.Probed = true
		}
	}
	return cov, nil
}
