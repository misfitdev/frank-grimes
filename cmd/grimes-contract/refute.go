package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/misfitdev/frank-grimes/gen/go/frank_grimes/v2"
	"github.com/misfitdev/frank-grimes/internal/contracts"
	"github.com/misfitdev/frank-grimes/internal/envelope"
	"github.com/misfitdev/frank-grimes/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultRefutationPath is where an in-progress refutation report accumulates.
// One file per run: two passes in a directory would otherwise answer from each
// other's outcomes, and the pass that sealed second would deliver both.
func defaultRefutationPath() string {
	return filepath.Join(store.WorkDir, "refutation-"+store.RunSlug(os.Getenv("GRIMES_RUN_ID"))+".textproto")
}

const refuteUsage = `grimes-contract refute claims|add|seal ...

  refute claims [--claims=<f>] [--print0]
      List the claims this pass must answer: ref, category, anchor, and the
      claim text, four fields per claim.

  refute add --ref=<r> (--refuted|--upheld) --action=<cmd> --cwd=<dir>
      --exit-code=<n> (--output=<excerpt>|--output-sha256=<hex>)
  refute add --ref=<r> --unavailable=<what was missing>
      Record one outcome. Answering a ref twice is refused here rather than
      by the engine.

  refute seal [--raw]
      Emit the report. Run identity and target fingerprint come from the
      environment the engine exported.
`

func cmdRefute(args []string) error {
	if len(args) == 0 {
		return errors.New(refuteUsage)
	}
	switch args[0] {
	case "claims":
		return cmdRefuteClaims(args[1:])
	case "add":
		return cmdRefuteAdd(args[1:])
	case "seal":
		return cmdRefuteSeal(args[1:])
	default:
		return fmt.Errorf("unknown refute subcommand %q\n\n%s", args[0], refuteUsage)
	}
}

