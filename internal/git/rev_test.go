package git

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// Range spellings are git's, not this package's, and the ones that matter here
// are the ones a CLI test cannot reach cheaply: an omitted side, a spelling that
// is not a range at all, and two histories with nothing in common.

// commits adds one commit touching the named file and returns its object name.
func commits(t *testing.T, r *Repo, name, content string) string {
	t.Helper()
	write(t, filepath.Join(r.Dir, name), content)
	git(t, r.Dir, "add", "-A")
	git(t, r.Dir, "commit", "--quiet", "--message", "touch "+name)
	head, err := r.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func TestAnOmittedSideOfARangeIsHead(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	first, _ := r.Head(ctx)
	second := commits(t, r, "b.sh", "b\n")

	for _, tc := range []struct {
		spelling    string
		from, to    string
		whatItMeans string
	}{
		{spelling: first + "..", from: first, to: second, whatItMeans: "an omitted right side is HEAD"},
		{spelling: ".." + first, from: second, to: first, whatItMeans: "an omitted left side is HEAD"},
	} {
		got, err := r.ResolveRange(ctx, tc.spelling)
		if err != nil {
			t.Errorf("%s: %v", tc.whatItMeans, err)
			continue
		}
		if got.From != tc.from || got.To != tc.to {
			t.Errorf("%s: %q resolved to %s, want %s..%s",
				tc.whatItMeans, tc.spelling, got, tc.from, tc.to)
		}
	}
}

// The symmetric form is what one side contributed since the two diverged. Taken
// from the other tip instead, it would credit the range with every commit the
// other side made in the meantime.
func TestASymmetricRangeStartsAtTheMergeBase(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	base, _ := r.Head(ctx)
	main := commits(t, r, "main.sh", "main\n")
	git(t, r.Dir, "checkout", "--quiet", "-b", "topic", base)
	topic := commits(t, r, "topic.sh", "topic\n")

	got, err := r.ResolveRange(ctx, "main...topic")
	if err != nil {
		t.Fatal(err)
	}
	if got.From != base {
		t.Errorf("a symmetric range started at %s, want the merge base %s", got.From, base)
	}
	if got.To != topic {
		t.Errorf("a symmetric range ended at %s, want %s", got.To, topic)
	}
	if got.From == main {
		t.Error("a symmetric range started at the other branch's tip")
	}
}

func TestTwoHistoriesWithNothingInCommonAreNotARange(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	git(t, r.Dir, "checkout", "--quiet", "--orphan", "separate")
	git(t, r.Dir, "rm", "--quiet", "-rf", ".")
	commits(t, r, "alone.sh", "alone\n")

	_, err := r.ResolveRange(ctx, "main...separate")

	if !errors.Is(err, ErrNotRange) {
		t.Fatalf("err = %v, want ErrNotRange", err)
	}
	if !strings.Contains(err.Error(), "common ancestor") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// A scope that is not a range has to be refused rather than half-resolved, so
// that the collector can fall back to reading it as a path.
func TestWhatIsNotARangeIsRefusedAsOne(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	commits(t, r, "b.sh", "b\n")

	for _, tc := range []struct{ spelling, whatItIs string }{
		{"src", "a bare path"},
		{"", "nothing"},
		{"HEAD^^HEAD", "two revisions with no separator"},
		{"nosuchref..HEAD", "a left side naming no commit"},
		{"HEAD..nosuchref", "a right side naming no commit"},
		{"HEAD..HEAD..HEAD", "three sides"},
		// Read as an option by every git command it would be handed to.
		{"--output=/tmp/x..HEAD", "a left side that is a flag"},
		{"HEAD..--output=/tmp/x", "a right side that is a flag"},
	} {
		if _, err := r.ResolveRange(ctx, tc.spelling); !errors.Is(err, ErrNotRange) {
			t.Errorf("%s (%q): err = %v, want ErrNotRange", tc.whatItIs, tc.spelling, err)
		}
	}
}

// An annotated tag names a tag object, which nothing downstream can diff.
func TestATagResolvesToTheCommitItPointsAt(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	first, _ := r.Head(ctx)
	git(t, r.Dir, "tag", "--annotate", "v1", "--message", "one", first)
	second := commits(t, r, "b.sh", "b\n")

	got, err := r.ResolveRange(ctx, "v1..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got.From != first || got.To != second {
		t.Errorf("v1..HEAD resolved to %s, want %s..%s", got, first, second)
	}
}

func TestRangeFilesNamesWhatChangedAndSkipsWhatWentAway(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	write(t, filepath.Join(r.Dir, "gone.sh"), "gone\n")
	git(t, r.Dir, "add", "-A")
	git(t, r.Dir, "commit", "--quiet", "--message", "add one to remove")
	from, _ := r.Head(ctx)
	write(t, filepath.Join(r.Dir, "kept.sh"), "kept\n")
	git(t, r.Dir, "rm", "--quiet", "gone.sh")
	git(t, r.Dir, "add", "-A")
	git(t, r.Dir, "commit", "--quiet", "--message", "add one and remove one")
	to, _ := r.Head(ctx)

	got, err := r.RangeFiles(ctx, Range{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}

	names := strings.Join(got, " ")
	if !strings.Contains(names, "kept.sh") {
		t.Errorf("the range added kept.sh and did not report it; got %q", names)
	}
	// There are no bytes to review and no line to anchor a finding in, so
	// counting it would put a unit nobody can examine into the denominator.
	if strings.Contains(names, "gone.sh") {
		t.Errorf("the range deleted gone.sh and reported it anyway; got %q", names)
	}
}
