package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// cmdAdjudicate emits the report an independent review hands back.
//
// The engine exports the request when it invokes an adjudicator, so the
// identity of the run and the target are read from there rather than restated;
// what the caller supplies is the tuple it reached on its own.
func cmdAdjudicate(args []string) error {
	fs := flag.NewFlagSet("adjudicate", flag.ExitOnError)
	decision := fs.String("decision", "", "block, conditional, pass, or informational")
	risk := fs.String("residual-risk", "", "critical, high, moderate, low, or unknown")
	confidence := fs.String("review-confidence", "", "high, medium, or low")
	completeness := fs.String("review-completeness", "", "sufficient, limited, or inconclusive")
	reviewerID := fs.String("reviewer-id", "", "who reached this verdict; defaults to $GRIMES_REVIEWER_ID, then \"adjudicator\"")
	runID := fs.String("run-id", "", "run identity; defaults to $GRIMES_RUN_ID")
	targetFP := fs.String("target-fingerprint", "", "hex target fingerprint; defaults to $GRIMES_TARGET_FINGERPRINT")
	raw := fs.Bool("raw", false, "write canonical bytes instead of the envelope")
	if err := fs.Parse(args); err != nil {
		return err
	}

	verdict := &pb.Verdict{}
	var err error
	if verdict.Decision, err = parseDecision(*decision); err != nil {
		return err
	}
	if verdict.ResidualRisk, err = parseResidualRisk(*risk); err != nil {
		return err
	}
	if verdict.ReviewConfidence, err = parseReviewConfidence(*confidence); err != nil {
		return err
	}
	if verdict.ReviewCompleteness, err = parseReviewCompleteness(*completeness); err != nil {
		return err
	}

	report := &pb.AdjudicationReport{
		SchemaMajor: contracts.SchemaMajor,
		RunId:       firstNonEmpty(*runID, os.Getenv("GRIMES_RUN_ID"), "adjudication"),
		ReviewerId:  firstNonEmpty(*reviewerID, os.Getenv("GRIMES_REVIEWER_ID"), "adjudicator"),
		Verdict:     verdict,
		CompletedAt: timestamppb.New(contracts.Now().UTC()),
	}

	fpHex := firstNonEmpty(*targetFP, os.Getenv("GRIMES_TARGET_FINGERPRINT"))
	decoded, err := hex.DecodeString(fpHex)
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("--target-fingerprint must be 64 hex characters; the engine exports it as $GRIMES_TARGET_FINGERPRINT")
	}
	report.TargetFingerprintSha256 = decoded

	encoded, err := contracts.EncodeCanonical(report)
	if err != nil {
		return err
	}
	if *raw {
		_, err = os.Stdout.Write(encoded)
		return err
	}
	return emitSealed(store.WorkDir, encoded)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseDecision(s string) (pb.Decision, error) {
	switch strings.ToLower(s) {
	case "block":
		return pb.Decision_DECISION_BLOCK, nil
	case "conditional":
		return pb.Decision_DECISION_CONDITIONAL, nil
	case "pass":
		return pb.Decision_DECISION_PASS, nil
	case "informational":
		return pb.Decision_DECISION_INFORMATIONAL, nil
	default:
		return 0, fmt.Errorf("unknown decision %q; use block, conditional, pass, or informational", s)
	}
}

func parseResidualRisk(s string) (pb.ResidualRisk, error) {
	switch strings.ToLower(s) {
	case "critical":
		return pb.ResidualRisk_RESIDUAL_RISK_CRITICAL, nil
	case "high":
		return pb.ResidualRisk_RESIDUAL_RISK_HIGH, nil
	case "moderate":
		return pb.ResidualRisk_RESIDUAL_RISK_MODERATE, nil
	case "low":
		return pb.ResidualRisk_RESIDUAL_RISK_LOW, nil
	case "unknown":
		return pb.ResidualRisk_RESIDUAL_RISK_UNKNOWN, nil
	default:
		return 0, fmt.Errorf("unknown residual risk %q; use critical, high, moderate, low, or unknown", s)
	}
}

func parseReviewConfidence(s string) (pb.ReviewConfidence, error) {
	switch strings.ToLower(s) {
	case "high":
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_HIGH, nil
	case "medium":
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_MEDIUM, nil
	case "low":
		return pb.ReviewConfidence_REVIEW_CONFIDENCE_LOW, nil
	default:
		return 0, fmt.Errorf("unknown review confidence %q; use high, medium, or low", s)
	}
}

func parseReviewCompleteness(s string) (pb.ReviewCompleteness, error) {
	switch strings.ToLower(s) {
	case "sufficient":
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_SUFFICIENT, nil
	case "limited":
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_LIMITED, nil
	case "inconclusive":
		return pb.ReviewCompleteness_REVIEW_COMPLETENESS_INCONCLUSIVE, nil
	default:
		return 0, fmt.Errorf("unknown review completeness %q; use sufficient, limited, or inconclusive", s)
	}
}