// cmdRefuteClaims lists what this pass was asked to answer, exactly as the
// engine wrote it.
//
// A claim is arbitrary text and prototext escapes it. Every refuter grepping
// that rendering back into fields would get the awkward cases wrong in the same
// way, so the decoding happens once, here.
func cmdRefuteClaims(args []string) error {
	fs := flag.NewFlagSet("refute claims", flag.ContinueOnError)
	path := fs.String("claims", os.Getenv("GRIMES_CLAIMS"), "claims to read; defaults to $GRIMES_CLAIMS")
	file := fs.String("file", defaultRefutationPath(), "report this pass will build")
	zero := fs.Bool("print0", false, "separate fields with NUL, for claims containing newlines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	task, err := loadClaims(*path)
	if err != nil {
		return err
	}
	// Reading the claims is where a pass begins. A report left behind by one
	// that died before sealing would otherwise be answered from again, and its
	// outcomes would be delivered as this pass's attacks.
	if err := os.Remove(*file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sep, end := "\t", "\n"
	if *zero {
		sep, end = "\x00", "\x00"
	}
	for _, c := range task.GetClaims() {
		fields := []string{
			c.GetRef(),
			strings.ToLower(strings.TrimPrefix(c.GetCategory().String(), "CATEGORY_")),
			anchorText(c.GetAnchor()),
			c.GetClaim(),
		}
		if _, err := fmt.Print(strings.Join(fields, sep) + end); err != nil {
			return err
		}
	}
	return nil
}

func loadClaims(path string) (*pb.RefutationTask, error) {
	if path == "" {
		return nil, errors.New("no claims file; the engine exports it as $GRIMES_CLAIMS")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	task := &pb.RefutationTask{}
	if err := contracts.UnmarshalCanonical(data, task); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return task, nil
}

// anchorText renders an anchor as the one string a refuter needs to find it.
func anchorText(a *pb.Anchor) string {
	switch {
	case a.GetRepoLine() != nil:
		return a.GetRepoLine().GetPath().GetValue()
	case a.GetDocumentPart() != nil:
		return a.GetDocumentPart().GetSection()
	case a.GetArgumentStep() != nil:
		return fmt.Sprintf("%s step %d", a.GetArgumentStep().GetArgument(), a.GetArgumentStep().GetStep())
	case a.GetRetrievedSource() != nil:
		return a.GetRetrievedSource().GetUri()
	default:
		return ""
	}
}

func cmdRefuteAdd(args []string) error {
	fs := flag.NewFlagSet("refute add", flag.ExitOnError)
	file := fs.String("file", defaultRefutationPath(), "report being built")
	claims := fs.String("claims", os.Getenv("GRIMES_CLAIMS"), "claims this answers; defaults to $GRIMES_CLAIMS")
	ref := fs.String("ref", "", "the claim handle this answers")
	refuted := fs.Bool("refuted", false, "the claim did not survive")
	upheld := fs.Bool("upheld", false, "the claim survived the attack")
	unavailable := fs.String("unavailable", "", "what was missing that stopped the attack")
	action := fs.String("action", "", "command run against the claim")
	cwd := fs.String("cwd", ".", "where the command ran")
	exitCode := fs.Int("exit-code", 0, "what the command exited with")
	output := fs.String("output", "", "bounded excerpt of what it wrote")
	outputSum := fs.String("output-sha256", "", "hex digest of the full output, when the excerpt is not carried")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ref == "" {
		return errors.New("--ref is required; it is the handle the engine issued")
	}
	// The engine drops an answer to a claim it did not issue, so an outcome
	// under a ref from some other pass would go missing without an error.
	task, err := loadClaims(*claims)
	if err != nil {
		return err
	}
	if !issuedClaim(task, *ref) {
		return fmt.Errorf("claim %s was not issued by this pass", *ref)
	}

	outcome := &pb.ClaimOutcome{Ref: *ref}
	switch {
	case *unavailable != "":
		if *refuted || *upheld {
			return errors.New("--unavailable is what happens instead of an outcome, not alongside one")
		}
		outcome.Outcome = &pb.ClaimOutcome_Unavailable{Unavailable: *unavailable}
	case *refuted && *upheld:
		return errors.New("a claim is refuted or upheld, not both")
	case *refuted, *upheld:
		repro, err := reproductionOf("", *action, *cwd, *exitCode, *output, *outputSum)
		if err != nil {
			return err
		}
		if *refuted {
			outcome.Outcome = &pb.ClaimOutcome_Refuted{Refuted: repro}
		} else {
			outcome.Outcome = &pb.ClaimOutcome_Upheld{Upheld: repro}
		}
	default:
		return errors.New("one of --refuted, --upheld, or --unavailable is required")
	}

	report, err := loadRefutation(*file)
	if err != nil {
		return err
	}
	// The contract refuses a report answering one claim twice, and an error
	// naming the ref here is the one a refuter can act on.
	for _, existing := range report.GetOutcomes() {
		if existing.GetRef() == *ref {
			return fmt.Errorf("claim %s has already been answered", *ref)
		}
	}
	report.Outcomes = append(report.Outcomes, outcome)
	return saveRefutation(*file, report)
}

func issuedClaim(task *pb.RefutationTask, ref string) bool {
	for _, c := range task.GetClaims() {
		if c.GetRef() == ref {
			return true
		}
	}
	return false
}

func cmdRefuteSeal(args []string) error {
	fs := flag.NewFlagSet("refute seal", flag.ExitOnError)
	file := fs.String("file", defaultRefutationPath(), "report being built")
	refuterID := fs.String("refuter-id", "", "who mounted these attacks; defaults to $GRIMES_REFUTER_ID, then \"refuter\"")
	runID := fs.String("run-id", "", "run identity; defaults to $GRIMES_RUN_ID")
	targetFP := fs.String("target-fingerprint", "", "hex target fingerprint; defaults to $GRIMES_TARGET_FINGERPRINT")
	raw := fs.Bool("raw", false, "write canonical bytes instead of the envelope")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report, err := loadRefutation(*file)
	if err != nil {
		return err
	}
	report.SchemaMajor = contracts.SchemaMajor
	report.RunId = firstNonEmpty(*runID, os.Getenv("GRIMES_RUN_ID"))
	if report.RunId == "" {
		return errors.New("no run identity; the engine exports it as $GRIMES_RUN_ID")
	}
	report.RefuterId = firstNonEmpty(*refuterID, os.Getenv("GRIMES_REFUTER_ID"), "refuter")
	report.CompletedAt = timestamppb.New(contracts.Now().UTC())

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
		if _, err := os.Stdout.Write(encoded); err != nil {
			return err
		}
	} else if _, err := fmt.Print(envelope.WrapReport(encoded)); err != nil {
		return err
	}

	if !*raw {
		if err := deliverSealed(filepath.Dir(*file), encoded); err != nil {
			return err
		}
	}

	// The outcomes in the working file are the only durable copy, so clearing
	// waits until they have been delivered. A stale file would otherwise be
	// sealed again into the next pass, answering claims that pass never saw.
	if err := os.Remove(*file); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clearing the sealed report: %w", err)
	}
	return nil
}

func loadRefutation(path string) (*pb.RefutationReport, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &pb.RefutationReport{}, nil
	}
	if err != nil {
		return nil, err
	}
	report := &pb.RefutationReport{}
	if err := prototextUnmarshal(data, report); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return report, nil
}

func saveRefutation(path string, report *pb.RefutationReport) error {
	out, err := prototextMarshal(report)
	if err != nil {
		return err
	}
	return contracts.WriteAtomic(path, []byte(out))
}
