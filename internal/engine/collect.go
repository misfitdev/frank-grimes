package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/git"
)

// maxUnitLabel is the contract's bound on TargetUnit.label.
const maxUnitLabel = 512

// maxDocumentLine bounds one line of a document, so a pathological file cannot
// make collection allocate without limit.
const maxDocumentLine = 4 * 1024 * 1024

// TargetCollector resolves a caller's target into a fingerprinted target and
// the inventory of units a review is accountable for.
//
// The fingerprint is taken over the target's content, so a target is immutable
// for the length of a review: editing it while the loop runs produces a
// different fingerprint and the ledger raised against the old one is refused.
//
// Kind selects how a target is read and what a unit is. It never selects a
// verdict rule.
type TargetCollector struct {
	// Stdin supplies an idea target given as "-". Nil reads os.Stdin.
	Stdin io.Reader
}

func (c TargetCollector) Collect(ctx context.Context, spec TargetSpec) (*Collected, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spec.Scope == "" {
		return nil, fmt.Errorf("target needs a scope")
	}

	var (
		out *Collected
		err error
	)
	switch spec.Kind {
	case pb.TargetKind_TARGET_KIND_CODE:
		out, err = c.collectCode(ctx, spec)
	case pb.TargetKind_TARGET_KIND_DOCUMENT:
		out, err = c.collectDocument(spec)
	case pb.TargetKind_TARGET_KIND_IDEA:
		out, err = c.collectIdea(spec)
	case pb.TargetKind_TARGET_KIND_EXTERNAL:
		out, err = c.collectExternal(spec)
	default:
		// Falling through to code would review a caller's unnamed kind as a
		// repository path and report a verdict over the wrong thing.
		return nil, fmt.Errorf("unsupported target kind %v", spec.Kind)
	}
	if err != nil {
		return nil, err
	}

	out.Categories = spec.Categories
	if len(out.Categories) == 0 {
		out.Categories = AllCategories
	}
	return out, nil
}

// collected assembles what every kind returns in common.
func collected(target *pb.Target, units []*pb.TargetUnit, contentPath string) *Collected {
	// Absolute, because the provider runs in the review directory while
	// collection resolved this against its own. The same relative name in both
	// is two different files, and the provider would review the one nobody
	// fingerprinted.
	if contentPath != "" {
		if abs, err := filepath.Abs(contentPath); err == nil {
			contentPath = abs
		}
	}
	return &Collected{
		Target: target,
		Inventory: &pb.TargetInventory{
			SchemaMajor:             contracts.SchemaMajor,
			TargetFingerprintSha256: target.GetFingerprintSha256(),
			Units:                   units,
		},
		ContentPath: contentPath,
	}
}

// asRange decides whether the scope names a revision range rather than a path,
// and returns the range it resolved to.
//
// What is on disk wins: an operator who names a file gets that file, whatever
// it is called. A scope that reads as both is refused rather than guessed at,
// because the two readings select different files and the run record would not
// say which was meant.
func (c TargetCollector) asRange(ctx context.Context, spec TargetSpec) (*git.Range, error) {
	// Every range spelling contains it, and no other scope is even a candidate,
	// so nothing below runs for an ordinary path.
	if !strings.Contains(spec.Scope, "..") {
		return nil, nil
	}
	repo, err := git.Open(ctx, spec.Root)
	if err != nil {
		return nil, nil
	}
	resolved, err := repo.ResolveRange(ctx, spec.Scope)
	if err != nil {
		return nil, nil
	}
	if _, err := os.Lstat(filepath.Join(spec.Root, spec.Scope)); err == nil {
		return nil, fmt.Errorf(
			"%q names both a path in the repository and the revision range %s; "+
				"write ./%s for the path, or the resolved range for the commits",
			spec.Scope, resolved, spec.Scope)
	}
	return &resolved, nil
}

