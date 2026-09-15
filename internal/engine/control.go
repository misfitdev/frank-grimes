package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// control is a claim the engine has established to be false about this target,
// handed to the refuter alongside the real ones.
//
// witness is what the artifact says where the claim says otherwise. Breaking
// this control means exhibiting it, so the grading is against the engine's own
// read of the target rather than against the shape of the answer.
type control struct {
	ref     string
	claim   *pb.ClaimUnderTest
	witness string
}

// controlWord is the shape of a name worth denying the existence of. Short runs
// match too much of any text to be a claim about one place in it.
var controlWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{4,}`)

// controlFor builds the control the refuter has to break to be believed.
//
// The control denies something the target does contain. That direction is what
// makes it gradeable: the disproof of an absence is an absence, which a refuter
// that never opened the artifact can report just as convincingly as one that
// did, whereas the disproof of a denial is the line itself.
//
// The claim is placed at the anchor of a real claim, carries the same category,
// and names a term out of the target rather than one the engine minted, so
// nothing about its shape marks it out; the only thing separating it from its
// neighbours is that it is untrue. The engine reads the artifact to confirm
// that, because a control whose falsity was assumed rather than checked would
// be an assertion in a prompt, and a refuter cannot be graded against one.
func controlFor(dir, contentPath string, claims []*pb.ClaimUnderTest, runID string, iteration uint32) (*control, error) {
	if len(claims) == 0 {
		return nil, fmt.Errorf("no claim to model a control on")
	}
	host := claims[0]

	body, err := artifactAt(dir, contentPath, host.GetAnchor())
	if err != nil {
		return nil, err
	}
	term, witness, err := controlTermIn(body, contracts.ControlSeed(runID, iteration))
	if err != nil {
		return nil, err
	}

	ref := contracts.ClaimRef(runID, iteration, "control/"+term)
	claim := fmt.Sprintf("nothing at this anchor mentions %s, so no path through it can depend on that name", term)
	// A witness the claim already carries would be quotable without reading
	// anything, which is the fabrication this control exists to catch.
	if strings.Contains(claim, witness) {
		return nil, fmt.Errorf("the control's witness is derivable from its own claim")
	}
	return &control{
		ref:     ref,
		witness: witness,
		claim: &pb.ClaimUnderTest{
			Ref:      ref,
			Category: host.GetCategory(),
			Anchor:   host.GetAnchor(),
			Claim:    claim,
		},
	}, nil
}

// controlTermIn picks a term the artifact contains and the line that carries
// it. The choice is derived from the run so it is stable within an iteration
// and does not repeat across them.
//
// The line has to say more than the term does. One that said only the term
// would be quotable from the claim, and the exhibit would prove nothing about
// whether the refuter read the target.
func controlTermIn(body []byte, seed string) (term, witness string, err error) {
	type candidate struct{ term, line string }
	var found []candidate

	scan := bufio.NewScanner(bytes.NewReader(body))
	scan.Buffer(make([]byte, 0, 64*1024), maxControlLine)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if len(line) > maxControlLine {
			continue
		}
		for _, m := range controlWord.FindAllString(line, -1) {
			if len(line) > len(m) {
				found = append(found, candidate{term: m, line: line})
			}
		}
	}
	if err := scan.Err(); err != nil {
		return "", "", err
	}
	if len(found) == 0 {
		return "", "", fmt.Errorf("no term in the target to model a control on")
	}
	// Distinct exhibits across a scan: one line is quoted once however many of
	// its terms are eligible, so an earlier iteration's witness is not reused
	// under a different name.
	pick := found[int(contracts.Offset(seed, uint64(len(found))))]
	return pick.term, pick.line, nil
}

// maxControlLine bounds what the engine will quote at a refuter and hold it to.
// A minified or generated artifact can hold one line of unbounded length, and a
// witness nobody can read back is not an exhibit.
const maxControlLine = 4096

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
// Only breaking the control counts, and only by exhibiting the line the engine
// read out of the target. Upholding it is the rubber stamp this exists to
// catch; saying nothing about it is a refuter that has not been shown capable
// of refuting anything, which is the same position as having no refuter at all;
// and a refuted outcome carrying no excerpt of the artifact is a refuter that
// recognised the control rather than one that attacked it.
func checkOf(attempt *pb.RefutationAttempt, ctrl *control) pb.RefuterCheck {
	switch attempt.GetOutcome().(type) {
	case *pb.RefutationAttempt_Refuted:
		if !exhibits(attempt.GetRefuted(), ctrl.witness) {
			return pb.RefuterCheck_REFUTER_CHECK_FAILED
		}
		return pb.RefuterCheck_REFUTER_CHECK_PASSED
	case *pb.RefutationAttempt_Upheld:
		return pb.RefuterCheck_REFUTER_CHECK_FAILED
	default:
		return pb.RefuterCheck_REFUTER_CHECK_INCONCLUSIVE
	}
}

// exhibits reports whether the disproof carries what the artifact says.
//
// A digest of the output is no exhibit here: the engine is checking the content
// against its own read, and it has nothing to compare a hash of a command's
// whole output to.
func exhibits(repro *pb.Reproduction, witness string) bool {
	if witness == "" {
		return false
	}
	return strings.Contains(repro.GetExecutedCommand().GetOutputExcerpt(), witness)
}
