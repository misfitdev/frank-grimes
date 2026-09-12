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
	Role       Role
	Target     *pb.Target
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
}

// Collector resolves a caller's target spec into a fingerprinted target and the
// categories to route.
type Collector interface {
	Collect(ctx context.Context, spec TargetSpec) (*pb.Target, []pb.Category, error)
}

// Provider runs one review and returns its untrusted output.
type Provider interface {
	Review(ctx context.Context, req Request) (*ProviderOutput, error)
}

// EvidenceBroker decides whether a proposed finding's evidence entitles it to
// the tier and severity it claims.
type EvidenceBroker interface {
	Admit(ctx context.Context, f *pb.Finding) (*pb.Finding, error)
}

// Adjudicator obtains a second verdict reached without sight of the first.
// A nil review with a nil error means adjudication was unavailable.
type Adjudicator interface {
	Adjudicate(ctx context.Context, target *pb.Target, claimed *pb.Verdict) (*pb.IndependentReview, error)
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

// StateStore persists loop state between iterations.
type StateStore interface {
	Load(ctx context.Context) (*pb.LoopState, error)
	Save(ctx context.Context, s *pb.LoopState) error
	Clear(ctx context.Context) error
}
