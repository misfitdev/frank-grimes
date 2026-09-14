package engine

import (
	"context"
	"errors"
	"testing"
)

// A collector is an exported seam, so the engine cannot assume one filled in
// what it was not asked for.
//
// Not a black-box case: TargetCollector fills these in for every code unit
// while it fingerprints, so no CLI invocation can produce a code target that
// lacks them. The mismatched-digest branch beside this one is covered end to
// end by the provider that edits its target and quotes the edit.
func TestAdmitRefusesACodeUnitWithNoCollectionDigest(t *testing.T) {
	out, err := stubCollector{}.Collect(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	out.UnitDigests = nil

	_, err = StrictBroker{}.Admit(context.Background(), seedCandidate(), out)
	if !errors.Is(err, ErrTargetChanged) {
		t.Errorf("err = %v, want a refusal for the missing digest", err)
	}
}
