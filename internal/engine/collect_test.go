package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// Unit splitting is pure derivation over text, and these inputs are awkward to
// express as files in a shell suite. Everything observable through the CLI is
// asserted in tests/test-collector.sh instead.

func TestSectionsFallsBackToTheWholeDocument(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"no headings", "just prose\nand more prose\n"},
		{"hash with no title", "#\n##   \ntext\n"},
	} {
		units, err := sections(c.text, "docs/spec.md")
		if err != nil {
			t.Fatal(err)
		}
		if len(units) != 1 {
			t.Errorf("%s: got %d units, want the whole document as one", c.name, len(units))
			continue
		}
		if units[0].GetId() != "docs/spec.md" {
			t.Errorf("%s: unit id = %q, want the document path", c.name, units[0].GetId())
		}
	}
}

func TestSectionsSplitsHeadingsRegardlessOfLineEnding(t *testing.T) {
	crlf := "# One\r\ntext\r\n## Two\r\nmore\r\n"
	if got := mustSections(t, crlf); len(got) != 2 {
		t.Errorf("CRLF document produced %d units, want 2", len(got))
	}
}

func mustSections(t *testing.T, text string) []*pb.TargetUnit {
	t.Helper()
	units, err := sections(text, "d.md")
	if err != nil {
		t.Fatal(err)
	}
	return units
}

// A unit id is what coverage will name a section by, so two sections cannot
// share one, and none may be empty: the contract refuses an empty id and the
// whole document would then be unreviewable.
func TestSectionIDsAreUniqueAndNonEmpty(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want int
	}{
		{"repeated heading", "# Same\n# Same\n# Same\n", 3},
		{"punctuation only", "# ---\n## ***\n", 2},
		{"no slug characters", "# \U0001F512\n", 1},
	} {
		units := mustSections(t, c.text)
		if len(units) != c.want {
			t.Errorf("%s: got %d units, want %d", c.name, len(units), c.want)
			continue
		}
		seen := map[string]bool{}
		for _, u := range units {
			if u.GetId() == "" {
				t.Errorf("%s: a unit has an empty id", c.name)
			}
			if seen[u.GetId()] {
				t.Errorf("%s: id %q used twice", c.name, u.GetId())
			}
			seen[u.GetId()] = true
		}
	}
}

// A hash inside a fenced block is a comment or a directive, not a section.
// Counting it would inflate the denominator coverage is measured against.
func TestSectionsIgnoreFencedBlocksAndNonHeadings(t *testing.T) {
	text := "# Real\n\n```sh\n# not a heading\n#also not\n```\n\n## Also Real\n\n#nospace\n"
	units := mustSections(t, text)
	if len(units) != 2 {
		var got []string
		for _, u := range units {
			got = append(got, u.GetLabel())
		}
		t.Errorf("got %d units (%v), want 2", len(units), got)
	}
}

// A line past the scanner's buffer stops the scan. Returning the headings found
// so far would hand back an inventory that silently omits the rest of a
// document whose fingerprint still covers all of it.
func TestSectionsReportAnOverlongLine(t *testing.T) {
	text := "# One\n" + strings.Repeat("x", maxDocumentLine+1) + "\n# Two\n"
	if _, err := sections(text, "d.md"); err == nil {
		t.Error("an unscannable document was collected anyway")
	}
}

// A slug is what coverage will name a section by, so two headings that differ
// only in punctuation or case must not collide into one identifier.
func TestSlugIsStableAndDistinguishing(t *testing.T) {
	for _, c := range []struct{ title, want string }{
		{"Authentication", "authentication"},
		{"  Spaced   Out  ", "spaced-out"},
		{"Phase 1: The Grimey Read", "phase-1-the-grimey-read"},
		{"AUTHENTICATION", "authentication"},
	} {
		if got := slug(c.title); got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.title, got, c.want)
		}
	}
	if slug("Storage") == slug("Authentication") {
		t.Error("distinct headings produced one slug")
	}
}

func TestParagraphsNumbersSteps(t *testing.T) {
	units := paragraphs("first\n\nsecond\n\n\n\nthird\n")
	if len(units) != 3 {
		t.Fatalf("got %d steps, want 3", len(units))
	}
	for i, want := range []string{"step-1", "step-2", "step-3"} {
		if units[i].GetId() != want {
			t.Errorf("step %d id = %q, want %q", i, units[i].GetId(), want)
		}
	}
	if got := paragraphs("only one\r\n\r\nand two\r\n"); len(got) != 2 {
		t.Errorf("CRLF argument produced %d steps, want 2", len(got))
	}
	if got := paragraphs("   \n\n  \n"); len(got) != 0 {
		t.Errorf("whitespace-only argument produced %d steps, want 0", len(got))
	}
}

// A label above the contract's bound would make the inventory unencodable, and
// cutting mid-rune would make it invalid UTF-8.
func TestTruncateHoldsTheLabelBoundWithoutSplittingARune(t *testing.T) {
	long := strings.Repeat("é", maxUnitLabel)
	got := truncate(long)
	if len(got) > maxUnitLabel {
		t.Errorf("label is %d bytes, want at most %d", len(got), maxUnitLabel)
	}
	if !strings.HasPrefix(long, got) {
		t.Error("truncation did not cut on a rune boundary")
	}
}

// The CLI refuses a missing snapshot before collection is reached, so this
// guard is only observable here. It is the guarantee that no code path fetches
// an external source, which is what makes an external review repeatable.
func TestCollectExternalRefusesAMissingSnapshot(t *testing.T) {
	_, _, _, err := (TargetCollector{}).Collect(context.Background(), TargetSpec{
		Scope: "https://example.com/policy", Kind: pb.TargetKind_TARGET_KIND_EXTERNAL,
	})
	if err == nil {
		t.Fatal("an external target with no snapshot was collected")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error %q does not name the missing snapshot", err)
	}
}

// The CLI maps kind names before collection, so an unnamed or future kind only
// reaches here. Falling through to code would review it as a repository path.
func TestCollectRefusesAnUnnamedKind(t *testing.T) {
	// A root that would collect cleanly as code, so the refusal can only be
	// about the kind.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.sh"), []byte("echo one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := (TargetCollector{}).Collect(context.Background(), TargetSpec{
		Root: root, Scope: "a.sh", Kind: pb.TargetKind_TARGET_KIND_CODE,
	}); err != nil {
		t.Fatalf("the control case did not collect: %v", err)
	}
	for _, kind := range []pb.TargetKind{
		pb.TargetKind_TARGET_KIND_UNSPECIFIED,
		pb.TargetKind(99),
	} {
		_, _, _, err := (TargetCollector{}).Collect(context.Background(), TargetSpec{
			Root: root, Scope: "a.sh", Kind: kind,
		})
		if err == nil {
			t.Errorf("kind %v was collected as a code target", kind)
		}
	}
}

// Only the delimiter that opened a block closes it. A quoted "~~~" inside a
// backtick block would otherwise end it and expose the commented-out lines
// after it as sections that do not exist.
func TestSectionsDoNotMixFenceDelimiters(t *testing.T) {
	text := "# Real\n\n```md\n~~~\n# not a heading\n~~~\n# still not\n```\n\n## Also Real\n"
	if units := mustSections(t, text); len(units) != 2 {
		var got []string
		for _, u := range units {
			got = append(got, u.GetLabel())
		}
		t.Errorf("got %d units (%v), want 2", len(units), got)
	}
}
