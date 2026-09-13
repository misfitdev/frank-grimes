package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
)

// maxUnitLabel is the contract's bound on TargetUnit.label.
const maxUnitLabel = 512

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

func (c TargetCollector) Collect(ctx context.Context, spec TargetSpec) (*pb.Target, *pb.TargetInventory, []pb.Category, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	if spec.Scope == "" {
		return nil, nil, nil, fmt.Errorf("target needs a scope")
	}

	var (
		target *pb.Target
		units  []*pb.TargetUnit
		err    error
	)
	switch spec.Kind {
	case pb.TargetKind_TARGET_KIND_DOCUMENT:
		target, units, err = c.collectDocument(spec)
	case pb.TargetKind_TARGET_KIND_IDEA:
		target, units, err = c.collectIdea(spec)
	case pb.TargetKind_TARGET_KIND_EXTERNAL:
		target, units, err = c.collectExternal(spec)
	default:
		target, units, err = c.collectCode(spec)
	}
	if err != nil {
		return nil, nil, nil, err
	}

	categories := spec.Categories
	if len(categories) == 0 {
		categories = AllCategories
	}
	inventory := &pb.TargetInventory{
		SchemaMajor:             contracts.SchemaMajor,
		TargetFingerprintSha256: target.GetFingerprintSha256(),
		Units:                   units,
	}
	return target, inventory, categories, nil
}

// collectCode fingerprints every file under the target path and makes each one
// a unit.
//
// Ignore rules are not consulted: a target is what is on disk, and a review
// that silently skipped a generated or ignored file would report coverage it
// does not have. Only .git and the engine's own .grimes directory are skipped,
// neither being part of any target.
func (c TargetCollector) collectCode(spec TargetSpec) (*pb.Target, []*pb.TargetUnit, error) {
	if spec.Root == "" {
		return nil, nil, fmt.Errorf("a code target needs a repository root")
	}
	abs := filepath.Join(spec.Root, spec.Scope)
	info, err := os.Stat(abs)
	if err != nil {
		return nil, nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
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
			return nil, nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
		}
	} else {
		paths = []string{abs}
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("code target %q holds no files to review", spec.Scope)
	}
	sort.Strings(paths)

	sum := sha256.New()
	units := make([]*pb.TargetUnit, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(spec.Root, p)
		if err != nil {
			return nil, nil, err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("code target %q: %w", spec.Scope, err)
		}
		body := sha256.Sum256(content)
		// Path and content digest both enter the fingerprint, so renaming a file
		// or editing it are each a different target.
		fmt.Fprintf(sum, "%s\x00%s\n", rel, hex.EncodeToString(body[:]))
		units = append(units, unit(rel, rel))
	}

	return &pb.Target{
		Root:              spec.Root,
		Scope:             spec.Scope,
		FingerprintSha256: sum.Sum(nil),
		Display:           truncate(spec.Scope),
		Kind:              pb.TargetKind_TARGET_KIND_CODE,
	}, units, nil
}

// collectDocument fingerprints a document's bytes and makes each heading a
// unit. No repository is required.
func (c TargetCollector) collectDocument(spec TargetSpec) (*pb.Target, []*pb.TargetUnit, error) {
	content, err := os.ReadFile(spec.Scope)
	if err != nil {
		return nil, nil, fmt.Errorf("document target %q: %w", spec.Scope, err)
	}
	sum := sha256.Sum256(content)
	return &pb.Target{
		Scope:             spec.Scope,
		FingerprintSha256: sum[:],
		Display:           truncate(filepath.Base(spec.Scope)),
		Kind:              pb.TargetKind_TARGET_KIND_DOCUMENT,
	}, sections(string(content), spec.Scope), nil
}

// collectIdea fingerprints an argument's text and makes each paragraph a step.
// "-" reads the argument from stdin, since a pasted argument has no file.
func (c TargetCollector) collectIdea(spec TargetSpec) (*pb.Target, []*pb.TargetUnit, error) {
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
		content, err = os.ReadFile(spec.Scope)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("idea target %q: %w", spec.Scope, err)
	}
	steps := paragraphs(string(content))
	if len(steps) == 0 {
		return nil, nil, fmt.Errorf("idea target %q holds no argument to review", spec.Scope)
	}
	sum := sha256.Sum256(content)
	return &pb.Target{
		Scope:             scope,
		FingerprintSha256: sum[:],
		Display:           truncate(scope),
		Kind:              pb.TargetKind_TARGET_KIND_IDEA,
	}, steps, nil
}

// collectExternal fingerprints a frozen snapshot of an external source.
//
// Nothing here fetches the URI. A review must be repeatable and a network fetch
// is not, so the snapshot the reviewer already took is the target and the URI
// only names where it came from.
func (c TargetCollector) collectExternal(spec TargetSpec) (*pb.Target, []*pb.TargetUnit, error) {
	if spec.Snapshot == "" {
		return nil, nil, fmt.Errorf("an external target needs a frozen snapshot; nothing here fetches %q", spec.Scope)
	}
	content, err := os.ReadFile(spec.Snapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("external snapshot %q: %w", spec.Snapshot, err)
	}
	sum := sha256.Sum256(content)
	return &pb.Target{
		Scope:             spec.Scope,
		FingerprintSha256: sum[:],
		Display:           truncate(spec.Scope),
		Kind:              pb.TargetKind_TARGET_KIND_EXTERNAL,
	}, []*pb.TargetUnit{unit(spec.Scope, spec.Scope)}, nil
}

// sections splits a document at its ATX headings. A document with no heading is
// one unit, since the whole of it is still accountable.
func sections(text, name string) []*pb.TargetUnit {
	var units []*pb.TargetUnit
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(line, "#"))
		if title == "" {
			continue
		}
		units = append(units, unit(slug(title), title))
	}
	if len(units) == 0 {
		return []*pb.TargetUnit{unit(name, filepath.Base(name))}
	}
	return units
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
