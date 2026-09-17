// Package engine owns a review run: collection, provider execution,
// validation, verdict derivation, and lifecycle transitions.
//
// Provider output is untrusted. Nothing a provider returns reaches a final
// field; the engine recomputes every one of them.
package engine

import (
	"context"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
)

// TargetSpec is what the caller asked to have reviewed, before resolution.
type TargetSpec struct {
	Root       string
	Scope      string
	Kind       pb.TargetKind
	Snapshot   string
	Categories []pb.Category
	// KeepBodies asks collection to hold what it read, for a run that is about
	// to change it. Evidence is checked against the bytes that were reviewed,
	// and after a fix the file on disk is not those.
	KeepBodies bool
}

// Role distinguishes the primary review from the roles that receive far less
// input: adjudication, which sees only the target, and refutation, which sees
// the claims under attack and nothing that argues for them.
type Role int

const (
	RolePrimary Role = iota
	RoleAdjudicator
	RoleRefuter
)

// String names a role in a path and in an error.
func (r Role) String() string {
	switch r {
	case RoleAdjudicator:
		return "adjudicator"
	case RoleRefuter:
		return "refuter"
	default:
		return "primary"
	}
}

// Request is everything a provider is given. An adjudicator request carries
// Claimed and no findings, evidence, or ledger data.
type Request struct {
	Role   Role
	RunID  string
	Target *pb.Target
	// ContentPath is where the provider reads the target. Both roles get it: an
	// adjudicator that cannot see the artifact cannot form an opinion of its
	// own, and zero knowledge is about the first report, not the target.
	ContentPath string
	// InventoryPath is the set of units the primary review must account for.
	// A provider asked to report coverage has to be able to read the
	// denominator it is being measured against. The adjudicator reports no
	// coverage and does not receive it.
	InventoryPath string
	// ClaimsPath is where a refuter reads the claims it is to attack. No other
	// role receives it.
	ClaimsPath string
	// WriteRoot is the worktree a fixing role may change. Empty for every role
	// that is only reading, which is all of them outside fix mode.
	WriteRoot string
	// Root is the directory this role resolves unit ids against. Empty means
	// the target's own, which is every role outside fix mode and the fixing
	// role inside it; the roles after that one are given the reviewed copy.
	Root       string
	Mode       pb.Mode
	Iteration  uint32
	Categories []pb.Category
	Research   string
	Claimed    *pb.Verdict
}

// ProviderOutput is raw transport output. Raw is untrusted bytes; the engine
// decodes and strips it.
type ProviderOutput struct {
	Raw []byte
	// Diagnostics is what the provider wrote to stderr, bounded. Carried even
	// when the provider exited cleanly: an agent CLI logs its working here and
	// prints its answer on stdout, so a clean exit with an unusable answer is
	// the case where this is the only account of what happened.
	Diagnostics string
	// Sealed is the report the role left behind for this pass, if it sealed
	// one. Untrusted in the same way Raw is, and preferred over it: this binary
	// wrote these bytes, where Raw is whatever a model chose to say last.
	Sealed []byte
}

// Collected is what resolving a target produced.
type Collected struct {
	Target     *pb.Target
	Inventory  *pb.TargetInventory
	Categories []pb.Category
	// ContentPath is where the reviewed bytes are. Always absolute: collection
	// resolves a relative scope against its own working directory and the
	// provider runs in another, so a relative path would name two files.
	//
	// A directory when the target is a tree, a file when it is one file. A code
	// target may be either.
	ContentPath string
	// ContentBytes is set only when the target has no path of its own, which
	// today means an argument read from stdin. The engine persists it and fills
	// ContentPath in with where it put it.
	ContentBytes []byte
	// UnitDigests is what each unit hashed to at collection time, keyed by unit
	// id. Evidence is checked after the provider has run, and the provider can
	// write to the artifact it was asked to review: without this, it could
	// inject the line it then quotes and have the citation admitted against a
	// fingerprint taken before the edit.
	UnitDigests map[string][]byte
	// UnitBodies is what each unit held at collection time, kept only when the
	// run is going to change them. Evidence is checked against the bytes that
	// were reviewed, and in fix mode the file on disk is no longer those.
	UnitBodies map[string][]byte
	// ReviewedRoot is a copy of the target as it stood before a fixing role
	// touched it, set only in fix mode. The roles after that one read it
	// instead of the worktree, and resolve their unit ids against it.
	ReviewedRoot string
	// Range reports that the scope named a revision range. The inventory is then
	// the whole of the scope rather than everything under one path, which is
	// what a fix batch has to stay inside of.
	Range bool
}