// collectRange resolves a target from the files a range of commits changed.
//
// The range selects which files are under review; the bytes are the ones in the
// tree. A review that fixes what it finds edits the tree, so an inventory built
// from an older version of a file would anchor findings to lines no role could
// read and no gate could run over.
func (c TargetCollector) collectRange(ctx context.Context, spec TargetSpec, resolved *git.Range) (*Collected, error) {
	repo, err := git.Open(ctx, spec.Root)
	if err != nil {
		return nil, err
	}
	rel, err := repo.RangeFiles(ctx, *resolved)
	if err != nil {
		return nil, fmt.Errorf("range target %q: %w", spec.Scope, err)
	}
	base, err := filepath.Abs(spec.Root)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, p := range rel {
		abs := filepath.Join(base, p)
		// A file the range changed and something later removed is not reviewable
		// now, the same as one the range itself deleted.
		if info, err := os.Lstat(abs); err != nil || !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, abs)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("range target %q (%s) changed no file that is still here to review",
			spec.Scope, resolved)
	}
	sort.Strings(paths)

	// The resolved commits seed the fingerprint, so that two ranges over one
	// unchanged tree are two targets. Without it a review of an earlier range
	// could be resumed as though it were this one.
	seed := sha256.New()
	fmt.Fprintf(seed, "range\x00%s\x00%s\n", resolved.From, resolved.To)

	units, digests, bodies, sum, err := codeUnits(spec, base, paths, seed)
	if err != nil {
		return nil, err
	}

	// Scope carries the resolved commits and display carries what the operator
	// wrote. "HEAD^..HEAD" names a different change after every commit, and the
	// record has to say which one this was.
	out := collected(&pb.Target{
		Root:              spec.Root,
		Scope:             resolved.String(),
		FingerprintSha256: sum,
		Display:           truncate(spec.Scope),
		Kind:              pb.TargetKind_TARGET_KIND_CODE,
	}, units, base)
	out.UnitDigests = digests
	out.UnitBodies = bodies
	out.Range = true
	return out, nil
}

// collectCode fingerprints every file under the target path and makes each one
// a unit.
//
// Ignore rules are not consulted: a target is what is on disk, and a review
// that silently skipped a generated or ignored file would report coverage it
// does not have. Only .git and the engine's own .grimes directory are skipped,
// neither being part of any target.
func (c TargetCollector) collectCode(ctx context.Context, spec TargetSpec) (*Collected, error) {
	if spec.Root == "" {
		return nil, fmt.Errorf("a code target needs a repository root")
	}
	switch rangeSpec, err := c.asRange(ctx, spec); {
	case err != nil:
		return nil, err
	case rangeSpec != nil:
		return c.collectRange(ctx, spec, rangeSpec)
	}
	abs, base, err := contained(spec.Root, spec.Scope)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
	}

	var paths []string
	if info.IsDir() {
		err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == ".git" || name == ".grimes" {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			paths = append(paths, p)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
		}
	} else {
		// A walk skips anything that is not a regular file; a target named
		// directly has to be held to the same rule, since reading a FIFO or a
		// device blocks with nothing to cancel it.
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("code target %q is not a regular file", spec.Scope)
		}
		paths = []string{abs}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("code target %q holds no files to review", spec.Scope)
	}
	sort.Strings(paths)

	units, digests, bodies, sum, err := codeUnits(spec, base, paths, sha256.New())
	if err != nil {
		return nil, err
	}

	// The scope itself: a tree when it names a directory, one file when it names
	// a file. Both are supported targets, and the fingerprint is taken over
	// path-and-digest pairs either way.
	out := collected(&pb.Target{
		Root:              spec.Root,
		Scope:             spec.Scope,
		FingerprintSha256: sum,
		Display:           truncate(spec.Scope),
		Kind:              pb.TargetKind_TARGET_KIND_CODE,
	}, units, abs)
	out.UnitDigests = digests
	out.UnitBodies = bodies
	return out, nil
}

