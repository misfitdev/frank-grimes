package engine

import (
	"testing"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func openP(sev pb.Severity, tier pb.EvidenceTier) Candidate {
	return Candidate{
		Severity:       sev,
		Status:         pb.FindingStatus_FINDING_STATUS_OPEN,
		Tier:           tier,
		ProbeAttempted: true,
	}
}

// clean is the only input shape that can reach a pass, so each test names the
// single fact it changes.
func clean() DeriveInput {
	return DeriveInput{
		AdjudicationAvailable:   true,
		IndependentDecision:     pb.Decision_DECISION_PASS,
		AllCategoriesStopped:    true,
		CriticalInvariantProbed: true,
	}
}

func TestDeriveCleanRunPasses(t *testing.T) {
	d := Derive(clean())
	if d.Verdict.GetDecision() != pb.Decision_DECISION_PASS {
		t.Errorf("decision = %v, want pass", d.Verdict.GetDecision())
	}
	if d.Color != pb.LegacyColor_LEGACY_COLOR_GREEN {
		t.Errorf("color = %v, want green", d.Color)
	}
	if len(d.UnmetGates) != 0 {
		t.Errorf("unmet gates = %v, want none", d.UnmetGates)
	}
}

func TestDeriveOpenP0Blocks(t *testing.T) {
	in := clean()
	in.Candidates = []Candidate{openP(pb.Severity_SEVERITY_P0, pb.EvidenceTier_EVIDENCE_TIER_E1)}
	d := Derive(in)
	if d.Verdict.GetDecision() != pb.Decision_DECISION_BLOCK {
		t.Errorf("decision = %v, want block", d.Verdict.GetDecision())
	}
	if d.Color != pb.LegacyColor_LEGACY_COLOR_RED {
		t.Errorf("color = %v, want red", d.Color)
	}
	if d.Counts.GetOpenP0() != 1 {
		t.Errorf("open_p0 = %d, want 1", d.Counts.GetOpenP0())
	}
}

func TestDeriveOpenP1Conditional(t *testing.T) {
	in := clean()
	in.Candidates = []Candidate{openP(pb.Severity_SEVERITY_P1, pb.EvidenceTier_EVIDENCE_TIER_E2)}
	d := Derive(in)
	if d.Verdict.GetDecision() != pb.Decision_DECISION_CONDITIONAL {
		t.Errorf("decision = %v, want conditional", d.Verdict.GetDecision())
	}
	if d.Color != pb.LegacyColor_LEGACY_COLOR_YELLOW {
		t.Errorf("color = %v, want yellow", d.Color)
	}
}

func TestDeriveAdjudicationUnavailableCapsAtConditional(t *testing.T) {
	in := clean()
	in.AdjudicationAvailable = false
	d := Derive(in)
	if d.Verdict.GetDecision() != pb.Decision_DECISION_CONDITIONAL {
		t.Errorf("decision = %v, want conditional", d.Verdict.GetDecision())
	}
	if d.Verdict.GetReviewConfidence() != pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW {
		t.Errorf("confidence = %v, want low", d.Verdict.GetReviewConfidence())
	}
	if d.Color == pb.LegacyColor_LEGACY_COLOR_GREEN {
		t.Error("green is unreachable without adjudication")
	}
}

func TestDeriveIndependentDowngrades(t *testing.T) {
	for _, c := range []struct {
		independent pb.Decision
		want        pb.Decision
	}{
		{pb.Decision_DECISION_PASS, pb.Decision_DECISION_PASS},
		{pb.Decision_DECISION_CONDITIONAL, pb.Decision_DECISION_CONDITIONAL},
		{pb.Decision_DECISION_BLOCK, pb.Decision_DECISION_BLOCK},
	} {
		in := clean()
		in.IndependentDecision = c.independent
		if got := Derive(in).Verdict.GetDecision(); got != c.want {
			t.Errorf("independent %v: decision = %v, want %v", c.independent, got, c.want)
		}
	}
}

func TestDeriveAcceptedP0CapsAtConditional(t *testing.T) {
	in := clean()
	in.Candidates = []Candidate{{
		Severity:       pb.Severity_SEVERITY_P0,
		Status:         pb.FindingStatus_FINDING_STATUS_ACCEPTED,
		Tier:           pb.EvidenceTier_EVIDENCE_TIER_E1,
		ProbeAttempted: true,
	}}
	d := Derive(in)
	if d.Verdict.GetDecision() != pb.Decision_DECISION_CONDITIONAL {
		t.Errorf("decision = %v, want conditional", d.Verdict.GetDecision())
	}
	if d.Counts.GetOpenP0() != 0 {
		t.Errorf("open_p0 = %d, want 0 for an accepted finding", d.Counts.GetOpenP0())
	}
}

func TestDeriveUnweightedFindingsExcluded(t *testing.T) {
	for _, tag := range []string{TagAssumptionDependent, TagUnverified} {
		in := clean()
		c := openP(pb.Severity_SEVERITY_P0, pb.EvidenceTier_EVIDENCE_TIER_E1)
		c.Tags = []string{tag}
		in.Candidates = []Candidate{c}
		d := Derive(in)
		if d.Verdict.GetDecision() == pb.Decision_DECISION_BLOCK {
			t.Errorf("tag %q: a %s finding drove the decision", tag, tag)
		}
		if d.Counts.GetOpenP0() != 0 {
			t.Errorf("tag %q: open_p0 = %d, want 0", tag, d.Counts.GetOpenP0())
		}
	}
}

func TestDeriveRegressedCountsAsOpen(t *testing.T) {
	in := clean()
	in.Candidates = []Candidate{{
		Severity:       pb.Severity_SEVERITY_P0,
		Status:         pb.FindingStatus_FINDING_STATUS_REGRESSED,
		Tier:           pb.EvidenceTier_EVIDENCE_TIER_E1,
		ProbeAttempted: true,
	}}
	if got := Derive(in).Counts.GetOpenP0(); got != 1 {
		t.Errorf("open_p0 = %d, want 1 for a regressed finding", got)
	}
}

func TestDeriveResidualRiskMapsHighestOpen(t *testing.T) {
	for _, c := range []struct {
		sev  pb.Severity
		want pb.ResidualRisk
	}{
		{pb.Severity_SEVERITY_P0, pb.ResidualRisk_RESIDUAL_RISK_CRITICAL},
		{pb.Severity_SEVERITY_P1, pb.ResidualRisk_RESIDUAL_RISK_HIGH},
		{pb.Severity_SEVERITY_P2, pb.ResidualRisk_RESIDUAL_RISK_MODERATE},
		{pb.Severity_SEVERITY_P3, pb.ResidualRisk_RESIDUAL_RISK_LOW},
	} {
		in := clean()
		in.Candidates = []Candidate{openP(c.sev, pb.EvidenceTier_EVIDENCE_TIER_E1)}
		if got := Derive(in).Verdict.GetResidualRisk(); got != c.want {
			t.Errorf("severity %v: residual risk = %v, want %v", c.sev, got, c.want)
		}
	}
}

func TestDeriveResidualRiskTakesWorst(t *testing.T) {
	in := clean()
	in.Candidates = []Candidate{
		openP(pb.Severity_SEVERITY_P3, pb.EvidenceTier_EVIDENCE_TIER_E1),
		openP(pb.Severity_SEVERITY_P0, pb.EvidenceTier_EVIDENCE_TIER_E1),
		openP(pb.Severity_SEVERITY_P2, pb.EvidenceTier_EVIDENCE_TIER_E1),
	}
	if got := Derive(in).Verdict.GetResidualRisk(); got != pb.ResidualRisk_RESIDUAL_RISK_CRITICAL {
		t.Errorf("residual risk = %v, want critical", got)
	}
}

func TestDeriveResidualRiskUnknownWhenRankingBlocked(t *testing.T) {
	in := clean()
	in.RankingBlocked = true
	if got := Derive(in).Verdict.GetResidualRisk(); got != pb.ResidualRisk_RESIDUAL_RISK_UNKNOWN {
		t.Errorf("residual risk = %v, want unknown", got)
	}
}

func TestDeriveConfidence(t *testing.T) {
	e3P1 := openP(pb.Severity_SEVERITY_P1, pb.EvidenceTier_EVIDENCE_TIER_E3)
	unprobed := openP(pb.Severity_SEVERITY_P1, pb.EvidenceTier_EVIDENCE_TIER_E2)
	unprobed.ProbeAttempted = false
	conflicted := openP(pb.Severity_SEVERITY_P1, pb.EvidenceTier_EVIDENCE_TIER_E1)
	conflicted.EvidenceConflict = true

	for _, c := range []struct {
		name string
		in   func() DeriveInput
		want pb.ReviewConfidence
	}{
		{"all E1/E2 probed", clean, pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH},
		{"E3 drives", func() DeriveInput {
			in := clean()
			in.Candidates = []Candidate{e3P1}
			return in
		}, pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM},
		{"probe not attempted", func() DeriveInput {
			in := clean()
			in.Candidates = []Candidate{unprobed}
			return in
		}, pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM},
		{"evidence conflict", func() DeriveInput {
			in := clean()
			in.Candidates = []Candidate{conflicted}
			return in
		}, pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW},
		{"critical falsifier unavailable", func() DeriveInput {
			in := clean()
			in.CriticalFalsifierUnavailable = true
			return in
		}, pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW},
	} {
		if got := Derive(c.in()).Verdict.GetReviewConfidence(); got != c.want {
			t.Errorf("%s: confidence = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDeriveCompleteness(t *testing.T) {
	for _, c := range []struct {
		name string
		in   func() DeriveInput
		want pb.ReviewCompleteness
	}{
		{"all stopped, no unknown", clean, pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT},
		{"a category short", func() DeriveInput {
			in := clean()
			in.AllCategoriesStopped = false
			return in
		}, pb.ReviewCompleteness_REVIEW_COMPLETENESS_LIMITED},
		{"critical unknown remains", func() DeriveInput {
			in := clean()
			in.CriticalUnknownRemains = true
			return in
		}, pb.ReviewCompleteness_REVIEW_COMPLETENESS_LIMITED},
		{"no critical invariant probed", func() DeriveInput {
			in := clean()
			in.CriticalInvariantProbed = false
			return in
		}, pb.ReviewCompleteness_REVIEW_COMPLETENESS_INCONCLUSIVE},
	} {
		if got := Derive(c.in()).Verdict.GetReviewCompleteness(); got != c.want {
			t.Errorf("%s: completeness = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDeriveOscillationBlocksGreen(t *testing.T) {
	in := clean()
	in.Oscillation = true
	d := Derive(in)
	if d.Color != pb.LegacyColor_LEGACY_COLOR_YELLOW {
		t.Errorf("color = %v, want yellow", d.Color)
	}
	if !hasGate(d.UnmetGates, GateOscillation) {
		t.Errorf("unmet gates = %v, want %q", d.UnmetGates, GateOscillation)
	}
}

func TestDeriveRedIffBlock(t *testing.T) {
	for _, in := range deriveCorpus() {
		d := Derive(in)
		isRed := d.Color == pb.LegacyColor_LEGACY_COLOR_RED
		isBlock := d.Verdict.GetDecision() == pb.Decision_DECISION_BLOCK
		if isRed != isBlock {
			t.Errorf("red = %v but block = %v for %+v", isRed, isBlock, in)
		}
	}
}

func TestDeriveUnmetGatesUniqueAndStable(t *testing.T) {
	for _, in := range deriveCorpus() {
		first := Derive(in).UnmetGates
		second := Derive(in).UnmetGates
		if len(first) != len(second) {
			t.Fatalf("unstable gate count: %v vs %v", first, second)
		}
		seen := map[string]bool{}
		for i, g := range first {
			if g == "" {
				t.Errorf("empty gate name in %v", first)
			}
			if seen[g] {
				t.Errorf("duplicate gate %q in %v", g, first)
			}
			seen[g] = true
			if second[i] != g {
				t.Errorf("unstable gate order: %v vs %v", first, second)
			}
		}
	}
}

// Every tuple Derive can produce must survive the contract's CEL rules. This is
// what keeps the Go derivation and the proto constraints from drifting apart.
func TestDeriveAlwaysEncodesCanonically(t *testing.T) {
	for _, in := range deriveCorpus() {
		d := Derive(in)
		result := resultFrom(d, in)
		if _, err := contracts.EncodeCanonical(result); err != nil {
			t.Errorf("derived result rejected by the contract: %v\ninput: %+v\nverdict: %v color: %v counts: %v",
				err, in, d.Verdict, d.Color, d.Counts)
		}
	}
}

func hasGate(gates []string, want string) bool {
	for _, g := range gates {
		if g == want {
			return true
		}
	}
	return false
}

// deriveCorpus enumerates the input space across every flag and severity.
func deriveCorpus() []DeriveInput {
	var out []DeriveInput
	severities := []pb.Severity{
		pb.Severity_SEVERITY_P0, pb.Severity_SEVERITY_P1,
		pb.Severity_SEVERITY_P2, pb.Severity_SEVERITY_P3,
	}
	statuses := []pb.FindingStatus{
		pb.FindingStatus_FINDING_STATUS_OPEN,
		pb.FindingStatus_FINDING_STATUS_ACCEPTED,
		pb.FindingStatus_FINDING_STATUS_VERIFIED,
		pb.FindingStatus_FINDING_STATUS_REGRESSED,
	}
	tiers := []pb.EvidenceTier{
		pb.EvidenceTier_EVIDENCE_TIER_E1,
		pb.EvidenceTier_EVIDENCE_TIER_E2,
		pb.EvidenceTier_EVIDENCE_TIER_E3,
	}
	for _, sev := range severities {
		for _, st := range statuses {
			for _, tier := range tiers {
				for _, tags := range [][]string{nil, {TagAssumptionDependent}, {TagUnverified}} {
					for _, adj := range []bool{true, false} {
						for _, indep := range []pb.Decision{
							pb.Decision_DECISION_PASS,
							pb.Decision_DECISION_CONDITIONAL,
							pb.Decision_DECISION_BLOCK,
						} {
							for _, osc := range []bool{true, false} {
								for _, probe := range []bool{true, false} {
									in := clean()
									in.AdjudicationAvailable = adj
									in.IndependentDecision = indep
									in.Oscillation = osc
									in.Candidates = []Candidate{{
										Severity: sev, Status: st, Tier: tier,
										Tags: tags, ProbeAttempted: probe,
									}}
									out = append(out, in)
								}
							}
						}
					}
				}
			}
		}
	}
	for _, blocked := range []bool{true, false} {
		for _, stopped := range []bool{true, false} {
			for _, probed := range []bool{true, false} {
				for _, unknown := range []bool{true, false} {
					in := clean()
					in.RankingBlocked = blocked
					in.AllCategoriesStopped = stopped
					in.CriticalInvariantProbed = probed
					in.CriticalUnknownRemains = unknown
					out = append(out, in)
				}
			}
		}
	}
	return out
}

// resultFrom builds the smallest valid GrimesResult around a derived verdict.
func resultFrom(d Derived, in DeriveInput) *pb.GrimesResult {
	digest := make([]byte, 32)
	ts := timestamppb.New(SystemClock())
	target := &pb.Target{Root: "/repo", Scope: "src", FingerprintSha256: digest}

	r := &pb.GrimesResult{
		SchemaMajor:     2,
		RunId:           "run-derive",
		ProducerRole:    pb.ProducerRole_PRODUCER_ROLE_ORCHESTRATOR,
		Target:          target,
		Mode:            pb.Mode_MODE_REPORT,
		Iteration:       1,
		MaxIterations:   5,
		CompletionState: pb.CompletionState_COMPLETION_STATE_REVIEW_COMPLETE,
		Verdict:         d.Verdict,
		LegacyColor:     d.Color,
		MarginalYield:   &pb.MarginalYield{},
		Counts:          d.Counts,
		Verification:    &pb.Verification{Status: pb.VerificationStatus_VERIFICATION_STATUS_NOT_APPLICABLE},
		Ledger: &pb.LedgerRef{
			Path:                contracts.LedgerPath,
			DigestSha256:        digest,
			OscillationDetected: in.Oscillation,
		},
		UnmetGates: d.UnmetGates,
		Summary:    "derived",
	}
	if in.AdjudicationAvailable {
		r.IndependentReview = &pb.IndependentReview{
			RunId:                   "run-derive-adj",
			ReviewerId:              "adjudicator",
			ZeroKnowledge:           true,
			TargetFingerprintSha256: digest,
			Verdict: &pb.Verdict{
				Decision:           in.IndependentDecision,
				ResidualRisk:       d.Verdict.GetResidualRisk(),
				ReviewConfidence:   d.Verdict.GetReviewConfidence(),
				ReviewCompleteness: d.Verdict.GetReviewCompleteness(),
			},
			CompletedAt: ts,
		}
	}
	return r
}
