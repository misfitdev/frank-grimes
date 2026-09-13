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
}

// Role distinguishes the primary review from adjudication, which receives far
// less input.
type Role int

const (
	RolePrimary Role = iota
	RoleAdjudicator
)

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
	Mode        pb.Mode
	Iteration   uint32
	Categories  []pb.Category
	Research    string
	Claimed     *pb.Verdict
}

// ProviderOutput is raw transport output. Raw is untrusted bytes; the engine
// decodes and strips it.
type ProviderOutput struct {
	Raw []byte
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
type EvidenceBroker interface {
	Admit(ctx context.Context, c *pb.CandidateFinding) (*pb.CandidateFinding, error)
}

// Adjudicator obtains a second verdict reached without sight of the first.
// A nil review with a nil error means adjudication was unavailable.
type Adjudicator interface {
	Adjudicate(ctx context.Context, target *pb.Target, contentPath string, claimed *pb.Verdict) (*pb.IndependentReview, error)
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
	Load(ctx context.Context) (*pb.GrimesResult, error)
	Save(ctx context.Context, r *pb.GrimesResult) ([]byte, error)
}

// InventoryStore persists what collection resolved, so a later iteration is
// accountable to the same units the first one was.
type InventoryStore interface {
	Save(ctx context.Context, i *pb.TargetInventory) error
}

// ContentStore persists collected bytes that have no path of their own, so a
// provider can be pointed at them.
type ContentStore interface {
	Save(ctx context.Context, content []byte) (path string, err error)
}

// StateStore persists loop state between iterations.
type StateStore interface {
	Load(ctx context.Context) (*pb.LoopState, error)
	Save(ctx context.Context, s *pb.LoopState) error
	Clear(ctx context.Context) error
}