// Collector resolves a caller's target spec into a fingerprinted target, the
// inventory of units a review is accountable for, the categories to route, and
// where the reviewed bytes can be read.
type Collector interface {
	Collect(ctx context.Context, spec TargetSpec) (*Collected, error)
}

// Provider runs one review and returns its untrusted output.
type Provider interface {
	Review(ctx context.Context, req Request) (*ProviderOutput, error)
}

// EvidenceBroker decides whether a reported candidate's evidence entitles it to
// the tier and severity it claims.
//
// The broker is given the target so it can check the evidence against it. A
// claim about an artifact nobody resolved is not evidence about this review,
// and the shape of the record cannot reveal that on its own.
type EvidenceBroker interface {
	Admit(ctx context.Context, c *pb.CandidateFinding, against *Collected) (*pb.CandidateFinding, error)
}

// Handoff is where a role reads the target and what its unit ids are relative
// to. The two differ only in fix mode, where a role after the fixing one reads
// a copy taken before the batch: its content path is inside that copy, and so
// is the root it resolves ids against.
type Handoff struct {
	ContentPath string
	// Root is empty when it is the target's own.
	Root string
}

// Adjudicator obtains a second verdict reached without sight of the first.
// A nil review with a nil error means adjudication was unavailable.
type Adjudicator interface {
	Adjudicate(ctx context.Context, target *pb.Target, at Handoff, claimed *pb.Verdict) (*pb.IndependentReview, error)
}

// Refuter puts claims to a context that did not form them and reports what
// that context managed to do to each, keyed by the ref the engine issued.
//
// An error means the pass did not happen. That is not the same as every claim
// surviving one, and the verdict reads the difference.
type Refuter interface {
	Refute(ctx context.Context, target *pb.Target, at Handoff, claimsPath string) (map[string]*pb.RefutationAttempt, error)
}

// GateRunner runs the verification command once over a batch.
type GateRunner interface {
	Run(ctx context.Context, cwd string) (*pb.Verification, error)
}

// Ledger persists findings across iterations. Save returns the digest of what
// it wrote.
type Ledger interface {
	Load(ctx context.Context) (*pb.Ledger, error)
	Save(ctx context.Context, l *pb.Ledger) ([]byte, error)
}

// ResultStore persists the derived result so the loop can verify it.
type ResultStore interface {
	Load(ctx context.Context) (*pb.GrimesResult, []byte, error)
	Save(ctx context.Context, r *pb.GrimesResult) ([]byte, error)
}

// InventoryStore persists what collection resolved, so a later iteration is
// accountable to the same units the first one was.
type InventoryStore interface {
	Save(ctx context.Context, i *pb.TargetInventory) error
	// Path is where a provider can read what it is accountable for.
	Path() string
}

// ContentStore persists collected bytes that have no path of their own, so a
// provider can be pointed at them.
type ContentStore interface {
	Save(ctx context.Context, content []byte) (path string, err error)
	// Stage writes one role its own copy, at a path of its own.
	Stage(ctx context.Context, role Role, content []byte) (path string, err error)
}

// ClaimStore persists the claims a refutation pass was asked to attack, so the
// refuter reads them from disk rather than from a prompt the engine composed.
type ClaimStore interface {
	Save(ctx context.Context, task *pb.RefutationTask) (path string, err error)
	Discard(ctx context.Context, path string) error
}

// StateStore persists loop state between iterations.
type StateStore interface {
	Load(ctx context.Context) (*pb.LoopState, error)
	Save(ctx context.Context, s *pb.LoopState) error
	Clear(ctx context.Context) error
}
