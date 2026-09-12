package contracts

import (
	"bytes"
	"testing"
	"time"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func docAnchor(doc, section string) *pb.Anchor {
	return &pb.Anchor{At: &pb.Anchor_DocumentPart{DocumentPart: &pb.DocumentPart{
		Document: doc, Section: section,
	}}}
}

func argAnchor(name string, step uint32) *pb.Anchor {
	return &pb.Anchor{At: &pb.Anchor_ArgumentStep{ArgumentStep: &pb.ArgumentStep{
		Argument: name, Step: step,
	}}}
}

func srcAnchor(uri string) *pb.Anchor {
	return &pb.Anchor{At: &pb.Anchor_RetrievedSource{RetrievedSource: &pb.RetrievedSource{
		Uri: uri, Publisher: "Example", SnapshotSha256: make([]byte, 32),
		RetrievedAt: timestamppb.New(timeFixture()),
	}}}
}

func fp(a *pb.Anchor, evidence string) []byte {
	return Fingerprint(pb.Category_CATEGORY_SEC, a, evidence)
}

const ev = "caller-controlled deletion"

func TestFingerprintIsStable(t *testing.T) {
	if !bytes.Equal(fp(RepoAnchor("bad.sh"), ev), fp(RepoAnchor("bad.sh"), ev)) {
		t.Error("the same anchor and evidence produced different fingerprints")
	}
}

// A path and a URI that read the same are different places, so they must not
// share an identity.
func TestFingerprintSeparatesAnchorKinds(t *testing.T) {
	same := "example.com/a"
	seen := map[string]string{}
	for _, c := range []struct {
		kind   string
		anchor *pb.Anchor
	}{
		{"repo", RepoAnchor(same)},
		{"doc", docAnchor(same, "1")},
		{"arg", argAnchor(same, 1)},
		{"src", srcAnchor("https://" + same)},
	} {
		key := string(fp(c.anchor, ev))
		if prior, ok := seen[key]; ok {
			t.Errorf("%s collides with %s", c.kind, prior)
		}
		seen[key] = c.kind
	}
}

func TestFingerprintRepoNormalization(t *testing.T) {
	want := fp(RepoAnchor("bad.sh"), ev)
	for _, variant := range []string{"./bad.sh", "bad.sh"} {
		if !bytes.Equal(fp(RepoAnchor(variant), ev), want) {
			t.Errorf("%q produced a different identity than bad.sh", variant)
		}
	}
	if !bytes.Equal(fp(RepoAnchor("dir/"), ev), fp(RepoAnchor("dir"), ev)) {
		t.Error("a trailing slash changed identity")
	}
	if !bytes.Equal(fp(RepoAnchor("a\\b.sh"), ev), fp(RepoAnchor("a/b.sh"), ev)) {
		t.Error("a backslash separator changed identity")
	}
}

// A line moving does not make a new finding, and neither does the prose
// equivalent: the position within an artifact is not an identity input.
func TestFingerprintIgnoresPositionWithinArtifact(t *testing.T) {
	moved := &pb.Anchor{At: &pb.Anchor_RepoLine{RepoLine: &pb.RepoLine{
		Path: &pb.RepoPath{Value: "bad.sh"}, Line: u32(99),
	}}}
	if !bytes.Equal(fp(moved, ev), fp(RepoAnchor("bad.sh"), ev)) {
		t.Error("a line number changed identity")
	}

	symbol := &pb.Anchor{At: &pb.Anchor_RepoLine{RepoLine: &pb.RepoLine{
		Path: &pb.RepoPath{Value: "bad.sh"}, Symbol: strptr("main"),
	}}}
	if !bytes.Equal(fp(symbol, ev), fp(RepoAnchor("bad.sh"), ev)) {
		t.Error("a symbol name changed identity")
	}
}

// A section keeps the document's own numbering, so case and spacing in a
// heading reference must not fork the identity.
func TestFingerprintNormalizesLabels(t *testing.T) {
	want := fp(docAnchor("policy.md", "Appendix B"), ev)
	for _, variant := range []string{"appendix b", "APPENDIX  B", " Appendix B "} {
		if !bytes.Equal(fp(docAnchor("policy.md", variant), ev), want) {
			t.Errorf("section %q produced a different identity", variant)
		}
	}
}

// A step number is the claim's identity, not an offset into it: claim 3 is a
// different claim from claim 4.
func TestFingerprintDistinguishesArgumentSteps(t *testing.T) {
	if bytes.Equal(fp(argAnchor("plan", 3), ev), fp(argAnchor("plan", 4), ev)) {
		t.Error("two different claims in an argument share an identity")
	}
}

func TestFingerprintSourceIgnoresTrailingSlash(t *testing.T) {
	if !bytes.Equal(fp(srcAnchor("https://example.com/a/"), ev), fp(srcAnchor("https://example.com/a"), ev)) {
		t.Error("a trailing slash on a URI changed identity")
	}
}

func TestFingerprintTracksAnchorAndEvidence(t *testing.T) {
	base := fp(RepoAnchor("bad.sh"), ev)
	if bytes.Equal(fp(RepoAnchor("other.sh"), ev), base) {
		t.Error("a different anchor produced the same identity")
	}
	if bytes.Equal(fp(RepoAnchor("bad.sh"), "something else"), base) {
		t.Error("different evidence produced the same identity")
	}
	if bytes.Equal(Fingerprint(pb.Category_CATEGORY_COR, RepoAnchor("bad.sh"), ev), base) {
		t.Error("a different category produced the same identity")
	}
}

// Components are length-prefixed so no component's content can reach across its
// own boundary. Protobuf permits a NUL in a string, so a component that could
// contain the separator could forge one.
func TestFingerprintResistsComponentSmuggling(t *testing.T) {
	if bytes.Equal(fp(docAnchor("policy.md", "4.2"), ev), fp(docAnchor("policy.md\x004.2", ""), ev)) {
		t.Error("a NUL inside an anchor forged a boundary within the anchor")
	}

	// The case a separator alone misses: the NUL moves the boundary between two
	// adjacent components, so a longer anchor and a shorter evidence string hash
	// the same as the reverse.
	if bytes.Equal(fp(RepoAnchor("a\x00b"), "c"), fp(RepoAnchor("a"), "b\x00c")) {
		t.Error("a NUL moved the boundary between the anchor and the evidence")
	}

	// A NUL moving the boundary between two nested parts of one anchor. The
	// earlier form of this case compared ("a","b") against ("a\0b","") and
	// passed only because the empty section left a trailing separator, so it
	// could never have caught the collision below.
	if bytes.Equal(fp(docAnchor("a\x00b", "c"), ev), fp(docAnchor("a", "b\x00c"), ev)) {
		t.Error("a NUL moved the boundary between the document and its section")
	}
	if bytes.Equal(fp(argAnchor("a\x00b", 1), ev), fp(argAnchor("a", 0), "")) {
		t.Error("a NUL moved the boundary between the argument and its step")
	}
}

// Every anchor kind with more than one part must resist the same shift, so the
// guarantee does not depend on which kind happens to be tested.
func TestFingerprintNestedPartsAreDelimited(t *testing.T) {
	seen := map[string]string{}
	for _, c := range []struct {
		name   string
		anchor *pb.Anchor
	}{
		{"doc(ab, c)", docAnchor("ab", "c")},
		{"doc(a, bc)", docAnchor("a", "bc")},
		{"doc(a NUL b, c)", docAnchor("a\x00b", "c")},
		{"doc(a, b NUL c)", docAnchor("a", "b\x00c")},
		{"arg(ab, 1)", argAnchor("ab", 1)},
		{"arg(a, 1)", argAnchor("a", 1)},
		{"arg(a NUL 1, 1)", argAnchor("a\x001", 1)},
	} {
		key := string(fp(c.anchor, ev))
		if prior, ok := seen[key]; ok {
			t.Errorf("%s collides with %s", c.name, prior)
		}
		seen[key] = c.name
	}
}

// A component's length is part of the hash, so adjacent components cannot be
// re-partitioned without changing identity.
func TestFingerprintComponentsAreLengthDelimited(t *testing.T) {
	seen := map[string]string{}
	for _, c := range []struct {
		name   string
		anchor *pb.Anchor
		ev     string
	}{
		{"anchor ab, evidence c", RepoAnchor("ab"), "c"},
		{"anchor a, evidence bc", RepoAnchor("a"), "bc"},
		{"anchor a NUL b, evidence c", RepoAnchor("a\x00b"), "c"},
		{"anchor a, evidence b NUL c", RepoAnchor("a"), "b\x00c"},
	} {
		key := string(fp(c.anchor, c.ev))
		if prior, ok := seen[key]; ok {
			t.Errorf("%q collides with %q", c.name, prior)
		}
		seen[key] = c.name
	}
}

func TestFindingIDFormat(t *testing.T) {
	id := FindingID(pb.Category_CATEGORY_SEC, fp(RepoAnchor("bad.sh"), ev), false)
	if len(id) != len("FG-SEC-")+12 {
		t.Errorf("id = %q, want a 12-hex suffix", id)
	}
	wide := FindingID(pb.Category_CATEGORY_SEC, fp(RepoAnchor("bad.sh"), ev), true)
	if len(wide) != len("FG-SEC-")+16 {
		t.Errorf("wide id = %q, want a 16-hex suffix", wide)
	}
	if wide[:len(id)] != id {
		t.Error("the wide id is not an extension of the narrow one")
	}
}

func TestAnchorKeyUnsetAnchor(t *testing.T) {
	kind, parts := AnchorKey(nil)
	if kind != "none" || len(parts) != 0 {
		t.Errorf("AnchorKey(nil) = %q, %q; want \"none\", no parts", kind, parts)
	}
}

func timeFixture() time.Time { return time.Unix(1780000000, 0).UTC() }

func strptr(s string) *string { return &s }

func u32(v uint32) *uint32 { return &v }
