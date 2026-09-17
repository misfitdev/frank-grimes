package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/git"
)

// ErrFixNeedsRepository reports a fix run against something with no history to
// put a worktree on.
var ErrFixNeedsRepository = errors.New("fix mode needs a git repository")

// ErrBatchOutOfScope reports a batch that changed something the review never
// resolved.
var ErrBatchOutOfScope = errors.New("the batch changed files outside the reviewed scope")

// fixRun is the worktree one fix run edits in, and what it started from.
//
// One per review directory rather than one per run: an iteration is a process,
// and the next one has to find what the last one built. Two fix runs over one
// directory would share it, which is the same collision two reviews over one
// directory already have.
type fixRun struct {
	repo *git.Repo
	tree *git.Worktree
	// inScope reports whether a changed path is one the review resolved. Taken
	// from the collected target, since what bounds a batch differs by how the
	// scope was named: everything under a path, or exactly the files a range
	// changed.
	inScope func(string) bool
	// continued reports a worktree an earlier iteration left behind.
	continued bool
	gate      *pb.Verification
	closed    []string
	commit    string
}

// prepareFix puts the batch somewhere the operator's own working tree is not.
//
// The worktree is created off HEAD, which is why the tree has to be clean: a
// dirty tree means the bytes the operator is looking at are not the bytes this
// would review, and a fix run that silently reviewed HEAD instead would be
// answering a question nobody asked.
func (e *Engine) prepareFix(ctx context.Context) (*fixRun, error) {
	repo, err := git.Open(ctx, e.Dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFixNeedsRepository, err)
	}
	dir := filepath.Join(e.Dir, contracts.FixDir)

	// An existing worktree is the last iteration's, and its edits are what this
	// one reviews. Its branch is whatever that iteration left checked out, not
	// a name derived from this run's own identity.
	if _, err := os.Stat(dir); err == nil {
		tree, err := git.OpenWorktree(ctx, repo, dir)
		if err != nil {
			return nil, err
		}
		return &fixRun{repo: repo, tree: tree, continued: true}, nil
	}

	// .grimes is the review's own, not the operator's work.
	if err := repo.Clean(ctx, contracts.GrimesDir); err != nil {
		return nil, err
	}
	head, err := repo.Head(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	tree, err := repo.AddWorktree(ctx, dir, fixBranch(e.RunID), head)
	if err != nil {
		return nil, err
	}
	return &fixRun{repo: repo, tree: tree}, nil
}

func fixBranch(runID string) string { return "grimes/fix-" + contracts.RunSlug(runID) }

// pinned returns the scope a continued fix run is to collect, which is the one
// its first iteration resolved.
//
// A symbolic range names different commits once the run has committed a batch.
// Re-resolving "HEAD^..HEAD" after a fix commit would make the next iteration a
// review of that commit, dropping every other file the range originally
// selected, and coverage would be measured against what the fixer touched
// rather than against what was asked about. The recorded scope is itself a
// range spelling, so collecting it again resolves to the same commits.
func pinned(f *fixRun, ledger *pb.Ledger, spec TargetSpec) TargetSpec {
	if f == nil || !f.continued {
		return spec
	}
	was := ledger.GetTarget().GetScope()
	if was == "" || !strings.Contains(was, "..") {
		return spec
	}
	spec.Scope = was
	return spec
}

// record returns what the run says it did with its edits.
func (f *fixRun) record() *pb.FixBatch {
	b := &pb.FixBatch{Worktree: f.tree.Dir, Branch: f.tree.Branch}
	if f.commit != "" {
		if sum, err := hex.DecodeString(f.commit); err == nil {
			b.CommitSha1 = sum
			b.ClosedFindingIds = f.closed
		}
	}
	return b
}

// settle turns the batch into ledger state: what the fixer edited, what the
// gate then passed over, and a commit if one was authorized and earned.
//
// The order is the skill's. An edit makes a finding fixed; only a gate makes it
// verified; only a verified batch is committed.
func (e *Engine) settle(ctx context.Context, f *fixRun, ledger *pb.Ledger, iteration uint32) error {
	changed, err := f.tree.Changed(ctx)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		return nil
	}
	if outside := outOfScope(changed, f.inScope); len(outside) > 0 {
		return fmt.Errorf("%w: %s", ErrBatchOutOfScope, strings.Join(outside, ", "))
	}

	edited := editedFindings(ledger, changed)
	// A batch credited with nothing has closed nothing: the files it touched
	// carry no finding this run may count, or the only claims there were broken
	// by a context that did not form them. The edits stay in the worktree to be
	// read; a commit is the strongest form of crediting, and there is nothing
	// here to credit.
	if len(edited) == 0 {
		return nil
	}
	for _, id := range edited {
		if _, err := contracts.Transition(ledger, id, pb.FindingStatus_FINDING_STATUS_FIXED,
			contracts.TransitionOpts{Iteration: iteration, Actor: actorName}); err != nil {
			return err
		}
	}
	if f.gate.GetStatus() != pb.VerificationStatus_VERIFICATION_STATUS_PASSED {
		return nil
	}
	for _, id := range edited {
		if _, err := contracts.Transition(ledger, id, pb.FindingStatus_FINDING_STATUS_VERIFIED,
			contracts.TransitionOpts{
				Iteration: iteration, Actor: actorName,
				VerifiedBySum: f.gate.GetOutputSha256(),
			}); err != nil {
			return err
		}
	}
	f.closed = edited

	if !e.Commit {
		return nil
	}
	sha, err := f.tree.Commit(ctx, commitMessage(edited))
	if err != nil {
		return err
	}
	f.commit = sha
	return nil
}