// codeUnits reads each path and folds it into the inventory and the
// fingerprint. seed carries whatever already identifies the target beyond its
// files, so that two targets holding identical bytes are still distinguishable.
func codeUnits(spec TargetSpec, base string, paths []string, seed hash.Hash) (
	units []*pb.TargetUnit, digests, bodies map[string][]byte, sum []byte, err error) {
	units = make([]*pb.TargetUnit, 0, len(paths))
	digests = make(map[string][]byte, len(paths))
	if spec.KeepBodies {
		bodies = make(map[string][]byte, len(paths))
	}
	for _, p := range paths {
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
		}
		body := sha256.Sum256(content)
		// Path and content digest both enter the fingerprint, so renaming a file
		// or editing it are each a different target.
		fmt.Fprintf(seed, "%s\x00%s\n", rel, hex.EncodeToString(body[:]))
		units = append(units, unit(rel, rel))
		digests[rel] = append([]byte(nil), body[:]...)
		if bodies != nil {
			bodies[rel] = content
		}
	}
	return units, digests, bodies, seed.Sum(nil), nil
}

// contained resolves a code scope against its root and refuses one that leaves
// it.
//
// A review names the repository it covers, and a scope of "../elsewhere" would
// walk files the caller did not put under review while the ledger still claims
// the named root. Symlinks are resolved first, so a link out of the tree is
// refused the same way a "../" is.
func contained(root, scope string) (abs, base string, err error) {
	// Absolute first, then resolved: EvalSymlinks leaves a relative root
	// relative, and comparing a relative base against a resolved target reads
	// every target as an escape.
	if base, err = filepath.Abs(root); err != nil {
		return "", "", err
	}
	if base, err = filepath.EvalSymlinks(base); err != nil {
		return "", "", fmt.Errorf("repository root %q: %w", root, err)
	}
	if abs, err = filepath.Abs(filepath.Join(base, scope)); err != nil {
		return "", "", err
	}
	// A missing target is reported by the caller's stat, so an unresolvable
	// path is passed through rather than reported as an escape. What it is
	// checked against is the deepest part of it that does exist: "link/missing"
	// resolves to nothing, and reading only its spelling would miss that link
	// leaves the tree.
	resolved, err := resolveExisting(abs)
	if err != nil {
		return "", "", err
	}
	if full, err := filepath.EvalSymlinks(abs); err == nil {
		abs = full
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("code target %q resolves outside the repository root", scope)
	}
	return abs, base, nil
}

// resolveExisting returns the path with its longest existing prefix resolved,
// so a name that does not exist yet is still judged by where it would sit.
//
// EvalSymlinks fails outright on a missing component, and falling back to the
// spelling of the path would read "link/missing" as inside the tree however far
// outside link points. A link whose own target does not exist is followed by
// hand for the same reason: it still says where the path would lead.
func resolveExisting(path string) (string, error) {
	// Bounded, because a link that points at itself would otherwise be followed
	// forever. The limit is the usual kernel one.
	const maxLinks = 40
	var missing []string
	rejoin := func(base string) string {
		return filepath.Join(append([]string{base}, missing...)...)
	}
	for range maxLinks {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return rejoin(resolved), nil
		}
		if link, err := os.Readlink(path); err == nil {
			if !filepath.IsAbs(link) {
				link = filepath.Join(filepath.Dir(path), link)
			}
			path = link
			continue
		}
		parent := filepath.Dir(path)
		// The root resolves or nothing does; without this a malformed path
		// would climb forever.
		if parent == path {
			return rejoin(path), nil
		}
		missing = append([]string{filepath.Base(path)}, missing...)
		path = parent
	}
	return "", fmt.Errorf("too many symbolic links resolving %q", path)
}

// readRegular reads a file-backed target, refusing anything that is not a
// regular file.
//
// A FIFO or an unbounded device would block inside os.ReadFile with nothing to
// cancel it: collection checks the context before dispatch and not during a
// read.
func readRegular(kind, path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%s target %q: %w", kind, path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s target %q is not a regular file", kind, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s target %q: %w", kind, path, err)
	}
	return content, nil
}

// collectDocument fingerprints a document's bytes and makes each heading a
// unit. No repository is required.
func (c TargetCollector) collectDocument(spec TargetSpec) (*Collected, error) {
	content, err := readRegular("document", spec.Scope)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(content)
	target := &pb.Target{
		Scope:             spec.Scope,
		FingerprintSha256: sum[:],
		Display:           truncate(filepath.Base(spec.Scope)),
		Kind:              pb.TargetKind_TARGET_KIND_DOCUMENT,
	}
	units, err := sections(string(content), spec.Scope)
	if err != nil {
		return nil, err
	}
	return collected(target, units, spec.Scope), nil
}

