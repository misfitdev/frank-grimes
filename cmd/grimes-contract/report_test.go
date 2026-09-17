package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Two runs over one review directory are something the engine already allows,
// and a claim that reads before it writes lets both of them win. Concurrency is
// not something the CLI tests can arrange, so it is arranged here.

func TestOnlyOnePassClaimsAReport(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")

	const racers = 16
	var wg sync.WaitGroup
	claimed := make([]bool, racers)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed[i] = claimPass(report, fmt.Sprintf("pass-%d", i)) == nil
		}()
	}
	close(start)
	wg.Wait()

	won := 0
	for _, ok := range claimed {
		if ok {
			won++
		}
	}
	if won != 1 {
		t.Errorf("%d of %d passes claimed the same report; exactly one may", won, racers)
	}
}

// The pass that claimed it keeps it, however many times it asks.
func TestTheClaimingPassMayGoOnWriting(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")

	for range 3 {
		if err := claimPass(report, "mine"); err != nil {
			t.Fatalf("the pass that opened the report was refused: %v", err)
		}
	}
}

// A marker a pass created and did not live to write names nobody, and nobody
// is not this pass.
func TestAnEmptyMarkerIsNotAnUnclaimedReport(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")
	if err := os.WriteFile(passPath(report), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := claimPass(report, "mine")

	if err == nil {
		t.Fatal("a pass claimed a report someone else had already opened")
	}
	if !strings.Contains(err.Error(), "another pass") {
		t.Errorf("error = %v, want it to name the other pass", err)
	}
}

// A marker exists before the pass that made it has written to it: every claim
// passes through that moment. Sealing there would take the report and the
// marker with it, out from under the pass that was claiming them.
func TestAClaimInProgressIsNotSealedOutFromUnderIt(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")
	if err := os.WriteFile(passPath(report), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := sealablePass(report, "another-pass")

	if err == nil {
		t.Fatal("a pass sealed a report another pass was in the middle of claiming")
	}
	if !strings.Contains(err.Error(), "another pass") {
		t.Errorf("error = %v, want it to name the other pass", err)
	}
}

// The pass that holds the marker seals its own report, which is the ordinary
// end of every pass.
func TestThePassThatClaimedItSealsIt(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")
	if err := claimPass(report, "mine"); err != nil {
		t.Fatal(err)
	}

	if err := sealablePass(report, "mine"); err != nil {
		t.Errorf("the pass that opened the report was refused its own seal: %v", err)
	}
}

// Outside a pass there is nothing to claim: that is the documented sequence,
// where a review builds its report before the run that carries it.
func TestOutsideAPassNothingIsClaimed(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.textproto")

	if err := claimPass(report, ""); err != nil {
		t.Fatalf("claimPass outside a pass: %v", err)
	}
	if _, err := os.Stat(passPath(report)); !os.IsNotExist(err) {
		t.Error("a marker was written for a report no pass opened")
	}
	if err := sealablePass(report, "any-pass"); err != nil {
		t.Errorf("an unclaimed report was refused to a pass that would carry it: %v", err)
	}
}
