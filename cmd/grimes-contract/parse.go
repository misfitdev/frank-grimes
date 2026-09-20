package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func parseSeverity(s string) (pb.Severity, error) {
	switch strings.ToUpper(s) {
	case "P0":
		return pb.Severity_SEVERITY_P0, nil
	case "P1":
		return pb.Severity_SEVERITY_P1, nil
	case "P2":
		return pb.Severity_SEVERITY_P2, nil
	case "P3":
		return pb.Severity_SEVERITY_P3, nil
	default:
		return 0, fmt.Errorf("unknown severity %q; use P0, P1, P2, or P3", s)
	}
}

// parseEase reads the tie-break. Empty is unstated, which is neutral: a report
// that declines to estimate the repair is not worse for it.
func parseEase(s string) (pb.EaseOfFix, error) {
	switch strings.ToLower(s) {
	case "":
		return pb.EaseOfFix_EASE_OF_FIX_UNSPECIFIED, nil
	case "trivial":
		return pb.EaseOfFix_EASE_OF_FIX_TRIVIAL, nil
	case "moderate":
		return pb.EaseOfFix_EASE_OF_FIX_MODERATE, nil
	case "involved":
		return pb.EaseOfFix_EASE_OF_FIX_INVOLVED, nil
	default:
		return 0, fmt.Errorf("unknown ease of fix %q", s)
	}
}

func parseLikelihood(s string) (pb.Likelihood, error) {
	switch strings.ToLower(s) {
	case "likely":
		return pb.Likelihood_LIKELIHOOD_LIKELY, nil
	case "plausible":
		return pb.Likelihood_LIKELIHOOD_PLAUSIBLE, nil
	case "unlikely":
		return pb.Likelihood_LIKELIHOOD_UNLIKELY, nil
	case "unknown":
		return pb.Likelihood_LIKELIHOOD_UNKNOWN, nil
	default:
		return 0, fmt.Errorf("unknown likelihood %q", s)
	}
}

func parseBlast(s string) (pb.BlastRadius, error) {
	switch strings.ToLower(strings.ReplaceAll(s, "_", "-")) {
	case "single-user":
		return pb.BlastRadius_BLAST_RADIUS_SINGLE_USER, nil
	case "local-component":
		return pb.BlastRadius_BLAST_RADIUS_LOCAL_COMPONENT, nil
	case "service":
		return pb.BlastRadius_BLAST_RADIUS_SERVICE, nil
	case "systemic":
		return pb.BlastRadius_BLAST_RADIUS_SYSTEMIC, nil
	default:
		return 0, fmt.Errorf("unknown blast radius %q", s)
	}
}

func parseTargetKind(s string) (pb.TargetKind, error) {
	switch strings.ToLower(s) {
	case "code":
		return pb.TargetKind_TARGET_KIND_CODE, nil
	case "document":
		return pb.TargetKind_TARGET_KIND_DOCUMENT, nil
	case "idea":
		return pb.TargetKind_TARGET_KIND_IDEA, nil
	case "external":
		return pb.TargetKind_TARGET_KIND_EXTERNAL, nil
	default:
		return 0, fmt.Errorf("unknown target kind %q", s)
	}
}

func parseCategories(s string) ([]pb.Category, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []pb.Category
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cat, err := contracts.ParseCategory(strings.ToUpper(name))
		if err != nil {
			return nil, err
		}
		out = append(out, cat)
	}
	return out, nil
}

func hexBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("not hex: %w", err)
	}
	return b, nil
}

// targetFingerprint mirrors what the collector computes, so a report names the
// same target the engine resolves.
func targetFingerprint(root, scope string) []byte {
	sum := sha256.Sum256([]byte(root + "\x00" + scope))
	return sum[:]
}

func runID2() string {
	return fmt.Sprintf("report-%d-%d", contracts.Now().UTC().Unix(), os.Getpid())
}

func prototextUnmarshal(b []byte, m proto.Message) error {
	return prototext.Unmarshal(b, m)
}

// anchorLabel names an anchor for a progress line.
func anchorLabel(a *pb.Anchor) string {
	kind, parts := contracts.AnchorKey(a)
	return kind + ":" + strings.Join(parts, "/")
}

// wasSetIn reports whether a flag was given on the command line, so an
// environment default does not override an explicit one.
func wasSetIn(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
