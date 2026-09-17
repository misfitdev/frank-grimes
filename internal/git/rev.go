package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotRange reports a string git does not read as a revision range.
var ErrNotRange = errors.New("not a revision range")

// Range is a revision range resolved to the two commits it spans.
//
// Held resolved rather than as the caller's spelling: "HEAD^..HEAD" names
// different commits after every commit, and a record that reported the spelling
// could not say afterwards which change was reviewed.
type Range struct {
	From string
	To   string
}

// String renders the range the way a record names it.
func (r Range) String() string { return r.From + ".." + r.To }

// ResolveRange reads a revision range spelling and returns the commits it
// spans.
//
// Both of git's spellings are accepted. "A..B" is what B has that A does not;
// "A...B" is what B has since the two diverged, which is the range a branch
// under review actually contributed. An omitted side is HEAD, so "..topic" and
// "main.." both mean what they do everywhere else in git.
func (r *Repo) ResolveRange(ctx context.Context, spelling string) (Range, error) {
	left, right, symmetric, ok := splitRange(spelling)
	if !ok {
		return Range{}, fmt.Errorf("%w: %q", ErrNotRange, spelling)
	}

	to, err := r.commit(ctx, right)
	if err != nil {
		return Range{}, fmt.Errorf("%w: %q: %v", ErrNotRange, spelling, err)
	}
	from, err := r.commit(ctx, left)
	if err != nil {
		return Range{}, fmt.Errorf("%w: %q: %v", ErrNotRange, spelling, err)
	}
	if symmetric {
		if from, err = r.mergeBase(ctx, from, to); err != nil {
			return Range{}, fmt.Errorf("%w: %q: %v", ErrNotRange, spelling, err)
		}
	}
	return Range{From: from, To: to}, nil
}

// splitRange separates the two sides of a range spelling. The three-dot form is
// looked for first, since every symmetric spelling also contains a two-dot one.
func splitRange(spelling string) (left, right string, symmetric, ok bool) {
	sep := "..."
	i := strings.Index(spelling, sep)
	if i < 0 {
		sep = ".."
		if i = strings.Index(spelling, sep); i < 0 {
			return "", "", false, false
		}
	}
	left, right = spelling[:i], spelling[i+len(sep):]
	if left == "" {
		left = "HEAD"
	}
	if right == "" {
		right = "HEAD"
	}
	return left, right, sep == "...", true
}

// commit resolves one side of a range to a commit.
//
// Peeled to a commit, so an annotated tag names the commit it points at rather
// than the tag object, which nothing downstream can diff. --end-of-options is
// what keeps a side beginning with a dash from being read as an option.
func (r *Repo) commit(ctx context.Context, rev string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%q names no commit", rev)
	}
	name := strings.TrimSpace(out)
	if name == "" {
		return "", fmt.Errorf("%q names no commit", rev)
	}
	return name, nil
}

func (r *Repo) mergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := r.run(ctx, "merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("%s and %s have no common ancestor", short(a), short(b))
	}
	return strings.TrimSpace(out), nil
}

// RangeFiles lists the repository-relative paths the range changed and that
// still exist at its far end.
//
// A file the range deleted is left out. There are no bytes to review, no line
// for a finding to anchor to, and counting it would inflate the denominator
// coverage is measured against with a unit no reviewer could examine.
func (r *Repo) RangeFiles(ctx context.Context, rg Range) ([]string, error) {
	out, err := r.run(ctx, "diff", "--name-only", "-z", "--diff-filter=d",
		"--no-renames", "--end-of-options", rg.From, rg.To)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func short(name string) string {
	if len(name) > 12 {
		return name[:12]
	}
	return name
}