// editedFindings names the open findings anchored in a file this batch touched
// that a refutation pass did not break.
//
// Derived rather than reported: a fixer naming what it fixed would be grading
// its own work, and what is actually known is which files changed. That is what
// the skill calls edited, and it is the gate that turns it into verified.
//
// A broken claim is left out. The gate is about the repair, not about the
// defect: an edit that compiles is not evidence there was something to repair,
// and a claim a second context refused to uphold has not earned one. The edit
// itself stands in the worktree for the operator to read, credited to nothing.
func editedFindings(ledger *pb.Ledger, changed []string) []string {
	touched := make(map[string]bool, len(changed))
	for _, p := range changed {
		touched[p] = true
	}
	var ids []string
	for id, f := range ledger.GetFindings() {
		if !Open(f.GetStatus()) {
			continue
		}
		if provenanceOf(f) == pb.FindingProvenance_FINDING_PROVENANCE_REFUTED {
			continue
		}
		if touched[f.GetLocation().GetAnchor().GetRepoLine().GetPath().GetValue()] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// adopt takes the bound on a batch from what collection resolved.
//
// A range target's scope is not a path, so a batch is bounded by the inventory
// itself: the files the range changed are the whole of what was reviewed, and
// an edit to any other file is an edit to something no role examined.
func (f *fixRun) adopt(c *Collected) {
	if !c.Range {
		scope := c.Target.GetScope()
		f.inScope = func(p string) bool { return underPath(p, scope) }
		return
	}
	units := make(map[string]bool, len(c.Inventory.GetUnits()))
	for _, u := range c.Inventory.GetUnits() {
		units[u.GetId()] = true
	}
	f.inScope = func(p string) bool { return units[p] }
}

// outOfScope names the changed paths the review never resolved.
func outOfScope(changed []string, in func(string) bool) []string {
	var outside []string
	for _, p := range changed {
		if in(p) {
			continue
		}
		outside = append(outside, p)
	}
	sort.Strings(outside)
	return outside
}

func underPath(path, scope string) bool {
	scope = strings.TrimSuffix(filepath.Clean(scope), "/")
	if scope == "." || scope == "" {
		return true
	}
	return path == scope || strings.HasPrefix(path, scope+"/")
}

func commitMessage(closed []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fix %d finding", len(closed))
	if len(closed) != 1 {
		b.WriteString("s")
	}
	b.WriteString(" the gate passed over\n\n")
	for _, id := range closed {
		fmt.Fprintf(&b, "Closes %s\n", id)
	}
	return b.String()
}

// rebaseline carries the ledger onto the bytes the batch produced.
//
// This is the recheck a report-mode run does, answered differently because the
// answer is different: the change came from the role this run spawned, into the
// one directory confinement grants it. Recording the fingerprint it moved from
// is what separates that from a target that moved on its own, and a later run
// that cannot show the same record is still refused.
func (e *Engine) rebaseline(ctx context.Context, spec TargetSpec, collected *Collected, ledger *pb.Ledger) error {
	// A range is re-collected as the commits it resolved to, not as the
	// spelling. By now the batch may have been committed, and the spelling
	// would name that commit: the ledger would come away describing a review of
	// the fix rather than of what was asked about.
	if collected.Range {
		spec.Scope = collected.Target.GetScope()
	}
	again, err := e.Collector.Collect(ctx, spec)
	if err != nil {
		return fmt.Errorf("rechecking the target: %w", err)
	}
	was := collected.Target.GetFingerprintSha256()
	if string(again.Target.GetFingerprintSha256()) == string(was) {
		return nil
	}
	ledger.AncestorFingerprintsSha256 = append(ledger.GetAncestorFingerprintsSha256(), was)
	ledger.Target = again.Target
	return nil
}

// descends reports whether the ledger says it reached this target from that
// fingerprint.
func descends(ledger *pb.Ledger, from []byte) bool {
	for _, a := range ledger.GetAncestorFingerprintsSha256() {
		if string(a) == string(from) {
			return true
		}
	}
	return false
}