// collectIdea fingerprints an argument's text and makes each paragraph a step.
// "-" reads the argument from stdin, since a pasted argument has no file.
func (c TargetCollector) collectIdea(spec TargetSpec) (*Collected, error) {
	var (
		content []byte
		err     error
		scope   = spec.Scope
	)
	if spec.Scope == "-" {
		in := c.Stdin
		if in == nil {
			in = os.Stdin
		}
		content, err = io.ReadAll(in)
		scope = "stdin"
	} else {
		content, err = readRegular("idea", spec.Scope)
	}
	if err != nil {
		return nil, fmt.Errorf("idea target %q: %w", spec.Scope, err)
	}
	steps := paragraphs(string(content))
	if len(steps) == 0 {
		return nil, fmt.Errorf("idea target %q holds no argument to review", spec.Scope)
	}
	sum := sha256.Sum256(content)
	out := collected(&pb.Target{
		Scope:             scope,
		FingerprintSha256: sum[:],
		Display:           truncate(scope),
		Kind:              pb.TargetKind_TARGET_KIND_IDEA,
	}, steps, spec.Scope)
	if spec.Scope == "-" {
		// A pasted argument has no path of its own. The engine persists these
		// and fills in where it put them.
		out.ContentPath = ""
		out.ContentBytes = content
	}
	return out, nil
}

// collectExternal fingerprints a frozen snapshot of an external source.
//
// Nothing here fetches the URI. A review must be repeatable and a network fetch
// is not, so the snapshot the reviewer already took is the target and the URI
// only names where it came from.
func (c TargetCollector) collectExternal(spec TargetSpec) (*Collected, error) {
	if spec.Snapshot == "" {
		return nil, fmt.Errorf("an external target needs a frozen snapshot; nothing here fetches %q", spec.Scope)
	}
	content, err := readRegular("external snapshot", spec.Snapshot)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(content)
	// The snapshot is the target: pointing at the file the reviewer froze is
	// what makes an external review repeatable.
	return collected(&pb.Target{
		Scope:             spec.Scope,
		FingerprintSha256: sum[:],
		Display:           truncate(spec.Scope),
		Kind:              pb.TargetKind_TARGET_KIND_EXTERNAL,
	}, []*pb.TargetUnit{unit(spec.Scope, spec.Scope)}, spec.Snapshot), nil
}

// sections splits a document at its ATX headings. A document with no heading is
// one unit, since the whole of it is still accountable.
//
// A fenced block is skipped: a shell comment or a preprocessor line inside one
// is not a section, and counting it would inflate the denominator coverage is
// measured against.
func sections(text, name string) ([]*pb.TargetUnit, error) {
	spans, err := sectionSpans(text, name)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return []*pb.TargetUnit{unit(name, filepath.Base(name))}, nil
	}
	units := make([]*pb.TargetUnit, 0, len(spans))
	for _, s := range spans {
		units = append(units, s.unit)
	}
	return units, nil
}

// docSection is one heading and the lines it covers, which run to the next
// heading. Evidence quoted "in" a section has to be checkable against the lines
// that section actually holds.
type docSection struct {
	unit  *pb.TargetUnit
	start int // index of the heading line
	end   int // exclusive
}

