// Package contracts implements the identity, codec, and lifecycle rules that
// proto/frank_grimes/v2/contracts.proto declares.
package contracts

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// NormalizeEvidence renders evidence text comparable across runs: UTF-8, LF
// line endings, no trailing whitespace, no leading or trailing blank lines.
// Line numbers are deliberately excluded by the caller, so that moving code
// without changing it keeps the same identity.
func NormalizeEvidence(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}

	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// NormalizePath renders a repository-relative path comparable: forward slashes,
// no leading "./", no trailing slash.
func NormalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// AnchorKey reduces an anchor to the kind tag and the canonical parts identity
// is taken over.
//
// The parts are returned separately rather than joined. Joining them would need
// a separator, and protobuf permits any byte inside a string, so a part could
// contain whatever separator was chosen and impersonate a boundary.
//
// The kind tag is a part because the canonical forms are not comparable across
// kinds: a path and a URI that read the same are different places. Line,
// section, and step numbers are excluded, so content moving within an artifact
// is not a new finding.
func AnchorKey(a *pb.Anchor) (kind string, parts []string) {
	switch at := a.GetAt().(type) {
	case *pb.Anchor_RepoLine:
		return "repo", []string{NormalizePath(at.RepoLine.GetPath().GetValue())}
	case *pb.Anchor_DocumentPart:
		return "doc", []string{
			NormalizePath(at.DocumentPart.GetDocument()),
			normalizeLabel(at.DocumentPart.GetSection()),
		}
	case *pb.Anchor_ArgumentStep:
		// The step number is the claim's identity here, not an offset into it:
		// claim 3 of an argument is a different claim from claim 4.
		return "arg", []string{
			normalizeLabel(at.ArgumentStep.GetArgument()),
			strconv.FormatUint(uint64(at.ArgumentStep.GetStep()), 10),
		}
	case *pb.Anchor_RetrievedSource:
		// The section is a named place in the source, the way a document's is,
		// not an offset into it. Two clauses of one policy are two findings.
		return "src", []string{
			strings.TrimRight(at.RetrievedSource.GetUri(), "/"),
			normalizeLabel(at.RetrievedSource.GetSection()),
		}
	default:
		return "none", nil
	}
}

// normalizeLabel renders a document or argument identifier comparable: single
// spaces, no surrounding space, case-folded, since "Appendix B" and "appendix
// b" name the same section.
func normalizeLabel(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// Fingerprint is the content address of a finding: the same defect in the same
// place with the same evidence yields the same bytes on every run and every
// machine. Positions within an artifact are not inputs, so a finding survives
// the content above it moving.
func Fingerprint(category pb.Category, anchor *pb.Anchor, evidence string) []byte {
	kind, parts := AnchorKey(anchor)
	h := sha256.New()
	writeComponent(h, CategoryName(category))
	writeComponent(h, kind)
	for _, part := range parts {
		writeComponent(h, part)
	}
	writeComponent(h, NormalizeEvidence(evidence))
	return h.Sum(nil)
}

// writeComponent length-prefixes a component so its content cannot reach across
// its own boundary. Separators alone are not enough: an anchor holding a NUL
// would otherwise hash identically to a shorter anchor plus a longer evidence
// string, giving two different findings one identity.
func writeComponent(h io.Writer, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}

// RepoAnchor is the common case: a finding at a repository-relative path.
func RepoAnchor(path string) *pb.Anchor {
	return &pb.Anchor{At: &pb.Anchor_RepoLine{RepoLine: &pb.RepoLine{
		Path: &pb.RepoPath{Value: path},
	}}}
}

// FindingID renders a fingerprint as the stable, human-referenceable ID.
// Width is 12 hex by default; a caller that has observed a collision between
// two distinct fingerprints extends both to 16.
func FindingID(category pb.Category, fingerprint []byte, wide bool) string {
	width := 12
	if wide {
		width = 16
	}
	return fmt.Sprintf("FG-%s-%s", CategoryName(category), hex.EncodeToString(fingerprint)[:width])
}

// ClaimRef is the opaque handle a claim is put to a refuter under.
//
// A finding ID would carry its category and stay the same across iterations, so
// a refuter that saw one twice would know it was looking at a claim it had
// already judged. Deriving over the run and the iteration keeps the handle
// meaningless outside the pass that issued it.
func ClaimRef(runID string, iteration uint32, findingID string) string {
	h := sha256.New()
	writeComponent(h, runID)
	writeComponent(h, strconv.FormatUint(uint64(iteration), 10))
	writeComponent(h, findingID)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ControlToken returns the identifier the engine plants in a control claim.
//
// Derived rather than fixed so it cannot be recognised across runs, and long
// enough that no artifact under review contains it by accident. The engine
// still checks that it is absent before asserting it is there: a control whose
// falsity was assumed proves nothing about the refuter that broke it.
func ControlToken(runID string, iteration uint32) string {
	h := sha256.New()
	writeComponent(h, "control")
	writeComponent(h, runID)
	writeComponent(h, strconv.FormatUint(uint64(iteration), 10))
	return "fgq" + hex.EncodeToString(h.Sum(nil))[:13]
}

// CategoryName maps the enum to the three-letter code used in IDs and reports.
func CategoryName(c pb.Category) string {
	name := c.String() // CATEGORY_SEC
	return strings.TrimPrefix(name, "CATEGORY_")
}

// ParseCategory accepts the three-letter code used in reports.
func ParseCategory(code string) (pb.Category, error) {
	v, ok := pb.Category_value["CATEGORY_"+strings.ToUpper(code)]
	if !ok || v == 0 {
		return pb.Category_CATEGORY_UNSPECIFIED, fmt.Errorf("unknown category %q", code)
	}
	return pb.Category(v), nil
}
