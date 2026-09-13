package engine

import (
	"context"
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
		units := sections(c.text, "docs/spec.md")
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
	if got := len(sections(crlf, "d.md")); got != 2 {
		t.Errorf("CRLF document produced %d units, want 2", got)
	}
	if got := sections("# Same\n# Same\n", "d.md"); len(got) != 2 {
		t.Errorf("repeated heading produced %d units, want 2", len(got))
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
		{"---", ""},
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

func TestCollectRefusesAnEmptyScope(t *testing.T) {
	if _, _, _, err := (TargetCollector{}).Collect(context.Background(), TargetSpec{Root: "/repo"}); err == nil {
		t.Error("a target with no scope was accepted")
	}
}

func TestCollectIdeaReadsStdin(t *testing.T) {
	c := TargetCollector{Stdin: strings.NewReader("one\n\ntwo\n")}
	target, inventory, _, err := c.Collect(context.Background(), TargetSpec{
		Scope: "-", Kind: pb.TargetKind_TARGET_KIND_IDEA,
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.GetRoot() != "" {
		t.Errorf("root = %q, want empty for an idea", target.GetRoot())
	}
	if target.GetScope() != "stdin" {
		t.Errorf("scope = %q, want stdin", target.GetScope())
	}
	if len(inventory.GetUnits()) != 2 {
		t.Errorf("got %d steps, want 2", len(inventory.GetUnits()))
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
