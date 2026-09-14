package engine

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// resolve checks that a candidate's evidence points at the target this run
// resolved, rather than only having the shape the tier requires.
//
// The contract can see that an E2 carries a quote and an anchor. It cannot see
// that the anchor names a unit of this target or that the quote is anywhere in
// it, because it never holds the artifact. Only the engine holds both.
func resolve(c *pb.CandidateFinding, against *Collected) error {
	if _, err := locate(c.GetLocation().GetAnchor(), against); err != nil {
		return err
	}
	switch detail := c.GetEvidence().GetDetail().(type) {
	case *pb.Evidence_Citation:
		// The citation carries its own anchor, which is where the quote is said
		// to be visible. It is not always the finding's location: a defect at
		// one place can be shown by a line somewhere else.
		cited, err := locate(detail.Citation.GetAnchor(), against)
		if err != nil {
			return err
		}
		return quoted(detail.Citation.GetQuote(), cited, against)
	case *pb.Evidence_Reproduction:
		return ranInside(detail.Reproduction, against)
	}
	return nil
}

// locate returns the unit the anchor names, or an error naming what it could
// not find.
//
// The inventory is the engine's own record of what this target is made of, so
// an anchor outside it describes an artifact this run did not resolve. Same
// treatment coverage gives a unit outside the target, for the same reason.
func locate(a *pb.Anchor, against *Collected) (*pb.TargetUnit, error) {
	if a.GetAt() == nil {
		return nil, fmt.Errorf("%w: a finding carries no anchor", ErrProviderOutput)
	}
	kind := against.Target.GetKind()
	want, label := anchorTarget(a, kind)
	if want == "" {
		return nil, fmt.Errorf("%w: a %s target cannot be anchored by %s",
			ErrProviderOutput, kindName(kind), anchorKindName(a))
	}
	for _, u := range against.Inventory.GetUnits() {
		if matches(u, a, kind) {
			return u, nil
		}
	}
	return nil, fmt.Errorf("%w: %s names %q, which is not part of %q",
		ErrProviderOutput, label, want, against.Target.GetScope())
}

// matches reports whether the unit is the one the anchor names.
//
// A document section is the loose case. The unit id is the heading's slug, but
// a document with no heading is one unit named after the file, and a reviewer
// reads a heading rather than the slug it was reduced to. All three spellings
// name the same place, so all three are accepted.
func matches(u *pb.TargetUnit, a *pb.Anchor, kind pb.TargetKind) bool {
	want, _ := anchorTarget(a, kind)
	if kind != pb.TargetKind_TARGET_KIND_DOCUMENT {
		return unitKey(u, kind) == want
	}
	section := a.GetAt().(*pb.Anchor_DocumentPart).DocumentPart.GetSection()
	return section == u.GetId() ||
		want == u.GetId() ||
		want == normalizeSection(u.GetId()) ||
		want == normalizeSection(u.GetLabel())
}

// anchorTarget reduces an anchor to the key it would match in the inventory,
// and a phrase naming what was anchored. An empty key means the anchor is not
// the kind this target uses, which is a mismatch rather than a miss.
func anchorTarget(a *pb.Anchor, kind pb.TargetKind) (key, label string) {
	switch at := a.GetAt().(type) {
	case *pb.Anchor_RepoLine:
		if kind != pb.TargetKind_TARGET_KIND_CODE {
			return "", ""
		}
		return contracts.NormalizePath(at.RepoLine.GetPath().GetValue()), "a path"
	case *pb.Anchor_DocumentPart:
		if kind != pb.TargetKind_TARGET_KIND_DOCUMENT {
			return "", ""
		}
		return normalizeSection(at.DocumentPart.GetSection()), "a section"
	case *pb.Anchor_ArgumentStep:
		if kind != pb.TargetKind_TARGET_KIND_IDEA {
			return "", ""
		}
		return fmt.Sprintf("step-%d", at.ArgumentStep.GetStep()), "a step"
	case *pb.Anchor_RetrievedSource:
		if kind != pb.TargetKind_TARGET_KIND_EXTERNAL {
			return "", ""
		}
		return strings.TrimRight(at.RetrievedSource.GetUri(), "/"), "a source"
	}
	return "", ""
}

