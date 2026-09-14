package engine

import (
	"context"
	"errors"
	"testing"
)

// A collector is an exported seam, so the engine cannot assume one filled in
// what it was not asked for. Reachable only from here: the CLI always uses the
// real collector, which takes these digests while it fingerprints.
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

// The same citation is admitted when the digest is there and matches, so the
// refusal above is about the missing digest rather than the fixture.
func TestAdmitAcceptsACitationBackedByItsDigest(t *testing.T) {
	out, err := stubCollector{}.Collect(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	_, err = StrictBroker{}.Admit(context.Background(), seedCandidate(), out)
	if err != nil {
		t.Errorf("a citation matching the collected bytes was refused: %v", err)
	}
}
