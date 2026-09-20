package engine

import (
	"encoding/hex"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// keepAcquittals records what this pass attacked and could not break.
//
// An acquittal was report-scoped: a claim that survived a controlled probe last
// iteration was re-earned from scratch on the next one, or silently not
// re-earned, and nothing could tell an invariant that keeps surviving from one
// that stopped being probed. Content-addressed here the way a finding is, so
// the claim keeps its identity and the count means something.
//
// A pass that acquits nothing leaves what is already there. Absence of a probe
// is not evidence the claim stopped holding; the iteration it was last probed
// in is what says whether anyone is still asking.
func keepAcquittals(ledger *pb.Ledger, report *pb.ProviderReport, iteration uint32) {
	if len(report.GetAcquittals()) == 0 {
		return
	}
	if ledger.Acquittals == nil {
		ledger.Acquittals = map[string]*pb.AcquittalRecord{}
	}
	// One report is one probe of a claim, however many times it says so. Two
	// acquittals of the same claim in the same report are the same probe
	// reported twice, and counting both would let a report inflate the record
	// by repeating itself. Keyed first, applied once each.
	probed := map[string]*pb.Acquittal{}
	for _, a := range report.GetAcquittals() {
		probed[acquittalID(a)] = a
	}
	for id, a := range probed {
		held, ok := ledger.Acquittals[id]
		if !ok {
			ledger.Acquittals[id] = &pb.AcquittalRecord{
				Acquittal:           a,
				Survived:            1,
				LastProbedIteration: iteration,
			}
			continue
		}
		held.Acquittal = a
		held.Survived++
		held.LastProbedIteration = iteration
	}
}

// overturnAcquittals links an acquittal to the finding that later contradicted
// it.
//
// The transition worth keeping: the review said it had tested this claim and
// cleared it, and a later pass raised a defect for the same claim in the same
// place. Both halves stay, because the acquittal is what makes the finding
// interesting.
func overturnAcquittals(ledger *pb.Ledger) {
	for _, f := range ledger.GetFindings() {
		if !Open(f.GetStatus()) {
			continue
		}
		id := hex.EncodeToString(contracts.Fingerprint(
			f.GetCategory(), f.GetLocation().GetAnchor(), f.GetEvidence().GetClaim()))
		held, ok := ledger.GetAcquittals()[id]
		if !ok || held.GetOverturnedByFindingId() != "" {
			continue
		}
		held.OverturnedByFindingId = f.GetId()
	}
}

// acquittalID is the content address of a claim that held: the same claim in
// the same place, whoever probed it and however many times.
func acquittalID(a *pb.Acquittal) string {
	return hex.EncodeToString(contracts.Fingerprint(
		a.GetCategory(), a.GetClaimAnchor(), a.GetClaim()))
}
