package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// control is a claim the engine has established to be false about this target,
// handed to the refuter alongside the real ones.
type control struct {
	ref   string
	claim *pb.ClaimUnderTest
}

// controlFor builds the control the refuter has to break to be believed.
//
// The claim is placed at the anchor of a real claim and carries the same
// category, so nothing about its shape marks it out; the only thing separating
// it from its neighbours is that it is untrue. The engine reads the artifact at
// that anchor and confirms the planted identifier is absent, because a control
// whose falsity was assumed rather than checked would be an assertion in a
// prompt, and a refuter cannot be graded against one of those.
func controlFor(dir, contentPath string, claims []*pb.ClaimUnderTest, runID string, iteration uint32) (*control, error) {
	if len(claims) == 0 {
		return nil, fmt.Errorf("no claim to model a control on")
	}
	token := contracts.ControlToken(runID, iteration)
	host := claims[0]

	body, err := artifactAt(dir, contentPath, host.GetAnchor())
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte(token)) {
		return nil, fmt.Errorf("control identifier %s is present in the target", token)
	}

	ref := contracts.ClaimRef(runID, iteration, "control/"+token)
	return &control{
		ref: ref,
		claim: &pb.ClaimUnderTest{
			Ref:      ref,
			Category: host.GetCategory(),
			Anchor:   host.GetAnchor(),
			Claim:    fmt.Sprintf("the identifier %s is defined at this anchor and every path through it reads that identifier", token),
		},
	}, nil
}

// artifactAt returns the bytes the control's claim is about.
//
// A repo anchor names a file under the reviewed directory. Everything else is a
// target with no path of its own, whose content the engine already wrote down
// for the provider to read.
func artifactAt(dir, contentPath string, anchor *pb.Anchor) ([]byte, error) {
	path := contentPath
	if line := anchor.GetRepoLine(); line != nil {
		path = filepath.Join(dir, line.GetPath().GetValue())
	}
	if path == "" {
		return nil, fmt.Errorf("no artifact to establish a control against")
	}
	return os.ReadFile(path)
}

// insertControl returns the claims with the control mixed in at a position
// derived from its own handle, so it is not always the first or last thing a
// refuter reads.
func insertControl(claims []*pb.ClaimUnderTest, c *control) []*pb.ClaimUnderTest {
	at := int(c.ref[0]) % (len(claims) + 1)
	out := make([]*pb.ClaimUnderTest, 0, len(claims)+1)
	out = append(out, claims[:at]...)
	out = append(out, c.claim)
	return append(out, claims[at:]...)
}

// checkOf grades the refuter on what it did to the control.
//
// Only breaking the control counts. Upholding it is the rubber stamp this
// exists to catch, and saying nothing about it is a refuter that has not been
// shown capable of refuting anything, which is the same position as having no
// refuter at all.
func checkOf(attempt *pb.RefutationAttempt) pb.RefuterCheck {
	switch attempt.GetOutcome().(type) {
	case *pb.RefutationAttempt_Refuted:
		return pb.RefuterCheck_REFUTER_CHECK_PASSED
	case *pb.RefutationAttempt_Upheld:
		return pb.RefuterCheck_REFUTER_CHECK_FAILED
	default:
		return pb.RefuterCheck_REFUTER_CHECK_INCONCLUSIVE
	}
}