// sectionSpans splits a document at its ATX headings.
//
// A fenced block is skipped: a shell comment or a preprocessor line inside one
// is not a section, and counting it would inflate the denominator coverage is
// measured against.
func sectionSpans(text, name string) ([]docSection, error) {
	var (
		spans []docSection
		taken = map[string]int{}
		open  fence
		n     int
	)
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), maxDocumentLine)
	for ; scanner.Scan(); n++ {
		line := strings.TrimRight(scanner.Text(), "\r")
		// A block ends only on the delimiter that opened it, at no less than its
		// length. Ending it early would expose the commented-out lines of a code
		// sample as sections that do not exist.
		if f := fenceOf(line); f.n > 0 {
			switch {
			case open.n == 0:
				open = f
			case f.closes(open):
				open = fence{}
			}
			continue
		}
		if open.n > 0 || !strings.HasPrefix(line, "#") {
			continue
		}
		// ATX requires a space after the hashes, so "#nottag" is text.
		rest := strings.TrimLeft(line, "#")
		if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
			continue
		}
		title := strings.TrimSpace(rest)
		if title == "" {
			continue
		}
		if len(spans) > 0 {
			spans[len(spans)-1].end = n
		}
		spans = append(spans, docSection{
			unit:  unit(uniqueID(slug(title), title, taken), title),
			start: n,
		})
	}
	// A line past the buffer stops the scan, and the headings after it would
	// silently vanish from an inventory that still claims the whole document.
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("document %q: %w", name, err)
	}
	if len(spans) > 0 {
		spans[len(spans)-1].end = n
	}
	return spans, nil
}

// sectionBody returns the lines the named section covers. A document with no
// heading is one unit, so the whole of it is the body.
func sectionBody(text, id string) string {
	spans, err := sectionSpans(text, "")
	if err != nil || len(spans) == 0 {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, s := range spans {
		if s.unit.GetId() != id {
			continue
		}
		if s.end > len(lines) {
			s.end = len(lines)
		}
		return strings.Join(lines[s.start:s.end], "\n")
	}
	return ""
}

// stepBody returns the paragraph the numbered step covers, matching how
// paragraphs numbered them.
func stepBody(text, id string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "step-"))
	if err != nil || n < 1 {
		return ""
	}
	seen := 0
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		seen++
		if seen == n {
			return block
		}
	}
	return ""
}

// opener returns the fence delimiter a line opens or closes with, or "" when
// the line is not a fence.
// fence describes a fenced block's delimiter: which character runs it, and how
// long the run is. A block closes only on the same character at no less than
// the opening length, so a document may quote a shorter fence inside a longer
// one.
type fence struct {
	char byte
	n    int
	// info is what follows the run. Only an opening fence may carry one, so a
	// "```text" line inside a block is a quoted opener, not a close.
	info string
}

// fenceOf returns the fence a line opens or closes with, or a zero fence when
// the line is not one.
func fenceOf(line string) fence {
	trimmed := strings.TrimSpace(line)
	for _, c := range []byte{'`', '~'} {
		n := 0
		for n < len(trimmed) && trimmed[n] == c {
			n++
		}
		if n >= 3 {
			return fence{char: c, n: n, info: strings.TrimSpace(trimmed[n:])}
		}
	}
	return fence{}
}

// closes reports whether f ends a block opened by open.
func (f fence) closes(open fence) bool {
	return f.char == open.char && f.n >= open.n && f.info == ""
}

// uniqueID keeps a unit's identifier non-empty and distinct.
//
// A heading of "---" or one made only of emoji slugs to nothing, and the
// contract refuses an empty id; two identical headings would otherwise share
// one id and coverage could not tell them apart.
func uniqueID(id, title string, taken map[string]int) string {
	if id == "" {
		sum := sha256.Sum256([]byte(title))
		id = "section-" + hex.EncodeToString(sum[:4])
	}
	taken[id]++
	if n := taken[id]; n > 1 {
		return fmt.Sprintf("%s-%d", id, n)
	}
	return id
}

// paragraphs numbers an argument's steps. A step is the unit an argument is
// made of, so the number is its identity rather than an offset into a file.
func paragraphs(text string) []*pb.TargetUnit {
	var units []*pb.TargetUnit
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		n := len(units) + 1
		units = append(units, unit(fmt.Sprintf("step-%d", n), truncate(block)))
	}
	return units
}

func unit(id, label string) *pb.TargetUnit {
	return &pb.TargetUnit{Id: id, Label: truncate(label)}
}

// slug renders a heading comparable the way normalizeLabel does for anchors:
// case-folded, single-dashed.
func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// truncate holds a label inside the contract's bound without splitting a rune.
func truncate(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxUnitLabel {
		return s
	}
	cut := maxUnitLabel
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut]
}

func utf8Start(b byte) bool {
	return b&0xC0 != 0x80
}