// unitKey renders a unit the way an anchor of this target's kind would name it.
// Only a code unit needs normalizing: its id is a path, and a path has more
// than one spelling.
func unitKey(u *pb.TargetUnit, kind pb.TargetKind) string {
	if kind == pb.TargetKind_TARGET_KIND_CODE {
		return contracts.NormalizePath(u.GetId())
	}
	return u.GetId()
}

// normalizeSection renders a section reference the way a unit id was slugged,
// so a heading and its slug name the same place.
func normalizeSection(s string) string {
	return slug(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}

// quoted reports whether the quote is visible in the unit the citation anchors
// to.
//
// A quote that is nowhere in the artifact cannot establish anything about it,
// and E2 is the tier that rests entirely on the quote being there.
func quoted(quote string, unit *pb.TargetUnit, against *Collected) error {
	body, err := unitBytes(unit, against)
	if err != nil {
		return err
	}
	// Compared under the normalization identity already uses, so a quote is not
	// refused for line endings or trailing whitespace the reviewer never saw.
	if !strings.Contains(contracts.NormalizeEvidence(body), contracts.NormalizeEvidence(quote)) {
		return fmt.Errorf("%w: the quoted text is not in %q", ErrProviderOutput, unit.GetLabel())
	}
	return nil
}

// unitBytes returns the bytes the unit covers.
//
// Spans are re-derived rather than carried in the inventory: the split is a
// pure function of the content the engine already persisted, and a second copy
// in the contract could disagree with it.
func unitBytes(unit *pb.TargetUnit, against *Collected) (string, error) {
	if against.Target.GetKind() == pb.TargetKind_TARGET_KIND_CODE {
		path, err := codePath(unit, against)
		if err != nil {
			return "", err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("%w: cannot read %q", ErrProviderOutput, unit.GetId())
		}
		return string(body), nil
	}
	content, err := targetContent(against)
	if err != nil {
		return "", err
	}
	switch against.Target.GetKind() {
	case pb.TargetKind_TARGET_KIND_DOCUMENT:
		return sectionBody(content, unit.GetId()), nil
	case pb.TargetKind_TARGET_KIND_IDEA:
		return stepBody(content, unit.GetId()), nil
	default:
		return content, nil
	}
}

// codePath resolves a code unit against the repository root.
//
// A unit id is relative to the root, not to the scope: collectCode takes it
// with filepath.Rel against the base. Joining it onto the scope would name
// src/src/a.sh for a target whose scope is already src.
func codePath(unit *pb.TargetUnit, against *Collected) (string, error) {
	abs, _, err := contained(against.Target.GetRoot(), unit.GetId())
	if err != nil {
		return "", fmt.Errorf("%w: %q is not inside the repository", ErrProviderOutput, unit.GetId())
	}
	return abs, nil
}

func targetContent(against *Collected) (string, error) {
	if len(against.ContentBytes) > 0 {
		return string(against.ContentBytes), nil
	}
	if against.ContentPath == "" {
		return "", fmt.Errorf("%w: this run holds no content for %q",
			ErrProviderOutput, against.Target.GetScope())
	}
	body, err := os.ReadFile(against.ContentPath)
	if err != nil {
		return "", fmt.Errorf("%w: cannot read the reviewed content", ErrProviderOutput)
	}
	return string(body), nil
}

// ranInside refuses an executed command whose working directory is outside the
// target. A command run somewhere else observed something else.
func ranInside(r *pb.Reproduction, against *Collected) error {
	cmd, ok := r.GetExhibit().(*pb.Reproduction_ExecutedCommand)
	if !ok {
		return nil
	}
	root := against.Target.GetRoot()
	if root == "" {
		return nil
	}
	cwd := cmd.ExecutedCommand.GetCwd().GetValue()
	if _, _, err := contained(root, cwd); err != nil {
		return fmt.Errorf("%w: the command ran in %q, outside the target", ErrProviderOutput, cwd)
	}
	return nil
}

func kindName(k pb.TargetKind) string {
	return strings.ToLower(strings.TrimPrefix(k.String(), "TARGET_KIND_"))
}

func anchorKindName(a *pb.Anchor) string {
	switch a.GetAt().(type) {
	case *pb.Anchor_RepoLine:
		return "a repository path"
	case *pb.Anchor_DocumentPart:
		return "a document section"
	case *pb.Anchor_ArgumentStep:
		return "an argument step"
	case *pb.Anchor_RetrievedSource:
		return "a retrieved source"
	}
	return "nothing"
}
